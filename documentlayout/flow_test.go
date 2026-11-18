package documentlayout

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/render"
)

func flowFixture(t *testing.T, source string) (*invocation, *preparationCalls) {
	t.Helper()
	i := boundaryInvocation(t, source)
	c := countPreparation(t, i)
	return i, c
}
func flowFrames(widths ...float64) []FlowFrame {
	fs := make([]FlowFrame, len(widths))
	for j, w := range widths {
		fs[j] = FlowFrame{ID: string(rune('A' + j)), X: float64(j) * 110, Width: w, Height: 20}
	}
	fs[len(fs)-1].Height = 1000
	return fs
}
func flowDrawing() render.Services {
	b := geometryProofBackend{}
	return render.Services{Backend: b, Identity: b}
}

func TestFlowInvocationDispatchAndRelease(t *testing.T) {
	for _, widths := range [][]float64{{100, 100}, {100, 60}, {100, 150}, {100, 60, 100}} {
		original, calls := flowFixture(t, `<div id="root" style="font-size:20px;color:red;margin:3px 0">aa bb cc dd ee ff gg hh ii jj kk ll mm nn oo pp</div>`)
		frames := flowFrames(widths...)
		i, owned, err := original.document.flowInvocation(t.Context(), FlowRequest{Layout: original.request, Frames: frames}, original.config)
		if err != nil {
			t.Fatal(err)
		}
		source, err := i.run()
		if err != nil {
			t.Fatal(err)
		}
		if len(i.started) != 1 || len(i.prepared) != 1 || calls.shape != 1 {
			t.Fatal("initial callback/preparation repeated", calls, i.started)
		}
		before := *calls
		released := false
		i.config.Trace = func(s string) {
			if strings.Contains(s, "flow frame=") && i.flow.token == nil {
				released = true
				if len(i.prepared) != 0 {
					t.Error("completion retained preparation")
				}
			}
		}
		out, err := i.finishFlow(source, owned)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Complete() || !released || i.flow.token != nil || len(i.prepared) != 0 || *calls != before {
			t.Fatal("lifecycle or base-service work", calls, before)
		}
		if !out.frames[0].Reused || (widths[1] == 100) != out.frames[1].Reused {
			t.Fatal("wrong dispatch", out.frames)
		}
		var end uint64
		for _, f := range out.Frames() {
			if f.Fragment != nil {
				for _, l := range f.Fragment.Lines() {
					if l.Horizontal.Source.Start != end {
						t.Fatal("source overlap/gap")
					}
					end = l.Horizontal.Source.End
				}
			}
		}
		if end != uint64(len(out.Group().Text)) || out.SurroundingMargins().Resolved.Top != 3 {
			t.Fatal("source/margins")
		}
		frames[0].Width = 999
		detached := out.Frames()
		detached[0].Frame.Width = 777
		if out.Frames()[0].Frame.Width != 100 {
			t.Fatal("frame alias")
		}
		g := out.Group()
		g.Paint[0].Paint.Color = "blue"
		if out.Group().Paint[0].Paint.Color != "red" {
			t.Fatal("paint alias")
		}
		proof, err := BuildFlowProof(t.Context(), out, flowDrawing())
		if err != nil || len(proof.Glyphs) != 32 {
			t.Fatal("retained rendering", err)
		}
		var svg bytes.Buffer
		if err := WriteFlowSVG(t.Context(), &svg, out, render.Services{}, LayoutSVGOptions{}); err != nil || !strings.Contains(svg.String(), `data-frame="A"`) || strings.Contains(svg.String(), `data-gid=`) {
			t.Fatal("boxes export", err)
		}
		if *calls != before {
			t.Fatal("export called services")
		}
	}
}

