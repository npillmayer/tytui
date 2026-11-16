package htmlinput

import (
	"strings"
	"testing"
)

func TestAdditionalCSS(t *testing.T) {
	doc, err := ParseWithCSS("input", strings.NewReader(`<style>#root {font-size:12px}</style><div id="root">x</div>`), `#root {font-size:24px}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range doc.Nodes {
		if n.Name == "div" {
			v, err := doc.Property(n.ID, "font-size")
			if err != nil || v != "24px" {
				t.Fatalf("%s %v", v, err)
			}
		}
	}
	if _, err := ParseWithCSS("input", strings.NewReader(`<div>x</div>`), `div {made-up:yes}`); err == nil {
		t.Fatal("accepted unsupported CSS")
	}
}
