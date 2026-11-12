package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	"github.com/npillmayer/uax/bidi"
	xbidi "golang.org/x/text/unicode/bidi"
)

type guardedMetricFace struct {
	helpers.SyntheticFont
	deny bool
}

func (f *guardedMetricFace) Metrics() (contracts.FontMetrics, error) {
	if f.deny {
		return contracts.FontMetrics{}, errors.New("rendering must not query line metrics")
	}
	return f.SyntheticFont.Metrics()
}

type guardedStream struct {
	*helpers.SyntheticStreamingShaper
}

func (s guardedStream) Shape(ctx context.Context, p contracts.ShapeParams, r contracts.RuneReader, o contracts.ShapeBufferOptions) error {
	p.Font = helpers.NewSyntheticFont()
	return s.SyntheticStreamingShaper.Shape(ctx, p, r, o)
}

type emptyProofBackend struct{}

func (emptyProofBackend) Identify(contracts.FontFace) (typography.Info, error) {
	return typography.Info{}, nil
}
func (emptyProofBackend) Outline(contracts.FontFace, uint32) (render.Outline, error) {
	return render.Outline{}, nil
}
func (emptyProofBackend) VisualOrder(g []contracts.ShapedGlyph, _ xbidi.Direction) ([]contracts.ShapedGlyph, error) {
	return g, nil
}

func TestRenderingDoesNotRemeasureRetainedLines(t *testing.T) {
	for _, css := range []bool{false, true} {
		face := &guardedMetricFace{}
		text := styled.TextFromString("ab")
		config := khipu.Config{Streaming: &khipu.StreamingConfig{
			NewShaper: func() (contracts.StreamingShaper, error) {
				s, err := helpers.NewSyntheticStreamingShaper(8)
				return guardedStream{s}, err
			},
			Resolver: helpers.DefaultStyleResolver{}, Defaults: contracts.ResolvedRunStyle{Font: face, EmSize: 10 * dimen.BP},
		}}
		params := paragraph.Params{
			Constraints: paragraph.Constraints{Width: 100 * dimen.BP}, Breaker: paragraph.FirstFit,
			Assembly: paragraph.Options{Policy: paragraph.ProofCompatibility, DefaultEmSize: 10 * dimen.BP},
		}
		baseline, height := 8.0, 12.0
		if css {
			params.Assembly = paragraph.Options{Policy: paragraph.CSSLineHeight}
			params.LineLayout = &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}}
			baseline, height = 10.5, 15
		}
		frozen, err := paragraph.Run(t.Context(), paragraph.Input{Text: text, Direction: bidi.LeftToRight}, params, config)
		if err != nil {
			t.Fatal(err)
		}
		placements := frozen.VList
		r := &Result{vlist: placements, Frozen: frozen.Frozen, paintPolicy: typography.GlyphBoundary, ParagraphText: "ab", Renderer: emptyProofBackend{}, Identity: emptyProofBackend{}}
		face.deny = true
		proof, err := BuildGlyphProof(r)
		if err != nil || len(proof.Lines) != 1 || proof.Lines[0].Baseline != baseline || proof.Lines[0].Height != height {
			t.Fatal("renderer recomputed placement", proof, err)
		}
		// Missing/mismatched placement must not silently trigger reassembly.
		r.vlist = nil
		if _, err = BuildGlyphProof(r); err == nil {
			t.Fatal("silently rebuilt missing placements")
		}
		r.vlist = placements
		r.Frozen = &khipu.FrozenKhipu{}
		if _, err = BuildGlyphProof(r); err == nil {
			t.Fatal("accepted placements from another frozen result")
		}
	}
}
