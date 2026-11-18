package documentlayout_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/layout"
	"github.com/npillmayer/typography/render"
	renderot "github.com/npillmayer/typography/render/adapter/opentype"
	"tytui/documentlayout"
)

func lineFragment(t *testing.T, v *layout.VList, start, end int) *layout.Fragment {
	t.Helper()
	f, err := paragraph.Fragment(t.Context(), v)
	if err != nil {
		t.Fatal(err)
	}
	if end < len(f.Lines()) {
		f, _, err = paragraph.SplitFragment(t.Context(), f, end)
		if err != nil {
			t.Fatal(err)
		}
	}
	if start > 0 {
		_, f, err = paragraph.SplitFragment(t.Context(), f, start)
		if err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestPositionedFragmentsRealEqualWidthProof(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		d, _, _ := build(t, `<div id="root" dir="`+dir+`" style="font-size:20px;padding:7px;border-width:3px;border-style:solid;color:navy">office office <span style="color:red">office office</span> אב אב office office אב אב office office</div>`)
		r := runDocument(t, d, 130, paragraph.FirstFit, realConfig(t))
		services := render.Services{Backend: renderot.New(), Identity: renderot.New()}
		full, err := documentlayout.BuildProof(t.Context(), r, services)
		if err != nil {
			t.Fatal(err)
		}
		group, _ := r.Group(d.Groups()[0].GroupID)
		lines := group.Paragraph.VList.Lines()
		if len(lines) < 2 {
			t.Fatal("fixture must wrap")
		}
		cut := len(lines) / 2
		placements := []documentlayout.PositionedFragment{
			{GroupID: group.Input.GroupID, Fragment: lineFragment(t, group.Paragraph.VList, 0, cut), FrameOrigin: render.Point{X: 100, Y: 200}, Offset: render.Point{X: 3, Y: 5}},
			{GroupID: group.Input.GroupID, Fragment: lineFragment(t, group.Paragraph.VList, cut, len(lines)), FrameOrigin: render.Point{X: 400, Y: 700}, Offset: render.Point{X: 7, Y: 11}},
		}
		proof, err := documentlayout.BuildFragmentProof(t.Context(), r, placements, services)
		if err != nil {
			t.Fatal(err)
		}
		if len(proof.Glyphs) != len(full.Glyphs) || len(proof.Lines) != len(full.Lines) || len(proof.Boxes) != 0 {
			t.Fatal("repeated owner/prefix", len(proof.Glyphs), len(full.Glyphs))
		}
		owner := r.Geometry()[group.Input.OwnerID].Content
		for i, want := range full.Glyphs {
			n := 0
			if want.Line >= cut {
				n = 1
			}
			p := placements[n]
			start := p.Fragment.Range().Start
			want.X += p.FrameOrigin.X + p.Offset.X - float64(owner.X)*.75
			want.Y += p.FrameOrigin.Y + p.Offset.Y - float64(owner.Y)*.75 - lines[start].Top
			want.Line -= start
			got := proof.Glyphs[i]
			if math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 {
				t.Fatal("frame translation", dir, got, want)
			}
			got.X, got.Y = want.X, want.Y
			if !reflect.DeepEqual(got, want) {
				t.Fatal("glyph/paint/source changed", got, want)
			}
		}
		colors := map[string]bool{}
		for _, g := range proof.Glyphs {
			colors[g.Color] = true
		}
		if !colors["red"] || !colors["navy"] {
			t.Fatal("missing paint fixture")
		}
		// The existing real-font fixture shapes office with ligatures. Fewer
		// glyphs than non-space runes establishes that we exercised them here.
		letters := 0
		for _, r := range group.Input.Text {
			if r != ' ' {
				letters++
			}
		}
		if len(proof.Glyphs) >= letters {
			t.Fatal("ligature fixture did not ligate")
		}
		var out bytes.Buffer
		if err := documentlayout.WriteFragmentSVG(t.Context(), &out, r, placements, services); err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(&out)
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		proof.Glyphs[0].Sources[0].NodeStart = 999
		again, err := documentlayout.BuildFragmentProof(t.Context(), r, placements, services)
		if err != nil || again.Glyphs[0].Sources[0].NodeStart == 999 {
			t.Fatal("proof source alias", err)
		}
	}
}

func TestFragmentProofNestedOpaqueAndNegativeAdvance(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px;padding:10px">aa aa aa aa aa aa <span style="display:inline-block;width:30px;padding:2px;border-width:1px;border-style:solid;margin:4px 5px 6px -45px"><span style="display:inline-block;width:10px;padding:1px;border-width:1px;border-style:solid">x</span><img data-tytui-leaf="rectangle" style="width:4px;height:6px;display:inline-block"></span> bb bb bb bb</div>`)
	r := runDocument(t, d, 80, paragraph.FirstFit, layoutConfig(t))
	group, _ := r.Group(d.Groups()[0].GroupID)
	atoms := group.Paragraph.VList.AtomicPlacements()
	if len(atoms) != 1 {
		t.Fatal("outer atomic fixture", atoms)
	}
	selected := atoms[0].Line
	f := lineFragment(t, group.Paragraph.VList, selected, selected+1)
	// Preserve a precise fractional origin that CSS's float32 summary cannot
	// represent. Publication accepts authoritative retained atomic geometry.
	a := f.AtomicPlacements()
	a[0].X += 1.0 / 65536
	var err error
	f, err = layout.NewFragment(t.Context(), f.Frozen(), f.Range(), f.Box(), f.Lines(), f.Materials(), a)
	if err != nil {
		t.Fatal(err)
	}
	p := documentlayout.PositionedFragment{GroupID: group.Input.GroupID, Fragment: f, FrameOrigin: render.Point{X: 100, Y: 200}, Offset: render.Point{X: 3, Y: 5}}
	proof, err := documentlayout.BuildFragmentProof(t.Context(), r, []documentlayout.PositionedFragment{p}, cells())
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.Atomics) != 3 || len(proof.Leaves) != 1 {
		t.Fatal("missing/repeated opaque interiors", proof.Atomics, proof.Leaves)
	}
	negative := false
	for k := 0; k < f.Frozen().KnotCount(); k++ {
		knot, _ := f.Frozen().Knot(k)
		if knot.W < 0 {
			negative = true
		}
	}
	if !negative {
		t.Fatal("fixture must have a negative advance")
	}
	outer := proof.Atomics[0]
	if outer.X != 103+a[0].X || outer.Y != 205+a[0].Y || outer.Line != 0 {
		t.Fatal("atomic translated twice", outer, a)
	}
	for _, b := range proof.Boxes {
		if b.BoxID == outer.BoxID && (b.Frame.X != outer.X || b.Frame.Y != outer.Y || b.Content.X != outer.X+2.25 || b.Content.Y != outer.Y+2.25) {
			t.Fatal("lost precise border/content edges", b, outer)
		}
	}
	seen := map[box.BoxID]bool{}
	for _, a := range proof.Atomics {
		if seen[a.BoxID] {
			t.Fatal("duplicate atomic subtree")
		}
		seen[a.BoxID] = true
	}
	xCount := 0
	for _, g := range proof.Glyphs {
		if g.GroupID == group.Input.GroupID && g.Line != 0 {
			t.Fatal("rendered an excluded parent line")
		}
		if g.GlyphID == 'x'+1 {
			xCount++
			if g.X != proof.Atomics[1].X+1.5 || g.Y != proof.Atomics[1].Y+1.5+12 {
				t.Fatal("nested origin counted twice", g, proof.Atomics[1])
			}
		}
	}
	if xCount != 1 {
		t.Fatal("nested glyph duplicated/lost", xCount)
	}
}

