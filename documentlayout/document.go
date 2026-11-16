// Package documentlayout adapts styled documents to CSS-normalized inline
// groups. Source correspondence and computed typography stay here, outside the
// box and paragraph engines. Layout publishes CSS flow and retained paragraphs
// together after successful whole-document layout.
package documentlayout

import (
	"context"
	"fmt"
	"math"
	"slices"

	box "github.com/npillmayer/css-box-layout/layout"
	boxtext "github.com/npillmayer/css-box-layout/text"
	"github.com/npillmayer/khipu/contracts"
	typestyle "github.com/npillmayer/typography/style"
	"golang.org/x/net/html"

	"tytui/htmlinput"
)

// Locator identifies original DOM source, not a normalized box or HTML byte
// offset. One element may produce several BoxIDs after split/hoist normalization.
type Locator struct {
	DocumentID string
	NodeID     int
	Path       string
}

type NodeSource struct {
	Locator Locator
	Style   typestyle.Style
}

// TextSource holds decoded DOM text-node bytes (entities already decoded).
type TextSource struct {
	NodeSource
	Text string
}

// Group records the actual normalized owner and its inherited paragraph context.
// Anonymous boxes inherit the nearest source block's context, never first text.
type Group struct {
	GroupID, OwnerID box.BoxID
	Paragraph        NodeSource
}

type groupEntry struct {
	Group
	node *box.LayoutNode
}

// Document owns detached source/style tables and a CSS-normalized tree. Root is
// borrowed read-only, like css-box-layout's published structural trees. All other
// accessors return values/copies. The original htmlinput.Document is not retained.
type Document struct {
	root    *box.LayoutNode
	nodes   map[box.NodeID]NodeSource
	sources map[boxtext.TextSourceID]TextSource
	leaves  map[box.NodeID]LeafPayload
	groups  map[box.BoxID]groupEntry
	order   []Group
}

func (d *Document) Root() *box.LayoutNode { return d.root }

// RectangleLabel returns the retained renderer payload for a diagnostic leaf.
// Node IDs are CSS source IDs, as on LayoutNode.NodeID, not DOM preorder IDs.
func (d *Document) RectangleLabel(id box.NodeID) (string, bool) {
	payload, ok := d.leaves[id]
	return payload.Label, ok && payload.Kind == "rectangle"
}

// Leaf returns detached metadata and a shared immutable image, if present.
func (d *Document) Leaf(id box.NodeID) (LeafPayload, bool) { p, ok := d.leaves[id]; return p, ok }

func (d *Document) Groups() []Group                             { return slices.Clone(d.order) }
func (d *Document) NodeSource(id box.NodeID) (NodeSource, bool) { s, ok := d.nodes[id]; return s, ok }
func (d *Document) TextSource(id boxtext.TextSourceID) (TextSource, bool) {
	s, ok := d.sources[id]
	return s, ok
}

