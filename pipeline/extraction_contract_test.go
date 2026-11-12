package pipeline_test

import (
	"context"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"

	"tytui/pipeline"
)

func TestExtractionContractSourceStyleAndFrozenSize(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			e := styledInput(t, `<p style="font-size:12pt;color:red">é<span style="font-size:150%;color:blue">a</span></p>`)
			if e.Text != "éa" || len(e.Styles) != 2 || e.Styles[0].End != 2 || e.Styles[1].Start != 2 || e.Styles[1].End != 3 {
				t.Fatalf("UTF-8 style projection %+v", e)
			}
			if err := e.Validate(e.Text); err != nil {
				t.Fatal(err)
			}
			r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 10 * dimen.BP}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(r.FontBindings) != 2 || r.FontBindings[0].Start != 0 || r.FontBindings[0].End != 2 || r.FontBindings[1].Start != 2 || r.FontBindings[1].End != 3 || r.FontBindings[0].EmSize != 12*dimen.BP || r.FontBindings[1].EmSize != 18*dimen.BP {
				t.Fatalf("bindings %+v", r.FontBindings)
			}
			proof, err := pipeline.BuildGlyphProof(r)
			if err != nil {
				t.Fatal(err)
			}
			if len(proof.Glyphs) != 2 || proof.Glyphs[0].Cluster != 0 || proof.Glyphs[1].Cluster != 1 || proof.Glyphs[0].Color != "red" || proof.Glyphs[1].Color != "blue" {
				t.Fatalf("scalar anchors/paint %+v", proof.Glyphs)
			}
			for i, g := range proof.Glyphs {
				snapshot, ok := r.Frozen.CopyText(g.Knot)
				if !ok {
					t.Fatal("missing frozen text")
				}
				want := []dimen.DU{12 * dimen.BP, 18 * dimen.BP}[i]
				if snapshot.Context.EmSize != want {
					t.Fatalf("frozen size %v want %v", snapshot.Context.EmSize, want)
				}
			}
			e.Spans[0].NodePath = "changed"
			e.Styles[0].Style.Color = "green"
			if r.Input.Source.Spans[0].NodePath == "changed" || r.Input.Source.Styles[0].Style.Color != "red" {
				t.Fatal("published source/style aliases caller")
			}
		})
	}
}