func TestFlowEdgesAtomicsNoProgressAndExhaustion(t *testing.T) {
	original, calls := flowFixture(t, `<div id="root" style="font-size:20px;padding:7px;border-width:3px;border-style:solid;margin-top:4px">aa bb cc dd ee ff <span style="display:inline-block;width:30px;padding:2px">x</span> gg hh ii jj</div>`)
	req := FlowRequest{Layout: original.request, Frames: []FlowFrame{{ID: "empty", Width: 80, Height: 0}, {ID: "A", Width: 80, Height: 20}, {ID: "B", X: 100, Width: 60, Height: 1000}, {ID: "unused", Width: 60, Height: 10}}}
	out, err := original.document.LayoutFlow(t.Context(), req, original.config)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Complete() || out.frames[0].Status != paragraph.FitNoProgress || out.frames[3].Attempted || calls.shape != 2 {
		t.Fatal("progress/preparation", out.frames, calls)
	}
	p, err := BuildFlowProof(t.Context(), out, flowDrawing())
	if err != nil || len(p.Atomics) != 1 {
		t.Fatal("atomic duplicated/lost", err)
	}
	m := out.SurroundingMargins()
	if m.Resolved.Top != 4 {
		t.Fatal("owner margins", m)
	}
	before := out.Frames()
	req.Frames = req.Frames[:1]
	incomplete, err := original.document.LayoutFlow(t.Context(), req, original.config)
	if err != nil || incomplete.Complete() || len(incomplete.Remainder().Lines()) == 0 {
		t.Fatal("missing remainder", err)
	}
	var buf bytes.Buffer
	if err := WriteFlowSVG(t.Context(), &buf, incomplete, flowDrawing(), LayoutSVGOptions{ShowGlyphs: true}); !errors.Is(err, khipu.ErrUnsupportedPolicy) || buf.Len() != 0 {
		t.Fatal("incomplete export", err)
	}
	if !reflect.DeepEqual(before, out.Frames()) {
		t.Fatal("later invocation mutated earlier output")
	}
	req.Frames[0].Width = 100
	if bad, err := original.document.LayoutFlow(t.Context(), req, original.config); bad != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
		t.Fatal("ignored CSS edges", err)
	}
}

func TestFlowAdmissionCancellationAndOverflow(t *testing.T) {
	for _, source := range []string{`<div id="root"><p>a</p><p>b</p></div>`, `<div id="root">a<span style="display:inline-block">b</span></div>`} {
		i, _ := flowFixture(t, source)
		if out, err := i.document.LayoutFlow(t.Context(), FlowRequest{Layout: i.request, Frames: flowFrames(100, 100)}, i.config); out != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
			t.Fatal("unsupported flow accepted", err)
		}
	}
	i, _ := flowFixture(t, `<div id="root" style="font-size:20px">abcdefghijklmno</div>`)
	req := FlowRequest{Layout: i.request, Frames: flowFrames(100)}
	out, err := i.document.LayoutFlow(t.Context(), req, i.config)
	if err != nil || !out.Complete() || len(out.Frames()[0].Overfull) == 0 {
		t.Fatal("horizontal overflow hidden", err)
	}
	for _, mutate := range []func(*FlowRequest){func(r *FlowRequest) { r.Frames = nil }, func(r *FlowRequest) { r.Frames[0].Width = math.NaN() }, func(r *FlowRequest) { r.Frames[0].X = math.MaxFloat64 }, func(r *FlowRequest) { r.Frames[0].Height = -1 }, func(r *FlowRequest) { r.Frames[0].ID = "" }, func(r *FlowRequest) { r.Frames = append(r.Frames, r.Frames[0]) }} {
		bad := FlowRequest{Layout: i.request, Frames: flowFrames(100)}
		mutate(&bad)
		if out, err := i.document.LayoutFlow(t.Context(), bad, i.config); out != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal("invalid frame admitted", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cfg := i.config
	cfg.Trace = func(s string) {
		if strings.Contains(s, "flow frame=") {
			cancel()
		}
	}
	if out, err := i.document.LayoutFlow(ctx, req, cfg); out != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("late cancellation published", err)
	}
	for _, source := range []string{`<div id="root"></div>`, `<div id="root"><span style="display:inline-block;width:0px"></span></div>`} {
		i, _ := flowFixture(t, source)
		r, err := i.document.LayoutFlow(t.Context(), FlowRequest{Layout: i.request, Frames: flowFrames(100, 100)}, i.config)
		if err != nil || !r.Complete() {
			t.Fatal("empty/zero content stalled", err)
		}
	}
}

