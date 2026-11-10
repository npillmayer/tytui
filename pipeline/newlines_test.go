package pipeline_test

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"tytui/fontbackend"
	"tytui/pipeline"
)

func TestNewlinesAreWhitespaceBeforeShaping(t *testing.T) {
	for _, fonts := range []*fontbackend.Settings{nil, {DefaultPath: gentium, Size: 12 * dimen.BP}} {
		for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
			for _, tc := range []struct{ raw, want string }{{"\r\né\t a\r\n\r\nb\u2028c\u2029 \u00a0d\n", "é a b c \u00a0d"}, {"\r\n\n \t", ""}} {
				r, err := pipeline.Run(context.Background(), pipeline.Input{Text: tc.raw, Width: 300 * dimen.BP, Fonts: fonts, Breaker: breaker})
				if err != nil {
					t.Fatal(err)
				}
				if r.Input.Text != tc.raw || r.ParagraphText != tc.want || r.Text.Raw().String() != tc.want || r.Frozen.LineCount() != 1 {
					t.Fatalf("normalization: %+v", r)
				}
				reader, err := r.Frozen.SourceReader(khipu.ByteRange{Start: 0, End: uint64(len(tc.want))})
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(reader)
				if err != nil || string(data) != tc.want {
					t.Fatal("frozen coordinates mismatch")
				}
				var rawEnd, outEnd uint64
				for _, s := range r.TextSpans {
					if s.SourceStart != rawEnd || s.Start != outEnd {
						t.Fatal("mapping gap")
					}
					rawEnd, outEnd = s.SourceEnd, s.End
				}
				if rawEnd != uint64(len(tc.raw)) || outEnd != uint64(len(tc.want)) {
					t.Fatal("map coverage")
				}
			}
		}
	}
}
func TestHTMLNewlinesRetainDOMMapping(t *testing.T) {
	e := styledInput(t, "<p>  é&#13;<b>\n  a</b>\n\nb </p>")
	if e.Text != "é a b" {
		t.Fatal(e.Text)
	}
	if err := e.Validate(e.Text); err != nil {
		t.Fatal(err)
	}
	for _, fonts := range []*fontbackend.Settings{nil, {DefaultPath: gentium, Size: 12 * dimen.BP}} {
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Fonts: fonts, Width: 300 * dimen.BP})
		if err != nil {
			t.Fatal(err)
		}
		if r.Frozen.LineCount() != 1 || r.ParagraphText != e.Text || len(r.TextSpans) != 0 {
			t.Fatal("HTML renormalized or broken")
		}
		if !reflect.DeepEqual(r.Input.Source.Spans, e.Spans) || !reflect.DeepEqual(r.Input.Source.Styles, e.Styles) {
			t.Fatal("DOM mapping lost")
		}
	}
}
