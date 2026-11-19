package documentlayout_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/npillmayer/khipu"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/documentlayout"
)

func TestBoxesSVGCoordinates(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="padding:7px;border-width:3px;border-style:solid"><div style="margin:13px 0 0 11px;padding:5px;border-width:2px;border-style:solid">x</div></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 300, breaker, layoutConfig(t))
		var b bytes.Buffer
		if err := documentlayout.WriteBoxesSVG(&b, r); err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(&b)
		found := false
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if el, ok := tok.(xml.StartElement); ok {
				if el.Name.Local == "path" || el.Name.Local == "image" || el.Name.Local == "text" {
					t.Fatal("non-box output", el.Name)
				}
				if el.Name.Local == "rect" {
					m := map[string]string{}
					for _, a := range el.Attr {
						m[a.Name.Local] = a.Value
					}
					x, _ := strconv.ParseFloat(m["x"], 64)
					y, _ := strconv.ParseFloat(m["y"], 64)
					// Root content at (10,10), child frame at (21,23) CSS px.
					if x == 15.75 && y == 17.25 {
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("missing independently positioned child border")
		}
	}
}

func TestBoxesSVGKindsAndTextFragments(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="padding:7px;font-size:10px">before <span style="display:inline-block;width:30px;padding:2px">inside</span> after<section style="display:flow-root;margin-top:11px;padding:3px"><div dir="rtl">אבג abc אבג abc אבג abc אבג abc</div></section></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			r := runDocument(t, d, 75, breaker, layoutConfig(t))
			before := r.Geometry()
			proof, err := documentlayout.BuildProof(r, cells())
			if err != nil {
				t.Fatal(err)
			}
			// Use retained proof lines as the independent absolute-coordinate
			// reference, including nested owner transforms and wrapped RTL lines.
			want := map[[3]uint64][4]float64{}
			wrapped, atomic := false, false
			for _, entry := range r.Groups() {
				g, _ := r.Group(entry.GroupID)
				var lines []documentlayout.ProofLine
				for _, l := range proof.Lines {
					if l.GroupID == entry.GroupID {
						lines = append(lines, l)
					}
				}
				wrapped = wrapped || len(lines) > 1
				for i, placed := range g.Paragraph.VList.Lines() {
					x := lines[i].X + placed.Horizontal.LeftSkip.Points()
					for k := placed.Horizontal.FirstKnot; k < placed.Horizontal.FirstKnot+placed.Horizontal.KnotCount; k++ {
						knot, _ := g.Paragraph.Frozen.Knot(k)
						if knot.Kind == khipu.KTTextBox {
							want[[3]uint64{uint64(entry.GroupID), uint64(placed.Index), uint64(k)}] = [4]float64{x, lines[i].Top, knot.W.Points(), lines[i].Height}
						}
						atomic = atomic || knot.Kind == khipu.KTAtomic
						x += knot.W.Points()
					}
				}
			}
			if !wrapped || !atomic {
				t.Fatal("fixture must include wrapped text and an atomic")
			}
			var out bytes.Buffer
			if err := documentlayout.WriteBoxesSVG(&out, r); err != nil {
				t.Fatal(err)
			}
			original := out.String()
			dec := xml.NewDecoder(strings.NewReader(original))
			colors := map[string]string{"block": "#6366f1", "flow-root": "#0d9488", "inline-block": "#d97706"}
			seen := map[string]int{}
			rects, pairs, fragments := 0, 0, 0
			for {
				tok, err := dec.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				el, ok := tok.(xml.StartElement)
				if !ok {
					continue
				}
				a := map[string]string{}
				for _, attr := range el.Attr {
					a[attr.Name.Local] = attr.Value
				}
				if kind := a["data-kind"]; kind != "" {
					if a["stroke"] != colors[kind] {
						t.Fatal("incorrect box color", a)
					}
					seen[kind]++
					pairs++
					rects = 0
					if a["data-box"] == strconv.FormatUint(uint64(d.Root().BoxID), 10) && kind != "block" {
						t.Fatal("root containment was mistaken for flow-root display")
					}
				}
				if a["id"] == "text-fragments" && a["stroke"] != "#d1d5db" {
					t.Fatal("incorrect text color")
				}
				if el.Name.Local != "rect" {
					continue
				}
				if a["data-knot"] == "" {
					rects++
					if (rects == 1 && a["stroke-dasharray"] != "") || (rects == 2 && a["stroke-dasharray"] != "2 2") || rects > 2 {
						t.Fatal("incorrect border/content styles", a)
					}
					continue
				}
				var key [3]uint64
				for i, name := range []string{"data-group", "data-line", "data-knot"} {
					key[i], err = strconv.ParseUint(a[name], 10, 64)
					if err != nil {
						t.Fatal(err)
					}
				}
				w, ok := want[key]
				if !ok {
					t.Fatal("unexpected or duplicate fragment", key)
				}
				for i, name := range []string{"x", "y", "width", "height"} {
					v, err := strconv.ParseFloat(a[name], 64)
					if err != nil || math.Abs(v-w[i]) > 1e-6 {
						t.Fatalf("fragment %v %s=%s; want %g", key, name, a[name], w[i])
					}
				}
				delete(want, key)
				fragments++
			}
			if len(seen) != 3 || pairs != len(before) || fragments == 0 || len(want) != 0 {
				t.Fatal("missing box kinds or fragments", seen, len(want))
			}
			out.Reset()
			if err := documentlayout.WriteBoxesSVG(&out, r); err != nil {
				t.Fatal(err)
			}
			if out.String() != original || !reflect.DeepEqual(before, r.Geometry()) {
				t.Fatal("export mutated retained output")
			}
		})
	}
}

