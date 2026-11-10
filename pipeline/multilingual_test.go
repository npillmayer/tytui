package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"tytui/fontbackend"
	"tytui/pipeline"
)

const hebrew = "../../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf"

func TestIndependentGlyphProofs(t *testing.T) {
	var corpus struct {
		Cases []struct {
			ID, Text string
			FontPath string `json:"font_path"`
			Hash     string `json:"font_sha256"`
			Fixture  struct {
				Output []struct {
					G, Cl          uint32
					DX, DY, AX, AY int32
				}
			}
		}
	}
	data, err := os.ReadFile("../../opentype/internal/hbcmp/testdata/text/baselines.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"latin_office_liga_on": true, "latin_cafe_nfd": true, "hebrew_shalom_pointed": true, "hebrew_shin_pointed": true}
	count := 0
	for _, c := range corpus.Cases {
		if !wanted[c.ID] {
			continue
		}
		count++
		dir := bidi.LeftToRight
		if strings.HasPrefix(c.ID, "hebrew") {
			dir = bidi.RightToLeft
		}
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: c.Text, Direction: dir, Width: 200 * dimen.BP, Fonts: &fontbackend.Settings{DefaultPath: "../../opentype/" + c.FontPath, Size: 18 * dimen.BP}})
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		if r.Fonts[0].SHA256 != c.Hash {
			t.Fatal("baseline font changed")
		}
		p, err := pipeline.BuildGlyphProof(r)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Glyphs) != len(c.Fixture.Output) {
			t.Fatalf("%s: count %d want %d", c.ID, len(p.Glyphs), len(c.Fixture.Output))
		}
		pen := p.Glyphs[0].X - float64(c.Fixture.Output[0].DX)*p.Glyphs[0].Scale
		penY := p.Lines[0].Baseline
		for i, w := range c.Fixture.Output {
			g := p.Glyphs[i]
			if g.GlyphID != w.G || g.Cluster != w.Cl {
				t.Fatalf("%s glyph %d %+v want %+v", c.ID, i, g, w)
			}
			if math.Abs(g.X-(pen+float64(w.DX)*g.Scale)) > 1e-7 || math.Abs(g.Y-(penY-float64(w.DY)*g.Scale)) > 1e-7 {
				t.Fatalf("%s glyph %d positioning %+v", c.ID, i, g)
			}
			pen += float64(w.AX) * g.Scale
			penY -= float64(w.AY) * g.Scale
		}
		var svg bytes.Buffer
		if err = pipeline.WriteSVG(&svg, r); err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(&svg)
		for {
			_, err = decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if count != 4 {
		t.Fatal("missing baselines")
	}
}
func TestMixedScriptsFallbackAndProof(t *testing.T) {
	e := styledInput(t, `<p dir="rtl" lang="he" style="font-size:18pt;color:navy">שָׁלוֹם <span lang="en" style="color:#b02020">office café</span> עולם</p>`)
	for _, direction := range []bidi.Direction{bidi.LeftToRight, bidi.RightToLeft} {
		e.Direction = direction
		for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
			r, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 18 * dimen.BP, Fallbacks: []string{hebrew}}})
			if err != nil {
				t.Fatal(err)
			}
			latin, hebr := false, false
			for _, b := range r.FontBindings {
				latin = latin || b.Script == "Latn"
				hebr = hebr || (b.Script == "Hebr" && b.Path == hebrew)
			}
			if !latin || !hebr {
				t.Fatalf("itemization %+v", r.FontBindings)
			}
			p, err := pipeline.BuildGlyphProof(r)
			if err != nil {
				t.Fatal(err)
			}
			red, navy := false, false
			for _, g := range p.Glyphs {
				red = red || g.Color == "#b02020"
				navy = navy || g.Color == "navy"
				if g.Path == "" {
					t.Fatal("missing outline")
				}
			}
			if !red || !navy {
				t.Fatal("style color lost")
			}
			for i, l := range p.Lines {
				f, _ := r.Frozen.Line(i)
				if l.Width != f.Width.Points() || l.TargetWidth != f.TargetWidth.Points() {
					t.Fatal("geometry disagrees")
				}
			}
			var first, again bytes.Buffer
			if err = pipeline.WriteSVG(&first, r); err != nil {
				t.Fatal(err)
			}
			r.Input.Fonts.DefaultPath = "missing.ttf"
			if err = pipeline.WriteSVG(&again, r); err != nil || !bytes.Equal(first.Bytes(), again.Bytes()) {
				t.Fatal("proof reread file or changed")
			}
		}
	}
}
func TestSoftHyphenFrozenProof(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "office\u00adoffice", Width: 35 * dimen.BP, Breaker: breaker, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}})
		if err != nil {
			t.Fatal(err)
		}
		replacement := false
		for i := 0; i < r.Frozen.KnotCount(); i++ {
			k, _ := r.Frozen.Knot(i)
			replacement = replacement || k.Material == khipu.PreBreakMaterial
		}
		if !replacement || r.Frozen.LineCount() < 2 {
			t.Fatalf("discretionary not selected (%s)", breaker)
		}
		if _, err = pipeline.BuildGlyphProof(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProofErrorsAndGraphemeStyleBoundary(t *testing.T) {
	e := styledInput(t, `<p>a<b>́</b></p>`)
	_, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Source: e, Width: 100 * dimen.BP, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}})
	if err == nil || !strings.Contains(err.Error(), "grapheme") {
		t.Fatalf("split grapheme accepted: %v", err)
	}
	r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: &fontbackend.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}})
	if err != nil {
		t.Fatal(err)
	}
	if err = pipeline.WriteSVG(nil, r); err == nil {
		t.Fatal("nil writer accepted")
	}
	sentinel := fmt.Errorf("write failed")
	if err = pipeline.WriteGlyphSVG(failingWriter{sentinel}, r); err != sentinel {
		t.Fatalf("write error lost: %v", err)
	}
}
