package replay

import (
	"context"
	"github.com/npillmayer/typography"
	"os"
	"path/filepath"
	"testing"
	"tytui/pipeline"
)

func TestPaintPolicyVersions(t *testing.T) {
	store := t.TempDir()
	data, err := os.ReadFile(latin)
	if err != nil {
		t.Fatal(err)
	}
	if err = put(store, digest(data), data); err != nil {
		t.Fatal(err)
	}
	old, err := Load(context.Background(), "testdata/pre-step6-paint.json", store)
	if err != nil {
		t.Fatal(err)
	}
	if old.Input.PaintPolicy != typography.LegacyStyleBoundary {
		t.Fatal("old case reinterpreted")
	}
	p, err := pipeline.BuildGlyphProof(old)
	if err != nil || len(p.Glyphs) != 2 || p.Glyphs[0].Color != "red" || p.Glyphs[1].Color != "blue" {
		t.Fatalf("old paint changed %+v %v", p, err)
	}
	legacyPath := filepath.Join(t.TempDir(), "legacy.json")
	if err = Save(context.Background(), old, legacyPath, store, revisions()); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, legacyPath)
	if m.Version != 1 || m.Input.PaintPolicy != "" {
		t.Fatal("legacy save changed policy/version")
	}
	if _, err = Load(context.Background(), legacyPath, store); err != nil {
		t.Fatal(err)
	}
	input := old.Input
	input.PaintPolicy = typography.GlyphBoundary
	modern, err := pipeline.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	p, err = pipeline.BuildGlyphProof(modern)
	if err != nil || len(p.Glyphs) != 1 || p.Glyphs[0].Color != "red" {
		t.Fatalf("new ligature paint %+v %v", p, err)
	}
	newPath := filepath.Join(t.TempDir(), "modern.json")
	if err = Save(context.Background(), modern, newPath, store, revisions()); err != nil {
		t.Fatal(err)
	}
	m = readManifest(t, newPath)
	if m.Version != 2 || m.Input.PaintPolicy != typography.GlyphBoundary {
		t.Fatal("new policy not recorded")
	}
	if _, err = Load(context.Background(), newPath, store); err != nil {
		t.Fatal(err)
	}
	m.Input.PaintPolicy = ""
	rewrite(t, newPath, m)
	if _, err = Load(context.Background(), newPath, store); err == nil {
		t.Fatal("ambiguous version-2 paint accepted")
	}
	m.Input.PaintPolicy = typography.GlyphBoundary
	m.Version = 1
	rewrite(t, newPath, m)
	if _, err = Load(context.Background(), newPath, store); err == nil {
		t.Fatal("new policy accepted in old format")
	}
}