// Glyph paths must match the existing frozen document renderer exactly, including
// bidi, nested owner transforms, paint, and both line breakers.
func TestLayoutSVGGlyphs(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="padding:7px;color:#123456">office <span style="display:inline-block;width:35px">café</span><section style="display:flow-root;padding:3px" dir="rtl">אבג abc אבג abc</section></div>`)
	paths := func(data []byte) []xml.StartElement {
		t.Helper()
		dec := xml.NewDecoder(bytes.NewReader(data))
		var result []xml.StartElement
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				return result
			}
			if err != nil {
				t.Fatal(err)
			}
			if el, ok := tok.(xml.StartElement); ok && el.Name.Local == "path" {
				for _, a := range el.Attr {
					if a.Name.Local == "data-gid" {
						result = append(result, el.Copy())
						break
					}
				}
			}
		}
	}
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 90, breaker, layoutConfig(t))
		var reference bytes.Buffer
		if err := documentlayout.WriteSVG(&reference, r, cells()); err != nil {
			t.Fatal(err)
		}
		want := paths(reference.Bytes())
		if len(want) == 0 {
			t.Fatal("missing reference glyphs")
		}
		for _, opts := range []documentlayout.LayoutSVGOptions{{ShowGlyphs: true}, {HideBoxes: true}, {ShowGlyphs: true, HideBoxes: true}} {
			var out, again bytes.Buffer
			if err := documentlayout.WriteLayoutSVG(&out, r, cells(), opts); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(paths(out.Bytes()), want) {
				t.Fatal("changed retained glyph outlines or placements")
			}
			if opts.HideBoxes && (strings.Contains(out.String(), "<rect") || strings.Contains(out.String(), "<circle")) {
				t.Fatal("clean proof contains diagnostics")
			}
			if err := documentlayout.WriteLayoutSVG(&again, r, cells(), opts); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out.Bytes(), again.Bytes()) {
				t.Fatal("export changed retained result")
			}
		}
		var after bytes.Buffer
		if err := documentlayout.WriteSVG(&after, r, cells()); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(reference.Bytes(), after.Bytes()) {
			t.Fatal("glyph export mutated frozen layout")
		}
	}
}
