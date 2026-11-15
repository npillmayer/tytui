package documentlayout

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/npillmayer/typography/render"
)

// WriteSVG writes a diagnostic of retained border/content boxes, baselines and
// glyph outlines. Coordinates use big points; text/font binaries are not embedded.
// Local images embed retained validated bytes as data URIs, without source I/O.
// Proof construction completes before writing anything; writer failures can still
// leave partial bytes at the destination. This is not a document replay format.
func WriteSVG(ctx context.Context, w io.Writer, result *Result, services render.Services) error {
	if w == nil {
		return invalid("document proof requires a writer")
	}
	p, err := BuildProof(ctx, result, services)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	escape := func(s string) string { var out bytes.Buffer; _ = xml.EscapeText(&out, []byte(s)); return out.String() }
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.6f %.6f %.6f %.6f"><title>Retained document proof</title>`, p.MinX-12, p.MinY-12, p.MaxX-p.MinX+24, p.MaxY-p.MinY+24)
	fmt.Fprintf(&b, `<rect x="%.6f" y="%.6f" width="%.6f" height="%.6f" fill="white"/>`, p.MinX-12, p.MinY-12, p.MaxX-p.MinX+24, p.MaxY-p.MinY+24)
	b.WriteString(`<g id="boxes" fill="none" stroke-width="0.4">`)
	for _, box := range p.Boxes {
		f, c := box.Frame, box.Content
		fmt.Fprintf(&b, `<g data-box="%d" data-parent="%d"><rect x="%.6f" y="%.6f" width="%.6f" height="%.6f" stroke="#818cf8"/><rect x="%.6f" y="%.6f" width="%.6f" height="%.6f" stroke="#94a3b8" stroke-dasharray="2 2"/></g>`, box.BoxID, box.RelativeTo, f.X, f.Y, f.W, f.H, c.X, c.Y, c.W, c.H)
	}
	b.WriteString(`</g><g id="lines" fill="none" stroke="#cbd5e1" stroke-width="0.25">`)
	for _, l := range p.Lines {
		fmt.Fprintf(&b, `<g data-group="%d" data-owner="%d"><rect x="%.6f" y="%.6f" width="%.6f" height="%.6f"/><path d="M%.6f %.6f H%.6f"/></g>`, l.GroupID, l.OwnerID, l.X, l.Top, l.TargetWidth, l.Height, l.X, l.Baseline, l.X+l.Width)
	}
	b.WriteString(`</g><g id="atomics" fill="#818cf8">`)
	for _, a := range p.Atomics {
		fmt.Fprintf(&b, `<circle data-group="%d" data-owner="%d" data-box="%d" data-atomic="%d" data-node="%d" cx="%.6f" cy="%.6f" r="0.75"/>`, a.GroupID, a.OwnerID, a.BoxID, a.ID, a.Locator.NodeID, a.X, a.Y)
	}
	b.WriteString(`</g><g id="glyphs">`)
	for _, g := range p.Glyphs {
		fmt.Fprintf(&b, `<path data-group="%d" data-owner="%d" data-line="%d" data-knot="%d" data-gid="%d" data-cluster="%d" data-byte="%d" data-font="%s" fill="%s" transform="translate(%.6f %.6f) scale(%.9f)" d="%s"/>`, g.GroupID, g.OwnerID, g.Line, g.Knot, g.GlyphID, g.Cluster, g.SourceByte, escape(g.FaceSHA256), escape(g.Color), g.X, g.Y, g.Scale, escape(g.Path))
	}
	b.WriteString(`</g>`)
	if len(p.Leaves) > 0 {
		b.WriteString(`<g id="leaves">`)
		for _, leaf := range p.Leaves {
			c := leaf.Content
			if leaf.Image != nil {
				info := leaf.Image.Info()
				fmt.Fprintf(&b, `<image data-leaf="image" data-box="%d" data-node="%d" data-sha256="%s" x="%.6f" y="%.6f" width="%.6f" height="%.6f" preserveAspectRatio="none" href="data:%s;base64,%s"><title>%s</title></image>`, leaf.BoxID, leaf.Locator.NodeID, info.SHA256, c.X, c.Y, c.W, c.H, info.MIME, base64.StdEncoding.EncodeToString(leaf.Image.data), escape(leaf.Label))
				continue
			}
			fmt.Fprintf(&b, `<rect data-leaf="rectangle" data-box="%d" data-node="%d" x="%.6f" y="%.6f" width="%.6f" height="%.6f" fill="#f59e0b"><title>%s</title></rect>`, leaf.BoxID, leaf.Locator.NodeID, c.X, c.Y, c.W, c.H, escape(leaf.Label))
		}
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
