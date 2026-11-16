package documentlayout

import (
	"errors"
	"fmt"
	"math"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	boxtext "github.com/npillmayer/css-box-layout/text"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	"github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"
	"golang.org/x/net/html"
	xbidi "golang.org/x/text/unicode/bidi"
)

// Build the three-owner fixture directly from typed CSS input. No DOM parser or
// adapter geometry helper supplies its widths, edges, sources or expectations.
func typedNestedGeometry(t *testing.T, outerStyle, innerStyle box.ComputedStyle, text string) *Document {
	t.Helper()
	rootStyle := box.DefaultComputedStyle()
	rootStyle.FontSizePx = 20
	outerStyle.FontSizePx, innerStyle.FontSizePx = 20, 20
	rootStyle.Width = box.Length{Kind: box.LenAuto}
	textNode := &box.RenderNode{ID: 4, HTML: &html.Node{Type: html.TextNode, Data: text}, Text: &boxtext.TextRef{Source: 4, Range: boxtext.TextRange{End: uint64(len(text))}}}
	inner := &box.RenderNode{ID: 3, HTML: &html.Node{Type: html.ElementNode, Data: "span"}, Style: &innerStyle, Display: box.Display{Kind: box.DisplayBox, Outer: box.OuterInline, Inner: box.InnerFlowRoot}}
	if text != "" {
		inner.ChildrenNodes = []*box.RenderNode{textNode}
	}
	outer := &box.RenderNode{ID: 2, HTML: &html.Node{Type: html.ElementNode, Data: "span"}, Style: &outerStyle, Display: box.Display{Kind: box.DisplayBox, Outer: box.OuterInline, Inner: box.InnerFlowRoot}, ChildrenNodes: []*box.RenderNode{inner}}
	root, err := box.BuildLayoutTree(&box.RenderNode{ID: 1, HTML: &html.Node{Type: html.ElementNode, Data: "div"}, Style: &rootStyle, Display: box.Display{Kind: box.DisplayBox, Outer: box.OuterBlock, Inner: box.InnerFlow}, ChildrenNodes: []*box.RenderNode{outer}}, box.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d := &Document{root: root, nodes: map[box.NodeID]NodeSource{}, sources: map[boxtext.TextSourceID]TextSource{}, groups: map[box.BoxID]groupEntry{}}
	for id := box.NodeID(1); id <= 4; id++ {
		d.nodes[id] = NodeSource{Locator: Locator{DocumentID: "typed-geometry", NodeID: int(id)}, Style: style.Style{EmSize: 15 * dimen.BP, Language: "en", Direction: bidi.LeftToRight, WhiteSpace: "normal", UnicodeBidi: "normal", Color: "black"}}
	}
	d.sources[4] = TextSource{NodeSource: d.nodes[4], Text: text}
	var index func(*box.LayoutNode, *box.LayoutNode)
	index = func(n, owner *box.LayoutNode) {
		if box.IsBlockContainer(n.Box) {
			owner = n
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
	return d
}

func geometryGroup(t *testing.T, r *Result, n int) GroupResult {
	t.Helper()
	groups := r.Groups()
	if n >= len(groups) {
		t.Fatal("missing geometry fixture group", n)
	}
	g, ok := r.Group(groups[n].GroupID)
	if !ok {
		t.Fatal("missing retained group", n)
	}
	return g
}
func requireRect(t *testing.T, label string, got, want box.Rect) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: %+v want %+v", label, got, want)
	}
}
func requireTargets(t *testing.T, g GroupResult, width dimen.DU) {
	t.Helper()
	for i := 0; i < g.Paragraph.Frozen.LineCount(); i++ {
		l, _ := g.Paragraph.Frozen.Line(i)
		if l.TargetWidth != width {
			t.Fatal("wrong retained target", l, width)
		}
	}
}
func requireSingleAtomic(t *testing.T, g GroupResult, width dimen.DU, x, y float64) {
	t.Helper()
	placements := g.Paragraph.VList.AtomicPlacements()
	if len(placements) != 1 || len(g.Input.AtomicSources) != 1 || placements[0].ID != g.Input.AtomicSources[0].ID || placements[0].X != x || placements[0].Y != y {
		t.Fatal("atomic placement", placements, x, y)
	}
	count := 0
	for i := 0; i < g.Paragraph.Frozen.KnotCount(); i++ {
		if id, ok := g.Paragraph.Frozen.Atomic(i); ok {
			count++
			k, _ := g.Paragraph.Frozen.Knot(i)
			if id != placements[0].ID || k.W != width {
				t.Fatal("atomic advance", id, k.W, width)
			}
		}
	}
	if count != 1 {
		t.Fatal("atomic occurrence count", count)
	}
}

// A tiny outline backend lets proof coordinates be tested independently of font
// parsing. Synthetic glyph advances and line metrics still come from real layout.
type geometryProofBackend struct{}

func (geometryProofBackend) Identify(contracts.FontFace) (typography.Info, error) {
	return typography.Info{SHA256: "synthetic"}, nil
}
func (geometryProofBackend) Outline(contracts.FontFace, uint32) (render.Outline, error) {
	return render.Outline{Path: "M0 0", Points: []render.Point{{}}}, nil
}
func (geometryProofBackend) VisualOrder(g []contracts.ShapedGlyph, _ xbidi.Direction) ([]contracts.ShapedGlyph, error) {
	return g, nil
} // Fixtures are LTR.
func geometryProof(t *testing.T, r *Result) *Proof {
	t.Helper()
	backend := geometryProofBackend{}
	p, err := BuildProof(t.Context(), r, render.Services{Backend: backend, Identity: backend})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNestedAutoGeometryOracles(t *testing.T) {
	// Synthetic 20px text has 10px glyph/space advances. "aa bbbb" gives
	// 40..70 content; inner border adds 6, then signed horizontal margins.
	// These tables are hand-calculated; no sizing/conversion helper is an oracle.
	for _, m := range []struct {
		name                     string
		left, right, top, bottom float32
		outer, inner             []float32
	}{
		{"positive", 2, 2, 0, 0, []float32{50, 50, 60, 80}, []float32{40, 40, 50, 70}},
		{"negative left and top", -2, 6, -1, 3, []float32{50, 50, 60, 80}, []float32{40, 40, 50, 70}},
		{"negative net horizontal margins", -8, 2, 0, 0, []float32{40, 40, 60, 70}, []float32{40, 40, 60, 70}},
	} {
		px := func(v float32) box.Length { return box.Length{Kind: box.LenPx, Value: v} }
		edges := func(v float32) box.EdgeLengths {
			return box.EdgeLengths{Top: px(v), Right: px(v), Bottom: px(v), Left: px(v)}
		}
		outerStyle := box.DefaultComputedStyle()
		outerStyle.Padding, outerStyle.Border = edges(2), edges(1)
		innerStyle := outerStyle
		innerStyle.Margin = box.EdgeLengths{Left: px(m.left), Right: px(m.right), Top: px(m.top), Bottom: px(m.bottom)}
		source := fmt.Sprintf(`<div id="root" style="font-size:20px"><span style="display:inline-block;padding:2px;border-width:1px;border-style:solid"><span style="display:inline-block;padding:2px;border-width:1px;border-style:solid;margin:%gpx %gpx %gpx %gpx">aa bbbb</span></span></div>`, m.top, m.right, m.bottom, m.left)
		htmlInput := boundaryInvocation(t, source)
		typed := typedNestedGeometry(t, outerStyle, innerStyle, "aa bbbb")
		for _, input := range []struct {
			name string
			d    *Document
		}{{"typed", typed}, {"HTML", htmlInput.document}} {
			for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
				for n, width := range []float32{0, 30, 66, 100} {
					t.Run(fmt.Sprintf("%s/%s/%s/%g", m.name, input.name, breaker, width), func(t *testing.T) {
						out, err := input.d.Layout(t.Context(), LayoutRequest{Width: width, Breaker: breaker}, htmlInput.config)
						if err != nil {
							t.Fatal(err)
						}
						root, outer, inner := geometryGroup(t, out, 0), geometryGroup(t, out, 1), geometryGroup(t, out, 2)
						geom := out.Geometry()
						rg, og, ig := geom[root.Input.OwnerID], geom[outer.Input.OwnerID], geom[inner.Input.OwnerID]
						// Break choices may differ, so use only the number of accepted text lines.
						// Each line's independent metric oracle is ascent 16, descent 4, height 20px.
						lines := inner.Paragraph.Frozen.LineCount()
						if lines < 1 {
							t.Fatal("no inner text line")
						}
						ih := float32(lines * 20)
						oh := ih + 10 + m.top + m.bottom
						requireRect(t, "inner content", ig.Content, box.Rect{X: m.left + 3, Y: m.top + 3, W: m.inner[n], H: ih})
						requireRect(t, "inner border", ig.Frame, box.Rect{X: m.left, Y: m.top, W: m.inner[n] + 6, H: ih + 6})
						requireRect(t, "outer content", og.Content, box.Rect{X: 3, Y: 3, W: m.outer[n], H: oh})
						requireRect(t, "outer border", og.Frame, box.Rect{W: m.outer[n] + 6, H: oh + 6})
						requireRect(t, "root content", rg.Content, box.Rect{W: width, H: oh + 10})
						requireRect(t, "root border", rg.Frame, rg.Content)
						if ig.RelativeTo != outer.Input.OwnerID || og.RelativeTo != root.Input.OwnerID || rg.RelativeTo != 0 {
							t.Fatal("wrong geometry owner")
						}
						requireTargets(t, root, dimen.DU(width*49152))
						requireTargets(t, outer, dimen.DU(m.outer[n]*49152))
						requireTargets(t, inner, dimen.DU(m.inner[n]*49152))
						requireSingleAtomic(t, outer, dimen.DU((m.inner[n]+6+m.left+m.right)*49152), float64(m.left)*.75, float64(m.top)*.75)
						requireSingleAtomic(t, root, dimen.DU((m.outer[n]+6)*49152), 0, 0)
						for l, line := range inner.Paragraph.VList.Lines() {
							if line.Top != float64(l)*15 || line.Baseline != 12+float64(l)*15 || line.Height != 15 {
								t.Fatal("inner baseline/height", line)
							}
						}
						for _, g := range []GroupResult{root, outer} {
							ls := g.Paragraph.VList.Lines()
							if len(ls) != 1 {
								t.Fatal("single atomic should occupy one line", ls)
							}
							want := float64(oh-4) * .75
							if g.Input.GroupID == root.Input.GroupID {
								want = float64(oh+6) * .75
							}
							if ls[0].Baseline != want || ls[0].Height != want+3 {
								t.Fatal("atomic bottom-margin-edge baseline", ls[0], want)
							}
						}
						p := geometryProof(t, out)
						if len(p.Atomics) != 2 {
							t.Fatal("missing proof atomics")
						}
						for _, a := range p.Atomics {
							wx, wy := 0.0, 0.0
							if a.BoxID == inner.Input.OwnerID {
								wx, wy = float64(3+m.left)*.75, float64(3+m.top)*.75
							}
							if a.X != wx || a.Y != wy {
								t.Fatal("ancestor transform", a, wx, wy)
							}
						}
						first := false
						for _, g := range p.Glyphs {
							if g.GroupID == inner.Input.GroupID && g.SourceByte == 0 {
								first = true
								if g.X != float64(6+m.left)*.75 || g.Y != float64(22+m.top)*.75 {
									t.Fatal("nested first glyph origin", g)
								}
							}
						}
						if !first {
							t.Fatal("missing first glyph")
						}
						if width == 0 {
							line, _ := root.Paragraph.Frozen.Line(0)
							if line.Fit != khipu.FitOverfull || line.Width <= line.TargetWidth {
								t.Fatal("positive overflow was hidden", line)
							}
						}
					})
				}
			}
		}
	}
}

func TestNestedEmptyAutoGeometry(t *testing.T) {
	px := func(v float32) box.Length { return box.Length{Kind: box.LenPx, Value: v} }
	s := box.DefaultComputedStyle()
	s.Padding = box.EdgeLengths{Top: px(2), Right: px(2), Bottom: px(2), Left: px(2)}
	s.Border = box.EdgeLengths{Top: px(1), Right: px(1), Bottom: px(1), Left: px(1)}
	inner := s
	inner.Margin = box.EdgeLengths{Left: px(2), Right: px(2)}
	htmlInput := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;padding:2px;border-width:1px;border-style:solid"><span style="display:inline-block;padding:2px;border-width:1px;border-style:solid;margin:0 2px"></span></span></div>`)
	for _, d := range []*Document{typedNestedGeometry(t, s, inner, ""), htmlInput.document} {
		for _, b := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			out, err := d.Layout(t.Context(), LayoutRequest{Width: 0, Breaker: b}, htmlInput.config)
			if err != nil {
				t.Fatal(err)
			}
			root, outer, inner := geometryGroup(t, out, 0), geometryGroup(t, out, 1), geometryGroup(t, out, 2)
			g := out.Geometry()
			requireRect(t, "empty inner content", g[inner.Input.OwnerID].Content, box.Rect{X: 5, Y: 3, H: 20})
			requireRect(t, "empty inner border", g[inner.Input.OwnerID].Frame, box.Rect{X: 2, W: 6, H: 26})
			requireRect(t, "empty outer content", g[outer.Input.OwnerID].Content, box.Rect{X: 3, Y: 3, W: 10, H: 30})
			requireRect(t, "empty outer border", g[outer.Input.OwnerID].Frame, box.Rect{W: 16, H: 36})
			requireRect(t, "empty root", g[root.Input.OwnerID].Frame, box.Rect{H: 40})
			requireTargets(t, inner, 0)
			requireTargets(t, outer, 10*49152)
			requireSingleAtomic(t, outer, 10*49152, 1.5, 0)
			requireSingleAtomic(t, root, 16*49152, 0, 0)
		}
	}
}

