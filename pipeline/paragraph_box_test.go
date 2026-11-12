package pipeline_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/discovery"

	"tytui/pipeline"
)

// Exercise the retained-placement/rendering boundary with real mixed-size fonts.
// Independent synthetic fixtures and static replay evidence are the geometry oracles.
func TestBoxedVListPreservesProofGeometry(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, policy := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
			for _, width := range []dimen.DU{90 * dimen.BP, 600 * dimen.BP} {
				e := styledInput(t, `<p dir="rtl" lang="he" style="font-size:13.3pt;color:navy">שָׁלוֹם אב&shy;גד <span lang="en" style="font-size:24pt;color:red">office café</span> עולם hello שָׁלוֹם</p>`)
				r, err := pipeline.Run(t.Context(), pipeline.Input{Text: e.Text, Source: e, Width: width, Breaker: breaker, PaintPolicy: policy, Fonts: &discovery.Resolution{Settings: typography.Settings{
					DefaultPath: "../../opentype/testdata/fonts/GentiumPlus-R.ttf", Size: 12 * dimen.BP, Fallbacks: []string{hebrew},
				}}})
				if err != nil {
					t.Fatal(err)
				}
				if width == 90*dimen.BP && r.Frozen.LineCount() < 2 {
					t.Fatal("fixture must cover multiple lines")
				}
				faces := map[string]bool{}
				for _, b := range r.FontBindings {
					faces[b.ResourceID] = true
				}
				if len(faces) < 2 {
					t.Fatal("fixture must exercise fallback")
				}
				before, err := pipeline.BuildGlyphProof(r)
				if err != nil {
					t.Fatal(err)
				}
				paint := r.PaintSpans()
				var text []khipu.FrozenText
				for k := 0; k < r.Frozen.KnotCount(); k++ {
					tx, _ := r.Frozen.CopyText(k)
					text = append(text, tx)
				}
				v := r.VList()
				if v == nil {
					t.Fatal("missing retained placements")
				}
				lines := v.Lines()
				if len(lines) != len(before.Lines) || v.Frozen() != r.Frozen {
					t.Fatal("lost frozen content")
				}
				widthWant, extentWant := 0.0, 0.0
				for i, l := range lines {
					p := before.Lines[i]
					if l.X != p.X || l.Top != p.Top || l.Baseline != p.Baseline || l.Height != p.Height || l.Horizontal.Width.Points() != p.Width || l.Horizontal.TargetWidth.Points() != p.TargetWidth {
						t.Fatalf("%s %s line %d: assembly %+v proof %+v", breaker, policy, i, l, p)
					}
					original, _ := r.Frozen.Line(i)
					if !reflect.DeepEqual(l.Horizontal, original) {
						t.Fatal("horizontal geometry or source changed")
					}
					widthWant = math.Max(widthWant, math.Max(p.Width, p.TargetWidth))
					extentWant = p.Top + p.Height
				}
				box := v.Box()
				if box.Width != widthWant || box.Extent != extentWant || box.Baseline != before.Lines[0].Baseline || !box.HasBaseline {
					t.Fatalf("box %+v", box)
				}
				lines[0].Baseline = -1
				material := v.Materials()
				material[0].Extent = -1
				if v.Lines()[0].Baseline < 0 || v.Materials()[0].Extent < 0 {
					t.Fatal("aliased placement slices")
				}
				for k, want := range text {
					got, _ := r.Frozen.CopyText(k)
					if !reflect.DeepEqual(got, want) {
						t.Fatal("assembly changed glyph/source snapshot")
					}
				}
				after, err := pipeline.BuildGlyphProof(r)
				if err != nil || !reflect.DeepEqual(after, before) || !reflect.DeepEqual(r.PaintSpans(), paint) {
					t.Fatal("assembly changed proof or renderer paint", err)
				}
			}
		}
	}
}
