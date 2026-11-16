package documentlayout_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/typography/paragraph"
	domcss "github.com/npillmayer/w3c-dom/style/css"

	"tytui/documentlayout"
)

func TestDocumentNormalizedDisplaySpellings(t *testing.T) {
	const source = `<div id="root" style="display:%s;font-size:10px">before <span style="display:%s;color:blue">é</span> <span style="display:%s;width:20px;padding:1px" dir="rtl">אב</span><section style="display:%s;margin-top:6px">inside</section>after</div>`
	canonical, _, _ := build(t, fmt.Sprintf(source, "block", "inline", "inline-block", "flow-root"))
	for _, tc := range []struct{ root, inline, atomic, inner string }{
		{"block flow", "inline flow", "inline flow-root", "block flow-root"},
		{"flow block", "flow inline", "flow-root inline", "flow-root block"},
		{"BLOCK /**/ FLOW", "INLINE FLOW", "INLINE FLOW-ROOT", "FLOW-ROOT"},
		{`\62 lock`, `\69 nline`, `\69 nline-block`, `\66 low-root`},
	} {
		t.Run(tc.root, func(t *testing.T) {
			d, _, _ := build(t, fmt.Sprintf(source, tc.root, tc.inline, tc.atomic, tc.inner))
			if !reflect.DeepEqual(d.Root(), canonical.Root()) {
				t.Fatal("equivalent display spelling changed the typed box tree")
			}
			checkSource(t, d, extractAll(t, d))
			for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
				cfg := layoutConfig(t)
				got := runDocument(t, d, 100, breaker, cfg)
				want := runDocument(t, canonical, 100, breaker, cfg)
				if !reflect.DeepEqual(got.Geometry(), want.Geometry()) || len(got.Groups()) != len(want.Groups()) {
					t.Fatal("equivalent display spelling changed geometry/groups")
				}
				for _, group := range got.Groups() {
					g, _ := got.Group(group.GroupID)
					w, ok := want.Group(group.GroupID)
					if !ok || g.Input.Text != w.Input.Text || !reflect.DeepEqual(g.Paragraph.BreakPositions, w.Paragraph.BreakPositions) {
						t.Fatal("equivalent display spelling changed text or breaks")
					}
				}
				var svg, expected bytes.Buffer
				if err := documentlayout.WriteBoxesSVG(t.Context(), &svg, got); err != nil {
					t.Fatal(err)
				}
				if err := documentlayout.WriteBoxesSVG(t.Context(), &expected, want); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(svg.Bytes(), expected.Bytes()) {
					t.Fatal("equivalent display spelling changed the box proof")
				}
			}
		})
	}
}

func TestDocumentDisplayCascade(t *testing.T) {
	d, _, _ := build(t, `<style>#root {display:block flow-root} .initial {display:block}</style><div id="root" style="display:bogus!important"><span style="display:inherit">one</span><span class="initial" style="display:initial">two</span><span style="display:unset">three</span><span style="display:inline flow-root;display:bogus!important;width:10px">four</span></div>`)
	root := d.Root()
	if !root.EstablishesBFC || root.Box != box.BoxBlock || len(root.Children) != 2 {
		t.Fatalf("invalid declaration displaced the stylesheet's flow-root: %+v", root)
	}
	inherited := root.Children[0]
	if inherited.Box != box.BoxBlock || !inherited.EstablishesBFC {
		t.Fatalf("inherit lost the normalized pair: %+v", inherited)
	}
	// initial/unset resolve to ordinary inline; the final span retains the
	// earlier valid declaration despite the invalid !important duplicate.
	group := root.Children[1].Children[0]
	if len(group.Children) != 3 || group.Children[0].Box != box.BoxInline || group.Children[1].Box != box.BoxInline ||
		group.Children[2].Box != box.BoxInlineBlock || !group.Children[2].EstablishesBFC {
		t.Fatalf("cascade or typed display projection changed: %+v", group.Children)
	}
}

