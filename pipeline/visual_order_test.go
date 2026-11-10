package pipeline_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/opentype/otshape"
	"github.com/npillmayer/uax/bidi"

	"tytui/fontbackend"
	"tytui/pipeline"
)

// This checks compatibility with the upstream ordering contract. The independent
// HarfBuzz expectations in TestIndependentGlyphProofs remain the shaping oracle.
func TestFrozenGlyphProofVisualOrderCompatibility(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, tc := range []struct {
			name, text, font string
			direction        bidi.Direction
			width            dimen.DU
			lines            int
			pointed          bool
			replacement      bool
		}{
			{"pointed_hebrew", "שָׁלוֹם", hebrew, bidi.RightToLeft, 100 * dimen.BP, 1, true, false},
			{"repeated_cluster_anchor", "שָׁ", hebrew, bidi.RightToLeft, 100 * dimen.BP, 1, true, false},
			{"discretionary_selected", "אב\u00adגד", hebrew, bidi.RightToLeft, 20 * dimen.BP, 2, false, true},
			{"discretionary_unbroken", "אב\u00adגד", hebrew, bidi.RightToLeft, 100 * dimen.BP, 1, false, false},
			{"ltr_control", "office", gentium, bidi.LeftToRight, 100 * dimen.BP, 1, false, false},
		} {
			t.Run(string(breaker)+"/"+tc.name, func(t *testing.T) {
				r, err := pipeline.Run(context.Background(), pipeline.Input{
					Text: tc.text, Direction: tc.direction, Width: tc.width, Breaker: breaker,
					Fonts: &fontbackend.Settings{DefaultPath: tc.font, Size: 12 * dimen.BP},
				})
				if err != nil {
					t.Fatal(err)
				}
				if r.Frozen.LineCount() != tc.lines {
					t.Fatalf("lines = %d, want %d", r.Frozen.LineCount(), tc.lines)
				}
				before := make(map[int]khipu.FrozenText)
				for k := 0; k < r.Frozen.KnotCount(); k++ {
					if snapshot, ok := r.Frozen.CopyText(k); ok {
						before[k] = snapshot
					}
				}
				proof, err := pipeline.BuildGlyphProof(r)
				if err != nil {
					t.Fatal(err)
				}
				glyphIndex := 0
				repeated, offset, replacement, visiblePreBreak := false, false, false, false
				for li := 0; li < r.Frozen.LineCount(); li++ {
					line, _ := r.Frozen.Line(li)
					x := line.LeftSkip.Points()
					// Frozen knots are already in physical order. VisualOrder applies
					// only to each retained logical glyph fragment within a knot.
					for k := line.FirstKnot; k < line.FirstKnot+line.KnotCount; k++ {
						knot, _ := r.Frozen.Knot(k)
						if snapshot, ok := before[k]; ok {
							anchors := make(map[uint32]bool)
							for _, g := range snapshot.Glyphs {
								repeated = repeated || anchors[g.Cluster]
								anchors[g.Cluster] = true
								offset = offset || g.XOffset != 0 || g.YOffset != 0
							}
							if snapshot.IsReplacement {
								replacement = true
								if knot.Material == khipu.PreBreakMaterial && snapshot.ReplacementText != "" && len(snapshot.Glyphs) > 0 {
									visiblePreBreak = true
								}
							}
							visual, err := otshape.VisualOrder(snapshot.Glyphs, snapshot.Context.Params.Direction, false)
							if err != nil {
								t.Fatalf("knot %d: %v", k, err)
							}
							scale := snapshot.Context.Scale.ToDU(1) / float64(dimen.BP)
							penX, penY := x, proof.Lines[li].Baseline
							for _, want := range visual {
								if glyphIndex >= len(proof.Glyphs) {
									t.Fatal("renderer omitted glyphs")
								}
								got := proof.Glyphs[glyphIndex]
								if got.Line != li || got.Knot != k || got.GlyphID != want.GlyphID || got.Cluster != want.Cluster {
									t.Fatalf("glyph %d: %+v, want line %d knot %d record %+v", glyphIndex, got, li, k, want)
								}
								wantX, wantY := penX+float64(want.XOffset)*scale, penY-float64(want.YOffset)*scale
								if math.Abs(got.X-wantX) > 1e-7 || math.Abs(got.Y-wantY) > 1e-7 || got.Scale != scale {
									t.Fatalf("glyph %d: position (%g,%g) scale %g, want (%g,%g) scale %g", glyphIndex, got.X, got.Y, got.Scale, wantX, wantY, scale)
								}
								penX += float64(want.XAdvance) * scale
								penY -= float64(want.YAdvance) * scale
								glyphIndex++
							}
						}
						x += knot.W.Points()
					}
				}
				if glyphIndex != len(proof.Glyphs) {
					t.Fatalf("renderer emitted %d extra glyphs", len(proof.Glyphs)-glyphIndex)
				}
				if tc.pointed && (!repeated || !offset) {
					t.Fatal("fixture must exercise repeated anchors and positioned marks")
				}
				if replacement != tc.replacement || visiblePreBreak != tc.replacement {
					t.Fatalf("replacement = %t, visible pre-break = %t, want %t", replacement, visiblePreBreak, tc.replacement)
				}
				again, err := pipeline.BuildGlyphProof(r)
				if err != nil || !reflect.DeepEqual(proof, again) {
					t.Fatalf("repeated rendering changed proof: %v", err)
				}
				for k, want := range before {
					got, ok := r.Frozen.CopyText(k)
					if !ok || !reflect.DeepEqual(got, want) {
						t.Fatalf("rendering mutated frozen text at knot %d", k)
					}
				}
			})
		}
	}
}
