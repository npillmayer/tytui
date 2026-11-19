package documentlayout

import (
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/layout"
)

// Capture the token during the one CSS final callback. Its initial range is the
// complete paragraph, so republish those same records as a VList for the existing
// CSS summary/provenance path. No second layout, freeze or assembly occurs.
func (i *invocation) startFlow(p *preparedGroup, request paragraph.LayoutRequest) (*paragraph.Result, error) {
	token, err := p.paragraph.StartFlow(request)
	if err != nil {
		return nil, err
	}
	f := token.Remainder()
	lines := make([]layout.Line, len(f.Lines()))
	for j, line := range f.Lines() {
		lines[j] = line.Line
	}
	v, err := layout.NewVList(f.Frozen(), f.Box(), lines, f.Materials(), f.AtomicPlacements())
	if err != nil {
		return nil, err
	}
	i.flow.token = token
	return &paragraph.Result{Frozen: f.Frozen(), VList: v}, nil
}
