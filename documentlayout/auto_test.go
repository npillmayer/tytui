package documentlayout

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync/atomic"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography/paragraph"
)

// flowEvent retains values only, never a CSS request, lookup view or tree node.
// The event is appended at callback entry and completed after real delegation.
type flowEvent struct {
	phase            string
	group, owner     box.BoxID
	width            float32
	bounds           box.IntrinsicWidths
	intrinsic        map[box.BoxID]box.IntrinsicAtomic
	atomics          map[box.BoxID]box.PreparedAtomic
	edges            map[box.BoxID]box.BoxEdges
	used             box.UsedValuesTable
	shapes           int
	runs             int32
	reused, complete bool
}

// Observe the real callback boundary without adding counters to production.
type observedFlow struct {
	*invocation
	t                   *testing.T
	calls               *preparationCalls
	runs                *atomic.Int32
	measures, layouts   map[box.BoxID]int
	widths              map[box.BoxID]box.IntrinsicWidths
	events              []flowEvent
	measuredPreparation map[box.BoxID]*preparedGroup // private identity witnesses, not request views
}

func newObservedFlow(t *testing.T, i *invocation, calls *preparationCalls) *observedFlow {
	o := &observedFlow{invocation: i, t: t, calls: calls, runs: new(atomic.Int32), measures: map[box.BoxID]int{}, layouts: map[box.BoxID]int{}, widths: map[box.BoxID]box.IntrinsicWidths{}, measuredPreparation: map[box.BoxID]*preparedGroup{}}
	factory := i.config.NewShaper
	i.config.NewShaper = func() (contracts.StreamingShaper, error) {
		s, err := factory()
		if err != nil {
			return nil, err
		}
		return observedShapeRuns{StreamingShaper: s, runs: o.runs}, nil
	}
	return o
}

// A preparation creates a stream even for empty/object-only input. Count
// actual Shape calls separately, across the producer goroutine's run boundaries.
type observedShapeRuns struct {
	contracts.StreamingShaper
	runs *atomic.Int32
}

func (s observedShapeRuns) Shape(ctx context.Context, p contracts.ShapeParams, r contracts.RuneReader, opts contracts.ShapeBufferOptions) error {
	s.runs.Add(1)
	return s.StreamingShaper.Shape(ctx, p, r, opts)
}

