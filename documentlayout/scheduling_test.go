package documentlayout

import (
	"maps"
	"reflect"
	"testing"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing/paragraph"
)

func TestNestedSchedulingThroughParagraphEngine(t *testing.T) {
	// Group indexes refer to normalized source order, checked against independently
	// specified extracted text. Schedules are owner/group identities, not callback
	// indexes masquerading as IDs. Every fixture runs the real measurement,
	// bind/break/freeze/assembly pipeline with synthetic font services.
	for _, tc := range []struct {
		name, html        string
		text              []string
		runs              []int32
		measures, layouts []int
		anonymous         int
	}{
		{
			name: "auto inside auto with multiple shaping runs",
			html: `<span style="display:inline-block"><span style="display:inline-block">a<span style="font-size:10px">bb</span> c</span></span>`,
			text: []string{"\uFFFC", "\uFFFC", "abb c"}, runs: []int32{0, 0, 3}, measures: []int{2, 1}, layouts: []int{2, 1, 0},
		},
		{
			name: "auto behind fixed boundary inside auto",
			html: `<span style="display:inline-block">o<span style="display:inline-block;width:60px;padding:2px;border-width:1px;border-style:solid">f<span style="display:inline-block">aa bb</span></span></span>`,
			text: []string{"\uFFFC", "o\uFFFC", "f\uFFFC", "aa bb"}, runs: []int32{0, 1, 1, 1}, measures: []int{1, 3}, layouts: []int{3, 2, 1, 0},
		},
		{
			name: "auto inside fixed",
			html: `<span style="display:inline-block;width:70px">q<span style="display:inline-block">aa bbbb</span></span>`,
			text: []string{"\uFFFC", "q\uFFFC", "aa bbbb"}, runs: []int32{0, 1, 1}, measures: []int{2}, layouts: []int{2, 1, 0},
		},
		{
			name: "auto fixed and empty siblings",
			html: `p<span style="display:inline-block">aa bb</span><span style="display:inline-block;width:25px">fixed</span><span style="display:inline-block"></span>z`,
			text: []string{"p\uFFFC\uFFFC\uFFFCz", "aa bb", "fixed", ""}, runs: []int32{2, 1, 1, 0}, measures: []int{1, 3}, layouts: []int{1, 2, 3, 0},
		},
		{
			name: "auto with block and anonymous descendants",
			html: `<span style="display:inline-block">lead<div>middle<span style="display:inline-block">aa bb</span></div>tail</span>`,
			text: []string{"\uFFFC", "lead", "middle\uFFFC", "aa bb", "tail"}, runs: []int32{0, 1, 1, 1, 1}, measures: []int{1, 3, 2, 4}, layouts: []int{1, 3, 2, 4, 0}, anonymous: 2,
		},
		{
			name: "entirely fixed nested and sibling atomics",
			html: `a<span style="display:inline-block;width:60px">b<span style="display:inline-block;width:10px"></span></span><span style="display:inline-block;width:25px">cd</span>z`,
			text: []string{"a\uFFFC\uFFFCz", "b\uFFFC", "", "cd"}, runs: []int32{2, 1, 0, 1}, layouts: []int{2, 1, 3, 0},
		},
	} {
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			t.Run(tc.name+"/"+string(breaker), func(t *testing.T) {
				i := boundaryInvocation(t, `<div id="root" style="font-size:20px">`+tc.html+`</div>`)
				i.request = LayoutRequest{Width: 120, Breaker: breaker}
				calls := countPreparation(t, i)
				o := newObservedFlow(t, i, calls)
				order := i.document.order
				if len(order) != len(tc.text) {
					t.Fatalf("normalized groups: %d want %d", len(order), len(tc.text))
				}
				groupIndex := map[box.BoxID]int{}
				for n, g := range order {
					groupIndex[g.GroupID] = n
				}
				nodes := map[box.BoxID]*box.LayoutNode{}
				var index func(*box.LayoutNode)
				index = func(n *box.LayoutNode) {
					nodes[n.BoxID] = n
					for _, c := range n.Children {
						index(c)
					}
				}
				index(i.document.root)
				anonymous := 0
				for _, g := range order {
					if nodes[g.OwnerID].Box == box.BoxAnonymousBlock {
						anonymous++
					}
				}
				if anonymous != tc.anonymous {
					t.Fatal("anonymous owner fixture", anonymous, tc.anonymous)
				}

				containing := box.Rect{W: i.request.Width}
				used, err := box.ResolveUsedValues(i.document.root, box.ResolveContext{ContainingBlock: containing})
				if err != nil {
					t.Fatal(err)
				}
				originalUsed := maps.Clone(used)
				flow, err := box.FlowLayout(i.document.root, used, o, o, box.LayoutContext{ContainingBlock: containing}, box.LayoutOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if !maps.Equal(originalUsed, used) {
					t.Fatal("flow modified caller used values")
				}
				if len(i.prepared) != len(order) || len(i.started) != len(order) || len(i.groups) != len(order) {
					t.Fatal("missing preparation/final paragraph", len(i.prepared), len(i.started), len(i.groups))
				}
				var measured, laidOut []int
				shaped := map[box.BoxID]int32{}
				for _, e := range o.events {
					n, known := groupIndex[e.group]
					if !known || !e.complete || e.owner != order[n].OwnerID {
						t.Fatal("incomplete or misidentified event", e)
					}
					shaped[e.group] += e.runs
					if e.phase == "measure" {
						if len(laidOut) != 0 {
							t.Fatal("measurement interleaved with final layout")
						}
						measured = append(measured, n)
						if e.reused || e.shapes != 1 || e.runs != tc.runs[n] {
							t.Fatal("unexpected measured preparation work", n, e.shapes, e.runs)
						}
						for id, a := range e.intrinsic {
							node := nodes[id]
							if node.Style.Width.Kind == box.LenPx {
								// Fixed bounds come from CSS dimensions, regardless of text or auto
								// descendants. This includes padding/border exactly once.
								edges := e.edges[id]
								want := node.Style.Width.Value + edges.Padding.Left + edges.Padding.Right + edges.Border.Left + edges.Border.Right
								if a.BorderWidths != (box.IntrinsicWidths{MinContent: want, MaxContent: want}) {
									t.Fatal("fixed atomic was not supplied as equal border bounds", a, want)
								}
								for _, child := range node.Children {
									if child.Box == box.BoxAnonymousInline && o.measures[child.BoxID] != 0 {
										t.Fatal("fixed atomic interior measured unnecessarily")
									}
								}
							} else if len(node.Children) == 1 && node.Children[0].Box == box.BoxAnonymousInline {
								child := node.Children[0].BoxID
								childBounds, ok := o.widths[child]
								if !ok {
									t.Fatal("missing nested intrinsic result", child)
								}
								edges := e.edges[id]
								extra := edges.Padding.Left + edges.Padding.Right + edges.Border.Left + edges.Border.Right
								if a.BorderWidths != (box.IntrinsicWidths{MinContent: childBounds.MinContent + extra, MaxContent: childBounds.MaxContent + extra}) {
									t.Fatal("wrong nested border bounds", a)
								}
							}
						}
					} else {
						laidOut = append(laidOut, n)
						if o.measures[e.group] > 0 {
							if !e.reused || e.shapes != 0 || e.runs != 0 {
								t.Fatal("measured group prepared again during layout", n, e)
							}
						} else if e.reused || e.shapes != 1 || e.runs != tc.runs[n] {
							t.Fatal("layout-only preparation work", n, e.shapes, e.runs)
						}
						if e.width != flow.Geometry[e.owner].Content.W {
							t.Fatal("callback target differs from final CSS content width")
						}
						for id, a := range e.atomics {
							g := flow.Geometry[id]
							if g.Frame.W != a.BorderWidth || g.Frame.H != a.BorderHeight {
								t.Fatal("parent received wrong prepared border box", a, g)
							}
						}
						// Retained lines must use the final target; no assumption that different
						// breakers choose the same breaks, line count or actual extent.
						f := i.groups[e.group].Paragraph.Frozen
						for line := 0; line < f.LineCount(); line++ {
							l, _ := f.Line(line)
							if l.TargetWidth != dimen.DU(e.width*49152) {
								t.Fatal("paragraph used a provisional width", l.TargetWidth, e.width)
							}
						}
					}
				}
				if !reflect.DeepEqual(measured, tc.measures) || !reflect.DeepEqual(laidOut, tc.layouts) {
					t.Fatal("callback schedule", measured, laidOut, "want", tc.measures, tc.layouts)
				}
				for n, g := range order {
					if o.layouts[g.GroupID] != 1 || o.measures[g.GroupID] > 1 || shaped[g.GroupID] != tc.runs[n] {
						t.Fatal("duplicated/missing group work", n, o.measures, o.layouts, shaped)
					}
					if got := i.groups[g.GroupID].Input.Text; got != tc.text[n] {
						t.Fatal("group flattened, lost or reordered", n, got, tc.text[n])
					}
				}
			})
		}
	}
}
