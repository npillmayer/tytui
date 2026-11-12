package pipeline_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	"github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"
	xbidi "golang.org/x/text/unicode/bidi"

	"tytui/pipeline"
)

// This renderer is deliberately limited to the synthetic one-glyph-per-scalar
// fixture. It supplies no metrics; expected origins below are hand calculated.
// Real shaping/mark order remains checked against the existing HarfBuzz fixtures.
type cellProof struct{}

func (cellProof) Identify(contracts.FontFace) (typography.Info, error) {
	return typography.Info{SHA256: "synthetic-cells"}, nil
}

func TestAssemblyFailureReturnsNoPartialResult(t *testing.T) {
	// Each line fits the finite coordinate range, but three 24000bp line boxes
	// do not. Freezing succeeds before the assembler rejects accumulated height.
	s := style.Style{EmSize: 20000 * dimen.BP, Language: "en", WhiteSpace: "normal", UnicodeBidi: "normal"}
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		r, err := pipeline.Run(t.Context(), pipeline.Input{Text: "a a a", Width: 10000 * dimen.BP, Breaker: breaker, Styles: []style.Span{{Start: 0, End: 5, Style: s}}})
		var stage *pipeline.StageError
		if r != nil || !errors.As(err, &stage) || stage.Stage != pipeline.StageAssemble {
			t.Fatalf("expected assembly failure without partial result: %v %v", r, err)
		}
	}
}
func (cellProof) Outline(contracts.FontFace, uint32) (render.Outline, error) {
	return render.Outline{Path: "M0 0 L1 1", Points: []render.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}}, nil
}
func (cellProof) VisualOrder(g []contracts.ShapedGlyph, d xbidi.Direction) ([]contracts.ShapedGlyph, error) {
	out := slices.Clone(g)
	if d == xbidi.RightToLeft {
		slices.Reverse(out)
	}
	return out, nil
}

func TestIndependentRetainedLinePlacements(t *testing.T) {
	// Synthetic 10bp: 5bp cells (including ordinary spaces), ascent 8, descent 2,
	// gap 2. Final fill occupies the trailing edge: left for an RTL paragraph.
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, tc := range []struct {
			name, html, want string
			width            int
			xs, ys           []float64
			baselines        []float64
		}{
			{"ltr", `<p dir="ltr" style="font-size:10pt">ab אב</p>`, "abבא", 100, []float64{0, 5, 15, 20}, []float64{8, 8, 8, 8}, []float64{8}},
			{"rtl", `<p dir="rtl" style="font-size:10pt">ab אב</p>`, "באab", 100, []float64{75, 80, 90, 95}, []float64{8, 8, 8, 8}, []float64{8}},
			{"ltr_isolate_digits", `<p dir="ltr" style="font-size:10pt">ab <bdi dir="rtl">אב 12</bdi></p>`, "ab12בא", 100, []float64{0, 5, 15, 20, 30, 35}, []float64{8, 8, 8, 8, 8, 8}, []float64{8}},
			{"rtl_isolate_digits", `<p dir="rtl" style="font-size:10pt"><bdi dir="ltr">ab 12</bdi> אב</p>`, "באab12", 100, []float64{60, 65, 75, 80, 90, 95}, []float64{8, 8, 8, 8, 8, 8}, []float64{8}},
			{"ltr_two_lines", `<p dir="ltr" style="font-size:10pt">ab אב ab אב</p>`, "abבאabבא", 25, []float64{0, 5, 15, 20, 0, 5, 15, 20}, []float64{8, 8, 8, 8, 20, 20, 20, 20}, []float64{8, 20}},
			{"rtl_two_lines", `<p dir="rtl" style="font-size:10pt">ab אב ab אב</p>`, "באabבאab", 25, []float64{0, 5, 15, 20, 0, 5, 15, 20}, []float64{8, 8, 8, 8, 20, 20, 20, 20}, []float64{8, 20}},
			{"rtl_discretionary", `<p dir="rtl" style="font-size:10pt">אב&shy;גד</p>`, "-באדג", 15, []float64{0, 5, 10, 5, 10}, []float64{8, 8, 8, 20, 20}, []float64{8, 20}},
			{"rtl_unbroken", `<p dir="rtl" style="font-size:10pt">אב&shy;גד</p>`, "דגבא", 100, []float64{80, 85, 90, 95}, []float64{8, 8, 8, 8}, []float64{8}},
		} {
			t.Run(string(breaker)+"/"+tc.name, func(t *testing.T) {
				e := styledInput(t, tc.html)
				r, err := pipeline.Run(t.Context(), pipeline.Input{Text: e.Text, Source: e, Width: dimen.DU(tc.width) * dimen.BP, Breaker: breaker})
				if err != nil {
					t.Fatal(err)
				}
				r.Renderer, r.Identity = cellProof{}, cellProof{}
				p, err := pipeline.BuildGlyphProof(r)
				if err != nil {
					t.Fatal(err)
				}
				var chars []rune
				var xs, ys, baselines []float64
				for _, g := range p.Glyphs {
					chars = append(chars, rune(g.GlyphID-1))
					xs = append(xs, g.X)
					ys = append(ys, g.Y)
					knot, _ := r.Frozen.Knot(g.Knot)
					snapshot, _ := r.Frozen.CopyText(g.Knot)
					sources, err := render.PaintSources(r.ParagraphText, knot, snapshot)
					if err != nil {
						t.Fatal(err)
					}
					original, _ := utf8.DecodeRuneInString(r.ParagraphText[sources[g.Cluster]:])
					want := rune(g.GlyphID - 1)
					if want == '-' {
						want = '\u00ad'
					}
					if original != want {
						t.Fatalf("glyph %U mapped to source %U", rune(g.GlyphID-1), original)
					}
				}
				for i, l := range p.Lines {
					baselines = append(baselines, l.Baseline)
					if l.X != 0 || l.Top != float64(12*i) || l.Height != 12 || l.TargetWidth != float64(tc.width) {
						t.Fatalf("line %d %+v", i, l)
					}
				}
				if string(chars) != tc.want || !reflect.DeepEqual(xs, tc.xs) || !reflect.DeepEqual(ys, tc.ys) || !reflect.DeepEqual(baselines, tc.baselines) {
					t.Fatalf("order=%q xs=%v ys=%v baselines=%v; want %q %v %v %v", string(chars), xs, ys, baselines, tc.want, tc.xs, tc.ys, tc.baselines)
				}
				v := r.VList()
				if v == nil || v.Box().Extent != float64(12*len(tc.baselines)) || v.Box().Baseline != 8 || v.Frozen() != r.Frozen {
					t.Fatal("missing retained geometry")
				}
			})
		}
	}
}

