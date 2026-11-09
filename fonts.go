package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"tytui/fontbackend"
)

type fontEditor struct {
	open                       bool
	path, size                 textinput.Model
	rules                      textarea.Model
	focus                      int
	revision, nextID, activeID uint64
	status                     string
}
type fontsLoadedMsg struct {
	id, revision uint64
	settings     *fontbackend.Settings
	infos        []fontbackend.Info
	err          error
}

func newFontEditor() fontEditor {
	p := textinput.New()
	p.Prompt = ""
	p.Placeholder = "/path/to/font.ttf"
	p.CharLimit = 0
	p.SetVirtualCursor(false)
	s := textinput.New()
	s.Prompt = ""
	s.SetValue("12")
	s.SetVirtualCursor(false)
	r := textarea.New()
	r.Placeholder = "serif | 700 | normal | /path/to/bold.ttf"
	r.CharLimit = 0
	r.MaxHeight = 0
	r.MaxWidth = 0
	r.ShowLineNumbers = false
	r.SetVirtualCursor(false)
	return fontEditor{path: p, size: s, rules: r, status: "Default face is used for unmatched styles. Rows match in order; * means any."}
}
func (f *fontEditor) setFocus(n int) tea.Cmd {
	f.path.Blur()
	f.size.Blur()
	f.rules.Blur()
	f.focus = n
	switch n {
	case 0:
		return f.path.Focus()
	case 1:
		return f.size.Focus()
	default:
		return f.rules.Focus()
	}
}
func (f *fontEditor) settings() (*fontbackend.Settings, error) {
	size, err := parseWidth(f.size.Value())
	if err != nil {
		return nil, fmt.Errorf("Font size: %w", err)
	}
	s := &fontbackend.Settings{DefaultPath: strings.TrimSpace(f.path.Value()), Size: size}
	for i, line := range strings.Split(f.rules.Value(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("row %d: use family | weight | slant | font path", i+1)
		}
		for j := range parts {
			parts[j] = strings.TrimSpace(parts[j])
			if parts[j] == "*" && j < 3 {
				parts[j] = ""
			}
		}
		weight := 0
		if parts[1] != "" {
			weight, err = strconv.Atoi(parts[1])
			if err != nil || weight < 1 || weight > 1000 {
				return nil, fmt.Errorf("row %d: weight must be * or 1–1000", i+1)
			}
		}
		s.Rules = append(s.Rules, fontbackend.Rule{Family: parts[0], Weight: weight, Slant: parts[2], Path: parts[3]})
	}
	return s, nil
}
func (m *model) toggleFonts() tea.Cmd {
	m.fontEditor.open = !m.fontEditor.open
	if !m.fontEditor.open {
		m.fontEditor.revision++
		m.fontEditor.path.Blur()
		m.fontEditor.size.Blur()
		m.fontEditor.rules.Blur()
		return m.setFocus(m.focus)
	}
	m.browser.open = false
	m.textarea.Blur()
	m.widthInput.Blur()
	return m.fontEditor.setFocus(0)
}
func (m model) updateFonts(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := &m.fontEditor
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			cmd := m.toggleFonts()
			return m, cmd
		case "tab":
			cmd := f.setFocus((f.focus + 1) % 3)
			return m, cmd
		case "shift+tab":
			cmd := f.setFocus((f.focus + 2) % 3)
			return m, cmd
		case "ctrl+s":
			s, err := f.settings()
			if err != nil {
				f.status = err.Error()
				return m, nil
			}
			f.nextID++
			f.activeID = f.nextID
			id, rev := f.activeID, f.revision
			f.status = "Loading and validating fonts…"
			return m, func() tea.Msg {
				r, err := fontbackend.LoadSettings(context.Background(), *s)
				var infos []fontbackend.Info
				if r != nil {
					infos = r.Infos()
				}
				return fontsLoadedMsg{id, rev, s, infos, err}
			}
		}
	}
	before := f.path.Value() + "\x00" + f.size.Value() + "\x00" + f.rules.Value()
	var cmd tea.Cmd
	switch f.focus {
	case 0:
		f.path, cmd = f.path.Update(msg)
	case 1:
		f.size, cmd = f.size.Update(msg)
	default:
		f.rules, cmd = f.rules.Update(msg)
	}
	if before != f.path.Value()+"\x00"+f.size.Value()+"\x00"+f.rules.Value() {
		f.revision++
		f.status = "Draft changed · ctrl+s validates and applies"
	}
	return m, cmd
}
func (m model) fontsView() tea.View {
	f := m.fontEditor
	label := func(n int, s string) string {
		if f.focus == n {
			return "> " + s
		}
		return "  " + s
	}
	mode := "synthetic"
	if m.fontSettings != nil {
		mode = "OpenType"
	}
	content := strings.Join([]string{"Fonts · current backend: " + mode, label(0, "Default font file"), f.path.View(), label(1, "Plain-text size (bp); CSS sizes override"), f.size.View(), label(2, "Mappings: family | weight | slant | path (* = any)"), f.rules.View(), ansi.Truncate(f.status, m.width, "…"), "Tab focus · Ctrl+S apply · F6 synthetic · F5/Esc close"}, "\n")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	content = strings.Join(lines, "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "tytui · Fonts"
	switch f.focus {
	case 0:
		v.Cursor = f.path.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y += 2
		}
	case 1:
		v.Cursor = f.size.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y += 4
		}
	default:
		v.Cursor = f.rules.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y += 6
		}
	}
	return v
}
