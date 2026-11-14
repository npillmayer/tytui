package htmlinput

import (
	"testing"
)

func TestResolvedStylesDetachedAndEligibilityIndependent(t *testing.T) {
	d := parseTest(t, `<div id="root">a<section style="font-size:30px">b</section></div>`)
	id := nodeNamed(d, "div")
	a, err := d.ResolvedStyles(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Extract(id); err == nil {
		t.Fatal("legacy extraction accepted nested block")
	}
	s := a[id]
	s.Color = "changed"
	a[id] = s
	delete(a, nodeNamed(d, "section"))
	b, err := d.ResolvedStyles(id)
	if err != nil || b[id].Color == "changed" || len(b) <= len(a) {
		t.Fatal("styles alias", b, err)
	}
	if _, err := d.ResolvedStyles(-1); err == nil {
		t.Fatal("invalid selection accepted")
	}
	if _, err := (*Document)(nil).ResolvedStyles(0); err == nil {
		t.Fatal("nil document accepted")
	}
}
