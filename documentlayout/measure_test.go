package documentlayout

import (
	"errors"
	"math"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
)

type intrinsicView map[box.BoxID]box.IntrinsicAtomic

func (v intrinsicView) Lookup(id box.BoxID) (box.IntrinsicAtomic, bool) { a, ok := v[id]; return a, ok }

type edgesView map[box.BoxID]box.BoxEdges

func (v edgesView) Lookup(id box.BoxID) (box.BoxEdges, bool) { e, ok := v[id]; return e, ok }

func TestAtomicIntrinsicConversion(t *testing.T) {
	g := &InlineGroup{AtomicSources: []AtomicSource{{ID: 91, BoxID: 7}, {ID: 3, BoxID: 19}}}
	atoms := intrinsicView{19: {BoxID: 19, BorderWidths: box.IntrinsicWidths{MaxContent: 2}}, 7: {BoxID: 7, BorderWidths: box.IntrinsicWidths{MinContent: 1.0 / 32768, MaxContent: 1.0 / 32768}}}
	edges := edgesView{7: {Margin: box.Edges{Left: 1.0 / 32768, Right: 1.0 / 32768}, Padding: box.Edges{Left: 100}, Border: box.Edges{Right: 100}}, 19: {Margin: box.Edges{Left: -5, Right: 1}}}
	got, err := atomicIntrinsics(g, box.InlineMeasureRequest{Atomics: atoms, Edges: edges})
	want := []khipu.AtomicIntrinsic{{ID: 91, MinAdvance: 6, MaxAdvance: 6}, {ID: 3, MinAdvance: -4 * 49152, MaxAdvance: -2 * 49152}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("primitive rounding, signed margins or ID mapping", got, err)
	}
	// A wide intermediate sum is valid if the final signed advance is finite.
	for _, tc := range []struct {
		border, left, right float32
		valid               bool
	}{
		{30000, 30000, -30000, true}, {30000, 30000, 0, false}, {0, -30000, -30000, false},
	} {
		atoms[7] = box.IntrinsicAtomic{BoxID: 7, BorderWidths: box.IntrinsicWidths{MinContent: tc.border, MaxContent: tc.border}}
		edges[7] = box.BoxEdges{Margin: box.Edges{Left: tc.left, Right: tc.right}}
		got, err := atomicIntrinsics(g, box.InlineMeasureRequest{Atomics: atoms, Edges: edges})
		if tc.valid {
			if err != nil || got[0].MinAdvance != 30000*49152 {
				t.Fatal(got, err)
			}
		} else if got != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted overflowing advance", got, err)
		}
	}
}

func TestIntrinsicSummaryFiniteBounds(t *testing.T) {
	for _, w := range []khipu.IntrinsicWidths{{}, {MinContent: 1, MaxContent: 72 * dimen.BP}} {
		got, err := intrinsicSummary(w)
		if err != nil || got.MinContent != float32(float64(w.MinContent)/duPerCSSPixel) || got.MaxContent != float32(float64(w.MaxContent)/duPerCSSPixel) {
			t.Fatal(got, err)
		}
	}
	for _, w := range []khipu.IntrinsicWidths{{MinContent: -1}, {MinContent: 2, MaxContent: 1}, {MaxContent: dimen.Fil}, {MaxContent: dimen.Fil - 1}} {
		// Fil-1 is a valid DU but rounds into the reserved domain as float32 CSS.
		if got, err := intrinsicSummary(w); got != (box.IntrinsicWidths{}) || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal(got, err)
		}
	}
}