// Half a float32 ULP is the only tolerance for each CSS publication. Addition
// budgets below include the input publication error and the new float32 rounding.
// Expected coordinates never use the adapter's cssToDU/pointsToCSS helpers.
func cssHalfULP(value float64) float64 {
	v := float32(math.Abs(value))
	return float64(math.Nextafter32(v, float32(math.Inf(1)))-v) / 2
}
func requireGeometryBudget(t *testing.T, label string, got, want, budget float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > budget {
		t.Fatalf("%s: %.17g want %.17g; error %.8g exceeds budget %.8g", label, got, want, math.Abs(got-want), budget)
	}
}

func TestNestedGeometryPrimitiveRounding(t *testing.T) {
	// q is binary-exact in float32 CSS and represents 1.5 DU. The empty inner
	// border is 3q = 4.5 DU; margins are -q/2 = -.75 DU and q = 1.5 DU.
	// Independent conversion: 5 - 1 + 2 = 6 DU, NOT round(5.25) = 5 DU.
	// Outer padding adds 3 DU in CSS. Its 9 DU border and two 1.5 DU margins
	// become 9 + 2 + 2 = 13 DU, NOT round(12) = 12 DU.
	const q float32 = 1.0 / 32768
	const bp = 65536.0
	const duCSS = 1.0 / 49152
	px := func(v float32) box.Length { return box.Length{Kind: box.LenPx, Value: v} }
	outerStyle := box.DefaultComputedStyle()
	outerStyle.Padding = box.EdgeLengths{Left: px(q), Right: px(q)}
	outerStyle.Margin = box.EdgeLengths{Left: px(q), Right: px(q)}
	innerStyle := box.DefaultComputedStyle()
	innerStyle.Padding = box.EdgeLengths{Left: px(q), Right: px(2 * q)}
	innerStyle.Margin = box.EdgeLengths{Top: px(-q / 2), Bottom: px(q), Left: px(-q / 2), Right: px(q)}
	htmlInput := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;padding-left:0.000030517578125px;padding-right:0.000030517578125px;margin-left:0.000030517578125px;margin-right:0.000030517578125px"><span style="display:inline-block;padding-left:0.000030517578125px;padding-right:0.00006103515625px;margin-top:-0.0000152587890625px;margin-bottom:0.000030517578125px;margin-left:-0.0000152587890625px;margin-right:0.000030517578125px"></span></span></div>`)
	for _, input := range []struct {
		name string
		d    *Document
	}{{"typed", typedNestedGeometry(t, outerStyle, innerStyle, "")}, {"HTML", htmlInput.document}} {
		for _, b := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			t.Run(input.name+"/"+string(b), func(t *testing.T) {
				out, err := input.d.Layout(t.Context(), LayoutRequest{Width: 0, Breaker: b}, htmlInput.config)
				if err != nil {
					t.Fatal(err)
				}
				root, outer, inner := geometryGroup(t, out, 0), geometryGroup(t, out, 1), geometryGroup(t, out, 2)
				geom := out.Geometry()
				rg, og, ig := geom[root.Input.OwnerID], geom[outer.Input.OwnerID], geom[inner.Input.OwnerID]
				if ig.Content.W != 0 || ig.Frame.W != 3*q || og.Content.W != 1.0/8192 || og.Frame.W != 3.0/16384 || rg.Frame.W != 0 {
					t.Fatal("nested widths or preserved CSS border", rg, og, ig)
				}
				// In particular the CSS-owned 4.5 DU border must not become the paragraph's
				// rounded 5 DU contribution. Text/atomic targets stay in their own domains.
				if float64(ig.Frame.W) == 5*duCSS {
					t.Fatal("CSS border was quantized to DU")
				}
				requireTargets(t, inner, 0)
				requireTargets(t, outer, 6)
				requireTargets(t, root, 0)
				requireSingleAtomic(t, outer, 6, -1/bp, -1/bp)
				requireSingleAtomic(t, root, 13, 2/bp, 0)
				outerLine := outer.Paragraph.VList.Lines()[0]
				rootLine := root.Paragraph.VList.Lines()[0]
				if outerLine.Baseline != 15+1/bp || outerLine.Height != 18+1/bp || rootLine.Baseline != 18+1/bp || rootLine.Height != 21+1/bp {
					t.Fatal("signed vertical primitive rounding", outerLine, rootLine)
				}
				if ig.Frame.H != 20 {
					t.Fatal("empty normal strut changed", ig.Frame.H)
				}
				requireGeometryBudget(t, "outer height", float64(og.Frame.H), 24+duCSS, cssHalfULP(24+duCSS))
				requireGeometryBudget(t, "root height", float64(rg.Frame.H), 28+duCSS, cssHalfULP(28+duCSS))
				requireGeometryBudget(t, "outer border x", float64(og.Frame.X), 2*duCSS, cssHalfULP(2*duCSS))
				requireGeometryBudget(t, "inner border x", float64(ig.Frame.X), -duCSS, cssHalfULP(duCSS))
				requireGeometryBudget(t, "inner border y", float64(ig.Frame.Y), -duCSS, cssHalfULP(duCSS))
				// Outer content adds exactly 1.5 DU of CSS padding to a narrowed 2 DU
				// origin; inner content adds 1.5 DU to a narrowed -1 DU origin.
				outerOriginBudget := cssHalfULP(2*duCSS) + cssHalfULP(3.5*duCSS)
				innerOriginBudget := cssHalfULP(duCSS) + cssHalfULP(.5*duCSS)
				requireGeometryBudget(t, "outer content x", float64(og.Content.X), 3.5*duCSS, outerOriginBudget)
				requireGeometryBudget(t, "inner content x", float64(ig.Content.X), .5*duCSS, innerOriginBudget)
				p := geometryProof(t, out)
				if len(p.Atomics) != 2 || len(p.Glyphs) != 0 {
					t.Fatal("empty nested proof contents")
				}
				for _, a := range p.Atomics {
					if a.BoxID == outer.Input.OwnerID {
						if a.X != 2/bp || a.Y != 0 {
							t.Fatal("precise outer paragraph origin", a)
						}
					} else if a.BoxID == inner.Input.OwnerID {
						// The proof composes the CSS owner-content origin with the precise
						// retained atomic placement. It must not reuse the narrowed child X.
						requireGeometryBudget(t, "precise composed atomic x", a.X, 2.5/bp, outerOriginBudget*.75)
						if a.Y != -1/bp {
							t.Fatal("precise signed y", a)
						}
					} else {
						t.Fatal("foreign atomic")
					}
				}
				for _, b := range p.Boxes {
					if b.BoxID == inner.Input.OwnerID {
						requireGeometryBudget(t, "composed CSS border x", b.Frame.X, 2.5/bp, (outerOriginBudget+cssHalfULP(duCSS))*.75)
						requireGeometryBudget(t, "composed CSS content x", b.Content.X, 4/bp, (outerOriginBudget+innerOriginBudget)*.75)
					}
				}
				// The unquantized path would contribute 11.25 DU to the root. The two
				// independent three-primitive conversions permit at most 3 DU total error;
				// the specific exact fixture gives 13 - 11.25 = 1.75 DU, with no saturation.
				line, _ := root.Paragraph.Frozen.Line(0)
				if line.Width != 13 || math.Abs(float64(line.Width)-11.25) > 3 || line.Fit != khipu.FitOverfull {
					t.Fatal("nested rounding/overflow budget", line)
				}
			})
		}
	}
}

