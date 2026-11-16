package documentlayout

import (
	box "github.com/npillmayer/css-box-layout/layout"
	domdisplay "github.com/npillmayer/w3c-dom/style/display"
)

// projectDisplay maps the supported document profile explicitly. Syntax and
// cascade belong to w3c-dom; box normalization and containment belong to CSS core.
// Hidden content is rejected by visible, rather than silently omitted here.
func projectDisplay(v domdisplay.Value) (box.Display, error) {
	bad := func() (box.Display, error) {
		return box.Display{}, unsupported("display " + v.CSS())
	}
	if v.Kind() != domdisplay.Box || v.IsListItem() {
		return bad()
	}
	d := box.Display{Kind: box.DisplayBox}
	switch v.Outer() {
	case domdisplay.Block:
		d.Outer = box.OuterBlock
	case domdisplay.Inline:
		d.Outer = box.OuterInline
	default:
		return bad()
	}
	switch v.Inner() {
	case domdisplay.Flow:
		d.Inner = box.InnerFlow
	case domdisplay.FlowRoot:
		d.Inner = box.InnerFlowRoot
	default:
		return bad()
	}
	return d, nil
}
