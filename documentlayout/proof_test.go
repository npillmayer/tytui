package documentlayout_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	fontmeta "github.com/npillmayer/typography/font"
	ot "github.com/npillmayer/typography/glyphing/adapter/opentype"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/render"
	renderot "github.com/npillmayer/typography/render/adapter/opentype"
	xbidi "golang.org/x/text/unicode/bidi"

	"tytui/documentlayout"
	"tytui/fontselection"
)

type cellProof struct{ fail error }

func (cellProof) Identify(contracts.FontFace) (fontmeta.Info, error) {
	return fontmeta.Info{SHA256: "synthetic"}, nil
}
func (c cellProof) Outline(contracts.FontFace, uint32) (render.Outline, error) {
	return render.Outline{Path: "M0 0 L1 1", Points: []render.Point{{}, {X: 1, Y: 1}}}, c.fail
}
func (cellProof) VisualOrder(g []contracts.ShapedGlyph, d xbidi.Direction) ([]contracts.ShapedGlyph, error) {
	if d == xbidi.RightToLeft {
		slices.Reverse(g)
	}
	return g, nil
}
func cells() render.Services { b := cellProof{}; return render.Services{Backend: b, Identity: b} }

func TestDocumentProofIndependentAncestorCoordinates(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px;padding:7px;border-width:3px;border-style:solid"><div style="margin:13px 0 0 11px;padding:5px;border-width:2px;border-style:solid">before<div style="margin:0">middle</div><span style="display:inline-block;width:30px;font-size:10px;padding:2px;border-width:1px;border-style:solid;margin:4px 5px 6px -2px">x</span></div></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 300, breaker, layoutConfig(t))
		p, err := documentlayout.BuildProof(t.Context(), r, cells())
		if err != nil {
			t.Fatal(err)
		}
		// Independently: root content (10,10), child content (18,20), anonymous
		// owner y=40, atomic border (-2,4), inner edges (3,3), baseline=8 CSSpx.
		found := false
		for _, g := range p.Glyphs {
			if g.GlyphID == 'x'+1 {
				found = true
				if g.X != 21.75 || g.Y != 63.75 || g.SourceByte != 0 || len(g.Sources) != 1 || g.Sources[0].NodeStart != 0 {
					t.Fatal("glyph transform/source", g)
				}
			}
		}
		if !found || len(p.Atomics) != 1 || p.Atomics[0].X != 19.5 || p.Atomics[0].Y != 55.5 {
			t.Fatal("atomic transform", p.Atomics)
		}
		var svg bytes.Buffer
		if err := documentlayout.WriteSVG(t.Context(), &svg, r, cells()); err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(&svg)
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDocumentProofBidiAndAtomicSourceOrder(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		for _, dir := range []string{"ltr", "rtl"} {
			d, _, _ := build(t, `<div id="root" dir="`+dir+`" style="font-size:20px">ab <span style="display:inline-block;width:20px;font-size:10px" dir="ltr">x</span> אב</div>`)
			r := runDocument(t, d, 100, breaker, layoutConfig(t))
			p, err := documentlayout.BuildProof(t.Context(), r, cells())
			if err != nil {
				t.Fatal(err)
			}
			var text string
			var xs []float64
			rootGroup := d.Groups()[0].GroupID
			for _, g := range p.Glyphs {
				if g.GroupID == rootGroup {
					text += string(rune(g.GlyphID - 1))
					xs = append(xs, g.X)
					if g.Y != 12 {
						t.Fatal("restacked baseline", g)
					}
				}
			}
			wantText := "abבא"
			wantX := []float64{0, 7.5, 45, 52.5}
			atomX := 22.5
			if dir == "rtl" {
				wantText = "באab"
				wantX = []float64{15, 22.5, 60, 67.5}
				atomX = 37.5
			}
			if text != wantText || !reflect.DeepEqual(xs, wantX) || len(p.Atomics) != 1 || p.Atomics[0].X != atomX {
				t.Fatalf("%s/%s: %s %v atoms=%v", breaker, dir, text, xs, p.Atomics)
			}
			for _, g := range p.Glyphs {
				if len(g.Sources) == 0 {
					t.Fatal("lost glyph source")
				}
				if rune(g.GlyphID-1) == '\uFFFC' {
					t.Fatal("atomic marker drawn as text")
				}
			}
		}
	}
}

