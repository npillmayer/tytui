package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/paragraph"
	"tytui/pipeline"
)

func TestLineHeightReplayVersion3(t *testing.T) {
	for _, fonts := range []*discovery.Resolution{nil, {Settings: typography.Settings{DefaultPath: latin, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}}}} {
		for _, b := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
			for _, paint := range []typography.PaintPolicy{typography.GlyphBoundary, typography.LegacyStyleBoundary} {
				in := pipeline.Input{Text: "fi ab\u00adcd שָׁלוֹם", Width: 80 * dimen.BP, Breaker: b, PaintPolicy: paint, Fonts: fonts, LineLayout: &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}, Runs: []paragraph.LineHeightSpan{{Start: 1, End: 2, Height: paragraph.LineHeight{Kind: paragraph.LengthHeight, Length: 30 * dimen.BP}}}}}
				out, err := pipeline.Run(t.Context(), in)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "case.json")
				store := filepath.Join(dir, "fonts")
				// Replay policy is retained independently of mutable inspection Input.
				out.Input.LineLayout = nil
				if err = Save(t.Context(), out, path, store, revisions()); err != nil {
					t.Fatal(err)
				}
				m := readManifest(t, path)
				if m.Version != LineHeightVersion || m.Input.LineLayout == nil || m.Evidence.Layout == nil || m.Input.PaintPolicy != paint {
					t.Fatal("wrong version/policy")
				}
				loaded, err := Load(t.Context(), path, store)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(loaded.VList().Lines(), out.VList().Lines()) || loaded.VList().Box() != out.VList().Box() {
					t.Fatal("placement replay changed")
				}
				resave := filepath.Join(dir, "resave.json")
				if err = Save(t.Context(), loaded, resave, store, revisions()); err != nil {
					t.Fatal(err)
				}
				saved := readManifest(t, resave)
				if !reflect.DeepEqual(saved.Evidence, m.Evidence) || !reflect.DeepEqual(saved.Input.LineLayout, m.Input.LineLayout) || saved.Version != 3 {
					t.Fatal("resave changed")
				}
				for i, mutate := range []func(*Manifest){
					func(m *Manifest) { m.Version = 2 },
					func(m *Manifest) { m.Input.LineLayout = nil },
					func(m *Manifest) { m.Input.PaintPolicy = "" },
					func(m *Manifest) { m.Input.LineLayout.Default.Factor = 4 },
					func(m *Manifest) { m.Evidence.Layout = nil },
				} {
					bad := readManifest(t, path)
					mutate(&bad)
					data, e := json.Marshal(bad)
					if e != nil {
						t.Fatal(e)
					}
					name := filepath.Join(dir, "bad.json")
					if e = os.WriteFile(name, data, 0600); e != nil {
						t.Fatal(e)
					}
					if _, e = Load(t.Context(), name, store); e == nil {
						t.Fatalf("accepted tampered case %d", i)
					}
				}
			}
		}
	}
}
