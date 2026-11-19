package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPipeline(t *testing.T) {
	dir := t.TempDir()
	font, err := filepath.Abs("../../opentype/testdata/fonts/GentiumPlus-R.ttf")
	if err != nil {
		t.Fatal(err)
	}
	mapping := filepath.Join(dir, "fonts.yaml")
	if err := os.WriteFile(mapping, []byte("default: "+font+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	html := filepath.Join(dir, "input.html")
	if err := os.WriteFile(html, []byte(`<div id="root" style="padding:7px;border-width:3px;border-style:solid">office <span style="display:inline-block;width:50px">café</span><div>more text</div></div>`), 0600); err != nil {
		t.Fatal(err)
	}
	css := filepath.Join(dir, "extra.css")
	if err := os.WriteFile(css, []byte(`#root { font-size:20px; }`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, breaker := range []string{"firstfit", "knuth-plass"} {
		for _, level := range []string{"quiet", "info", "trace"} {
			t.Run(breaker+"/"+level, func(t *testing.T) {
				var out, log bytes.Buffer
				err := run(options{input: html, css: css, fonts: mapping, root: "root", output: "-", width: "200", breaker: breaker, log: level}, &out, &log)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.String(), "data-parent=") || strings.Contains(out.String(), "<path") || strings.Contains(out.String(), "<text") {
					t.Fatal(out.String())
				}
				if level == "quiet" && log.Len() != 0 {
					t.Fatal(log.String())
				}
				if level == "trace" && (!strings.Contains(log.String(), "shape:") || !strings.Contains(log.String(), "knot=")) {
					t.Fatal(log.String())
				}
			})
		}
	}
}

func TestFlowRootPipeline(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "flow-root.html")
	css := filepath.Join(dir, "flow-root.css")
	// The invalid inline declaration must not displace the valid stylesheet.
	source := `<div id="root" style="display:bogus!important;font-size:10px"><section id="inner" style="margin-top:13px;margin-bottom:5px"><div style="margin-top:20px;margin-bottom:7px">x</div></section></div>`
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, breaker := range []string{"firstfit", "knuth-plass"} {
		var canonical []byte
		for _, display := range []string{"flow-root", "block flow-root", "FLOW-ROOT BLOCK"} {
			if err := os.WriteFile(css, []byte(`#root, #inner {display:`+display+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			var out, log bytes.Buffer
			if err := run(options{input: input, css: css, root: "root", fonts: "examples/fonts.yaml", output: "-", width: "200", breaker: breaker, log: "quiet"}, &out, &log); err != nil {
				t.Fatalf("%s/%s: %v", breaker, display, err)
			}
			if canonical == nil {
				canonical = bytes.Clone(out.Bytes())
			} else if !bytes.Equal(canonical, out.Bytes()) {
				t.Fatal("equivalent display spelling changed CLI output")
			}
			decoder := xml.NewDecoder(&out)
			found := false
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if el, ok := token.(xml.StartElement); ok {
					if el.Name.Local == "path" || el.Name.Local == "text" || el.Name.Local == "image" {
						t.Fatal("non-box output", el.Name)
					}
					if el.Name.Local == "rect" {
						for _, a := range el.Attr {
							// Child's absolute top = 13 + 20 CSS px = 24.75 bp.
							if a.Name.Local == "y" && a.Value == "24.75" {
								found = true
							}
						}
					}
				}
			}
			if !found || log.Len() != 0 {
				t.Fatalf("flow-root child placement missing or quiet mode produced diagnostics: %s", log.String())
			}
		}
	}
}

func TestMappingValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fonts.yaml")
	for _, bad := range []string{"default: foo\nunknown: true", "default: foo\n---\ndefault: bar", "rules: []", "default: foo\ndefault: bar"} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadFonts(path); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if err := os.WriteFile(path, []byte("default: fonts/regular.ttf\nrules:\n  - family: serif\n    weight: 700\n    path: fonts/bold.ttf\nfallbacks: [fonts/hebrew.ttf]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := loadFonts(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.DefaultPath != filepath.Join(dir, "fonts/regular.ttf") || r.Rules[0].Weight != 700 || r.Fallbacks[0] != filepath.Join(dir, "fonts/hebrew.ttf") {
		t.Fatal(r)
	}
}

func TestFailurePreservesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.svg")
	os.WriteFile(path, []byte("previous"), 0600)
	for _, width := range []string{"NaN", "Inf", "-1", "bogus"} {
		err := run(options{width: width, output: path, log: "quiet", breaker: "firstfit"}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatalf("accepted width %s", width)
		}
	}
	data, _ := os.ReadFile(path)
	if string(data) != "previous" {
		t.Fatal(string(data))
	}
}

func TestCLIProcess(t *testing.T) {
	if os.Getenv("TYCLI_TEST_PROCESS") == "1" {
		os.Args = []string{"tycli", "../documentlayout/testdata/nested.html", "--root", "root", "--fonts", "examples/fonts.yaml", "--width", "420", "--output", "-", "--log", "quiet"}
		os.Args = append(os.Args, strings.Fields(os.Getenv("TYCLI_TEST_FLAGS"))...)
		main()
		os.Exit(0)
	}
	for _, flags := range []string{"", "--show-glyphs", "--hide-boxes", "--show-glyphs --hide-boxes"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCLIProcess$")
		cmd.Env = append(os.Environ(), "TYCLI_TEST_PROCESS=1", "TYCLI_TEST_FLAGS="+flags)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v: %s", flags, err, out)
		}
		if !bytes.HasPrefix(out, []byte("<svg ")) {
			t.Fatalf("not an SVG: %s", out)
		}
		if got := bytes.Contains(out, []byte("data-gid=")); got != (flags != "") {
			t.Fatalf("%s: glyphs=%v", flags, got)
		}
		hidden := strings.Contains(flags, "--hide-boxes")
		if got := bytes.Contains(out, []byte("data-parent=")); got == hidden {
			t.Fatalf("%s: boxes=%v", flags, got)
		}
		if got := bytes.Contains(out, []byte(`id="text-fragments"`)); got == hidden {
			t.Fatalf("%s: fragments=%v", flags, got)
		}
	}
}

func TestStylesFixtureParagraphsWithoutCSSOverride(t *testing.T) {
	for _, root := range []string{"bidi", "verse"} {
		for _, breaker := range []string{"firstfit", "knuth-plass"} {
			var out, log bytes.Buffer
			err := run(options{input: "../testdata/html/styles.html", root: root, fonts: "examples/fonts.yaml", output: "-", width: "420", breaker: breaker, log: "quiet"}, &out, &log)
			if err != nil {
				t.Fatalf("root=%s breaker=%s: %v", root, breaker, err)
			}
			if !bytes.HasPrefix(out.Bytes(), []byte("<svg ")) {
				t.Fatal("missing SVG")
			}
		}
	}
}
