package replay

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/layout"
	"github.com/npillmayer/typography/render"
	"github.com/npillmayer/typography/style"

	"tytui/fontselection"
	"tytui/htmlinput"
	"tytui/pipeline"
)

// These controls describe current output, independently of the replay wire schema.
// SVG hashes capture outlines/order/color; numeric placements remain inspectable.
type currentControl struct {
	SVG   string
	Box   layout.Box
	Lines []LinePlacement
}

func control(t *testing.T, r *pipeline.Result) currentControl {
	t.Helper()
	var svg bytes.Buffer
	if err := pipeline.WriteSVG(&svg, r); err != nil {
		t.Fatal(err)
	}
	c := currentControl{SVG: digest(svg.Bytes()), Box: r.VList().Box()}
	for _, l := range r.VList().Lines() {
		c.Lines = append(c.Lines, LinePlacement{l.X, l.Top, l.Baseline, l.Ascent, l.Descent, l.Height})
	}
	return c
}

func currentControls(t *testing.T) map[string]currentControl {
	t.Helper()
	data, err := os.ReadFile("testdata/current-controls.json")
	if err != nil {
		t.Fatal(err)
	}
	var controls map[string]currentControl
	if err := json.Unmarshal(data, &controls); err != nil {
		t.Fatal(err)
	}
	return controls
}

func TestCurrentBehaviorReplayControls(t *testing.T) {
	controls := currentControls(t)
	seen := 0
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, tc := range []struct {
			name, html string
			width      int
		}{
			{"ligature", `<p style="font-size:12pt"><span style="color:red">f</span><span style="color:blue">i</span></p>`, 100},
			{"mixed_marks", `<p dir="rtl" style="font-size:12pt;color:navy">שָׁלוֹם <span dir="ltr" style="color:red">office café</span> אב</p>`, 100},
			{"selected_shy", `<p dir="rtl" style="font-size:12pt"><span style="color:red">אב</span><span style="color:blue">&shy;</span><span style="color:green">גד</span></p>`, 20},
			{"unbroken_shy", `<p dir="rtl" style="font-size:12pt"><span style="color:red">אב</span><span style="color:blue">&shy;</span><span style="color:green">גד</span></p>`, 100},
		} {
			name := string(breaker) + "/css/" + tc.name
			t.Run(name, func(t *testing.T) {
				doc, err := htmlinput.Parse("current.html", strings.NewReader(tc.html))
				if err != nil {
					t.Fatal(err)
				}
				id := -1
				for _, n := range doc.Nodes {
					if n.Name == "p" {
						id = n.ID
					}
				}
				source, err := doc.Extract(id)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				paths := []string{filepath.Join(dir, "latin.ttf"), filepath.Join(dir, "hebrew.ttf")}
				for i, original := range []string{latin, hebrew} {
					data, err := os.ReadFile(original)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(paths[i], data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				in := pipeline.Input{Text: source.Text, Source: source, Width: dimen.DU(tc.width) * dimen.BP, Breaker: breaker, Fonts: selectFonts(t, &fontselection.Request{Settings: fontselection.Settings{DefaultPath: paths[0], Size: 12 * dimen.BP, Fallbacks: []string{paths[1]}, Rules: []fontselection.Rule{{Weight: 700, Path: paths[0]}}}})}
				in.LineLayout = &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}}
				r, err := pipeline.Run(t.Context(), in)
				if err != nil {
					t.Fatal(err)
				}
				before := control(t, r)
				want, ok := controls[name]
				if !ok || !reflect.DeepEqual(before, want) {
					t.Fatalf("current control changed: got %+v want %+v", before, want)
				}
				seen++
				proof, err := pipeline.BuildGlyphProof(r)
				if err != nil {
					t.Fatal(err)
				}
				if tc.name == "ligature" && (len(proof.Glyphs) != 1 || proof.Glyphs[0].Color != "red" || proof.Glyphs[0].Cluster != 0) {
					t.Fatalf("ligature split or recolored: %+v", proof.Glyphs)
				}
				if strings.HasSuffix(tc.name, "shy") {
					colors := map[string]int{}
					for _, g := range proof.Glyphs {
						colors[g.Color]++
						knot, _ := r.Frozen.Knot(g.Knot)
						if g.Color == "blue" {
							tx, ok := r.Frozen.CopyText(g.Knot)
							if !ok || !tx.IsReplacement {
								t.Fatal("blue glyph is not a selected replacement")
							}
							sources, err := render.PaintSources(r.ParagraphText, knot, tx)
							if err != nil || sources[g.Cluster] != 4 {
								t.Fatalf("replacement lost marker source: %v %v", sources, err)
							}
						}
					}
					blue := 0
					if tc.name == "selected_shy" {
						blue = 1
					}
					if colors["red"] != 2 || colors["green"] != 2 || colors["blue"] != blue {
						t.Fatalf("unexpected discretionary colors: %v", colors)
					}
				}
				// Both active and unused configured resources must survive removal before Save.
				for _, path := range paths {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				path, store := filepath.Join(dir, "case.json"), filepath.Join(dir, "store")
				if err := Save(t.Context(), r, path, store, revisions()); err != nil {
					t.Fatal(err)
				}
				m := readManifest(t, path)
				if len(m.Fonts) != 2 || len(m.Selection.Rules) != 1 || len(m.Selection.Fallbacks) != 1 {
					t.Fatal("configured resource lost")
				}
				loaded, err := Load(t.Context(), path, store)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(loaded.Input.Source, r.Input.Source) || !reflect.DeepEqual(loaded.FontBindings, r.FontBindings) {
					t.Fatal("source/style mapping or font identity changed")
				}
				if got := control(t, loaded); !reflect.DeepEqual(got, before) {
					t.Fatal("replay changed current proof/placements")
				}
				// Rendering uses retained paint, fonts and placements, not inspection input.
				loaded.Input.Source = nil
				loaded.Input.Styles = nil
				if got := control(t, loaded); !reflect.DeepEqual(got, before) {
					t.Fatal("rendering consulted mutable source")
				}
				if got := control(t, r); !reflect.DeepEqual(got, before) {
					t.Fatal("save/render mutated original")
				}
			})
		}
	}
	expected := 0
	for name := range controls {
		if strings.Contains(name, "/css/") {
			expected++
		}
	}
	if seen != expected {
		t.Fatalf("exercised %d controls, want %d CSS controls", seen, expected)
	}
}