func TestMeasureInlineReuseAndRequestValidation(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root" style="font-size:20px">éa <span style="display:inline-block;width:20px">x</span> z</div>`)
	calls := countPreparation(t, i)
	entry := i.document.groups[i.document.order[0].GroupID]
	g, err := i.document.Extract(entry.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	id := g.AtomicSources[0].BoxID
	atoms := intrinsicView{id: {BoxID: id, BorderWidths: box.IntrinsicWidths{MinContent: 10, MaxContent: 20}}}
	edges := edgesView{id: {Margin: box.Edges{Left: -2, Right: 5}, Padding: box.Edges{Left: 100}, Border: box.Edges{Right: 100}}, entry.OwnerID: {Padding: box.Edges{Left: 1000}}}
	req := box.InlineMeasureRequest{Group: entry.node, OwnerID: entry.OwnerID, Atomics: atoms, Edges: edges}
	check := func(want box.IntrinsicWidths) {
		t.Helper()
		got, err := i.MeasureInline(req)
		if err != nil || got != want {
			t.Fatalf("got %v, %v; want %v", got, err, want)
		}
	}
	check(box.IntrinsicWidths{MinContent: 20, MaxContent: 73})
	before := *calls
	check(box.IntrinsicWidths{MinContent: 20, MaxContent: 73})
	atoms[id] = box.IntrinsicAtomic{BoxID: id, BorderWidths: box.IntrinsicWidths{MinContent: 30, MaxContent: 40}}
	check(box.IntrinsicWidths{MinContent: 33, MaxContent: 93})
	if *calls != before || calls.shape != 1 || len(i.started) != 0 || len(i.groups) != 0 {
		t.Fatal("measurement reshaped or published final output")
	}
	// Validate identity even after preparation has been cached.
	foreign := *entry.node
	for _, mutate := range []func(*box.InlineMeasureRequest){
		func(r *box.InlineMeasureRequest) { r.Group = nil }, func(r *box.InlineMeasureRequest) { r.Group = &foreign },
		func(r *box.InlineMeasureRequest) { r.OwnerID++ }, func(r *box.InlineMeasureRequest) { r.Edges = nil }, func(r *box.InlineMeasureRequest) { r.Atomics = nil },
		func(r *box.InlineMeasureRequest) { r.Edges = edgesView{} }, func(r *box.InlineMeasureRequest) { r.Atomics = intrinsicView{} },
		func(r *box.InlineMeasureRequest) { r.Atomics = intrinsicView{id: {BoxID: id + 1}} },
	} {
		bad := req
		mutate(&bad)
		if got, err := i.MeasureInline(bad); got != (box.IntrinsicWidths{}) || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted bad request", got, err)
		}
	}
	for _, w := range []box.IntrinsicWidths{
		{MinContent: -0.0000001}, {MinContent: 0.0000001}, // Invalid before DU rounding.
		{MinContent: float32(math.NaN())}, {MaxContent: float32(math.Inf(1))}, {MaxContent: float32(float64(dimen.Fil) / duPerCSSPixel)},
	} {
		atoms[id] = box.IntrinsicAtomic{BoxID: id, BorderWidths: w}
		if got, err := i.MeasureInline(req); got != (box.IntrinsicWidths{}) || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted invalid border bounds", got, err)
		}
	}
	atoms[id] = box.IntrinsicAtomic{BoxID: id, BorderWidths: box.IntrinsicWidths{MinContent: 30, MaxContent: 40}}
	for _, margin := range []float32{float32(math.NaN()), float32(math.Inf(-1)), float32(float64(dimen.Fil) / duPerCSSPixel)} {
		edges[id] = box.BoxEdges{Margin: box.Edges{Left: margin}}
		if got, err := i.MeasureInline(req); got != (box.IntrinsicWidths{}) || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted invalid margin", got, err)
		}
	}
	edges[id] = box.BoxEdges{}
	check(box.IntrinsicWidths{MinContent: 30, MaxContent: 90})
	out, err := i.run()
	if err != nil {
		t.Fatal("measurement prevented final layout", err)
	}
	retained, _ := out.Group(entry.GroupID)
	// Request tables may change or be discarded without changing final results.
	delete(atoms, id)
	delete(edges, id)
	after, _ := out.Group(entry.GroupID)
	if !reflect.DeepEqual(retained, after) {
		t.Fatal("retained result depends on measurement views")
	}
	if _, err := i.LayoutInline(box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, Used: usedView{}, Atomics: preparedView{}}); !errors.Is(err, khipu.ErrUnsupportedPolicy) {
		t.Fatal("measurement weakened final-callback guard", err)
	}
}