func TestDocumentUnsupportedNormalizedDisplay(t *testing.T) {
	for _, value := range []string{"block grid", "inline flex", "block table", "table-cell", "inline ruby", "ruby-base", "contents", "list-item", "inline flow-root list-item", "run-in flow-root"} {
		for _, root := range []bool{true, false} {
			source := `<div id="root" style="display:` + value + `">x</div>`
			if !root {
				source = `<div id="root">before<span style="display:` + value + `">inside</span>after</div>`
			}
			input, id := parse(t, source)
			badID := id
			if !root {
				for _, n := range input.Nodes {
					if n.Name == "span" {
						badID = n.ID
					}
				}
			}
			v, err := input.Display(badID)
			if err != nil { // Valid CSS must reach the adapter's support boundary.
				t.Fatal(err)
			}
			d, err := documentlayout.Build(t.Context(), input, id)
			if d != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) || !strings.Contains(err.Error(), input.Nodes[badID].Path) || !strings.Contains(err.Error(), "display "+v.CSS()) {
				t.Fatalf("display %q root=%v: document=%v, error=%v", value, root, d, err)
			}
		}
	}
}

func TestDocumentDisplayErrorsKeepSource(t *testing.T) {
	for _, value := range []string{"revert", "revert-layer", `\72 evert-layer`, "NONE", `\6e one`} {
		for _, ancestor := range []bool{false, true} {
			source := `<div id="root"><span style="display:` + value + `">x</span></div>`
			badName := "span"
			if ancestor {
				source = `<section style="display:` + value + `"><div id="root">x</div></section>`
				badName = "section"
			}
			input, id := parse(t, source)
			d, err := documentlayout.Build(t.Context(), input, id)
			want := domcss.ErrUnsupportedDisplayCascade
			if value == "NONE" || value == `\6e one` {
				want = khipu.ErrUnsupportedPolicy
			}
			if d != nil || !errors.Is(err, want) {
				t.Fatalf("lost display error for %q: %v, %v", value, d, err)
			}
			for _, n := range input.Nodes {
				if n.Name == badName && !strings.Contains(err.Error(), n.Path) {
					t.Fatalf("lost failing node locator: %v", err)
				}
			}
		}
	}
}

func TestDocumentFlowRootMarginsAndProof(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		for _, tc := range []struct {
			display                 string
			firstY, lastY, h, nextY float32
			bfc                     bool
		}{
			{"block flow", 0, 23, 33, 64, false},
			{"block flow-root", 6, 29, 46, 77, true},
		} {
			t.Run(string(breaker)+"/"+tc.display, func(t *testing.T) {
				d, _, _ := build(t, `<div id="root" style="display:flow-root;font-size:10px"><div style="margin-bottom:12px">a</div><section style="display:`+tc.display+`;margin-top:3px;margin-bottom:4px"><div style="margin-top:6px;margin-bottom:13px">x</div><div style="margin-top:8px;margin-bottom:7px">y</div></section><div style="margin-top:9px">b</div></div>`)
				root := d.Root()
				container, after := root.Children[1], root.Children[2]
				first, last := container.Children[0], container.Children[1]
				if !root.EstablishesBFC || container.EstablishesBFC != tc.bfc || container.Box != box.BoxBlock {
					t.Fatal("normalized display did not reach CSS core")
				}
				cfg := layoutConfig(t)
				r := runDocument(t, d, 100, breaker, cfg)
				g := r.Geometry()
				if g[container.BoxID].Frame.Y != 22 || g[container.BoxID].Content.H != tc.h ||
					g[first.BoxID].Frame.Y != tc.firstY || g[last.BoxID].Frame.Y != tc.lastY ||
					g[after.BoxID].Frame.Y != tc.nextY || g[root.BoxID].Content.H != tc.nextY+10 {
					t.Fatalf("incorrect parent/sibling containment: %+v", g)
				}
				proof, err := documentlayout.BuildProof(t.Context(), r, cells())
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, b := range proof.Boxes {
					if b.BoxID == first.BoxID {
						found = true
						if b.Frame.Y != float64(22+tc.firstY)*.75 {
							t.Fatal("proof changed retained containment coordinates", b)
						}
					}
				}
				if !found {
					t.Fatal("proof lost child box")
				}
				var before, afterSVG bytes.Buffer
				if err := documentlayout.WriteBoxesSVG(t.Context(), &before, r); err != nil {
					t.Fatal(err)
				}
				resized := runDocument(t, d, 200, breaker, cfg)
				if resized.Geometry()[container.BoxID].Content.W != 200 || g[container.BoxID].Content.W != 100 {
					t.Fatal("flow-root did not retain normal auto block sizing")
				}
				if err := documentlayout.WriteBoxesSVG(t.Context(), &afterSVG, r); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(g, r.Geometry()) || !bytes.Equal(before.Bytes(), afterSVG.Bytes()) {
					t.Fatal("later layout changed the retained proof")
				}
			})
		}
	}
}

