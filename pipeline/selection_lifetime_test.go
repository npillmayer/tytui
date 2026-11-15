package pipeline_test

import (
	"github.com/npillmayer/khipu/dimen"
	"os"
	"path/filepath"
	"testing"
	"tytui/fontselection"
	"tytui/pipeline"
)

func TestLoadedSelectionSurvivesFileRemovalBeforeRun(t *testing.T) {
	data, err := os.ReadFile(gentium)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "selected.ttf")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	request := &fontselection.Request{Settings: fontselection.Settings{DefaultPath: path, Size: 12 * dimen.BP}}
	selected := selectFonts(t, request)
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	input := pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: selected}
	for i := 0; i < 2; i++ {
		r, err := pipeline.Run(t.Context(), input)
		if err != nil {
			t.Fatal("Run reopened selected file", err)
		}
		if len(r.FontSnapshot.Faces) != 1 || r.FontSnapshot.Default != selected.Fonts.Default {
			t.Fatal("selected identity lost")
		}
		if _, err = pipeline.BuildGlyphProof(r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = fontselection.Prepare(t.Context(), request); err == nil {
		t.Fatal("editable path was pinned forever")
	}
}
