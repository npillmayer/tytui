package htmlinput

import (
	"strings"
	"testing"
)

func parseTest(t *testing.T, source string) *Document {
	t.Helper()
	d, err := Parse("fixture.html", strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func nodeNamed(d *Document, name string) int {
	for _, n := range d.Nodes {
		if n.Name == name {
			return n.ID
		}
	}
	return -1
}
func TestExtractMapping(t *testing.T) {
	d := parseTest(t, `<!doctype html><html><head><style></style><style>p {color: red} b {font-weight: bold}</style></head><body><p id="chosen">hé &amp; <b>世界</b>!</p></body></html>`)
	id := nodeNamed(d, "p")
	e, err := d.Extract(id)
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != "hé & 世界!" || len(e.Spans) != 3 {
		t.Fatalf("extraction: %+v", e)
	}
	var end uint64
	for _, span := range e.Spans {
		n := d.Nodes[span.NodeID]
		if span.Start != end || e.Text[span.Start:span.End] != n.Text[span.NodeStart:span.NodeEnd] || span.NodePath != n.Path {
			t.Fatalf("bad map: %+v", span)
		}
		end = span.End
	}
	if end != uint64(len(e.Text)) {
		t.Fatal("incomplete map")
	}
	if got, err := d.Property(id, "color"); err != nil || got != "red" {
		t.Fatalf("color=%q %v", got, err)
	}
	if got, err := d.Property(nodeNamed(d, "b"), "color"); err != nil || got != "red" {
		t.Fatalf("inherited color=%q %v", got, err)
	}
	if _, err := d.Property(0, "font-family"); err == nil {
		t.Log("font default supplied")
	}
	again := parseTest(t, `<!doctype html><html><head><style></style><style>p {color: red} b {font-weight: bold}</style></head><body><p id="chosen">hé &amp; <b>世界</b>!</p></body></html>`)
	e2, err := again.Extract(nodeNamed(again, "p"))
	if err != nil || e2.DocumentID != e.DocumentID || e2.SelectedPath != e.SelectedPath {
		t.Fatal("unstable locator")
	}
}
func TestUnsupportedSelection(t *testing.T) {
	for _, source := range []string{`<div>before<p>nested</p></div>`, `<div>before<br>after</div>`, `<div><img src="x"></div>`, `<div><span hidden>x</span></div>`, `<div><span style="display:block">x</span></div>`} {
		d := parseTest(t, source)
		if e, err := d.Extract(nodeNamed(d, "div")); err == nil || e != nil {
			t.Fatalf("accepted %s", source)
		}
	}
}
func TestParseEdges(t *testing.T) {
	for _, source := range []string{"", `<p></p>`, `<p>A<!-- comment --> B</p>`, `<style></style><p>x</p>`} {
		d := parseTest(t, source)
		if len(d.Nodes) == 0 {
			t.Fatal("no DOM")
		}
	}
	d := parseTest(t, `<p>A<!-- comment --> B</p>`)
	e, err := d.Extract(nodeNamed(d, "p"))
	if err != nil || e.Text != "A B" {
		t.Fatalf("comment: %+v %v", e, err)
	}
	if _, err := Parse("bad", strings.NewReader("\xff")); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestManyStyledSiblings(t *testing.T) {
	source := `<style>span {color: red} .x {font-weight: bold}</style><div>` + strings.Repeat(`<span class="x">text</span>`, 100) + `</div>`
	d := parseTest(t, source)
	if len(d.Nodes) < 100 {
		t.Fatal("missing nodes")
	}
}

func TestCSSSubsetErrors(t *testing.T) {
	for _, source := range []string{
		`<style>@media print {p {color:red}}</style><p>x</p>`,
		`<p style="made-up-property: yes">x</p>`,
		`<style>[ {color:red}</style><p>x</p>`,
	} {
		if _, err := Parse("unsupported.html", strings.NewReader(source)); err == nil {
			t.Fatalf("accepted unsupported CSS: %s", source)
		}
	}
}

func TestExplicitDefaultAndHiddenContent(t *testing.T) {
	d := parseTest(t, `<style>span {display:block}</style><div><span>x</span></div>`)
	if _, err := d.Extract(nodeNamed(d, "div")); err == nil {
		t.Fatal("default-valued display:block was lost")
	}
	if v, err := d.Property(nodeNamed(d, "span"), "display"); err != nil || v != "block" {
		t.Fatalf("display=%q %v", v, err)
	}
	d = parseTest(t, `<div style="display:none"><p>hidden</p></div>`)
	if _, err := d.Extract(nodeNamed(d, "p")); err == nil {
		t.Fatal("hidden ancestor ignored")
	}
}