func TestDocumentProofRetainedMultilineBaselines(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px;padding:4px">ab ab ab ab</div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 28, breaker, layoutConfig(t))
		p, err := documentlayout.BuildProof(t.Context(), r, cells())
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Lines) != 4 {
			t.Fatalf("%s: expected four retained lines, got %d", breaker, len(p.Lines))
		}
		for n, l := range p.Lines {
			if l.Top != 3+float64(n)*15 || l.Baseline != 15+float64(n)*15 {
				t.Fatal("lines restacked", n, l)
			}
		}
		for _, g := range p.Glyphs {
			if g.Y != 15+float64(g.Line)*15 {
				t.Fatal("glyph baseline restacked", g)
			}
		}
	}
}

func realConfig(t *testing.T) documentlayout.Config {
	t.Helper()
	s := ot.Services{}
	r, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "../../opentype/testdata/fonts/GentiumPlus-R.ttf", Fallbacks: []string{"../../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf"}, Size: 15 * dimen.BP}, testServices{Loader: s, Coverage: s})
	if err != nil {
		t.Fatal(err)
	}
	return documentlayout.Config{Resolver: r, NewShaper: func() (contracts.StreamingShaper, error) { return ot.NewStream(64) }}
}

//go:embed testdata/nested.html
var nestedFixture string

func TestRealDocumentFixtureAndSnapshotLifetime(t *testing.T) {
	d, input, _ := build(t, nestedFixture)
	cfg := realConfig(t)
	services := render.Services{Backend: renderot.New(), Identity: renderot.New()}
	var baseline *documentlayout.Result
	var firstSVG []byte
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		for _, width := range []float32{420, 260, 420} {
			r := runDocument(t, d, width, breaker, cfg)
			p, err := documentlayout.BuildProof(t.Context(), r, services)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Atomics) != 6 {
				t.Fatalf("atomic interiors duplicated or lost: %d", len(p.Atomics))
			}
			seen := map[uint64]bool{}
			fonts := map[string]bool{}
			colors := map[string]bool{}
			for _, a := range p.Atomics {
				if seen[uint64(a.BoxID)] {
					t.Fatal("duplicate atomic")
				}
				seen[uint64(a.BoxID)] = true
			}
			for _, g := range p.Glyphs {
				fonts[g.FaceSHA256] = true
				colors[g.Color] = true
				if g.Path == "" || len(g.Sources) == 0 {
					t.Fatal("missing outline/provenance", g)
				}
				for _, s := range g.Sources {
					if !(s.Start <= g.SourceByte && g.SourceByte < s.End) {
						t.Fatal("glyph mapped outside source")
					}
				}
			}
			if len(fonts) != 2 || !colors["#991b1b"] || !colors["#172554"] {
				t.Fatal("font/paint correspondence", fonts, colors)
			}
			var svg bytes.Buffer
			if err := documentlayout.WriteSVG(t.Context(), &svg, r, services); err != nil {
				t.Fatal(err)
			}
			if baseline == nil {
				baseline = r
				firstSVG = bytes.Clone(svg.Bytes())
			}
			if breaker == paragraph.FirstFit && width == 420 && !bytes.Equal(svg.Bytes(), firstSVG) {
				t.Fatal("repeated layout changed proof")
			}
		}
	}
	// The source DOM and mutable returned envelopes are no longer needed.
	input.Nodes = nil
	group, _ := baseline.Group(baseline.Groups()[0].GroupID)
	group.Input.Text = "removed"
	group.Input.Paint = nil
	var again bytes.Buffer
	if err := documentlayout.WriteSVG(t.Context(), &again, baseline, services); err != nil || !bytes.Equal(firstSVG, again.Bytes()) {
		t.Fatal("old snapshot changed", err)
	}
}

