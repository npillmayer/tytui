package documentlayout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	boxtext "github.com/npillmayer/css-box-layout/text"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/fontselection"
	"tytui/htmlinput"
)

const nestedLifetimeHTML = `<div id="root" style="font-size:20px;color:red">start <span style="display:inline-block;padding:2px">pre <span style="display:inline-block;padding:1px;margin-left:-1px;margin-right:2px">éa <span style="color:blue">bbbb</span> <span dir="rtl">אב</span></span> post</span> end</div>`

// Capture values immediately into an immutable string, not a GroupResult holding
// shared Frozen/VList pointers or even potentially aliased accessor slices. The
// formatter orders map keys; synthetic font/scale values have no mutable state.
// Read every frozen record, including glyphs, clusters, features and source bytes.
func lifetimeSnapshot(t *testing.T, r *Result) string {
	t.Helper()
	var s strings.Builder
	fmt.Fprintf(&s, "geometry=%#v\ngroups=%#v\n", r.Geometry(), r.Groups())
	for _, entry := range r.Groups() {
		g, ok := r.Group(entry.GroupID)
		if !ok || g.Input.Group != entry || g.Paragraph.Frozen == nil || g.Paragraph.VList == nil {
			t.Fatal("incomplete retained group", entry)
		}
		f, v := g.Paragraph.Frozen, g.Paragraph.VList
		if v.Frozen() != f {
			t.Fatal("vlist lost its retained frozen paragraph")
		}
		fmt.Fprintf(&s, "input=%#v\nbreaks=%#v bidi=%#v counts=%d,%d\n", g.Input,
			g.Paragraph.BreakPositions, g.Paragraph.BidiRuns, g.Paragraph.EncodedKnots, g.Paragraph.BreakCount)
		fmt.Fprintf(&s, "box=%#v lines=%#v materials=%#v atomics=%#v\n", v.Box(), v.Lines(), v.Materials(), v.AtomicPlacements())
		reader, err := f.SourceReader(khipu.ByteRange{End: f.SourceLen()})
		if err != nil {
			t.Fatal(err)
		}
		text, err := io.ReadAll(reader)
		if err != nil || string(text) != g.Input.Text {
			t.Fatal("retained source differs from extracted input", string(text), err)
		}
		fmt.Fprintf(&s, "source=%q offset=%d\n", text, f.SourceOffset())
		for n := 0; n < f.LineCount(); n++ {
			line, ok := f.Line(n)
			if !ok {
				t.Fatal("missing frozen line", n)
			}
			fmt.Fprintf(&s, "line=%#v\n", line)
		}
		for n := 0; n < f.KnotCount(); n++ {
			knot, ok := f.Knot(n)
			if !ok {
				t.Fatal("missing frozen knot", n)
			}
			text, hasText := f.CopyText(n)
			id, hasAtomic := f.Atomic(n)
			fmt.Fprintf(&s, "knot=%#v text=%t:%#v atomic=%t:%d\n", knot, hasText, text, hasAtomic, id)
			if hasText {
				if text.Context.Params.Font.Face() == nil || text.Context.Params.Font.Scale() == nil {
					t.Fatal("missing retained font/scale service")
				}
				metrics, err := text.Context.Params.Font.Face().Metrics()
				if err != nil {
					t.Fatal("retained font service became unusable", err)
				}
				fmt.Fprintf(&s, "metrics=%#v scaled-em=%g\n", metrics, text.Context.Params.Font.Scale().ToDU(int32(metrics.UnitsPerEm)))
			}
		}
		for _, source := range g.Input.TextSources {
			// Locator NodeIDs are original DOM indexes; CSS IDs reserve zero.
			node, ok := r.Document().NodeSource(box.NodeID(source.Locator.NodeID + 1))
			if !ok || node.Locator != source.Locator {
				t.Fatal("lost original source locator", source)
			}
			fmt.Fprintf(&s, "node=%#v\n", node)
			text, ok := r.Document().TextSource(boxtext.TextSourceID(source.Locator.NodeID + 1))
			if !ok || text.Locator != source.Locator {
				t.Fatal("lost original text source", source)
			}
			fmt.Fprintf(&s, "original=%#v\n", text)
		}
		placements := v.AtomicPlacements()
		if len(placements) != len(g.Input.AtomicSources) {
			t.Fatal("lost retained atomic occurrence")
		}
		for _, source := range g.Input.AtomicSources {
			found := 0
			for _, p := range placements {
				if p.ID == source.ID {
					found++
				}
			}
			if found != 1 || r.Geometry()[source.BoxID].RelativeTo != g.Input.OwnerID {
				t.Fatal("lost AtomicID-to-BoxID correspondence", source)
			}
		}
	}
	return s.String()
}

