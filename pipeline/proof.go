package pipeline

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/npillmayer/typography/render"
	"io"
)

// ProofGlyph exposes positioned frozen glyphs in physical drawing order. X/Y
// include shaping offsets, in big points; paths use font units, Y downward.
type ProofGlyph struct {
	Line, Knot       int
	GlyphID, Cluster uint32
	X, Y, Scale      float64
	Color, Path      string
	FaceSHA256       string
}
type ProofLine struct{ X, Baseline, Top, Height, Width, TargetWidth float64 }
type GlyphProof struct {
	Glyphs                 []ProofGlyph
	Lines                  []ProofLine
	MinX, MinY, MaxX, MaxY float64
}

// BuildGlyphProof renders only retained content through the shared proof walker.
func BuildGlyphProof(result *Result) (*GlyphProof, error) {
	if result == nil {
		return nil, fmt.Errorf("glyph proof requires a completed result")
	}
	proof, err := render.BuildParagraphProof(context.Background(), render.ParagraphInput{
		Frozen: result.Frozen, VList: result.vlist, Text: result.ParagraphText,
		Paint: result.paint}, render.Services{Backend: result.Renderer, Identity: result.Identity})
	if err != nil {
		return nil, err
	}
	p := &GlyphProof{MinX: proof.MinX, MinY: proof.MinY, MaxX: proof.MaxX, MaxY: proof.MaxY}
	for _, l := range proof.Lines {
		p.Lines = append(p.Lines, ProofLine(l))
	}
	for _, g := range proof.Glyphs {
		p.Glyphs = append(p.Glyphs, ProofGlyph{g.Line, g.Knot, g.GlyphID, g.Cluster, g.X, g.Y, g.Scale, g.Color, g.Path, g.FaceSHA256})
	}
	return p, nil
}
func WriteGlyphSVG(w io.Writer, result *Result) error {
	if w == nil {
		return fmt.Errorf("glyph proof requires a writer")
	}
	p, err := BuildGlyphProof(result)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.6f %.6f %.6f %.6f"><title>Frozen glyph proof</title>`, p.MinX-12, p.MinY-12, p.MaxX-p.MinX+24, p.MaxY-p.MinY+24)
	fmt.Fprintf(&b, `<rect x="%.6f" y="%.6f" width="%.6f" height="%.6f" fill="white"/>`, p.MinX-12, p.MinY-12, p.MaxX-p.MinX+24, p.MaxY-p.MinY+24)
	b.WriteString(`<g id="geometry" fill="none" stroke="#9ca3af" stroke-width="0.25">`)
	for _, l := range p.Lines {
		fmt.Fprintf(&b, `<rect x="0" y="%.6f" width="%.6f" height="%.6f"/><path d="M0 %.6f H%.6f"/>`, l.Top, l.TargetWidth, l.Height, l.Baseline, l.Width)
	}
	b.WriteString(`</g><g id="glyphs">`)
	for _, g := range p.Glyphs {
		fmt.Fprintf(&b, `<path data-line="%d" data-knot="%d" data-gid="%d" data-cluster="%d" data-font="%s" fill="%s" transform="translate(%.6f %.6f) scale(%.9f)" d="`, g.Line, g.Knot, g.GlyphID, g.Cluster, g.FaceSHA256, g.Color, g.X, g.Y, g.Scale)
		if err := xml.EscapeText(&b, []byte(g.Path)); err != nil {
			return err
		}
		b.WriteString(`"/>`)
	}
	b.WriteString(`</g></svg>`)
	n, err := w.Write(b.Bytes())
	if err == nil && n != b.Len() {
		return io.ErrShortWrite
	}
	return err
}
