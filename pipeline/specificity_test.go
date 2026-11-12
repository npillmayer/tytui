package pipeline_test

import (
	"context"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"

	"tytui/pipeline"
)

func TestCSSSpecificityThroughFrozenProof(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			e := styledInput(t, `<style>
 p#x {color:red; font-size:18pt}
 .c {color:blue; font-size:10pt}
 #other, span {color:green}
 span.accent {color:purple}
</style><p id="x" class="c">é<span class="accent">a</span></p>`)
			if e.Text != "éa" || len(e.Styles) != 2 {
				t.Fatalf("unexpected extraction: %+v", e)
			}
			for i, want := range []string{"red", "purple"} {
				if s := e.Styles[i].Style; s.Color != want || s.EmSize != 18*dimen.BP {
					t.Fatalf("style %d = %+v, want %s at 18pt", i, s, want)
				}
			}
			r, err := pipeline.Run(context.Background(), pipeline.Input{
				Text: e.Text, Source: e, Width: 200 * dimen.BP, Breaker: breaker,
				Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}},
			})
			if err != nil {
				t.Fatal(err)
			}
			// Prove that inherited size reaches the frozen shaper context and
			// winning colors reach the glyph proof at multibyte source offsets.
			for k := 0; k < r.Frozen.KnotCount(); k++ {
				if text, ok := r.Frozen.CopyText(k); ok && text.Context.EmSize != 18*dimen.BP {
					t.Fatalf("frozen size = %v", text.Context.EmSize)
				}
			}
			proof, err := pipeline.BuildGlyphProof(r)
			if err != nil {
				t.Fatal(err)
			}
			if len(proof.Glyphs) != 2 || proof.Glyphs[0].Color != "red" || proof.Glyphs[1].Color != "purple" {
				t.Fatalf("wrong proof colors: %+v", proof.Glyphs)
			}
		})
	}
}
