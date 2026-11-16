package documentlayout

import (
	"github.com/npillmayer/typography/paragraph"
	"testing"
)

func TestInlineSummaryLineFreeCollapsePermission(t *testing.T) {
	result := &paragraph.Result{VList: &paragraph.VList{}}
	out, err := inlineSummary(&InlineGroup{}, result)
	if err != nil || !out.AllowsMarginCollapse || out.Extent.H != 0 {
		t.Fatalf("line-free summary=%+v err=%v", out, err)
	}
	// Atomic source coverage still has to be satisfied before permission can be
	// returned; an empty paragraph cannot silently discard an atomic participant.
	out, err = inlineSummary(&InlineGroup{AtomicSources: []AtomicSource{{ID: 1, BoxID: 2}}}, result)
	if err == nil || out.AllowsMarginCollapse {
		t.Fatal("unplaced atomic granted collapse permission")
	}
}
