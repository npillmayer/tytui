package fontbackend

import (
	"fmt"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Outline loads unhinted paths from the face's retained bytes, never from its
// path on disk. Coordinates are font units with Y pointing down, as in SVG.
func (f *Face) Outline(gid uint32) (sfnt.Segments, error) {
	if f == nil || gid > 65535 {
		return nil, fmt.Errorf("invalid outline face/glyph %d", gid)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.outline == nil {
		var err error
		f.outline, err = sfnt.Parse(f.data)
		if err != nil {
			return nil, fmt.Errorf("outline parser: %w", err)
		}
	}
	var buffer sfnt.Buffer
	segments, err := f.outline.LoadGlyph(&buffer, sfnt.GlyphIndex(gid), fixed.I(int(f.metrics.UnitsPerEm)), nil)
	if err != nil {
		return nil, fmt.Errorf("outline glyph %d: %w", gid, err)
	}
	return append(sfnt.Segments(nil), segments...), nil
}