func TestFragmentProofDifferentOwners(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px">aa bb cc dd ee ff gg hh ii jj</div>`)
	cfg := layoutConfig(t)
	r := runDocument(t, d, 80, paragraph.FirstFit, cfg)
	g, _ := r.Group(d.Groups()[0].GroupID)
	text, _, err := cfg.Resolver.ItemizeInline(t.Context(), g.Input.Text, g.Input.Styles, g.Input.Anchors)
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := cfg.Resolver.ResolveStyle(g.Input.Paragraph.Style, cfg.Resolver.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	markup, err := g.Input.BidiMarkup()
	if err != nil {
		t.Fatal(err)
	}
	p, err := paragraph.Prepare(t.Context(), paragraph.Input{Text: text, Direction: g.Input.Paragraph.Style.Direction, Markup: markup}, khipu.PrepareOptions{}, khipu.Config{Streaming: &khipu.StreamingConfig{Resolver: cfg.Resolver, Defaults: defaults, NewShaper: cfg.NewShaper}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.StartFlow(t.Context(), paragraph.LayoutRequest{Params: paragraph.Params{Constraints: paragraph.Constraints{Width: 60 * dimen.BP}, Breaker: paragraph.FirstFit, Assembly: paragraph.Options{Policy: paragraph.CSSLineHeight}, LineLayout: g.Input.LineLayout.Clone()}, Horizontal: &paragraph.HorizontalLayout{}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Fit(t.Context(), paragraph.Frame{Width: 60 * dimen.BP, Height: 15})
	if err != nil || first.Continuation == nil {
		t.Fatal(err)
	}
	last, err := first.Continuation.Fit(t.Context(), paragraph.Frame{Width: 30 * dimen.BP, Height: 1000})
	if err != nil || last.Continuation != nil || last.Placed.Frozen() == first.Placed.Frozen() {
		t.Fatal("needs completed multiple owners", err)
	}
	places := []documentlayout.PositionedFragment{{GroupID: g.Input.GroupID, Fragment: first.Placed}, {GroupID: g.Input.GroupID, Fragment: last.Placed, FrameOrigin: render.Point{X: 200, Y: 50}, Offset: render.Point{X: 2, Y: 3}}}
	proof, err := documentlayout.BuildFragmentProof(t.Context(), r, places, cells())
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, place := range places {
		local, err := render.BuildFragmentProof(t.Context(), render.FragmentInput{Fragment: place.Fragment, Text: g.Input.Text, Paint: g.Input.Paint}, cells())
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range local.Glyphs {
			want.X += place.FrameOrigin.X + place.Offset.X
			want.Y += place.FrameOrigin.Y + place.Offset.Y
			if !reflect.DeepEqual(proof.Glyphs[count].Glyph, want) {
				t.Fatal("wrong owner or transform", proof.Glyphs[count], want)
			}
			count++
		}
	}
	if count != 20 || len(proof.Glyphs) != count {
		t.Fatal("prefix repeated", count, len(proof.Glyphs))
	}
}

type fragmentFailureBackend struct {
	cellProof
	calls  int
	fail   error
	cancel context.CancelFunc
}

func (b *fragmentFailureBackend) Outline(f contracts.FontFace, g uint32) (render.Outline, error) {
	b.calls++
	if b.calls == 3 {
		if b.cancel != nil {
			b.cancel()
		}
		if b.fail != nil {
			return render.Outline{}, b.fail
		}
	}
	return b.cellProof.Outline(f, g)
}
func TestFragmentProofFailuresPublishNothing(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="font-size:20px">aa bb cc dd</div>`)
	r := runDocument(t, d, 30, paragraph.FirstFit, layoutConfig(t))
	g, _ := r.Group(d.Groups()[0].GroupID)
	places := []documentlayout.PositionedFragment{{GroupID: g.Input.GroupID, Fragment: lineFragment(t, g.Paragraph.VList, 0, 1)}, {GroupID: g.Input.GroupID, Fragment: lineFragment(t, g.Paragraph.VList, 1, len(g.Paragraph.VList.Lines()))}}
	sentinel := errors.New("late fragment outline")
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		b := &fragmentFailureBackend{fail: sentinel}
		want := sentinel
		if cancelled {
			b.fail = nil
			b.cancel = cancel
			want = context.Canceled
		}
		var out bytes.Buffer
		err := documentlayout.WriteFragmentSVG(ctx, &out, r, places, render.Services{Backend: b, Identity: b})
		cancel()
		if !errors.Is(err, want) || out.Len() != 0 || b.calls != 3 {
			t.Fatal("partial export", err, out.Len(), b.calls)
		}
	}
	bad := places[0]
	bad.Offset.X = math.Inf(1)
	if p, err := documentlayout.BuildFragmentProof(t.Context(), r, []documentlayout.PositionedFragment{bad}, cells()); p != nil || err == nil {
		t.Fatal("invalid origin")
	}
	bad = places[0]
	bad.GroupID = 999999
	if p, err := documentlayout.BuildFragmentProof(t.Context(), r, []documentlayout.PositionedFragment{bad}, cells()); p != nil || err == nil {
		t.Fatal("unknown group")
	}
	if err := documentlayout.WriteFragmentSVG(t.Context(), shortWriter{}, r, places, cells()); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short writer", err)
	}
	if p, err := documentlayout.BuildFragmentProof(nil, r, places, cells()); p != nil || err == nil {
		t.Fatal("nil context")
	}
	before, err := documentlayout.BuildProof(t.Context(), r, cells())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := documentlayout.BuildFragmentProof(t.Context(), r, places, cells()); err != nil {
		t.Fatal(err)
	}
	after, err := documentlayout.BuildProof(t.Context(), r, cells())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("fragment proof mutated ordinary result", err)
	}
}

func TestFragmentProofEmptyAndSourceValidation(t *testing.T) {
	d, _, _ := build(t, `<div id="root">aa bb</div>`)
	r := runDocument(t, d, 100, paragraph.FirstFit, layoutConfig(t))
	g, _ := r.Group(d.Groups()[0].GroupID)
	empty, err := layout.NewFragment(t.Context(), g.Paragraph.Frozen, layout.LineRange{}, layout.Box{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := &fragmentFailureBackend{}
	proof, err := documentlayout.BuildFragmentProof(t.Context(), r, []documentlayout.PositionedFragment{{GroupID: g.Input.GroupID, Fragment: empty, FrameOrigin: render.Point{X: 999, Y: 999}}}, render.Services{Backend: b, Identity: b})
	if err != nil || !reflect.DeepEqual(proof, &documentlayout.Proof{}) || b.calls != 0 {
		t.Fatal("empty range rendered owner", proof, err, b.calls)
	}
	foreign, _, _ := build(t, `<div id="root">cc dd</div>`)
	other := runDocument(t, foreign, 100, paragraph.FirstFit, layoutConfig(t))
	h, _ := other.Group(foreign.Groups()[0].GroupID)
	f := lineFragment(t, h.Paragraph.VList, 0, len(h.Paragraph.VList.Lines()))
	proof, err = documentlayout.BuildFragmentProof(t.Context(), r, []documentlayout.PositionedFragment{{GroupID: g.Input.GroupID, Fragment: f}}, cells())
	if proof != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("foreign text paired with source/paint", proof, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	proof, err = documentlayout.BuildFragmentProof(ctx, r, nil, cells())
	if proof != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("early cancellation lost", err)
	}
}