// Mutate only caller-owned return values. Frozen/VList, their font services and
// the normalized tree stay read-only. Record coverage so an empty fixture cannot
// silently skip the glyph, cluster, isolate or placement ownership assertions.
func mutateLifetimeAccessors(t *testing.T, r *Result) {
	t.Helper()
	clear(r.Geometry())
	groups := r.Groups()
	groups[0] = Group{}
	var glyphs, clusters, placements, isolates int
	for _, entry := range r.Groups() {
		g, _ := r.Group(entry.GroupID)
		clear(g.Input.Styles)
		clear(g.Input.Paint)
		clear(g.Input.LineLayout.Runs)
		g.Input.LineLayout.Default = paragraph.LineHeight{}
		isolates += len(g.Input.Isolates)
		clear(g.Input.Isolates)
		clear(g.Input.Anchors)
		clear(g.Input.TextSources)
		clear(g.Input.AtomicSources)
		clear(g.Paragraph.BreakPositions)
		clear(g.Paragraph.BidiRuns)
		v, f := g.Paragraph.VList, g.Paragraph.Frozen
		clear(v.Lines())
		clear(v.Materials())
		p := v.AtomicPlacements()
		placements += len(p)
		clear(p)
		for n := 0; n < f.KnotCount(); n++ {
			text, ok := f.CopyText(n)
			if ok {
				glyphs += len(text.Glyphs)
				clusters += len(text.Clusters)
				clear(text.Glyphs)
				clear(text.Clusters)
				clear(text.Context.Params.Features)
			}
		}
		g.Input.Text = "mutated envelope"
		g.Input.Group = Group{}
		g.Paragraph = paragraph.Result{}
	}
	if glyphs == 0 || clusters == 0 || placements != 2 || isolates == 0 {
		t.Fatal("ownership fixture lost coverage", glyphs, clusters, placements, isolates)
	}
}

// A separate resolver is used only by a failing invocation. Previously retained
// results keep their original stable services. Metrics fail synchronously
// after a child completes, deterministically reaching final assembly.
type lifetimeFaultLoader struct{ metrics func() error }
type lifetimeFaultFont struct {
	contracts.FontFace
	metrics func() error
}

type lifetimeFaultStream struct{ contracts.StreamingShaper }

func (s lifetimeFaultStream) Shape(ctx context.Context, p contracts.ShapeParams, text contracts.RuneReader, opts contracts.ShapeBufferOptions) error {
	// Only the test shaper needs the concrete synthetic face. Khipu retains the
	// original wrapper in its run context for the later assembly metric query.
	if f, ok := p.Font.Face().(*lifetimeFaultFont); ok {
		p.Font, _ = contracts.NewFontInstance(f.FontFace, p.Font.EmSize())
	}
	return s.StreamingShaper.Shape(ctx, p, text, opts)
}

func (f lifetimeFaultFont) Metrics() (contracts.FontMetrics, error) {
	if err := f.metrics(); err != nil {
		return contracts.FontMetrics{}, err
	}
	return f.FontFace.Metrics()
}
func (l lifetimeFaultLoader) Load(ctx context.Context, path string, resource *fontresource.Resource) (glyphing.FaceEntry, error) {
	f, err := (boundaryLoader{}).Load(ctx, path, resource)
	if err == nil {
		f.Face = &lifetimeFaultFont{FontFace: f.Face, metrics: l.metrics}
	}
	return f, err
}

