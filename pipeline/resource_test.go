package pipeline_test

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/fontfind/locate/fallbackfont"
	"github.com/npillmayer/khipu/dimen"
	"tytui/fontbackend"
	"tytui/pipeline"
)

func TestOfflineResourceProof(t *testing.T) {
	data, err := os.ReadFile(gentium)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"same.ttf": &fstest.MapFile{Data: data}}
	sf := fontfind.ScalableFont{Name: "Gentium"}
	sf.SetFS(fsys, "same.ttf")
	resource, err := fontbackend.FontfindResource(sf, "fixture", fontfind.Descriptor{Pattern: "Gentium"})
	if err != nil {
		t.Fatal(err)
	}
	// Removing the resource filesystem entry cannot affect its immutable snapshot.
	delete(fsys, "same.ttf")
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		input := pipeline.Input{Text: "office café", Width: 100 * dimen.BP, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}}
		file, err := pipeline.Run(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		input.Fonts = &fontbackend.Settings{DefaultResource: resource, Size: 12 * dimen.BP}
		resolved, err := pipeline.Run(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		a, err := pipeline.BuildGlyphProof(file)
		if err != nil {
			t.Fatal(err)
		}
		b, err := pipeline.BuildGlyphProof(resolved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatal("resource changed physical proof")
		}
		if resolved.Fonts[0].SHA256 != file.Fonts[0].SHA256 || resolved.Fonts[0].ResourceID == file.Fonts[0].ResourceID {
			t.Fatal("content/provenance identities confused")
		}
		for _, binding := range resolved.FontBindings {
			if binding.ResourceID != resolved.Fonts[0].ResourceID || binding.Origin.Provider != "fixture" {
				t.Fatalf("binding %+v", binding)
			}
		}
		var svg bytes.Buffer
		if err := pipeline.WriteGlyphSVG(&svg, resolved); err != nil || svg.Len() == 0 {
			t.Fatalf("export: %v", err)
		}
	}
}

func TestResourceMappingAndFallback(t *testing.T) {
	load := func(path string) *fontbackend.Resource {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := fontbackend.ReadResource(bytes.NewReader(data), fontbackend.Origin{Provider: "fixture", Location: "same.ttf"})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	latin, hebrewResource := load(gentium), load(hebrew)
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		e := styledInput(t, `<p dir="rtl">אב<span style="font-weight:bold">ab</span></p>`)
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultResource: latin, Size: 12 * dimen.BP, FallbackResources: []*fontbackend.Resource{hebrewResource}, Rules: []fontbackend.Rule{{Weight: 700, Resource: latin}}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Fonts) != 2 || r.Fonts[0].ResourceID == r.Fonts[1].ResourceID || r.Fonts[0].SHA256 == r.Fonts[1].SHA256 {
			t.Fatalf("colliding resources: %+v", r.Fonts)
		}
		if len(r.FontBindings) != 2 || r.FontBindings[0].SHA256 == r.FontBindings[1].SHA256 {
			t.Fatalf("bindings: %+v", r.FontBindings)
		}
		if _, err := pipeline.BuildGlyphProof(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPackagedFontResource(t *testing.T) {
	// Use the explicit matched packaged face; no discovery or fallback substitution.
	sf, err := fallbackfont.FindFallbackFont("Gentium", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := fontbackend.FontfindResource(sf, "embedded", fontfind.Descriptor{Pattern: "Gentium"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: &fontbackend.Settings{DefaultResource: resource, Size: 12 * dimen.BP}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.BuildGlyphProof(r); err != nil {
		t.Fatal(err)
	}
}
