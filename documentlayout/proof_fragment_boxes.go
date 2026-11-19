package documentlayout

import (
	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/typography/render"
)

func (w *fragmentProofWalker) index(n *box.LayoutNode) error {
	if n == nil || w.nodes[n.BoxID] != nil {
		return invalid("missing or cyclic retained box tree")
	}
	w.nodes[n.BoxID] = n
	for _, child := range n.Children {
		if err := w.index(child); err != nil {
			return err
		}
	}
	return nil
}

// Walk only an included opaque subtree. Inline groups visit atomic children via
// their precise placements, never again through the structural child list.
func (w *fragmentProofWalker) box(n *box.LayoutNode, border *render.Point) error {
	if n == nil || w.visited[n.BoxID] {
		return invalid("missing or repeated opaque box")
	}
	w.visited[n.BoxID] = true
	if n.Box == box.BoxAnonymousInline {
		g, ok := w.result.groups[n.BoxID]
		if !ok || g.Paragraph.VList == nil {
			return invalid("missing opaque paragraph")
		}
		origin, ok := w.origins[g.Input.OwnerID]
		if !ok {
			return invalid("missing opaque paragraph origin")
		}
		var local *render.ParagraphProof
		var err error
		if w.boxesOnly {
			local, err = retainedLineProof(g.Paragraph.VList.Lines())
		} else {
			local, err = render.BuildParagraphProof(render.ParagraphInput{Frozen: g.Paragraph.Frozen, VList: g.Paragraph.VList, Text: g.Input.Text, Paint: g.Input.Paint}, w.services)
		}
		if err != nil {
			return err
		}
		return w.paragraph(g, origin, local, g.Paragraph.VList.AtomicPlacements())
	}
	if err := w.boxGeometry(n, border); err != nil {
		return err
	}
	for _, child := range n.Children {
		if err := w.box(child, nil); err != nil {
			return err
		}
	}
	return nil
}

func (w *fragmentProofWalker) boxGeometry(n *box.LayoutNode, border *render.Point) error {
	g, ok := w.result.geometry[n.BoxID]
	if !ok {
		return invalid("missing opaque box geometry")
	}
	parent, ok := w.origins[g.RelativeTo]
	if border == nil && !ok {
		return invalid("missing opaque geometry parent")
	}
	absolute := func(r box.Rect) ProofRect {
		return ProofRect{parent.X + float64(r.X)*.75, parent.Y + float64(r.Y)*.75, float64(r.W) * .75, float64(r.H) * .75}
	}
	b := ProofBox{BoxID: n.BoxID, RelativeTo: g.RelativeTo, Frame: absolute(g.Frame), Content: absolute(g.Content)}
	if border != nil {
		// Replace the old CSS border origin, preserving content edges. Do not
		// narrow the precise paragraph placement back to float32 CSS geometry.
		b.Frame.X, b.Frame.Y = border.X, border.Y
		b.Content.X = border.X + (float64(g.Content.X)-float64(g.Frame.X))*.75
		b.Content.Y = border.Y + (float64(g.Content.Y)-float64(g.Frame.Y))*.75
	}
	for _, r := range []ProofRect{b.Frame, b.Content} {
		if !finiteProof(r.X, r.Y, r.W, r.H, r.X+r.W, r.Y+r.H) || r.W < 0 || r.H < 0 {
			return invalid("invalid opaque proof geometry")
		}
		w.proof.include(r.X, r.Y)
		w.proof.include(r.X+r.W, r.Y+r.H)
	}
	w.origins[n.BoxID] = render.Point{X: b.Content.X, Y: b.Content.Y}
	w.proof.Boxes = append(w.proof.Boxes, b)
	if payload, ok := w.result.document.leaves[n.NodeID]; ok {
		source, _ := w.result.document.NodeSource(n.NodeID)
		w.proof.Leaves = append(w.proof.Leaves, ProofLeaf{BoxID: n.BoxID, Locator: source.Locator, Content: b.Content, Label: payload.Label, Kind: payload.Kind, Image: payload.Image})
	}
	return nil
}
