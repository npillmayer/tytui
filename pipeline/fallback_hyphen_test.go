package pipeline_test

import (
	"context"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"io"
	"reflect"
	"testing"
	"tytui/fontbackend"
	"tytui/pipeline"
)

func TestFallbackPreservesSoftHyphenBreaks(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, width := range []dimen.DU{20 * dimen.BP, 100 * dimen.BP} {
			t.Run(string(breaker)+"/"+width.String(), func(t *testing.T) {
				source := "אב\u00adגד"
				input := pipeline.Input{Text: source, Direction: bidi.RightToLeft, Width: width, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultPath: hebrew, Size: 12 * dimen.BP}}
				direct, err := pipeline.Run(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				input.Fonts = &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}}
				fallback, err := pipeline.Run(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				wantLines := 1
				if width == 20*dimen.BP {
					wantLines = 2
				}
				if fallback.Frozen.LineCount() != wantLines {
					t.Fatalf("fallback lines=%d want %d", fallback.Frozen.LineCount(), wantLines)
				}
				if !reflect.DeepEqual(direct.BreakPositions, fallback.BreakPositions) || direct.Frozen.KnotCount() != fallback.Frozen.KnotCount() {
					t.Fatal("fallback changed breakpoints or knots")
				}
				for i := 0; i < direct.Frozen.LineCount(); i++ {
					a, _ := direct.Frozen.Line(i)
					b, _ := fallback.Frozen.Line(i)
					if a != b {
						t.Fatalf("line %d differs: %+v %+v", i, a, b)
					}
				}
				replacement := false
				for i := 0; i < direct.Frozen.KnotCount(); i++ {
					a, _ := direct.Frozen.Knot(i)
					b, _ := fallback.Frozen.Knot(i)
					if a != b {
						t.Fatalf("knot %d differs", i)
					}
					replacement = replacement || b.Material == khipu.PreBreakMaterial
					ta, oka := direct.Frozen.CopyText(i)
					tb, okb := fallback.Frozen.CopyText(i)
					if oka != okb {
						t.Fatal("missing glyph snapshot")
					}
					if !oka {
						continue
					}
					if !reflect.DeepEqual(ta.Glyphs, tb.Glyphs) || !reflect.DeepEqual(ta.Clusters, tb.Clusters) || ta.ReplacementText != tb.ReplacementText || ta.IsReplacement != tb.IsReplacement {
						t.Fatalf("glyphs or provenance differ at knot %d", i)
					}
					if tb.Context.Params.Font.(*fontbackend.Face).Info().SHA256 != direct.Fonts[0].SHA256 {
						t.Fatal("replacement uses wrong font")
					}
				}
				if replacement != (wantLines == 2) {
					t.Fatal("wrong discretionary selection")
				}
				r, err := fallback.Frozen.SourceReader(khipu.ByteRange{Start: 0, End: uint64(len(source))})
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(r)
				if err != nil || string(data) != source {
					t.Fatal("source lost")
				}
			})
		}
	}
}