func TestDocumentFlowRootInsideAtomic(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		for _, width := range []string{"50px", "auto"} {
			d, _, _ := build(t, `<div id="root" style="font-size:10px"><span style="display:inline flow-root;width:`+width+`"><section style="display:block flow-root;margin-top:3px;margin-bottom:4px"><div style="margin-top:6px;margin-bottom:13px">x</div><div style="margin-top:8px;margin-bottom:7px">y</div></section></span></div>`)
			r := runDocument(t, d, 200, breaker, layoutConfig(t))
			a := d.Root().Children[0].Children[0]
			f := a.Children[0]
			g := r.Geometry()
			if a.Box != box.BoxInlineBlock || !a.EstablishesBFC || f.Box != box.BoxBlock || !f.EstablishesBFC ||
				g[f.BoxID].Content.H != 46 || g[f.BoxID].Frame.Y != 3 || g[a.BoxID].Content.H != 53 ||
				g[d.Root().BoxID].Content.H != 55 || g[f.BoxID].Content.W != g[a.BoxID].Content.W ||
				g[f.Children[0].BoxID].Frame.Y != 6 || g[f.Children[1].BoxID].Frame.Y != 29 {
				t.Fatalf("breaker=%s width=%s: nested flow-root geometry %+v", breaker, width, g)
			}
		}
	}
}

func TestDocumentFlowRootKeepsEmptyParagraphStrut(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		d, _, _ := build(t, `<div id="root" style="font-size:10px"><section style="display:flow-root;margin-top:13px;margin-bottom:5px"><div style="margin-top:20px;margin-bottom:7px"></div></section></div>`)
		r := runDocument(t, d, 200, breaker, layoutConfig(t))
		parent := d.Root().Children[0]
		child := parent.Children[0]
		g := r.Geometry()
		if g[parent.BoxID].Frame.Y != 13 || g[parent.BoxID].Content.H != 37 || g[child.BoxID].Frame.Y != 20 ||
			g[child.BoxID].Content.H != 10 || g[d.Root().BoxID].Content.H != 55 {
			t.Fatal("flow-root containment changed the reserved empty strut", g)
		}
	}
}

func TestDocumentNormalizedLeafDisplay(t *testing.T) {
	for _, display := range []string{"inline flow", "inline flow-root"} {
		d, _, _ := build(t, `<div id="root" style="font-size:10px"><img data-tytui-leaf="rectangle" style="display:`+display+`;width:20px;height:10px;padding:2px"></div>`)
		leaf := d.Root().Children[0].Children[0]
		if leaf.Box != box.BoxInlineBlock || leaf.Replaced == nil || len(leaf.Children) != 0 {
			t.Fatal("normalized leaf did not produce a childless atomic")
		}
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			r := runDocument(t, d, 100, breaker, layoutConfig(t))
			g := r.Geometry()[leaf.BoxID]
			if g.Frame.W != 24 || g.Frame.H != 14 {
				t.Fatal("leaf edges were treated as ordinary inline edges", g)
			}
		}
	}
}
