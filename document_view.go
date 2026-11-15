package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
)

func (m model) documentView() tea.View {
	d := m.document
	panel := make([]string, m.textarea.Height())
	panel[0] = "F4: select a block subtree. F5: OpenType fonts."
	if d.source != nil {
		n := d.source.Nodes[d.selected]
		panel[0] = fmt.Sprintf("%q · node %d · %q", d.source.Name, n.ID, n.Path)
		if len(panel) > 1 {
			panel[1] = fmt.Sprintf("Preview: %q", d.source.Preview(n.ID, 160))
		}
	}
	for i := range panel {
		panel[i] = ansi.Truncate(panel[i], m.width, "…")
	}
	widthLabel, resultLabel := "  Width (CSS px): ", "  Retained document · tab to scroll"
	if m.focus == focusWidth {
		widthLabel = "> Width (CSS px): "
	} else {
		resultLabel = "> Retained document · tab to scroll"
	}
	fonts := "F5 fonts required"
	if m.fontSettings != nil {
		fonts = "OpenType"
	}
	v := tea.NewView(strings.Join([]string{
		ansi.Truncate("Document experiment · "+string(d.breaker)+" · "+fonts, m.width, "…"),
		"  Selected block · read-only", strings.Join(panel, "\n"), widthLabel + d.widthInput.View(),
		ansi.Truncate(m.status, m.width, "…"), resultLabel, m.viewport.View(), m.helpView(),
	}, "\n"))
	v.AltScreen = true
	v.WindowTitle = "tytui · Document experiment"
	if m.focus == focusWidth {
		v.Cursor = d.widthInput.Cursor()
		if v.Cursor != nil {
			v.Cursor.Y += 2 + m.textarea.Height()
			v.Cursor.X += lipgloss.Width(widthLabel)
		}
	}
	return v
}

// The inspector reports retained values in their native coordinate domains.
// It never resolves CSS, measures fonts, reshapes glyphs or composes placements.
func renderDocumentResult(s *documentSnapshot) string {
	r := s.result
	doc := r.Document()
	geometry := r.Geometry()
	var b strings.Builder
	fmt.Fprintf(&b, "Retained document · %s · containing width %.2f CSS px\nDocument %q\nSHA-256 %s\nSelected DOM node %d · %q\nInput controls may differ from this snapshot.\n\n", s.breaker, s.width, s.name, s.documentID, s.selected, s.path)
	b.WriteString("Profile: normal CSSLineHeight; owner font strut; NoHyphenation; AlignBottomMarginEdge; zero side skips; outer LTR flow.\nCSS boxes: px, relative to parent content origin. Paragraph placements: bp, relative to owner content origin.\n\n")
	var boxes func(*box.LayoutNode, int)
	boxes = func(n *box.LayoutNode, depth int) {
		fmt.Fprintf(&b, "%sBox %d · %v", strings.Repeat("  ", depth), n.BoxID, n.Box)
		if src, ok := doc.NodeSource(n.NodeID); ok {
			fmt.Fprintf(&b, " → DOM node %d %q", src.Locator.NodeID, src.Locator.Path)
		} else {
			b.WriteString(" · anonymous")
		}
		b.WriteByte('\n')
		if payload, ok := doc.Leaf(n.NodeID); ok {
			fmt.Fprintf(&b, "  Diagnostic %s %q · no paragraph interior\n", payload.Kind, payload.Label)
			if payload.Image != nil {
				info := payload.Image.Info()
				fmt.Fprintf(&b, "  Retained image %q · %s · %dx%d pixels · SHA-256 %s\n", info.Path, info.MIME, info.Width, info.Height, info.SHA256)
			}
		}
		if g, ok := geometry[n.BoxID]; ok {
			fmt.Fprintf(&b, "  parent Box %d · border %+v · content %+v CSS px\n", g.RelativeTo, g.Frame, g.Content)
		}
		for _, child := range n.Children {
			boxes(child, depth+1)
		}
	}
	boxes(doc.Root(), 0)
	for _, group := range r.Groups() {
		g, _ := r.Group(group.GroupID)
		fmt.Fprintf(&b, "\nGroup %d → owner Box %d · paragraph context DOM node %d %q\nLogical text %q · direction %s\n", group.GroupID, group.OwnerID, group.Paragraph.Locator.NodeID, group.Paragraph.Locator.Path, g.Input.Text, g.Input.Paragraph.Style.Direction)
		for _, p := range g.Paragraph.VList.Lines() {
			l := p.Horizontal
			fit := "within capacity"
			if l.Fit == khipu.FitOverfull {
				fit = "OVERFULL"
			} else if l.Fit == khipu.FitUnderfull {
				fit = "underfull"
			}
			fmt.Fprintf(&b, "  Line %d · bytes [%d,%d) · %s\n    actual %.3f / target %.3f bp · residual %+.3f · ratio %.3g · badness %d\n    x %.3f top %.3f baseline %.3f height %.3f bp\n", p.Index+1, l.Source.Start, l.Source.End, fit, l.Width.Points(), l.TargetWidth.Points(), l.Residual.Points(), l.Ratio, l.Badness, p.X, p.Top, p.Baseline, p.Height)
		}
		for _, span := range g.Input.TextSources {
			fmt.Fprintf(&b, "  text bytes [%d,%d) → Box %d → DOM node %d %q bytes [%d,%d) %s\n", span.Start, span.End, span.BoxID, span.Locator.NodeID, span.Locator.Path, span.NodeStart, span.NodeEnd, span.Kind)
		}
		for _, a := range g.Input.AtomicSources {
			fmt.Fprintf(&b, "  Atomic %d · bytes [%d,%d) → Box %d → DOM node %d %q\n", a.ID, a.Start, a.End, a.BoxID, a.Locator.NodeID, a.Locator.Path)
			for _, p := range g.Paragraph.VList.AtomicPlacements() {
				if p.ID == a.ID {
					fmt.Fprintf(&b, "    line %d · border origin (%.3f, %.3f) bp in owner Box %d\n", p.Line+1, p.X, p.Y, group.OwnerID)
				}
			}
		}
		for _, p := range g.Input.Paint {
			fmt.Fprintf(&b, "  paint bytes [%d,%d): %q (rendering bypass)\n", p.Start, p.End, p.Paint.Color)
		}
	}
	b.WriteString("\n" + fontReport(s.fonts, s.diagnostics))
	if fonts := s.fontSnapshot; fonts != nil {
		fmt.Fprintf(&b, "\nRetained font mapping: default resource %s\n", fonts.DefaultResource.ID())
		for i, rule := range fonts.Rules {
			fmt.Fprintf(&b, "  rule %d: family %q weight %d slant %q → resource %s\n", i+1, rule.Family, rule.Weight, rule.Slant, rule.Resource.ID())
		}
		for i, resource := range fonts.FallbackResources {
			fmt.Fprintf(&b, "  fallback %d → resource %s\n", i+1, resource.ID())
		}
		for _, info := range s.fonts {
			fmt.Fprintf(&b, "  resource %s → %q · SHA-256 %s\n", info.ResourceID, info.Path, info.SHA256)
		}
	}
	for _, w := range s.warnings {
		fmt.Fprintf(&b, "\nSource note: %s", w)
	}
	fmt.Fprintf(&b, "\nRetained HTML source (embedded CSS included):\n%q\n", s.originalHTML)
	b.WriteString("\nCtrl+S exports this retained proof. Diagnostic boxes/glyphs; no pagination, CSS painting or document replay.\n")
	return b.String()
}
