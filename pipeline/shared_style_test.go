package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	typestyle "github.com/npillmayer/typography/style"

	"tytui/htmlinput"
	"tytui/pipeline"
)

func TestSharedStylesWithoutDOM(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, real := range []bool{false, true} {
			t.Run(string(breaker)+map[bool]string{false: "/synthetic", true: "/opentype"}[real], func(t *testing.T) {
				s := typestyle.Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "red", Language: "en", WhiteSpace: "normal", UnicodeBidi: "normal"}
				spans := []typestyle.Span{{Start: 0, End: 2, Style: s}, {Start: 2, End: 3, Style: s}}
				spans[1].Style.EmSize = 18 * dimen.BP
				spans[1].Style.Color = "blue"
				in := pipeline.Input{Text: "\néa\n", Styles: spans, Width: 100 * dimen.BP, Breaker: breaker}
				if real {
					in.Fonts = &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 10 * dimen.BP}}
				}
				r, err := pipeline.Run(context.Background(), in)
				if err != nil {
					t.Fatal(err)
				}
				if r.Input.Source != nil || r.ParagraphText != "éa" || len(r.TextSpans) == 0 {
					t.Fatal("non-DOM normalization/source map lost")
				}
				sizes := map[dimen.DU]bool{}
				for k := 0; k < r.Frozen.KnotCount(); k++ {
					if tx, ok := r.Frozen.CopyText(k); ok {
						sizes[tx.Context.EmSize] = true
					}
				}
				if !sizes[12*dimen.BP] || !sizes[18*dimen.BP] {
					t.Fatalf("frozen sizes: %v", sizes)
				}
				spans[0].Style.Color = "green"
				if r.Input.Styles[0].Style.Color != "red" {
					t.Fatal("retained caller spans")
				}
				if real {
					proof, err := pipeline.BuildGlyphProof(r)
					if err != nil {
						t.Fatal(err)
					}
					if len(proof.Glyphs) != 2 || proof.Glyphs[0].Color != "red" || proof.Glyphs[1].Color != "blue" || proof.Glyphs[1].Cluster != 1 {
						t.Fatalf("paint/scalar coordinates: %+v", proof.Glyphs)
					}
					if len(r.FontBindings) != 2 || r.FontBindings[0].End != 2 || r.FontBindings[1].Start != 2 {
						t.Fatalf("byte bindings: %+v", r.FontBindings)
					}
				}
			})
		}
	}
}

func TestSharedStyleInputValidation(t *testing.T) {
	s := typestyle.Style{EmSize: 12 * dimen.BP, WhiteSpace: "normal", UnicodeBidi: "normal"}
	for _, tc := range []struct {
		name   string
		spans  []typestyle.Span
		source *htmlinput.Extraction
	}{
		{"empty", []typestyle.Span{}, nil},
		{"split scalar", []typestyle.Span{{Start: 0, End: 1, Style: s}}, nil},
		{"two sources", []typestyle.Span{{Start: 0, End: 2, Style: s}}, &htmlinput.Extraction{}},
		{"preserving whitespace", []typestyle.Span{{Start: 0, End: 2, Style: typestyle.Style{EmSize: 12 * dimen.BP, WhiteSpace: "pre-wrap", UnicodeBidi: "normal"}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "é", Styles: tc.spans, Source: tc.source, Width: 100 * dimen.BP})
			if r != nil || !errors.Is(err, pipeline.ErrInvalidInput) {
				t.Fatalf("result=%v error=%v", r, err)
			}
		})
	}
}
