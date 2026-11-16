package documentlayout

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/paragraph"
	typestyle "github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"

	csstext "github.com/npillmayer/css-box-layout/text"
	"tytui/htmlinput"
)

// TextSpan maps processed UTF-8 bytes to decoded DOM text. BoxID identifies the
// normalized text fragment; Locator stays the original source identity. Atomic
// marker bytes are covered only by AtomicSource, never invented text-node bytes.
type TextSpan struct {
	Start, End         uint64
	BoxID              box.BoxID
	Locator            Locator
	NodeStart, NodeEnd uint64
	Kind               string // identity (empty), collapsed, omitted; same policy as htmlinput
}

type AtomicSource struct {
	ID         contracts.AtomicID
	BoxID      box.BoxID
	Locator    Locator
	Start, End uint64
}

// InlineGroup is detached extracted input; caller-owned slices may be modified
// without changing Document or another extraction. Text, styles, paint, layout,
// anchors and isolates all address this same normalized UTF-8 stream. Paragraph
// supplies the strut's typography separately, including for object-only groups.
type InlineGroup struct {
	Group
	Text          string
	Styles        []typestyle.Span
	Paint         []typography.PaintSpan
	LineLayout    paragraph.LineLayout
	Isolates      []htmlinput.Isolate
	Anchors       []contracts.AtomicAnchor
	TextSources   []TextSpan
	AtomicSources []AtomicSource
}

// BidiMarkup snapshots the isolate ranges; the callback may outlive this value.
func (g *InlineGroup) BidiMarkup() (bidi.OutOfLineBidiMarkup, error) {
	if g == nil {
		return nil, invalid("nil inline group")
	}
	for _, span := range g.Isolates {
		if !boundary(g.Text, span.Start) || !boundary(g.Text, span.End) {
			return nil, invalid("bidi boundary splits UTF-8")
		}
	}
	return (&htmlinput.Extraction{Text: g.Text, Isolates: g.Isolates}).BidiMarkup()
}

type rawPiece struct {
	start, end uint64
	node       *box.LayoutNode
	source     NodeSource
	nodeStart  uint64
	atomic     bool
}

type inlineRange struct {
	start, end uint64
	source     NodeSource
	isolate    bool
}

