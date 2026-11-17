package documentlayout

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/fontselection"
	"tytui/htmlinput"
)

func TestCSSPrimitiveConversion(t *testing.T) {
	// These binary-exact CSS values produce exact half-DU ties (3/2 and 9/2).
	for _, test := range []struct {
		px float32
		du dimen.DU
	}{
		{0, 0}, {1, 49152}, {96, 72 * dimen.BP}, {1.0 / 32768, 2}, {-1.0 / 32768, -2}, {3.0 / 32768, 5}, {-3.0 / 32768, -5},
	} {
		got, err := cssToDU(test.px, true)
		if err != nil || got != test.du {
			t.Fatalf("%gpx: %d, %v, want %d", test.px, got, err, test.du)
		}
		if math.Abs(float64(got)-float64(test.px)*duPerCSSPixel) > .5 {
			t.Fatal("primitive quantization budget exceeded")
		}
	}
	for _, px := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)), float32(float64(dimen.Fil) / duPerCSSPixel), -float32(float64(dimen.Fil) / duPerCSSPixel)} {
		if _, err := cssToDU(px, true); !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted non-finite primitive", px, err)
		}
	}
	if _, err := cssToDU(-.00000001, false); !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("negative size rounded to zero")
	}
	last := math.Nextafter32(float32(float64(dimen.Fil)/duPerCSSPixel), 0)
	if _, err := cssToDU(last, false); err != nil {
		t.Fatal("last representable CSS width", err)
	}
}

func TestCSSSummaryConversion(t *testing.T) {
	for _, points := range []float64{0, 72, -72, 1.0 / 65536, 123.456789, -123.456789} {
		px, err := pointsToCSS(points, true)
		if err != nil {
			t.Fatal(err)
		}
		// Only float32 narrowing: no additional DU rounding at publication.
		want := points * (4.0 / 3.0)
		ulp := math.Abs(float64(math.Nextafter32(px, float32(math.Inf(1)))) - float64(px))
		if math.Abs(float64(px)-want) > ulp/2 {
			t.Fatal("float32 rounding budget exceeded", points, px)
		}
	}
	limit := float64(dimen.Fil) / float64(dimen.BP)
	for _, points := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), limit, -limit, math.Nextafter(limit, 0), -math.Nextafter(limit, 0)} {
		if _, err := pointsToCSS(points, true); !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted unrepresentable summary", points, err)
		}
	}
	if _, err := pointsToCSS(-1, false); !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("negative extent accepted")
	}
}

type preparedView map[box.BoxID]box.PreparedAtomic

func (v preparedView) Lookup(id box.BoxID) (box.PreparedAtomic, bool) { a, ok := v[id]; return a, ok }

type usedView map[box.BoxID]box.UsedValues

func (v usedView) Lookup(id box.BoxID) (box.UsedValues, bool) { a, ok := v[id]; return a, ok }

func TestAtomicConversionUsesIndependentPrimitives(t *testing.T) {
	g := &InlineGroup{AtomicSources: []AtomicSource{{ID: 91, BoxID: 7}, {ID: 3, BoxID: 19}}}
	prepared := preparedView{7: {BoxID: 7, BorderWidth: 1.0 / 32768, BorderHeight: 0}, 19: {BoxID: 19}}
	used := usedView{7: {Margin: box.Edges{Left: 1.0 / 32768, Right: 1.0 / 32768}}, 19: {}}
	request := box.InlineRequest{Atomics: prepared, Used: used}
	atoms, err := atomicBoxes(g, request)
	if err != nil {
		t.Fatal(err)
	}
	// 1.5 + 1.5 + 1.5 DU rounds independently to 6, not round(4.5)=5.
	if atoms[0].ID != 91 || atoms[0].BorderWidth+atoms[0].Margin.Left+atoms[0].Margin.Right != 6 || atoms[1].ID != 3 {
		t.Fatal("summed before conversion or mapped by index", atoms)
	}
	if prepared[7].BorderWidth != 1.0/32768 {
		t.Fatal("changed CSS prepared width")
	}
	for _, mutation := range []func(){
		func() { delete(prepared, 7) },
		func() { prepared[7] = box.PreparedAtomic{BoxID: 9} },
		func() { prepared[7] = box.PreparedAtomic{BoxID: 7, BorderHeight: -1} },
		func() { prepared[7] = box.PreparedAtomic{BoxID: 7}; delete(used, 7) },
		func() { used[7] = box.UsedValues{WidthPending: true} },
		func() { used[7] = box.UsedValues{EdgesPending: true} },
		func() { used[7] = box.UsedValues{Margin: box.Edges{Left: float32(math.Inf(1))}} },
	} {
		mutation()
		if _, err := atomicBoxes(g, request); !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("accepted malformed atomic view", err)
		}
	}
}

type boundaryLoader struct{}

