package htmlinput

import "testing"

func TestDeclarationCascade(t *testing.T) {
	for _, tc := range []struct{ name, source, key, want string }{
		{"inline duplicate", `<p style="color:red;color:blue">é</p>`, "color", "blue"},
		{"sheet duplicate", `<style>p{color:red;color:blue}</style><p>é</p>`, "color", "blue"},
		{"inline important survives", `<p style="color:red!important;color:blue">é</p>`, "color", "red"},
		{"sheet important survives", `<style>p{color:red!important;color:blue}</style><p>é</p>`, "color", "red"},
		{"important duplicate order", `<p style="color:red!important;color:blue!important">é</p>`, "color", "blue"},
		{"sheet beats normal inline", `<style>p{color:red!important}</style><p style="color:blue">é</p>`, "color", "red"},
		{"inline important wins", `<style>#x{color:red!important}</style><p id="x" style="color:blue!important">é</p>`, "color", "blue"},
		{"important specificity", `<style>#x{color:red!important}p{color:blue!important}</style><p id="x">é</p>`, "color", "red"},
		{"sheet order", `<style>p{color:red!important}</style><style>p{color:blue!important}</style><p>é</p>`, "color", "blue"},
		{"inline shorthand", `<p style="margin-left:8px;margin:1px 2px">é</p>`, "margin-left", "2px"},
		{"sheet shorthand", `<style>p{margin-left:8px!important;margin:1px 2px}</style><p>é</p>`, "margin-left", "8px"},
		{"quoted punctuation", `<p style="font-family:'Hello!; World';color:red">é</p>`, "font-family", "'Hello!; World'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := parseTest(t, tc.source)
			got, err := d.Property(nodeNamed(d, "p"), tc.key)
			if err != nil || got != tc.want {
				t.Fatalf("%s=%q %v want %q", tc.key, got, err, tc.want)
			}
			e, err := d.Extract(nodeNamed(d, "p"))
			if err != nil {
				t.Fatal(err)
			}
			if e.Text != "é" {
				t.Fatal(e.Text)
			}
			if tc.key == "color" && e.Styles[0].Style.Color != tc.want {
				t.Fatalf("extracted %+v", e.Styles)
			}
		})
	}
}
