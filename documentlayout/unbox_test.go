package documentlayout

import (
	"errors"
	"maps"
	"math"
	"reflect"
	"sync"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/layout"
)

func TestUnboxRetainsOriginalMarginsAndProof(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><p style="margin:0;margin-bottom:12px">A</p><p style="margin:0;margin-top:8px;margin-bottom:-3px">B</p><p style="margin:0"></p></div>`)
			i.request.Breaker = breaker
			calls := countPreparation(t, i)
			r, err := i.run()
			if err != nil {
				t.Fatal(err)
			}
			before, proof := lifetimeSnapshot(t, r), geometryProof(t, r)
			beforeCalls := *calls
			groups := r.Groups()
			if len(groups) != 3 {
				t.Fatal("fixture needs three paragraphs", groups)
			}
			a, b := r.Geometry()[groups[0].OwnerID], r.Geometry()[groups[1].OwnerID]
			if b.Frame.Y != a.Frame.Y+a.Frame.H+12 {
				t.Fatal("fixture did not collapse 12/8 margins", a, b)
			}
			for n, entry := range groups {
				want := SurroundingMargins{OwnerID: entry.OwnerID}
				switch n {
				case 0:
					want.Resolved.Bottom = 12
				case 1:
					want.Resolved.Top, want.Resolved.Bottom = 8, -3
				}
				m, ok := r.SurroundingMargins(entry.GroupID)
				if !ok || m != want {
					t.Fatal("lost original margins", m, want)
				}
				out, err := r.Unbox(entry.GroupID)
				if err != nil || out.Source != r || out.Margins == nil || *out.Margins != want {
					t.Fatal(out, err)
				}
				g, _ := r.Group(entry.GroupID)
				if out.List.Len() != len(g.Paragraph.VList.Lines()) || (n == 2 && out.List.Len() != 0) {
					t.Fatal("line count or empty-output mismatch")
				}
				for j, line := range g.Paragraph.VList.Lines() {
					item, _ := out.List.Item(j)
					f, h := item.Horizontal()
					if f != g.Paragraph.Frozen || h != line.Horizontal || item.Box().Extent != line.Height || item.Box().Baseline != line.Baseline-line.Top {
						t.Fatal("changed line content/geometry")
					}
				}
				m.Resolved.Top = 999
				out.Margins.OwnerID, out.Margins.Resolved.Top = 0, 777
				out.List, out.Source = nil, nil
				again, err := r.Unbox(entry.GroupID)
				if err != nil || *again.Margins != want || again.Source != r {
					t.Fatal("retained mutable envelope", again, err)
				}
			}
			if *calls != beforeCalls {
				t.Fatal("unboxing called typography services", beforeCalls, *calls)
			}
			if after := lifetimeSnapshot(t, r); after != before {
				t.Fatal("unboxing changed the source result")
			}
			if !reflect.DeepEqual(geometryProof(t, r), proof) {
				t.Fatal("unboxing changed retained proof")
			}
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 20 {
						out, err := r.Unbox(groups[0].GroupID)
						if err != nil || out.Margins.Resolved.Bottom != 12 {
							t.Error("concurrent recovery changed metadata", err)
							return
						}
						out.Margins.Resolved.Bottom = 0
					}
				})
			}
			wg.Wait()
		})
	}
}

func TestUnboxMarginsUseFinalAutoResolution(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;width:auto;padding:4px"><div style="width:20px;margin:0 auto">é</div><div style="width:60px;margin:0 auto">x</div></span></div>`)
		i.request.Breaker = breaker
		entry := i.document.order[1]
		initial, err := box.ResolveUsedValues(i.document.root, box.ResolveContext{ContainingBlock: box.Rect{W: 100}})
		if err != nil || !initial[entry.OwnerID].EdgesPending {
			t.Fatal("fixture has no provisional owner edges", err)
		}
		r, err := i.run()
		if err != nil {
			t.Fatal(err)
		}
		out, err := r.Unbox(entry.GroupID)
		if err != nil || out.Margins == nil || out.Margins.Resolved != (box.Edges{Left: 20, Right: 20}) {
			t.Fatal("retained provisional auto margins", out, err)
		}
		if !initial[entry.OwnerID].EdgesPending {
			t.Fatal("mutated CSS input table")
		}

		resize := boundaryInvocation(t, `<div id="root" style="font-size:20px"><div style="width:40px;margin:0 auto">é</div></div>`)
		var old *Result
		for _, width := range []float32{100, 80} {
			r, err := resize.document.Layout(t.Context(), LayoutRequest{Width: width, Breaker: breaker}, resize.config)
			if err != nil {
				t.Fatal(err)
			}
			m, ok := r.SurroundingMargins(r.Groups()[0].GroupID)
			if !ok || m.Resolved != (box.Edges{Left: (width - 40) / 2, Right: (width - 40) / 2}) {
				t.Fatal("wrong resolution context", m)
			}
			if old == nil {
				old = r
			}
		}
		m, _ := old.SurroundingMargins(old.Groups()[0].GroupID)
		if m.Resolved.Left != 30 || m.Resolved.Right != 30 {
			t.Fatal("resize mutated original margins", m)
		}
	}
}

