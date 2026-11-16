package documentlayout_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"testing"

	"github.com/npillmayer/typography/paragraph"
	"tytui/documentlayout"
)

func TestBoxesSVGCoordinates(t *testing.T) {
	d, _, _ := build(t, `<div id="root" style="padding:7px;border-width:3px;border-style:solid"><div style="margin:13px 0 0 11px;padding:5px;border-width:2px;border-style:solid">x</div></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		r := runDocument(t, d, 300, breaker, layoutConfig(t))
		var b bytes.Buffer
		if err := documentlayout.WriteBoxesSVG(t.Context(), &b, r); err != nil {
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