func (boundaryLoader) Load(ctx context.Context, path string, r *fontresource.Resource) (glyphing.FaceEntry, error) {
	var err error
	if r == nil {
		r, err = fontresource.Read(strings.NewReader("boundary-test"), fontresource.Origin{Provider: "test", Location: path})
	}
	if err != nil {
		return glyphing.FaceEntry{}, err
	}
	return glyphing.FaceEntry{Face: helpers.NewSyntheticFont(), Resource: r, Info: fontmeta.Info{ResourceID: r.ID(), SHA256: r.SHA256(), Origin: r.Origin(), Path: r.Origin().Location}}, nil
}

func boundaryInvocation(t *testing.T, source ...string) *invocation {
	t.Helper()
	html := `<div id="root" style="font-size:20px"><span style="display:inline-block;width:10px"></span></div>`
	if len(source) > 0 {
		html = source[0]
	}
	input, err := htmlinput.Parse("boundary.html", strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	selected := -1
	for _, n := range input.Nodes {
		for _, a := range n.Attributes {
			if a.Key == "id" && a.Val == "root" {
				selected = n.ID
			}
		}
	}
	d, err := Build(t.Context(), input, selected)
	if err != nil {
		t.Fatal(err)
	}
	r, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "synthetic", Size: 10 * dimen.BP}, testServices{Loader: boundaryLoader{}, Coverage: glyphing.CoverageFuncs{Check: checkSyntheticFace, Lookup: func(contracts.FontFace, rune) (uint32, error) { return 1, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	return &invocation{ctx: t.Context(), document: d, request: LayoutRequest{Width: 100, Breaker: paragraph.FirstFit}, config: Config{Resolver: r, NewShaper: func() (contracts.StreamingShaper, error) { return helpers.NewSyntheticStreamingShaper(8) }}, prepared: map[box.BoxID]*preparedGroup{}, started: map[box.BoxID]bool{}, groups: map[box.BoxID]GroupResult{}}
}

func TestInlineCallbackIdentityAndRepetition(t *testing.T) {
	i := boundaryInvocation(t)
	entry := i.document.groups[i.document.order[0].GroupID]
	g, err := i.document.Extract(t.Context(), entry.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	id := g.AtomicSources[0].BoxID
	// These arbitrary completed dimensions must be consumed, never recomputed
	// from the atomic subtree. Fractional CSS border width must stay unchanged.
	req := box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, ContentWidth: 100,
		Used: usedView{id: {}, entry.OwnerID: {}}, Atomics: preparedView{id: {BoxID: id, BorderWidth: .12345, BorderHeight: 3}}}
	wrong := req
	wrong.OwnerID++
	if _, err := i.LayoutInline(wrong); !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("accepted wrong owner", err)
	}
	wrong = req
	node := *req.Group
	wrong.Group = &node
	if _, err := i.LayoutInline(wrong); !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("accepted foreign tree", err)
	}
	out, err := i.LayoutInline(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Atomics) != 1 || out.Atomics[0].BoxID != id || out.Atomics[0].Y != 13 {
		t.Fatal("did not use prepared height", out)
	}
	if _, err := i.LayoutInline(req); !errors.Is(err, khipu.ErrUnsupportedPolicy) || len(i.groups) != 1 {
		t.Fatal("repeated callback overwrote result", err)
	}
}

func TestInlineCallbackPlacesZeroSizedPreparedBox(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		i := boundaryInvocation(t)
		i.request.Breaker = breaker
		entry := i.document.groups[i.document.order[0].GroupID]
		g, err := i.document.Extract(t.Context(), entry.GroupID)
		if err != nil {
			t.Fatal(err)
		}
		id := g.AtomicSources[0].BoxID
		out, err := i.LayoutInline(box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, ContentWidth: 0,
			Used: usedView{id: {}, entry.OwnerID: {}}, Atomics: preparedView{id: {BoxID: id}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Atomics) != 1 || out.Atomics[0] != (box.AtomicPlacement{BoxID: id, Y: 16}) {
			t.Fatal("zero-sized prepared item lost or resized", out)
		}
	}
}

// Cancellation becomes visible only after the final callback retained a complete
// paragraph. This exercises the whole-flow publication check deterministically.
type publicationContext struct {
	context.Context
	ready func() bool
}

func (c publicationContext) Err() error {
	if c.ready() {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestCancellationAtDocumentPublication(t *testing.T) {
	i := boundaryInvocation(t)
	i.ctx = publicationContext{Context: t.Context(), ready: func() bool { return len(i.groups) == len(i.document.groups) }}
	out, err := i.run()
	if out != nil || !errors.Is(err, context.Canceled) || len(i.groups) != len(i.document.groups) {
		t.Fatal("partial document published", out, err)
	}
}

func checkSyntheticFace(face contracts.FontFace) error {
	if f, ok := face.(*lifetimeFaultFont); ok {
		if f == nil {
			return contracts.ErrIllegalArguments
		}
		face = f.FontFace
	}
	return new(helpers.SyntheticStreamingShaper).CheckFace(face)
}
