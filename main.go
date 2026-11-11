package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/npillmayer/khipu/dimen"
	"tytui/fontbackend"
	"tytui/htmlinput"
	"tytui/pipeline"
)

type focus int

const (
	focusText focus = iota
	focusWidth
	focusResults
)

type job struct {
	input    pipeline.Input
	revision uint64
}
type layoutMsg struct {
	id, revision uint64
	result       *pipeline.Result
	err          error
}
type pipelineRunner func(context.Context, pipeline.Input) (*pipeline.Result, error)

type model struct {
	fontEditor                 fontEditor
	fontSettings               *fontbackend.Settings
	browser                    htmlBrowser
	source                     *htmlinput.Extraction
	width, height              int
	textarea                   textarea.Model
	widthInput                 textinput.Model
	viewport                   viewport.Model
	help                       help.Model
	focus                      focus
	status                     string
	result                     *pipeline.Result
	breaker                    pipeline.Breaker
	history                    []*pipeline.Result
	historyIndex               int
	exporting                  bool
	export                     func(*pipeline.Result) (string, error)
	revision, nextID, activeID uint64
	cancel                     context.CancelFunc
	pending                    *job
	run                        pipelineRunner
}

func initialModel() model {
	ta := textarea.New()
	ta.Placeholder = "Enter a paragraph…"
	ta.SetValue("A paragraph begins with a few words. Change the text or width, then run the layout to inspect its lines.")
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.MaxWidth = 0
	ta.ShowLineNumbers = false
	ta.SetVirtualCursor(false)
	ta.Focus()
	wi := textinput.New()
	wi.Prompt = ""
	wi.SetValue("120")
	wi.CharLimit = 24
	wi.SetVirtualCursor(false)
	vp := viewport.New()
	vp.SoftWrap = true
	vp.SetContent("Run the layout to inspect lines and diagnostics.")
	m := model{fontEditor: newFontEditor(), browser: newHTMLBrowser(), textarea: ta, widthInput: wi, viewport: vp, help: help.New(), run: pipeline.Run,
		breaker: pipeline.FirstFit, historyIndex: -1,
		export: func(r *pipeline.Result) (string, error) { return exportSVG(r, ".") },
		status: "Ready · synthetic font · firstfit · LTR"}
	m.resize(80, 24)
	return m
}
func (m model) Init() tea.Cmd { return nil }

// Width is entered in big points (1/72 inch), not terminal cells or TeX points.
func parseWidth(value string) (dimen.DU, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	scaled := math.Round(n * float64(dimen.BP))
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || scaled < 1 || scaled >= float64(dimen.Fil) {
		return 0, fmt.Errorf("Width must be a positive finite number below %.2f bp", float64(dimen.Fil)/float64(dimen.BP))
	}
	return dimen.DU(scaled), nil
}

func (m *model) invalidate() {
	m.revision++
	if m.cancel != nil {
		m.cancel()
	}
	m.pending = nil
	m.result = nil
	m.status = "Input changed · press ctrl+r to run"
	m.viewport.SetContent("No current layout. Run again after editing.")
	m.viewport.GotoTop()
}

func (m *model) launch(j job) tea.Cmd {
	m.nextID++
	m.activeID = m.nextID
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	id, run := m.activeID, m.run
	m.status = "Running layout…"
	return func() tea.Msg {
		defer cancel()
		result, err := run(ctx, j.input)
		return layoutMsg{id: id, revision: j.revision, result: result, err: err}
	}
}

func (m *model) requestRun() tea.Cmd {
	width, err := parseWidth(m.widthInput.Value())
	if err != nil {
		m.status = err.Error()
		return nil
	}
	j := job{input: pipeline.Input{Text: m.textarea.Value(), Width: width, Breaker: m.breaker, Source: m.source.Clone(), Fonts: m.fontSettings.Clone()}, revision: m.revision}
	m.result = nil
	m.viewport.SetContent("Waiting for layout…")
	m.viewport.GotoTop()
	if m.activeID != 0 {
		m.cancel()
		m.pending = &j
		m.status = "Waiting for previous run to stop…"
		return nil
	}
	return m.launch(j)
}

