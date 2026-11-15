package main

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	ot "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	"golang.org/x/net/html"

	"tytui/documentlayout"
	"tytui/fontselection"
	"tytui/htmlinput"
	"tytui/pipeline"
)

// Document work shares the paragraph scheduler, but keeps its controls/history
// separate. Published DOMs are read-only; selecting/reloading replaces a pointer.
type documentJob struct {
	source   *htmlinput.Document
	selected int
	width    float32
	breaker  pipeline.Breaker
	fonts    *fontselection.Request
}

// Only complete results escape the command. Rendering retains the services and
// immutable faces from this run, never the current font editor or source DOM.
type documentSnapshot struct {
	result                 *documentlayout.Result
	services               render.Services
	name, documentID, path string
	originalHTML           string
	selected               int
	width                  float32
	breaker                pipeline.Breaker
	fonts                  []typography.Info
	fontSnapshot           *typography.SelectedFonts
	diagnostics            []fontselection.FontDiagnostic
	warnings               []string
}

type documentWorkbench struct {
	widthInput   textinput.Model
	breaker      pipeline.Breaker
	source       *htmlinput.Document
	selected     int
	result       *documentSnapshot
	history      []*documentSnapshot
	historyIndex int
	lastError    string
	run          func(context.Context, documentJob) (*documentSnapshot, error)
	export       func(*documentSnapshot) (string, error)
}

func newDocumentWorkbench() documentWorkbench {
	w := textinput.New()
	w.Prompt = ""
	w.SetValue("420")
	w.CharLimit = 24
	w.SetVirtualCursor(false)
	return documentWorkbench{widthInput: w, breaker: pipeline.FirstFit, selected: -1, historyIndex: -1,
		run: runDocumentExperiment, export: func(r *documentSnapshot) (string, error) { return exportDocumentSVG(r, ".") }}
}

func parseDocumentWidth(value string) (float32, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || math.Round(n*49152) >= float64(dimen.Fil) {
		return 0, fmt.Errorf("Document width must be finite, nonnegative and below %.2f CSS px", float64(dimen.Fil)/49152)
	}
	return float32(n), nil
}

