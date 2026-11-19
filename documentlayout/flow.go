package documentlayout

import (
	"fmt"
	"math"
	"slices"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/layout"
	"github.com/npillmayer/typography/render"
)

// FlowFrame is an already usable content rectangle in CSS pixels. IDs must be
// nonempty and unique. Frames supply no styles and do not inherit paragraph edges.
// X/Y may be signed; Width/Height may be zero. Values stay within the finite DU domain.
type FlowFrame struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// FlowRequest retains ordinary source CSS containing-width/breaker semantics.
// The first frame width must match the initial CSS callback's content width after
// edges, at DU precision. Frames are copied; layouts sharing services are serialized.
type FlowRequest struct {
	Layout LayoutRequest
	Frames []FlowFrame
}

// FrameResult owns a detached diagnostic envelope around an immutable fragment.
// Overfull contains original frozen line indexes of placed horizontally overfull
// lines, not the unplaced tail. Breaker identifies the suffix policy for this frame.
type FrameResult struct {
	Attempted bool // false for destinations after paragraph completion
	Frame     FlowFrame
	Fragment  *layout.Fragment
	Status    paragraph.FragmentFitStatus
	Reused    bool
	Breaker   paragraph.Breaker
	Overfull  []int
}

// FlowResult is an immutable bounded invocation snapshot. It retains frozen
// source/paint/child content and margins, never working preparation or tokens.
// An incomplete result retains a frozen remainder for inspection, not a resumable
// checkpoint. A new flow request is needed to retry with additional frames.
type FlowResult struct {
	source    *Result
	group     box.BoxID
	frames    []FrameResult
	remainder *layout.Fragment
	complete  bool
}

// Frames returns detached envelopes and diagnostics in supplied frame order.
// Frames after completion are retained as empty, unused destinations.
func (r *FlowResult) Frames() []FrameResult {
	out := slices.Clone(r.frames)
	for i := range out {
		out[i].Overfull = slices.Clone(out[i].Overfull)
	}
	return out
}

// Complete reports whether every source line was placed.
func (r *FlowResult) Complete() bool { return r != nil && r.complete }

// Remainder returns the immutable unplaced range (empty on completion).
func (r *FlowResult) Remainder() *layout.Fragment {
	if r == nil {
		return nil
	}
	return r.remainder
}

// Group returns detached original source, paint and style associations.
func (r *FlowResult) Group() InlineGroup { g, _ := r.source.Group(r.group); return g.Input }

// SurroundingMargins returns the original owner margin snapshot, not per-frame margins.
func (r *FlowResult) SurroundingMargins() SurroundingMargins {
	m, _ := r.source.SurroundingMargins(r.group)
	return m
}

// LayoutFlow flows one direct paragraph group of the selected root through a
// finite ordered frame list. Only fixed-width opaque inline children are admitted.
// CSS schedules the single initial layout; subsequent frames use paragraph.Fit.
// No-progress frames are skipped once, and exhaustion returns an explicit frozen
// remainder. Errors return nil. Working state is released on every
// exit, including incomplete flow. There is no document-wide preparation cache.
// Execution is synchronous, including upstream bidi preparation.
func (d *Document) LayoutFlow(request FlowRequest, config Config) (*FlowResult, error) {
	i, frames, err := d.flowInvocation(request, config)
	if err != nil {
		return nil, err
	}
	defer i.releaseFlow()
	source, err := i.run()
	if err != nil {
		return nil, err
	}
	return i.finishFlow(source, frames)
}

type flowInvocation struct {
	group box.BoxID
	width dimen.DU
	token *paragraph.Continuation
}

