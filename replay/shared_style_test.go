package replay

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	typestyle "github.com/npillmayer/typography/style"

	"tytui/pipeline"
)

func TestSharedStylesOfflineReplay(t *testing.T) {
	s := typestyle.Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "red", Language: "en", WhiteSpace: "normal", UnicodeBidi: "normal"}
	original, err := pipeline.Run(context.Background(), pipeline.Input{Text: "éa", Width: 100 * dimen.BP, Styles: []typestyle.Span{{Start: 0, End: 3, Style: s}}, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: latin, Size: 10 * dimen.BP}}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, store := filepath.Join(dir, "case.json"), filepath.Join(dir, "fonts")
	if err = Save(context.Background(), original, path, store, revisions()); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), path, store)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original.Input.Styles, got.Input.Styles) || got.Input.Source != nil {
		t.Fatal("shared style replay changed")
	}
	a, err := pipeline.BuildGlyphProof(original)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pipeline.BuildGlyphProof(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("replayed proof changed")
	}
}

func TestCaptureUnversionedModules(t *testing.T) {
	root := t.TempDir()
	modules := []string{"tytui", "cords", "khipu", "uax", "opentype", "w3c-dom", "tree", "schuko", "fontfind", "typography"}
	for _, name := range modules {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := CaptureRevisions(context.Background(), filepath.Join(root, "tytui"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(modules) {
		t.Fatalf("revisions %+v", got)
	}
	for i, r := range got {
		if r.Module != modules[i] || r.Commit != "unversioned" || !r.Dirty {
			t.Fatalf("false provenance %+v", r)
		}
	}
	if err := os.Remove(filepath.Join(root, "typography", "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureRevisions(context.Background(), filepath.Join(root, "tytui")); err == nil {
		t.Fatal("missing module accepted")
	}
}
