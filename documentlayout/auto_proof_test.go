package documentlayout_test

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	box "github.com/npillmayer/css-box-layout/layout"
	boxtext "github.com/npillmayer/css-box-layout/text"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	ot "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	xbidi "golang.org/x/text/unicode/bidi"
	"tytui/documentlayout"
	"tytui/htmlinput"
)

//go:embed testdata/nested-auto.html
var nestedAutoFixture string

// These guards wrap real OpenType services only in tests. Backend-specific
// unwrapping stays here; no concrete face assumptions enter documentlayout.
type proofServiceGuard struct {
	proofOnly                                         atomic.Bool
	loads, coverage, metrics, scales, streams, shapes atomic.Int64
	outlines, identities, conversions                 atomic.Int64
}

var errProofPreparation = errors.New("proof attempted paragraph preparation or metrics")

func (g *proofServiceGuard) preparationCalls() [6]int64 {
	return [6]int64{g.loads.Load(), g.coverage.Load(), g.metrics.Load(), g.scales.Load(), g.streams.Load(), g.shapes.Load()}
}

type proofGuardFace struct {
	contracts.FontFace
	guard *proofServiceGuard
}

func (f *proofGuardFace) Metrics() (contracts.FontMetrics, error) {
	f.guard.metrics.Add(1)
	if f.guard.proofOnly.Load() {
		return contracts.FontMetrics{}, errProofPreparation
	}
	return f.FontFace.Metrics()
}
func (f *proofGuardFace) Scale(size dimen.DU) (contracts.FontScale, error) {
	f.guard.scales.Add(1)
	if f.guard.proofOnly.Load() {
		return nil, errProofPreparation
	}
	s, err := f.FontFace.Scale(size)
	if err != nil {
		return nil, err
	}
	return proofGuardScale{s, f.guard}, nil
}

type proofGuardScale struct {
	contracts.FontScale
	guard *proofServiceGuard
}

func (s proofGuardScale) ToDU(units int32) float64 {
	s.guard.conversions.Add(1)
	return s.FontScale.ToDU(units)
}
func unguardProofFace(f contracts.FontFace) contracts.FontFace {
	if wrapper, ok := f.(*proofGuardFace); ok {
		return wrapper.FontFace
	}
	return f
}
func (g *proofServiceGuard) Load(ctx context.Context, path string, resource *fontresource.Resource) (typography.FaceEntry, error) {
	g.loads.Add(1)
	if g.proofOnly.Load() {
		return typography.FaceEntry{}, errProofPreparation
	}
	f, err := (ot.Services{}).Load(ctx, path, resource)
	if err == nil {
		f.Face = &proofGuardFace{f.Face, g}
	}
	return f, err
}
func (g *proofServiceGuard) GlyphIndex(f contracts.FontFace, r rune) (uint32, error) {
	g.coverage.Add(1)
	if g.proofOnly.Load() {
		return 0, errProofPreparation
	}
	return (ot.Services{}).GlyphIndex(unguardProofFace(f), r)
}
func (g *proofServiceGuard) Identify(f contracts.FontFace) (typography.Info, error) {
	g.identities.Add(1)
	return (ot.Services{}).Identify(unguardProofFace(f))
}
func (g *proofServiceGuard) Outline(f contracts.FontFace, id uint32) (render.Outline, error) {
	g.outlines.Add(1)
	return (ot.Services{}).Outline(unguardProofFace(f), id)
}
func (*proofServiceGuard) VisualOrder(g []contracts.ShapedGlyph, d xbidi.Direction) ([]contracts.ShapedGlyph, error) {
	return (ot.Services{}).VisualOrder(g, d)
}

type proofGuardStream struct {
	contracts.StreamingShaper
	guard *proofServiceGuard
}

