package pipeline

import (
	"context"
	"reflect"
	"testing"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	layout "github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"
)

func TestPaintCoalescingPreservesRelativeFeatureDomains(t *testing.T) {
	r, err := typography.LoadSettings(context.Background(), typography.Settings{DefaultPath: "../../opentype/testdata/fonts/ShaperTestProgress.ttf", Size: 12 * dimen.BP}, typography.Services{Loader: otadapter.Services{}, Coverage: otadapter.Services{}})
	if err != nil {
		t.Fatal(err)
	}
	defaults := r.Defaults()
	defaults.Features = []contracts.ShapeFeature{{Tag: [4]byte{'t', 'e', 's', 't'}, Enabled: true, Start: 2, End: 4}}
	red := style.Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "red", Language: "en", WhiteSpace: "normal", UnicodeBidi: "normal"}
	blue := red
	blue.Color = "blue"
	spans, err := typography.ProjectStyles("fifififi", []style.Span{{Start: 0, End: 4, Style: red}, {Start: 4, End: 8, Style: blue}}, r, defaults, typography.GlyphBoundary)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 {
		t.Fatal("relative feature origins coalesced")
	}
	for _, breaker := range []layout.Breaker{layout.FirstFit, layout.KnuthPlass} {
		text := styled.TextFromString("fifififi")
		for _, s := range spans {
			text, err = text.Style(s.Style, s.Start, s.End)
			if err != nil {
				t.Fatal(err)
			}
		}
		result, err := layout.Run(context.Background(), layout.Input{Text: text, Direction: bidi.LeftToRight}, layout.Params{
			Constraints: layout.Constraints{Width: 100 * dimen.BP}, Breaker: breaker,
			Assembly: layout.Options{Policy: layout.ProofCompatibility, DefaultEmSize: defaults.EmSize},
		}, khipu.Config{Streaming: &khipu.StreamingConfig{NewShaper: func() (contracts.StreamingShaper, error) { return otadapter.NewStream(8) }, Resolver: r, Defaults: defaults}})
		if err != nil {
			t.Fatal(err)
		}
		var ids, anchors []uint32
		for k := 0; k < result.Frozen.KnotCount(); k++ {
			if tx, ok := result.Frozen.CopyText(k); ok {
				for _, g := range tx.Glyphs {
					ids = append(ids, g.GlyphID)
					anchors = append(anchors, g.Cluster)
				}
			}
		}
		if !reflect.DeepEqual(ids, []uint32{4, 5, 6, 4, 5, 6}) || !reflect.DeepEqual(anchors, []uint32{0, 1, 2, 4, 5, 6}) {
			t.Fatalf("feature ranges moved across paint domains: %v %v", ids, anchors)
		}
	}
	if defaults.Features[0].Start != 2 || defaults.Features[0].End != 4 {
		t.Fatal("feature defaults mutated")
	}
}
