package replay

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/style"

	"tytui/pipeline"
)

func TestRenderingBypassReplayKeepsWireColorsAndPolicies(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, policy := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
			t.Run(string(breaker)+"/"+string(policy), func(t *testing.T) {
				red := style.Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "red", Language: "en", WhiteSpace: "normal", UnicodeBidi: "normal"}
				blue := red
				blue.Color = "blue"
				in := pipeline.Input{Text: "\nfi\n", Width: 100 * dimen.BP, Breaker: breaker, PaintPolicy: policy,
					Styles: []style.Span{{Start: 0, End: 1, Style: red}, {Start: 1, End: 2, Style: blue}},
					Fonts:  &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: latin, Size: 12 * dimen.BP}}}
				original, err := pipeline.Run(context.Background(), in)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				path, store := filepath.Join(dir, "case.json"), filepath.Join(dir, "fonts")
				if err := Save(context.Background(), original, path, store, revisions()); err != nil {
					t.Fatal(err)
				}
				m := readManifest(t, path)
				wantVersion := 2
				if policy == typography.LegacyStyleBoundary {
					wantVersion = 1
				}
				if m.Version != wantVersion || !reflect.DeepEqual(m.Input.Styles, in.Styles) {
					t.Fatal("bypass changed replay version or serialized document colors")
				}
				got, err := Load(context.Background(), path, store)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got.PaintSpans(), original.PaintSpans()) || got.Input.PaintPolicy != policy {
					t.Fatal("replay failed to rebuild separate rendering input")
				}
				if got.VList() == nil || got.VList().Box() != original.VList().Box() || !reflect.DeepEqual(got.VList().Lines(), original.VList().Lines()) {
					t.Fatal("replay failed to rebuild retained placements")
				}
				a, _, _ := got.Text.StyleAt(0)
				b, _, _ := got.Text.StyleAt(1)
				if a.Equals(b) != (policy == typography.GlyphBoundary) {
					t.Fatal("replay changed legacy/modern shaping domains")
				}
				before, err := pipeline.BuildGlyphProof(original)
				if err != nil {
					t.Fatal(err)
				}
				// The rebuilt renderer snapshot must no longer consult wire styles.
				got.Input.Styles = nil
				after, err := pipeline.BuildGlyphProof(got)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("separate replayed paint changed proof: %v", err)
				}
			})
		}
	}
}