func (m *model) setFocus(f focus) tea.Cmd {
	m.textarea.Blur()
	m.widthInput.Blur()
	m.focus = f
	switch f {
	case focusText:
		return m.textarea.Focus()
	case focusWidth:
		return m.widthInput.Focus()
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fontsLoadedMsg:
		f := &m.fontEditor
		if msg.id != f.activeID {
			return m, nil
		}
		f.activeID = 0
		if msg.revision != f.revision || !f.open {
			return m, nil
		}
		if f.cancel != nil {
			f.cancel()
			f.cancel = nil
		}
		if msg.err != nil {
			f.status = "Font load failed: " + msg.err.Error()
			f.preview = fontReport(msg.infos, msg.diagnostics) + "\n" + f.status
			f.previewOpen = true
			f.previewOffset = 0
			return m, nil
		}
		if msg.preview {
			f.pending = msg.settings.Clone()
			f.preview = fontReport(msg.infos, msg.diagnostics)
			f.previewOpen = true
			f.previewOffset = 0
			f.status = "Validated · Ctrl+S applies this selection"
			return m, nil
		}
		m.fontSettings = msg.settings.Clone()
		m.invalidate()
		m.status = fmt.Sprintf("OpenType enabled · %d faces · ctrl+r to run", len(msg.infos))
		var report strings.Builder
		for _, info := range msg.infos {
			fmt.Fprintf(&report, "Font %q · %s %s\nSHA-256 %s\n", info.Path, info.Family, info.Subfamily, info.SHA256)
			for _, warning := range info.Warnings {
				fmt.Fprintf(&report, "Warning: %s\n", warning)
			}
		}
		m.viewport.SetContent(report.String())
		cmd := m.toggleFonts()
		return m, cmd
	case htmlLoadedMsg:
		if msg.id != m.browser.activeID {
			return m, nil
		}
		m.browser.activeID = 0
		if msg.revision != m.browser.revision {
			return m, nil
		}
		if msg.err != nil {
			m.browser.status = "Load failed: " + msg.err.Error()
			return m, nil
		}
		m.browser.doc = msg.doc
		m.browser.selected = 0
		m.browser.top = 0
		m.browser.collapsed = make(map[int]bool)
		m.browser.status = fmt.Sprintf("Loaded %d DOM nodes · choose an inline-text subtree", len(msg.doc.Nodes))
		m.browser.inspectSelected()
		cmd := m.browser.setFocus(treeFocus)
		return m, cmd
	case exportMsg:
		m.exporting = false
		if msg.err != nil {
			m.status = "SVG export failed: " + msg.err.Error()
		} else {
			m.status = "SVG saved: " + msg.path
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case layoutMsg:
		if msg.id != m.activeID {
			return m, nil
		}
		m.activeID, m.cancel = 0, nil
		if m.pending != nil {
			j := *m.pending
			m.pending = nil
			cmd := m.launch(j)
			return m, cmd
		}
		if msg.revision != m.revision {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Layout failed · see results"
			m.viewport.SetContent(msg.err.Error())
		} else {
			m.result = msg.result
			m.history = append(m.history, msg.result)
			m.historyIndex = len(m.history) - 1
			m.status = fmt.Sprintf("Complete · %d lines · %s · %s", msg.result.Frozen.LineCount(), msg.result.Input.Breaker, msg.result.Input.Direction)
			m.viewport.SetContent(renderResult(msg.result))
		}
		m.viewport.GotoTop()
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "f5" {
			cmd := m.toggleFonts()
			return m, cmd
		}
		if msg.String() == "f6" {
			m.fontEditor.clearPending()
			m.fontEditor.revision++
			m.fontSettings = nil
			m.invalidate()
			m.status = "Synthetic backend selected"
			m.fontEditor.status = m.status
			return m, nil
		}
		if m.fontEditor.open && !key.Matches(msg, keys.Quit) {
			return m.updateFonts(msg)
		}
		if msg.String() == "f4" || msg.String() == "ctrl+o" {
			cmd := m.toggleBrowser()
			return m, cmd
		}
		if m.browser.open && !key.Matches(msg, keys.Quit) {
			return m.updateBrowser(msg)
		}
		switch {
		case key.Matches(msg, keys.Breaker):
			if m.breaker == pipeline.FirstFit {
				m.breaker = pipeline.KnuthPlass
			} else {
				m.breaker = pipeline.FirstFit
			}
			m.invalidate()
			return m, nil
		case key.Matches(msg, keys.History):
			if len(m.history) == 0 {
				m.status = "No completed layouts yet"
				return m, nil
			}
			if m.activeID != 0 {
				m.status = "Wait for the current run before browsing saved layouts"
				return m, nil
			}
			if m.result != nil {
				m.historyIndex = (m.historyIndex + len(m.history) - 1) % len(m.history)
			}
			m.result = m.history[m.historyIndex]
			m.status = fmt.Sprintf("Saved layout %d/%d · %s · input controls unchanged", m.historyIndex+1, len(m.history), m.result.Input.Breaker)
			m.viewport.SetContent(renderResult(m.result))
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, keys.Export):
			if m.result == nil {
				m.status = "Run a layout or select a saved layout before exporting"
				return m, nil
			}
			if m.exporting {
				return m, nil
			}
			m.exporting = true
			m.status = "Exporting displayed layout…"
			result, export := m.result, m.export
			return m, func() tea.Msg { path, err := export(result); return exportMsg{path, err} }
		case key.Matches(msg, keys.Quit):
			m.fontEditor.clearPending()
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case key.Matches(msg, keys.Run):
			cmd := m.requestRun()
			return m, cmd
		case key.Matches(msg, keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.resize(m.width, m.height)
			return m, nil
		case msg.String() == "tab":
			cmd := m.setFocus((m.focus + 1) % 3)
			return m, cmd
		case msg.String() == "shift+tab":
			cmd := m.setFocus((m.focus + 2) % 3)
			return m, cmd
		case msg.String() == "enter" && m.focus == focusWidth:
			cmd := m.requestRun()
			return m, cmd
		}
	}
	if m.fontEditor.open {
		return m.updateFonts(msg)
	}
	if m.browser.open {
		return m.updateBrowser(msg)
	}
	beforeText, beforeWidth := m.textarea.Value(), m.widthInput.Value()
	var cmd tea.Cmd
	switch m.focus {
	case focusText:
		m.textarea, cmd = m.textarea.Update(msg)
	case focusWidth:
		m.widthInput, cmd = m.widthInput.Update(msg)
	case focusResults:
		m.viewport, cmd = m.viewport.Update(msg)
	}
	// Includes paste messages, not just keypresses.
	if beforeText != m.textarea.Value() || beforeWidth != m.widthInput.Value() {
		if beforeText != m.textarea.Value() {
			m.source = nil
		}
		m.invalidate()
	}
	return m, cmd
}

func (m *model) resize(w, h int) {
	m.width, m.height = w, h
	w = max(1, w)
	m.fontEditor.path.SetWidth(max(1, w-2))
	m.fontEditor.size.SetWidth(min(24, max(1, w-2)))
	m.fontEditor.rules.SetWidth(w)
	m.fontEditor.rules.SetHeight(max(2, h-9))
	m.help.SetWidth(w)
	m.textarea.SetWidth(w)
	helpHeight := lipgloss.Height(m.helpView())
	m.textarea.SetHeight(max(1, min(6, h/4, h-7-helpHeight)))
	m.widthInput.SetWidth(min(24, max(1, w-24)))
	m.widthInput.SetCursor(m.widthInput.Position())
	m.viewport.SetWidth(w)
	// Title, text label, width row, status, results label, and help.
	m.viewport.SetHeight(max(1, h-m.textarea.Height()-5-helpHeight))
	m.browser.path.SetWidth(max(1, w-12))
	m.browser.path.SetCursor(m.browser.path.Position())
	m.browser.inspect.SetWidth(w)
	m.browser.inspect.SetHeight(max(1, h-6-max(1, (h-6)/2)))
	m.browser.ensureVisible(max(1, (h-6)/2))
}

func (m model) helpView() string {
	if !m.help.ShowAll {
		return "^R run · F4 HTML · F5 fonts · F1 help"
	}
	return strings.Join([]string{
		"ctrl+r run · ctrl+s export SVG",
		"f2 breaker · f3 saved layouts",
		"f5 fonts · f6 synthetic backend",
		"tab/shift+tab focus · F4 HTML",
		"enter newline / run from width",
		"↑/↓ pgup/pgdn scroll results",
		"f1 close help · ctrl+c quit",
	}, "\n")
}

func (m model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.WindowTitle = "tytui · Paragraph workbench"
	if m.width < 40 || m.height < 14 {
		v.SetContent("Resize to at least 40 × 14.\nctrl+c to quit.")
		return v
	}
	if m.fontEditor.open {
		return m.fontsView()
	}
	if m.browser.open {
		return m.browserView()
	}
	label := func(s string, f focus) string {
		if m.focus == f {
			return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render("> " + s)
		}
		return "  " + s
	}
	title := "Paragraph workbench · " + string(m.breaker)
	if m.fontSettings != nil {
		title += " · OpenType"
	} else {
		title += " · synthetic"
	}
	textLabel := label("Paragraph text", focusText)
	if m.source != nil {
		textLabel = label("HTML selection · edit detaches map", focusText)
	}
	widthLabel := label("Width (bp, 1/72 in): ", focusWidth)
	content := strings.Join([]string{
		ansi.Truncate(title, m.width, "…"), textLabel, m.textarea.View(), widthLabel + m.widthInput.View(),
		ansi.Truncate(m.status, m.width, "…"), label("Results · tab here to scroll", focusResults),
		m.viewport.View(), m.helpView(),
	}, "\n")
	v.SetContent(content)
	switch m.focus {
	case focusText:
		v.Cursor = m.textarea.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y += 2
		}
	case focusWidth:
		v.Cursor = m.widthInput.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y += 2 + m.textarea.Height()
			v.Cursor.X += lipgloss.Width(widthLabel)
		}
	}
	return v
}

func main() {
	final, err := tea.NewProgram(initialModel()).Run()
	if m, ok := final.(model); ok && m.cancel != nil {
		m.cancel()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error running TUI:", err)
		os.Exit(1)
	}
}
