package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/pipeline"
)

func TestUnsupportedVersionsBeforeCurrentSchemaAndResources(t *testing.T) {
	for _, v := range []int{0, 1, 2, 3, 4, 6, 99} {
		t.Run(fmt.Sprint(v), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "case.json")
			// The old/unknown payload is deliberately not a valid current schema.
			data := fmt.Sprintf(`{"Version":%d,"Input":{"PaintPolicy":"style-boundary-v1"},"Fonts":[{"Info":{"SHA256":"bad"}}]}`, v)
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Load(t.Context(), path, "missing-store")
			if result != nil || err == nil || !strings.Contains(err.Error(), fmt.Sprintf("unsupported replay version %d", v)) {
				t.Fatalf("version did not fail first: %v", err)
			}
		})
	}
}

func TestCurrentFormatValidation(t *testing.T) {
	for _, css := range []bool{false, true} {
		mode := "normal"
		if css {
			mode = "css"
		}
		t.Run(mode, func(t *testing.T) {
			in := pipeline.Input{Text: "ab ab", Width: 15 * dimen.BP}
			if css {
				in.LineLayout = &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 2}}
			}
			result, err := pipeline.Run(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "case.json")
			if err := Save(t.Context(), result, path, filepath.Join(dir, "store"), revisions()); err != nil {
				t.Fatal(err)
			}
			good := readManifest(t, path)
			if good.Version != Version || good.Evidence.Layout == nil {
				t.Fatal("missing current placements/version")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"PaintPolicy"`) {
				t.Fatal("serialized paint selector")
			}
			for _, tc := range []struct {
				name, want string
				mutate     func(*Manifest)
			}{
				{"missing-mode", "assembly", func(m *Manifest) { m.Input.Assembly = "" }},
				{"unknown-mode", "assembly", func(m *Manifest) { m.Input.Assembly = "future" }},
				{"retired-mode", "assembly", func(m *Manifest) { m.Input.Assembly = "proof-compatibility-v1" }},
				{"missing-line-height", "assembly", func(m *Manifest) { m.Input.LineLayout = nil }},
				{"missing-placements", "placement evidence", func(m *Manifest) { m.Evidence.Layout = nil }},
				{"changed-box", "placements", func(m *Manifest) { m.Evidence.Layout.Extent++ }},
				{"changed-baseline", "placements", func(m *Manifest) { m.Evidence.Layout.Lines[0].Baseline++ }},
				{"changed-units", "units", func(m *Manifest) { m.Units = "pixels" }},
				{"unpinned-fonts", "unpinned", func(m *Manifest) { m.Input.Fonts = json.RawMessage(`{}`) }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					bad := readManifest(t, path)
					tc.mutate(&bad)
					badpath := filepath.Join(dir, tc.name+".json")
					rewrite(t, badpath, bad)
					if _, err := Load(t.Context(), badpath, "absent-store"); err == nil || !strings.Contains(err.Error(), tc.want) {
						t.Fatalf("expected %s error, got %v", tc.want, err)
					}
				})
			}
			// Strict current decoding must not accept a selector, even the current value.
			var obj map[string]any
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatal(err)
			}
			obj["Input"].(map[string]any)["PaintPolicy"] = "glyph-boundary-v1"
			extra, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			badpath := filepath.Join(dir, "selector.json")
			if err := os.WriteFile(badpath, extra, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(t.Context(), badpath, "absent-store"); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("accepted selector: %v", err)
			}
			delete(obj["Input"].(map[string]any), "PaintPolicy")
			placements := obj["Evidence"].(map[string]any)["Layout"].(map[string]any)["Lines"].([]any)
			placements[0].(map[string]any)["Gap"] = 0
			extra, err = json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(badpath, extra, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(t.Context(), badpath, "absent-store"); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("accepted removed Gap field: %v", err)
			}
		})
	}
}
