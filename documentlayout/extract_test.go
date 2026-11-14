package documentlayout_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	box "github.com/npillmayer/css-box-layout/layout"
	boxtext "github.com/npillmayer/css-box-layout/text"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/uax/bidi"
	"golang.org/x/net/html"

	"tytui/documentlayout"
	"tytui/htmlinput"
	"tytui/paragraphtext"
)

func parse(t *testing.T, source string) (*htmlinput.Document, int) {
	t.Helper()
	d, err := htmlinput.Parse("fixture.html", strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range d.Nodes {
		for _, a := range n.Attributes {
			if a.Key == "id" && a.Val == "root" {
				return d, n.ID
			}
		}
	}
	t.Fatal("fixture lacks root")
	return nil, 0
}

func build(t *testing.T, source string) (*documentlayout.Document, *htmlinput.Document, int) {
	t.Helper()
	input, id := parse(t, source)
	d, err := documentlayout.Build(t.Context(), input, id)
	if err != nil {
		t.Fatal(err)
	}
	return d, input, id
}

func extractAll(t *testing.T, d *documentlayout.Document) []*documentlayout.InlineGroup {
	t.Helper()
	var groups []*documentlayout.InlineGroup
	for _, id := range d.Groups() {
		g, err := d.Extract(t.Context(), id.GroupID)
		if err != nil {
			t.Fatal(err)
		}
		groups = append(groups, g)
	}
	return groups
}

// Check byte provenance against decoded source, not another normalization result.
func checkSource(t *testing.T, d *documentlayout.Document, groups []*documentlayout.InlineGroup) map[int][]int {
	t.Helper()
	seen := map[int][]int{}
	for _, g := range groups {
		coverage := make([]int, len(g.Text))
		for _, s := range g.TextSources {
			src, ok := d.TextSource(boxtext.TextSourceID(s.Locator.NodeID + 1))
			if !ok || src.Locator != s.Locator {
				t.Fatal("lost source", s)
			}
			if seen[s.Locator.NodeID] == nil {
				seen[s.Locator.NodeID] = make([]int, len(src.Text))
			}
			for i := s.NodeStart; i < s.NodeEnd; i++ {
				seen[s.Locator.NodeID][i]++
			}
			original := src.Text[s.NodeStart:s.NodeEnd]
			if s.Kind == "" {
				if original != g.Text[s.Start:s.End] {
					t.Fatal("identity map", s, original, g.Text)
				}
			} else {
				for _, r := range original {
					if !paragraphtext.IsCollapsible(r) {
						t.Fatal("transformed content", original)
					}
				}
				if s.Kind == "collapsed" && g.Text[s.Start:s.End] != " " {
					t.Fatal(s)
				}
				if s.Kind == "omitted" && s.Start != s.End {
					t.Fatal(s)
				}
			}
			for i := s.Start; i < s.End; i++ {
				coverage[i]++
			}
		}
		if len(g.Anchors) != len(g.AtomicSources) {
			t.Fatal("atomic association lost")
		}
		for i, a := range g.AtomicSources {
			if a.ID == 0 || a.ID != g.Anchors[i].ID || a.Start != g.Anchors[i].ByteOffset || a.End != a.Start+3 || g.Text[a.Start:a.End] != "\uFFFC" {
				t.Fatal(a, g.Anchors)
			}
			if i > 0 && g.Anchors[i-1].ByteOffset >= a.Start {
				t.Fatal("unsorted anchors")
			}
			for j := a.Start; j < a.End; j++ {
				if coverage[j] != 0 {
					t.Fatal("invented text source for atom")
				}
				coverage[j]++
			}
		}
		for i, n := range coverage {
			if n == 0 {
				t.Fatal("unmapped output byte", i, g.Text)
			}
		}
		var end uint64
		for i, s := range g.Styles {
			if s.Start != end || !utf8.RuneStart(g.Text[s.Start]) {
				t.Fatal(s)
			}
			end = s.End
			if g.Paint[i].Start != s.Start || g.Paint[i].End != s.End || g.Paint[i].Paint != s.Style.Paint() {
				t.Fatal("paint coordinates")
			}
			if g.LineLayout.Runs[i].Start != s.Start || g.LineLayout.Runs[i].End != s.End {
				t.Fatal("layout coordinates")
			}
		}
		if end != uint64(len(g.Text)) {
			t.Fatal("style gap")
		}
		if _, err := g.BidiMarkup(); err != nil {
			t.Fatal(err)
		}
	}
	for id, counts := range seen {
		for i, count := range counts {
			if count != 1 {
				t.Fatal("source byte duplicated or omitted", id, i, count)
			}
		}
	}
	return seen
}

func TestNormalizedSplitHoistAndAnonymousContexts(t *testing.T) {
	d, input, _ := build(t, `<div id="root" style="font-size:20px;color:navy">before <span style="color:red;font-size:30px">one<section style="display:block">inside</section>two</span> after</div>`)
	groups := extractAll(t, d)
	if len(groups) != 3 {
		t.Fatal(len(groups))
	}
	for i, want := range []string{"before one", "inside", "two after"} {
		if groups[i].Text != want {
			t.Fatal(i, groups[i].Text)
		}
	}
	if groups[0].Paragraph.Style.EmSize != 15*dimen.BP || groups[2].Paragraph.Style.Color != "navy" || groups[1].Paragraph.Style.EmSize != dimen.DU(22.5*float64(dimen.BP)) || groups[1].Paragraph.Style.Color != "red" {
		t.Fatal("anonymous context lost", d.Groups())
	}
	if groups[0].OwnerID == groups[2].OwnerID || groups[0].OwnerID == d.Root().BoxID {
		t.Fatal("anonymous owners not retained")
	}
	var fragments []*box.LayoutNode
	var walk func(*box.LayoutNode)
	walk = func(n *box.LayoutNode) {
		if n.Box == box.BoxInline {
			fragments = append(fragments, n)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(d.Root())
	if len(fragments) != 2 || fragments[0].NodeID != fragments[1].NodeID || fragments[0].BoxID == fragments[1].BoxID {
		t.Fatal("split source/box identity conflated")
	}
	seen := checkSource(t, d, groups)
	for _, n := range input.Nodes {
		if n.Type == html.TextNode && (n.Text == "before " || n.Text == "inside" || n.Text == "two" || n.Text == " after" || n.Text == "one") {
			if _, ok := seen[n.ID]; !ok {
				t.Fatal("missing text node", n.ID)
			}
		}
	}
}

func TestNestedAtomicsWhitespaceEntitiesAndLiteralMarker(t *testing.T) {
	d, _, _ := build(t, "<div id='root'> \né&amp; <span style='color:red'> \t</span><span id='outer' style='display:inline-block;width:20px;color:purple'> X <span style='display:inline-block;width:0px'>Y</span> Z </span> \t<span style='display:inline-block;width:10px'></span> &nbsp; \uFFFC tail \n</div>")
	groups := extractAll(t, d)
	want := []string{"é& \uFFFC \uFFFC \u00a0 \uFFFC tail", "X \uFFFC Z", "Y", ""}
	if len(groups) != len(want) {
		t.Fatal(len(groups))
	}
	for i, w := range want {
		if groups[i].Text != w {
			t.Fatal(i, groups[i].Text, w)
		}
	}
	if len(groups[0].Anchors) != 2 || len(groups[1].Anchors) != 1 || len(groups[2].Anchors) != 0 {
		t.Fatal("atomic interiors duplicated")
	}
	if groups[0].Anchors[0].ByteOffset != 4 || groups[0].Anchors[1].ByteOffset != 8 {
		t.Fatal(groups[0].Anchors)
	}
	if groups[0].AtomicSources[0].BoxID == groups[1].AtomicSources[0].BoxID {
		t.Fatal("nested occurrence identity reused")
	}
	if groups[0].AtomicSources[0].ID != 1 || groups[0].AtomicSources[1].ID != 2 || groups[1].AtomicSources[0].ID != 1 {
		t.Fatal("occurrence IDs are not group-local")
	}
	checkSource(t, d, groups)
	// Atom style/color is independent of its interior's text-byte sources.
	for _, s := range groups[0].Styles {
		if s.Start <= 4 && s.End > 4 && s.Style.Color != "purple" {
			t.Fatal(s)
		}
	}
}

func TestExtractionMatchesExistingTextPath(t *testing.T) {
	for _, source := range []string{
		"<div id='root'> \n é <span> \t </span><span style='color:red'>  世 </span> z\n </div>",
		`<div id="root" dir="rtl">a <span dir="ltr">é&amp;fi</span> z</div>`,
		`<div id="root"><span style="font-size:150%;color:green">abc</span>&nbsp; tail</div>`,
		`<div id="root"> </div>`,
	} {
		d, input, id := build(t, source)
		old, err := input.Extract(id)
		if err != nil {
			t.Fatal(err)
		}
		g := extractAll(t, d)[0]
		if g.Text != old.Text || !reflect.DeepEqual(g.Styles, old.Styles) || g.Paragraph.Style.Direction != old.Direction {
			t.Fatal("legacy emission changed", g, old)
		}
		var spans []htmlinput.Span
		for _, s := range g.TextSources {
			spans = append(spans, htmlinput.Span{Start: s.Start, End: s.End, NodeID: s.Locator.NodeID, NodePath: s.Locator.Path, NodeStart: s.NodeStart, NodeEnd: s.NodeEnd, Kind: s.Kind})
		}
		if !reflect.DeepEqual(spans, old.Spans) {
			t.Fatal("legacy provenance changed", spans, old.Spans)
		}
		a, err := g.BidiMarkup()
		if err != nil {
			t.Fatal(err)
		}
		b, err := old.BidiMarkup()
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(0); i <= uint64(len(g.Text)); i++ {
			if a(i) != b(i) {
				t.Fatal("bidi projection changed", i)
			}
		}
		checkSource(t, d, []*documentlayout.InlineGroup{g})
	}
}

func TestIsolatesSurviveNormalizationAndAtoms(t *testing.T) {
	d, _, _ := build(t, `<div id="root">a<span dir="rtl">אב<span style="display:inline-block;width:5px" dir="ltr">inside</span>ג</span>z</div>`)
	groups := extractAll(t, d)
	g := groups[0]
	if g.Text != "aאב\uFFFCגz" || len(g.Isolates) != 2 {
		t.Fatal(g)
	}
	markup, err := g.BidiMarkup()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []struct {
		at   uint64
		want int
	}{{1, bidi.MarkupRLI}, {5, bidi.MarkupLRI}, {8, bidi.MarkupPDI}, {10, bidi.MarkupPDI}} {
		if markup(event.at) != event.want {
			t.Fatal(event, markup(event.at))
		}
	}
	g.Isolates = nil
	if markup(5) != bidi.MarkupLRI {
		t.Fatal("callback aliases caller slice")
	}
	if groups[1].Text != "inside" || groups[1].Paragraph.Style.Direction != bidi.LeftToRight || len(groups[1].Isolates) != 0 {
		t.Fatal("atomic scope leaked")
	}
	checkSource(t, d, groups)
}

func TestDocumentExtractionOwnershipAndTypedStyles(t *testing.T) {
	d, input, root := build(t, `<div id="root" style="width:200px;padding:3px;border-width:2px;border-style:solid;margin:4px">a<span style="display:inline-block;width:20px;margin-left:-1px;padding:2px;border-width:1px;border-style:solid">b</span>c</div>`)
	groups := extractAll(t, d)
	before := groups[0]
	rootStyle := *d.Root().Style
	if rootStyle.Width.Value != 200 || rootStyle.Padding.Left.Value != 3 || rootStyle.Border.Top.Value != 2 || rootStyle.Margin.Right.Value != 4 {
		t.Fatal(rootStyle)
	}
	used, err := box.ResolveUsedValues(d.Root(), box.ResolveContext{ContainingBlock: box.Rect{W: 300}, FontSizePx: 16})
	if err != nil {
		t.Fatal(err)
	}
	atomic := before.AtomicSources[0]
	if u := used[atomic.BoxID]; u.ContentWidth != 20 || u.Padding.Left != 2 || u.Border.Left != 1 || u.Margin.Left != -1 {
		t.Fatal(u)
	}
	input.Nodes[root].Text = "changed"
	input.Nodes = nil
	input.OriginalHTML = "gone"
	ids := d.Groups()
	ids[0].Paragraph.Style.Color = "changed"
	before.TextSources[0].Locator.Path = "changed"
	before.Styles[0].Style.Color = "changed"
	before.Paint[0].Paint.Color = "changed"
	before.LineLayout.Runs[0].End = 999
	before.Anchors[0].ID = 999
	before.AtomicSources[0].BoxID = 0
	after, err := d.Extract(t.Context(), ids[0].GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Text != "a\uFFFCc" || after.Anchors[0].ID != 1 || after.AtomicSources[0].BoxID != atomic.BoxID || after.Styles[0].Style.Color == "changed" || after.Paint[0].Paint.Color == "changed" || after.TextSources[0].Locator.Path == "changed" || after.LineLayout.Runs[0].End == 999 {
		t.Fatal("snapshot aliases source or output", after)
	}
	if *d.Root().Style != rootStyle {
		t.Fatal("tree mutated")
	}
	checkSource(t, d, extractAll(t, d))
}

func TestUnsupportedDocumentPolicies(t *testing.T) {
	for _, content := range []string{
		`<span style="display:inline-block">x</span>`,
		`<span style="display:inline-block;width:20%">x</span>`,
		`<span style="padding:1px">x</span>`,
		`<span style="font-size:30px"></span>`,
		`<span style="font-size:30px"> </span>`,
		`<span style="line-height:2">x</span>`,
		`<span style="white-space:pre">x</span>`,
		`<span style="unicode-bidi:bidi-override">x</span>`,
		`<span style="position:absolute">x</span>`,
		`<span style="float:left">x</span>`,
		`<span style="display:none">x</span>`,
		`<span style="display:flex">x</span>`,
		`<br>`, `<img src="x">`, `<custom>x</custom>`,
	} {
		input, id := parse(t, `<div id="root">a `+content+` z</div>`)
		d, err := documentlayout.Build(t.Context(), input, id)
		if d != nil || !errors.Is(err, khipu.ErrUnsupportedPolicy) {
			t.Fatal(content, d, err)
		}
	}
	input, id := parse(t, `<div id="root">a</div>`)
	if d, err := documentlayout.Build(nil, input, id); d != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal(d, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if d, err := documentlayout.Build(ctx, input, id); d != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(d, err)
	}
	d, err := documentlayout.Build(t.Context(), input, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() (*documentlayout.InlineGroup, error){func() (*documentlayout.InlineGroup, error) { return d.Extract(nil, d.Groups()[0].GroupID) }, func() (*documentlayout.InlineGroup, error) { return d.Extract(t.Context(), 0) }} {
		g, err := call()
		if g != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal(g, err)
		}
	}
	g, err := d.Extract(ctx, d.Groups()[0].GroupID)
	if g != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(g, err)
	}
	// IDs follow emission, never sorted BoxID order. The normalized tree's
	// allocator is intentionally unrelated to the sequential occurrence IDs.
	d, _, _ = build(t, `<div id="root"><span style="display:inline-block;width:1px">x</span><span style="display:inline-block;width:1px">y</span></div>`)
	g = extractAll(t, d)[0]
	ids := []contracts.AtomicID{g.AtomicSources[0].ID, g.AtomicSources[1].ID}
	if !slices.Equal(ids, []contracts.AtomicID{1, 2}) {
		t.Fatal(ids)
	}
}

func TestMalformedGeometryAndBidiRanges(t *testing.T) {
	for _, style := range []string{"width:-1px", "width:NaNpx", "padding:-1px", "width:1e100px", "border-style:unknown"} {
		input, id := parse(t, `<div id="root" style="`+style+`">x</div>`)
		if d, err := documentlayout.Build(t.Context(), input, id); d != nil || !errors.Is(err, contracts.ErrIllegalArguments) {
			t.Fatal(style, d, err)
		}
	}
	d, _, _ := build(t, `<div id="root">é</div>`)
	g := extractAll(t, d)[0]
	g.Isolates = []htmlinput.Isolate{{Start: 1, End: 2}}
	if _, err := g.BidiMarkup(); !errors.Is(err, contracts.ErrIllegalArguments) {
		t.Fatal("UTF-8 split accepted", err)
	}
}

func TestSplitIsolateScopesAndEmptyInlinePolicy(t *testing.T) {
	d, _, _ := build(t, `<div id="root">a<span dir="rtl">אב<section style="display:block">ב</section>ג</span>z<span style="color:red;padding:0.0"></span></div>`)
	groups := extractAll(t, d)
	if len(groups) != 3 {
		t.Fatal(len(groups))
	}
	for i, want := range []string{"aאב", "ב", "גz"} {
		if groups[i].Text != want {
			t.Fatal(i, groups[i].Text)
		}
	}
	if !reflect.DeepEqual(groups[0].Isolates, []htmlinput.Isolate{{Start: 1, End: 5, Direction: bidi.RightToLeft}}) || len(groups[1].Isolates) != 0 || !reflect.DeepEqual(groups[2].Isolates, []htmlinput.Isolate{{Start: 0, End: 2, Direction: bidi.RightToLeft}}) {
		t.Fatal("split isolate ranges", groups[0].Isolates, groups[1].Isolates, groups[2].Isolates)
	}
	if groups[1].Paragraph.Style.Direction != bidi.RightToLeft || groups[0].Paragraph.Style.Direction != bidi.LeftToRight || groups[2].Paragraph.Style.Direction != bidi.LeftToRight {
		t.Fatal("hoisted paragraph lost inherited context")
	}
	checkSource(t, d, groups)
}
