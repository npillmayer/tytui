package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"tytui/pipeline"
)

func TestBreakerHistoryAndExportSnapshot(t *testing.T) {
	m := initialModel()
	m, cmd := updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	first := m.result
	if len(m.history) != 1 {
		t.Fatal("completed result not retained")
	}
	directory := t.TempDir()
	m.export = func(r *pipeline.Result) (string, error) { return exportSVG(r, directory) }
	m, export := updateModel(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if export == nil {
		t.Fatal("no export command")
	}
	m, _ = updateModel(m, keyPress(tea.KeyF2))
	if m.result != nil || m.breaker != pipeline.KnuthPlass {
		t.Fatal("selection did not invalidate input")
	}
	m, cmd = updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	if m.result == nil || m.result.Input.Breaker != pipeline.KnuthPlass || len(m.history) != 2 {
		t.Fatalf("second layout failed: %s", m.status)
	}
	// Export was requested before the new run and must still contain firstfit.
	completion := export().(exportMsg)
	if completion.err != nil {
		t.Fatal(completion.err)
	}
	data, err := os.ReadFile(completion.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "firstfit") || strings.Contains(string(data), "knuth-plass") {
		t.Fatal("export used newer layout")
	}
	m, _ = updateModel(m, completion)
	m, _ = updateModel(m, keyPress(tea.KeyF3))
	if m.result != first {
		t.Fatal("previous frozen result lost")
	}
	if m.breaker != pipeline.KnuthPlass {
		t.Fatal("history changed input selection")
	}
	secondPath, err := exportSVG(first, directory)
	if err != nil {
		t.Fatal(err)
	}
	if secondPath == completion.path {
		t.Fatal("export overwrote existing proof")
	}
	data2, _ := os.ReadFile(secondPath)
	if string(data) != string(data2) {
		t.Fatal("retained proof changed")
	}
}

func TestBreakerSwitchDiscardsActiveRun(t *testing.T) {
	m := initialModel()
	m, first := updateModel(m, runKey())
	m, _ = updateModel(m, keyPress(tea.KeyF2))
	m, cmd := updateModel(m, runKey())
	if cmd != nil || m.pending.input.Breaker != pipeline.KnuthPlass {
		t.Fatal("wrong queued breaker")
	}
	m, second := updateModel(m, first())
	if m.result != nil || len(m.history) != 0 {
		t.Fatal("stale result published")
	}
	m, _ = updateModel(m, second())
	if m.result == nil || m.result.Input.Breaker != pipeline.KnuthPlass {
		t.Fatal("new breaker not run")
	}
}

func TestExportFailures(t *testing.T) {
	dir := t.TempDir()
	if _, err := exportSVG(nil, dir); err == nil {
		t.Fatal("exported nil result")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("failed export left partial file")
	}
	if _, err := exportSVG(nil, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory not reported")
	}
	m := initialModel()
	_, cmd := updateModel(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("export launched without result")
	}
	m, cmd = updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	m.export = func(r *pipeline.Result) (string, error) { return exportSVG(r, filepath.Join(dir, "missing")) }
	m, cmd = updateModel(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m, _ = updateModel(m, cmd())
	if m.exporting || !strings.Contains(m.status, "SVG export failed") {
		t.Fatal("file error not surfaced")
	}
}
