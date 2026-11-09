// Package fontbackend adapts local OpenType fonts to khipu's public contracts.
package fontbackend

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/opentype"
	"github.com/npillmayer/opentype/ot"
	"github.com/npillmayer/opentype/otquery"
)

// Face retains private immutable bytes and serializes access to lazy backend data.
// Use Info to obtain detached identity and parser diagnostics.
type Face struct {
	mu        sync.Mutex
	font      *ot.Font
	data      []byte
	info      Info
	metrics   contracts.FontMetrics
	unitScale float64
}
type Info struct {
	Path, Family, Subfamily, SHA256 string
	Warnings                        []string
}

func (f *Face) Info() Info { i := f.info; i.Warnings = append([]string(nil), i.Warnings...); return i }

type Loader struct{}

var _ contracts.FontLoader = Loader{}

const maxFontBytes = 64 << 20

func (Loader) LoadFont(src io.Reader) (contracts.FontFace, error) {
	if src == nil {
		return nil, fmt.Errorf("font reader is nil")
	}
	data, err := io.ReadAll(io.LimitReader(src, maxFontBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read font: %w", err)
	}
	if len(data) > maxFontBytes {
		return nil, fmt.Errorf("font exceeds 64 MiB")
	}
	font, err := ot.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse OpenType: %w", err)
	}
	if font == nil {
		return nil, fmt.Errorf("parser returned no font")
	}
	if errs := font.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("OpenType parser diagnostics: %v (critical: %v)", errs, font.CriticalErrors())
	}
	family, subfamily := opentype.FamilyName(font)
	f := &Face{font: font, data: data, info: Info{Family: family, Subfamily: subfamily, SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}}
	for _, w := range font.Warnings() {
		f.info.Warnings = append(f.info.Warnings, fmt.Sprint(w))
	}
	m := otquery.FontMetrics(font)
	if m.UnitsPerEm <= 0 {
		return nil, fmt.Errorf("invalid units per em")
	}
	f.metrics = contracts.FontMetrics{UnitsPerEm: uint32(m.UnitsPerEm), Ascent: int32(m.Ascent), Descent: int32(m.Descent), LineGap: int32(m.LineGap)}
	f.unitScale, err = otquery.FontScale(font, 1)
	if err != nil {
		return nil, err
	}
	return f, nil
}
func LoadFile(path string) (*Face, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("font %q: %w", path, err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("font %q is not a regular file", path)
	}
	face, err := (Loader{}).LoadFont(file)
	if err != nil {
		return nil, fmt.Errorf("font %q: %w", path, err)
	}
	f := face.(*Face)
	f.info.Path = path
	return f, nil
}
func (f *Face) Metrics() (contracts.FontMetrics, error) {
	if f == nil || f.font == nil {
		return contracts.FontMetrics{}, fmt.Errorf("nil OpenType face")
	}
	return f.metrics, nil
}

type Scale float64

func (s Scale) ToDU(units int32) float64 { return float64(s) * float64(units) }
func (f *Face) Scale(size dimen.DU) (contracts.FontScale, error) {
	if f == nil || f.font == nil || size <= 0 || size >= dimen.Fil {
		return nil, fmt.Errorf("invalid face or font size")
	}
	return Scale(f.unitScale * float64(size)), nil
}
func (f *Face) GlyphIndex(r rune) (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, err := otquery.GlyphIndex(f.font, r)
	return uint32(g), err
}
