package htmlinput

import (
	"errors"
	"strings"
	"testing"

	domcss "github.com/npillmayer/w3c-dom/style/css"
	domdisplay "github.com/npillmayer/w3c-dom/style/display"
	"golang.org/x/net/html"
)

func TestNormalizedDisplay(t *testing.T) {
	for _, tc := range []struct{ declarations, want string }{
		{"", "block"},
		{"display:BLOCK /**/ FLOW", "block"},
		{`display:\66 low-root`, "flow-root"},
		{"display:flow-root inline", "inline-block"},
		{"display:inherit", "flow-root"},
		{"display:initial", "inline"},
		{"display:unset", "inline"},
		{"display:inline flow;display:bogus!important", "inline"},
		{"display:bogus", "block"},
		{"display:flow-root;display:block-inline!important", "flow-root"},
		{"display:block grid", "grid"}, // Normalization is independent of layout support.
	} {
		t.Run(tc.declarations, func(t *testing.T) {
			d := parseTest(t, `<div style="display:block flow-root"><p style="`+tc.declarations+`">x</p></div>`)
			got, err := d.Display(nodeNamed(d, "p"))
			want, parseErr := domdisplay.Parse(tc.want)
			if parseErr != nil || err != nil || got != want {
				t.Fatalf("Display()=%v, %v; want %v", got, err, want)
			}
		})
	}
	d := parseTest(t, `<p style="display:InLiNe FLOW">x</p>`)
	if raw, err := d.Property(nodeNamed(d, "p"), "display"); err != nil || raw != "InLiNe FLOW" {
		t.Fatalf("inspection spelling changed: %q, %v", raw, err)
	}
}

func TestDisplayRequiresElement(t *testing.T) {
	d := parseTest(t, `<p>x<!-- comment --></p>`)
	for _, id := range []int{-1, len(d.Nodes), 0} {
		if v, err := d.Display(id); err == nil || v.Valid() {
			t.Fatalf("accepted invalid/non-element id %d: %v, %v", id, v, err)
		}
	}
	for _, n := range d.Nodes {
		if n.Type == html.ElementNode {
			continue
		}
		if v, err := d.Display(n.ID); err == nil || v.Valid() || !strings.Contains(err.Error(), n.Path) {
			t.Fatalf("non-element inherited display: %v, %v", v, err)
		}
	}
	if v, err := (*Document)(nil).Display(0); err == nil || v.Valid() {
		t.Fatalf("accepted nil document: %v, %v", v, err)
	}
}

func TestParagraphNormalizedInlineDisplay(t *testing.T) {
	for _, value := range []string{"inline flow", "flow inline", "INLINE FLOW", `\69 nline`, "CONTENTS"} {
		e := extractP(t, `<p>é<span style="display:`+value+`">a</span>b</p>`)
		if e.Text != "éab" {
			t.Fatalf("display %q changed text: %q", value, e.Text)
		}
	}
	for _, value := range []string{"block flow", "block flow-root", "inline flow-root", "inline list-item", "inline grid", "NONE", `\6e one`} {
		d := parseTest(t, `<p>a<span style="display:`+value+`">b</span>c</p>`)
		if e, err := d.Extract(nodeNamed(d, "p")); err == nil || e != nil || !strings.Contains(err.Error(), d.Nodes[nodeNamed(d, "span")].Path) {
			t.Fatalf("accepted unsupported nested display %q: %+v, %v", value, e, err)
		}
	}
}

func TestDisplayCascadeErrorPropagation(t *testing.T) {
	for _, value := range []string{"revert", "revert-layer", `\72 evert`} {
		d := parseTest(t, `<p style="display:`+value+`">x</p>`)
		id := nodeNamed(d, "p")
		v, err := d.Display(id)
		if v.Valid() || !errors.Is(err, domcss.ErrUnsupportedDisplayCascade) || !strings.Contains(err.Error(), d.Nodes[id].Path) {
			t.Fatalf("lost display error or locator: %v, %v", v, err)
		}
		if e, err := d.Extract(id); e != nil || !errors.Is(err, domcss.ErrUnsupportedDisplayCascade) {
			t.Fatalf("paragraph extraction hid display failure: %+v, %v", e, err)
		}
	}
}