// Extract visits only the specified normalized inline group. At an inline-block
// it emits one U+FFFC and skips the entire subtree; nested groups are independent
// entries in Document.Groups. Atomic IDs are assigned in emitted occurrence order
// and scoped to this group, independently of BoxID or dimension-table ordering.
func (d *Document) Extract(ctx context.Context, id box.BoxID) (*InlineGroup, error) {
	if ctx == nil {
		return nil, invalid("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d == nil {
		return nil, invalid("nil document")
	}
	entry, ok := d.groups[id]
	if !ok {
		return nil, invalid("unknown inline group")
	}
	g := &InlineGroup{Group: entry.Group, LineLayout: paragraph.LineLayout{Default: paragraph.LineHeight{Kind: paragraph.NormalHeight}}}
	var raw strings.Builder
	var pieces []rawPiece
	var ranges []inlineRange
	var walk func(*box.LayoutNode) error
	walk = func(n *box.LayoutNode) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n == nil {
			return invalid("nil normalized node")
		}
		s, ok := d.nodes[n.NodeID]
		if !ok {
			return invalid("missing source node")
		}
		start := uint64(raw.Len())
		switch n.Box {
		case box.BoxText:
			source, ok := d.sources[n.Text.Source]
			ref := n.Text.Range
			if !ok || source.Locator != s.Locator || ref.Start > ref.End || !boundary(source.Text, ref.Start) || !boundary(source.Text, ref.End) {
				return invalid("invalid normalized text reference")
			}
			raw.WriteString(source.Text[ref.Start:ref.End])
			pieces = append(pieces, rawPiece{start: start, end: uint64(raw.Len()), node: n, source: s, nodeStart: ref.Start})
		case box.BoxInlineBlock:
			raw.WriteRune('\uFFFC')
			pieces = append(pieces, rawPiece{start: start, end: uint64(raw.Len()), node: n, source: s, atomic: true})
			if s.Style.UnicodeBidi == "isolate" {
				ranges = append(ranges, inlineRange{start, uint64(raw.Len()), s, true})
			}
		case box.BoxInline:
			for _, child := range n.Children {
				if err := walk(child); err != nil {
					return err
				}
			}
			ranges = append(ranges, inlineRange{start, uint64(raw.Len()), s, s.Style.UnicodeBidi == "isolate"})
		default:
			return invalid("non-inline child in normalized group")
		}
		return nil
	}
	for _, child := range entry.node.Children {
		if err := walk(child); err != nil {
			return nil, err
		}
	}
	// Reuse the established whitespace transform. Markers are non-whitespace,
	// so pending spaces flush at atomic boundaries without entering interiors.
	text, transforms := csstext.NormalizeNormal(raw.String())
	g.Text = text
	appendStyle := func(start, end uint64, s typestyle.Style) {
		if start == end {
			return
		}
		if len(g.Styles) > 0 && g.Styles[len(g.Styles)-1].Style == s {
			g.Styles[len(g.Styles)-1].End = end
		} else {
			g.Styles = append(g.Styles, typestyle.Span{Start: start, End: end, Style: s})
		}
	}
	pi := 0
	for _, tr := range transforms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for pi < len(pieces) && pieces[pi].end <= tr.SourceStart {
			pi++
		}
		for j := pi; j < len(pieces) && pieces[j].start < tr.SourceEnd; j++ {
			p := pieces[j]
			from, to := max(tr.SourceStart, p.start), min(tr.SourceEnd, p.end)
			start, end := tr.Start, tr.End
			if tr.Kind == "" {
				start = tr.Start + from - tr.SourceStart
				end = start + to - from
			}
			if p.atomic {
				if tr.Kind != "" || end-start != 3 {
					return nil, invalid("atomic marker transformed")
				}
				id := contracts.AtomicID(len(g.Anchors) + 1)
				g.Anchors = append(g.Anchors, contracts.AtomicAnchor{ID: id, ByteOffset: start})
				g.AtomicSources = append(g.AtomicSources, AtomicSource{ID: id, BoxID: p.node.BoxID, Locator: p.source.Locator, Start: start, End: end})
			} else {
				g.appendSource(TextSpan{Start: start, End: end, BoxID: p.node.BoxID, Locator: p.source.Locator, NodeStart: p.nodeStart + from - p.start, NodeEnd: p.nodeStart + to - p.start, Kind: tr.Kind})
			}
			// Collapsed whitespace belongs to its first source unit's style.
			if tr.Kind == "" || (tr.Kind == "collapsed" && from == tr.SourceStart) {
				appendStyle(start, end, p.source.Style)
			}
		}
	}
	for _, r := range ranges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start, end, found := projectRange(transforms, r.start, r.end)
		if !found {
			continue
		}
		if r.isolate {
			g.Isolates = append(g.Isolates, htmlinput.Isolate{Start: start, End: end, Direction: r.source.Style.Direction})
		}
	}
	for _, s := range g.Styles {
		g.Paint = append(g.Paint, typography.PaintSpan{Start: s.Start, End: s.End, Paint: s.Style.Paint()})
		g.LineLayout.Runs = append(g.LineLayout.Runs, paragraph.LineHeightSpan{Start: s.Start, End: s.End, Height: g.LineLayout.Default})
	}
	if _, err := g.BidiMarkup(); err != nil {
		return nil, err
	}
	if err := typestyle.ValidateSpans(g.Text, g.Styles); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return g, nil
}

func boundary(s string, p uint64) bool {
	return p <= uint64(len(s)) && (p == uint64(len(s)) || utf8.RuneStart(s[p]))
}

func (g *InlineGroup) appendSource(s TextSpan) {
	if len(g.TextSources) > 0 {
		last := &g.TextSources[len(g.TextSources)-1]
		if last.BoxID == s.BoxID && last.Locator == s.Locator && last.NodeEnd == s.NodeStart && last.Kind == s.Kind &&
			((s.Kind == "" && last.End == s.Start) || (s.Kind != "" && last.Start == s.Start && last.End == s.End)) {
			last.End, last.NodeEnd = s.End, s.NodeEnd
			return
		}
	}
	g.TextSources = append(g.TextSources, s)
}

// Project original inline scopes using the same first-unit ownership as emitted
// whitespace. A collapsed character is not duplicated into later source scopes.
func projectRange(transforms []csstext.Span, from, to uint64) (start, end uint64, found bool) {
	i := sort.Search(len(transforms), func(i int) bool { return transforms[i].SourceEnd > from })
	for _, tr := range transforms[i:] {
		if tr.SourceStart >= to {
			break
		}
		if tr.SourceEnd <= from || tr.Kind == "omitted" {
			continue
		}
		a, b := tr.Start, tr.End
		if tr.Kind == "" {
			a = tr.Start + max(from, tr.SourceStart) - tr.SourceStart
			b = tr.Start + min(to, tr.SourceEnd) - tr.SourceStart
		} else if tr.SourceStart < from {
			continue
		}
		if a < b {
			if !found {
				start = a
				found = true
			}
			end = b
		}
	}
	return
}
