package documentlayout

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/typography/render"
)

// WriteBoxesSVG exports retained CSS box outlines and line-height rectangles
// for frozen text fragments, in big points. It never requests glyph outlines.
func WriteBoxesSVG(ctx context.Context, w io.Writer, result *Result) error {
	return WriteLayoutSVG(ctx, w, result, render.Services{}, LayoutSVGOptions{})
}

// LayoutSVGOptions selects diagnostic boxes and retained glyph outlines.
// HideBoxes also enables glyphs and hides text-fragment outlines.
type LayoutSVGOptions struct {
	ShowGlyphs bool
	HideBoxes  bool
}

// WriteLayoutSVG exports colored boxes with optional final glyph outlines.
// Glyph export requires render services; it never reshapes or reflows text.
func WriteLayoutSVG(ctx context.Context, w io.Writer, result *Result, services render.Services, options LayoutSVGOptions) error {
	showGlyphs := options.ShowGlyphs || options.HideBoxes
	if w == nil {
		return invalid("box proof requires a writer")
	}
	p, err := buildProof(ctx, result, services, !showGlyphs)
	if err != nil {
		return err
	}
	kinds := make(map[box.BoxID]*box.LayoutNode)
	var visit func(*box.LayoutNode)
	visit = func(n *box.LayoutNode) {
		kinds[n.BoxID] = n
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(result.document.root)
	origins := make(map[box.BoxID]ProofRect, len(p.Boxes))
	for _, b := range p.Boxes {
		origins[b.BoxID] = b.Content
	}
	var fragments bytes.Buffer
	if !options.HideBoxes {
		for _, group := range result.document.order {
			g, ok := result.groups[group.GroupID]
			origin, hasOrigin := origins[group.OwnerID]
			if !ok || !hasOrigin || g.Paragraph.VList == nil || g.Paragraph.Frozen == nil {
				return invalid("missing retained paragraph geometry")
			}
			for _, line := range g.Paragraph.VList.Lines() {
				if err := ctx.Err(); err != nil {
					return err
				}
				x := origin.X + line.X + line.Horizontal.LeftSkip.Points()
				y := origin.Y + line.Top
				for k := line.Horizontal.FirstKnot; k < line.Horizontal.FirstKnot+line.Horizontal.KnotCount; k++ {
					knot, _ := g.Paragraph.Frozen.Knot(k)
					advance := knot.W.Points()
					if knot.Kind == khipu.KTTextBox {
						// Knots are already in physical order, including RTL fragments.
						// Line height is diagnostic geometry, not a glyph ink bound.
						left, width := math.Min(x, x+advance), math.Abs(advance)
						fmt.Fprintf(&fragments, `<rect data-group="%d" data-owner="%d" data-line="%d" data-knot="%d" x="%g" y="%g" width="%g" height="%g"/>`, group.GroupID, group.OwnerID, line.Index, k, left, y, width, line.Height)
						p.MinX, p.MinY = math.Min(p.MinX, left), math.Min(p.MinY, y)
						p.MaxX, p.MaxY = math.Max(p.MaxX, left+width), math.Max(p.MaxY, y+line.Height)
					}
					x += advance // Include glue and atomics, without outlining them as text.
				}
			}
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%g %g %g %g"><title>Retained layout proof</title>`, p.MinX-12, p.MinY-12, p.MaxX-p.MinX+24, p.MaxY-p.MinY+24)
	if !options.HideBoxes {
		b.WriteString(`<g id="boxes" fill="none" stroke-width="0.5">`)
		for _, entry := range p.Boxes {
			n := kinds[entry.BoxID]
			kind, color := "block", "#6366f1"
			strokeWidth := 0.5
			if n.Box == box.BoxInlineBlock {
				kind, color = "inline-block", "#d97706"
			} else if n.EstablishesBFC {
				kind, color = "flow-root", "#0d9488"
				strokeWidth = 1.5
			}
			f, c := entry.Frame, entry.Content
			fmt.Fprintf(&b, `<g data-box="%d" data-parent="%d" data-kind="%s" stroke="%s" stroke-width="%g"><rect x="%g" y="%g" width="%g" height="%g"/><rect x="%g" y="%g" width="%g" height="%g" stroke-dasharray="2 2"/></g>`, entry.BoxID, entry.RelativeTo, kind, color, strokeWidth, f.X, f.Y, f.W, f.H, c.X, c.Y, c.W, c.H)
		}
		b.WriteString(`</g><g id="text-fragments" fill="none" stroke="#d1d5db" stroke-width="0.35">`)
		b.Write(fragments.Bytes())
		b.WriteString(`</g>`)
	}
	if showGlyphs {
		b.WriteString(`<g id="glyphs">`)
		writeGlyphPaths(&b, p.Glyphs)
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := w.Write(b.Bytes())
	if err == nil && n != b.Len() {
		return io.ErrShortWrite
	}
	return err
}
