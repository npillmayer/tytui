package htmlinput

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/npillmayer/uax/bidi"
	"golang.org/x/net/html"
	"tytui/paragraphtext"
)

type textUnit struct {
	r          rune
	id         int
	start, end uint64
	style      Style
}

// Extract resolves inherited styles and collapses whitespace across text nodes.
// Paragraph boundaries are explicit; newline-preserving CSS modes are rejected.
// Mapping records retain removed and many-to-one whitespace provenance.
func (d *Document) Extract(id int) (*Extraction, error) {
	if id < 0 || id >= len(d.Nodes) {
		return nil, fmt.Errorf("unknown DOM node %d", id)
	}
	selected := d.Nodes[id]
	if selected.Type != html.TextNode && !(selected.Type == html.ElementNode && (contains(rootTags, selected.Name) || contains(inlineTags, selected.Name))) {
		return nil, fmt.Errorf("select a paragraph-like element or text node, not %s", selected.Name)
	}
	for parent := id; parent >= 0; parent = d.Nodes[parent].Parent {
		n := d.Nodes[parent]
		if n.Name == "head" || n.Name == "script" || n.Name == "style" {
			return nil, fmt.Errorf("%s is not paragraph content", n.Name)
		}
		if err := d.checkVisibility(parent); err != nil {
			return nil, err
		}
	}
	styles, err := d.resolvedStyles(id)
	if err != nil {
		return nil, err
	}
	e := &Extraction{DocumentName: d.Name, DocumentID: d.ID, SelectedID: id, SelectedPath: selected.Path, Direction: styles[id].Direction}
	var units []textUnit
	var isolates []int
	var collect func(int) error
	collect = func(current int) error {
		n := d.Nodes[current]
		s := styles[current]
		if n.Type == html.ElementNode {
			if s.WhiteSpace != "normal" {
				return fmt.Errorf("%s: white-space %q is unsupported; use normal (newlines collapse as whitespace)", n.Path, s.WhiteSpace)
			}
			if n.Name == "br" || n.Name == "hr" {
				return fmt.Errorf("%s: <%s> introduces an unsupported mandatory break", n.Path, n.Name)
			}
			if current != id && !contains(inlineTags, n.Name) {
				return fmt.Errorf("%s: nested block or unsupported <%s>; select a smaller paragraph", n.Path, n.Name)
			}
			if err := d.checkVisibility(current); err != nil {
				return err
			}
			if current != id {
				if value, ok := n.styled.Styles().Property("display"); ok && value != "inline" && value != "contents" {
					return fmt.Errorf("%s: display %q is not inline content", n.Path, value)
				}
			}
			if s.UnicodeBidi != "normal" && s.UnicodeBidi != "isolate" {
				return fmt.Errorf("%s: unicode-bidi %q is unsupported", n.Path, s.UnicodeBidi)
			}
			if current != id && s.UnicodeBidi == "isolate" {
				isolates = append(isolates, current)
			}
		}
		if n.Type == html.TextNode {
			if s.WhiteSpace != "normal" {
				return fmt.Errorf("%s: white-space %q is unsupported; use normal (newlines collapse as whitespace)", n.Path, s.WhiteSpace)
			}
			for start, r := range n.Text {
				units = append(units, textUnit{r, current, uint64(start), uint64(start + utf8.RuneLen(r)), s})
			}
		}
		for _, child := range n.Children {
			if err := collect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(id); err != nil {
		return nil, err
	}
	var out strings.Builder
	ranges := make(map[int][2]uint64)
	appendMap := func(u textUnit, start, end uint64, kind string) {
		span := Span{Start: start, End: end, NodeID: u.id, NodePath: d.Nodes[u.id].Path, NodeStart: u.start, NodeEnd: u.end, Kind: kind}
		if len(e.Spans) > 0 {
			last := &e.Spans[len(e.Spans)-1]
			if last.NodeID == span.NodeID && last.NodeEnd == span.NodeStart && last.Kind == kind {
				if (kind == "" && last.End == start) || (kind != "" && last.Start == start && last.End == end) {
					last.End = end
					last.NodeEnd = u.end
					return
				}
			}
		}
		e.Spans = append(e.Spans, span)
	}
	emit := func(u textUnit, r rune) (uint64, uint64) {
		start := uint64(out.Len())
		out.WriteRune(r)
		end := uint64(out.Len())
		if len(e.Styles) > 0 && e.Styles[len(e.Styles)-1].Style == u.style {
			e.Styles[len(e.Styles)-1].End = end
		} else {
			e.Styles = append(e.Styles, StyleSpan{start, end, u.style})
		}
		for node := u.id; node >= 0; node = d.Nodes[node].Parent {
			if old, ok := ranges[node]; ok {
				ranges[node] = [2]uint64{old[0], end}
			} else {
				ranges[node] = [2]uint64{start, end}
			}
			if node == id {
				break
			}
		}
		return start, end
	}
	var pending []textUnit
	flush := func(trim bool) {
		if len(pending) == 0 {
			return
		}
		start, end := uint64(out.Len()), uint64(out.Len())
		kind := "omitted"
		if !trim {
			start, end = emit(pending[0], ' ')
			kind = "collapsed"
		}
		if len(pending) == 1 && pending[0].r == ' ' && !trim {
			kind = ""
		}
		for _, u := range pending {
			appendMap(u, start, end, kind)
		}
		pending = nil
	}
	for _, u := range units {
		collapse := paragraphtext.IsCollapsible(u.r)
		if collapse {
			pending = append(pending, u)
			continue
		}
		flush(out.Len() == 0)
		start, end := emit(u, u.r)
		appendMap(u, start, end, "")
	}
	flush(true)
	e.Text = out.String()
	for _, node := range isolates {
		if span, ok := ranges[node]; ok && span[0] < span[1] {
			e.Isolates = append(e.Isolates, Isolate{span[0], span[1], styles[node].Direction})
		}
	}
	if _, err := e.BidiMarkup(); err != nil {
		return nil, err
	}
	if err := e.Validate(e.Text); err != nil {
		return nil, err
	}
	return e, nil
}

func byteBoundary(text string, p uint64) bool {
	return p <= uint64(len(text)) && (p == uint64(len(text)) || utf8.RuneStart(text[p]))
}

// Validate checks processed-text coverage, transformed provenance, style spans,
// and bidi coordinates. Collapsed ranges may repeat for multiple source nodes;
// omitted whitespace consumes source bytes but zero output bytes.
func (e *Extraction) Validate(text string) error {
	if e == nil {
		return nil
	}
	if e.Text != text || !utf8.ValidString(text) {
		return fmt.Errorf("DOM source text no longer matches paragraph")
	}
	normalized, _ := paragraphtext.Normalize(text)
	if normalized != text {
		return fmt.Errorf("DOM paragraph whitespace is not normalized")
	}
	var end uint64
	var previous *Span
	for i := range e.Spans {
		s := &e.Spans[i]
		if s.End < s.Start || !byteBoundary(text, s.Start) || !byteBoundary(text, s.End) || s.NodeEnd <= s.NodeStart || s.NodeID < 0 {
			return fmt.Errorf("invalid DOM source span")
		}
		switch s.Kind {
		case "":
			if s.Start != end || s.End == s.Start || s.End-s.Start != s.NodeEnd-s.NodeStart {
				return fmt.Errorf("invalid identity span")
			}
		case "collapsed":
			repeated := previous != nil && previous.Kind == "collapsed" && previous.Start == s.Start && previous.End == s.End
			if (!repeated && s.Start != end) || s.End != s.Start+1 || text[s.Start:s.End] != " " {
				return fmt.Errorf("invalid collapsed span")
			}
		case "omitted":
			if s.Start != end || s.End != s.Start {
				return fmt.Errorf("invalid omitted span")
			}
		default:
			return fmt.Errorf("unknown source transformation %q", s.Kind)
		}
		end = s.End
		previous = s
	}
	if end != uint64(len(text)) {
		return fmt.Errorf("DOM source map does not cover paragraph")
	}
	end = 0
	for _, s := range e.Styles {
		if s.Style.WhiteSpace != "normal" {
			return fmt.Errorf("unsupported white-space %q", s.Style.WhiteSpace)
		}
		if s.Start != end || s.End <= s.Start || !byteBoundary(text, s.End) || s.Style.EmSize <= 0 {
			return fmt.Errorf("invalid style span")
		}
		end = s.End
	}
	if end != uint64(len(text)) {
		return fmt.Errorf("style spans do not cover paragraph")
	}
	if e.Direction != bidi.LeftToRight && e.Direction != bidi.RightToLeft {
		return fmt.Errorf("invalid paragraph direction")
	}
	for _, s := range e.Isolates {
		if !byteBoundary(text, s.Start) || !byteBoundary(text, s.End) {
			return fmt.Errorf("bidi boundary splits UTF-8")
		}
	}
	_, err := e.BidiMarkup()
	return err
}