func (s proofGuardStream) Shape(ctx context.Context, p contracts.ShapeParams, text contracts.RuneReader, opts contracts.ShapeBufferOptions) error {
	s.guard.shapes.Add(1)
	if s.guard.proofOnly.Load() {
		return errProofPreparation
	}
	p.Font = unguardProofFace(p.Font)
	return s.StreamingShaper.Shape(ctx, p, text, opts)
}
func guardedRealProofConfig(t *testing.T) (documentlayout.Config, *proofServiceGuard) {
	t.Helper()
	g := new(proofServiceGuard)
	r, err := typography.LoadSettings(t.Context(), typography.Settings{DefaultPath: "../../opentype/testdata/fonts/GentiumPlus-R.ttf", Fallbacks: []string{"../../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf"}, Size: 15 * dimen.BP}, typography.Services{Loader: g, Coverage: g})
	if err != nil {
		t.Fatal(err)
	}
	return documentlayout.Config{Resolver: r, NewShaper: func() (contracts.StreamingShaper, error) {
		g.streams.Add(1)
		if g.proofOnly.Load() {
			return nil, errProofPreparation
		}
		s, err := ot.NewStream(64)
		return proofGuardStream{s, g}, err
	}}, g
}

func retainedAutoProof(t *testing.T, r *documentlayout.Result, guard *proofServiceGuard) (*documentlayout.Proof, []byte) {
	t.Helper()
	before := guard.preparationCalls()
	for _, calls := range before {
		if calls == 0 {
			t.Fatal("layout did not exercise every guarded service")
		}
	}
	outlines, identities, conversions := guard.outlines.Load(), guard.identities.Load(), guard.conversions.Load()
	guard.proofOnly.Store(true)
	defer guard.proofOnly.Store(false)
	services := render.Services{Backend: guard, Identity: guard}
	p, err := documentlayout.BuildProof(t.Context(), r, services)
	if err != nil {
		t.Fatal(err)
	}
	var svg bytes.Buffer
	if err := documentlayout.WriteSVG(t.Context(), &svg, r, services); err != nil {
		t.Fatal(err)
	}
	if guard.preparationCalls() != before {
		t.Fatal("rendering invoked layout services")
	}
	if guard.outlines.Load() <= outlines || guard.identities.Load() <= identities || guard.conversions.Load() <= conversions {
		t.Fatal("proof did not use retained render services")
	}
	return p, bytes.Clone(svg.Bytes())
}

