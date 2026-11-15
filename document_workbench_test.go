package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/npillmayer/khipu/dimen"

	"tytui/documentlayout"
	"tytui/fontselection"
	"tytui/htmlinput"
	"tytui/pipeline"
)

func selectDocumentFixture(t *testing.T, m model) model {
	t.Helper()
	if !m.documentMode {
		m, _ = updateModel(m, keyPress(tea.KeyF7))
	}
	m, _ = updateModel(m, keyPress(tea.KeyF4))
	m.browser.setFocus(pathFocus)
	m.browser.path.SetValue("documentlayout/testdata/nested-auto.html")
	m, cmd := updateModel(m, keyPress(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("HTML load was not scheduled")
	}
	m, _ = updateModel(m, cmd())
	if m.browser.doc == nil {
		t.Fatal(m.browser.status)
	}
	m.browser.selected = findID(m, "root")
	if _, err := m.browser.doc.Extract(m.browser.selected); err == nil {
		t.Fatal("fixture should require document selection, not paragraph extraction")
	}
	m.browser.inspectSelected()
	m, _ = updateModel(m, keyPress(tea.KeyEnter))
	if m.browser.open || m.document.source == nil || m.focus != focusWidth {
		t.Fatal("document selection failed", m.browser.status)
	}
	return m
}

func documentFonts(t *testing.T, m model) model {
	t.Helper()
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m.fontEditor.path.SetValue(uiFont)
	m.fontEditor.rules.SetValue("fallback | * | * | ../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf")
	m, cmd := updateModel(m, fontApplyKey())
	if cmd == nil {
		t.Fatal("font loading was not scheduled")
	}
	m, _ = updateModel(m, cmd())
	if m.fontEditor.open || m.fontSettings == nil {
		t.Fatal(m.fontEditor.status)
	}
	return m
}

func runWorkbenchDocument(t *testing.T, m model) model {
	t.Helper()
	m, cmd := updateModel(m, runKey())
	if cmd == nil {
		t.Fatal(m.status)
	}
	m, _ = updateModel(m, cmd())
	if m.document.result == nil || m.document.lastError != "" || m.activeID != 0 {
		t.Fatalf("document run failed: %s %s", m.status, m.document.lastError)
	}
	return m
}

func documentSVGBytes(t *testing.T, s *documentSnapshot) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := documentlayout.WriteSVG(t.Context(), &b, s.result, s.services); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDocumentWorkbenchNestedAutoHistoryAndExport(t *testing.T) {
	m := selectDocumentFixture(t, initialModel())
	m = documentFonts(t, m)
	var snapshots []*documentSnapshot
	var proofs [][]byte
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		if m.document.breaker != breaker {
			m, _ = updateModel(m, keyPress(tea.KeyF2))
		}
		for _, width := range []string{"420", "100"} {
			m.document.widthInput.SetValue(width)
			m.invalidate()
			previous := m.document.result
			var cmd tea.Cmd
			m, cmd = updateModel(m, runKey())
			if cmd == nil || m.activeID == 0 || m.document.result != previous {
				t.Fatal("run should schedule work and preserve displayed result")
			}
			m, _ = updateModel(m, cmd())
			if m.document.lastError != "" {
				t.Fatal(m.document.lastError)
			}
			s := m.document.result
			if s == nil || s.breaker != breaker || len(s.result.Groups()) < 4 || len(s.fonts) != 2 {
				t.Fatal("incomplete retained document")
			}
			proof := documentSVGBytes(t, s)
			// Compare against the existing headless proof controls without
			// regenerating them or recomputing expected geometry in the TUI.
			size := "wide"
			if width == "100" {
				size = "narrow"
			}
			want, err := os.ReadFile("documentlayout/testdata/nested-auto-" + string(breaker) + "-" + size + ".svg")
			if err != nil || !bytes.Equal(proof, want) {
				t.Fatalf("TUI proof differs from headless %s %s control: %v", breaker, size, err)
			}
			snapshots = append(snapshots, s)
			proofs = append(proofs, proof)
			inspection := renderDocumentResult(s)
			for _, want := range []string{"CSS px", "NoHyphenation", "AlignBottomMarginEdge", "zero side skips", "Box ", "DOM node", "Group ", "owner Box", "Atomic ", "border origin", "baseline", "text bytes", "paint bytes", "SHA-256", "Retained font mapping", "Retained HTML source"} {
				if !strings.Contains(inspection, want) {
					t.Errorf("missing %q in document inspector", want)
				}
			}
			if width == "100" && !strings.Contains(inspection, "OVERFULL") {
				t.Fatal("narrow overflow is not inspectable")
			}
		}
	}
	if len(m.document.history) != 4 || len(m.history) != 0 {
		t.Fatal("document history mixed with paragraph history")
	}
	// Capture the current narrow proof, then browse older results/change fonts
	// and source before the export command actually executes.
	dir := t.TempDir()
	m.document.export = func(s *documentSnapshot) (string, error) { return exportDocumentSVG(s, dir) }
	m, export := updateModel(m, fontApplyKey())
	if export == nil {
		t.Fatal("no document export command")
	}
	m, _ = updateModel(m, keyPress(tea.KeyF3))
	if m.document.result != snapshots[2] || m.document.widthInput.Value() != "100" || m.document.breaker != pipeline.KnuthPlass {
		t.Fatal("history changed input controls or selected wrong snapshot")
	}
	m, _ = updateModel(m, keyPress(tea.KeyF6))
	if m.document.result != snapshots[2] || !strings.Contains(m.status, "F5 OpenType") {
		t.Fatal("synthetic selection obscured document policy or lost retained output")
	}
	m.document.source, m.browser.doc = nil, nil
	completed := export().(exportMsg)
	data, err := os.ReadFile(completed.path)
	if completed.err != nil || err != nil || !bytes.Equal(data, proofs[3]) || !strings.HasPrefix(filepath.Base(completed.path), "document-") {
		t.Fatal("export did not use captured document", completed, err)
	}
	m, _ = updateModel(m, completed)
	for i, s := range snapshots {
		if !bytes.Equal(documentSVGBytes(t, s), proofs[i]) {
			t.Fatal("later runs altered retained proof", i)
		}
	}
	second, err := exportDocumentSVG(snapshots[3], dir)
	if err != nil || second == completed.path {
		t.Fatal("export did not reserve a unique file", err)
	}
}