func TestCurrentSyntheticReplayPlacements(t *testing.T) {
	for _, b := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		for _, css := range []bool{false, true} {
			in := pipeline.Input{Text: "ab ab", Styles: []style.Span{{Start: 0, End: 5, Style: style.Style{EmSize: 10 * dimen.BP, Language: "en", WhiteSpace: "normal", UnicodeBidi: "normal"}}}, Width: 10 * dimen.BP, Breaker: b}
			height, baseline := 10.0, 8.0 // CSS normal: 8bp ascent and 2bp descent.
			if css {
				in.LineLayout = &paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 2}}
				height, baseline = 20, 13
			}
			r, err := pipeline.Run(t.Context(), in)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "case.json")
			if err := Save(t.Context(), r, path, filepath.Join(dir, "store"), revisions()); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(t.Context(), path, filepath.Join(dir, "absent-store"))
			if err != nil {
				t.Fatal(err)
			}
			for _, result := range []*pipeline.Result{r, loaded} {
				lines := result.VList().Lines()
				if len(lines) != 2 || result.VList().Box().Extent != 2*height {
					t.Fatalf("unexpected synthetic extent: %+v", result.VList().Box())
				}
				for i, l := range lines {
					if l.Top != float64(i)*height || l.Height != height || l.Baseline != float64(i)*height+baseline {
						t.Fatalf("unexpected line: %+v", l)
					}
				}
			}
		}
	}
}
