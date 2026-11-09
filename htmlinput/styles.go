package htmlinput

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"golang.org/x/net/html"
	"golang.org/x/text/language"
	ubidi "golang.org/x/text/unicode/bidi"
)

// Style is an immutable-by-value resolved style. All fields are values; store it
// in styled.Text as a value, not a pointer. Family/weight/slant/color are metadata
// until a real-font adapter is selected. EmSize is in scaled big points.
type Style struct {
	Family                                          string
	EmSize                                          dimen.DU
	Weight                                          int
	Slant, Color, Language, WhiteSpace, UnicodeBidi string
	Direction                                       bidi.Direction
}

func (s Style) Equals(other styled.Style) bool { o, ok := other.(Style); return ok && s == o }
func (s Style) String() string {
	return fmt.Sprintf("%s %.3g bp weight=%d %s color=%s lang=%s dir=%s white-space=%s", s.Family, s.EmSize.Points(), s.Weight, s.Slant, s.Color, s.Language, s.Direction, s.WhiteSpace)
}

// StyleSpan uses half-open UTF-8 byte coordinates in processed paragraph text.
type StyleSpan struct {
	Start, End uint64
	Style      Style
}

// Isolate supplies zero-byte bidi markup; it does not change source coordinates.
type Isolate struct {
	Start, End uint64
	Direction  bidi.Direction
}

func initialStyle() Style {
	return Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "black", Language: "und", WhiteSpace: "normal", UnicodeBidi: "normal"}
}

func (d *Document) resolvedStyles(selected int) (map[int]Style, error) {
	result := make(map[int]Style, len(d.Nodes))
	rootSize := initialStyle().EmSize
	needed := make(map[int]bool)
	for id := selected; id >= 0; id = d.Nodes[id].Parent {
		needed[id] = true
	}
	var descendants func(int)
	descendants = func(id int) {
		needed[id] = true
		for _, child := range d.Nodes[id].Children {
			descendants(child)
		}
	}
	descendants(selected)
	for _, n := range d.Nodes {
		if !needed[n.ID] {
			continue
		}
		parent := initialStyle()
		if n.Parent >= 0 {
			parent = result[n.Parent]
		}
		s := parent
		s.UnicodeBidi = "normal" // unicode-bidi is not inherited.
		if n.Type == html.TextNode {
			s = parent
			result[n.ID] = s
			continue
		}
		switch n.Name {
		case "b", "strong":
			s.Weight = 700
		case "i", "em":
			s.Slant = "italic"
		case "pre":
			s.WhiteSpace = "pre"
		case "bdi":
			s.Direction = d.firstStrong(n.ID, parent.Direction)
			s.UnicodeBidi = "isolate"
		}
		for _, a := range n.Attributes {
			switch a.Key {
			case "lang":
				tag, err := language.Parse(a.Val)
				if err != nil {
					return nil, fmt.Errorf("%s: invalid lang %q", n.Path, a.Val)
				}
				s.Language = tag.String()
			case "dir":
				switch strings.ToLower(a.Val) {
				case "ltr":
					s.Direction = bidi.LeftToRight
				case "rtl":
					s.Direction = bidi.RightToLeft
				case "auto":
					s.Direction = d.firstStrong(n.ID, parent.Direction)
				default:
					return nil, fmt.Errorf("%s: unsupported dir %q", n.Path, a.Val)
				}
				s.UnicodeBidi = "isolate"
			}
		}
		for _, key := range []string{"font-family", "font-size", "font-weight", "font-style", "color", "white-space", "direction", "unicode-bidi"} {
			value, ok := n.styled.Styles().Property(key)
			if !ok {
				continue
			}
			v := strings.TrimSpace(string(value))
			defaults := initialStyle()
			// All properties here except unicode-bidi are inherited by default.
			if v == "inherit" || (v == "unset" && key != "unicode-bidi") {
				assignProperty(&s, parent, key)
				continue
			}
			if v == "initial" || v == "unset" {
				assignProperty(&s, defaults, key)
				continue
			}
			var err error
			switch key {
			case "font-family":
				if v == "" {
					err = fmt.Errorf("empty font-family")
				} else {
					s.Family = v
				}
			case "font-size":
				s.EmSize, err = resolveSize(v, parent.EmSize, rootSize)
			case "font-weight":
				switch v {
				case "normal":
					s.Weight = 400
				case "bold":
					s.Weight = 700
				case "bolder":
					if parent.Weight < 350 {
						s.Weight = 400
					} else if parent.Weight < 550 {
						s.Weight = 700
					} else {
						s.Weight = 900
					}
				case "lighter":
					if parent.Weight < 550 {
						s.Weight = 100
					} else if parent.Weight < 750 {
						s.Weight = 400
					} else {
						s.Weight = 700
					}
				default:
					s.Weight, err = strconv.Atoi(v)
					if s.Weight < 1 || s.Weight > 1000 {
						err = fmt.Errorf("weight must be in 1..1000")
					}
				}
			case "font-style":
				if v != "normal" && v != "italic" && v != "oblique" {
					err = fmt.Errorf("unsupported font-style %q", v)
				} else {
					s.Slant = v
				}
			case "color":
				if v == "currentcolor" {
					s.Color = parent.Color
				} else {
					s.Color = v
				}
			case "white-space":
				s.WhiteSpace = v
			case "direction":
				switch v {
				case "ltr":
					s.Direction = bidi.LeftToRight
				case "rtl":
					s.Direction = bidi.RightToLeft
				default:
					err = fmt.Errorf("unsupported direction %q", v)
				}
			case "unicode-bidi":
				s.UnicodeBidi = v
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", n.Path, key, err)
			}
		}
		if n.Name == "html" {
			rootSize = s.EmSize
		}
		result[n.ID] = s
	}
	return result, nil
}
func assignProperty(s *Style, from Style, key string) {
	switch key {
	case "font-family":
		s.Family = from.Family
	case "font-size":
		s.EmSize = from.EmSize
	case "font-weight":
		s.Weight = from.Weight
	case "font-style":
		s.Slant = from.Slant
	case "color":
		s.Color = from.Color
	case "white-space":
		s.WhiteSpace = from.WhiteSpace
	case "direction":
		s.Direction = from.Direction
	case "unicode-bidi":
		s.UnicodeBidi = from.UnicodeBidi
	}
}
func resolveSize(v string, parent, root dimen.DU) (dimen.DU, error) {
	var value float64
	keywords := map[string]float64{"xx-small": 6.75, "x-small": 7.5, "small": 9.75, "medium": 12, "large": 13.5, "x-large": 18, "xx-large": 24}
	if points, ok := keywords[v]; ok {
		value = points * float64(dimen.BP)
	} else if v == "larger" {
		value = float64(parent) * 1.2
	} else if v == "smaller" {
		value = float64(parent) / 1.2
	} else {
		units := []struct {
			suffix string
			scale  float64
		}{{"rem", float64(root)}, {"em", float64(parent)}, {"px", .75 * float64(dimen.BP)}, {"pt", float64(dimen.BP)}, {"%", float64(parent) / 100}}
		found := false
		for _, unit := range units {
			if strings.HasSuffix(v, unit.suffix) {
				n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, unit.suffix)), 64)
				if err != nil {
					return 0, err
				}
				value = n * unit.scale
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("unsupported font-size %q (use px, pt, em, rem or %%)", v)
		}
	}
	value = math.Round(value)
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value >= float64(dimen.Fil) {
		return 0, fmt.Errorf("font-size must be positive and finite")
	}
	return dimen.DU(value), nil
}
func (d *Document) firstStrong(id int, fallback bidi.Direction) bidi.Direction {
	var scan func(int) (bidi.Direction, bool)
	scan = func(i int) (bidi.Direction, bool) {
		for _, r := range d.Nodes[i].Text {
			p, _ := ubidi.LookupRune(r)
			switch p.Class() {
			case ubidi.L:
				return bidi.LeftToRight, true
			case ubidi.R, ubidi.AL:
				return bidi.RightToLeft, true
			}
		}
		for _, child := range d.Nodes[i].Children {
			if dir, ok := scan(child); ok {
				return dir, true
			}
		}
		return fallback, false
	}
	dir, _ := scan(id)
	return dir
}

