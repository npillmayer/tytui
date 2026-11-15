package htmlinput

import (
	"html"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
)

func TestInlineDeclarationEndings(t *testing.T) {
	const declarations = `margin:1px 2px;font-family:'Hello; } World';font-size:20px;color:red;color:blue!important`
	for _, tc := range []struct{ name, suffix string }{
		{"unterminated", ""},
		{"terminated", ";"},
		{"whitespace", " \t\n"},
		{"terminated whitespace", "; \t\n"},
		{"comment", " /* trailing ; } */"},
		{"terminated comment", "; /* trailing ; } */"},
		{"comment then terminator", " /* trailing */; /* after ; } */\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attr := declarations + tc.suffix
			source := `<p style="` + html.EscapeString(attr) + `">é &amp; x</p>`
			d := parseTest(t, source)
			id := nodeNamed(d, "p")
			if d.OriginalHTML != source {
				t.Fatal("validation changed retained HTML")
			}
			var retained string
			for _, a := range d.Nodes[id].Attributes {
				if a.Key == "style" {
					retained = a.Val
				}
			}
			if retained != attr {
				t.Fatalf("style attribute = %q, want original %q", retained, attr)
			}
			for key, want := range map[string]string{"margin-left": "2px", "font-family": "'Hello; } World'", "font-size": "20px", "color": "blue"} {
				got, err := d.Property(id, key)
				if err != nil || got != want {
					t.Fatalf("%s = %q (%v), want %q", key, got, err, want)
				}
			}
			e, err := d.Extract(id)
			if err != nil {
				t.Fatal(err)
			}
			if e.Text != "é & x" || len(e.Styles) != 1 || e.Styles[0].Style.EmSize != 15*dimen.BP || e.Styles[0].Style.Color != "blue" || e.Styles[0].Style.Family != "'Hello; } World'" {
				t.Fatalf("extracted text/styles = %+v", e)
			}
			if len(e.Spans) != 1 || e.Spans[0].Start != 0 || e.Spans[0].End != uint64(len(e.Text)) || e.Spans[0].NodeID != d.Nodes[id].Children[0] {
				t.Fatalf("source correspondence = %+v", e.Spans)
			}
		})
	}
}

func TestInlineDeclarationEmptyAndFinalValues(t *testing.T) {
	for _, attr := range []string{"", " \t\n", "/* only ; } */"} {
		t.Run("empty/"+attr, func(t *testing.T) {
			d := parseTest(t, `<p style="`+attr+`">x</p>`)
			if got, err := d.Property(nodeNamed(d, "p"), "font-family"); err != nil || got != "serif" {
				t.Fatalf("default font-family = %q (%v)", got, err)
			}
		})
	}
	for _, tc := range []struct{ attr, key, want string }{
		{`margin:1px 2px`, "margin-left", "2px"},
		{`font-family:'Hello; } World'`, "font-family", "'Hello; } World'"},
	} {
		for _, suffix := range []string{"", ";", "; /* trailing ; } */"} {
			t.Run(tc.attr+suffix, func(t *testing.T) {
				d := parseTest(t, `<p style="`+tc.attr+suffix+`">x</p>`)
				if got, err := d.Property(nodeNamed(d, "p"), tc.key); err != nil || got != tc.want {
					t.Fatalf("%s = %q (%v), want %q", tc.key, got, err, tc.want)
				}
			})
		}
	}
}

func TestInlineDeclarationValidationErrors(t *testing.T) {
	for _, tc := range []struct{ attr, diagnostic string }{
		{`color:red;made-up-property:yes`, `CSS property "made-up-property"`},
		{`color:red;made-up-property:yes; /* trailing */`, `CSS property "made-up-property"`},
		{`font-family:'quoted; }';made-up-property:yes;`, `CSS property "made-up-property"`},
		{`color red;`, "inline CSS:"},
		{`font-family:'unterminated`, "inline CSS:"},
		{`color:red; /* unterminated`, "inline CSS:"},
	} {
		t.Run(tc.attr, func(t *testing.T) {
			d, err := Parse("invalid.html", strings.NewReader(`<p style="`+tc.attr+`">x</p>`))
			if d != nil || err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("Parse = %v, %v; want no document and %q", d, err, tc.diagnostic)
			}
		})
	}
}
