package documentlayout

import (
	"context"
	"fmt"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/paragraph"
)

// LayoutRequest specifies the containing-block width in CSS pixels, not DU or
// big points. Zero width is valid. Select FirstFit or KnuthPlass explicitly.
type LayoutRequest struct {
	Width   float32
	Breaker paragraph.Breaker
}

// Config supplies caller-owned typography services. No backend, font discovery
// or coverage policy is installed here. The caller supplies suitable fonts;
// ItemizeInline's existing checks remain until their separate relocation.
// Services must be stable during layout and retained font/scale services must
// remain usable for the result's lifetime. Serialize calls sharing a Resolver.
type Config struct {
	Resolver      *typography.Resolver
	NewShaper     func() (contracts.StreamingShaper, error) // fresh exclusive stream per call
	BufferOptions contracts.ShapeBufferOptions
}

// Layout resolves CSS used values and runs fixed-width document flow. CSS core
// prepares nested atomics; each callback itemizes/prepares/layouts just one group.
// Initial policy is normal CSS line height, bottom-margin-edge atomic alignment,
// no hyphenation and explicit zero side skips. There is no intrinsic measurer.
//
// Only successful whole-document completion publishes a Result. Errors return
// nil and leave all previous snapshots intact. There is no mutable current result
// on Document: the caller replaces its displayed snapshot after success. Context
// is checked at callback/publication boundaries; synchronous CSS and paragraph
// collaborators retain their cancellation limits. Root must remain read-only.
func (d *Document) Layout(ctx context.Context, request LayoutRequest, config Config) (*Result, error) {
	if ctx == nil {
		return nil, invalid("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d == nil || d.root == nil || config.Resolver == nil || config.NewShaper == nil {
		return nil, invalid("document, resolver and shaper factory are required")
	}
	if _, err := cssToDU(request.Width, false); err != nil {
		return nil, err
	}
	if request.Breaker != paragraph.FirstFit && request.Breaker != paragraph.KnuthPlass {
		return nil, invalid("unknown line breaker")
	}
	i := &invocation{ctx: ctx, document: d, request: request, config: config,
		started: make(map[box.BoxID]bool), groups: make(map[box.BoxID]GroupResult)}
	return i.run()
}

type invocation struct {
	ctx      context.Context
	document *Document
	request  LayoutRequest
	config   Config
	started  map[box.BoxID]bool
	groups   map[box.BoxID]GroupResult
}

var _ box.InlineLayouter = (*invocation)(nil)

func (i *invocation) run() (*Result, error) {
	containing := box.Rect{W: i.request.Width}
	used, err := box.ResolveUsedValues(i.document.root, box.ResolveContext{ContainingBlock: containing})
	if err != nil {
		return nil, err
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	flow, err := box.FlowLayout(i.document.root, used, i, nil, box.LayoutContext{ContainingBlock: containing}, box.LayoutOptions{})
	if err != nil {
		return nil, err
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	if len(i.groups) != len(i.document.groups) {
		return nil, invalid("CSS flow did not complete every inline group")
	}
	// Transfer both invocation-local tables together; neither is reused or exposed.
	return &Result{document: i.document, geometry: flow.Geometry, groups: i.groups}, nil
}

func (i *invocation) LayoutInline(request box.InlineRequest) (box.InlineResult, error) {
	fail := func(err error) (box.InlineResult, error) { return box.InlineResult{}, err }
	if err := i.ctx.Err(); err != nil {
		return fail(err)
	}
	if request.Group == nil || request.Used == nil || request.Atomics == nil {
		return fail(invalid("incomplete inline request"))
	}
	id := request.Group.BoxID
	entry, ok := i.document.groups[id]
	if !ok || request.Group != entry.node || request.OwnerID != entry.OwnerID {
		return fail(invalid("unknown inline group or mismatched owner"))
	}
	if i.started[id] {
		return fail(unsupported("repeated inline-group callback"))
	}
	i.started[id] = true
	width, err := cssToDU(request.ContentWidth, false)
	if err != nil {
		return fail(err)
	}
	g, err := i.document.Extract(i.ctx, id)
	if err != nil {
		return fail(err)
	}
	atoms, err := atomicBoxes(g, request)
	if err != nil {
		return fail(err)
	}
	r := i.config.Resolver
	text, _, err := r.ItemizeInline(i.ctx, g.Text, g.Styles, g.Anchors, typography.GlyphBoundary)
	if err != nil {
		return fail(err)
	}
	// The strut is the owner's inherited typography, not the first glyph/atomic.
	defaults, err := r.ResolveStyle(g.Paragraph.Style, r.Defaults())
	if err != nil {
		return fail(err)
	}
	markup, err := g.BidiMarkup()
	if err != nil {
		return fail(err)
	}
	prepared, err := paragraph.Prepare(i.ctx, paragraph.Input{Text: text, Direction: g.Paragraph.Style.Direction, Markup: markup, Atomics: g.Anchors},
		khipu.PrepareOptions{Hyphenation: khipu.NoHyphenation}, khipu.Config{Streaming: &khipu.StreamingConfig{
			Resolver: r, Defaults: defaults, NewShaper: i.config.NewShaper, BufferOptions: i.config.BufferOptions,
		}})
	if err != nil {
		return fail(err)
	}
	result, err := prepared.Layout(i.ctx, paragraph.LayoutRequest{Params: paragraph.Params{
		Constraints: paragraph.Constraints{Width: width}, Breaker: i.request.Breaker,
		Assembly: paragraph.Options{Policy: paragraph.CSSLineHeight}, LineLayout: &g.LineLayout,
	}, Horizontal: &paragraph.HorizontalLayout{}, Atomics: atoms})
	if err != nil {
		return fail(err)
	}
	summary, err := inlineSummary(g, result)
	if err != nil {
		return fail(err)
	}
	if err := i.ctx.Err(); err != nil {
		return fail(err)
	}
	i.groups[id] = GroupResult{Input: *g, Paragraph: *result}
	return summary, nil
}

func atomicBoxes(g *InlineGroup, request box.InlineRequest) ([]paragraph.AtomicBox, error) {
	atoms := make([]paragraph.AtomicBox, 0, len(g.AtomicSources))
	for _, source := range g.AtomicSources {
		prepared, ok := request.Atomics.Lookup(source.BoxID)
		if !ok || prepared.BoxID != source.BoxID {
			return nil, invalid("missing or mismatched prepared atomic box")
		}
		used, ok := request.Used.Lookup(source.BoxID)
		if !ok || used.WidthPending || used.EdgesPending {
			return nil, invalid("missing or provisional atomic used values")
		}
		a := paragraph.AtomicBox{ID: source.ID, Alignment: paragraph.AlignBottomMarginEdge}
		// CSS border dimensions already include padding/borders. Convert margins
		// independently; paragraph owns checked sums and baseline alignment.
		for _, v := range []struct {
			px     float32
			signed bool
			dest   *dimen.DU
		}{
			{prepared.BorderWidth, false, &a.BorderWidth}, {prepared.BorderHeight, false, &a.BorderHeight},
			{used.Margin.Top, true, &a.Margin.Top}, {used.Margin.Right, true, &a.Margin.Right},
			{used.Margin.Bottom, true, &a.Margin.Bottom}, {used.Margin.Left, true, &a.Margin.Left},
		} {
			value, err := cssToDU(v.px, v.signed)
			if err != nil {
				return nil, fmt.Errorf("atomic box %d: %w", source.BoxID, err)
			}
			*v.dest = value
		}
		atoms = append(atoms, a)
	}
	return atoms, nil
}

func inlineSummary(g *InlineGroup, result *paragraph.Result) (box.InlineResult, error) {
	var out box.InlineResult
	var err error
	geometry := result.VList.Box()
	if out.Extent.W, err = pointsToCSS(geometry.Width, false); err != nil {
		return box.InlineResult{}, err
	}
	if out.Extent.H, err = pointsToCSS(geometry.Extent, false); err != nil {
		return box.InlineResult{}, err
	}
	pending := make(map[contracts.AtomicID]box.BoxID, len(g.AtomicSources))
	for _, source := range g.AtomicSources {
		pending[source.ID] = source.BoxID
	}
	for _, p := range result.VList.AtomicPlacements() {
		id, ok := pending[p.ID]
		if !ok {
			return box.InlineResult{}, invalid("unknown or duplicate paragraph atomic placement")
		}
		x, err := pointsToCSS(p.X, true)
		if err != nil {
			return box.InlineResult{}, err
		}
		y, err := pointsToCSS(p.Y, true)
		if err != nil {
			return box.InlineResult{}, err
		}
		out.Atomics = append(out.Atomics, box.AtomicPlacement{BoxID: id, X: x, Y: y})
		delete(pending, p.ID)
	}
	if len(pending) != 0 {
		return box.InlineResult{}, invalid("missing paragraph atomic placement")
	}
	return out, nil
}
