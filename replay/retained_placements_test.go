package replay

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/npillmayer/typography/paragraph"
	"tytui/pipeline"
)

func TestCurrentFixturesSurviveResave(t *testing.T) {
	store := t.TempDir()
	data, err := os.ReadFile(latin)
	if err != nil {
		t.Fatal(err)
	}
	if err := put(store, digest(data), data); err != nil {
		t.Fatal(err)
	}
	for _, b := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		t.Run(string(b), func(t *testing.T) {
			path := filepath.Join("testdata", "current-fi-"+string(b)+".json")
			original := readManifest(t, path)
			result, err := Load(t.Context(), path, store)
			if err != nil {
				t.Fatal(err)
			}
			proof, err := pipeline.BuildGlyphProof(result)
			if err != nil {
				t.Fatal(err)
			}
			if len(proof.Glyphs) != 1 || proof.Glyphs[0].Color != "red" || proof.Glyphs[0].Cluster != 0 {
				t.Fatalf("current ligature changed: %+v", proof.Glyphs)
			}
			want := currentControls(t)[string(b)+"/compatibility/ligature"]
			if got := control(t, result); !reflect.DeepEqual(got, want) {
				t.Fatalf("fixture differs from step-1 control: %+v", got)
			}
			// Retained policies must be authoritative even if inspection metadata changes.

			result.Input.LineLayout = &paragraph.LineLayout{}
			saved := filepath.Join(t.TempDir(), "case.json")
			if err := Save(t.Context(), result, saved, store, revisions()); err != nil {
				t.Fatal(err)
			}
			m := readManifest(t, saved)
			if m.Version != Version || m.Input.Assembly != paragraph.ProofCompatibility || m.Input.LineLayout != nil || !reflect.DeepEqual(m.Evidence, original.Evidence) || !reflect.DeepEqual(m.Selection, original.Selection) || !reflect.DeepEqual(m.Fonts, original.Fonts) {
				t.Fatal("current resave changed layout or selection")
			}
			again, err := Load(t.Context(), saved, store)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(control(t, again), want) {
				t.Fatal("load/save/load changed retained proof")
			}
		})
	}
}
