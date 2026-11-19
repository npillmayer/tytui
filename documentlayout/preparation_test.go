package documentlayout

import (
	"errors"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography/glyphing"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/fontselection"
)

type preparationCalls struct{ shape, coverage int }

func countPreparation(t *testing.T, i *invocation) *preparationCalls {
	t.Helper()
	calls := new(preparationCalls)
	r, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "synthetic", Size: 10 * dimen.BP}, testServices{
		Loader: boundaryLoader{}, Coverage: glyphing.CoverageFuncs{Check: checkSyntheticFace, Lookup: func(contracts.FontFace, rune) (uint32, error) {
			calls.coverage++
			return 1, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	i.config.Resolver = r
	i.config.NewShaper = func() (contracts.StreamingShaper, error) {
		calls.shape++
		return helpers.NewSyntheticStreamingShaper(8)
	}
	return calls
}

func TestInvocationPreparationMeasureThenLayout(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			i := boundaryInvocation(t, `<div id="root" style="font-size:20px;color:red">éa <span style="display:inline-block;width:20px">x</span> z</div>`)
			i.request.Breaker = breaker
			calls := countPreparation(t, i)
			entry := i.document.groups[i.document.order[0].GroupID]
			p, err := i.prepareGroup(entry.node, entry.OwnerID)
			if err != nil {
				t.Fatal(err)
			}
			if calls.shape != 1 || calls.coverage == 0 {
				t.Fatal("fixture did not prepare text", calls)
			}
			request := khipu.MeasureRequest{Atomics: []khipu.AtomicIntrinsic{{ID: p.input.Anchors[0].ID, MinAdvance: 10 * dimen.BP, MaxAdvance: 20 * dimen.BP}}}
			measure := func(want khipu.IntrinsicWidths) {
				t.Helper()
				before := *calls
				cached, err := i.prepareGroup(entry.node, entry.OwnerID)
				if err != nil || cached != p {
					t.Fatal("lost preparation identity", err)
				}
				got, err := cached.paragraph.MeasureIntrinsic(request)
				if err != nil || got != want {
					t.Fatalf("measure = %v, %v; want %v", got, err, want)
				}
				if *calls != before {
					t.Fatal("measurement repeated itemization or shaping", before, calls)
				}
			}
			measure(khipu.IntrinsicWidths{MinContent: 15 * dimen.BP, MaxContent: 115 * dimen.BP / 2})
			request.Atomics[0].MinAdvance, request.Atomics[0].MaxAdvance = 25*dimen.BP, 30*dimen.BP
			want := khipu.IntrinsicWidths{MinContent: 25 * dimen.BP, MaxContent: 135 * dimen.BP / 2}
			measure(want)
			if len(i.prepared) != 1 || len(i.started) != 0 || len(i.groups) != 0 {
				t.Fatal("measurement accepted a final callback")
			}
			out, err := i.run() // Child is layout-only and must prepare on demand.
			if err != nil {
				t.Fatal(err)
			}
			if calls.shape != len(i.document.groups) || len(i.prepared) != len(i.groups) || len(i.started) != len(i.groups) {
				t.Fatal("group prepared twice or not finalized", calls, i.started)
			}
			parent, ok := out.Group(entry.GroupID)
			if !ok || !reflect.DeepEqual(parent.Input, p.input) {
				t.Fatal("accepted input differs from measured input")
			}
			if parent.Paragraph.Frozen.SourceLen() != uint64(len(p.input.Text)) {
				t.Fatal("source length changed")
			}
			for index := 0; index < parent.Paragraph.Frozen.KnotCount(); index++ {
				if id, ok := parent.Paragraph.Frozen.Atomic(index); ok {
					knot, _ := parent.Paragraph.Frozen.Knot(index)
					if id != request.Atomics[0].ID || knot.Source.Bytes != (khipu.ByteRange{Start: 4, End: 7}) || knot.Source.ScalarStart != 3 {
						t.Fatal("atomic source coordinates changed", knot)
					}
				}
			}
			measure(want) // Accepted result and final-attempt guard do not block reuse.
			geometry := out.Geometry()
			before := calls.shape
			resized, err := i.document.Layout(LayoutRequest{Width: 60, Breaker: breaker}, i.config)
			if err != nil {
				t.Fatal(err)
			}
			if calls.shape-before != len(i.document.groups) {
				t.Fatal("preparation leaked across document invocations")
			}
			next, _ := resized.Group(entry.GroupID)
			if next.Paragraph.Frozen == parent.Paragraph.Frozen {
				t.Fatal("independent layout reused prepared result")
			}
			after, _ := out.Group(entry.GroupID)
			if !reflect.DeepEqual(parent, after) || !reflect.DeepEqual(geometry, out.Geometry()) {
				t.Fatal("later measurement/layout changed published snapshot")
			}
			// Published input envelopes cannot mutate the private preparation.
			after.Input.Paint[0].Paint.Color = "blue"
			after.Input.Anchors[0].ID = 999
			if !reflect.DeepEqual(parent.Input, p.input) {
				t.Fatal("published accessors alias preparation")
			}
			_, err = i.LayoutInline(box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, Used: usedView{}, Atomics: preparedView{}})
			if !errors.Is(err, khipu.ErrUnsupportedPolicy) {
				t.Fatal("repeated final callback accepted", err)
			}
		})
	}
}

