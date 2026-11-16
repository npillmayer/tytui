package documentlayout_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/paragraph"

	"tytui/documentlayout"
	"tytui/fontselection"
)

func layoutConfig(t *testing.T) documentlayout.Config {
	t.Helper()
	r, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "synthetic", Size: 10 * dimen.BP}, testServices{
		Loader: syntheticLoader{}, Coverage: typography.CoverageFuncs{Check: checkSyntheticFace, Lookup: func(_ contracts.FontFace, c rune) (uint32, error) {
			if c == '\uFFFC' {
				t.Error("queried coverage of an atomic marker")
			}
			return 1, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return documentlayout.Config{Resolver: r, NewShaper: func() (contracts.StreamingShaper, error) { return helpers.NewSyntheticStreamingShaper(8) }}
}

func runDocument(t *testing.T, d *documentlayout.Document, width float32, breaker paragraph.Breaker, cfg documentlayout.Config) *documentlayout.Result {
	t.Helper()
	r, err := d.Layout(t.Context(), documentlayout.LayoutRequest{Width: width, Breaker: breaker}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Document() != d || len(r.Groups()) != len(d.Groups()) {
		t.Fatal("incomplete document snapshot")
	}
	return r
}

func TestDocumentFlowAtomicEdges(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="width:100px;font-size:20px;padding:7px;border-width:3px;border-style:solid"><span style="display:inline-block;width:30px;font-size:10px;padding:2px;border-width:1px;border-style:solid;margin:4px 5px 6px -2px">x</span></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 200, breaker, layoutConfig(t))
		geometry := r.Geometry()
		parent, ok := r.Group(d.Groups()[0].GroupID)
		if !ok || len(parent.Input.AtomicSources) != 1 {
			t.Fatal("missing atomic group")
		}
		atom := parent.Input.AtomicSources[0]
		a := geometry[atom.BoxID]
		// Child: 30px content + two 3px edges; height 10px text + 6px edges.
		// Parent: above = 4 + 16 + 6 = 26px, below = 4px owner strut.
		if a.Frame != (box.Rect{X: -2, Y: 4, W: 36, H: 16}) || a.RelativeTo != parent.Input.OwnerID {
			t.Fatalf("atomic edges/placement: %+v", a)
		}
		if a.Content != (box.Rect{X: 1, Y: 7, W: 30, H: 10}) {
			t.Fatalf("atomic content: %+v", a.Content)
		}
		root := geometry[d.Root().BoxID]
		if root.Frame != (box.Rect{W: 120, H: 50}) || root.Content != (box.Rect{X: 10, Y: 10, W: 100, H: 30}) {
			t.Fatalf("owner edges: %+v", root)
		}
		line := parent.Paragraph.VList.Lines()[0]
		if line.Baseline != 19.5 || line.Height != 22.5 || line.Horizontal.LeftSkip != 0 || line.Horizontal.RightSkip != 0 {
			t.Fatal("strut or side skips", line)
		}
		p := parent.Paragraph.VList.AtomicPlacements()[0]
		if p.ID != atom.ID || p.X != -1.5 || p.Y != 3 {
			t.Fatal("retained big-point coordinates", p)
		}
		for _, group := range r.Groups() {
			g, ok := r.Group(group.GroupID)
			if !ok || g.Input.OwnerID != group.OwnerID || g.Paragraph.Frozen.SourceLen() != uint64(len(g.Input.Text)) {
				t.Fatal("lost group source/owner", group)
			}
		}
	}
}

func TestDocumentAnonymousOwnerTransforms(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px;padding:7px;border-width:3px;border-style:solid"><div style="margin:13px 0 0 11px;padding:5px;border-width:2px;border-style:solid">before<div style="margin:0">middle</div><span style="display:inline-block;width:30px;font-size:10px;padding:2px;border-width:1px;border-style:solid;margin:4px 5px 6px -2px">x</span></div></div>`)
	r := runDocument(t, d, 300, paragraph.FirstFit, layoutConfig(t))
	geometry := r.Geometry()
	var found bool
	for _, group := range r.Groups() {
		g, _ := r.Group(group.GroupID)
		if len(g.Input.AtomicSources) == 0 {
			continue
		}
		found = true
		a := geometry[g.Input.AtomicSources[0].BoxID]
		owner := geometry[g.Input.OwnerID]
		if a.RelativeTo != g.Input.OwnerID || owner.Frame.Y != 40 || owner.Content != owner.Frame {
			t.Fatal("anonymous owner or preceding lines counted incorrectly", a, owner)
		}
		// Compose each parent's content origin once, independently of paragraphs.
		x, y := a.Frame.X, a.Frame.Y
		for id := a.RelativeTo; id != 0; id = geometry[id].RelativeTo {
			v := geometry[id]
			x += v.Content.X
			y += v.Content.Y
		}
		if x != 26 || y != 74 {
			t.Fatalf("absolute border origin = %v,%v, want 26,74", x, y)
		}
		if g.Input.Paragraph.Locator == r.Document().Groups()[0].Paragraph.Locator && g.Input.OwnerID == d.Root().BoxID {
			t.Fatal("anonymous owner replaced by source owner")
		}
	}
	if !found {
		t.Fatal("missing atomic group")
	}
}

func TestDocumentZeroWidthAndOverfullAtomics(t *testing.T) {
	for _, width := range []string{"0px", "30px"} {
		d, _, _ := build(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;width:`+width+`"></span><span style="display:inline-block;width:0px"></span></div>`)
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			r := runDocument(t, d, 0, breaker, layoutConfig(t))
			g, _ := r.Group(d.Groups()[0].GroupID)
			placements := g.Paragraph.VList.AtomicPlacements()
			if len(placements) != 2 {
				t.Fatal("zero-size item omitted", placements)
			}
			seen := map[contracts.AtomicID]bool{}
			for _, p := range placements {
				if seen[p.ID] {
					t.Fatal("duplicate placement", p.ID)
				}
				seen[p.ID] = true
			}
			for _, source := range g.Input.AtomicSources {
				a, ok := r.Geometry()[source.BoxID]
				if !ok || !seen[source.ID] || a.RelativeTo != g.Input.OwnerID || a.Frame.H != 20 {
					t.Fatal("atomic correspondence", source, a)
				}
			}
		}
	}
}

func TestNestedAtomicSchedulingAndUnquantizedCSSDimensions(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px">a <span style="display:inline-block;width:50.12345px"><span style="display:inline-block;width:10.12345px">éאב</span></span> z</div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 300, breaker, layoutConfig(t))
		geometry := r.Geometry()
		count := 0
		for _, group := range r.Groups() {
			g, _ := r.Group(group.GroupID)
			if len(g.Input.AtomicSources) == 0 {
				continue
			}
			count++
			atom := g.Input.AtomicSources[0]
			frame := geometry[atom.BoxID].Frame
			wantWidth := float32(10.12345)
			if g.Input.OwnerID == d.Root().BoxID {
				wantWidth = 50.12345
			}
			if frame.W != wantWidth || geometry[atom.BoxID].RelativeTo != g.Input.OwnerID {
				t.Fatal("prepared dimensions overwritten or wrong nested owner", frame, wantWidth)
			}
			placements := g.Paragraph.VList.AtomicPlacements()
			if len(placements) != 1 || placements[0].ID != atom.ID {
				t.Fatal("nested atomic source association lost")
			}
			// These accessor edits cannot alter retained anchors/source identity.
			g.Input.Anchors[0].ID = 999
			g.Input.AtomicSources[0].BoxID = 999
			after, _ := r.Group(group.GroupID)
			if after.Input.Anchors[0].ID != atom.ID || after.Input.AtomicSources[0].BoxID != atom.BoxID {
				t.Fatal("atomic source slices shared")
			}
		}
		if count != 2 {
			t.Fatal("missing nested callback", count)
		}
	}
}

