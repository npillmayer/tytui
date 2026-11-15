package documentlayout

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	typestyle "github.com/npillmayer/typography/style"
	"golang.org/x/net/html"

	"tytui/htmlinput"
)

func unsupported(message string) error {
	return fmt.Errorf("%w: documentlayout: %s", khipu.ErrUnsupportedPolicy, message)
}

// The document experiment prepares every group with NoHyphenation, for both
// measurement and final layout. Reject authored discretionary markers unless CSS
// explicitly disables them; accepting manual here would silently override CSS.
// Resolve inheritance through the DOM, before normalization changes byte offsets.
func validateTextHyphenation(d *htmlinput.Document, id int) error {
	n := d.Nodes[id]
	offset := strings.IndexRune(n.Text, '\u00ad')
	if offset < 0 {
		return nil
	}
	value, err := d.Property(id, "hyphens")
	if err != nil {
		return err
	}
	if value != "none" {
		return unsupported(fmt.Sprintf("%s: decoded text byte %d: U+00AD with hyphens:%s conflicts with the document NoHyphenation profile; use hyphens:none to explicitly disable discretionary breaks", n.Path, offset, value))
	}
	return nil
}

func supportedElement(name string) bool {
	return strings.Contains(" body div p section article main header footer nav aside blockquote h1 h2 h3 h4 h5 h6 figure figcaption a abbr b bdi cite code del dfn em i ins kbd mark q s samp small span strong sub sup time u var ", " "+name+" ")
}

func visible(d *htmlinput.Document, id int) error {
	n := d.Nodes[id]
	if n.Type != html.ElementNode {
		return nil
	}
	if n.Name == "head" || n.Name == "script" || n.Name == "style" {
		return unsupported(n.Path + ": not rendered paragraph content")
	}
	for _, a := range n.Attributes {
		if a.Key == "hidden" {
			return unsupported(n.Path + ": hidden content")
		}
	}
	for _, key := range []string{"display", "visibility"} {
		v, err := d.Property(id, key)
		if err != nil {
			return err
		}
		if v == "none" || v == "hidden" || v == "collapse" {
			return unsupported(n.Path + ": hidden content")
		}
	}
	return nil
}

// Box lengths deliberately accept only resolved px (plus auto where supported),
// leaving relative lengths and sizing semantics to later CSS integration work.
func pixelLength(value string, auto, signed bool) (box.Length, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "auto" && auto {
		return box.Length{Kind: box.LenAuto}, nil
	}
	if !strings.HasSuffix(v, "px") {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n == 0 {
			return box.Length{Kind: box.LenPx}, nil
		}
		return box.Length{}, unsupported("box length " + value + "; use px")
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || (!signed && n < 0) || math.Abs(n) > math.MaxFloat32 {
		return box.Length{}, invalid("invalid box length " + value)
	}
	return box.Length{Kind: box.LenPx, Value: float32(n)}, nil
}

func boxStyle(d *htmlinput.Document, id int, display string, s typestyle.Style, leaf bool) (box.ComputedStyle, error) {
	result := box.DefaultComputedStyle()
	result.FontSizePx = float32(float64(s.EmSize) / float64(dimen.BP) * 4 / 3)
	get := func(key string) (string, error) { return d.Property(id, key) }
	for _, policy := range []struct{ key, want string }{
		{"line-height", "normal"}, {"float", "none"}, {"position", "static"}, {"height", "auto"},
		{"min-width", "none"}, {"max-width", "none"}, {"min-height", "none"}, {"max-height", "none"},
		{"word-spacing", "normal"}, {"letter-spacing", "normal"}, {"word-break", "normal"}, {"overflow-wrap", "normal"},
	} {
		if leaf && policy.key == "height" {
			continue
		}
		v, err := get(policy.key)
		if err != nil {
			return result, err
		}
		if v != policy.want {
			return result, unsupported(d.Nodes[id].Path + ": " + policy.key + " " + v)
		}
	}
	if v, err := get("hyphens"); err != nil {
		return result, err
	} else if v != "manual" && v != "none" {
		return result, unsupported("automatic hyphenation")
	}
	v, err := get("width")
	if err != nil {
		return result, err
	}
	result.Width, err = pixelLength(v, true, false)
	if err != nil {
		return result, err
	}
	for _, side := range []struct {
		name                    string
		margin, padding, border *box.Length
	}{
		{"top", &result.Margin.Top, &result.Padding.Top, &result.Border.Top},
		{"right", &result.Margin.Right, &result.Padding.Right, &result.Border.Right},
		{"bottom", &result.Margin.Bottom, &result.Padding.Bottom, &result.Border.Bottom},
		{"left", &result.Margin.Left, &result.Padding.Left, &result.Border.Left},
	} {
		for _, edge := range []struct {
			key          string
			out          *box.Length
			auto, signed bool
		}{
			{"margin-" + side.name, side.margin, true, true}, {"padding-" + side.name, side.padding, false, false},
		} {
			v, err := get(edge.key)
			if err != nil {
				return result, err
			}
			*edge.out, err = pixelLength(v, edge.auto, edge.signed)
			if err != nil {
				return result, err
			}
		}
		borderStyle, err := get("border-" + side.name + "-style")
		if err != nil {
			return result, err
		}
		switch borderStyle {
		case "none", "hidden", "solid", "dotted", "dashed", "double", "groove", "ridge", "inset", "outset":
		default:
			return result, invalid("unknown border style " + borderStyle)
		}
		if borderStyle != "none" && borderStyle != "hidden" {
			v, err := get("border-" + side.name + "-width")
			if err != nil {
				return result, err
			}
			*side.border, err = pixelLength(v, false, false)
			if err != nil {
				return result, err
			}
		}
		if display == "inline" {
			for _, edge := range []*box.Length{side.margin, side.padding, side.border} {
				if edge.Kind != box.LenPx || edge.Value != 0 {
					return result, unsupported(d.Nodes[id].Path + ": nonzero ordinary-inline edges")
				}
			}
		}
	}
	return result, nil
}

// Leaf syntax is opt-in; an unreadable image never becomes a rectangle.
func parseLeaf(n htmlinput.Node) (LeafPayload, string, error) {
	var payload LeafPayload
	var src string
	for _, a := range n.Attributes {
		switch a.Key {
		case "data-tytui-leaf":
			if n.Name != "img" || (a.Val != "rectangle" && a.Val != "image") {
				return payload, "", unsupported(n.Path + ": unknown leaf")
			}
			payload.Kind = a.Val
		case "alt":
			payload.Label = a.Val
		case "src":
			src = a.Val
		}
	}
	if payload.Kind != "" {
		for _, a := range n.Attributes {
			if a.Key == "srcset" || a.Key == "width" || a.Key == "height" || (payload.Kind == "rectangle" && a.Key == "src") {
				return payload, "", unsupported(n.Path + ": leaf requires CSS dimensions; image resources require the image marker")
			}
		}
		if payload.Kind == "image" && strings.TrimSpace(src) == "" {
			return payload, "", unsupported(n.Path + ": image requires src")
		}
	}
	return payload, src, nil
}
