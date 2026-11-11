package htmlinput

import (
	"errors"
	"strings"
	"testing"

	"github.com/npillmayer/w3c-dom/style/cssom/douceuradapter"
)

func TestUpstreamPropertyInspection(t *testing.T) {
	d := parseTest(t, `<div style="font-family:Example;display:block;color:red"><span style="display:unset;font-family:inherit;color:initial">text</span><i style="font-family:initial"></i></div>`)
	for _, tc := range []struct {
		id        int
		key, want string
	}{
		{nodeNamed(d, "span"), "display", "inline"},
		{nodeNamed(d, "span"), "font-family", "Example"},
		{nodeNamed(d, "span"), "color", "default"},
		{nodeNamed(d, "i"), "font-family", "serif"},
		{0, "font-family", "serif"},
	} {
		got, err := d.Property(tc.id, tc.key)
		if err != nil || got != tc.want {
			t.Errorf("node %d %s = %q, %v; want %q", tc.id, tc.key, got, err, tc.want)
		}
	}
	for _, n := range d.Nodes {
		if n.Text == "text" {
			if got, err := d.Property(n.ID, "font-family"); err != nil || got != "Example" {
				t.Fatalf("text inspection: %q %v", got, err)
			}
		}
	}
	if _, err := d.Property(-1, "color"); err == nil {
		t.Fatal("accepted invalid node")
	}
	if _, err := d.Property(0, "unknown-property"); err == nil {
		t.Fatal("accepted absent property")
	}
}

func TestUpstreamRecursiveStyleExtraction(t *testing.T) {
	d := parseTest(t, `<style>p{color:red}</style><div><style></style><section><style>p{color:blue}</style></section></div><p>hé &amp; text</p>`)
	id := nodeNamed(d, "p")
	if got, err := d.Property(id, "color"); err != nil || got != "blue" {
		t.Fatalf("nested sheet order: %q %v", got, err)
	}
	e, err := d.Extract(id)
	if err != nil || e.Text != "hé & text" {
		t.Fatalf("extraction: %+v %v", e, err)
	}
	if err := e.Validate(e.Text); err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamStyleExtractionRejectsPartialResults(t *testing.T) {
	d, err := Parse("broken", strings.NewReader(`<style>p{color:red}</style><div><style>p{broken;}</style></div><style>p{color:green}</style><p>x</p>`))
	var parseErr *douceuradapter.StyleExtractionError
	if d != nil || !errors.As(err, &parseErr) {
		t.Fatalf("expected upstream extraction error and no document: %v %v", d, err)
	}
}
