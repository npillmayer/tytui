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
	groups  map[box.BoxID]groupEntry
	order   []Group
}

func (d *Document) Root() *box.LayoutNode                       { return d.root }
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
// fixed-pixel atomic widths and zero ordinary-inline edges. No font coverage is
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
	d := &Document{nodes: make(map[box.NodeID]NodeSource), sources: make(map[boxtext.TextSourceID]TextSource), groups: make(map[box.BoxID]groupEntry)}
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
			sid := boxtext.TextSourceID(id + 1)
			d.sources[sid] = TextSource{NodeSource: source, Text: n.Text}
			r.Text = &boxtext.TextRef{Source: sid, Range: boxtext.TextRange{End: uint64(len(n.Text))}}
			r.HTML.Data = n.Text
			return r, nil
		}
		if err := visible(input, id); err != nil {
			return nil, err
		}
		display, err := input.Property(id, "display")
		if err != nil {
			return nil, err
		}
		if id == selected && display != "block" {
			return nil, unsupported(n.Path + ": root must be display:block")
		}
		if !supportedElement(n.Name) {
			return nil, unsupported(n.Path + ": unsupported <" + n.Name + ">")
		}
		if display != "block" && display != "inline" && display != "inline-block" {
			return nil, unsupported(n.Path + ": display " + display)
		}
		computed, err := boxStyle(input, id, display, s)
		if err != nil {
			return nil, err
		}
		r.Style = &computed
		r.Styles = map[string]string{"display": display}
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
	d.root, err = box.BuildLayoutTree(renderRoot, box.BuildOptions{})
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
