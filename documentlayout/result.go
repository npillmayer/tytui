package documentlayout

import (
	"maps"
	"slices"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/typography/glyphing/paragraph"
)

// GroupResult keeps source, paint and the concrete paragraph together. Geometry
// is float64 big points relative to Input.OwnerID's content origin. Lines already
// include their vertical placement; a renderer must not stack them again.
type GroupResult struct {
	Input      InlineGroup
	Paragraph  paragraph.Result
	margins    SurroundingMargins
	hasMargins bool
}

// Result is one complete document snapshot. No working/prepared engine or partial
// callback table escapes. Accessors copy mutable envelopes/slices/maps; the
// Document, frozen paragraphs, vlists and retained font services are read-only.
type Result struct {
	document *Document
	geometry box.LayoutGeometryTable
	groups   map[box.BoxID]GroupResult
}

func (r *Result) Document() *Document               { return r.document }
func (r *Result) Geometry() box.LayoutGeometryTable { return maps.Clone(r.geometry) }
func (r *Result) Groups() []Group                   { return r.document.Groups() }

// Group returns a detached envelope keyed by normalized inline-group BoxID,
// including its explicit OwnerID and AtomicID-to-BoxID source correspondence.
func (r *Result) Group(id box.BoxID) (GroupResult, bool) {
	g, ok := r.groups[id]
	if !ok {
		return GroupResult{}, false
	}
	g.Input.Styles = slices.Clone(g.Input.Styles)
	g.Input.Paint = slices.Clone(g.Input.Paint)
	g.Input.LineLayout = *g.Input.LineLayout.Clone()
	g.Input.Isolates = slices.Clone(g.Input.Isolates)
	g.Input.Anchors = slices.Clone(g.Input.Anchors)
	g.Input.TextSources = slices.Clone(g.Input.TextSources)
	g.Input.AtomicSources = slices.Clone(g.Input.AtomicSources)
	g.Paragraph.BreakPositions = slices.Clone(g.Paragraph.BreakPositions)
	g.Paragraph.BidiRuns = slices.Clone(g.Paragraph.BidiRuns)
	return g, true
}