func freshLifetimeInvocation(d *Document, request LayoutRequest, cfg Config) *invocation {
	return &invocation{document: d, request: request, config: cfg,
		prepared: map[box.BoxID]*preparedGroup{}, started: map[box.BoxID]bool{}, groups: map[box.BoxID]GroupResult{}}
}

func TestNestedResizeFailureRecoveryAndLifetime(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		for _, fault := range []string{"intrinsic error", "assembly error"} {
			t.Run(string(breaker)+"/"+fault, func(t *testing.T) {
				seed := boundaryInvocation(t, nestedLifetimeHTML)
				d, cfg := seed.document, seed.config
				if len(d.Groups()) != 3 {
					t.Fatal("nested fixture changed")
				}
				// The helper has released the original HTML input after Build. All
				// subsequent calls use the normalized document and stable services.
				factories := 0
				var shapes atomic.Int32
				cfg.NewShaper = func() (contracts.StreamingShaper, error) {
					factories++
					s, err := helpers.NewSyntheticStreamingShaper(8)
					return observedShapeRuns{StreamingShaper: s, runs: &shapes}, err
				}
				var retained []*Result
				var snapshots []string
				checkRetained := func() {
					t.Helper()
					for n, r := range retained {
						if r.Document() != d || lifetimeSnapshot(t, r) != snapshots[n] {
							t.Fatalf("earlier result %d changed after later operation", n)
						}
					}
				}
				var baseRuns int32
				success := func(width float32) *Result {
					t.Helper()
					beforeFactories, beforeShapes := factories, shapes.Load()
					r, err := d.Layout(LayoutRequest{Width: width, Breaker: breaker}, cfg)
					if err != nil {
						t.Fatal(err)
					}
					if factories-beforeFactories != len(d.Groups()) {
						t.Fatal("preparation reused across invocations or repeated within invocation")
					}
					runs := shapes.Load() - beforeShapes
					if baseRuns == 0 {
						baseRuns = runs
					}
					if runs == 0 || runs != baseRuns {
						t.Fatal("resize/recovery changed base shaping work", runs, baseRuns)
					}
					checkRetained()
					for _, old := range retained {
						for _, entry := range r.Groups() {
							a, _ := r.Group(entry.GroupID)
							b, _ := old.Group(entry.GroupID)
							if a.Paragraph.Frozen == b.Paragraph.Frozen || a.Paragraph.VList == b.Paragraph.VList {
								t.Fatal("independent layout reused a completed result")
							}
						}
					}
					retained = append(retained, r)
					snapshots = append(snapshots, lifetimeSnapshot(t, r))
					return r
				}
				wide := success(300)
				narrow := success(60)
				inner := d.Groups()[2]
				if wide.Geometry()[inner.OwnerID].Content.W <= narrow.Geometry()[inner.OwnerID].Content.W {
					t.Fatal("resize did not change nested auto content width")
				}
				i := freshLifetimeInvocation(d, LayoutRequest{Width: 60, Breaker: breaker}, cfg)
				sentinel := errors.New("nested lifetime injected failure")
				want := error(sentinel)
				hits := 0
				switch {
				case strings.HasPrefix(fault, "intrinsic"):
					calls := 0
					i.config.NewShaper = func() (contracts.StreamingShaper, error) {
						calls++
						if calls == 2 {
							hits++
							if len(i.prepared) != 1 || i.prepared[inner.GroupID] == nil || len(i.started) != 0 || len(i.groups) != 0 {
								t.Error("failure did not occur during later intrinsic preparation")
							}
							return nil, sentinel
						}
						return helpers.NewSyntheticStreamingShaper(8)
					}
				case strings.HasPrefix(fault, "assembly"):
					resolver, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "synthetic", Size: 10 * dimen.BP}, testServices{
						Loader: lifetimeFaultLoader{metrics: func() error {
							if len(i.groups) == 1 {
								hits++
								if _, ok := i.groups[inner.GroupID]; !ok || !i.started[d.Groups()[1].GroupID] {
									t.Error("assembly fault preceded nested child completion")
								}
								return sentinel
							}
							return nil
						}}, Coverage: glyphing.CoverageFuncs{Check: checkSyntheticFace, Lookup: func(contracts.FontFace, rune) (uint32, error) { return 1, nil }},
					})
					if err != nil {
						t.Fatal(err)
					}
					i.config.Resolver = resolver
					i.config.NewShaper = func() (contracts.StreamingShaper, error) {
						s, err := helpers.NewSyntheticStreamingShaper(8)
						return lifetimeFaultStream{StreamingShaper: s}, err
					}
				}
				// Use the actual invocation runner to witness private state at failure;
				// successes/recovery use the public API. No replacement CSS scheduler.
				out, err := i.run()
				if hits != 1 || out != nil || !errors.Is(err, want) {
					t.Fatal("fault not reached or partial document published", hits, out, err)
				}
				switch {
				case strings.HasPrefix(fault, "intrinsic"):
					var stage *paragraph.Error
					if !errors.As(err, &stage) || stage.Stage != paragraph.StagePrepare || len(i.prepared) != 1 || len(i.groups) != 0 || len(i.started) != 0 {
						t.Fatal("failed intrinsic preparation mutated acceptance/cache or failed at the wrong stage", err)
					}
				case strings.HasPrefix(fault, "assembly"):
					var stage *paragraph.Error
					if !errors.As(err, &stage) || stage.Stage != paragraph.StageAssemble || len(i.prepared) != 2 || len(i.groups) != 1 || len(i.started) != 2 {
						t.Fatal("not a late assembly failure", err, len(i.groups), len(i.started))
					}
					// Exercise the failed-attempt guard directly, never through a scheduler retry.
					entry := d.groups[d.Groups()[1].GroupID]
					_, repeat := i.LayoutInline(box.InlineRequest{Group: entry.node, OwnerID: entry.OwnerID, Used: usedView{}, Atomics: preparedView{}})
					if !errors.Is(repeat, khipu.ErrUnsupportedPolicy) || hits != 1 || len(i.groups) != 1 || len(i.started) != 2 {
						t.Fatal("failed final callback was retried", repeat)
					}
				}
				checkRetained()
				for _, r := range retained {
					mutateLifetimeAccessors(t, r)
					checkRetained()
				}
				success(300)
				if snapshots[0] != snapshots[2] {
					t.Fatal("fresh wide layout did not reproduce original output")
				}
				checkRetained()
			})
		}
	}
}

