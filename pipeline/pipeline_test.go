package pipeline_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"tytui/pipeline"
)

func TestSyntheticParagraph(t *testing.T) {
	// Each synthetic scalar occupies half of the default 10 pt em. A two-letter
	// word fits in 12 pt, but two words cannot fit even with compressed glue.
	const source = "éa bb cc"
	input := pipeline.Input{Text: source, Width: 12 * dimen.PT}
	result, err := pipeline.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Frozen.LineCount() != 3 {
		t.Fatalf("got %d lines, want 3", result.Frozen.LineCount())
	}
	if !reflect.DeepEqual(result.BreakPositions, []uint64{4, 7, 9}) {
		t.Fatalf("break byte endpoints = %v", result.BreakPositions)
	}
	if result.Text.Raw().String() != source || result.Frozen.SourceLen() != uint64(len(source)) {
		t.Fatal("source changed during layout")
	}
	r, err := result.Frozen.SourceReader(khipu.ByteRange{End: uint64(len(source))})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != source {
		t.Fatalf("retained source = %q, error %v", got, err)
	}
	var glyphs int
	for i := 0; i < result.Frozen.LineCount(); i++ {
		line, ok := result.Frozen.Line(i)
		if !ok || line.TargetWidth != input.Width || line.Width != input.Width || line.Fit != khipu.FitWithinCapacity {
			t.Fatalf("line %d geometry: %+v", i, line)
		}
		width := line.LeftSkip + line.RightSkip
		for j := line.FirstKnot; j < line.FirstKnot+line.KnotCount; j++ {
			knot, _ := result.Frozen.Knot(j)
			width += knot.W
			text, ok := result.Frozen.CopyText(j)
			if !ok {
				continue
			}
			for _, g := range text.Glyphs {
				glyphs++
				if g.Cluster >= uint32(utf8.RuneCountInString(source)) || g.XAdvance != 500 {
					t.Fatalf("invalid synthetic glyph: %+v", g)
				}
				if want := uint32([]rune(source)[g.Cluster]) + 1; g.GlyphID != want {
					t.Fatalf("glyph %d at scalar %d, want %d", g.GlyphID, g.Cluster, want)
				}
			}
		}
		if width != line.Width {
			t.Fatalf("line %d: knot and skip widths %d != %d", i, width, line.Width)
		}
	}
	if glyphs != 6 {
		t.Fatalf("got %d retained text glyphs, want 6", glyphs)
	}
	if len(result.Diagnostics) != 5 || result.Diagnostics[4].Stage != pipeline.StageFreeze {
		t.Fatalf("missing stage diagnostics: %v", result.Diagnostics)
	}
	wide, err := pipeline.Run(context.Background(), pipeline.Input{Text: source, Width: 100 * dimen.PT})
	if err != nil || wide.Frozen.LineCount() != 1 {
		t.Fatalf("wide layout: result %v, error %v", wide, err)
	}
	if result.Frozen.LineCount() != 3 {
		t.Fatal("later run changed the previous result")
	}
}

func TestEmptyParagraph(t *testing.T) {
	result, err := pipeline.Run(context.Background(), pipeline.Input{Width: 100 * dimen.PT})
	if err != nil {
		t.Fatal(err)
	}
	if result.Frozen.LineCount() != 1 || result.Frozen.SourceLen() != 0 {
		t.Fatal("empty paragraph must retain a final line with no source")
	}
	knot, ok := result.Frozen.Knot(result.Frozen.KnotCount() - 1)
	if !ok || knot.Material != khipu.ParagraphFillMaterial {
		t.Fatal("empty paragraph must retain final fill")
	}
}

func TestRejectedInput(t *testing.T) {
	for _, input := range []pipeline.Input{
		{}, {Width: -1}, {Width: dimen.Fill},
		{Text: "\xff", Width: dimen.PT},
		{Width: dimen.PT, Direction: bidi.Direction(99)},
	} {
		result, err := pipeline.Run(context.Background(), input)
		var stageErr *pipeline.StageError
		if result != nil || !errors.Is(err, pipeline.ErrInvalidInput) || !errors.As(err, &stageErr) || stageErr.Stage != pipeline.StageInput {
			t.Fatalf("input %+v: result %v, error %v", input, result, err)
		}
	}
	if result, err := pipeline.Run(nil, pipeline.Input{Width: dimen.PT}); result != nil || !errors.Is(err, pipeline.ErrInvalidInput) {
		t.Fatalf("nil context: result %v, error %v", result, err)
	}
}

func TestCanceledParagraph(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := pipeline.Run(ctx, pipeline.Input{Text: "hello", Width: 100 * dimen.PT})
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: result %v, error %v", result, err)
	}
}
