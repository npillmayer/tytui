package documentlayout

import (
	"fmt"
	"math"
	"slices"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/typography/render"
)

// ProofRect is absolute document geometry in float64 big points, Y downward.
type ProofRect struct{ X, Y, W, H float64 }
type ProofBox struct {
	BoxID, RelativeTo box.BoxID
	Frame, Content    ProofRect
}
type ProofLine struct {
	GroupID, OwnerID box.BoxID
	render.Line
}
type ProofGlyph struct {
	GroupID, OwnerID box.BoxID
	render.Glyph
	// Sources contains every text provenance span covering Glyph.SourceByte.
	// Consult the span's Kind before mapping a collapsed anchor to DOM bytes.
	Sources []TextSpan
}
type ProofAtomic struct {
	GroupID, OwnerID box.BoxID
	AtomicSource
	Line int
	X, Y float64 // precise paragraph border origin, before CSS summary narrowing
}

// ProofLeaf carries renderer-owned diagnostic payload and absolute content geometry.
type ProofLeaf struct {
	BoxID   box.BoxID
	Locator Locator
	Content ProofRect
	Label   string
	Kind    string
	Image   *ImageResource
}

// Proof is a detached diagnostic with shared immutable image resources.
// Coordinates are absolute big points; source locators address decoded DOM bytes.
type Proof struct {
	Leaves                 []ProofLeaf
	Boxes                  []ProofBox
	Lines                  []ProofLine
	Glyphs                 []ProofGlyph
	Atomics                []ProofAtomic
	MinX, MinY, MaxX, MaxY float64
}

// BuildProof composes retained owner-content transforms exactly once. It never
// resolves styles, prepares, measures, shapes, breaks or stacks paragraph lines.
// Outlines/identity come from explicitly supplied services for the retained faces.
// Errors return no partial proof. Keep inputs/services stable.
func BuildProof(result *Result, services render.Services) (*Proof, error) {
	return buildProof(result, services, false)
}

func buildProof(result *Result, services render.Services, boxesOnly bool) (*Proof, error) {
	if result == nil || result.document == nil {
		return nil, invalid("document proof requires context and a completed result")
	}

	p := &Proof{}
	include := p.include

	// Geometry.RelativeTo identifies a content-coordinate parent, including for
	// anonymous owners. Cache each absolute content origin, never add Frame twice.
	origins := map[box.BoxID]render.Point{0: {}}
	visiting := map[box.BoxID]bool{}
	var contentOrigin func(box.BoxID) (render.Point, error)
	contentOrigin = func(id box.BoxID) (render.Point, error) {
		if p, ok := origins[id]; ok {
			return p, nil
		}
		g, ok := result.geometry[id]
		if !ok || visiting[id] {
			return render.Point{}, invalid("missing or cyclic geometry parent")
		}
		visiting[id] = true
		parent, err := contentOrigin(g.RelativeTo)
		if err != nil {
			return render.Point{}, err
		}
		point := render.Point{X: parent.X + float64(g.Content.X)*.75, Y: parent.Y + float64(g.Content.Y)*.75}
		if !finiteProof(point.X, point.Y) {
			return render.Point{}, invalid("non-finite document origin")
		}
		origins[id] = point
		delete(visiting, id)
		return point, nil
	}
	ids := make([]box.BoxID, 0, len(result.geometry))
	for id := range result.geometry {
		ids = append(ids, id)
	}
	slices.Sort(ids) // stable diagnostics/SVG, independent of map iteration
	for _, id := range ids {
		g := result.geometry[id]
		if _, err := contentOrigin(id); err != nil {
			return nil, err
		}
		parent, err := contentOrigin(g.RelativeTo)
		if err != nil {
			return nil, err
		}
		absolute := func(r box.Rect) ProofRect {
			return ProofRect{parent.X + float64(r.X)*.75, parent.Y + float64(r.Y)*.75, float64(r.W) * .75, float64(r.H) * .75}
		}
		b := ProofBox{BoxID: id, RelativeTo: g.RelativeTo, Frame: absolute(g.Frame), Content: absolute(g.Content)}
		for _, r := range []ProofRect{b.Frame, b.Content} {
			if !finiteProof(r.X, r.Y, r.W, r.H) || r.W < 0 || r.H < 0 {
				return nil, invalid("invalid document proof box")
			}
			include(r.X, r.Y)
			include(r.X+r.W, r.Y+r.H)
		}
		p.Boxes = append(p.Boxes, b)
	}
	if boxesOnly {
		return p, nil
	}
	var leaves func(*box.LayoutNode)
	leaves = func(n *box.LayoutNode) {
		if payload, ok := result.document.leaves[n.NodeID]; ok {
			for _, b := range p.Boxes {
				if b.BoxID == n.BoxID {
					source, _ := result.document.NodeSource(n.NodeID)
					p.Leaves = append(p.Leaves, ProofLeaf{BoxID: n.BoxID, Locator: source.Locator, Content: b.Content, Label: payload.Label, Kind: payload.Kind, Image: payload.Image})
				}
			}
		}
		for _, child := range n.Children {
			leaves(child)
		}
	}
	leaves(result.document.root)
	for _, group := range result.document.order {
		g, ok := result.groups[group.GroupID]
		if !ok {
			return nil, invalid("missing retained paragraph")
		}
		origin, err := contentOrigin(group.OwnerID)
		if err != nil {
			return nil, err
		}
		local, err := render.BuildParagraphProof(render.ParagraphInput{
			Frozen: g.Paragraph.Frozen, VList: g.Paragraph.VList, Text: g.Input.Text, Paint: g.Input.Paint,
		}, services)
		if err != nil {
			return nil, fmt.Errorf("document proof group %d: %w", group.GroupID, err)
		}
		if err := p.appendParagraph(g, origin, local); err != nil {
			return nil, err
		}

		byID := make(map[contracts.AtomicID]AtomicSource, len(g.Input.AtomicSources))
		for _, source := range g.Input.AtomicSources {
			byID[source.ID] = source
		}
		for _, a := range g.Paragraph.VList.AtomicPlacements() {
			source, ok := byID[a.ID]
			if !ok {
				return nil, invalid("atomic proof has no source")
			}
			p.Atomics = append(p.Atomics, ProofAtomic{group.GroupID, group.OwnerID, source, a.Line, origin.X + a.X, origin.Y + a.Y})
		}
	}

	if !finiteProof(p.MinX, p.MinY, p.MaxX, p.MaxY) {
		return nil, invalid("non-finite proof bounds")
	}
	return p, nil
}

func finiteProof(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}
