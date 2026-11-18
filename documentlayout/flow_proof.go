package documentlayout

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"

	"github.com/npillmayer/typography/layout"
	"github.com/npillmayer/typography/render"
)

// BuildFlowProof composes frozen frame placements and retained opaque interiors.
// It permits inspection of an incomplete flow but never draws its remainder.
// No preparation, metrics or layout work occurs. Services follow BuildFragmentProof.
func BuildFlowProof(ctx context.Context, r *FlowResult, services render.Services) (*Proof, error) {
	if r == nil {
		return nil, invalid("missing flow result")
	}
	return BuildFragmentProof(ctx, r.source, r.placements(), services)
}

// WriteFlowSVG exports a complete flow using the existing boxes/glyphs controls.
// Visible frame outlines describe usable content areas, without repeating owner
// margins or edges. Boxes-only export performs no outline/identity/metric calls.
// Incomplete flow, cancellation or serialization failure writes no bytes; writer
// failures have the ordinary partial-write contract. This is not a replay format.
func WriteFlowSVG(ctx context.Context, w io.Writer, r *FlowResult, services render.Services, options LayoutSVGOptions) error {
	if ctx == nil || w == nil || r == nil {
		return invalid("flow export requires context, writer and result")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !r.Complete() {
		return unsupported("incomplete frame flow: source remains unplaced")
	}
	p, err := buildFragmentProof(ctx, r.source, r.placements(), services, !(options.ShowGlyphs || options.HideBoxes))
	if err != nil {
		return err
	}
	var frames bytes.Buffer
	if !options.HideBoxes {
		frames.WriteString(`<g id="frames" fill="none" stroke="#0d9488">`)
	}
	for _, f := range r.frames {
		if err := ctx.Err(); err != nil {
			return err
		}
		x, y, width, height := f.Frame.X*.75, f.Frame.Y*.75, f.Frame.Width*.75, f.Frame.Height*.75
		p.include(x, y)
		p.include(x+width, y+height)
		if !options.HideBoxes {
			var id bytes.Buffer
			_ = xml.EscapeText(&id, []byte(f.Frame.ID))
			fmt.Fprintf(&frames, `<rect data-frame="%s" x="%g" y="%g" width="%g" height="%g"/>`, id.String(), x, y, width, height)
		}
	}
	if !options.HideBoxes {
		frames.WriteString(`</g>`)
	} else {
		p.Boxes = nil
		p.Lines = nil
		p.Atomics = nil
	}
	return writeDecoratedProofSVG(ctx, w, p, frames.String())
}

// Geometry-only proof for the boxes control; use the same retained line records
// without entering glyph traversal or any font service, including opaque children.
func retainedLineProof(ctx context.Context, lines []layout.Line) (*render.ParagraphProof, error) {
	p := &render.ParagraphProof{}
	for _, l := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.Lines = append(p.Lines, render.Line{X: l.X, Baseline: l.Baseline, Top: l.Top, Height: l.Height, Width: l.Horizontal.Width.Points(), TargetWidth: l.Horizontal.TargetWidth.Points()})
		p.MinX = math.Min(p.MinX, l.X)
		p.MinY = math.Min(p.MinY, l.Top)
		p.MaxX = math.Max(p.MaxX, l.X+math.Max(l.Horizontal.Width.Points(), l.Horizontal.TargetWidth.Points()))
		p.MaxY = math.Max(p.MaxY, l.Top+l.Height)
	}
	return p, nil
}
