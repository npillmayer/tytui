package documentlayout_test

import (
	"os"
	"strings"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/documentlayout"
)

func TestInterparagraphWhitespaceFixture(t *testing.T) {
	source, err := os.ReadFile("testdata/interparagraph-whitespace.html")
	if err != nil {
		t.Fatal(err)
	}
	// Preserve /tmp/,b.html's contents; add only a selector for the test helper.
	d, _, _ := build(t, strings.Replace(string(source), "<body>", `<body id="root">`, 1))
	if len(d.Root().Children) != 3 || len(d.Groups()) != 3 {
		t.Fatalf("whitespace created extra blocks/groups: %d/%d", len(d.Root().Children), len(d.Groups()))
	}
	want := []string{"Café styled text and inherited text.", "אב English גד", "First line Second line"}
	for i, g := range extractAll(t, d) {
		if g.Text != want[i] {
			t.Fatalf("group %d text=%q", i, g.Text)
		}
	}
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 800, breaker, layoutConfig(t))
		geometry := r.Geometry()
		var y float32
		for _, child := range d.Root().Children {
			g := geometry[child.BoxID]
			if child.Box != box.BoxBlock || g.Frame.Y != y {
				t.Fatalf("extra interparagraph height: %+v, want Y=%g", g, y)
			}
			y += g.Frame.H + 10 // Authored paragraph margin; no anonymous line strut.
		}
		if geometry[d.Root().BoxID].Content.H != y {
			t.Fatal("leading/trailing whitespace added height")
		}
	}
}

func TestAnonymousWhitespacePreservesRealParagraphs(t *testing.T) {
	for _, display := range []string{"block", "flow-root", "inline-block"} {
		// Includes split/hoist whitespace inside an inline wrapper, NBSP, a real
		// empty paragraph, and an explicitly whitespace-only paragraph.
		source := `<div id="root" style="font-size:10px"><section style="display:` + display + `;width:100px"> 
<span> <p></p> \t <p> <span> </span> </p> </span>
<p>a <i>b</i> c</p> <span>&nbsp;</span> <p>d</p>
</section></div>`
		source = strings.ReplaceAll(source, `\t`, "\t")
		d, _, _ := build(t, source)
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			r := runDocument(t, d, 200, breaker, layoutConfig(t))
			empty, nbsp, words := 0, 0, 0
			for _, entry := range r.Groups() {
				g, _ := r.Group(entry.GroupID)
				switch g.Input.Text {
				case "":
					empty++
					if len(g.Paragraph.VList.Lines()) != 0 || r.Geometry()[entry.OwnerID].Content.H != 0 {
						t.Fatal("empty paragraph created a CSS line")
					}
				case "\u00a0":
					nbsp++
				case "a b c":
					words++
				}
			}
			if empty != 2 || nbsp != 1 || words != 1 {
				t.Fatalf("display=%s empty=%d NBSP=%d words=%d", display, empty, nbsp, words)
			}
		}
	}
}

func TestAnonymousWhitespaceIgnoresEmptyFontMetrics(t *testing.T) {
	d, _, _ := build(t, `<div id="root"><p>x</p><span style="font-size:30px"> </span><p>y</p><p><span style="font-size:30px"></span></p></div>`)
	if len(d.Root().Children) != 3 {
		t.Fatal("font size created an anonymous block")
	}
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 200, breaker, layoutConfig(t))
		if r.Geometry()[d.Root().Children[2].BoxID].Content.H != 0 {
			t.Fatal("empty inline metrics created height")
		}
	}
}

func TestEmptyCSSParagraphRetainsEdgesAndProof(t *testing.T) {
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		d, _, _ := build(t, `<div id="root"><p style="margin:10px;padding:2px;border-width:2px;border-style:solid"><span style="font-size:50px"> </span></p></div>`)
		r := runDocument(t, d, 200, breaker, layoutConfig(t))
		g := r.Geometry()[d.Root().Children[0].BoxID]
		if g.Content.H != 0 || g.Frame.H != 8 || g.Frame.Y != 10 || r.Geometry()[d.Root().BoxID].Content.H != 28 {
			t.Fatal("empty principal box lost its edges/margins or gained a strut", g)
		}
		proof, err := documentlayout.BuildProof(r, cells())
		if err != nil || len(proof.Lines) != 0 || len(proof.Glyphs) != 0 || len(proof.Boxes) != 2 {
			t.Fatal("empty proof lost boxes or gained lines", proof, err)
		}
	}
}
