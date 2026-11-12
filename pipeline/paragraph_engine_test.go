package pipeline_test

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"

	"tytui/pipeline"
)

type publicEngineHyphens struct{}

func (publicEngineHyphens) Discretionaries(word string, _ *khipu.Params) ([]khipu.HyphenationFragment, error) {
	if word == "\u00ad" {
		return []khipu.HyphenationFragment{{PreBreak: "-"}}, nil
	}
	return nil, nil
}

func TestPublicParagraphEngineMatchesDOMWorkbench(t *testing.T) {
	// The direct input below is constructed without any htmlinput types/callbacks.
	// The DOM path is a separate producer of the same semantic content.
	const source = `<p dir="rtl" lang="he" style="font-family:serif;font-size:12pt;color:navy">אב&shy;גד <span dir="ltr" lang="en" style="font-size:20pt;color:red">office 12</span> הו</p>`
	const content = "אב\u00adגד office 12 הו"
	start := uint64(strings.Index(content, "office"))
	end := start + uint64(len("office 12"))
	base := style.Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "navy", Language: "he", Direction: bidi.RightToLeft, WhiteSpace: "normal", UnicodeBidi: "isolate"}
	inner := base
	inner.EmSize = 20 * dimen.BP
	inner.Direction = bidi.LeftToRight
	inner.Language = "en"
	inner.Color = "red"
	inner.UnicodeBidi = "isolate"
	spans := []style.Span{{Start: 0, End: start, Style: base}, {Start: start, End: end, Style: inner}, {Start: end, End: uint64(len(content)), Style: base}}
	markup := func(at uint64) int {
		if at == start {
			return bidi.MarkupLRI
		}
		if at == end {
			return bidi.MarkupPDI
		}
		return 0
	}
	settings := typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}}
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, paint := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
			for _, width := range []dimen.DU{20 * dimen.BP, 200 * dimen.BP} {
				e := styledInput(t, source)
				if e.Text != content {
					t.Fatal("fixture content mismatch")
				}
				workbench, err := pipeline.Run(t.Context(), pipeline.Input{Text: e.Text, Source: e, Width: width, Breaker: breaker, PaintPolicy: paint, Fonts: &discovery.Resolution{Settings: settings}})
				if err != nil {
					t.Fatal(err)
				}
				resolver, err := typography.LoadSettings(t.Context(), settings, typography.Services{Loader: otadapter.Services{}, Coverage: otadapter.Services{}})
				if err != nil {
					t.Fatal(err)
				}
				text, bindings, err := resolver.ItemizePolicy(t.Context(), content, spans, paint)
				if err != nil {
					t.Fatal(err)
				}
				prepared, _, err := text.StyleAt(start)
				if err != nil {
					t.Fatal(err)
				}
				requireTypographyOnly(t, prepared)
				// Preparation belongs to M; P consumes these exact itemized style identities.
				out, err := paragraph.Run(t.Context(), paragraph.Input{Text: text, Direction: bidi.RightToLeft, Markup: markup}, paragraph.Params{
					Constraints: paragraph.Constraints{Width: width}, Breaker: paragraph.Breaker(breaker), Assembly: paragraph.Options{Policy: paragraph.ProofCompatibility, DefaultEmSize: settings.Size},
				}, khipu.Config{Streaming: &khipu.StreamingConfig{NewShaper: func() (contracts.StreamingShaper, error) { return otadapter.NewStream(8) }, Resolver: resolver, Defaults: resolver.Defaults()}, Hyphenator: publicEngineHyphens{}})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(out.BreakPositions, workbench.BreakPositions) || !reflect.DeepEqual(out.BidiRuns, workbench.BidiRuns) || !reflect.DeepEqual(bindings, workbench.FontBindings) || out.VList.Box() != workbench.VList().Box() || !reflect.DeepEqual(out.VList.Lines(), workbench.VList().Lines()) {
					t.Fatalf("%s %s width=%v: breaks=%v bidi=%v bindings=%v box=%v lines=%v styles %#v manual %#v", breaker, paint, width, reflect.DeepEqual(out.BreakPositions, workbench.BreakPositions), reflect.DeepEqual(out.BidiRuns, workbench.BidiRuns), reflect.DeepEqual(bindings, workbench.FontBindings), out.VList.Box() == workbench.VList().Box(), reflect.DeepEqual(out.VList.Lines(), workbench.VList().Lines()), e.Styles, spans)
				}
				if out.Frozen.KnotCount() != workbench.Frozen.KnotCount() {
					t.Fatal("knot count differs")
				}
				for k := 0; k < out.Frozen.KnotCount(); k++ {
					a, _ := out.Frozen.Knot(k)
					b, _ := workbench.Frozen.Knot(k)
					if a != b {
						t.Fatal("source/geometry differs", k)
					}
					x, ok := out.Frozen.CopyText(k)
					y, ok2 := workbench.Frozen.CopyText(k)
					if ok != ok2 || !reflect.DeepEqual(x.Glyphs, y.Glyphs) || !reflect.DeepEqual(x.Clusters, y.Clusters) || x.ReplacementText != y.ReplacementText || x.Context.Input != y.Context.Input || x.Context.EmSize != y.Context.EmSize {
						t.Fatal("glyphs or source correspondence differ", k)
					}
					if ok {
						ix, err := (otadapter.Services{}).Identify(x.Context.Params.Font)
						if err != nil {
							t.Fatal(err)
						}
						iy, err := (otadapter.Services{}).Identify(y.Context.Params.Font)
						if err != nil || ix.ResourceID != iy.ResourceID {
							t.Fatal("font selection differs", err)
						}
					}
				}
				reader, err := out.Frozen.SourceReader(khipu.ByteRange{Start: 0, End: out.Frozen.SourceLen()})
				if err != nil {
					t.Fatal(err)
				}
				raw, err := io.ReadAll(reader)
				if err != nil || string(raw) != content {
					t.Fatal("source changed", err)
				}
				// Returned renderer paint remains application-owned, never part of P's input.
				proof, err := pipeline.BuildGlyphProof(workbench)
				if err != nil || len(proof.Glyphs) == 0 {
					t.Fatal("proof failure", err)
				}
			}
		}
	}
}
