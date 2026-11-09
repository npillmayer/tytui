package pipeline_test

import (
	"context"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"strings"
	"testing"
	"tytui/htmlinput"
	"tytui/pipeline"
)

func styledInput(t *testing.T, source string) *htmlinput.Extraction {
	t.Helper()
	d, err := htmlinput.Parse("styles.html", strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range d.Nodes {
		if n.Name == "p" {
			e, err := d.Extract(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			return e
		}
	}
	t.Fatal("no paragraph")
	return nil
}
func TestStyledFrozenParagraph(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		e := styledInput(t, `<p dir="rtl" lang="he" style="font-size:12pt">אב <span dir="ltr" lang="en" style="font-size:24pt;color:red">abc</span> גד</p>`)
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 200 * dimen.BP, Breaker: breaker})
		if err != nil {
			t.Fatal(err)
		}
		if r.Input.Direction != bidi.RightToLeft || r.Frozen.SourceLen() != uint64(len(e.Text)) {
			t.Fatal("lost base direction or changed source bytes")
		}
		var end uint64
		ltr, rtl := false, false
		for _, s := range r.BidiRuns {
			if s.From != end {
				t.Fatalf("bidi gap: %+v", r.BidiRuns)
			}
			end = s.To
			ltr = ltr || s.Dir == bidi.LeftToRight
			rtl = rtl || s.Dir == bidi.RightToLeft
		}
		if end != uint64(len(e.Text)) || !ltr || !rtl {
			t.Fatalf("bidi runs: %+v", r.BidiRuns)
		}
		sizes := map[dimen.DU]bool{}
		for i := 0; i < r.Frozen.KnotCount(); i++ {
			if text, ok := r.Frozen.CopyText(i); ok {
				sizes[text.Context.EmSize] = true
			}
		}
		if !sizes[12*dimen.BP] || !sizes[24*dimen.BP] {
			t.Fatalf("frozen sizes %v", sizes)
		}
		e.Styles[0].Style.EmSize = 1
		e.Spans[0].NodePath = "changed"
		e.Isolates[0].Start = 0
		if r.Input.Source.Styles[0].Style.EmSize != 12*dimen.BP || r.Input.Source.Spans[0].NodePath == "changed" || r.Input.Source.Isolates[0].Start == 0 {
			t.Fatal("retained extraction aliases input")
		}
	}
}
func TestWhitespaceThroughFreeze(t *testing.T) {
	for _, source := range []string{`<p> a  <b>b</b> c </p>`, "<p style=\"white-space:pre-line\">a  b\nc</p>", "<p style=\"white-space:pre-wrap\">a  b\nc</p>", `<p>   </p>`} {
		e := styledInput(t, source)
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 200 * dimen.BP})
		if err != nil {
			t.Fatalf("%s: %v", source, err)
		}
		if r.Frozen.SourceLen() != uint64(len(e.Text)) {
			t.Fatal("source length changed")
		}
		if strings.Contains(e.Text, "\n") && r.Frozen.LineCount() != 2 {
			t.Fatal("mandatory newline lost")
		}
	}
}
