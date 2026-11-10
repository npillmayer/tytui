package pipeline

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/npillmayer/khipu/dimen"
	"golang.org/x/image/colornames"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/bidi"
	"io"
	"math"
	"regexp"
	"strings"
	"tytui/fontbackend"
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

func proofColor(result *Result, pos uint64) (string, error) {
	color := "black"
	if result.Input.Source != nil {
		for _, s := range result.Input.Source.Styles {
			if s.Start <= pos && pos < s.End {
				color = strings.ToLower(strings.TrimSpace(s.Style.Color))
				break
			}
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
// Knots are already physical LTR. Within an RTL knot, frozen logical glyphs are
// visited in reverse visual order, preserving their supplied advances/offsets.
func BuildGlyphProof(result *Result) (*GlyphProof, error) {
	if result == nil || result.Frozen == nil || result.Input.Fonts == nil {
		return nil, fmt.Errorf("glyph proof requires a frozen OpenType result")
	}
	p := &GlyphProof{}
	top := 0.0
	include := func(x, y float64) {
		p.MinX = math.Min(p.MinX, x)
		p.MaxX = math.Max(p.MaxX, x)
		p.MinY = math.Min(p.MinY, y)
		p.MaxY = math.Max(p.MaxY, y)
	}
	for li := 0; li < result.Frozen.LineCount(); li++ {
		line, _ := result.Frozen.Line(li)
		ascent, descent, gap := 0.0, 0.0, 0.0
		for k := line.FirstKnot; k < line.FirstKnot+line.KnotCount; k++ {
			t, ok := result.Frozen.CopyText(k)
			if !ok {
				continue
			}
			m, err := t.Context.Params.Font.Metrics()
			if err != nil {
				return nil, err
			}
			sc := t.Context.Scale
			ascent = math.Max(ascent, sc.ToDU(m.Ascent)/float64(dimen.BP))
			descent = math.Max(descent, -sc.ToDU(m.Descent)/float64(dimen.BP))
			gap = math.Max(gap, math.Max(sc.ToDU(m.LineGap)/float64(dimen.BP), t.Context.EmSize.Points()*.2))
		}
		if ascent+descent == 0 {
			ascent = result.Input.Fonts.Size.Points() * .8
			descent = result.Input.Fonts.Size.Points() * .2
			gap = result.Input.Fonts.Size.Points() * .2
		}
		baseline := top + ascent
		height := ascent + descent + gap
		p.Lines = append(p.Lines, ProofLine{0, baseline, top, height, line.Width.Points(), line.TargetWidth.Points()})
		include(0, top)
		include(math.Max(line.Width.Points(), line.TargetWidth.Points()), top+height)
		x := line.LeftSkip.Points()
		for k := line.FirstKnot; k < line.FirstKnot+line.KnotCount; k++ {
			knot, _ := result.Frozen.Knot(k)
			t, ok := result.Frozen.CopyText(k)
			if ok {
				face, ok := t.Context.Params.Font.(*fontbackend.Face)
				if !ok {
					return nil, fmt.Errorf("unsupported proof face %T", t.Context.Params.Font)
				}
				color, err := proofColor(result, knot.Source.Bytes.Start)
				if err != nil {
					return nil, err
				}
				scale := t.Context.Scale.ToDU(1) / float64(dimen.BP)
				penX, penY := x, baseline
				for v := range t.Glyphs {
					idx := v
					if t.Context.Params.Direction == bidi.RightToLeft {
						idx = len(t.Glyphs) - 1 - v
					}
					g := t.Glyphs[idx]
					gx, gy := penX+float64(g.XOffset)*scale, penY-float64(g.YOffset)*scale
					segments, err := face.Outline(g.GlyphID)
					if err != nil {
						return nil, err
					}
					var path strings.Builder
					for _, seg := range segments {
						count := 0
						switch seg.Op {
						case sfnt.SegmentOpMoveTo:
							path.WriteString("M")
							count = 1
						case sfnt.SegmentOpLineTo:
							path.WriteString("L")
							count = 1
						case sfnt.SegmentOpQuadTo:
							path.WriteString("Q")
							count = 2
						case sfnt.SegmentOpCubeTo:
							path.WriteString("C")
							count = 3
						default:
							return nil, fmt.Errorf("unsupported outline operation")
						}
						for a := 0; a < count; a++ {
							px, py := float64(seg.Args[a].X)/64, float64(seg.Args[a].Y)/64
							fmt.Fprintf(&path, "%.6g %.6g ", px, py)
							include(gx+px*scale, gy+py*scale)
						}
					}
					p.Glyphs = append(p.Glyphs, ProofGlyph{li, k, g.GlyphID, g.Cluster, gx, gy, scale, color, path.String(), face.Info().SHA256})
					penX += float64(g.XAdvance) * scale
					penY -= float64(g.YAdvance) * scale
				}
			}
			x += knot.W.Points()
		}
		top += height
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