func TestDocumentFailureAndCancellationAreTransactional(t *testing.T) {
	d, input, _ := build(t, `<div id="root"><div style="color:red">first text</div><div>second text</div></div>`)
	cfg := layoutConfig(t)
	old := runDocument(t, d, 200, paragraph.FirstFit, cfg)
	geometry := old.Geometry()
	var before []documentlayout.GroupResult
	for _, g := range old.Groups() {
		v, _ := old.Group(g.GroupID)
		before = append(before, v)
	}
	backendErr := errors.New("late backend failure")
	for _, cancelRun := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		bad := cfg
		bad.NewShaper = func() (contracts.StreamingShaper, error) {
			calls++
			// CSS completes the first block's paragraph before starting the next.
			if calls == 2 {
				if cancelRun {
					cancel()
				} else {
					return nil, backendErr
				}
			}
			return helpers.NewSyntheticStreamingShaper(8)
		}
		out, err := d.Layout(ctx, documentlayout.LayoutRequest{Width: 100, Breaker: paragraph.FirstFit}, bad)
		cancel()
		want := backendErr
		if cancelRun {
			want = context.Canceled
		}
		if calls != 2 || out != nil || !errors.Is(err, want) {
			t.Fatalf("late failure: calls=%d, result=%v, err=%v", calls, out, err)
		}
		if !reflect.DeepEqual(geometry, old.Geometry()) {
			t.Fatal("old geometry mutated")
		}
		for n, g := range old.Groups() {
			after, _ := old.Group(g.GroupID)
			if !reflect.DeepEqual(before[n], after) {
				t.Fatal("old group mutated", g.GroupID)
			}
		}
	}
	// Source DOM can be released, and detached outputs can be edited independently.
	input.Nodes = nil
	first := old.Groups()[0].GroupID
	g, _ := old.Group(first)
	g.Input.Text = "changed"
	g.Input.Styles[0].Style.Color = "blue"
	g.Input.Paint[0].Paint.Color = "blue"
	g.Input.TextSources[0].NodeStart = 123
	g.Input.LineLayout.Runs[0].Start = 123
	g.Paragraph.BreakPositions[0] = 123
	g.Paragraph.BidiRuns[0].From = 123
	g.Paragraph.Frozen = nil
	g.Paragraph.VList = nil
	delete(geometry, d.Root().BoxID)
	after, _ := old.Group(first)
	if !reflect.DeepEqual(before[0], after) || len(old.Geometry()) == len(geometry) {
		t.Fatal("result accessors leaked mutable storage")
	}
	if _, ok := old.Group(box.BoxID(math.MaxUint64)); ok {
		t.Fatal("unknown group found")
	}
	resized := runDocument(t, d, 60, paragraph.KnuthPlass, cfg)
	if resized.Geometry()[d.Root().BoxID].Content.W != 60 || old.Geometry()[d.Root().BoxID].Content.W != 200 {
		t.Fatal("resize changed earlier result")
	}
}