func TestNestedResultOutlivesOriginalDOM(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			input, err := htmlinput.Parse("lifetime.html", strings.NewReader(nestedLifetimeHTML))
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
			d, err := Build(input, selected)
			if err != nil {
				t.Fatal(err)
			}
			cfg := boundaryInvocation(t).config
			r, err := d.Layout(LayoutRequest{Width: 300, Breaker: breaker}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			before := lifetimeSnapshot(t, r)
			// Destroy the original source envelope, not the normalized tree or services.
			clear(input.Nodes)
			*input = htmlinput.Document{}
			input, d = nil, nil
			mutateLifetimeAccessors(t, r)
			if lifetimeSnapshot(t, r) != before {
				t.Fatal("result depended on original DOM or caller's document variable")
			}
			again, err := r.Document().Layout(LayoutRequest{Width: 300, Breaker: breaker}, cfg)
			if err != nil || lifetimeSnapshot(t, again) != before || lifetimeSnapshot(t, r) != before {
				t.Fatal("retained normalized document lost source/style ownership", err)
			}
		})
	}
}

func (s lifetimeFaultStream) CheckFace(face contracts.FontFace) error {
	if f, ok := face.(*lifetimeFaultFont); ok {
		if f == nil {
			return contracts.ErrIllegalArguments
		}
		face = f.FontFace
	}
	return s.StreamingShaper.CheckFace(face)
}
