package documentlayout

import (
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	boxtext "github.com/npillmayer/css-box-layout/text"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography/glyphing"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"
	"golang.org/x/net/html"

	"tytui/fontselection"
)

// Typed normalization/flow fixture: no HTML/CSS parser supplies expected widths
// or source IDs. It isolates the core-to-paragraph bridge from DOM adaptation.
func TestTypedNestedFixedSizeIntegration(t *testing.T) {
	rootStyle := box.DefaultComputedStyle()
	rootStyle.FontSizePx = 20
	outerStyle := box.DefaultComputedStyle()
	outerStyle.Width = box.Length{Kind: box.LenPx, Value: 30}
	outerStyle.FontSizePx = 10
	innerStyle := outerStyle
	innerStyle.Width.Value = 10
	text := &box.RenderNode{ID: 4, HTML: &html.Node{Type: html.TextNode, Data: "x"}, Text: &boxtext.TextRef{Source: 4, Range: boxtext.TextRange{End: 1}}}
	inner := &box.RenderNode{ID: 3, HTML: &html.Node{Type: html.ElementNode, Data: "span"}, Style: &innerStyle, Display: box.Display{Kind: box.DisplayBox, Outer: box.OuterInline, Inner: box.InnerFlowRoot}, ChildrenNodes: []*box.RenderNode{text}}
	outer := &box.RenderNode{ID: 2, HTML: &html.Node{Type: html.ElementNode, Data: "span"}, Style: &outerStyle, Display: box.Display{Kind: box.DisplayBox, Outer: box.OuterInline, Inner: box.InnerFlowRoot}, ChildrenNodes: []*box.RenderNode{inner}}
	root, err := box.BuildLayoutTree(&box.RenderNode{ID: 1, HTML: &html.Node{Type: html.ElementNode, Data: "div"}, Style: &rootStyle, Display: box.Display{Kind: box.DisplayBox, Outer: box.OuterBlock, Inner: box.InnerFlow}, ChildrenNodes: []*box.RenderNode{outer}}, box.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := style.Style{EmSize: 15 * dimen.BP, Language: "en", Direction: bidi.LeftToRight, WhiteSpace: "normal", UnicodeBidi: "normal", Color: "black"}
	d := &Document{root: root, nodes: map[box.NodeID]NodeSource{}, sources: map[boxtext.TextSourceID]TextSource{}, groups: map[box.BoxID]groupEntry{}}
	for id := box.NodeID(1); id <= 4; id++ {
		if id > 1 {
			s.EmSize = 15 * dimen.BP / 2
		}
		d.nodes[id] = NodeSource{Locator: Locator{DocumentID: "typed", NodeID: int(id)}, Style: s}
	}
	d.sources[4] = TextSource{NodeSource: d.nodes[4], Text: "x"}
	owners := map[box.NodeID]box.BoxID{}
	var index func(*box.LayoutNode, *box.LayoutNode)
	index = func(n, owner *box.LayoutNode) {
		if box.IsBlockContainer(n.Box) {
			owner = n
			owners[n.NodeID] = n.BoxID
		}
		if n.Box == box.BoxAnonymousInline {
			g := Group{GroupID: n.BoxID, OwnerID: owner.BoxID, Paragraph: d.nodes[owner.NodeID]}
			d.groups[n.BoxID] = groupEntry{Group: g, node: n}
			d.order = append(d.order, g)
		}
		for _, c := range n.Children {
			index(c, owner)
		}
	}
	index(root, nil)
	r, err := loadResolver(t.Context(), fontselection.Settings{DefaultPath: "synthetic", Size: 10 * dimen.BP}, testServices{Loader: boundaryLoader{}, Coverage: glyphing.CoverageFuncs{Check: checkSyntheticFace, Lookup: func(contracts.FontFace, rune) (uint32, error) { return 1, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Resolver: r, NewShaper: func() (contracts.StreamingShaper, error) { return helpers.NewSyntheticStreamingShaper(8) }}
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		out, err := d.Layout(t.Context(), LayoutRequest{Width: 100, Breaker: breaker}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		geom := out.Geometry()
		if geom[owners[1]].Frame.H != 20 || geom[owners[2]].Frame != (box.Rect{Y: 4, W: 30, H: 12}) || geom[owners[3]].Frame != (box.Rect{W: 10, H: 10}) {
			t.Fatal("nested strut or border accounting", geom)
		}
		if geom[owners[2]].RelativeTo != owners[1] || geom[owners[3]].RelativeTo != owners[2] {
			t.Fatal("lost typed owners")
		}
		for _, group := range out.Groups() {
			g, ok := out.Group(group.GroupID)
			if !ok {
				t.Fatal("missing group")
			}
			if len(g.Input.AtomicSources) > 0 {
				source := g.Input.AtomicSources[0]
				if source.ID != 1 || source.Locator.DocumentID != "typed" || g.Paragraph.Frozen.SourceLen() != 3 {
					t.Fatal("typed marker/source correspondence", g.Input)
				}
			}
		}
	}
}
