package replay

import (
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"path/filepath"
	"reflect"
	"testing"
	"tytui/pipeline"
)

func TestCurrentCSSPlacementsSurviveResave(t *testing.T) {
	for _, b := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		result, err := pipeline.Run(pipeline.Input{Text: "ab ab", Width: 15 * dimen.BP, Breaker: b})
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		store := filepath.Join(dir, "store")
		path := filepath.Join(dir, "first.json")
		if err := Save(result, path, store, revisions()); err != nil {
			t.Fatal(err)
		}
		original := readManifest(t, path)
		loaded, err := Load(path, store)
		if err != nil {
			t.Fatal(err)
		}
		loaded.Input.LineLayout = &paragraph.LineLayout{}
		saved := filepath.Join(dir, "second.json")
		if err := Save(loaded, saved, store, revisions()); err != nil {
			t.Fatal(err)
		}
		m := readManifest(t, saved)
		if m.Version != Version || m.Input.Assembly != paragraph.CSSLineHeight || m.Input.LineLayout == nil || !reflect.DeepEqual(m.Evidence, original.Evidence) || !reflect.DeepEqual(m.Input.LineLayout, original.Input.LineLayout) {
			t.Fatal("resave changed retained policy/evidence")
		}
		again, err := Load(saved, store)
		if err != nil || !reflect.DeepEqual(control(t, again), control(t, result)) {
			t.Fatal("load/save/load changed geometry", err)
		}
	}
}