func TestNestedAutoBlockAndInlineGeometry(t *testing.T) {
	// Inline child: 20px fixed content + 4px edges + (-2+4)px margins = 26px
	// advance. Its block has "a" + atomic + "b": content max 10+26+10=46.
	// Block border adds 2, margins add 5 => max contribution 53. "lead" forces
	// outer min 40; outer max is max(40,53,30)=53, NOT their sum.
	i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block;padding:2px;border-width:1px;border-style:solid">lead<div style="padding:1px;margin:2px 3px 3px 2px">a<span style="display:inline-block;width:20px;padding:1px;border-width:1px;border-style:solid;margin-left:-2px;margin-right:4px">z</span>b</div>end</span></div>`)
	for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		t.Run(string(breaker), func(t *testing.T) {
			out, err := i.document.Layout(t.Context(), LayoutRequest{Width: 100, Breaker: breaker}, i.config)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Groups()) != 5 {
				t.Fatal("normalization fixture changed")
			}
			root, lead, block, inner, tail := geometryGroup(t, out, 0), geometryGroup(t, out, 1), geometryGroup(t, out, 2), geometryGroup(t, out, 3), geometryGroup(t, out, 4)
			if lead.Input.Text != "lead" || block.Input.Text != "a\uFFFCb" || inner.Input.Text != "z" || tail.Input.Text != "end" {
				t.Fatal("lost normalized group content")
			}
			outerID := root.Input.AtomicSources[0].BoxID
			g := out.Geometry()
			requireRect(t, "outer border", g[outerID].Frame, box.Rect{W: 59, H: 81})
			requireRect(t, "outer content", g[outerID].Content, box.Rect{X: 3, Y: 3, W: 53, H: 75})
			requireRect(t, "leading anonymous", g[lead.Input.OwnerID].Frame, box.Rect{W: 53, H: 20})
			requireRect(t, "block border", g[block.Input.OwnerID].Frame, box.Rect{X: 2, Y: 22, W: 48, H: 30})
			requireRect(t, "block content", g[block.Input.OwnerID].Content, box.Rect{X: 3, Y: 23, W: 46, H: 28})
			requireRect(t, "trailing anonymous", g[tail.Input.OwnerID].Frame, box.Rect{Y: 55, W: 53, H: 20})
			requireRect(t, "fixed inner border", g[inner.Input.OwnerID].Frame, box.Rect{X: 8, W: 24, H: 24})
			requireRect(t, "fixed inner content", g[inner.Input.OwnerID].Content, box.Rect{X: 10, Y: 2, W: 20, H: 20})
			requireRect(t, "root", g[root.Input.OwnerID].Frame, box.Rect{W: 100, H: 85})
			if g[inner.Input.OwnerID].RelativeTo != block.Input.OwnerID || g[block.Input.OwnerID].RelativeTo != outerID || g[lead.Input.OwnerID].RelativeTo != outerID || g[tail.Input.OwnerID].RelativeTo != outerID {
				t.Fatal("normalized coordinate owners")
			}
			requireTargets(t, root, 100*49152)
			requireTargets(t, lead, 53*49152)
			requireTargets(t, block, 46*49152)
			requireTargets(t, inner, 20*49152)
			requireTargets(t, tail, 53*49152)
			requireSingleAtomic(t, block, 26*49152, 6, 0)
			requireSingleAtomic(t, root, 59*49152, 0, 0)
			for _, entry := range []struct {
				g            GroupResult
				base, height float64
			}{{root, 60.75, 63.75}, {lead, 12, 15}, {block, 18, 21}, {inner, 12, 15}, {tail, 12, 15}} {
				lines := entry.g.Paragraph.VList.Lines()
				if len(lines) != 1 || lines[0].Baseline != entry.base || lines[0].Height != entry.height {
					t.Fatal("retained block/inline baseline", lines, entry.base, entry.height)
				}
			}
			p := geometryProof(t, out)
			found := false
			for _, a := range p.Atomics {
				if a.BoxID == inner.Input.OwnerID {
					found = true
					if a.X != 10.5 || a.Y != 19.5 {
						t.Fatal("absolute nested border origin", a)
					}
				}
			}
			if !found {
				t.Fatal("missing nested proof atomic")
			}
			found = false
			for _, glyph := range p.Glyphs {
				if glyph.GroupID == inner.Input.GroupID {
					found = true
					if glyph.X != 12 || glyph.Y != 33 {
						t.Fatal("absolute nested glyph origin", glyph)
					}
				}
			}
			if !found {
				t.Fatal("missing nested glyph")
			}
		})
	}
}

func TestNestedNegativeCompletedLineRemainsUnsupported(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root" style="font-size:20px"><span style="display:inline-block"><span style="display:inline-block;width:10px;margin-left:-20px"></span></span></div>`)
	for _, b := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
		out, err := i.document.Layout(t.Context(), LayoutRequest{Width: 100, Breaker: b}, i.config)
		if out != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
			t.Fatal("negative completed line silently clamped", b, out, err)
		}
	}
}