func TestRetainedPlacementsSurviveInputMutationAndSourceRemoval(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, policy := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
			for _, html := range []string{`<p style="font-size:13.3pt"><span style="color:red">f</span><span style="color:blue">i</span> אב</p>`, `<p></p>`} {
				dir := t.TempDir()
				path := filepath.Join(dir, "font.ttf")
				data, err := os.ReadFile(gentium)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				e := styledInput(t, html)
				fonts := &discovery.Resolution{Settings: typography.Settings{DefaultPath: path, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}}}
				r, err := pipeline.Run(t.Context(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, PaintPolicy: policy, Fonts: fonts})
				if err != nil {
					t.Fatal(err)
				}
				before, err := pipeline.BuildGlyphProof(r)
				if err != nil {
					t.Fatal(err)
				}
				var svg bytes.Buffer
				if err = pipeline.WriteSVG(&svg, r); err != nil {
					t.Fatal(err)
				}
				wantBox, wantLines := r.VList().Box(), r.VList().Lines()
				// Mutate input, retained inspection input and every exposed placement form.
				fonts.Size = dimen.BP
				e.Text = "changed"
				e.Styles = nil
				handle := r.VList()
				lines := handle.Lines()
				materials := handle.Materials()
				box := handle.Box()
				if len(lines) > 0 {
					lines[0].Baseline = -1
					materials[0].Extent = -1
				}
				box.Extent = -1
				*handle = paragraph.VList{}
				paints := r.PaintSpans()
				if len(paints) > 0 {
					paints[0].Paint.Color = "invalid"
				}
				r.Input = pipeline.Input{Direction: bidi.RightToLeft, PaintPolicy: "invalid"}
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				after, err := pipeline.BuildGlyphProof(r)
				if err != nil || !reflect.DeepEqual(before, after) || r.VList().Box() != wantBox || !reflect.DeepEqual(r.VList().Lines(), wantLines) {
					t.Fatal("retained proof changed", err)
				}
				var again bytes.Buffer
				if err = pipeline.WriteSVG(&again, r); err != nil || !bytes.Equal(svg.Bytes(), again.Bytes()) {
					t.Fatal("export consulted mutable input", err)
				}
			}
		}
	}
	if (*pipeline.Result)(nil).VList() != nil || (&pipeline.Result{}).VList() != nil {
		t.Fatal("absent placements")
	}
	if _, err := pipeline.BuildGlyphProof(&pipeline.Result{}); err == nil {
		t.Fatal("accepted incomplete result")
	}
}
