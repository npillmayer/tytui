package pipeline_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/style"

	"tytui/pipeline"
)

// Inspect the actual values passed into the paragraph, including M's private
// itemized style. Clearing Color while retaining a document style is insufficient.
func requireTypographyOnly(t *testing.T, value any) {
	t.Helper()
	var inspect func(reflect.Type)
	inspect = func(typ reflect.Type) {
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Name == "Color" || f.Type == reflect.TypeOf(style.Paint{}) {
				t.Fatalf("typesetter received rendering data in %T", value)
			}
			inspect(f.Type)
		}
	}
	inspect(reflect.TypeOf(value))
}

func TestRenderingBypassSeparatesPaintFromTypesetterAndInput(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, backend := range []string{"synthetic", "OpenType"} {
			for _, source := range []string{"DOM", "shared"} {
				for _, policy := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
					t.Run(string(breaker)+"/"+backend+"/"+source+"/"+string(policy), func(t *testing.T) {
						e := styledInput(t, "<p>\n<span style=\"color:red\">f</span><span style=\"color:blue\">i</span>\n</p>")
						in := pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, PaintPolicy: policy}
						if source == "shared" {
							in.Source, in.Styles, in.Text = nil, e.Styles, "\nfi \n"
						}
						if backend == "OpenType" {
							in.Fonts = &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}}
						}
						r, err := pipeline.Run(context.Background(), in)
						if err != nil {
							t.Fatal(err)
						}
						a, _, err := r.Text.StyleAt(0)
						if err != nil {
							t.Fatal(err)
						}
						b, _, err := r.Text.StyleAt(1)
						if err != nil {
							t.Fatal(err)
						}
						requireTypographyOnly(t, a)
						requireTypographyOnly(t, b)
						if a.Equals(b) != (policy == typography.GlyphBoundary) {
							t.Fatal("lost modern coalescing or legacy boundary")
						}
						want := []typography.PaintSpan{
							{Start: 0, End: 1, Paint: style.Paint{Color: "red"}},
							{Start: 1, End: 2, Paint: style.Paint{Color: "blue"}},
						}
						if !reflect.DeepEqual(r.PaintSpans(), want) {
							t.Fatalf("lost original logical paint ranges: %+v", r.PaintSpans())
						}
						// Neither original input, retained inspection/replay input, nor
						// a returned paint slice is the renderer's source of truth.
						e.Styles[0].Style.Color = "invalid-caller-color"
						if r.Input.Source != nil {
							r.Input.Source.Styles[0].Style.Color = "invalid-retained-color"
						}
						if r.Input.Styles != nil {
							r.Input.Styles[0].Style.Color = "invalid-retained-color"
						}
						r.Input.Source, r.Input.Styles = nil, nil
						paint := r.PaintSpans()
						paint[0].Paint.Color = "invalid-returned-color"
						if !reflect.DeepEqual(r.PaintSpans(), want) {
							t.Fatal("renderer paint aliases mutable input or accessor")
						}
						if backend == "OpenType" {
							proof, err := pipeline.BuildGlyphProof(r)
							if err != nil {
								t.Fatal(err)
							}
							wantColors := []string{"red"} // fi remains one red glyph.
							if policy == typography.LegacyStyleBoundary {
								wantColors = append(wantColors, "blue")
							}
							var colors []string
							for _, g := range proof.Glyphs {
								colors = append(colors, g.Color)
								if g.Cluster != 0 && policy == typography.GlyphBoundary {
									t.Fatalf("unexpected ligature anchor: %+v", g)
								}
							}
							if !reflect.DeepEqual(colors, wantColors) {
								t.Fatalf("bypass changed glyph colors: %v", colors)
							}
						}
					})
				}
			}
		}
	}
}
