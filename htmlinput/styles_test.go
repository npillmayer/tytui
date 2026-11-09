package htmlinput

import (
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
)

func extractP(t *testing.T, source string) *Extraction {
	t.Helper()
	d := parseTest(t, source)
	e, err := d.Extract(nodeNamed(d, "p"))
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestInheritedStyleSpans(t *testing.T) {
	e := extractP(t, `<html lang="he"><body style="font-family:serif;font-size:20px;color:navy"><p>é <span style="font-size:150%;font-weight:bold;font-style:italic">אב</span> z</p></body></html>`)
	if e.Text != "é אב z" || len(e.Styles) != 3 {
		t.Fatalf("unexpected extraction %+v", e)
	}
	a, b, c := e.Styles[0], e.Styles[1], e.Styles[2]
	if a.Start != 0 || a.End != 3 || b.Start != 3 || b.End != 7 || c.Start != 7 || c.End != 9 {
		t.Fatalf("byte spans %v", e.Styles)
	}
	if a.Style.EmSize != 15*dimen.BP || b.Style.EmSize != dimen.DU(22.5*float64(dimen.BP)) || c.Style.EmSize != a.Style.EmSize {
		t.Fatalf("sizes %v", e.Styles)
	}
	if a.Style.Language != "he" || b.Style.Color != "navy" || b.Style.Weight != 700 || b.Style.Slant != "italic" || c.Style.Weight != 400 {
		t.Fatalf("inheritance %v", e.Styles)
	}
	text, err := e.StyledText()
	if err != nil {
		t.Fatal(err)
	}
	for _, span := range e.Styles {
		value, _, err := text.StyleAt(span.Start)
		if err != nil || !span.Style.Equals(value) {
			t.Fatalf("style at byte %d: %v %v", span.Start, value, err)
		}
	}
}
func TestCSSCascadeRelevantProperties(t *testing.T) {
	e := extractP(t, `<style>p {color:red;font-size:10px} #chosen {color:blue;font-weight:bold}</style><p id="chosen" style="color:green">word</p>`)
	s := e.Styles[0].Style
	if s.Color != "green" || s.Weight != 700 || s.EmSize != dimen.DU(7.5*float64(dimen.BP)) {
		t.Fatalf("cascade lost declarations: %+v", s)
	}
}
func TestCSSWhitespaceAndMapping(t *testing.T) {
	d := parseTest(t, "<p> \n é <b> \t </b><i>  世 </i>  z\n </p>")
	e, err := d.Extract(nodeNamed(d, "p"))
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != "é 世 z" {
		t.Fatalf("processed text %q", e.Text)
	}
	if err = e.Validate(e.Text); err != nil {
		t.Fatal(err)
	}
	collapsed, omitted := false, false
	for _, s := range e.Spans {
		source := d.Nodes[s.NodeID].Text[s.NodeStart:s.NodeEnd]
		switch s.Kind {
		case "":
			if source != e.Text[s.Start:s.End] {
				t.Fatal("identity map changed bytes")
			}
		case "collapsed":
			collapsed = true
			if strings.Trim(source, " \t\n\r\f") != "" {
				t.Fatal("collapsed non-whitespace")
			}
		case "omitted":
			omitted = true
			if s.Start != s.End {
				t.Fatal("omission emits bytes")
			}
		}
	}
	if !collapsed || !omitted {
		t.Fatal("missing transformation provenance")
	}
	if extractP(t, `<p>a&nbsp;  b</p>`).Text != "a\u00a0 b" {
		t.Fatal("NBSP was collapsed")
	}
	if extractP(t, "<p style=\"white-space:pre-line\"> a  b\n  c </p>").Text != "a b\nc" {
		t.Fatal("pre-line policy")
	}
	if extractP(t, "<p style=\"white-space:pre-wrap\">  a  b\n c </p>").Text != "  a  b\n c " {
		t.Fatal("pre-wrap policy")
	}
	if extractP(t, "<p>   </p>").Text != "" {
		t.Fatal("whitespace-only input")
	}
}
func TestSizesAndCSSWideKeywords(t *testing.T) {
	e := extractP(t, `<html style="font-size:20px"><body style="font-size:2em;color:red"><p style="font-size:inherit">a<span style="font-size:1rem;color:initial">b</span><i style="font-size:50%;font-style:initial">c</i></p></body></html>`)
	if e.Styles[0].Style.EmSize != 30*dimen.BP || e.Styles[1].Style.EmSize != 15*dimen.BP || e.Styles[2].Style.EmSize != 15*dimen.BP || e.Styles[1].Style.Color != "black" || e.Styles[2].Style.Slant != "normal" {
		t.Fatalf("computed %v", e.Styles)
	}
	if extractP(t, `<p style="font-size:12pt">x</p>`).Styles[0].Style.EmSize != 12*dimen.BP {
		t.Fatal("CSS pt used TeX point")
	}
}
func TestBidiMetadata(t *testing.T) {
	e := extractP(t, `<p dir="rtl" lang="he">אב <span dir="ltr">abc</span> גד</p>`)
	if e.Direction != bidi.RightToLeft || len(e.Isolates) != 1 || e.Isolates[0].Start != 5 || e.Isolates[0].End != 8 {
		t.Fatalf("bidi coordinates: %+v", e)
	}
	m, err := e.BidiMarkup()
	if err != nil {
		t.Fatal(err)
	}
	if m(5) != bidi.MarkupLRI || m(8) != bidi.MarkupPDI {
		t.Fatal("incorrect external markup")
	}
	if extractP(t, `<p dir="auto">123 אב</p>`).Direction != bidi.RightToLeft {
		t.Fatal("auto direction")
	}
}
func TestUnsupportedCSSValues(t *testing.T) {
	for _, style := range []string{"white-space:nowrap", "white-space:pre", "font-size:calc(10px + 1em)", "font-style:oblique 10deg", "unicode-bidi:bidi-override"} {
		d := parseTest(t, `<p style="`+style+`">text</p>`)
		if _, err := d.Extract(nodeNamed(d, "p")); err == nil {
			t.Fatalf("accepted %s", style)
		}
	}
}

func TestUnselectedStyleDoesNotBlockExtraction(t *testing.T) {
	e := extractP(t, `<div style="font-size:calc(2px + 1em)">other</div><p>chosen</p>`)
	if e.Text != "chosen" {
		t.Fatal(e.Text)
	}
}
func TestRejectAmbiguousCascade(t *testing.T) {
	for _, source := range []string{`<p style="color:red!important">x</p>`, `<style>p {color:red;color:blue}</style><p>x</p>`} {
		if _, err := Parse("test", strings.NewReader(source)); err == nil {
			t.Fatalf("accepted unsupported cascade: %s", source)
		}
	}
}
func TestInvalidBidiAndStyleCoordinates(t *testing.T) {
	e := extractP(t, `<p>é abc xyz</p>`)
	e.Styles[0].End = 1
	if e.Validate(e.Text) == nil {
		t.Fatal("split UTF-8 style accepted")
	}
	e = extractP(t, `<p>é abc xyz</p>`)
	e.Isolates = []Isolate{{0, 6, bidi.RightToLeft}, {3, 9, bidi.LeftToRight}}
	if e.Validate(e.Text) == nil {
		t.Fatal("crossing isolates accepted")
	}
}

func TestStyledHTMLFixture(t *testing.T) {
	d, err := LoadFile("../testdata/html/styles.html")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, n := range d.Nodes {
		if n.Name == "p" {
			e, err := d.Extract(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = e.StyledText(); err != nil {
				t.Fatal(err)
			}
			count++
		}
	}
	if count != 3 {
		t.Fatalf("paragraph count %d", count)
	}
}
