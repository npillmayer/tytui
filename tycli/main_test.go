package main

import (
	"bytes"
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
				err := run(t.Context(), options{input: html, css: css, fonts: mapping, root: "root", output: "-", width: "200", breaker: breaker, log: level}, &out, &log)
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
		err := run(t.Context(), options{width: width, output: path, log: "quiet", breaker: "firstfit"}, &bytes.Buffer{}, &bytes.Buffer{})
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
		main()
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIProcess$")
	cmd.Env = append(os.Environ(), "TYCLI_TEST_PROCESS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !bytes.HasPrefix(out, []byte("<svg ")) || !bytes.Contains(out, []byte("data-parent=")) {
		t.Fatalf("not an SVG: %s", out)
	}
}