// StyledText constructs a persistent text with explicitly assigned style ranges.
// It deliberately avoids the current cords TextBuilder's style-loss path.
func (e *Extraction) StyledText() (styled.Text, error) {
	if err := e.Validate(e.Text); err != nil {
		return styled.Text{}, err
	}
	text := styled.TextFromString(e.Text)
	for _, span := range e.Styles {
		var err error
		text, err = text.Style(span.Style, span.Start, span.End)
		if err != nil {
			return styled.Text{}, err
		}
	}
	return text, nil
}

// BidiMarkup maps isolate boundaries to the UAX library's zero-byte delimiters.
// The callback can represent one close and one open at each byte position.
func (e *Extraction) BidiMarkup() (bidi.OutOfLineBidiMarkup, error) {
	ordered := append([]Isolate(nil), e.Isolates...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	var stack []uint64
	for _, span := range ordered {
		for len(stack) > 0 && span.Start >= stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && span.End > stack[len(stack)-1] {
			return nil, fmt.Errorf("crossing bidi isolates")
		}
		stack = append(stack, span.End)
	}
	boundaries := make(map[uint64]int)
	for _, span := range e.Isolates {
		if span.Start >= span.End || span.End > uint64(len(e.Text)) {
			return nil, fmt.Errorf("invalid bidi isolate range")
		}
		open := bidi.MarkupLRI
		if span.Direction == bidi.RightToLeft {
			open = bidi.MarkupRLI
		} else if span.Direction != bidi.LeftToRight {
			return nil, fmt.Errorf("invalid isolate direction")
		}
		if boundaries[span.Start]&^bidi.MarkupPDI != 0 || boundaries[span.End]&bidi.MarkupPDI != 0 {
			return nil, fmt.Errorf("coincident nested bidi boundaries are unsupported")
		}
		boundaries[span.Start] |= open
		boundaries[span.End] |= bidi.MarkupPDI
	}
	return func(pos uint64) int { return boundaries[pos] }, nil
}
