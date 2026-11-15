package documentlayout_test

import (
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/paragraph"

	"tytui/documentlayout"
)

func TestDocumentHorizontalAutoMargins(t *testing.T) {
	for _, tc := range []struct {
		name, css                                     string
		x, contentX, contentWidth, frameWidth, height float32
	}{
		{"center", "width:40px;margin:0 auto", 30, 30, 40, 40, 20},
		{"left", "width:40px;margin-left:auto;margin-right:10px", 50, 50, 40, 40, 20},
		{"right", "width:40px;margin-left:10px;margin-right:auto", 10, 10, 40, 40, 20},
		{"edges", "width:40px;margin:0 auto;padding:5px;border-width:2px;border-style:solid", 23, 30, 40, 54, 34},
		{"negative fixed margin", "width:40px;margin-left:auto;margin-right:-10px", 70, 70, 40, 40, 20},
		{"overflow", "width:120px;margin:0 auto", 0, 0, 120, 120, 20},
		{"overflow left auto", "width:120px;margin-left:auto;margin-right:10px", 0, 0, 120, 120, 20},
		{"auto width", "width:auto;margin:0 auto;padding:5px", 0, 5, 90, 100, 30},
		{"fractional", "width:40.5px;margin:0 auto", 29.75, 29.75, 40.5, 40.5, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := build(t, `<div id="root" style="font-size:20px"><div style="`+tc.css+`;color:blue;">é</div></div>`)
			for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
				t.Run(string(breaker), func(t *testing.T) {
					r := runDocument(t, d, 100, breaker, layoutConfig(t))
					g, ok := r.Group(d.Groups()[0].GroupID)
					if !ok {
						t.Fatal("missing paragraph")
					}
					geom := r.Geometry()[g.Input.OwnerID]
					if geom.Frame != (box.Rect{X: tc.x, W: tc.frameWidth, H: tc.height}) || geom.Content.X != tc.contentX || geom.Content.W != tc.contentWidth || geom.RelativeTo != d.Root().BoxID {
						t.Fatalf("geometry=%+v", geom)
					}
					line, ok := g.Paragraph.Frozen.Line(0)
					if !ok || line.TargetWidth != dimen.DU(float64(tc.contentWidth)*49152) || g.Input.Text != "é" || len(g.Input.Paint) != 1 || g.Input.Paint[0].Paint.Color != "blue" {
						t.Fatal("paragraph width/text/paint", line, g.Input)
					}
					checkSource(t, d, []*documentlayout.InlineGroup{&g.Input})
				})
			}
		})
	}
}

func TestDocumentAutoMarginsResizeRetainsResults(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px;padding:3px;border-width:2px;border-style:solid"><div style="width:40px;margin:0 auto">é</div></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			cfg := layoutConfig(t)
			old := runDocument(t, d, 110, breaker, cfg) // root content width 100
			before := old.Geometry()
			g, _ := old.Group(d.Groups()[0].GroupID)
			oldLines := g.Paragraph.VList.Lines()
			next := runDocument(t, d, 90, breaker, cfg) // root content width 80
			if before[g.Input.OwnerID].Frame.X != 30 || next.Geometry()[g.Input.OwnerID].Frame.X != 20 {
				t.Fatal("centering did not use current parent content width")
			}
			if !reflect.DeepEqual(before, old.Geometry()) || !reflect.DeepEqual(oldLines, g.Paragraph.VList.Lines()) {
				t.Fatal("resize changed retained result")
			}
			// Compose parent content offsets once; paragraph-local X remains zero.
			if before[d.Root().BoxID].Content.X+before[g.Input.OwnerID].Content.X != 35 || oldLines[0].X != 0 {
				t.Fatal("margin/owner edges leaked into paragraph-local positions")
			}
		})
	}
}

func TestDocumentBlockAutoMarginsInsideAutoAtomic(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;width:auto;margin:auto;padding:3px;border-width:1px;border-style:solid;font-size:10px"><div style="width:20px;margin:0 auto">é</div><div style="width:60px;margin:0 auto">x</div></span></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			r := runDocument(t, d, 100, breaker, layoutConfig(t))
			parent, _ := r.Group(d.Groups()[0].GroupID)
			atom := parent.Input.AtomicSources[0]
			geometry := r.Geometry()
			if geometry[atom.BoxID].Frame != (box.Rect{W: 68, H: 28}) || geometry[atom.BoxID].Content != (box.Rect{X: 4, Y: 4, W: 60, H: 20}) {
				t.Fatal("auto atomic sizing or margins", geometry[atom.BoxID])
			}
			center, _ := r.Group(d.Groups()[1].GroupID)
			g := geometry[center.Input.OwnerID]
			if g.Frame != (box.Rect{X: 20, W: 20, H: 10}) || g.RelativeTo != atom.BoxID || g.Content.X+geometry[atom.BoxID].Content.X != 24 {
				t.Fatal("block did not center in final atomic content", g)
			}
			if center.Paragraph.VList.Lines()[0].Baseline != 6 {
				t.Fatal("changed local text baseline")
			}
		})
	}
}
