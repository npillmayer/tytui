// Package paragraphtext defines whitespace policy for one explicitly selected
// paragraph. Character separators never create additional paragraphs or lines.
package paragraphtext

import "strings"

// Span maps half-open processed UTF-8 bytes to original input bytes. Kind is
// empty for identity, collapsed for many-to-one whitespace, or omitted at edges.
type Span struct {
	Start, End             uint64
	SourceStart, SourceEnd uint64
	Kind                   string
}

// IsCollapsible includes ASCII whitespace and Unicode line/paragraph separators.
// Nonbreaking spaces and other ordinary Unicode spaces remain unchanged.
func IsCollapsible(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f', '\v', '\u0085', '\u2028', '\u2029':
		return true
	}
	return false
}

// Normalize collapses whitespace runs to one space and trims paragraph edges.
// The caller must supply valid UTF-8. Spans account for every original byte,
// including whitespace omitted from empty or whitespace-only paragraphs.
func Normalize(source string) (string, []Span) {
	var out strings.Builder
	var spans []Span
	appendSpan := func(start, end int, kind string, text string) {
		a := uint64(out.Len())
		out.WriteString(text)
		b := uint64(out.Len())
		if kind == "" && len(spans) > 0 {
			last := &spans[len(spans)-1]
			if last.Kind == "" && last.SourceEnd == uint64(start) {
				last.End = b
				last.SourceEnd = uint64(end)
				return
			}
		}
		spans = append(spans, Span{a, b, uint64(start), uint64(end), kind})
	}
	pending := -1
	literal := 0
	for pos, r := range source {
		if IsCollapsible(r) {
			if pending < 0 {
				if literal < pos {
					appendSpan(literal, pos, "", source[literal:pos])
				}
				pending = pos
			}
			continue
		}
		if pending >= 0 {
			kind, value := "collapsed", " "
			if out.Len() == 0 {
				kind, value = "omitted", ""
			} else if source[pending:pos] == " " {
				kind = ""
			}
			appendSpan(pending, pos, kind, value)
			pending = -1
			literal = pos
		}
	}
	if pending >= 0 {
		appendSpan(pending, len(source), "omitted", "")
	} else if literal < len(source) {
		appendSpan(literal, len(source), "", source[literal:])
	}
	return out.String(), spans
}