func TestRealGlyphsAgainstIndependentHarfBuzzFixtures(t *testing.T) {
	var corpus struct {
		Cases []struct {
			ID, Text string
			Hash     string `json:"font_sha256"`
			Fixture  struct {
				Output []struct {
					G, Cl          uint32
					DX, DY, AX, AY int32
				}
			}
		}
	}
	data, err := os.ReadFile("../../opentype/internal/hbcmp/testdata/text/baselines.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, c := range corpus.Cases {
		if c.ID != "latin_office_liga_on" && c.ID != "hebrew_shalom_pointed" {
			continue
		}
		checked++
		dir, lang := "ltr", "en"
		if strings.HasPrefix(c.ID, "hebrew") {
			dir, lang = "rtl", "he"
		}
		d, _, _ := build(t, `<div id="root" style="font-size:24px;padding:4px">a <span style="display:inline-block;width:180px;padding:3px" dir="`+dir+`" lang="`+lang+`">`+c.Text+`</span> z</div>`)
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			r := runDocument(t, d, 300, breaker, realConfig(t))
			p, err := documentlayout.BuildProof(t.Context(), r, render.Services{Backend: renderot.New(), Identity: renderot.New()})
			if err != nil {
				t.Fatal(err)
			}
			var glyphs []documentlayout.ProofGlyph
			var baseline float64
			for _, group := range r.Groups() {
				g, _ := r.Group(group.GroupID)
				if g.Input.Text == c.Text {
					for _, v := range p.Glyphs {
						if v.GroupID == group.GroupID {
							glyphs = append(glyphs, v)
						}
					}
					for _, l := range p.Lines {
						if l.GroupID == group.GroupID {
							baseline = l.Baseline
						}
					}
				}
			}
			if len(glyphs) != len(c.Fixture.Output) {
				t.Fatal(c.ID, "glyph count", len(glyphs))
			}
			x := glyphs[0].X - float64(c.Fixture.Output[0].DX)*glyphs[0].Scale
			y := baseline
			offsets := []uint64{}
			for pos := range c.Text {
				offsets = append(offsets, uint64(pos))
			}
			for i, w := range c.Fixture.Output {
				g := glyphs[i]
				if g.GlyphID != w.G || g.Cluster != w.Cl || g.FaceSHA256 != c.Hash || g.SourceByte != offsets[w.Cl] {
					t.Fatal(c.ID, "independent glyph/source", g, w)
				}
				if math.Abs(g.X-(x+float64(w.DX)*g.Scale)) > 1e-7 || math.Abs(g.Y-(y-float64(w.DY)*g.Scale)) > 1e-7 {
					t.Fatal(c.ID, "glyph position", g)
				}
				x += float64(w.AX) * g.Scale
				y -= float64(w.AY) * g.Scale
			}
		}
	}
	if checked != 2 {
		t.Fatal("missing independent baselines")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestDocumentProofFailuresWriteNothing(t *testing.T) {
	d, _, _ := build(t, `<div id="root">abc</div>`)
	r := runDocument(t, d, 100, paragraph.FirstFit, layoutConfig(t))
	sentinel := errors.New("outline unavailable")
	b := cellProof{fail: sentinel}
	var out bytes.Buffer
	err := documentlayout.WriteSVG(t.Context(), &out, r, render.Services{Backend: b, Identity: b})
	if !errors.Is(err, sentinel) || out.Len() != 0 {
		t.Fatal("partial proof written", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = documentlayout.WriteSVG(ctx, &out, r, cells())
	if !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled proof written", err)
	}
	if err := documentlayout.WriteSVG(t.Context(), shortWriter{}, r, cells()); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short write ignored", err)
	}
	if p, err := documentlayout.BuildProof(t.Context(), r, render.Services{}); p != nil || err == nil {
		t.Fatal("renderer silently installed")
	}
}
