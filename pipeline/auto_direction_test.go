package pipeline_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"tytui/pipeline"
)

func TestAutoDirectionThroughFreeze(t *testing.T) {
	for _, tc := range []struct {
		child, tail, explicit string
		want                  bidi.Direction
	}{
		{`<span dir="rtl">אב</span>`, "abc", "ltr", bidi.LeftToRight},
		{`<span dir="auto">abc</span>`, "אב", "rtl", bidi.RightToLeft},
	} {
		for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
			t.Run(tc.explicit+"/"+string(breaker), func(t *testing.T) {
				source := `<p dir="auto">` + tc.child + tc.tail + `</p>`
				e := styledInput(t, source)
				if e.Direction != tc.want || len(e.Isolates) != 1 || e.Isolates[0].Start != 0 || e.Isolates[0].End != uint64(len(strings.TrimSuffix(e.Text, tc.tail))) {
					t.Fatalf("lost direction or isolate coordinates: %+v", e)
				}
				run := func(html string) *pipeline.Result {
					x := styledInput(t, html)
					r, err := pipeline.Run(context.Background(), pipeline.Input{Text: x.Text, Source: x, Width: 200 * dimen.BP, Breaker: breaker})
					if err != nil {
						t.Fatal(err)
					}
					return r
				}
				got := run(source)
				want := run(strings.Replace(source, `dir="auto"`, `dir="`+tc.explicit+`"`, 1))
				if got.Input.Direction != tc.want || !reflect.DeepEqual(got.BidiRuns, want.BidiRuns) || got.Frozen.LineCount() != want.Frozen.LineCount() || got.Frozen.KnotCount() != want.Frozen.KnotCount() {
					t.Fatal("auto layout differs from explicit base direction")
				}
				for k := 0; k < got.Frozen.KnotCount(); k++ {
					a, _ := got.Frozen.Knot(k)
					b, _ := want.Frozen.Knot(k)
					if !reflect.DeepEqual(a, b) {
						t.Fatalf("physical knot %d differs: %+v vs %+v", k, a, b)
					}
				}
				if !reflect.DeepEqual(got.Input.Source.Spans, e.Spans) {
					t.Fatal("DOM mapping changed")
				}
			})
		}
	}
}
