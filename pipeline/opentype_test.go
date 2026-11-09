package pipeline_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"tytui/fontbackend"
	"tytui/pipeline"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
)

const gentium = "../../opentype/testdata/fonts/GentiumPlus-R.ttf"
const progress = "../../opentype/testdata/fonts/ShaperTestProgress.ttf"

func TestOpenTypeFrozenReference(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "office", Width: 200 * dimen.BP, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}})
		if err != nil {
			t.Fatal(err)
		}
		if r.Fonts[0].SHA256 != "fa09788adcc651d77eba7d96f8cc3a600d6484c58e3b4ff79c2b2464dbfea6f8" {
			t.Fatal("reference font changed")
		}
		var glyphs []contracts.ShapedGlyph
		for i := 0; i < r.Frozen.KnotCount(); i++ {
			if text, ok := r.Frozen.CopyText(i); ok {
				glyphs = append(glyphs, text.Glyphs...)
				if text.Context.EmSize != 12*dimen.BP {
					t.Fatal("font size lost")
				}
			}
		}
		// HarfBuzz 15 capture: opentype/internal/hbcmp/testdata/text/streaming.json,
		// stream_ligatures, first word. No expectation is computed by this adapter.
		want := []contracts.ShapedGlyph{{GlyphID: 82, Cluster: 0, XAdvance: 1030}, {GlyphID: 741, Cluster: 1, XAdvance: 1829}, {GlyphID: 70, Cluster: 4, XAdvance: 895}, {GlyphID: 72, Cluster: 5, XAdvance: 946}}
		if len(glyphs) != len(want) {
			t.Fatalf("glyphs %+v", glyphs)
		}
		for i := range want {
			if glyphs[i] != want[i] {
				t.Fatalf("glyph %d: %+v want %+v", i, glyphs[i], want[i])
			}
		}
		line, _ := r.Frozen.Line(0)
		if line.Source.Start != 0 || line.Source.End != 6 || r.Frozen.SourceLen() != 6 {
			t.Fatal("source correspondence changed")
		}
	}
}
func TestOpenTypeStyleMappingAndSnapshot(t *testing.T) {
	e := styledInput(t, `<p lang="en" style="font-family:serif;font-size:12pt">a <b style="font-size:24pt">aa</b> a</p>`)
	fonts := &fontbackend.Settings{DefaultPath: gentium, Size: 10 * dimen.BP, Rules: []fontbackend.Rule{{Family: "serif", Weight: 700, Slant: "normal", Path: progress}}}
	input := pipeline.Input{Text: e.Text, Source: e, Width: 200 * dimen.BP, Fonts: fonts}
	r, err := pipeline.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.FontBindings) != 3 || r.FontBindings[1].Path != progress || r.FontBindings[1].EmSize != 24*dimen.BP || r.FontBindings[2].Path != gentium {
		t.Fatalf("bindings %+v", r.FontBindings)
	}
	found := false
	for i := 0; i < r.Frozen.KnotCount(); i++ {
		text, ok := r.Frozen.CopyText(i)
		if !ok {
			continue
		}
		f := text.Context.Params.Font.(*fontbackend.Face)
		if f.Info().Path == progress {
			found = true
			if text.Context.EmSize != 24*dimen.BP {
				t.Fatal("mapped size lost")
			}
			if len(text.Glyphs) != 2 || text.Glyphs[0].GlyphID != 1 || text.Glyphs[0].Cluster != 2 || text.Glyphs[0].XAdvance != 580 {
				t.Fatalf("mapped glyphs %+v", text.Glyphs)
			}
		}
	}
	if !found {
		t.Fatal("mapped face did not reach frozen output")
	}
	fonts.Rules[0].Path = "missing.ttf"
	if r.Input.Fonts.Rules[0].Path != progress {
		t.Fatal("settings aliased")
	}
	if _, err = pipeline.Run(context.Background(), input); err == nil {
		t.Fatal("bad font silently fell back")
	}
	if r.FontBindings[1].Path != progress {
		t.Fatal("old frozen result changed")
	}
}
func TestOpenTypeDiagnostics(t *testing.T) {
	for _, tc := range []struct{ text, path, diagnostic string }{{"a", "missing.ttf", "font"}, {"a", "../../opentype/testdata/fonts/ShaperTestHebrew-NoGPOS.ttf", "parse"}, {"a世", progress, "U+4E16"}} {
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: tc.text, Width: 100 * dimen.BP, Fonts: &fontbackend.Settings{DefaultPath: tc.path, Size: 12 * dimen.BP}})
		var se *pipeline.StageError
		if r != nil || !errors.As(err, &se) || se.Stage != pipeline.StageFonts || !strings.Contains(err.Error(), tc.diagnostic) {
			t.Fatalf("%+v: %v", tc, err)
		}
		if tc.text == "a世" && !strings.Contains(err.Error(), "[1,4)") {
			t.Fatal("missing UTF-8 coordinates", err)
		}
	}
}
