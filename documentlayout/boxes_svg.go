package documentlayout

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/npillmayer/typography/render"
)

// WriteBoxesSVG exports only retained CSS border and content outlines, in big
// points. It needs no rendering backend and never requests glyph outlines.
func WriteBoxesSVG(ctx context.Context, w io.Writer, result *Result) error {
	if w == nil {
		return invalid("box proof requires a writer")
	}
	p, err := buildProof(ctx, result, render.Services{}, true)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%g %g %g %g"><title>CSS box hierarchy</title><g fill="none" stroke-width="0.5">`, p.MinX-12, p.MinY-12, p.MaxX-p.MinX+24, p.MaxY-p.MinY+24)
	for _, box := range p.Boxes {
		f, c := box.Frame, box.Content
		fmt.Fprintf(&b, `<g data-box="%d" data-parent="%d"><rect x="%g" y="%g" width="%g" height="%g" stroke="#6366f1"/><rect x="%g" y="%g" width="%g" height="%g" stroke="#94a3b8" stroke-dasharray="2 2"/></g>`, box.BoxID, box.RelativeTo, f.X, f.Y, f.W, f.H, c.X, c.Y, c.W, c.H)
	}
	b.WriteString(`</g></svg>`)
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := w.Write(b.Bytes())
	if err == nil && n != b.Len() {
		return io.ErrShortWrite
	}
	return err
}