func TestDocumentFailuresPreserveInspectableExportableResult(t *testing.T) {
	m := runWorkbenchDocument(t, documentFonts(t, selectDocumentFixture(t, initialModel())))
	retained := m.document.result
	proof := documentSVGBytes(t, retained)
	original := m.document.source
	fonts := m.fontSettings.Clone()
	for _, tc := range []struct {
		name, want string
		change     func(*model)
	}{
		{"width", "Document width", func(m *model) { m.document.widthInput.SetValue("NaN") }},
		{"backend", "F5", func(m *model) { m.fontSettings = nil }},
		{"parser", "document fonts", func(m *model) { m.fontSettings.DefaultPath = "README.md" }},
		{"coverage", "missing glyph", func(m *model) { m.fontSettings.Fallbacks = nil }},
		{"manual hyphen", "NoHyphenation", func(m *model) {
			d, err := htmlinput.Parse("bad.html", strings.NewReader(`<div id="root">ab&shy;cd</div>`))
			if err != nil {
				t.Fatal(err)
			}
			m.browser.doc = d
			m.document.source, m.document.selected = d, findID(*m, "root")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.document.widthInput.SetValue("420")
			m.document.source, m.document.selected = original, retained.selected
			m.fontSettings = fonts.Clone()
			tc.change(&m)
			m.invalidate()
			m, cmd := updateModel(m, runKey())
			if cmd != nil {
				m, _ = updateModel(m, cmd())
			}
			if m.document.result != retained || len(m.document.history) != 1 || !strings.Contains(m.document.lastError, tc.want) {
				t.Fatal("failure replaced a completed result or was not diagnosed", m.document.lastError)
			}
			if !strings.Contains(m.viewport.View(), "Latest attempt failed") {
				t.Fatal("failure not visible")
			}
			m.setFocus(focusResults)
			m, _ = updateModel(m, keyPress(tea.KeyPgDown))
			if m.viewport.YOffset() == 0 {
				t.Fatal("retained inspection cannot scroll after failure")
			}
			dir := t.TempDir()
			m.document.export = func(s *documentSnapshot) (string, error) { return exportDocumentSVG(s, dir) }
			m, export := updateModel(m, fontApplyKey())
			if export == nil {
				t.Fatal("failure disabled export")
			}
			e := export().(exportMsg)
			data, err := os.ReadFile(e.path)
			if e.err != nil || err != nil || !bytes.Equal(data, proof) {
				t.Fatal("failure changed exported proof", e.err, err)
			}
		})
	}
}