func checkAutoProofCorrespondence(t *testing.T, r *documentlayout.Result, p *documentlayout.Proof) {
	t.Helper()
	if len(p.Atomics) != 6 {
		t.Fatal("missing or flattened nested atomic", len(p.Atomics))
	}
	seen := map[box.BoxID]bool{}
	geometry := r.Geometry()
	// Walk parent links independently, from child to root. The synthetic stage
	// 5B tests remain the numeric sizing oracle; here we verify transport of
	// retained real-font geometry, including anonymous content owners.
	origin := func(id box.BoxID) (float64, float64) {
		x, y := 0.0, 0.0
		for depth := 0; id != 0; depth++ {
			if depth > len(geometry) {
				t.Fatal("cyclic owner chain")
			}
			v, ok := geometry[id]
			if !ok {
				t.Fatal("missing owner")
			}
			x += float64(v.Content.X) * .75
			y += float64(v.Content.Y) * .75
			id = v.RelativeTo
		}
		return x, y
	}
	for _, a := range p.Atomics {
		if seen[a.BoxID] {
			t.Fatal("duplicate atomic occurrence")
		}
		seen[a.BoxID] = true
		g, ok := r.Group(a.GroupID)
		if !ok || g.Input.OwnerID != a.OwnerID {
			t.Fatal("wrong atomic owner")
		}
		found := false
		for _, source := range g.Input.AtomicSources {
			if source.ID == a.ID {
				found = true
				if source != a.AtomicSource || g.Input.Text[source.Start:source.End] != "\uFFFC" {
					t.Fatal("atomic source changed")
				}
			}
		}
		if !found || geometry[a.BoxID].RelativeTo != a.OwnerID {
			t.Fatal("atomic source/geometry mismatch")
		}
		x, y := origin(a.OwnerID)
		found = false
		for _, placement := range g.Paragraph.VList.AtomicPlacements() {
			if placement.ID == a.ID {
				found = true
				if a.X != x+placement.X || a.Y != y+placement.Y || a.Line != placement.Line {
					t.Fatal("atomic transform applied incorrectly")
				}
			}
		}
		if !found {
			t.Fatal("proof atomic lacked retained placement")
		}
	}
	fonts, colors := map[string]bool{}, map[string]bool{}
	sourceGroups := map[documentlayout.Locator]box.BoxID{}
	multibyte, hebrew, ligature := false, false, false
	for _, glyph := range p.Glyphs {
		g, _ := r.Group(glyph.GroupID)
		fonts[glyph.FaceSHA256] = true
		colors[glyph.Color] = true
		if glyph.Path == "" || glyph.GlyphID == 0 || glyph.OwnerID != g.Input.OwnerID || len(glyph.Sources) == 0 {
			t.Fatal("missing glyph/outline/owner/source", glyph)
		}
		if glyph.SourceByte >= uint64(len(g.Input.Text)) || !utf8.RuneStart(g.Input.Text[glyph.SourceByte]) {
			t.Fatal("invalid UTF-8 glyph anchor")
		}
		runeAt, _ := utf8.DecodeRuneInString(g.Input.Text[glyph.SourceByte:])
		multibyte = multibyte || runeAt == 'é'
		hebrew = hebrew || (runeAt >= 'א' && runeAt <= 'ת')
		if runeAt == '\uFFFC' {
			t.Fatal("atomic interior flattened into parent glyphs")
		}
		tx, ok := g.Paragraph.Frozen.CopyText(glyph.Knot)
		if !ok {
			t.Fatal("glyph lacks frozen text")
		}
		if _, ok := tx.Context.Params.Font.(*proofGuardFace); !ok {
			t.Fatal("retained face bypassed metric guard")
		}
		paintFound := false
		for _, paint := range g.Input.Paint {
			if paint.Start <= glyph.SourceByte && glyph.SourceByte < paint.End {
				paintFound = true
				if glyph.Color != paint.Paint.Color {
					t.Fatal("glyph paint lost source anchor")
				}
			}
		}
		if !paintFound {
			t.Fatal("glyph lacks retained paint")
		}
		for _, cluster := range tx.Clusters {
			if cluster.Input.ScalarEnd-cluster.Input.ScalarStart > uint64(cluster.GlyphCount) {
				ligature = true
			}
		}
		for _, source := range glyph.Sources {
			if !(source.Start <= glyph.SourceByte && glyph.SourceByte < source.End) {
				t.Fatal("glyph source outside original span")
			}
			if owner, seen := sourceGroups[source.Locator]; seen && owner != glyph.GroupID {
				t.Fatal("atomic interior text duplicated in another paragraph")
			}
			sourceGroups[source.Locator] = glyph.GroupID
			original, ok := r.Document().TextSource(boxtext.TextSourceID(source.Locator.NodeID + 1))
			if !ok || original.Locator != source.Locator {
				t.Fatal("lost original DOM text")
			}
			if source.Kind == "" {
				pos := source.NodeStart + glyph.SourceByte - source.Start
				if pos >= source.NodeEnd || pos >= uint64(len(original.Text)) {
					t.Fatal("invalid decoded DOM anchor")
				}
				decoded, _ := utf8.DecodeRuneInString(original.Text[pos:])
				if decoded != runeAt {
					t.Fatal("glyph anchor changed decoded source character")
				}
			}
		}
	}
	if len(fonts) != 2 || !colors["#991b1b"] || !colors["#172554"] || !multibyte || !hebrew || !ligature {
		t.Fatal("lost real glyph/source/paint coverage", fonts, colors, multibyte, hebrew, ligature)
	}
	lineCounts := map[box.BoxID]int{}
	for _, line := range p.Lines {
		g, _ := r.Group(line.GroupID)
		retained := g.Paragraph.VList.Lines()
		x, y := origin(line.OwnerID)
		n := lineCounts[line.GroupID]
		if n >= len(retained) {
			t.Fatal("duplicate proof line")
		}
		l := retained[n]
		lineCounts[line.GroupID]++
		if line.OwnerID != g.Input.OwnerID || line.Baseline != y+l.Baseline || line.Top != y+l.Top || line.X != x+l.X || line.Height != l.Height || line.Width != l.Horizontal.Width.Points() || line.TargetWidth != l.Horizontal.TargetWidth.Points() {
			t.Fatal("proof recomputed retained line geometry", line)
		}
	}
	for _, entry := range r.Groups() {
		g, _ := r.Group(entry.GroupID)
		if lineCounts[entry.GroupID] != len(g.Paragraph.VList.Lines()) {
			t.Fatal("missing proof line")
		}
	}
}

