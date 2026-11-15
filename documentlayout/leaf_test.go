package documentlayout_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/npillmayer/typography/paragraph"
	"tytui/documentlayout"
)

func TestDiagnosticLeafThroughAtomicBoundary(t *testing.T) {
	for _, direction := range []string{"ltr", "rtl"} {
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			t.Run(direction+string(breaker), func(t *testing.T) {
				d, _, _ := build(t, `<div id="root" dir="`+direction+`" style="font-size:20px">é <span style="display:inline-block"><img data-tytui-leaf="rectangle" alt="A &amp; B" style="width:30px;height:12px;padding:2px;border-width:1px;border-style:solid;margin:3px 4px"></span> אב<img data-tytui-leaf="rectangle" style="width:0;height:0"></div>`)
				if len(d.Groups()) != 2 {
					t.Fatal("leaf created an empty paragraph", len(d.Groups()))
				}
				groups := extractAll(t, d)
				checkSource(t, d, groups)
				r := runDocument(t, d, 120, breaker, layoutConfig(t))
				p, err := documentlayout.BuildProof(t.Context(), r, cells())
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Leaves) != 2 || len(p.Atomics) != 3 {
					t.Fatal("lost occurrences", p.Leaves, p.Atomics)
				}
				for _, leaf := range p.Leaves {
					g := r.Geometry()[leaf.BoxID]
					if leaf.Label == "A & B" {
						if g.Frame.W != 36 || g.Frame.H != 18 || leaf.Content.W != 22.5 || leaf.Content.H != 9 {
							t.Fatal("edges or strut changed leaf dimensions", g, leaf)
						}
					} else if g.Frame.W != 0 || g.Frame.H != 0 {
						t.Fatal("zero leaf acquired a strut", g)
					}
					found := false
					for _, a := range p.Atomics {
						if a.BoxID == leaf.BoxID {
							found = true
							if a.Locator != leaf.Locator {
								t.Fatal("lost provenance")
							}
						}
					}
					if !found {
						t.Fatal("leaf detached from atomic placement")
					}
				}
				var before, after bytes.Buffer
				if err := documentlayout.WriteSVG(t.Context(), &before, r, cells()); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(before.String(), `data-leaf="rectangle"`) || !strings.Contains(before.String(), "A &amp; B") {
					t.Fatal("payload missing")
				}
				p.Leaves[0].Label = "changed"
				runDocument(t, d, 65, breaker, layoutConfig(t))
				if err := documentlayout.WriteSVG(t.Context(), &after, r, cells()); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before.Bytes(), after.Bytes()) {
					t.Fatal("retained payload/geometry mutated")
				}
			})
		}
	}
}

func TestDiagnosticLeafRejectsAmbiguousInput(t *testing.T) {
	for _, attrs := range []string{
		`style="width:20px;height:10px"`,
		`data-tytui-leaf="rectangle" style="height:10px"`,
		`data-tytui-leaf="rectangle" style="width:20px"`,
		`data-tytui-leaf="rectangle" style="width:20px;height:-1px"`,
		`data-tytui-leaf="rectangle" src="missing.png" style="width:20px;height:10px"`,
		`data-tytui-leaf="rectangle" style="display:block;width:20px;height:10px"`,
	} {
		input, id := parse(t, `<div id="root"><img `+attrs+`></div>`)
		if d, err := documentlayout.Build(t.Context(), input, id); err == nil || d != nil {
			t.Fatal("accepted unsupported leaf", attrs)
		}
	}
	input, id := parse(t, `<div id="root"><img data-tytui-leaf="rectangle" style="width:20px;height:10px"></div>`)
	if _, err := input.Extract(id); err == nil {
		t.Fatal("paragraph mode accepted a document leaf")
	}
}
