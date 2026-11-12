package pipeline_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/paragraph"
	"tytui/pipeline"
)

func TestLineHeightPreservesShapingPaintAndFallback(t *testing.T) {
	for _, b := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, paint := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
			e := styledInput(t, `<p dir="rtl" style="font-size:12pt;color:navy">אב&shy;גד <span dir="ltr" lang="en" style="font-size:20pt;color:red">f<span style="color:blue">i café</span></span> שָׁלוֹם hello</p>`)
			in := pipeline.Input{Text: e.Text, Source: e, Width: 80 * dimen.BP, Breaker: b, PaintPolicy: paint, Fonts: &discovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}}}}
			before, err := pipeline.Run(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			idx := uint64(strings.Index(e.Text, "fi"))
			in.LineLayout = &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}, Runs: []paragraph.LineHeightSpan{{Start: idx + 1, End: idx + 2, Height: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 3}}}}
			after, err := pipeline.Run(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.BreakPositions, after.BreakPositions) || !reflect.DeepEqual(before.FontBindings, after.FontBindings) || !reflect.DeepEqual(before.BidiRuns, after.BidiRuns) || before.Frozen.KnotCount() != after.Frozen.KnotCount() {
				t.Fatal("line-height altered horizontal input/output")
			}
			if len(after.Fonts) < 2 {
				t.Fatal("fixture does not cover fallback")
			}
			for k := 0; k < after.Frozen.KnotCount(); k++ {
				a, _ := before.Frozen.Knot(k)
				c, _ := after.Frozen.Knot(k)
				if a != c {
					t.Fatal("source/geometry changed")
				}
				x, _ := before.Frozen.CopyText(k)
				y, _ := after.Frozen.CopyText(k)
				if !reflect.DeepEqual(x.Glyphs, y.Glyphs) || !reflect.DeepEqual(x.Clusters, y.Clusters) || x.Context.EmSize != y.Context.EmSize || x.ReplacementText != y.ReplacementText {
					t.Fatal("reshaped at line-height boundary")
				}
			}
			p, err := pipeline.BuildGlyphProof(before)
			if err != nil {
				t.Fatal(err)
			}
			q, err := pipeline.BuildGlyphProof(after)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Glyphs) != len(q.Glyphs) {
				t.Fatal("glyph count changed")
			}
			for i, g := range p.Glyphs {
				h := q.Glyphs[i]
				if g.GlyphID != h.GlyphID || g.Cluster != h.Cluster || g.X != h.X || g.Color != h.Color || g.Scale != h.Scale || g.Path != h.Path {
					t.Fatal("paint or glyph shape changed")
				}
			}
			if before.VList().Box() == after.VList().Box() {
				t.Fatal("line-height not used")
			}
			for i, l := range after.VList().Lines() {
				if l.Gap != 0 || q.Lines[i].Height != l.Height || q.Lines[i].Baseline != l.Baseline {
					t.Fatal("renderer ignored placements")
				}
			}
			if len(after.VList().Materials()) != after.Frozen.LineCount() {
				t.Fatal("trailing kern in CSS mode")
			}
			prepared, _, err := after.Text.StyleAt(idx)
			if err != nil {
				t.Fatal(err)
			}
			requireTypographyOnly(t, prepared)
			in.LineLayout.Default.Factor = 100
			after.Input.LineLayout.Runs[0].Height.Factor = 100
			copy := after.LineLayout()
			copy.Default.Factor = 100
			copy.Runs = nil
			again, err := pipeline.BuildGlyphProof(after)
			if err != nil || !reflect.DeepEqual(q, again) || after.LineLayout().Default.Factor != 1.5 {
				t.Fatal("mutable policy altered retained proof", err)
			}
		}
	}
}

func TestLineHeightInsideColoredLigature(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, policy := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
			e := styledInput(t, `<p><span style="color:red">f</span><span style="color:blue">i</span></p>`)
			r, err := pipeline.Run(t.Context(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, PaintPolicy: policy, Fonts: &discovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}}, LineLayout: &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}, Runs: []paragraph.LineHeightSpan{{Start: 1, End: 2, Height: paragraph.LineHeight{Kind: paragraph.LengthHeight, Length: 60 * dimen.BP}}}}})
			if err != nil {
				t.Fatal(err)
			}
			proof, err := pipeline.BuildGlyphProof(r)
			if err != nil {
				t.Fatal(err)
			}
			colors := []string{}
			for _, g := range proof.Glyphs {
				colors = append(colors, g.Color)
			}
			want := []string{"red"}
			if policy == typography.LegacyStyleBoundary {
				want = []string{"red", "blue"}
			}
			if !reflect.DeepEqual(colors, want) || r.VList().Box().Extent != 60 {
				t.Fatal("ligature paint or line-height", colors, r.VList().Box())
			}
		}
	}
}