func (d *Document) flowInvocation(request FlowRequest, config Config) (*invocation, []FlowFrame, error) {
	if d == nil || d.root == nil || config.Resolver == nil || config.NewShaper == nil {
		return nil, nil, invalid("document and services required")
	}
	if _, err := cssToDU(request.Layout.Width, false); err != nil {
		return nil, nil, err
	}
	if request.Layout.Breaker != paragraph.FirstFit && request.Layout.Breaker != paragraph.KnuthPlass {
		return nil, nil, invalid("unknown breaker")
	}
	if len(request.Frames) == 0 {
		return nil, nil, invalid("at least one frame required")
	}
	frames := slices.Clone(request.Frames)
	ids := map[string]bool{}
	for _, f := range frames {
		if f.ID == "" || ids[f.ID] {
			return nil, nil, invalid("frame IDs must be nonempty and unique")
		}
		ids[f.ID] = true
		for _, v := range []struct {
			n      float64
			signed bool
		}{{f.X, true}, {f.Y, true}, {f.Width, false}, {f.Height, false}, {f.X + f.Width, true}, {f.Y + f.Height, true}} {
			if _, err := frameDU(v.n, v.signed); err != nil {
				return nil, nil, err
			}
		}
	}
	if len(d.root.Children) != 1 || d.root.Children[0].Box != box.BoxAnonymousInline {
		return nil, nil, unsupported("flow requires one direct paragraph group; select its block as root")
	}
	var fixed func(*box.LayoutNode) error
	fixed = func(n *box.LayoutNode) error {
		if n.Box == box.BoxInlineBlock && (n.Style == nil || n.Style.Width.Kind != box.LenPx) {
			return unsupported("flow requires fixed pixel widths for opaque inline children")
		}
		for _, c := range n.Children {
			if err := fixed(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := fixed(d.root); err != nil {
		return nil, nil, err
	}
	width, _ := frameDU(frames[0].Width, false)
	i := &invocation{document: d, request: request.Layout, config: config, prepared: map[box.BoxID]*preparedGroup{}, started: map[box.BoxID]bool{}, groups: map[box.BoxID]GroupResult{}, flow: &flowInvocation{group: d.root.Children[0].BoxID, width: width}}
	return i, frames, nil
}

// Check the float64 primitive and rounded DU before casting; never narrow frame
// coordinates through float32 or substitute dimen.PX for the CSS conversion.
func frameDU(px float64, signed bool) (dimen.DU, error) {
	v := math.Round(px * duPerCSSPixel)
	if math.IsNaN(px) || math.IsInf(px, 0) || (!signed && px < 0) || math.Abs(px*duPerCSSPixel) >= float64(dimen.Fil) || math.Abs(v) >= float64(dimen.Fil) {
		return 0, invalid("frame outside finite CSS/DU domain")
	}
	return dimen.DU(v), nil
}

func (i *invocation) releaseFlow() {
	clear(i.prepared)
	if i.flow != nil {
		i.flow.token = nil
	}
}

func (i *invocation) finishFlow(source *Result, frames []FlowFrame) (*FlowResult, error) {
	defer i.releaseFlow()
	if i.flow.token == nil {
		return nil, invalid("missing initial flow token")
	}
	out := &FlowResult{source: source, group: i.flow.group, remainder: i.flow.token.Remainder()}
	for _, frame := range frames {
		record := FrameResult{Frame: frame}
		if !out.complete {
			record.Attempted = true
			width, _ := frameDU(frame.Width, false)
			fit, err := i.flow.token.Fit(paragraph.Frame{Width: width, Height: frame.Height * .75})
			if err != nil {
				return nil, err
			}
			record.Fragment, record.Status, record.Reused, record.Breaker = fit.Placed, fit.Status, fit.Reused, fit.Breaker
			span := fit.Placed.Range()
			for _, line := range fit.Overfull {
				if line >= span.Start && line < span.End {
					record.Overfull = append(record.Overfull, line)
				}
			}
			out.remainder = fit.Remainder
			out.complete = fit.Continuation == nil
			i.flow.token = fit.Continuation
			if out.complete {
				delete(i.prepared, i.flow.group)
			}
			i.trace(fmt.Sprintf("flow frame=%s reused=%t lines=%d status=%v suffix=%s overflow=%v", frame.ID, record.Reused, len(fit.Placed.Lines()), record.Status, record.Breaker, record.Overfull))
		}
		out.frames = append(out.frames, record)
	}

	// Exhaustion ends this bounded invocation too: only the frozen tail escapes.
	i.releaseFlow()
	return out, nil
}

func (r *FlowResult) placements() []PositionedFragment {
	out := make([]PositionedFragment, 0, len(r.frames))
	for _, f := range r.frames {
		if f.Fragment != nil {
			out = append(out, PositionedFragment{GroupID: r.group, Fragment: f.Fragment, FrameOrigin: render.Point{X: f.Frame.X * .75, Y: f.Frame.Y * .75}})
		}
	}
	return out
}
