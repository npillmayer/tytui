package main

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"tytui/pipeline"
)

func TestFontPreviewDoesNotPinEditablePathForLaterRun(t *testing.T) {
	data, err := os.ReadFile(uiFont)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "editable.ttf")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m.fontEditor.path.SetValue(path)
	m, cmd := updateModel(m, fontApplyKey())
	m, _ = updateModel(m, cmd())
	if m.fontSettings == nil {
		t.Fatal("preview failed", m.fontEditor.status)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	m, cmd = updateModel(m, runKey())
	msg := cmd().(layoutMsg)
	var stage *pipeline.StageError
	if msg.result != nil || !errors.As(msg.err, &stage) || stage.Stage != pipeline.StageFonts || !errors.Is(msg.err, os.ErrNotExist) {
		t.Fatal("run ignored current file or lost font-stage error", msg.err)
	}
}