// Probe actual shaping/preflight calls as well as preparation factories. The
// preparation remains serialized, so these counters need no synchronization.
type flowServiceCounter struct {
	contracts.StreamingShaper
	checks, shapes *int
}

func (s flowServiceCounter) CheckFace(f contracts.FontFace) error {
	*s.checks++
	return s.StreamingShaper.CheckFace(f)
}
func (s flowServiceCounter) Shape(ctx context.Context, p contracts.ShapeParams, r contracts.RuneReader, o contracts.ShapeBufferOptions) error {
	*s.shapes++
	return s.StreamingShaper.Shape(ctx, p, r, o)
}

type flowOutlineFailure struct {
	geometryProofBackend
	err error
}

func (b flowOutlineFailure) Outline(contracts.FontFace, uint32) (render.Outline, error) {
	return render.Outline{}, b.err
}

func TestFlowServiceBoundaryAndFailureCleanup(t *testing.T) {
	original, _ := flowFixture(t, `<div id="root" style="font-size:20px">aa bb cc dd ee ff gg hh ii jj</div>`)
	checks, shapes := 0, 0
	factory := original.config.NewShaper
	original.config.NewShaper = func() (contracts.StreamingShaper, error) {
		s, e := factory()
		return flowServiceCounter{s, &checks, &shapes}, e
	}
	i, frames, err := original.document.flowInvocation(t.Context(), FlowRequest{Layout: original.request, Frames: flowFrames(100, 60, 100)}, original.config)
	if err != nil {
		t.Fatal(err)
	}
	source, err := i.run()
	if err != nil {
		t.Fatal(err)
	}
	beforeChecks, beforeShapes := checks, shapes
	out, err := i.finishFlow(source, frames)
	if err != nil {
		t.Fatal(err)
	}
	if checks != beforeChecks || shapes != beforeShapes || beforeShapes == 0 {
		t.Fatal("continuation repeated shaping/CheckFace")
	}
	sentinel := errors.New("outline unavailable")
	b := flowOutlineFailure{err: sentinel}
	var buf bytes.Buffer
	if err := WriteFlowSVG(t.Context(), &buf, out, render.Services{Backend: b, Identity: b}, LayoutSVGOptions{ShowGlyphs: true}); !errors.Is(err, sentinel) || buf.Len() != 0 {
		t.Fatal("failed export wrote", err)
	}
	// A cancelled continuation clears both owning locations deterministically.
	ctx, cancel := context.WithCancel(t.Context())
	i, frames, err = original.document.flowInvocation(ctx, FlowRequest{Layout: original.request, Frames: flowFrames(100, 60)}, original.config)
	if err != nil {
		t.Fatal(err)
	}
	source, err = i.run()
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if out, err := i.finishFlow(source, frames); out != nil || !errors.Is(err, context.Canceled) || len(i.prepared) != 0 || i.flow.token != nil {
		t.Fatal("failed flow retained working handles", err)
	}
}

func TestFlowFrameRoundingAndInheritedStyles(t *testing.T) {
	i, _ := flowFixture(t, `<div id="root" style="font-size:20px;color:red">aa bb cc dd ee ff gg hh</div>`)
	fs := flowFrames(100+.25/duPerCSSPixel, 60)
	out, err := i.document.LayoutFlow(t.Context(), FlowRequest{Layout: i.request, Frames: fs}, i.config)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := BuildFlowProof(t.Context(), out, flowDrawing())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range proof.Glyphs {
		if g.Color != "red" {
			t.Fatal("frame changed paint")
		}
	}
	fs[0].Width = 100 + 1/duPerCSSPixel
	if out, err := i.document.LayoutFlow(t.Context(), FlowRequest{Layout: i.request, Frames: fs}, i.config); out != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
		t.Fatal("rounded callback mismatch accepted", err)
	}
}
