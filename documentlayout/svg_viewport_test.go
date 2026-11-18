package documentlayout_test

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/render"
	"tytui/documentlayout"
)

func TestFragmentSVGViewportOverflowWritesNothing(t *testing.T) {
	d, _, _ := build(t, `<div id="root">abc</div>`)
	r := runDocument(t, d, 100, paragraph.FirstFit, layoutConfig(t))
	g, _ := r.Group(d.Groups()[0].GroupID)
	f := lineFragment(t, g.Paragraph.VList, 0, len(g.Paragraph.VList.Lines()))
	for _, tc := range []struct {
		name     string
		x, y     float64
		overflow bool
	}{
		{"width", math.MaxFloat64 * .75, 0, true},
		{"height", 0, math.MaxFloat64 * .75, true},
		{"both", math.MaxFloat64 * .75, math.MaxFloat64 * .75, true},
		{"large finite", 1e150, 1e150, false},
		{"ordinary signed", 100, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			placements := []documentlayout.PositionedFragment{
				{GroupID: g.Input.GroupID, Fragment: f, FrameOrigin: render.Point{X: -tc.x, Y: -tc.y}},
				{GroupID: g.Input.GroupID, Fragment: f, FrameOrigin: render.Point{X: tc.x, Y: tc.y}},
			}
			// The retained proof is valid: only the derived viewport span overflows.
			if _, err := documentlayout.BuildFragmentProof(t.Context(), r, placements, cells()); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			err := documentlayout.WriteFragmentSVG(t.Context(), &out, r, placements, cells())
			if tc.overflow {
				if !errors.Is(err, contracts.ErrIllegalArguments) || out.Len() != 0 {
					t.Fatalf("overflow: error=%v, wrote %d bytes", err, out.Len())
				}
			} else if err != nil || out.Len() == 0 || strings.Contains(out.String(), "Inf") || strings.Contains(out.String(), "NaN") {
				t.Fatalf("finite viewport rejected or corrupted: error=%v, bytes=%d", err, out.Len())
			}
		})
	}
}