func TestDocumentModePreservesParagraphControlsAndResults(t *testing.T) {
	m := loadedWorkbench(t)
	m.browser.selected = findID(m, "first")
	m, _ = updateModel(m, keyPress(tea.KeyEnter))
	m, cmd := updateModel(m, runKey())
	m, _ = updateModel(m, cmd())
	paragraph, source := m.result, m.source
	text, width, breaker := m.textarea.Value(), m.widthInput.Value(), m.breaker
	m = selectDocumentFixture(t, m)
	m.document.widthInput.SetValue("100")
	m, _ = updateModel(m, keyPress(tea.KeyF2))
	// Document selection is read-only: paste with results focused cannot edit
	// or detach the paragraph's text/source map.
	m.setFocus(focusResults)
	m, _ = updateModel(m, tea.PasteMsg{Content: "do not edit the paragraph"})
	m, _ = updateModel(m, keyPress(tea.KeyF7))
	if m.documentMode || m.result != paragraph || m.source != source || m.textarea.Value() != text || m.widthInput.Value() != width || m.breaker != breaker || len(m.history) != 1 {
		t.Fatal("document experiment changed paragraph controls/history")
	}
	if !strings.Contains(m.viewport.View(), "firstfit") {
		t.Fatal("paragraph inspection not restored")
	}
	// Switching inside the browser must refresh its eligibility and the hidden
	// result pane; Escape may return without selecting anything.
	m, _ = updateModel(m, keyPress(tea.KeyF4))
	m, _ = updateModel(m, keyPress(tea.KeyF7))
	if !m.browser.documentMode || !strings.Contains(m.browserView().Content, "document") {
		t.Fatal("browser mode is stale")
	}
	m, _ = updateModel(m, keyPress(tea.KeyEscape))
	if !m.documentMode || m.focus != focusWidth || !strings.Contains(m.viewport.View(), "Select a block") {
		t.Fatal("browser escape restored the wrong result pane")
	}
}

func TestDocumentSchedulerCancellationAndCrossModeQueue(t *testing.T) {
	m := selectDocumentFixture(t, initialModel())
	var captured documentJob
	var active context.Context
	var cancelledOnEntry error
	m.document.run = func(ctx context.Context, in documentJob) (*documentSnapshot, error) {
		active, captured = ctx, in
		cancelledOnEntry = ctx.Err()
		return nil, errors.New("stale backend failure")
	}
	m.fontSettings = &fontselection.Request{Settings: fontselection.Settings{DefaultPath: uiFont, Size: 12 * dimen.BP, Fallbacks: []string{"original"}}}
	m, first := updateModel(m, runKey())
	oldID := m.activeID
	m.fontSettings.Fallbacks[0] = "changed"
	m, _ = updateModel(m, tea.PasteMsg{Content: "0"}) // edits document width
	if m.pending != nil {
		t.Fatal("edit retained queued work")
	}
	m, cmd := updateModel(m, runKey())
	if cmd != nil || m.pending == nil || m.pending.document == nil {
		t.Fatal("document rerun was not serialized")
	}
	// Switching modes cancels the queued document, then queues a paragraph
	// behind the still-active command. No shared resolver can run concurrently.
	m, _ = updateModel(m, keyPress(tea.KeyF7))
	m, _ = updateModel(m, keyPress(tea.KeyF6)) // paragraph control uses synthetic
	if m.pending != nil {
		t.Fatal("mode switch retained pending document")
	}
	m, cmd = updateModel(m, runKey())
	if cmd != nil || m.pending == nil || m.pending.document != nil {
		t.Fatal("cross-mode request bypassed serialization")
	}
	m, second := updateModel(m, first())
	if second == nil || active.Err() == nil || cancelledOnEntry == nil || captured.fonts.Fallbacks[0] != "original" || captured.width != 420 || m.activeID == oldID || len(m.document.history) != 0 {
		t.Fatal("cancellation, captured input or pending dispatch failed")
	}
	id := m.activeID
	m, _ = updateModel(m, layoutMsg{id: oldID, documentRun: true, err: errors.New("duplicate")})
	if m.activeID != id {
		t.Fatal("late completion disturbed current work")
	}
	m, _ = updateModel(m, second())
	if m.result == nil || m.document.lastError != "" {
		t.Fatal("stale document completion leaked into paragraph mode")
	}
}

