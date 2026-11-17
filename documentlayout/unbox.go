package documentlayout

import (
	"fmt"
	"math"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/typography/layout"
)

// SurroundingMargins records the enclosing CSS block's original resolved margins,
// before collapsing. Values are signed CSS pixels, including explicit zeros.
// They belong to OwnerID, not to each line, and are valid for the source layout's
// resolution context. Re-resolve through CSS when that context changes.
type SurroundingMargins struct {
	OwnerID  box.BoxID
	Resolved box.Edges
}

// UnboxedParagraph is a detached application envelope around immutable line
// items. Source retains document/source/paint associations. Margins is a detached
// copy, or nil if the source has no recorded margins; absence is not zero margins.
// Editing this envelope does not modify Source or the retained List.
type UnboxedParagraph struct {
	Source  *Result
	List    *layout.ElasticVList
	Margins *SurroundingMargins
}

// SurroundingMargins returns a value snapshot for an inline-group ID, as used by
// Group. False means an unknown group or missing metadata, including a nil result.
func (r *Result) SurroundingMargins(id box.BoxID) (SurroundingMargins, bool) {
	if r == nil {
		return SurroundingMargins{}, false
	}
	g, ok := r.groups[id]
	if !ok || !g.hasMargins {
		return SurroundingMargins{}, false
	}
	return g.margins, true
}

// Unbox recovers one retained inline group's full-height line items and a detached
// snapshot of its owner's margins. It performs no CSS resolution, measurement,
// shaping or rendering. The operation is synchronous, without cancellation.
// Invalid input or geometry returns nil and an error with its cause preserved.
func (r *Result) Unbox(id box.BoxID) (*UnboxedParagraph, error) {
	if r == nil {
		return nil, invalid("unbox requires a document result")
	}
	g, ok := r.groups[id]
	if !ok {
		return nil, invalid("unbox requires a retained inline group")
	}
	list, err := layout.Unbox(g.Paragraph.VList)
	if err != nil {
		return nil, fmt.Errorf("unbox group %d: %w", id, err)
	}
	out := &UnboxedParagraph{Source: r, List: list}
	if margins, ok := r.SurroundingMargins(id); ok {
		out.Margins = &margins
	}
	return out, nil
}

// Read the final callback view, not the initial used-values table: auto sizing
// may have resolved descendant margins in a private table after measurement.
func captureSurroundingMargins(request box.InlineRequest) (SurroundingMargins, error) {
	u, ok := request.Used.Lookup(request.OwnerID)
	if !ok || request.OwnerID == 0 || u.WidthPending || u.EdgesPending {
		return SurroundingMargins{}, invalid("missing or provisional paragraph-owner margins")
	}
	for _, value := range []float32{u.Margin.Top, u.Margin.Right, u.Margin.Bottom, u.Margin.Left} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return SurroundingMargins{}, invalid("non-finite paragraph-owner margin")
		}
	}
	return SurroundingMargins{OwnerID: request.OwnerID, Resolved: u.Margin}, nil
}
