package pipeline_test

import (
	"context"
	"github.com/npillmayer/khipu/dimen"
	"reflect"
	"testing"
	"tytui/fontbackend"
	"tytui/pipeline"
)

func TestCSSUnitsThroughFrozenProof(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			var reference *pipeline.GlyphProof
			for _, size := range []string{"96px", "72pt", "1in", "25.4mm", "2.54cm"} {
				e := styledInput(t, `<p style="font-size:`+size+`"><em style="font-size:50%;display:unset">é</em></p>`)
				r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 200 * dimen.BP, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}})
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for k := 0; k < r.Frozen.KnotCount(); k++ {
					if text, ok := r.Frozen.CopyText(k); ok {
						count++
						if text.Context.EmSize != 36*dimen.BP {
							t.Fatalf("%s frozen size %v", size, text.Context.EmSize)
						}
					}
				}
				if count == 0 {
					t.Fatal("no frozen text")
				}
				proof, err := pipeline.BuildGlyphProof(r)
				if err != nil {
					t.Fatal(err)
				}
				if reference == nil {
					reference = proof
				} else if !reflect.DeepEqual(reference, proof) {
					t.Fatalf("%s differs from 96px", size)
				}
			}
		})
	}
}
