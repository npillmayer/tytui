package pipeline

import (
	"fmt"
	"io"

	"github.com/npillmayer/khipu/debug"
)

// WriteSVG exports the exact frozen layout without encoding or breaking again.
// It draws structural widths and fit diagnostics, not glyph outlines. Source
// text and font data are not embedded. Writer errors are returned to the caller.
func WriteSVG(w io.Writer, result *Result) error {
	if result == nil || result.Frozen == nil {
		return fmt.Errorf("SVG export requires a completed layout")
	}
	return debug.WriteFrozenSVG(w, result.Frozen, debug.SVGOptions{
		Title: fmt.Sprintf("Paragraph layout · %s · %.2f bp", result.Input.Breaker, result.Input.Width.Points()),
	})
}
