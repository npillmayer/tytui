package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"tytui/htmlinput"
)

type htmlLoadedMsg struct {
	id, revision uint64
	doc          *htmlinput.Document
	err          error
}
type browserFocus int

const (
	pathFocus browserFocus = iota
	treeFocus
	propertiesFocus
)

type htmlBrowser struct {
	open                       bool
	path                       textinput.Model
	doc                        *htmlinput.Document
	selected, top              int // selected is a document node ID; top is a visible row index.
	collapsed                  map[int]bool
	focus                      browserFocus
	inspect                    viewport.Model
	status                     string
	revision, nextID, activeID uint64
	load                       func(string) (*htmlinput.Document, error)
}

func newHTMLBrowser() htmlBrowser {
	path := textinput.New()
	path.Prompt = "HTML file: "
	path.Placeholder = "/path/to/document.html"
	path.CharLimit = 0
	path.SetVirtualCursor(false)
	view := viewport.New()
	view.SoftWrap = true
	return htmlBrowser{path: path, inspect: view, collapsed: make(map[int]bool), load: htmlinput.LoadFile, status: "Enter a local HTML filename, then press Enter"}
}
func (b *htmlBrowser) setFocus(f browserFocus) tea.Cmd {
	b.path.Blur()
	b.focus = f
	if f == pathFocus {
		return b.path.Focus()
	}
	return nil
}
func (b *htmlBrowser) visible() []int {
	if b.doc == nil {
		return nil
	}
	ids := []int{}
	var walk func(int)
	walk = func(i int) {
		ids = append(ids, i)
		if !b.collapsed[i] {
			for _, c := range b.doc.Nodes[i].Children {
				walk(c)
			}
		}
	}
	walk(0)
	return ids
}
func (b *htmlBrowser) ensureVisible(rows int) {
	ids := b.visible()
	pos := 0
	for i, id := range ids {
		if id == b.selected {
			pos = i
			break
		}
	}
	if pos < b.top {
		b.top = pos
	}
	if pos >= b.top+rows {
		b.top = pos - rows + 1
	}
	b.top = max(0, b.top)
}
func (b *htmlBrowser) inspectSelected() {
	if b.doc == nil {
		return
	}
	n := b.doc.Nodes[b.selected]
	var out strings.Builder
	fmt.Fprintf(&out, "Node %d · %q\nDocument %q\nPath %q\n\n", n.ID, n.Name, b.doc.Name, n.Path)
	for _, a := range n.Attributes {
		fmt.Fprintf(&out, "%q = %q\n", a.Key, a.Val)
	}
	fmt.Fprintf(&out, "\nText preview (decoded, raw whitespace)\n%q\n\nLibrary property values (not browser computed CSS)\n", b.doc.Preview(n.ID, 1000))
	for _, key := range []string{"display", "visibility", "color", "font-family", "font-size", "font-weight", "font-style", "white-space", "direction", "line-height"} {
		value, err := b.doc.Property(n.ID, key)
		if err != nil {
			fmt.Fprintf(&out, "%s: unavailable (%s)\n", key, err)
		} else {
			fmt.Fprintf(&out, "%s: %q\n", key, value)
		}
	}
	e, err := b.doc.Extract(n.ID)
	if err != nil {
		fmt.Fprintf(&out, "\nSelection unavailable: %s\n", err)
	} else {
		fmt.Fprintf(&out, "\nSelectable: %d bytes, %d source spans\nEnter in tree to use this paragraph.\n", len(e.Text), len(e.Spans))
		fmt.Fprintf(&out, "Processed text: %q\nBase direction: %s\n", e.Text, e.Direction)
		for _, s := range e.Styles {
			fmt.Fprintf(&out, "style [%d,%d): %s\n", s.Start, s.End, s.Style)
		}
		for _, s := range e.Spans {
			fmt.Fprintf(&out, "text [%d,%d) → node %d bytes [%d,%d) %s\n", s.Start, s.End, s.NodeID, s.NodeStart, s.NodeEnd, s.Kind)
		}
	}
	for _, w := range b.doc.Warnings {
		fmt.Fprintf(&out, "\nNote: %s", w)
	}
	out.WriteString("\nF5 configures fonts and style mappings; synthetic shaping uses size/language. Color remains metadata.")
	b.inspect.SetContent(out.String())
	b.inspect.GotoTop()
}

func (m *model) toggleBrowser() tea.Cmd {
	m.browser.open = !m.browser.open
	if m.browser.open {
		m.textarea.Blur()
		m.widthInput.Blur()
		return m.browser.setFocus(m.browser.focus)
	}
	m.browser.path.Blur()
	return m.setFocus(m.focus)
}

