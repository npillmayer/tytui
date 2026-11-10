package pipeline

import (
	"fmt"
	"io"

	"github.com/npillmayer/khipu/debug"
)

// WriteSVG exports the exact frozen layout without encoding or breaking again.
// OpenType results draw positioned outlines and line geometry; synthetic results
// draw structural boxes. Text and font binaries are not embedded.
func WriteSVG(w io.Writer, result *Result) error {
	if result != nil && result.Input.Fonts != nil {
		return WriteGlyphSVG(w, result)
	}
	return WriteStructuralSVG(w, result)
}

// WriteStructuralSVG retains the advance-box diagnostic view for either backend.
func WriteStructuralSVG(w io.Writer, result *Result) error {
	if result == nil || result.Frozen == nil {
		return fmt.Errorf("SVG export requires a completed layout")
	}
	return debug.WriteFrozenSVG(w, result.Frozen, debug.SVGOptions{
		Title: fmt.Sprintf("Paragraph layout · %s · %.2f bp", result.Input.Breaker, result.Input.Width.Points()),
	})
}