func TestUnboxMarginsNativeUnitsAndMissingMetadata(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root" style="font-size:20px;margin:0.000030517578125px 2.25px -3.125px -4.5px">é</div>`)
	r, err := i.run()
	if err != nil {
		t.Fatal(err)
	}
	id := r.Groups()[0].GroupID
	out, err := r.Unbox(id)
	if err != nil || out.Margins.Resolved != (box.Edges{Top: 1.0 / 32768, Right: 2.25, Bottom: -3.125, Left: -4.5}) {
		t.Fatal("rounded or changed CSS margins", out, err)
	}
	copy := *r
	copy.groups = maps.Clone(r.groups)
	g := copy.groups[id]
	g.hasMargins = false
	copy.groups[id] = g
	if m, ok := copy.SurroundingMargins(id); ok || m != (SurroundingMargins{}) {
		t.Fatal("fabricated margins", m)
	}
	out, err = copy.Unbox(id)
	if err != nil || out.Margins != nil || out.List.Len() == 0 {
		t.Fatal("missing metadata became zero margins", out, err)
	}
	g.Paragraph.VList = new(layout.VList)
	copy.groups[id] = g
	if out, err := copy.Unbox(id); out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("partial result or lost validation cause", out, err)
	}
	for _, bad := range []*Result{nil, new(Result)} {
		if out, err := bad.Unbox(id); out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal(out, err)
		}
		if _, ok := bad.SurroundingMargins(id); ok {
			t.Fatal("nil/unknown margins")
		}
	}
	if out, err := r.Unbox(0); out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal(out, err)
	}
}

func TestOwnerMarginValidationPublishesNoGroup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   box.UsedValues
		missing bool
	}{
		{"missing", box.UsedValues{}, true},
		{"pending width", box.UsedValues{WidthPending: true}, false},
		{"pending edges", box.UsedValues{EdgesPending: true}, false},
		{"nan", box.UsedValues{Margin: box.Edges{Top: float32(math.NaN())}}, false},
		{"infinite", box.UsedValues{Margin: box.Edges{Right: float32(math.Inf(1))}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := boundaryInvocation(t, `<div id="root">x</div>`)
			entry := i.document.groups[i.document.order[0].GroupID]
			used := usedView{}
			if !tc.missing {
				used[entry.OwnerID] = tc.value
			}
			out, err := i.LayoutInline(box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, ContentWidth: 100, Used: used, Atomics: preparedView{}})
			if !errors.Is(err, contracts.ErrIllegalArguments) || len(i.groups) != 0 || out.Extent != (box.InlineExtent{}) {
				t.Fatal("published invalid margin snapshot", out, err)
			}
		})
	}
}