// Build snapshots the selected styled block subtree, constructs typed RenderNode
// input, then delegates split/hoist and anonymous groups to CSS core. Inputs must
// be read-only during this call. Errors/cancellation return no partial document.
// The initial subset is normal whitespace, normal line height, horizontal flow,
// fixed-pixel or auto atomic widths and zero ordinary-inline edges. The document
// profile is NoHyphenation: authored U+00AD requires effective hyphens:none;
// manual (including the CSS default) with U+00AD and automatic hyphenation fail
// with khipu.ErrUnsupportedPolicy. Marker errors locate the original decoded
// text-node byte offset, before whitespace normalization. No font coverage is
// checked and no atomic interior is flattened into its enclosing paragraph.
func Build(ctx context.Context, input *htmlinput.Document, selected int) (*Document, error) {
	if ctx == nil {
		return nil, invalid("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil || selected < 0 || selected >= len(input.Nodes) || len(input.Nodes) >= math.MaxUint32 {
		return nil, invalid("invalid document or selected node")
	}
	if input.Nodes[selected].Type != html.ElementNode {
		return nil, invalid("select a block element")
	}
	for id := selected; id >= 0; id = input.Nodes[id].Parent {
		if err := visible(input, id); err != nil {
			return nil, err
		}
	}
	styles, err := input.ResolvedStyles(selected)
	if err != nil {
		return nil, err
	}
	d := &Document{leaves: make(map[box.NodeID]LeafPayload), nodes: make(map[box.NodeID]NodeSource), sources: make(map[boxtext.TextSourceID]TextSource), groups: make(map[box.BoxID]groupEntry)}
	images := make(map[string]*ImageResource) // one byte snapshot per path per build
	var render func(int) (*box.RenderNode, error)
	render = func(id int) (*box.RenderNode, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := input.Nodes[id]
		if n.Type != html.ElementNode && n.Type != html.TextNode {
			return nil, nil
		}
		s := styles[id]
		if s.WhiteSpace != "normal" || (s.UnicodeBidi != "normal" && s.UnicodeBidi != "isolate") {
			return nil, unsupported(n.Path + ": white-space/unicode-bidi")
		}
		nodeID := box.NodeID(id + 1) // Reserve zero for CSS anonymous boxes.
		source := NodeSource{Locator: Locator{input.ID, id, n.Path}, Style: s}
		d.nodes[nodeID] = source
		r := &box.RenderNode{ID: nodeID, HTML: &html.Node{Type: n.Type, Data: n.Name}}
		if n.Type == html.TextNode {
			if err := validateTextHyphenation(input, id); err != nil {
				return nil, err
			}
			sid := boxtext.TextSourceID(id + 1)
			d.sources[sid] = TextSource{NodeSource: source, Text: n.Text}
			r.Text = &boxtext.TextRef{Source: sid, Range: boxtext.TextRange{End: uint64(len(n.Text))}}
			r.HTML.Data = n.Text
			return r, nil
		}
		if err := visible(input, id); err != nil {
			return nil, err
		}
		display, err := input.Display(id)
		if err != nil {
			return nil, err
		}
		r.Display, err = projectDisplay(display)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n.Path, err)
		}
		if id == selected && r.Display.Outer != box.OuterBlock {
			return nil, unsupported(n.Path + ": root must be block flow or block flow-root")
		}
		payload, src, err := parseLeaf(n)
		leaf := payload.Kind != ""
		if err != nil {
			return nil, err
		}
		if !leaf && !supportedElement(n.Name) {
			return nil, unsupported(n.Path + ": unsupported <" + n.Name + ">")
		}
		if leaf {
			if r.Display.Outer != box.OuterInline {
				return nil, unsupported(n.Path + ": leaf must be inline")
			}
		}
		computed, err := boxStyle(input, id, r.Display, s, leaf)
		if err != nil {
			return nil, err
		}
		if leaf {
			height, err := input.Property(id, "height")
			if err != nil {
				return nil, err
			}
			h, err := pixelLength(height, false, false)
			if err != nil {
				return nil, err
			}
			if computed.Width.Kind != box.LenPx {
				return nil, unsupported(n.Path + ": leaf requires explicit CSS width and height in px")
			}
			r.Replaced = &box.ReplacedContent{Height: h.Value}
			if payload.Kind == "image" {
				path, err := localImagePath(input.Name, src)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", n.Path, err)
				}
				resource := images[path]
				if resource == nil {
					resource, err = readImage(ctx, path)
					if err != nil {
						return nil, fmt.Errorf("%s: image %q: %w", n.Path, path, err)
					}
					images[path] = resource
				}
				payload.Image = resource
			}
			d.leaves[nodeID] = payload
		}
		r.Style = &computed
		for _, child := range n.Children {
			c, err := render(child)
			if err != nil {
				return nil, err
			}
			if c != nil {
				r.ChildrenNodes = append(r.ChildrenNodes, c)
			}
		}
		return r, nil
	}
	renderRoot, err := render(selected)
	if err != nil {
		return nil, err
	}
	d.root, err = box.BuildLayoutTree(renderRoot, box.BuildOptions{ReadText: func(ref boxtext.TextRef) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		source, ok := d.sources[ref.Source]
		if !ok || ref.Range.Start > ref.Range.End || !boundary(source.Text, ref.Range.Start) || !boundary(source.Text, ref.Range.End) {
			return "", invalid("invalid text reference")
		}
		return source.Text[ref.Range.Start:ref.Range.End], nil
	}})
	if err != nil {
		return nil, err
	}
	var index func(*box.LayoutNode, *box.LayoutNode, NodeSource) error
	index = func(n, owner *box.LayoutNode, paragraph NodeSource) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if box.IsBlockContainer(n.Box) {
			owner = n
			if n.NodeID != 0 {
				paragraph = d.nodes[n.NodeID]
			}
		}
		if n.Box == box.BoxAnonymousInline {
			g := Group{GroupID: n.BoxID, OwnerID: owner.BoxID, Paragraph: paragraph}
			d.groups[n.BoxID] = groupEntry{Group: g, node: n}
			d.order = append(d.order, g)
		}
		for _, child := range n.Children {
			if err := index(child, owner, paragraph); err != nil {
				return err
			}
		}
		return nil
	}
	if err := index(d.root, nil, NodeSource{}); err != nil {
		return nil, err
	}
	// Validate every normalized group before publishing the snapshot, including
	// nested atomic interiors and unsupported empty metric-bearing inlines.
	for _, g := range d.order {
		if _, err := d.Extract(ctx, g.GroupID); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func invalid(message string) error {
	return fmt.Errorf("%w: documentlayout: %s", contracts.ErrIllegalArguments, message)
}
