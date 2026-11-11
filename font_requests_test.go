package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"strings"
	"testing"
	"tytui/fontbackend"
)

func fontControl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
func requestModel() model {
	m := initialModel()
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m, _ = updateModel(m, fontControl('d'))
	m.fontEditor.path.SetValue("Gentium Plus ; 400 ; normal")
	return m
}
func TestFontRequestPreviewApply(t *testing.T) {
	m := requestModel()
	m.fontEditor.rules.SetValue("serif | 700 | * | Gentium Plus ; 400 ; normal\nfallback | * | * | Gentium ; 400 ; normal")
	m, cmd := updateModel(m, fontControl('p'))
	if cmd == nil {
		t.Fatal("no resolution command")
	}
	m, _ = updateModel(m, cmd())
	if !m.fontEditor.previewOpen || m.fontEditor.pending == nil || m.fontSettings != nil {
		t.Fatalf("preview: %s", m.fontEditor.status)
	}
	if !strings.Contains(m.fontEditor.preview, "SHA-256") || !strings.Contains(m.fontEditor.preview, "Gentium Plus") || !strings.Contains(m.fontEditor.preview, "packaged-catalog") {
		t.Fatal("preview missing identity")
	}
	for _, size := range [][2]int{{20, 8}, {40, 14}, {80, 24}} {
		m.resize(size[0], size[1])
		view := m.fontsView()
		if lipgloss.Width(view.Content) > size[0] || lipgloss.Height(view.Content) > size[1] {
			t.Fatal("preview exceeds viewport")
		}
	}
	m, _ = updateModel(m, fontApplyKey())
	if m.fontEditor.open || m.fontSettings == nil || len(m.fontSettings.Rules) != 1 || len(m.fontSettings.FallbackResources) != 1 {
		t.Fatal("preview not applied")
	}
	m.textarea.SetValue("office")
	m, cmd = updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	if m.result == nil || !strings.Contains(renderResult(m.result), "Discovery resolved") {
		t.Fatalf("missing result diagnostics: %s", m.status)
	}
}
func TestFontRequestSubstitutionAndRetainedResult(t *testing.T) {
	m := requestModel()
	m.textarea.SetValue("retained")
	m.fontEditor.open = false
	m, cmd := updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	old := m.result
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m.fontEditor.path.SetValue("Missing ; 700 ; italic")
	m, cmd = updateModel(m, fontApplyKey())
	m, _ = updateModel(m, cmd())
	if m.result != old || m.fontEditor.pending != nil || !strings.Contains(m.fontEditor.status, "failed") {
		t.Fatal("failed request altered result")
	}
	m, _ = updateModel(m, fontControl('t'))
	m, cmd = updateModel(m, fontApplyKey())
	m, _ = updateModel(m, cmd())
	if m.result != old || m.fontSettings != nil || !strings.Contains(m.fontEditor.preview, "SUBSTITUTION") {
		t.Fatal("substitution not previewed before apply")
	}
	m, _ = updateModel(m, fontControl('e'))
	if m.fontEditor.pending != nil {
		t.Fatal("editing retained pending apply")
	}
}
func TestFontRequestCancellationAndStalePreview(t *testing.T) {
	for _, action := range []tea.Msg{tea.PasteMsg{Content: "changed"}, keyPress(tea.KeyEscape), keyPress(tea.KeyF6), fontControl('t'), fontControl('d')} {
		m := requestModel()
		entered := make(chan struct{})
		done := make(chan tea.Msg, 1)
		m.fontEditor.resolve = func(ctx context.Context, _ fontbackend.DescriptorSettings) (*fontbackend.Settings, []fontbackend.FontDiagnostic, error) {
			close(entered)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}
		m, cmd := updateModel(m, fontControl('p'))
		go func() { done <- cmd() }()
		<-entered
		m, _ = updateModel(m, action)
		m, _ = updateModel(m, <-done)
		if m.fontSettings != nil || m.fontEditor.pending != nil || m.fontEditor.previewOpen {
			t.Fatal("cancelled result published")
		}
	}
	m := requestModel()
	m, cmd := updateModel(m, fontControl('p'))
	loaded := cmd()
	m, _ = updateModel(m, tea.PasteMsg{Content: "changed"})
	m, _ = updateModel(m, loaded)
	if m.fontEditor.pending != nil {
		t.Fatal("stale preview accepted")
	}
}
func TestRequestSyntaxAndMappingOrder(t *testing.T) {
	f := newFontEditor()
	f.path.SetValue("Missing, Gentium Plus ; 400 ; normal")
	f.substitute = true
	f.rules.SetValue("serif | 700 | italic | Gentium ; 400 ; normal\nfallback | * | * | Gentium Plus ; 400 ; normal")
	input, err := f.descriptorSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Default.Families) != 2 || input.Default.Families[0] != "Missing" || !input.Rules[0].Request.AllowSubstitution || input.Rules[0].Weight != 700 || len(input.Fallbacks) != 1 {
		t.Fatalf("%+v", input)
	}
	for _, bad := range []string{"Gentium", "Gentium ; 450 ; normal", "Gentium ; 400 ; sideways", ",Gentium ; 400 ; normal"} {
		if _, err := parseFontRequest(bad, false); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
