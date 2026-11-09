package main

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"tytui/htmlinput"
	"tytui/pipeline"
)

func loadedWorkbench(t *testing.T) model {
	t.Helper()
	m := initialModel()
	m, _ = updateModel(m, keyPress(tea.KeyF4))
	m.browser.path.SetValue("testdata/html/paragraphs.html")
	m, cmd := updateModel(m, keyPress(tea.KeyEnter))
	if cmd == nil || m.browser.activeID == 0 {
		t.Fatal("load not scheduled")
	}
	m, _ = updateModel(m, cmd())
	if m.browser.doc == nil {
		t.Fatalf("load failed: %s", m.browser.status)
	}
	return m
}
func findID(m model, value string) int {
	for _, n := range m.browser.doc.Nodes {
		for _, a := range n.Attributes {
			if a.Key == "id" && a.Val == value {
				return n.ID
			}
		}
	}
	return -1
}
func TestDOMSelectionToFrozenResult(t *testing.T) {
	m := loadedWorkbench(t)
	m.browser.selected = findID(m, "first")
	m.browser.inspectSelected()
	if !strings.Contains(m.browser.inspect.View(), "first") {
		t.Fatal("selection inspector not updated")
	}
	m, _ = updateModel(m, keyPress(tea.KeyEnter))
	if m.browser.open || m.source == nil || m.textarea.Value() != "Hello world & café." {
		t.Fatalf("selection: %q %s", m.textarea.Value(), m.browser.status)
	}
	m, cmd := updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	if m.result == nil || m.result.Input.Source == nil {
		t.Fatalf("source map not retained: %s", m.status)
	}
	retained := m.result.Input.Source
	if !strings.Contains(renderResult(m.result), "CSS style spans") || !strings.Contains(renderResult(m.result), "Logical bidi runs") {
		t.Fatal("resolved styles or bidi runs not inspectable")
	}
	if !strings.Contains(renderResult(m.result), "DOM source map") {
		t.Fatal("map not inspectable")
	}
	// A width change preserves DOM provenance; text edits detach it.
	m.setFocus(focusWidth)
	m, _ = updateModel(m, tea.KeyPressMsg{Code: '0', Text: "0"})
	if m.source == nil {
		t.Fatal("width detached source map")
	}
	m.setFocus(focusText)
	m, _ = updateModel(m, tea.PasteMsg{Content: "changed"})
	if m.source != nil {
		t.Fatal("text editing kept stale source map")
	}
	if retained.Text != "Hello world & café." {
		t.Fatal("history provenance changed")
	}
}
func TestDOMNavigationAndRejectedSelection(t *testing.T) {
	m := loadedWorkbench(t)
	m, _ = updateModel(m, keyPress(tea.KeyRight))
	if m.browser.selected == 0 {
		t.Fatal("right did not enter child")
	}
	m, _ = updateModel(m, keyPress(tea.KeyLeft)) // collapse
	if !m.browser.collapsed[m.browser.selected] {
		t.Fatal("left did not collapse")
	}
	m, _ = updateModel(m, keyPress(tea.KeyRight))
	if m.browser.collapsed[m.browser.selected] {
		t.Fatal("right did not expand")
	}
	old := m.textarea.Value()
	for _, id := range []string{"nested", "break"} {
		m.browser.selected = findID(m, id)
		m, _ = updateModel(m, keyPress(tea.KeyEnter))
		if !m.browser.open || m.source != nil || m.textarea.Value() != old {
			t.Fatal("unsupported selection replaced input")
		}
		if !strings.Contains(m.browser.status, "unsupported") && !strings.Contains(m.browser.status, "nested block") {
			t.Fatalf("missing reason: %s", m.browser.status)
		}
	}
	m, _ = updateModel(m, keyPress(tea.KeyTab))
	if m.browser.focus != propertiesFocus {
		t.Fatal("property focus missing")
	}
	before := m.browser.inspect.YOffset()
	m, _ = updateModel(m, keyPress(tea.KeyPgDown))
	if m.browser.inspect.YOffset() <= before {
		t.Fatal("inspector did not scroll")
	}
	m, _ = updateModel(m, keyPress(tea.KeyEscape))
	if m.browser.open {
		t.Fatal("escape did not return to paragraph")
	}
}
func TestHTMLLoadFailureAndStaleCompletion(t *testing.T) {
	m := loadedWorkbench(t)
	original := m.browser.doc
	m.browser.setFocus(pathFocus)
	m.browser.path.SetValue("testdata/html/missing.html")
	m, cmd := updateModel(m, keyPress(tea.KeyEnter))
	m, _ = updateModel(m, cmd())
	if m.browser.doc != original || !strings.Contains(m.browser.status, "Load failed") {
		t.Fatal("failure replaced working DOM")
	}
	m.browser.path.SetValue("testdata/html/paragraphs.html")
	m, cmd = updateModel(m, keyPress(tea.KeyEnter))
	m, _ = updateModel(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m, _ = updateModel(m, cmd())
	if m.browser.activeID != 0 || m.browser.doc != original || !strings.Contains(m.browser.status, "Filename changed") {
		t.Fatal("stale load accepted")
	}
}
func TestBrowserViewSizes(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 40}} {
		m := loadedWorkbench(t)
		m, _ = updateModel(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, f := range []browserFocus{pathFocus, treeFocus, propertiesFocus} {
			m.browser.setFocus(f)
			v := m.View()
			if lipgloss.Width(v.Content) > size[0] || lipgloss.Height(v.Content) > size[1] {
				for _, line := range strings.Split(v.Content, "\n") {
					if lipgloss.Width(line) > size[0] {
						t.Logf("wide %d: %q", lipgloss.Width(line), line)
					}
				}
				t.Fatalf("browser exceeds %v: %dx%d", size, lipgloss.Width(v.Content), lipgloss.Height(v.Content))
			}
			if v.Cursor != nil && (v.Cursor.X >= size[0] || v.Cursor.Y >= size[1]) {
				t.Fatal("cursor outside view")
			}
		}
	}
}
func TestSourceMapValidationAndOwnership(t *testing.T) {
	d, err := htmlinput.Parse("input", strings.NewReader("<p>abc</p>"))
	if err != nil {
		t.Fatal(err)
	}
	var id int
	for _, n := range d.Nodes {
		if n.Name == "p" {
			id = n.ID
		}
	}
	e, err := d.Extract(id)
	if err != nil {
		t.Fatal(err)
	}
	width, _ := parseWidth("100")
	result, err := pipeline.Run(context.Background(), pipeline.Input{Text: e.Text, Width: width, Source: e})
	if err != nil {
		t.Fatal(err)
	}
	e.Spans[0].NodePath = "modified"
	if result.Input.Source.Spans[0].NodePath == "modified" {
		t.Fatal("source map aliases input")
	}
	if _, err := pipeline.Run(context.Background(), pipeline.Input{Text: "other", Width: width, Source: e}); err == nil {
		t.Fatal("mismatched provenance accepted")
	}
}
