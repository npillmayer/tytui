package htmlinput

import (
	"github.com/npillmayer/khipu/dimen"
	"strings"
	"testing"
)

func TestCSSAbsoluteFontSizes(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  dimen.DU
	}{
		{"96px", 72 * dimen.BP}, {"72pt", 72 * dimen.BP}, {"1in", 72 * dimen.BP},
		{"25.4mm", 72 * dimen.BP}, {"2.54cm", 72 * dimen.BP}, {"1.5PX", 73728}, {".5pt", 32768},
	} {
		t.Run(tc.value, func(t *testing.T) {
			e := extractP(t, `<p style="font-size:`+tc.value+`">é<span>x</span></p>`)
			for _, s := range e.Styles {
				if s.Style.EmSize != tc.want {
					t.Fatalf("size=%v want %v", s.Style.EmSize, tc.want)
				}
			}
		})
	}
	for _, v := range []string{"0pt", "-1px", "32768pt", "1e100px", "NaNpt", "1bp", "12", "2vw"} {
		if _, err := resolveSize(v, 12*dimen.BP, 12*dimen.BP); err == nil {
			t.Errorf("accepted %s", v)
		}
	}
}

func TestResolvedDisplayEligibility(t *testing.T) {
	for _, decl := range []string{"", ` style="display:initial"`, ` style="display:unset"`} {
		e := extractP(t, `<p>é<em`+decl+`>a</em><abbr>b</abbr></p>`)
		if e.Text != "éab" {
			t.Fatal(e.Text)
		}
	}
	for _, src := range []string{
		`<p><span style="display:inherit">x</span></p>`,
		`<p><span style="display:inline-block">x</span></p>`,
		`<div><section style="display:inline">x</section></div>`,
		`<p><custom-element>x</custom-element></p>`,
		`<p><span style="display:none">x</span></p>`,
	} {
		d, err := Parse("test", strings.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		id := nodeNamed(d, "p")
		if id < 0 {
			id = nodeNamed(d, "div")
		}
		if _, err = d.Extract(id); err == nil {
			t.Errorf("accepted %s", src)
		}
	}
}