func TestDocumentLayoutInvalidInput(t *testing.T) {
	d, _, _ := build(t, `<div id="root">x</div>`)
	cfg := layoutConfig(t)
	for _, width := range []float32{-1, float32(math.NaN()), float32(math.Inf(1)), float32(float64(dimen.Fil) / 49152)} {
		out, err := d.Layout(t.Context(), documentlayout.LayoutRequest{Width: width, Breaker: paragraph.FirstFit}, cfg)
		if out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted invalid width", width, err)
		}
	}
	for _, bad := range []documentlayout.Config{{}, {Resolver: cfg.Resolver}, {NewShaper: cfg.NewShaper}} {
		out, err := d.Layout(t.Context(), documentlayout.LayoutRequest{Width: 100, Breaker: paragraph.FirstFit}, bad)
		if out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted missing services", err)
		}
	}
	out, err := d.Layout(nil, documentlayout.LayoutRequest{}, cfg)
	if out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("accepted nil context", err)
	}
	out, err = d.Layout(t.Context(), documentlayout.LayoutRequest{Width: 100}, cfg)
	if out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("accepted unknown breaker", err)
	}
}

func checkSyntheticFace(face contracts.FontFace) error {
	return new(helpers.SyntheticStreamingShaper).CheckFace(face)
}
