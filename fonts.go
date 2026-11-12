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
	"github.com/npillmayer/typography"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	fontdiscovery "github.com/npillmayer/typography/discovery"
)

type fontEditor struct {
	requests, substitute bool
	cancel               context.CancelFunc
	resolve              func(context.Context, fontdiscovery.DescriptorSettings) (*fontdiscovery.Resolution, []fontdiscovery.FontDiagnostic, error)
	pending              *fontdiscovery.Resolution
	preview              string
	previewOpen          bool
	previewOffset        int

	open                       bool
	path, size                 textinput.Model
	rules                      textarea.Model
	focus                      int
	revision, nextID, activeID uint64
	status                     string
}
type fontsLoadedMsg struct {
	preview      bool
	diagnostics  []fontdiscovery.FontDiagnostic
	id, revision uint64
	settings     *fontdiscovery.Resolution
	infos        []typography.Info
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
	return fontEditor{resolve: offlineFontDiscovery().ResolveSettings, path: p, size: s, rules: r, status: "Default face is used for unmatched styles. Rows match in order; * means any."}
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
func (f *fontEditor) settings() (*fontdiscovery.Resolution, error) {
	size, err := parseWidth(f.size.Value())
	if err != nil {
		return nil, fmt.Errorf("Font size: %w", err)
	}
	s := &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: strings.TrimSpace(f.path.Value()), Size: size}}
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
		if parts[0] == "fallback" {
			if parts[1] != "" || parts[2] != "" {
				return nil, fmt.Errorf("row %d: fallback requires * weight and slant", i+1)
			}
			s.Fallbacks = append(s.Fallbacks, parts[3])
			continue
		}
		s.Rules = append(s.Rules, typography.Rule{Family: parts[0], Weight: weight, Slant: parts[2], Path: parts[3]})
	}
	return s, nil
}
func (m *model) toggleFonts() tea.Cmd {
	m.fontEditor.open = !m.fontEditor.open
	if !m.fontEditor.open {
		m.fontEditor.clearPending()
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
		case "ctrl+d":
			f.clearPending()
			f.requests = !f.requests
			f.revision++
			f.status = "Offline catalog: Gentium Plus Regular; serif maps to it. Edit targets; Ctrl+P previews."
			return m, nil
		case "ctrl+t":
			f.clearPending()
			f.substitute = !f.substitute
			f.revision++
			f.status = "Substitution policy changed; preview again."
			return m, nil
		case "ctrl+e":
			f.clearPending()
			f.revision++
			return m, nil
		case "up", "down", "pgup", "pgdown":
			if f.previewOpen {
				delta := 1
				if key.String() == "pgup" || key.String() == "pgdown" {
					delta = max(1, m.height-3)
				}
				if key.String() == "up" || key.String() == "pgup" {
					delta = -delta
				}
				f.previewOffset = max(0, min(max(0, len(strings.Split(ansi.Hardwrap(f.preview, max(1, m.width), true), "\n"))-max(1, m.height-3)), f.previewOffset+delta))
				return m, nil
			}
		case "ctrl+s", "ctrl+p":
			if key.String() == "ctrl+s" && f.pending != nil {
				m.fontSettings = f.pending.Clone()
				m.invalidate()
				m.status = "OpenType enabled · ctrl+r to run"
				m.viewport.SetContent(f.preview)
				cmd := m.toggleFonts()
				return m, cmd
			}
			s, err := f.settings()
			var request fontdiscovery.DescriptorSettings
			if f.requests {
				request, err = f.descriptorSettings()
			}
			if err != nil {
				f.status = err.Error()
				return m, nil
			}
			f.clearPending()
			ctx, cancel := context.WithCancel(context.Background())
			f.cancel = cancel
			f.nextID++
			f.activeID = f.nextID
			id, rev := f.activeID, f.revision
			preview := f.requests || key.String() == "ctrl+p"
			requests, resolve := f.requests, f.resolve
			f.status = "Loading and validating fonts…"
			return m, func() tea.Msg {
				defer cancel()
				var diagnostics []fontdiscovery.FontDiagnostic
				var err error
				if requests {
					s, diagnostics, err = resolve(ctx, request)
				}
				var infos []typography.Info
				if err == nil {
					var r *typography.Resolver
					r, err = typography.LoadSettings(ctx, s.Settings, typography.Services{Loader: otadapter.Services{}, Coverage: otadapter.Services{}})
					if r != nil {
						infos = r.Infos()
					}
				}
				if err == nil {
					err = ctx.Err()
				}
				return fontsLoadedMsg{id: id, revision: rev, settings: s, infos: infos, err: err, preview: preview, diagnostics: diagnostics}
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
		f.clearPending()
		f.revision++
		f.status = "Draft changed · ctrl+p previews; ctrl+s applies"
	}
	return m, cmd
}
func (m model) fontsView() tea.View {
	f := m.fontEditor
	if f.previewOpen {
		lines := strings.Split(ansi.Hardwrap(f.preview, max(1, m.width), true), "\n")
		start := min(f.previewOffset, len(lines))
		end := min(len(lines), start+max(1, m.height-3))
		content := []string{"Font preview · Ctrl+S apply · Ctrl+E edit · Esc close"}
		content = append(content, lines[start:end]...)
		content = append(content, "↑/↓ PgUp/PgDn scroll · "+f.status)
		for i := range content {
			content[i] = ansi.Truncate(content[i], m.width, "…")
		}
		if len(content) > m.height && m.height > 0 {
			content = content[:m.height]
		}
		v := tea.NewView(strings.Join(content, "\n"))
		v.AltScreen = true
		return v
	}
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
	content := strings.Join([]string{"Fonts · current backend: " + mode, label(0, fontSourceLabel(f.requests, f.substitute)), f.path.View(), label(1, "Plain-text size (bp); CSS sizes override"), f.size.View(), label(2, "Mappings: family | weight | slant | target; fallback | * | * | target"), f.rules.View(), ansi.Truncate(f.status, m.width, "…"), "Tab focus · ^D file/request · ^T substitution · ^P preview · ^S apply · Esc close"}, "\n")
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

func (f *fontEditor) clearPending() {
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.pending = nil
	f.previewOpen = false
	f.preview = ""
	f.previewOffset = 0
}
func fontSourceLabel(requests, substitute bool) string {
	if !requests {
		return "Default font file (Ctrl+D for requests)"
	}
	policy := "strict"
	if substitute {
		policy = "allow substitution"
	}
	return "Request: families ; weight ; slant · " + policy
}
