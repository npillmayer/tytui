package replay

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	findadapter "github.com/npillmayer/typography/adapter/fontfind"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/uax/bidi"

	"tytui/htmlinput"
	"tytui/pipeline"
)

const latin = "../../opentype/testdata/fonts/GentiumPlus-R.ttf"
const hebrew = "../../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf"

func revisions() []Revision {
	return []Revision{{Module: "fixture", Commit: "test-revision", Dirty: true}}
}
func readManifest(t *testing.T, path string) Manifest {
	t.Helper()
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = json.Unmarshal(data, &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func rewrite(t *testing.T, path string, m Manifest) {
	t.Helper()
	data, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestOfflineRoundTrip(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, width := range []dimen.DU{20 * dimen.BP, 100 * dimen.BP} {
			t.Run(string(breaker)+width.String(), func(t *testing.T) {
				dir := t.TempDir()
				store := filepath.Join(dir, "fonts")
				manifest := filepath.Join(dir, "case.json")
				data, e := os.ReadFile(latin)
				if e != nil {
					t.Fatal(e)
				}
				path := filepath.Join(dir, "original.ttf")
				if e = os.WriteFile(path, data, 0600); e != nil {
					t.Fatal(e)
				}
				input := pipeline.Input{Text: "אב\u00adגד", Direction: bidi.RightToLeft, Width: width, Breaker: breaker, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: path, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}, Rules: []typography.Rule{{Weight: 700, Path: path}}}}}
				original, e := pipeline.Run(context.Background(), input)
				if e != nil {
					t.Fatal(e)
				}
				// Saving and rendering must not reread the file, even when it now contains junk.
				if e = os.WriteFile(path, []byte("replaced font"), 0600); e != nil {
					t.Fatal(e)
				}
				if e = Save(context.Background(), original, manifest, store, revisions()); e != nil {
					t.Fatal(e)
				}
				if e = os.Remove(path); e != nil {
					t.Fatal(e)
				}
				got, e := Load(context.Background(), manifest, store)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(original.FontBindings, got.FontBindings) {
					t.Fatal("bindings changed")
				}
				a, _ := pipeline.BuildGlyphProof(original)
				b, _ := pipeline.BuildGlyphProof(got)
				if !reflect.DeepEqual(a, b) {
					t.Fatal("proof changed")
				}
				m := readManifest(t, manifest)
				if len(m.Fonts) != 2 || len(m.Selection.Rules) != 1 || m.Selection.RequestedPath != path || !m.Revisions[0].Dirty {
					t.Fatal("lost unused primary, mapping, request or revision")
				}
				replacement := false
				for _, run := range m.Evidence.Runs {
					if run.Replacement && len(run.Glyphs) > 0 {
						replacement = true
						if run.SHA256 == "" || run.ResourceID == "" {
							t.Fatal("replacement lost font identity")
						}
					}
				}
				if replacement != (width == 20*dimen.BP) {
					t.Fatal("wrong discretionary evidence")
				}
				if e = Save(context.Background(), original, manifest, store, revisions()); e == nil {
					t.Fatal("overwrote case")
				}
			})
		}
	}
}
func TestHTMLAndSyntheticReplay(t *testing.T) {
	source := `<style>p{color:red}</style><p>hé <span style="font-size:150%">abc</span></p>`
	doc, e := htmlinput.Parse("original.html", strings.NewReader(source))
	if e != nil {
		t.Fatal(e)
	}
	id := -1
	for _, n := range doc.Nodes {
		if n.Name == "p" {
			id = n.ID
		}
	}
	extraction, e := doc.Extract(id)
	if e != nil {
		t.Fatal(e)
	}
	result, e := pipeline.Run(context.Background(), pipeline.Input{Text: extraction.Text, Source: extraction, Width: 100 * dimen.BP})
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "case.json")
	if e = Save(context.Background(), result, path, filepath.Join(dir, "fonts"), revisions()); e != nil {
		t.Fatal(e)
	}
	got, e := Load(context.Background(), path, filepath.Join(dir, "missing-store"))
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(got.Input.Source, extraction) || got.Input.Source.OriginalHTML != source {
		t.Fatal("HTML/styles/map lost")
	}
}
func TestReplayRejectsMissingAndTamperedResources(t *testing.T) {
	result, e := pipeline.Run(context.Background(), pipeline.Input{Text: "abc", Width: 100 * dimen.BP, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: latin, Size: 12 * dimen.BP}}})
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"missing", "bytes", "hash", "reference", "version", "proof", "html"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "case.json")
			store := filepath.Join(dir, "fonts")
			if e = Save(context.Background(), result, path, store, revisions()); e != nil {
				t.Fatal(e)
			}
			m := readManifest(t, path)
			switch kind {
			case "missing":
				os.Remove(filepath.Join(store, m.Fonts[0].Info.SHA256))
			case "bytes":
				os.WriteFile(filepath.Join(store, m.Fonts[0].Info.SHA256), []byte("corrupt"), 0600)
			case "hash":
				m.Fonts[0].Info.SHA256 = "../original.ttf"
			case "reference":
				m.Selection.Default = "unknown"
			case "version":
				m.Version++
			case "proof":
				m.Evidence.Proof.Glyphs[0].X++
			case "html":
				m.Input.Source = &htmlinput.Extraction{OriginalHTML: "tampered", DocumentID: "wrong"}
			}
			rewrite(t, path, m)
			if _, err := Load(context.Background(), path, store); err == nil {
				t.Fatal("accepted invalid replay")
			}
		})
	}
}
func TestReplayCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, "missing", "missing"); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestDescriptorReplayBypassesDiscovery(t *testing.T) {
	data, err := os.ReadFile(latin)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"font.ttf": &fstest.MapFile{Data: data}}
	sf := fontfind.ScalableFont{Name: "catalog"}
	sf.SetFS(fsys, "font.ttf")
	calls := 0
	search, err := findadapter.New([]findadapter.OfflineProvider{{Name: "offline", Find: func(context.Context, fontfind.Descriptor) (findadapter.Match, error) {
		calls++
		return findadapter.Match{Font: sf, Reason: "explicit substitute"}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := fontdiscovery.New(search, otadapter.Services{}, map[string]string{"serif": "catalog"})
	if err != nil {
		t.Fatal(err)
	}
	settings, _, err := discovery.ResolveSettings(context.Background(), fontdiscovery.DescriptorSettings{Default: fontdiscovery.FontRequest{Families: []string{"serif"}, AllowSubstitution: true}, Size: 12 * dimen.BP})
	if err != nil {
		t.Fatal(err)
	}
	result, err := pipeline.Run(context.Background(), pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: settings})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "case.json")
	store := filepath.Join(dir, "fonts")
	if err = Save(context.Background(), result, path, store, revisions()); err != nil {
		t.Fatal(err)
	}
	delete(fsys, "font.ttf")
	settings.DiscoveryPolicy.Providers[0] = "changed"
	settings.DescriptorInput.Default.Families[0] = "changed"
	got, err := Load(context.Background(), path, store)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.Input.Fonts.DiscoveryPolicy.Providers[0] != "offline" || got.Input.Fonts.DescriptorInput.Default.Families[0] != "serif" || !reflect.DeepEqual(got.FontDiagnostics, result.FontDiagnostics) {
		t.Fatal("replay resolved again or lost policy/diagnostics")
	}
}
