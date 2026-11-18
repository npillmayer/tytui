package documentlayout

import (
	"math"

	"github.com/npillmayer/typography/render"
)

func (p *Proof) include(x, y float64) {
	p.MinX = math.Min(p.MinX, x)
	p.MinY = math.Min(p.MinY, y)
	p.MaxX = math.Max(p.MaxX, x)
	p.MaxY = math.Max(p.MaxY, y)
}

// Consume a detached local proof, adding its application origin exactly once.
func (p *Proof) appendParagraph(g GroupResult, origin render.Point, local *render.ParagraphProof) error {
	p.include(origin.X+local.MinX, origin.Y+local.MinY)
	p.include(origin.X+local.MaxX, origin.Y+local.MaxY)
	for _, l := range local.Lines {
		l.X += origin.X
		l.Top += origin.Y
		l.Baseline += origin.Y
		if !finiteProof(l.X, l.Top, l.Baseline) {
			return invalid("non-finite translated line")
		}
		p.Lines = append(p.Lines, ProofLine{g.Input.GroupID, g.Input.OwnerID, l})
	}
	for _, glyph := range local.Glyphs {
		glyph.X += origin.X
		glyph.Y += origin.Y
		if !finiteProof(glyph.X, glyph.Y) {
			return invalid("non-finite translated glyph")
		}
		out := ProofGlyph{GroupID: g.Input.GroupID, OwnerID: g.Input.OwnerID, Glyph: glyph}
		for _, source := range g.Input.TextSources {
			if source.Start <= glyph.SourceByte && glyph.SourceByte < source.End {
				out.Sources = append(out.Sources, source)
			}
		}
		if len(out.Sources) == 0 {
			return invalid("glyph has no retained text provenance")
		}
		p.Glyphs = append(p.Glyphs, out)
	}
	return nil
}
