package pipeline_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"

	"tytui/pipeline"
)

func TestGlyphBoundaryPaintPreservesShaping(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, tc := range []struct {
			name, html, plain, font string
			ligature                bool
		}{
			{"ligature", `<p><span style="color:red">f</span><span style="color:blue">i</span></p>`, `<p>fi</p>`, gentium, true},
			{"Latin mark", `<p><span style="color:red">a</span><span style="color:blue">́</span></p>`, `<p>á</p>`, gentium, false},
			{"Hebrew points", `<p dir="rtl"><span style="color:red">א</span><span style="color:blue">ְ</span><span style="color:green">ב</span></p>`, `<p dir="rtl">אְב</p>`, hebrew, false},
		} {
			t.Run(string(breaker)+"/"+tc.name, func(t *testing.T) {
				run := func(html string) *pipeline.Result {
					e := styledInput(t, html)
					r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: tc.font, Size: 12 * dimen.BP}}})
					if err != nil {
						t.Fatal(err)
					}
					return r
				}
				colored, plain := run(tc.html), run(tc.plain)
				a, err := pipeline.BuildGlyphProof(colored)
				if err != nil {
					t.Fatal(err)
				}
				b, err := pipeline.BuildGlyphProof(plain)
				if err != nil {
					t.Fatal(err)
				}
				if tc.ligature && (len(a.Glyphs) != 1 || a.Glyphs[0].Color != "red") {
					t.Fatalf("split ligature/paint %+v", a.Glyphs)
				}
				for i, g := range a.Glyphs {
					want := "red"
					if tc.name == "Hebrew points" {
						switch g.Cluster {
						case 1:
							want = "blue"
						case 2:
							want = "green"
						}
					} else if tc.name == "Latin mark" && g.Cluster == 1 {
						want = "blue"
					}
					if g.Color != want {
						t.Fatalf("glyph color %+v want %s", g, want)
					}
					a.Glyphs[i].Color = "black"
				}
				if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(colored.BreakPositions, plain.BreakPositions) {
					t.Fatal("paint changed shaping or geometry")
				}
				if len(colored.Input.Source.Styles) < 2 {
					t.Fatal("source paint spans lost")
				}
			})
		}
	}
}
func TestSoftHyphenUsesMarkerPaint(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, width := range []dimen.DU{20 * dimen.BP, 100 * dimen.BP} {
			e := styledInput(t, `<p><span style="color:red">ab</span><span style="color:blue">&shy;</span><span style="color:green">cd</span></p>`)
			r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: width, Breaker: breaker, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}}})
			if err != nil {
				t.Fatal(err)
			}
			proof, err := pipeline.BuildGlyphProof(r)
			if err != nil {
				t.Fatal(err)
			}
			var colors []string
			for _, g := range proof.Glyphs {
				colors = append(colors, g.Color)
			}
			want := []string{"red", "red", "green", "green"}
			if width == 20*dimen.BP {
				want = []string{"red", "red", "blue", "green", "green"}
			}
			if !reflect.DeepEqual(colors, want) {
				t.Fatalf("%s %v: colors %v want %v", breaker, width, colors, want)
			}
		}
	}
}
func TestLegacyPaintRemainsAvailable(t *testing.T) {
	e := styledInput(t, `<p><span style="color:red">f</span><span style="color:blue">i</span></p>`)
	r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, PaintPolicy: typography.LegacyStyleBoundary, Width: 100 * dimen.BP, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := pipeline.BuildGlyphProof(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Glyphs) != 2 || p.Glyphs[0].Color != "red" || p.Glyphs[1].Color != "blue" {
		t.Fatalf("legacy proof changed %+v", p)
	}
}

func TestRTLFallbackAndSoftHyphenPaint(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, width := range []dimen.DU{20 * dimen.BP, 100 * dimen.BP} {
			e := styledInput(t, `<p dir="rtl"><span style="color:red">אב</span><span style="color:blue">&shy;</span><span style="color:green">גד</span></p>`)
			run := func(path string, fallback []string) *pipeline.Result {
				r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: width, Breaker: breaker, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: path, Size: 12 * dimen.BP, Fallbacks: fallback}}})
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			direct, fallback := run(hebrew, nil), run(gentium, []string{hebrew})
			a, err := pipeline.BuildGlyphProof(direct)
			if err != nil {
				t.Fatal(err)
			}
			b, err := pipeline.BuildGlyphProof(fallback)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatal("paint changed fallback context")
			}
			colors := map[string]int{}
			for _, g := range b.Glyphs {
				colors[g.Color]++
			}
			blue := 0
			if width == 20*dimen.BP {
				blue = 1
			}
			if colors["red"] != 2 || colors["green"] != 2 || colors["blue"] != blue {
				t.Fatalf("RTL replacement paint %v", colors)
			}
		}
	}
}
