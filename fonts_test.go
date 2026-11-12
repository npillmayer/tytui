package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"
)

const uiFont = "../opentype/testdata/fonts/GentiumPlus-R.ttf"

func fontApplyKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl} }
func TestFontPanelApplyRunAndSynthetic(t *testing.T) {
	m := initialModel()
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m.fontEditor.path.SetValue(uiFont)
	m.fontEditor.rules.SetValue("serif | 700 | normal | " + uiFont)
	m, cmd := updateModel(m, fontApplyKey())
	if cmd == nil {
		t.Fatal("no asynchronous validation")
	}
	m, _ = updateModel(m, cmd())
	if m.fontEditor.open || m.fontSettings == nil || len(m.fontSettings.Rules) != 1 {
		t.Fatalf("apply: %s", m.fontEditor.status)
	}
	m.textarea.SetValue("office")
	m, cmd = updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	if m.result == nil || len(m.result.Fonts) != 1 {
		t.Fatalf("layout: %s %s", m.status, m.viewport.View())
	}
	if !strings.Contains(renderResult(m.result), "OpenType faces") {
		t.Fatal("font diagnostics missing")
	}
	retained := m.result
	m, _ = updateModel(m, keyPress(tea.KeyF6))
	if m.fontSettings != nil || m.result != nil {
		t.Fatal("synthetic did not invalidate input")
	}
	if retained.Input.Fonts == nil || retained.Frozen.LineCount() != 1 {
		t.Fatal("retained snapshot changed")
	}
}
func TestFontPanelStaleAndFailure(t *testing.T) {
	m := initialModel()
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m.fontEditor.path.SetValue(uiFont)
	m, cmd := updateModel(m, fontApplyKey())
	loaded := cmd()
	m, _ = updateModel(m, tea.PasteMsg{Content: "changed"})
	m, _ = updateModel(m, loaded)
	if m.fontSettings != nil {
		t.Fatal("stale font validation applied")
	}
	m.fontEditor.path.SetValue("missing.ttf")
	m, cmd = updateModel(m, fontApplyKey())
	m, _ = updateModel(m, cmd())
	if m.fontSettings != nil || !strings.Contains(m.fontEditor.status, "failed") {
		t.Fatal("missing font not diagnosed")
	}
	m.fontEditor.path.SetValue(uiFont)
	m, cmd = updateModel(m, fontApplyKey())
	m, _ = updateModel(m, keyPress(tea.KeyEscape))
	m, _ = updateModel(m, cmd())
	if m.fontEditor.open || m.fontSettings != nil {
		t.Fatal("closed panel applied stale load")
	}
}
func TestFontEditorRowsAndSize(t *testing.T) {
	f := newFontEditor()
	f.path.SetValue("/path with spaces/font.ttf")
	f.rules.SetValue("* | * | italic | /italic font.ttf\nserif | 700 | * | /bold.ttf")
	s, err := f.settings()
	if err != nil || len(s.Rules) != 2 || s.Rules[0].Family != "" || s.Rules[1].Weight != 700 {
		t.Fatalf("settings %+v %v", s, err)
	}
	f.rules.SetValue("serif | bold | * | font.ttf")
	if _, err = f.settings(); err == nil {
		t.Fatal("invalid weight accepted")
	}
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 40}} {
		m := initialModel()
		m.resize(size[0], size[1])
		m.fontEditor.open = true
		for n := 0; n < 3; n++ {
			m.fontEditor.setFocus(n)
			v := m.View()
			if lipgloss.Width(v.Content) > size[0] || lipgloss.Height(v.Content) > size[1] {
				t.Fatalf("font view exceeds %v", size)
			}
		}
	}
}
func TestFontSwitchDiscardsActiveLayout(t *testing.T) {
	m := initialModel()
	m.fontSettings = &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: uiFont}}
	m, _ = updateModel(m, runKey())
	revision := m.revision
	m, _ = updateModel(m, keyPress(tea.KeyF6))
	if m.revision <= revision {
		t.Fatal("backend did not change revision")
	}
}

func TestFallbackEditor(t *testing.T) {
	f := newFontEditor()
	f.path.SetValue(uiFont)
	f.rules.SetValue("fallback | * | * | /hebrew.ttf")
	s, err := f.settings()
	if err != nil || len(s.Fallbacks) != 1 || len(s.Rules) != 0 {
		t.Fatalf("fallback settings %+v %v", s, err)
	}
	clone := s.Clone()
	s.Fallbacks[0] = "changed"
	if clone.Fallbacks[0] != "/hebrew.ttf" {
		t.Fatal("fallback settings alias")
	}
}
