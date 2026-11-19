package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tytui/documentlayout"
)

func cliFlowFixture(t *testing.T) (options, string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.html")
	frames := filepath.Join(dir, "frames.json")
	if err := os.WriteFile(input, []byte(`<div id="root" style="font-size:20px;color:navy">office office office office office office office office office office office office office office office office</div>`), 0600); err != nil {
		t.Fatal(err)
	}
	return options{input: input, fonts: "examples/fonts.yaml", root: "root", output: "-", width: "100", breaker: "firstfit", log: "trace", frames: frames}, frames
}
func writeFrames(t *testing.T, path string, frames []documentlayout.FlowFrame) {
	t.Helper()
	data, err := json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestCLIFrameFlowRoutesAndControls(t *testing.T) {
	for _, widths := range [][]float64{{100, 100}, {100, 70}, {100, 150}, {100, 70, 100}} {
		o, path := cliFlowFixture(t)
		frames := make([]documentlayout.FlowFrame, len(widths))
		for j, w := range widths {
			frames[j] = documentlayout.FlowFrame{ID: string(rune('A' + j)), X: float64(j) * 200, Width: w, Height: 30}
		}
		frames[len(frames)-1].Height = 2000
		writeFrames(t, path, frames)
		for _, mode := range []int{0, 1, 2} {
			o.showGlyphs = mode == 1
			o.hideBoxes = mode == 2
			var out, log bytes.Buffer
			if err := run(o, &out, &log); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), `data-frame=`) != (mode != 2) || strings.Contains(out.String(), `data-gid=`) != (mode != 0) {
				t.Fatal("controls ignored", mode)
			}
			if !strings.Contains(log.String(), "flow frame=A reused=true") || !strings.Contains(log.String(), "suffix=firstfit") {
				t.Fatal("missing route diagnostics", log.String())
			}
			if widths[1] != 100 && !strings.Contains(log.String(), "flow frame=B reused=false") {
				t.Fatal("did not continue at new width", log.String())
			}
		}
	}
}
func TestCLIFrameFailurePreservesOutput(t *testing.T) {
	o, path := cliFlowFixture(t)
	destination := filepath.Join(t.TempDir(), "old.svg")
	o.output = destination
	if err := os.WriteFile(destination, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	writeFrames(t, path, []documentlayout.FlowFrame{{ID: "too-short", Width: 100, Height: 0}})
	var stdout, log bytes.Buffer
	if err := run(o, &stdout, &log); err == nil || !strings.Contains(err.Error(), "incomplete frame flow") {
		t.Fatal("incomplete flow accepted", err)
	}
	data, _ := os.ReadFile(destination)
	if string(data) != "previous" || stdout.Len() != 0 {
		t.Fatal("incomplete export replaced output")
	}
	for _, bad := range []string{`[]`, `[{"id":"A","width":100,"height":2000,"font-size":90}]`, `[{"id":"A","width":100,"height":2000,"direction":"rtl"}]`, `[] []`} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if err := run(o, &stdout, &log); err == nil {
			t.Fatal("invalid/styled frames accepted", bad)
		}
		data, _ = os.ReadFile(destination)
		if string(data) != "previous" {
			t.Fatal("invalid export replaced output")
		}
	}
}