func mutateAutoProofAccessors(r *documentlayout.Result) {
	clear(r.Geometry())
	clear(r.Groups())
	for _, entry := range r.Groups() {
		g, _ := r.Group(entry.GroupID)
		clear(g.Input.Paint)
		clear(g.Input.Styles)
		clear(g.Input.TextSources)
		clear(g.Input.AtomicSources)
		clear(g.Input.Anchors)
		clear(g.Input.Isolates)
		clear(g.Input.LineLayout.Runs)
		clear(g.Paragraph.BreakPositions)
		clear(g.Paragraph.BidiRuns)
		clear(g.Paragraph.VList.Lines())
		clear(g.Paragraph.VList.Materials())
		clear(g.Paragraph.VList.AtomicPlacements())
		for k := 0; k < g.Paragraph.Frozen.KnotCount(); k++ {
			tx, ok := g.Paragraph.Frozen.CopyText(k)
			if ok {
				clear(tx.Glyphs)
				clear(tx.Clusters)
				clear(tx.Context.Params.Features)
			}
		}
	}
}

func TestRetainedNestedAutoRealProof(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			d, input, _ := build(t, nestedAutoFixture)
			cfg, guard := guardedRealProofConfig(t)
			var results []*documentlayout.Result
			var proofs []*documentlayout.Proof
			var svgs [][]byte
			checkOld := func() {
				t.Helper()
				for n, r := range results {
					p, svg := retainedAutoProof(t, r, guard)
					if !reflect.DeepEqual(p, proofs[n]) || !bytes.Equal(svg, svgs[n]) {
						t.Fatal("retained proof changed", n)
					}
				}
			}
			for _, width := range []float32{420, 100} {
				r := runDocument(t, d, width, breaker, cfg)
				checkOld()
				p, svg := retainedAutoProof(t, r, guard)
				checkAutoProofCorrespondence(t, r, p)
				results = append(results, r)
				proofs = append(proofs, p)
				svgs = append(svgs, svg)
			}
			changed := false
			for _, entry := range results[0].Groups() {
				g, _ := results[0].Group(entry.GroupID)
				if g.Input.Text == "office café encore" {
					changed = results[0].Geometry()[entry.OwnerID].Content.W > results[1].Geometry()[entry.OwnerID].Content.W
				}
			}
			if !changed {
				t.Fatal("nested auto fixture did not resize")
			}
			for _, cancellation := range []bool{false, true} {
				ctx, cancel := context.WithCancel(t.Context())
				bad := cfg
				calls := 0
				sentinel := errors.New("later real-font preparation failed")
				bad.NewShaper = func() (contracts.StreamingShaper, error) {
					calls++
					if calls == 2 {
						if cancellation {
							cancel()
						} else {
							return nil, sentinel
						}
					}
					return cfg.NewShaper()
				}
				out, err := d.Layout(ctx, documentlayout.LayoutRequest{Width: 100, Breaker: breaker}, bad)
				cancel()
				want := error(sentinel)
				if cancellation {
					want = context.Canceled
				}
				if calls != 2 || out != nil || !errors.Is(err, want) {
					t.Fatal("failed layout published output/lost cause", out, err)
				}
				checkOld()
			}
			clear(input.Nodes)
			*input = htmlinput.Document{}
			input = nil
			d = nil
			for _, r := range results {
				mutateAutoProofAccessors(r)
				checkOld()
			}
			recovered := runDocument(t, results[0].Document(), 420, breaker, cfg)
			checkOld()
			p, svg := retainedAutoProof(t, recovered, guard)
			if !reflect.DeepEqual(p, proofs[0]) || !bytes.Equal(svg, svgs[0]) {
				t.Fatal("fresh wide proof did not recover")
			}
		})
	}
}
