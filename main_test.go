package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/npillmayer/khipu/dimen"
	"tytui/pipeline"
)

func updateModel(m model, msg tea.Msg) (model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(model), cmd
}
func keyPress(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }
func runKey() tea.KeyPressMsg            { return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl} }

func TestWorkbenchRunsPipeline(t *testing.T) {
	m := initialModel()
	m.textarea.SetValue("éa bb cc")
	m.widthInput.SetValue("12")
	var cmd tea.Cmd
	m, cmd = updateModel(m, runKey())
	if cmd == nil || m.activeID == 0 || m.result != nil {
		t.Fatal("run did not schedule work")
	}
	m, _ = updateModel(m, cmd())
	if m.result == nil || m.result.Frozen.LineCount() != 3 || m.activeID != 0 {
		t.Fatalf("unexpected completion: %s", m.status)
	}
	content := renderResult(m.result)
	for _, want := range []string{"Line 1", "bytes [0,4)", "éa", "actual", "target", "residual", "badness", "Pipeline stages", "freeze:"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in result: %s", want, content)
		}
	}
	// Tab changes focus, and entering a new width invalidates the old layout.
	m, _ = updateModel(m, keyPress(tea.KeyTab))
	if m.focus != focusWidth {
		t.Fatal("tab did not focus width")
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Code: '0', Text: "0"})
	if m.result != nil {
		t.Fatal("width edit retained old result")
	}
	m, cmd = updateModel(m, keyPress(tea.KeyEnter))
	m, _ = updateModel(m, cmd())
	if m.result.Frozen.LineCount() != 1 {
		t.Fatal("wider width did not reflow paragraph")
	}
}

func TestEditCancelsAndRejectsStaleResult(t *testing.T) {
	m := initialModel()
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	var once sync.Once
	releaseRun := func() { once.Do(func() { close(release) }) }
	defer releaseRun()
	// Simulate a backend that finishes successfully despite cancellation.
	m.run = func(ctx context.Context, in pipeline.Input) (*pipeline.Result, error) {
		started <- ctx
		<-release
		return pipeline.Run(context.Background(), in)
	}
	m, cmd := updateModel(m, runKey())
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var ctx context.Context
	select {
	case ctx = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not start")
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !strings.Contains(m.textarea.Value(), "?") {
		t.Fatal("question mark was captured as help")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("edit did not cancel active work")
	}
	releaseRun()
	select {
	case completion := <-done:
		m, _ = updateModel(m, completion)
	case <-time.After(5 * time.Second):
		t.Fatal("run did not finish")
	}
	if m.result != nil || !strings.Contains(m.status, "Input changed") {
		t.Fatal("stale result accepted")
	}
}

func TestRerunsSerializeAndKeepLatest(t *testing.T) {
	m := initialModel()
	m, first := updateModel(m, runKey())
	oldID := m.activeID
	m, cmd := updateModel(m, runKey())
	if cmd != nil || m.pending == nil || m.activeID != oldID {
		t.Fatal("rerun started concurrently")
	}
	// Editing cancels the queued request; the next run captures the new value.
	m, _ = updateModel(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.pending != nil {
		t.Fatal("edit retained queued input")
	}
	m, cmd = updateModel(m, runKey())
	if cmd != nil {
		t.Fatal("new run started before old completion")
	}
	m, second := updateModel(m, first())
	if second == nil || m.activeID == 0 || m.activeID == oldID {
		t.Fatal("latest request not dispatched")
	}
	newID := m.activeID
	m, _ = updateModel(m, layoutMsg{id: oldID, err: errors.New("late duplicate")})
	if m.activeID != newID {
		t.Fatal("duplicate completion disturbed current run")
	}
	m, _ = updateModel(m, second())
	if m.result == nil || m.result.Input.Text != m.textarea.Value() {
		t.Fatal("latest input not published")
	}
}

func TestWidthValidation(t *testing.T) {
	for _, s := range []string{"", "0", "-1", "NaN", "Inf", "1e100", "32768", "0.00000001", "abc"} {
		if _, err := parseWidth(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	if w, err := parseWidth(" 12.5 "); err != nil || w != 12*dimen.BP+dimen.BP/2 {
		t.Fatalf("width %d, %v", w, err)
	}
	m := initialModel()
	m.widthInput.SetValue("bad")
	m, cmd := updateModel(m, runKey())
	if cmd != nil || m.activeID != 0 || !strings.Contains(m.status, "Width must") {
		t.Fatal("invalid width launched work")
	}
}

func TestPasteAndResultScrolling(t *testing.T) {
	m := initialModel()
	m, cmd := updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	m.setFocus(focusResults)
	before := m.viewport.YOffset()
	m, _ = updateModel(m, keyPress(tea.KeyPgDown))
	if m.viewport.YOffset() <= before {
		t.Fatal("results did not scroll")
	}
	m.setFocus(focusText)
	m, _ = updateModel(m, tea.PasteMsg{Content: " pasted text"})
	if m.result != nil || !strings.Contains(m.textarea.Value(), "pasted text") {
		t.Fatal("paste did not invalidate the layout")
	}
}

func TestFailureAndQuit(t *testing.T) {
	m := initialModel()
	var active context.Context
	m.run = func(ctx context.Context, _ pipeline.Input) (*pipeline.Result, error) {
		active = ctx
		return nil, &pipeline.StageError{Stage: pipeline.StageEncode, Err: errors.New("test backend error")}
	}
	m, cmd := updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	if m.result != nil || !strings.Contains(m.viewport.View(), "test backend error") {
		t.Fatal("failure not displayed")
	}
	if active.Err() == nil {
		t.Fatal("command context not released")
	}
	m, _ = updateModel(m, runKey())
	cancel := m.cancel
	if cancel == nil {
		t.Fatal("missing cancellation")
	}
	_, quit := updateModel(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("quit not dispatched")
	}
}

func TestViewSizesAndFocus(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 40}} {
		for _, full := range []bool{false, true} {
			m := initialModel()
			m.help.ShowAll = full
			m, _ = updateModel(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, f := range []focus{focusText, focusWidth, focusResults} {
				m.setFocus(f)
				v := m.View()
				if lipgloss.Width(v.Content) > size[0] || lipgloss.Height(v.Content) > size[1] {
					t.Errorf("view %dx%d exceeds %v: %q", lipgloss.Width(v.Content), lipgloss.Height(v.Content), size, v.Content)
				}
				if v.Cursor != nil && (v.Cursor.X >= size[0] || v.Cursor.Y >= size[1]) {
					t.Errorf("cursor outside view: %+v", v.Cursor)
				}
			}
		}
	}
	m := initialModel()
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 10, Height: 5})
	if !strings.Contains(m.View().Content, "Resize") {
		t.Fatal("missing small-terminal message")
	}
}
