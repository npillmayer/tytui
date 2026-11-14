package documentlayout

import (
	"context"
	"errors"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography/paragraph"
)

// Observe the real callback boundary without adding counters to production.
type observedFlow struct {
	*invocation
	measures, layouts map[box.BoxID]int
	widths            map[box.BoxID]box.IntrinsicWidths
}

func (o *observedFlow) MeasureInline(r box.InlineMeasureRequest) (box.IntrinsicWidths, error) {
	o.measures[r.Group.BoxID]++
	w, err := o.invocation.MeasureInline(r)
	if err == nil {
		o.widths[r.Group.BoxID] = w
	}
	return w, err
}
func (o *observedFlow) LayoutInline(r box.InlineRequest) (box.InlineResult, error) {
	o.layouts[r.Group.BoxID]++
	return o.invocation.LayoutInline(r)
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
			observer := &observedFlow{invocation: i, measures: map[box.BoxID]int{}, layouts: map[box.BoxID]int{}, widths: map[box.BoxID]box.IntrinsicWidths{}}
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
	o := &observedFlow{invocation: i, measures: map[box.BoxID]int{}, layouts: map[box.BoxID]int{}, widths: map[box.BoxID]box.IntrinsicWidths{}}
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
	if g.Content.W != 0 || g.Content.H != 20 {
		t.Fatal("empty auto sizing/strut", g)
	}
}
