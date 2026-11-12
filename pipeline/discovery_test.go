package pipeline_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"testing/fstest"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	findadapter "github.com/npillmayer/typography/adapter/fontfind"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/uax/bidi"

	"tytui/pipeline"
)

func TestDescriptorCoverageThroughFreezing(t *testing.T) {
	catalog := map[string]fontfind.ScalableFont{}
	for name, path := range map[string]string{"Latin": gentium, "Hebrew": hebrew} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sf := fontfind.ScalableFont{Name: name}
		sf.SetFS(fstest.MapFS{"font.ttf": &fstest.MapFile{Data: data}}, "font.ttf")
		catalog[name] = sf
	}
	search, err := findadapter.New([]findadapter.OfflineProvider{{Name: "offline", Find: func(_ context.Context, d fontfind.Descriptor) (findadapter.Match, error) {
		sf, ok := catalog[d.Pattern]
		if !ok {
			return findadapter.Match{Font: catalog["Latin"], Reason: "explicit catalog default"}, fmt.Errorf("missing %s", d.Pattern)
		}
		return findadapter.Match{Font: sf, Exact: true}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := fontdiscovery.New(search, otadapter.Services{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings, diagnostics, err := session.ResolveSettings(context.Background(), fontdiscovery.DescriptorSettings{Default: fontdiscovery.FontRequest{Families: []string{"Absent"}, AllowSubstitution: true}, Fallbacks: []fontdiscovery.FontRequest{{Families: []string{"Hebrew"}}}, Rules: []fontdiscovery.DescriptorRule{{Weight: 700, Request: fontdiscovery.FontRequest{Families: []string{"Latin"}}}}, Size: 12 * dimen.BP})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics[0].Message = "caller mutation"
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, width := range []dimen.DU{20 * dimen.BP, 100 * dimen.BP} {
			r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "אב\u00adגד", Direction: bidi.RightToLeft, Width: width, Breaker: breaker, Fonts: settings})
			if err != nil {
				t.Fatal(err)
			}
			wantLines := 1
			if width == 20*dimen.BP {
				wantLines = 2
			}
			if r.Frozen.LineCount() != wantLines {
				t.Fatalf("%s lines %d", breaker, r.Frozen.LineCount())
			}
			if len(r.FontDiagnostics) == 0 || r.FontDiagnostics[0].Message == "caller mutation" || !r.FontDiagnostics[0].Substituted {
				t.Fatal("lost discovery diagnostics")
			}
			for _, b := range r.FontBindings {
				if b.Origin.Name != "Hebrew" {
					t.Fatalf("coverage binding %+v", b)
				}
			}
			replacement := false
			for k := 0; k < r.Frozen.KnotCount(); k++ {
				knot, _ := r.Frozen.Knot(k)
				replacement = replacement || knot.Material == khipu.PreBreakMaterial
			}
			if replacement != (wantLines == 2) {
				t.Fatal("lost discretionary")
			}
			if _, err := pipeline.BuildGlyphProof(r); err != nil {
				t.Fatal(err)
			}
			r.Input.Fonts.ResolutionDiagnostics[0].Message = "input mutation"
			if r.FontDiagnostics[0].Message == "input mutation" || settings.ResolutionDiagnostics[0].Message == "input mutation" {
				t.Fatal("aliased retained diagnostics")
			}
		}
	}
}