// Only the current group's atomic leaves belong in its request; interiors have
// independent callbacks. This test walk does not extract, prepare or size them.
func observedAtomicLeaves(group *box.LayoutNode) []*box.LayoutNode {
	var atoms []*box.LayoutNode
	var walk func(*box.LayoutNode)
	walk = func(n *box.LayoutNode) {
		if n.Box == box.BoxInlineBlock {
			atoms = append(atoms, n)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(group)
	return atoms
}

func (o *observedFlow) checkIdentity(group *box.LayoutNode, owner box.BoxID) {
	o.t.Helper()
	entry, ok := o.document.groups[group.BoxID]
	if !ok || entry.node != group || entry.OwnerID != owner || group.Box != box.BoxAnonymousInline {
		o.t.Fatal("callback has wrong group/owner", group.BoxID, owner)
	}
}

func (o *observedFlow) MeasureInline(r box.InlineMeasureRequest) (box.IntrinsicWidths, error) {
	o.checkIdentity(r.Group, r.OwnerID)
	if len(o.layouts) != 0 || len(o.started) != 0 || len(o.groups) != 0 {
		o.t.Fatal("measurement after final layout began")
	}
	started := maps.Clone(o.started)
	before := o.prepared[r.Group.BoxID]
	shapes, runs := o.calls.shape, o.runs.Load()
	e := flowEvent{phase: "measure", group: r.Group.BoxID, owner: r.OwnerID, intrinsic: map[box.BoxID]box.IntrinsicAtomic{}, edges: map[box.BoxID]box.BoxEdges{}}
	for _, n := range observedAtomicLeaves(r.Group) {
		a, ok := r.Atomics.Lookup(n.BoxID)
		if !ok || a.BoxID != n.BoxID {
			o.t.Fatal("missing intrinsic leaf", n.BoxID)
		}
		edge, ok := r.Edges.Lookup(n.BoxID)
		if !ok {
			o.t.Fatal("missing intrinsic edges", n.BoxID)
		}
		e.intrinsic[n.BoxID], e.edges[n.BoxID] = a, edge
	}
	edge, ok := r.Edges.Lookup(r.OwnerID)
	if !ok {
		o.t.Fatal("missing owner measurement edges")
	}
	e.edges[r.OwnerID] = edge
	index := len(o.events)
	o.events = append(o.events, e)
	o.measures[r.Group.BoxID]++
	w, err := o.invocation.MeasureInline(r)
	if !maps.Equal(started, o.started) || len(o.groups) != 0 {
		o.t.Fatal("measurement accepted a paragraph or final attempt")
	}
	e.bounds, e.shapes, e.complete = w, o.calls.shape-shapes, err == nil
	e.runs = o.runs.Load() - runs
	e.reused = before != nil && o.prepared[r.Group.BoxID] == before
	if err == nil {
		o.widths[r.Group.BoxID] = w
		o.measuredPreparation[r.Group.BoxID] = o.prepared[r.Group.BoxID]
	}
	o.events[index] = e
	return w, err
}

func (o *observedFlow) LayoutInline(r box.InlineRequest) (box.InlineResult, error) {
	o.checkIdentity(r.Group, r.OwnerID)
	e := flowEvent{phase: "layout", group: r.Group.BoxID, owner: r.OwnerID, width: r.ContentWidth, atomics: map[box.BoxID]box.PreparedAtomic{}, used: box.UsedValuesTable{}}
	// All descendants, including those outside this immediate group, must have
	// final used values before the first final callback can observe the table.
	var resolved func(*box.LayoutNode)
	resolved = func(n *box.LayoutNode) {
		u, ok := r.Used.Lookup(n.BoxID)
		if !ok || u.WidthPending || u.EdgesPending {
			o.t.Fatal("provisional/missing used values at final layout", n.BoxID, u)
		}
		e.used[n.BoxID] = u
		for _, c := range n.Children {
			resolved(c)
		}
	}
	resolved(o.document.root)
	if e.used[r.OwnerID].ContentWidth != r.ContentWidth {
		o.t.Fatal("wrong final containing width")
	}
	for _, n := range observedAtomicLeaves(r.Group) {
		a, ok := r.Atomics.Lookup(n.BoxID)
		if !ok || a.BoxID != n.BoxID {
			o.t.Fatal("missing prepared leaf", n.BoxID)
		}
		e.atomics[n.BoxID] = a
		var completed func(*box.LayoutNode)
		completed = func(child *box.LayoutNode) {
			if child.Box == box.BoxAnonymousInline {
				if _, ok := o.groups[child.BoxID]; !ok {
					o.t.Fatal("parent called before atomic interior completed", r.Group.BoxID, child.BoxID)
				}
			}
			for _, c := range child.Children {
				completed(c)
			}
		}
		completed(n)
	}
	before := o.prepared[r.Group.BoxID]
	if measured, ok := o.measuredPreparation[r.Group.BoxID]; ok && (measured == nil || before != measured) {
		o.t.Fatal("measured preparation replaced before final layout", r.Group.BoxID)
	}
	shapes, runs := o.calls.shape, o.runs.Load()
	index := len(o.events)
	o.events = append(o.events, e)
	o.layouts[r.Group.BoxID]++
	result, err := o.invocation.LayoutInline(r)
	e.shapes, e.complete = o.calls.shape-shapes, err == nil
	e.runs = o.runs.Load() - runs
	e.reused = before != nil && o.prepared[r.Group.BoxID] == before
	o.events[index] = e
	return result, err
}

func TestAutoAtomicShrinkToFitAndPreparationReuse(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		for _, tc := range []struct {
			available, content float32
			lines              int
		}{{20, 40, 2}, {59, 50, 2}, {100, 70, 1}} {
			i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;width:auto;padding:2px;border-width:1px;border-style:solid;margin-left:-2px;margin-right:5px">aa bbbb</span></div>`)
			i.request = LayoutRequest{Width: tc.available, Breaker: breaker}
			calls := countPreparation(t, i)
			observer := newObservedFlow(t, i, calls)
			containing := box.Rect{W: tc.available}
			used, err := box.ResolveUsedValues(i.document.root, box.ResolveContext{ContainingBlock: containing})
			if err != nil {
				t.Fatal(err)
			}
			flow, err := box.FlowLayout(i.document.root, used, observer, observer, box.LayoutContext{ContainingBlock: containing}, box.LayoutOptions{})
			if err != nil {
				t.Fatal(breaker, tc, err)
			}
			child := i.document.order[1]
			if len(observer.measures) != 1 || observer.measures[child.GroupID] != 1 || observer.widths[child.GroupID] != (box.IntrinsicWidths{MinContent: 40, MaxContent: 70}) {
				t.Fatal("wrong intrinsic bounds/schedule", observer.measures, observer.widths)
			}
			for _, g := range i.document.order {
				if observer.layouts[g.GroupID] != 1 {
					t.Fatal("not exactly one final callback", observer.layouts)
				}
			}
			if calls.shape != len(i.document.groups) || len(i.prepared) != len(i.groups) {
				t.Fatal("measured text reshaped for layout", calls)
			}
			geometry := flow.Geometry[child.OwnerID]
			frozen := i.groups[child.GroupID].Paragraph.Frozen
			if geometry.Content.W != tc.content || geometry.Frame.W != tc.content+6 || geometry.Frame.X != -2 || geometry.Frame.H != float32(frozen.LineCount()*20+6) {
				t.Fatal("shrink-to-fit/edges", breaker, tc, geometry)
			}
			// Intrinsic bounds do not prescribe breaker scoring or emergency fit.
			// Check the independently selected CSS target on every frozen line.
			for line := 0; line < frozen.LineCount(); line++ {
				v, _ := frozen.Line(line)
				if v.TargetWidth != dimen.DU(tc.content*49152) {
					t.Fatal("final binding ignored selected width", v)
				}
			}
			if breaker == paragraph.FirstFit && frozen.LineCount() != tc.lines {
				t.Fatal("first-fit wrapping differs from the hand-calculated fixture")
			}
			// Exercise public publication separately with the same constraints.
			out, err := i.document.Layout(t.Context(), i.request, i.config)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(flow.Geometry, out.Geometry()) || calls.shape != 2*len(i.document.groups) {
				t.Fatal("public path differs or reused cross-invocation preparation")
			}
			g, _ := out.Group(child.GroupID)
			if g.Input.Text != "aa bbbb" || g.Paragraph.Frozen.SourceLen() != 7 {
				t.Fatal("lost auto-width source correspondence")
			}
		}
	}
}

func TestFixedFlowNeedsNoIntrinsicCallbacks(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root">a<span style="display:inline-block;width:20px">bc</span>z</div>`)
	calls := countPreparation(t, i)
	o := newObservedFlow(t, i, calls)
	containing := box.Rect{W: 100}
	used, err := box.ResolveUsedValues(i.document.root, box.ResolveContext{ContainingBlock: containing})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := box.FlowLayout(i.document.root, used, o, o, box.LayoutContext{ContainingBlock: containing}, box.LayoutOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(o.measures) != 0 || calls.shape != len(i.document.groups) {
		t.Fatal("fixed-width path acquired measurement work", o.measures, calls)
	}
}

func TestAutoMeasurementFailureIsTransactional(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block">first text</span><span style="display:inline-block;width:auto">second text</span></div>`)
		old, err := i.document.Layout(t.Context(), i.request, i.config)
		if err != nil {
			t.Fatal(err)
		}
		geometry := old.Geometry()
		var retained []GroupResult
		for _, entry := range old.Groups() {
			g, _ := old.Group(entry.GroupID)
			retained = append(retained, g)
		}
		ctx, cancel := context.WithCancel(t.Context())
		i.ctx = ctx
		calls := 0
		sentinel := errors.New("late measurement backend failure")
		i.config.NewShaper = func() (contracts.StreamingShaper, error) {
			calls++
			if len(i.started) != 0 || len(i.groups) != 0 {
				t.Error("final layout began before measurement completed")
			}
			if calls == 2 {
				if cancelRun {
					cancel()
				} else {
					return nil, sentinel
				}
			}
			return helpers.NewSyntheticStreamingShaper(8)
		}
		out, err := i.run()
		cancel()
		want := sentinel
		if cancelRun {
			want = context.Canceled
		}
		if out != nil || !errors.Is(err, want) || calls != 2 || len(i.prepared) != 1 || len(i.groups) != 0 || len(i.started) != 0 {
			t.Fatal("failed measurement published output or lost earlier preparation", out, err, calls)
		}
		// Retry in a new invocation with stable working services.
		i.config.NewShaper = func() (contracts.StreamingShaper, error) { return helpers.NewSyntheticStreamingShaper(8) }
		if _, err := i.document.Layout(t.Context(), i.request, i.config); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(geometry, old.Geometry()) {
			t.Fatal("old geometry changed")
		}
		for n, entry := range old.Groups() {
			g, _ := old.Group(entry.GroupID)
			if !reflect.DeepEqual(retained[n], g) {
				t.Fatal("old paragraph/source/paint changed")
			}
		}
	}
}

func TestEmptyAutoAtomic(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block"></span></div>`)
	out, err := i.run()
	if err != nil {
		t.Fatal(err)
	}
	child := i.document.order[1]
	g := out.Geometry()[child.OwnerID]
	if g.Content.W != 0 || g.Content.H != 0 {
		t.Fatal("empty auto sizing", g)
	}
}