func TestDocumentLateSuccessAndQuit(t *testing.T) {
	m := runWorkbenchDocument(t, documentFonts(t, selectDocumentFixture(t, initialModel())))
	old := m.document.result
	for _, change := range []string{"width", "breaker", "selection", "fonts", "mode", "quit"} {
		t.Run(change, func(t *testing.T) {
			m := m
			var ctx context.Context
			var cancelledOnEntry error
			m.document.run = func(c context.Context, _ documentJob) (*documentSnapshot, error) {
				ctx = c
				cancelledOnEntry = c.Err()
				return old, nil // a backend may complete despite cancellation
			}
			m, cmd := updateModel(m, runKey())
			switch change {
			case "width":
				m, _ = updateModel(m, tea.PasteMsg{Content: "0"})
			case "breaker":
				m, _ = updateModel(m, keyPress(tea.KeyF2))
			case "selection":
				m, _ = updateModel(m, keyPress(tea.KeyF4))
				m.browser.selected = findID(m, "root")
				m, _ = updateModel(m, keyPress(tea.KeyEnter))
			case "fonts":
				m, _ = updateModel(m, keyPress(tea.KeyF6))
			case "mode":
				m, _ = updateModel(m, keyPress(tea.KeyF7))
			case "quit":
				var quit tea.Cmd
				m, quit = updateModel(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
				if _, ok := quit().(tea.QuitMsg); !ok {
					t.Fatal("quit not dispatched")
				}
			}
			completion := cmd()
			if ctx.Err() == nil || cancelledOnEntry == nil {
				t.Fatal("work not cancelled")
			}
			if change != "quit" {
				m, _ = updateModel(m, completion)
				if m.document.result != old || len(m.document.history) != 1 || m.activeID != 0 {
					t.Fatal("stale success was published")
				}
			}
		})
	}
}

func TestDocumentOfflineFontRequestsAndExportErrors(t *testing.T) {
	m := selectDocumentFixture(t, initialModel())
	d, err := htmlinput.Parse("latin.html", strings.NewReader(`<div id="root">office</div>`))
	if err != nil {
		t.Fatal(err)
	}
	m.browser.doc = d
	m.document.source, m.document.selected = d, findID(m, "root")
	m, _ = updateModel(m, keyPress(tea.KeyF5))
	m, _ = updateModel(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m.fontEditor.path.SetValue("serif ; 400 ; normal")
	m, preview := updateModel(m, fontApplyKey())
	if preview == nil {
		t.Fatal(m.fontEditor.status)
	}
	m, _ = updateModel(m, preview())
	if m.fontEditor.pending == nil {
		t.Fatal(m.fontEditor.status)
	}
	m, _ = updateModel(m, fontApplyKey())
	m = runWorkbenchDocument(t, m)
	if len(m.document.result.diagnostics) == 0 || len(m.document.result.fonts) != 1 || m.fontSettings.DefaultResource == nil {
		t.Fatal("offline resource settings/diagnostics were not reused")
	}
	dir := t.TempDir()
	if _, err := exportDocumentSVG(nil, dir); err == nil {
		t.Fatal("nil export succeeded")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("partial export survived")
	}
	retained := m.document.result
	m.document.export = func(s *documentSnapshot) (string, error) { return exportDocumentSVG(s, filepath.Join(dir, "missing")) }
	m, cmd := updateModel(m, fontApplyKey())
	m, _ = updateModel(m, cmd())
	if m.exporting || m.document.result != retained || !strings.Contains(m.status, "SVG export failed") {
		t.Fatal("export failure was not reported without replacing the snapshot")
	}
}

func TestDocumentWidthViewAndFocus(t *testing.T) {
	for _, s := range []string{"", "-1", "NaN", "Inf", "1e100", "43690.667", "abc"} {
		if _, err := parseDocumentWidth(s); err == nil {
			t.Errorf("accepted document width %q", s)
		}
	}
	for _, s := range []string{"0", "12.5", " 420 "} {
		if _, err := parseDocumentWidth(s); err != nil {
			t.Errorf("rejected document width %q: %v", s, err)
		}
	}
	m := selectDocumentFixture(t, initialModel())
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 40}} {
		for _, full := range []bool{false, true} {
			m.help.ShowAll = full
			m.resize(size[0], size[1])
			for _, f := range []focus{focusWidth, focusResults} {
				m.setFocus(f)
				v := m.View()
				if lipgloss.Width(v.Content) > size[0] || lipgloss.Height(v.Content) > size[1] || !strings.Contains(v.Content, "CSS px") {
					t.Fatalf("document view exceeds %v or lacks units: %q", size, v.Content)
				}
				if v.Cursor != nil && (v.Cursor.X >= size[0] || v.Cursor.Y >= size[1]) {
					t.Fatal("document cursor outside view", v.Cursor)
				}
			}
		}
	}
	m.setFocus(focusWidth)
	m, _ = updateModel(m, keyPress(tea.KeyTab))
	if m.focus != focusResults {
		t.Fatal("document tab did not reach results")
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focus != focusWidth {
		t.Fatal("document reverse tab did not reach width")
	}
	before := m.document.source
	m, _ = updateModel(m, keyPress(tea.KeyF4))
	m.browser.selected = 0
	m, _ = updateModel(m, keyPress(tea.KeyEnter))
	if !m.browser.open || m.document.source != before || !strings.Contains(m.browser.status, "block element") {
		t.Fatal("non-element selection replaced document input")
	}
	m, _ = updateModel(m, keyPress(tea.KeyEscape))
	if !reflect.DeepEqual(m.document.source, before) {
		t.Fatal("browser mutated selected document")
	}
}

func TestDocumentWorkbenchDiagnosticLeaf(t *testing.T) {
	m := documentFonts(t, selectDocumentFixture(t, initialModel()))
	input, err := htmlinput.LoadFile("documentlayout/testdata/rectangle.html")
	if err != nil {
		t.Fatal(err)
	}
	m.browser.doc = input
	m.document.source = input
	m.document.selected = findID(m, "root")
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		m.document.breaker = breaker
		m = runWorkbenchDocument(t, m)
		if !strings.Contains(renderDocumentResult(m.document.result), "Diagnostic rectangle") {
			t.Fatal("leaf not inspectable")
		}
		svg := documentSVGBytes(t, m.document.result)
		if bytes.Count(svg, []byte(`data-leaf="rectangle"`)) != 2 {
			t.Fatal("leaf proof missing")
		}
	}
}