func runDocumentExperiment(ctx context.Context, input documentJob) (*documentSnapshot, error) {
	doc, err := documentlayout.Build(ctx, input.source, input.selected)
	if err != nil {
		return nil, fmt.Errorf("document input: %w", err)
	}
	if input.fonts == nil {
		return nil, fmt.Errorf("document proofs require OpenType fonts: use F5 to select a default face and any needed fallbacks; F6 synthetic remains available in paragraph mode")
	}
	services := ot.Services{}
	selection, err := fontselection.Prepare(ctx, input.fonts)
	if err != nil {
		return nil, fmt.Errorf("document fonts: %w", err)
	}
	resolver, err := typography.NewResolver(ctx, selection.Fonts, services)
	if err != nil {
		return nil, fmt.Errorf("document fonts: %w", err)
	}
	result, err := doc.Layout(ctx, documentlayout.LayoutRequest{Width: input.width, Breaker: paragraph.Breaker(input.breaker)}, documentlayout.Config{
		Resolver: resolver, NewShaper: func() (contracts.StreamingShaper, error) { return ot.NewStream(64) },
	})
	if err != nil {
		return nil, fmt.Errorf("document layout: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &documentSnapshot{result: result, services: render.Services{Backend: services, Identity: services},
		name: input.source.Name, documentID: input.source.ID, path: input.source.Nodes[input.selected].Path, selected: input.selected,
		originalHTML: input.source.OriginalHTML, width: input.width, breaker: input.breaker, fonts: resolver.Infos(), fontSnapshot: resolver.Snapshot(),
		diagnostics: slices.Clone(input.fonts.ResolutionDiagnostics), warnings: slices.Clone(input.source.Warnings)}, nil
}

func (m *model) cancelLayout() {
	m.revision++
	if m.cancel != nil {
		m.cancel()
	}
	m.pending = nil
}

func (m *model) toggleDocumentMode() tea.Cmd {
	m.cancelLayout()
	m.documentMode = !m.documentMode
	m.browser.documentMode = m.documentMode
	if m.documentMode {
		m.status = "Document experiment · F4 selects a block · F5 fonts · ctrl+r runs"
		m.refreshDocumentResult()
	} else {
		m.status = "Paragraph mode · retained paragraph · ctrl+r uses current controls"
		if m.result != nil {
			m.viewport.SetContent(renderResult(m.result))
		} else {
			m.viewport.SetContent("Run the paragraph layout to inspect lines and diagnostics.")
		}
		m.viewport.GotoTop()
	}
	if m.browser.open {
		m.browser.status = "Selection mode changed · Enter selects in the current mode"
		m.browser.inspectSelected()
		return nil
	}
	if m.documentMode {
		return m.setFocus(focusWidth)
	}
	return m.setFocus(focusText)
}

func (m *model) refreshDocumentResult() {
	text := "Select a block with F4, configure OpenType fonts with F5, then Ctrl+R."
	if m.document.result != nil {
		text = renderDocumentResult(m.document.result)
	}
	if m.document.lastError != "" {
		text = "Latest attempt failed: " + m.document.lastError + "\n\n" + text
	}
	m.viewport.SetContent(text)
	m.viewport.GotoTop()
}

func (m *model) documentFailure(err error) {
	m.document.lastError = err.Error()
	m.status = "Document failed · previous result retained · see results"
	m.refreshDocumentResult()
}

func (m *model) requestDocumentRun() tea.Cmd {
	w, err := parseDocumentWidth(m.document.widthInput.Value())
	if err != nil {
		m.documentFailure(err)
		return nil
	}
	if m.document.source == nil {
		m.documentFailure(fmt.Errorf("select a block subtree with F4 first"))
		return nil
	}
	j := job{document: &documentJob{source: m.document.source, selected: m.document.selected, width: w,
		breaker: m.document.breaker, fonts: m.fontSettings.Clone()}, revision: m.revision}
	m.document.lastError = ""
	m.refreshDocumentResult()
	if m.activeID != 0 {
		m.cancel()
		m.pending = &j
		m.status = "Waiting for previous run to stop · displayed document retained"
		return nil
	}
	return m.launch(j)
}

// Eligibility here is preliminary; complete profile validation and normalization
// happen asynchronously in the run command, never on every tree navigation key.
func documentCandidate(d *htmlinput.Document, id int) error {
	if d == nil || id < 0 || id >= len(d.Nodes) || d.Nodes[id].Type != html.ElementNode {
		return fmt.Errorf("select a block element")
	}
	display, err := d.Property(id, "display")
	if err != nil {
		return err
	}
	if display != "block" {
		return fmt.Errorf("select a display:block element (got %s)", display)
	}
	return nil
}

func (m *model) selectDocument() tea.Cmd {
	b := &m.browser
	if err := documentCandidate(b.doc, b.selected); err != nil {
		b.status = err.Error()
		return nil
	}
	m.document.source, m.document.selected = b.doc, b.selected
	m.invalidate()
	m.status = "Document block selected · ctrl+r validates and lays out · previous result retained"
	b.open = false
	b.path.Blur()
	return m.setFocus(focusWidth)
}

func (m *model) documentHistory() {
	d := &m.document
	if len(d.history) == 0 {
		m.status = "No completed document layouts yet"
		return
	}
	if m.activeID != 0 {
		m.status = "Wait for the current run before browsing saved layouts"
		return
	}
	d.historyIndex = (d.historyIndex + len(d.history) - 1) % len(d.history)
	d.result = d.history[d.historyIndex]
	d.lastError = ""
	m.status = fmt.Sprintf("Saved document %d/%d · controls unchanged", d.historyIndex+1, len(d.history))
	m.refreshDocumentResult()
}

func (m *model) requestDocumentExport() tea.Cmd {
	if m.document.result == nil {
		m.status = "Run a document layout before exporting"
		return nil
	}
	if m.exporting {
		return nil
	}
	m.exporting = true
	m.status = "Exporting displayed document…"
	result, export := m.document.result, m.document.export
	return func() tea.Msg { path, err := export(result); return exportMsg{path, err} }
}