func (m *model) updateBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	b := &m.browser
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			cmd := m.toggleBrowser()
			return *m, cmd
		case "tab":
			cmd := b.setFocus((b.focus + 1) % 3)
			return *m, cmd
		case "shift+tab":
			cmd := b.setFocus((b.focus + 2) % 3)
			return *m, cmd
		case "enter":
			switch b.focus {
			case pathFocus:
				path := strings.TrimSpace(b.path.Value())
				if path == "" {
					b.status = "Enter a filename"
					return *m, nil
				}
				if b.activeID != 0 {
					b.status = "A file is still loading; retry when it finishes"
					return *m, nil
				}
				b.nextID++
				b.activeID = b.nextID
				id, revision, load := b.activeID, b.revision, b.load
				b.status = "Loading HTML…"
				return *m, func() tea.Msg { doc, err := load(path); return htmlLoadedMsg{id, revision, doc, err} }
			case treeFocus:
				if b.doc == nil {
					return *m, nil
				}
				extracted, err := b.doc.Extract(b.selected)
				if err != nil {
					b.status = err.Error()
					return *m, nil
				}
				previous := m.textarea.Value()
				m.textarea.SetValue(extracted.Text)
				// Textarea may have an internal line/size ceiling; never attach a map to
				// content it did not accept unchanged.
				if m.textarea.Value() != extracted.Text {
					m.textarea.SetValue(previous)
					b.status = "Selection cannot be represented unchanged by the text editor"
					return *m, nil
				}
				m.source = extracted.Clone()
				m.invalidate()
				m.status = "HTML paragraph selected · ctrl+r to lay out"
				b.open = false
				b.path.Blur()
				cmd := m.setFocus(focusText)
				return *m, cmd
			}
		}
		if b.focus == treeFocus && b.doc != nil {
			ids := b.visible()
			pos := 0
			for i, id := range ids {
				if id == b.selected {
					pos = i
					break
				}
			}
			switch key.String() {
			case "up", "k":
				pos = max(0, pos-1)
				b.selected = ids[pos]
			case "down", "j":
				pos = min(len(ids)-1, pos+1)
				b.selected = ids[pos]
			case "home":
				b.selected = ids[0]
			case "end":
				b.selected = ids[len(ids)-1]
			case "pgup":
				pos = max(0, pos-max(1, (m.height-6)/2))
				b.selected = ids[pos]
			case "pgdown":
				pos = min(len(ids)-1, pos+max(1, (m.height-6)/2))
				b.selected = ids[pos]
			case "right", "l":
				if b.collapsed[b.selected] {
					delete(b.collapsed, b.selected)
				} else if children := b.doc.Nodes[b.selected].Children; len(children) > 0 {
					b.selected = children[0]
				}
			case "left", "h":
				if !b.collapsed[b.selected] && len(b.doc.Nodes[b.selected].Children) > 0 {
					b.collapsed[b.selected] = true
				} else if parent := b.doc.Nodes[b.selected].Parent; parent >= 0 {
					b.selected = parent
				}
			}
			b.ensureVisible(max(1, (m.height-6)/2))
			b.inspectSelected()
			return *m, nil
		}
	}
	var cmd tea.Cmd
	switch b.focus {
	case pathFocus:
		before := b.path.Value()
		b.path, cmd = b.path.Update(msg)
		if before != b.path.Value() {
			b.revision++
			b.status = "Filename changed · Enter to load"
		}
	case propertiesFocus:
		b.inspect, cmd = b.inspect.Update(msg)
	}
	return *m, cmd
}

func (m model) browserView() tea.View {
	b := m.browser
	width := max(1, m.width)
	treeRows := max(1, (m.height-6)/2)
	var lines []string
	lines = append(lines, "HTML / DOM explorer · F4 or Esc back", b.path.View(), ansi.Truncate(b.status, width, "…"))
	marker := "  "
	if b.focus == treeFocus {
		marker = "> "
	}
	lines = append(lines, marker+"DOM tree · Enter selects")
	ids := b.visible()
	for i := 0; i < treeRows; i++ {
		row := ""
		if b.top+i < len(ids) {
			n := b.doc.Nodes[ids[b.top+i]]
			mark := "  "
			if n.ID == b.selected {
				mark = "> "
			}
			branch := "  "
			if len(n.Children) > 0 {
				branch = "- "
				if b.collapsed[n.ID] {
					branch = "+ "
				}
			}
			label := strconv.Quote(n.Name)
			for _, a := range n.Attributes {
				if a.Key == "id" {
					label += " id=" + strconv.Quote(a.Val)
				}
			}
			row = mark + strings.Repeat("  ", min(n.Depth, 20)) + branch + label + " " + strconv.Quote(b.doc.Preview(n.ID, 50))
			row = ansi.Truncate(row, width, "…")
		}
		lines = append(lines, row)
	}
	marker = "  "
	if b.focus == propertiesFocus {
		marker = "> "
	}
	lines = append(lines, marker+"Properties / preview · tab to scroll")
	lines = append(lines, b.inspect.View(), "tab focus · arrows tree/scroll · ^C quit")
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.WindowTitle = "tytui · HTML / DOM"
	if b.focus == pathFocus {
		v.Cursor = b.path.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y++
		}
	}
	return v
}