func TestDocumentWorkbenchLocalImageLifetime(t *testing.T) {
	m := documentFonts(t, selectDocumentFixture(t, initialModel()))
	directory := t.TempDir()
	path := filepath.Join(directory, "local.png")
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	input, err := htmlinput.Parse(filepath.Join(directory, "source.html"), strings.NewReader(`<div id="root">office<img data-tytui-leaf="image" src="`+path+`" style="width:30px;height:10px"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	m.browser.doc = input
	m.document.source = input
	m.document.selected = findID(m, "root")
	m = runWorkbenchDocument(t, m)
	old := m.document.result
	if !strings.Contains(renderDocumentResult(old), "Retained image") {
		t.Fatal("missing image inspection")
	}
	before := documentSVGBytes(t, old)
	m.document.export = func(s *documentSnapshot) (string, error) { return exportDocumentSVG(s, directory) }
	m, export := updateModel(m, fontApplyKey())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	m, _ = updateModel(m, keyPress(tea.KeyF2))
	m, run := updateModel(m, runKey())
	m, _ = updateModel(m, run())
	if m.document.result != old || !strings.Contains(m.document.lastError, "image") {
		t.Fatal("missing payload did not preserve old result", m.document.lastError)
	}
	result := export().(exportMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	got, err := os.ReadFile(result.path)
	if err != nil || !bytes.Equal(got, before) {
		t.Fatal("export reopened image or used new state", err)
	}
}
