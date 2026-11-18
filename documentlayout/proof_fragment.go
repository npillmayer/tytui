package documentlayout

import (
	"context"
	"fmt"
	"io"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/typography/layout"
	"github.com/npillmayer/typography/render"
)

// PositionedFragment associates retained lines with their original document
// group and an explicit destination. FrameOrigin and Offset are big points,
// downward-positive Y; their sum is the fragment's local origin. They already
// account for application-owned margins/edges. The old owner's origin is not
// added. Text, styles, paint and fixed atomic interiors must match Result's group;
// continuation fragments may have different frozen owners and target widths.
type PositionedFragment struct {
	GroupID     box.BoxID
	Fragment    *layout.Fragment
	FrameOrigin render.Point
	Offset      render.Point
}

// BuildFragmentProof draws the supplied placements in order, including only
// their retained atomic subtrees. It does not schedule frames, select cuts or
// reflow content. Disjoint ranges are the caller's responsibility; a repeated
// placement explicitly draws that range again. Empty ranges draw nothing.
// Fixed opaque interiors retain their original dimensions and layout. Ordinary
// ancestor boxes outside those interiors are not repeated across frames.
// All input is borrowed for this call; the detached proof retains no preparation,
// continuation or result handle. Errors/cancellation publish no partial proof.
func BuildFragmentProof(ctx context.Context, result *Result, placements []PositionedFragment, services render.Services) (*Proof, error) {
	return buildFragmentProof(ctx, result, placements, services, false)
}

func buildFragmentProof(ctx context.Context, result *Result, placements []PositionedFragment, services render.Services, boxesOnly bool) (*Proof, error) {
	if ctx == nil || result == nil || result.document == nil || (!boxesOnly && (services.Backend == nil || services.Identity == nil)) {
		return nil, invalid("fragment proof requires a completed result and rendering services")
	}
	w := fragmentProofWalker{ctx: ctx, result: result, services: services, boxesOnly: boxesOnly, proof: &Proof{}, nodes: map[box.BoxID]*box.LayoutNode{}}
	if err := w.index(result.document.root); err != nil {
		return nil, err
	}
	for _, placement := range placements {
		if err := w.fragment(placement); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := w.proof
	if !finiteProof(p.MinX, p.MinY, p.MaxX, p.MaxY) {
		return nil, invalid("non-finite fragment proof bounds")
	}
	return p, nil
}

// WriteFragmentSVG exports a complete retained fragment proof using the ordinary
// diagnostic SVG format. Nothing is written on proof failure/cancellation;
// destination writer errors may leave partial bytes. This is not replay.
func WriteFragmentSVG(ctx context.Context, out io.Writer, result *Result, placements []PositionedFragment, services render.Services) error {
	if out == nil {
		return invalid("fragment proof requires a writer")
	}
	p, err := BuildFragmentProof(ctx, result, placements, services)
	if err != nil {
		return err
	}
	return writeProofSVG(ctx, out, p)
}

type fragmentProofWalker struct {
	boxesOnly bool
	ctx       context.Context
	result    *Result
	services  render.Services
	proof     *Proof
	nodes     map[box.BoxID]*box.LayoutNode
	origins   map[box.BoxID]render.Point
	visited   map[box.BoxID]bool
}

func (w *fragmentProofWalker) fragment(p PositionedFragment) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	g, ok := w.result.groups[p.GroupID]
	if !ok || p.Fragment == nil || p.Fragment.Frozen() == nil {
		return invalid("fragment proof has no matching group or frozen content")
	}
	origin := render.Point{X: p.FrameOrigin.X + p.Offset.X, Y: p.FrameOrigin.Y + p.Offset.Y}
	if !finiteProof(p.FrameOrigin.X, p.FrameOrigin.Y, p.Offset.X, p.Offset.Y, origin.X, origin.Y) {
		return invalid("non-finite fragment origin")
	}
	// A new frozen owner is legal, but it must address the same paragraph bytes.
	frozen := p.Fragment.Frozen()
	if frozen.SourceLen() != uint64(len(g.Input.Text)) {
		return invalid("fragment source does not match retained group")
	}
	r, err := frozen.SourceReader(khipu.ByteRange{End: frozen.SourceLen()})
	if err != nil {
		return err
	}
	text, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if string(text) != g.Input.Text {
		return invalid("fragment source does not match retained group")
	}
	var local *render.ParagraphProof
	if w.boxesOnly {
		lines := make([]layout.Line, len(p.Fragment.Lines()))
		for j, line := range p.Fragment.Lines() {
			lines[j] = line.Line
		}
		local, err = retainedLineProof(w.ctx, lines)
	} else {
		local, err = render.BuildFragmentProof(w.ctx, render.FragmentInput{Fragment: p.Fragment, Text: g.Input.Text, Paint: g.Input.Paint}, w.services)
	}
	if err != nil {
		return fmt.Errorf("fragment proof group %d: %w", p.GroupID, err)
	}
	if len(local.Lines) == 0 {
		return nil // No drawing or bounds contribution from an empty range.
	}
	w.origins = map[box.BoxID]render.Point{g.Input.OwnerID: origin}
	w.visited = map[box.BoxID]bool{p.GroupID: true}
	return w.paragraph(g, origin, local, p.Fragment.AtomicPlacements())
}

func (w *fragmentProofWalker) paragraph(g GroupResult, origin render.Point, local *render.ParagraphProof, atomics []layout.AtomicPlacement) error {
	if err := w.proof.appendParagraph(g, origin, local); err != nil {
		return err
	}
	byID := make(map[contracts.AtomicID]AtomicSource, len(g.Input.AtomicSources))
	for _, source := range g.Input.AtomicSources {
		byID[source.ID] = source
	}
	for _, a := range atomics {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		source, ok := byID[a.ID]
		if !ok {
			return invalid("atomic fragment proof has no source")
		}
		border := render.Point{X: origin.X + a.X, Y: origin.Y + a.Y}
		if !finiteProof(border.X, border.Y) {
			return invalid("non-finite atomic origin")
		}
		w.proof.Atomics = append(w.proof.Atomics, ProofAtomic{g.Input.GroupID, g.Input.OwnerID, source, a.Line, border.X, border.Y})
		if err := w.box(w.nodes[source.BoxID], &border); err != nil {
			return err
		}
	}
	return nil
}
