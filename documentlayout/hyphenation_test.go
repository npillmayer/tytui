package documentlayout_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/documentlayout"
)

func TestDocumentManualHyphenationRejected(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		byteOffset   int
	}{
		{"default", `<div id="root">ab&shy;cd</div>`, 2},
		{"explicit", `<div id="root" style="hyphens:manual;">ab&shy;cd</div>`, 2},
		{"literal and decoded byte offset", "<div id='root'> \né\u00adcd</div>", 4},
		{"inherit outside selection", `<main style="hyphens:manual"><div id="root"><span>ab&shy;cd</span></div></main>`, 2},
		{"manual overrides none", `<div id="root" style="hyphens:none"><span style="hyphens:manual">ab&shy;cd</span></div>`, 2},
		{"initial restores manual", `<div id="root" style="hyphens:none"><span style="hyphens:initial">ab&shy;cd</span></div>`, 2},
		{"unset inherits manual", `<div id="root" style="hyphens:manual"><span style="hyphens:unset">ab&shy;cd</span></div>`, 2},
		{"cascade priority", `<style>#root {hyphens:manual !important}</style><div id="root" style="hyphens:none">ab&shy;cd</div>`, 2},
		{"nested atomic interior", `<div id="root" style="hyphens:none"><span style="display:inline-block"><span style="display:inline-block;hyphens:manual">ab&shy;cd</span></span></div>`, 2},
		{"marker only", `<div id="root">&shy;</div>`, 0},
		{"wide paragraph", `<div id="root" style="width:500px">ab&shy;cd</div>`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, root := parse(t, tc.source)
			var path string
			for _, node := range input.Nodes {
				if strings.ContainsRune(node.Text, '\u00ad') {
					path = node.Path
					if v, err := input.Property(node.ID, "hyphens"); err != nil || v != "manual" {
						t.Fatalf("fixture effective hyphens = %q, %v", v, err)
					}
				}
			}
			if path == "" {
				t.Fatal("fixture has no authored soft hyphen")
			}
			d, err := documentlayout.Build(t.Context(), input, root)
			if d != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
				t.Fatalf("manual soft hyphen must reject the whole document: document=%v, err=%v", d != nil, err)
			}
			for _, want := range []string{path, fmt.Sprintf("byte %d", tc.byteOffset), "U+00AD", "hyphens:manual", "NoHyphenation", "hyphens:none"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("policy error %q lacks %q", err, want)
				}
			}
		})
	}
}

func TestDocumentHyphenationPolicyAcceptedInput(t *testing.T) {
	for _, tc := range []struct{ name, source, text string }{
		{"default without marker", `<div id="root">ab cd</div>`, "ab cd"},
		{"manual without marker", `<div id="root" style="hyphens:manual">ab-cd</div>`, "ab-cd"},
		{"none", `<div id="root" style="hyphens:none">ab&shy;cd</div>`, "ab\u00adcd"},
		{"whitespace and multibyte source", "<div id='root' style='hyphens:none'> \né&shy;cd\n </div>", "é\u00adcd"},
		{"inherit outside selection", `<main style="hyphens:none"><div id="root"><span>ab&shy;cd</span></div></main>`, "ab\u00adcd"},
		{"inherit keyword", `<div id="root" style="hyphens:none"><span style="hyphens:inherit">ab&shy;cd</span></div>`, "ab\u00adcd"},
		{"unset inherits none", `<div id="root" style="hyphens:none"><span style="hyphens:unset">ab&shy;cd</span></div>`, "ab\u00adcd"},
		{"none overrides manual", `<div id="root" style="hyphens:manual">a<span style="hyphens:none">b&shy;c</span>d</div>`, "ab\u00adcd"},
		{"cascade priority", `<style>#root {hyphens:none !important}</style><div id="root" style="hyphens:manual">ab&shy;cd</div>`, "ab\u00adcd"},
		{"last declaration", `<div id="root" style="hyphens:manual;hyphens:none;">ab&shy;cd</div>`, "ab\u00adcd"},
		{"unselected marker", `<main>ab&shy;cd<div id="root">selected</div></main>`, "selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := build(t, tc.source)
			groups := extractAll(t, d)
			if len(groups) != 1 || groups[0].Text != tc.text {
				t.Fatal("accepted policy changed text", groups)
			}
			checkSource(t, d, groups)
		})
	}
	input, root := parse(t, `<div id="root" style="hyphens:auto">abcd</div>`)
	if d, err := documentlayout.Build(t.Context(), input, root); d != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
		t.Fatalf("automatic hyphenation remains unsupported: document=%v, err=%v", d != nil, err)
	}
}

