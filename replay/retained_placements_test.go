package replay

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyPlacementEvidenceSurvivesResave(t *testing.T) {
	store := t.TempDir()
	for _, name := range []string{"GentiumPlus-R.ttf", "NotoSansHebrew-Regular.ttf"} {
		data, err := os.ReadFile(filepath.Join("../../opentype/testdata/fonts", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = put(store, digest(data), data); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"pre-step5-firstfit.json", "pre-step5-knuth-plass.json", "pre-step6-paint.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			original := readManifest(t, path)
			result, err := Load(t.Context(), path, store)
			if err != nil {
				t.Fatal(err)
			}
			if result.VList() == nil || result.VList().Frozen() != result.Frozen {
				t.Fatal("placements not rebuilt")
			}
			saved := filepath.Join(t.TempDir(), "resaved.json")
			if err = Save(t.Context(), result, saved, store, revisions()); err != nil {
				t.Fatal(err)
			}
			got := readManifest(t, saved)
			if got.Version != original.Version || got.Input.PaintPolicy != original.Input.PaintPolicy || !reflect.DeepEqual(got.Evidence, original.Evidence) || !reflect.DeepEqual(got.Bindings, original.Bindings) || !reflect.DeepEqual(got.Fonts, original.Fonts) {
				t.Fatal("legacy geometry, paint, identity or wire version changed")
			}
			data, err := os.ReadFile(saved)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{`"VList"`, `"Placements"`, `proof-compatibility-v1`} {
				if bytes.Contains(data, []byte(field)) {
					t.Fatalf("in-memory placement leaked into manifest: %s", field)
				}
			}
			again, err := Load(t.Context(), saved, store)
			if err != nil || again.VList().Box() != result.VList().Box() || !reflect.DeepEqual(again.VList().Lines(), result.VList().Lines()) {
				t.Fatal("resaved placements changed", err)
			}
		})
	}
}
