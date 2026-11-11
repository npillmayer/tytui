package fontbackend

import (
	"context"
	"fmt"
	"github.com/npillmayer/cords/styled"
	"github.com/rivo/uniseg"
	"tytui/htmlinput"
	"unicode"
)

// runStyle is a value identity: no mutable font pointer enters styled.Text.
type runStyle struct {
	CSS                htmlinput.Style
	Plain              bool
	ResourceID, Script string
}

func (s runStyle) Equals(other styled.Style) bool { o, ok := other.(runStyle); return ok && s == o }
func (s runStyle) String() string                 { return fmt.Sprintf("%s %s %s", s.Script, s.ResourceID, s.CSS) }

// Itemize keeps extended grapheme clusters intact. Common/Inherited clusters
// adopt the preceding strong script, or the following one at a span's start.
// Only Latin and Hebrew are currently supported; other strong scripts fail.
// Fallback chooses the first configured face covering an entire grapheme.
// Soft hyphens inherit the preceding face within the CSS span (the following
// face at its start). Marker-only spans retain their mapped primary face.
func (r *Resolver) Itemize(ctx context.Context, text string, source *htmlinput.Extraction) (styled.Text, []Binding, error) {
	out := styled.TextFromString(text)
	spans := []htmlinput.StyleSpan{{Start: 0, End: uint64(len(text))}}
	if source != nil {
		spans = source.Styles
	}
	boundaries := map[int]bool{0: true}
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		_, b := g.Positions()
		boundaries[b] = true
	}
	type item struct {
		start, end uint64
		style      runStyle
	}
	var items []item
	for _, span := range spans {
		if !boundaries[int(span.Start)] || !boundaries[int(span.End)] {
			return out, nil, fmt.Errorf("style boundary splits a grapheme at [%d,%d)", span.Start, span.End)
		}
		var css styled.Style
		if source != nil {
			css = span.Style
		}
		resolved, err := r.ResolveStyle(css, r.Defaults())
		if err != nil {
			return out, nil, err
		}
		primary := resolved.Font.(*Face).Info().ResourceID
		var local []item
		clusters := uniseg.NewGraphemes(text[span.Start:span.End])
		for clusters.Next() {
			if err := ctx.Err(); err != nil {
				return out, nil, err
			}
			a, b := clusters.Positions()
			chunk := clusters.Str()
			script := ""
			for _, c := range chunk {
				switch {
				case unicode.Is(unicode.Latin, c):
					script = "Latn"
				case unicode.Is(unicode.Hebrew, c):
					script = "Hebr"
				case unicode.IsLetter(c) && !unicode.Is(unicode.Common, c) && !unicode.Is(unicode.Inherited, c):
					return out, nil, fmt.Errorf("unsupported script for %U at UTF-8 bytes [%d,%d)", c, span.Start+uint64(a), span.Start+uint64(b))
				}
			}
			// A soft hyphen has no nominal glyph in the unbroken word. Do not
			// let vacuous coverage select the primary face and split a word
			// already using fallback. Resolve its face after adjacent text.
			if chunk == "\u00ad" {
				local = append(local, item{span.Start + uint64(a), span.Start + uint64(b), runStyle{span.Style, source == nil, "", script}})
				continue
			}
			chosen := ""
			var missing rune
			candidates := append([]string{primary}, r.fallbackIDs...)
			for _, path := range candidates {
				covered := true
				for _, c := range chunk {
					if coverageExempt(c) {
						continue
					}
					gid, err := r.faces[path].GlyphIndex(c)
					if err != nil {
						return out, nil, fmt.Errorf("font %q cmap: %w", path, err)
					}
					if gid == 0 {
						missing = c
						covered = false
						break
					}
				}
				if covered {
					chosen = path
					break
				}
			}
			if chosen == "" {
				return out, nil, fmt.Errorf("missing glyph %U at UTF-8 bytes [%d,%d): no configured face covers grapheme %q (tried %q)", missing, span.Start+uint64(a), span.Start+uint64(b), chunk, candidates)
			}
			local = append(local, item{span.Start + uint64(a), span.Start + uint64(b), runStyle{span.Style, source == nil, chosen, script}})
		}
		previous := ""
		for i := range local {
			if local[i].style.Script != "" {
				previous = local[i].style.Script
			} else if previous != "" {
				local[i].style.Script = previous
			}
		}
		next := "Latn"
		for i := len(local) - 1; i >= 0; i-- {
			if local[i].style.Script != "" {
				next = local[i].style.Script
			} else {
				local[i].style.Script = next
			}
		}
		// Use logical neighbors, just as for script inheritance. Never cross
		// an original CSS span or replace a real text item's selected face.
		previousFace := ""
		for i := range local {
			if local[i].style.ResourceID != "" {
				previousFace = local[i].style.ResourceID
			} else if previousFace != "" {
				local[i].style.ResourceID = previousFace
			}
		}
		nextFace := primary
		for i := len(local) - 1; i >= 0; i-- {
			if local[i].style.ResourceID != "" {
				nextFace = local[i].style.ResourceID
			} else {
				local[i].style.ResourceID = nextFace
			}
		}
		for _, it := range local {
			n := len(items)
			if n > 0 && items[n-1].end == it.start && items[n-1].style == it.style {
				items[n-1].end = it.end
			} else {
				items = append(items, it)
			}
		}
	}
	var bindings []Binding
	for _, it := range items {
		var err error
		out, err = out.Style(it.style, it.start, it.end)
		if err != nil {
			return out, nil, err
		}
		resolved, err := r.ResolveStyle(it.style, r.Defaults())
		if err != nil {
			return out, nil, err
		}
		info := r.faces[it.style.ResourceID].Info()
		bindings = append(bindings, Binding{Start: it.start, End: it.end, Path: info.Path, SHA256: info.SHA256, ResourceID: info.ResourceID, Origin: info.Origin, EmSize: resolved.EmSize, Script: it.style.Script})
	}
	return out, bindings, nil
}
func coverageExempt(c rune) bool {
	return unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || (c >= 0xFE00 && c <= 0xFE0F) || (c >= 0xE0100 && c <= 0xE01EF)
}