func TestDocumentHyphenationPolicyLeavesParagraphExtractionUnchanged(t *testing.T) {
	input, root := parse(t, `<div id="root">ab&shy;cd</div>`)
	if d, err := documentlayout.Build(t.Context(), input, root); d != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
		t.Fatalf("document policy was not enforced: document=%v, err=%v", d != nil, err)
	}
	// The original paragraph path has its own discretionary policy. A rejected
	// document build must neither impose this profile there nor mutate its input.
	p, err := input.Extract(root)
	if err != nil || p.Text != "ab\u00adcd" {
		t.Fatalf("paragraph extraction changed: result=%v, err=%v", p, err)
	}
}

// Fixed width exercises final breaking; shrink-to-fit also exercises intrinsic
// measurement. Both must ignore the explicitly disabled discretionary break.
func TestDocumentNoHyphenationLayout(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		width         float32
		fit           khipu.FrozenFit
	}{
		{"fixed", "ab&shy;cd", 30, khipu.FitOverfull},
		{"auto atomic", `<span style="display:inline-block">ab&shy;cd</span>`, 40, khipu.FitWithinCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := build(t, `<div id="root" style="font-size:20px;hyphens:none;color:blue">`+tc.content+`</div>`)
			for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
				t.Run(string(breaker), func(t *testing.T) {
					r := runDocument(t, d, 30, breaker, layoutConfig(t))
					groups := r.Groups()
					g, ok := r.Group(groups[len(groups)-1].GroupID)
					if !ok || g.Input.Text != "ab\u00adcd" || g.Paragraph.Frozen.LineCount() != 1 {
						t.Fatal("lost text or broke at a disabled soft hyphen", g)
					}
					if geom := r.Geometry()[g.Input.OwnerID]; geom.Content.W != tc.width {
						t.Fatal("intrinsic/final width disagreement", geom)
					}
					f := g.Paragraph.Frozen
					line, _ := f.Line(0)
					// Synthetic advances: four visible glyphs at 10px each. The
					// marker contributes no width; final fill cannot absorb overflow.
					if line.Width != 40*49152 || line.TargetWidth != dimen.DU(tc.width*49152) || line.Fit != tc.fit || line.Source != (khipu.ByteRange{End: 6}) {
						t.Fatal("disabled-marker geometry/provenance", line)
					}
					reader, err := f.SourceReader(khipu.ByteRange{End: 6})
					if err != nil {
						t.Fatal(err)
					}
					text, err := io.ReadAll(reader)
					if err != nil || string(text) != g.Input.Text {
						t.Fatalf("frozen source = %q, %v", text, err)
					}
					for i := range f.KnotCount() {
						knot, _ := f.Knot(i)
						if knot.Material == khipu.PreBreakMaterial || knot.Material == khipu.PostBreakMaterial {
							t.Fatal("selected a disabled discretionary replacement", knot)
						}
					}
					checkSource(t, d, []*documentlayout.InlineGroup{&g.Input})
					proof, err := documentlayout.BuildProof(t.Context(), r, cells())
					if err != nil {
						t.Fatal(err)
					}
					if len(proof.Glyphs) != 4 {
						t.Fatal("soft hyphen became visible", proof.Glyphs)
					}
					for i, glyph := range proof.Glyphs {
						if glyph.GlyphID != uint32("abcd"[i])+1 || glyph.SourceByte != []uint64{0, 1, 4, 5}[i] || glyph.Color != "blue" {
							t.Fatal("glyph, source or bypass paint changed", glyph)
						}
					}
				})
			}
		})
	}
}