func TestPreparationRejectsForeignIdentityOnMissAndHit(t *testing.T) {
	i := boundaryInvocation(t)
	calls := countPreparation(t, i)
	entry := i.document.groups[i.document.order[0].GroupID]
	foreign := *entry.node // Identical numeric IDs from a different tree are not sufficient.
	unknown := foreign
	unknown.BoxID = ^box.BoxID(0)
	for _, cached := range []bool{false, true} {
		if cached {
			if _, err := i.prepareGroup(entry.node, entry.OwnerID); err != nil {
				t.Fatal(err)
			}
		}
		before := *calls
		for _, req := range []struct {
			group *box.LayoutNode
			owner box.BoxID
		}{
			{nil, entry.OwnerID}, {&unknown, entry.OwnerID}, {&foreign, entry.OwnerID}, {entry.node, entry.OwnerID + 1},
		} {
			p, err := i.prepareGroup(req.group, req.owner)
			if p != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
				t.Fatal("accepted foreign preparation request", p, err)
			}
			if _, err := i.LayoutInline(box.InlineRequest{Group: req.group, OwnerID: req.owner, Used: usedView{}, Atomics: preparedView{}}); !errors.Is(err, contracts.ErrIllegalArguments) {
				t.Fatal("accepted foreign final request", err)
			}
		}
		if *calls != before || len(i.started) != 0 || len(i.groups) != 0 {
			t.Fatal("identity rejection changed invocation")
		}
	}
}

func TestPreparationFailureAndRetryPreserveCompletedGroups(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root"><div>first text</div><div>second text</div></div>`)
	first := i.document.groups[i.document.order[0].GroupID]
	second := i.document.groups[i.document.order[1].GroupID]
	calls := 0
	sentinel := errors.New("second preparation failed")
	i.config.NewShaper = func() (contracts.StreamingShaper, error) {
		calls++
		if calls == 2 {
			return nil, sentinel
		}
		return helpers.NewSyntheticStreamingShaper(8)
	}
	p, err := i.prepareGroup(first.node, first.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if bad, err := i.prepareGroup(second.node, second.OwnerID); bad != nil || !errors.Is(err, sentinel) {
		t.Fatal(bad, err)
	}
	if len(i.prepared) != 1 || i.prepared[first.GroupID] != p || len(i.groups) != 0 || len(i.started) != 0 {
		t.Fatal("partial preparation retained or earlier preparation lost")
	}
	if _, err := p.paragraph.MeasureIntrinsic(khipu.MeasureRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.prepareGroup(second.node, second.OwnerID); err != nil {
		t.Fatal("retry failed", err)
	}
	out, err := i.run()
	if err != nil || out == nil || calls != 3 {
		t.Fatal("failed preparation poisoned final layout", calls, err)
	}
}

func TestPreparationFailureDoesNotPublish(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root"><div>first text</div><div>second text</div></div>`)
	old, err := i.document.Layout(i.request, i.config)
	if err != nil {
		t.Fatal(err)
	}
	geometry := old.Geometry()
	first := i.document.groups[i.document.order[0].GroupID]
	second := i.document.groups[i.document.order[1].GroupID]
	retained, _ := old.Group(first.GroupID)
	failure := errors.New("preparation failed")
	calls := 0
	i.config.NewShaper = func() (contracts.StreamingShaper, error) {
		calls++
		if calls == 2 {
			return nil, failure
		}
		return helpers.NewSyntheticStreamingShaper(8)
	}
	p, err := i.prepareGroup(first.node, first.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if bad, err := i.prepareGroup(second.node, second.OwnerID); bad != nil || !errors.Is(err, failure) {
		t.Fatal(bad, err)
	}
	if len(i.prepared) != 1 || i.prepared[first.GroupID] != p || len(i.groups) != 0 {
		t.Fatal("failure retained partial preparation")
	}
	if _, err := i.document.Layout(i.request, i.config); err != nil {
		t.Fatal("fresh invocation failed", err)
	}
	after, _ := old.Group(first.GroupID)
	if !reflect.DeepEqual(geometry, old.Geometry()) || !reflect.DeepEqual(retained, after) {
		t.Fatal("failed invocation changed older snapshot")
	}
}

func TestFailedFinalCallbackKeepsPreparationButRejectsRepeat(t *testing.T) {
	i := boundaryInvocation(t)
	entry := i.document.groups[i.document.order[0].GroupID]
	request := box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, ContentWidth: 100, Used: usedView{}, Atomics: preparedView{}}
	if _, err := i.LayoutInline(request); !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("missing atomic dimensions accepted", err)
	}
	p := i.prepared[entry.GroupID]
	if p == nil || !i.started[entry.GroupID] || len(i.groups) != 0 {
		t.Fatal("preparation and acceptance were conflated")
	}
	id := p.input.AtomicSources[0].BoxID
	request.Used = usedView{id: {}}
	request.Atomics = preparedView{id: {BoxID: id, BorderWidth: 10, BorderHeight: 10}}
	if _, err := i.LayoutInline(request); !errors.Is(err, khipu.ErrUnsupportedPolicy) {
		t.Fatal("repeated failed final callback accepted", err)
	}
	cached, err := i.prepareGroup(entry.node, entry.OwnerID)
	if err != nil || cached != p {
		t.Fatal("final failure invalidated preparation", err)
	}
	if _, err := p.paragraph.MeasureIntrinsic(khipu.MeasureRequest{Atomics: []khipu.AtomicIntrinsic{{ID: p.input.Anchors[0].ID}}}); err != nil {
		t.Fatal(err)
	}
}
