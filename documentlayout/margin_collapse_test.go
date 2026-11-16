package documentlayout_test

import (
	"bytes"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	ot "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	"tytui/documentlayout"
)

func TestDocumentMarginCollapseReservedEmptyLines(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			d, _, _ := build(t, `<div id="root" style="font-size:10px"><div style="margin-bottom:5px">a</div><div style="margin-top:7px;margin-bottom:30px"><span> </span></div><div style="margin-top:-12px;margin-bottom:3px"></div><div style="margin-top:50px">b</div></div>`)
			cfg := layoutConfig(t)
			r := runDocument(t, d, 200, breaker, cfg)
			nodes := d.Root().Children
			g := r.Geometry()
			for j, y := range []float32{0, 17, 45, 105} {
				if g[nodes[j].BoxID].Frame.Y != y {
					t.Fatalf("child %d: %+v, want Y=%g", j, g[nodes[j].BoxID], y)
				}
			}
			if g[d.Root().BoxID].Content.H != 115 {
				t.Fatal("pending group counted more than once", g)
			}
			empty := 0
			for _, entry := range r.Groups() {
				group, _ := r.Group(entry.GroupID)
				if group.Input.Text == "" {
					empty++
					if len(group.Paragraph.VList.Lines()) != 1 || group.Paragraph.VList.Box().Extent != 7.5 || g[entry.OwnerID].Frame.H != 10 {
						t.Fatal("empty paragraph lost its reserved strut")
					}
				}
			}
			if empty != 2 {
				t.Fatalf("empty group count=%d", empty)
			}
			again := runDocument(t, d, 100, breaker, cfg)
			for id, old := range g {
				resized := again.Geometry()[id]
				if old.Frame.Y != resized.Frame.Y || old.Frame.H != resized.Frame.H {
					t.Fatal("resize changed vertical layout of single characters")
				}
			}
			replay := runDocument(t, d, 200, breaker, cfg)
			if !reflect.DeepEqual(g, replay.Geometry()) || !reflect.DeepEqual(g, r.Geometry()) {
				t.Fatal("replay changed retained geometry")
			}
			proof, err := documentlayout.BuildProof(t.Context(), r, cells())
			if err != nil {
				t.Fatal(err)
			}
			if len(proof.Lines) != 4 || proof.Lines[3].Top != 78.75 {
				t.Fatalf("proof restacked collapsed positions: %+v", proof.Lines)
			}
		})
	}
}

func TestDocumentMarginCollapseParentAndAtomicBoundaries(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			d, _, _ := build(t, `<div id="root" style="font-size:10px"><div style="margin-top:13px;margin-bottom:5px"><div style="margin-top:20px;margin-bottom:7px">x</div></div></div>`)
			r := runDocument(t, d, 200, breaker, layoutConfig(t))
			parent := d.Root().Children[0]
			child := parent.Children[0]
			g := r.Geometry()
			if g[parent.BoxID].Frame.Y != 20 || g[parent.BoxID].Content.H != 10 || g[child.BoxID].Frame.Y != 0 || g[d.Root().BoxID].Content.H != 37 {
				t.Fatal("parent margins did not collapse", g)
			}
			atomicDoc, _, _ := build(t, `<div id="root" style="font-size:10px"><span style="display:inline-block;width:auto"><div style="margin-top:6px;margin-bottom:13px">x</div><div style="margin-top:8px;margin-bottom:7px">y</div></span></div>`)
			a := atomicDoc.Root().Children[0].Children[0]
			result := runDocument(t, atomicDoc, 200, breaker, layoutConfig(t))
			ag := result.Geometry()
			if ag[a.BoxID].Content.H != 46 || ag[a.Children[0].BoxID].Frame.Y != 6 || ag[a.Children[1].BoxID].Frame.Y != 29 {
				t.Fatal("atomic boundary lost internal collapse", ag)
			}
			// An actual atomic line still reserves the outer strut below its baseline.
			if ag[atomicDoc.Root().BoxID].Content.H != 48 {
				t.Fatal("outer atomic line extent changed", ag)
			}
		})
	}
}

func TestRealDocumentCollapsedProofReplay(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			d, _, _ := build(t, `<div id="root" style="font-size:20px"><div style="margin-bottom:13px">a</div><div style="margin-top:-8px;margin-bottom:10px"></div><div style="margin-top:7px">b</div></div>`)
			cfg := realConfig(t)
			services := render.Services{Backend: ot.Services{}, Identity: ot.Services{}}
			first := runDocument(t, d, 200, breaker, cfg)
			nodes := d.Root().Children
			g := first.Geometry()
			a, b := g[nodes[0].BoxID], g[nodes[2].BoxID]
			if b.Frame.Y != a.Frame.H+5+g[nodes[1].BoxID].Frame.H+10 {
				t.Fatalf("real-font gap=%g, expected two gaps plus reserved empty line", b.Frame.Y-a.Frame.H)
			}
			if g[nodes[1].BoxID].Frame.H <= 0 {
				t.Fatal("real-font empty block lost reserved space")
			}
			var before bytes.Buffer
			if err := documentlayout.WriteSVG(t.Context(), &before, first, services); err != nil {
				t.Fatal(err)
			}
			for _, width := range []float32{100, 200} {
				runDocument(t, d, width, breaker, cfg)
			}
			var after bytes.Buffer
			if err := documentlayout.WriteSVG(t.Context(), &after, first, services); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before.Bytes(), after.Bytes()) || !reflect.DeepEqual(g, first.Geometry()) {
				t.Fatal("later runs changed frozen proof")
			}
			// Geometry accessors remain detached after margin collapse.
			detached := first.Geometry()
			detached[nodes[0].BoxID] = box.LayoutGeometry{}
			if !reflect.DeepEqual(g, first.Geometry()) {
				t.Fatal("geometry accessor aliases snapshot")
			}
		})
	}
}
