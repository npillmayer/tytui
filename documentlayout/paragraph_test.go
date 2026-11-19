package documentlayout_test

import (
	"context"
	"strings"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/fontselection"
)

type syntheticLoader struct{}

func (syntheticLoader) Load(ctx context.Context, path string, r *fontresource.Resource) (glyphing.FaceEntry, error) {
	if err := ctx.Err(); err != nil {
		return glyphing.FaceEntry{}, err
	}
	var err error
	if r == nil {
		r, err = fontresource.Read(strings.NewReader("synthetic-test"), fontresource.Origin{Provider: "test", Location: path})
		if err != nil {
			return glyphing.FaceEntry{}, err
		}
	}
	return glyphing.FaceEntry{Face: helpers.NewSyntheticFont(), Resource: r, Info: fontmeta.Info{ResourceID: r.ID(), SHA256: r.SHA256(), Origin: r.Origin(), Path: r.Origin().Location}}, nil
}

func TestExtractedGroupsFeedPreparedParagraphs(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;width:20px;font-size:30px">éאב</span></div>`)
	var markerQueries int
	r, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "synthetic", Size: 10 * dimen.BP}, testServices{Loader: syntheticLoader{}, Coverage: glyphing.CoverageFuncs{Check: checkSyntheticFace, Lookup: func(_ contracts.FontFace, c rune) (uint32, error) {
		if c == '\uFFFC' {
			markerQueries++
		}
		return 1, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	groups := extractAll(t, d)
	for groupIndex, g := range groups {
		text, bindings, err := r.ItemizeInline(g.Text, g.Styles, g.Anchors)
		if err != nil {
			t.Fatal(err)
		}
		if groupIndex == 0 && len(bindings) != 0 {
			t.Fatal("object acquired text binding")
		}
		// Resolve the explicit inherited owner context, never the first style
		// or glyph. The outer strut is 15bp; its atomic child has 22.5bp text.
		defaults, err := r.ResolveStyle(g.Paragraph.Style, r.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		markup, err := g.BidiMarkup()
		if err != nil {
			t.Fatal(err)
		}
		p, err := paragraph.Prepare(paragraph.Input{Text: text, Direction: g.Paragraph.Style.Direction, Markup: markup, Atomics: g.Anchors}, khipu.PrepareOptions{Hyphenation: khipu.NoHyphenation}, khipu.Config{Streaming: &khipu.StreamingConfig{Resolver: r, Defaults: defaults, NewShaper: func() (contracts.StreamingShaper, error) { return helpers.NewSyntheticStreamingShaper(8) }}})
		if err != nil {
			t.Fatal(err)
		}
		var atoms []paragraph.AtomicBox
		for _, anchor := range g.Anchors {
			atoms = append(atoms, paragraph.AtomicBox{ID: anchor.ID, BorderWidth: 20 * dimen.BP, Alignment: paragraph.AlignBottomMarginEdge})
		}
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			out, err := p.Layout(paragraph.LayoutRequest{Params: paragraph.Params{Constraints: paragraph.Constraints{Width: 100 * dimen.BP}, Breaker: breaker, Assembly: paragraph.Options{Policy: paragraph.CSSLineHeight}, LineLayout: &g.LineLayout}, Horizontal: &paragraph.HorizontalLayout{}, Atomics: atoms})
			if err != nil {
				t.Fatal(err)
			}
			want := 15.0
			if groupIndex == 1 {
				want = 22.5
			}
			if out.VList.Box().Extent != want || out.VList.Box().Baseline != want*.8 || len(out.VList.AtomicPlacements()) != len(g.Anchors) {
				t.Fatal("owner context or atomic association lost", g.GroupID, out.VList.Box())
			}
			if out.Frozen.SourceLen() != uint64(len(g.Text)) {
				t.Fatal("source coordinates changed")
			}
		}
	}
	if markerQueries != 0 {
		t.Fatal("atomic glyph queried")
	}
}
