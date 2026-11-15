package documentlayout_test

import (
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/paragraph"

	"tytui/documentlayout"
)

func TestInlineCSSTerminatorsReachDocumentLayout(t *testing.T) {
	const source = `<div id="root" style="width:100px;font-size:20px;color:red; /* owner ; } */"><span style="display:inline-block;width:30px;font-size:10px;color:blue; /* child ; } */">é</span></div>`
	d, input, _ := build(t, source)
	if input.OriginalHTML != source || len(d.Groups()) != 2 {
		t.Fatal("lost source or normalized groups")
	}
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			r := runDocument(t, d, 200, breaker, layoutConfig(t))
			geometry := r.Geometry()
			if got := geometry[d.Root().BoxID]; got.Frame != (box.Rect{W: 100, H: 20}) || got.Content != got.Frame {
				t.Fatalf("owner geometry = %+v", got)
			}
			parent, _ := r.Group(d.Groups()[0].GroupID)
			if parent.Input.Text != "\uFFFC" || len(parent.Input.AtomicSources) != 1 || parent.Input.Paragraph.Style.EmSize != 15*dimen.BP || parent.Input.Paragraph.Style.Color != "red" {
				t.Fatalf("parent input = %+v", parent.Input)
			}
			atom := parent.Input.AtomicSources[0]
			if got := geometry[atom.BoxID]; got.Frame != (box.Rect{Y: 6, W: 30, H: 10}) || got.RelativeTo != parent.Input.OwnerID {
				t.Fatalf("atomic geometry = %+v", got)
			}
			child, _ := r.Group(d.Groups()[1].GroupID)
			if child.Input.OwnerID != atom.BoxID || child.Input.Text != "é" || len(child.Input.Paint) != 1 || child.Input.Paint[0].Paint.Color != "blue" {
				t.Fatalf("child text/paint = %+v", child.Input)
			}
			if child.Paragraph.Frozen.LineCount() != 1 || child.Paragraph.Frozen.SourceLen() != uint64(len("é")) {
				t.Fatal("missing frozen child text")
			}
			text, ok := child.Paragraph.Frozen.CopyText(0)
			if !ok || text.Context.EmSize != 15*dimen.BP/2 {
				t.Fatalf("frozen font size = %+v (%v)", text, ok)
			}
			checkSource(t, d, []*documentlayout.InlineGroup{&parent.Input, &child.Input})
		})
	}
}
