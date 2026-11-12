package pipeline

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/render"
	"golang.org/x/image/colornames"
	"io"
	"math"
	"regexp"
	"strings"
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

var hexColor = regexp.MustCompile(`^#(?:[0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$`)

func proofColor(spans []typography.PaintSpan, pos uint64) (string, error) {
	color := "black"
	for _, s := range spans {
		if s.Start <= pos && pos < s.End {
			color = strings.ToLower(strings.TrimSpace(s.Paint.Color))
			break
		}
	}
	if color == "transparent" || hexColor.MatchString(color) {
		return color, nil
	}
	if _, ok := colornames.Map[color]; ok {
		return color, nil
	}
	return "", fmt.Errorf("unsupported proof color %q: use a named or hex color", color)
}

// BuildGlyphProof never reshapes, rebreaks, reloads a font file, or reorders knots.
// Vertical placement comes from the retained vlist; rendering never queries line
// metrics or derives baselines from mutable Input.
// Knots are already physical LTR. VisualOrder converts each detached logical
// glyph fragment to drawing order, preserving its supplied advances/offsets.
func BuildGlyphProof(result *Result) (*GlyphProof, error) {
	if result == nil || result.Frozen == nil || result.vlist == nil || result.vlist.Frozen() != result.Frozen || result.Renderer == nil || result.Identity == nil {
		return nil, fmt.Errorf("glyph proof requires retained placements, matching frozen content and rendering services")
	}
	p := &GlyphProof{}
	include := func(x, y float64) {
		p.MinX = math.Min(p.MinX, x)
		p.MaxX = math.Max(p.MaxX, x)
		p.MinY = math.Min(p.MinY, y)
		p.MaxY = math.Max(p.MaxY, y)
	}
	for _, placement := range result.vlist.Lines() {
		li, line := placement.Index, placement.Horizontal
		baseline := placement.Baseline
		p.Lines = append(p.Lines, ProofLine{placement.X, baseline, placement.Top, placement.Height, line.Width.Points(), line.TargetWidth.Points()})
		include(placement.X, placement.Top)
		include(placement.X+math.Max(line.Width.Points(), line.TargetWidth.Points()), placement.Top+placement.Height)
		x := placement.X + line.LeftSkip.Points()
		for k := line.FirstKnot; k < line.FirstKnot+line.KnotCount; k++ {
			knot, _ := result.Frozen.Knot(k)
			t, ok := result.Frozen.CopyText(k)
			if ok {
				identity, err := result.Identity.Identify(t.Context.Params.Font)
				if err != nil {
					return nil, err
				}
				color, err := proofColor(result.paint, knot.Source.Bytes.Start)
				if err != nil {
					return nil, err
				}
				var sources map[uint32]uint64
				if result.paintPolicy != typography.LegacyStyleBoundary {
					sources, err = render.PaintSources(result.ParagraphText, knot, t)
					if err != nil {
						return nil, err
					}
				}
				scale := t.Context.Scale.ToDU(1) / float64(dimen.BP)
				penX, penY := x, baseline
				// CopyText owns these slices; in-place ordering leaves frozen data intact.
				visual, err := result.Renderer.VisualOrder(t.Glyphs, t.Context.Params.Direction)
				if err != nil {
					return nil, fmt.Errorf("glyph proof line %d knot %d: %w", li, k, err)
				}
				for _, g := range visual {
					if sources != nil {
						color, err = proofColor(result.paint, sources[g.Cluster])
						if err != nil {
							return nil, err
						}
					}
					gx, gy := penX+float64(g.XOffset)*scale, penY-float64(g.YOffset)*scale
					outline, err := result.Renderer.Outline(t.Context.Params.Font, g.GlyphID)
					if err != nil {
						return nil, err
					}
					for _, point := range outline.Points {
						include(gx+point.X*scale, gy+point.Y*scale)
					}
					p.Glyphs = append(p.Glyphs, ProofGlyph{li, k, g.GlyphID, g.Cluster, gx, gy, scale, color, outline.Path, identity.SHA256})
					penX += float64(g.XAdvance) * scale
					penY -= float64(g.YAdvance) * scale
				}
			}
			x += knot.W.Points()
		}
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
