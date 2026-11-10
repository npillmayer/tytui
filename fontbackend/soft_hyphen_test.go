package fontbackend

import (
	"context"
	"github.com/npillmayer/khipu/dimen"
	"strings"
	"testing"
	"tytui/htmlinput"
)

func TestSoftHyphenFaceContext(t *testing.T) {
	const latin = "../../opentype/testdata/fonts/GentiumPlus-R.ttf"
	const hebrew = "../../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf"
	resolver, err := LoadSettings(context.Background(), Settings{DefaultPath: latin, Size: 12 * dimen.BP, Fallbacks: []string{hebrew}, Rules: []Rule{{Weight: 700, Path: progressFont}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"אב\u00adגד", "\u00adאב", "אב\u00ad", "אב\u00ad\u00adגד"} {
		t.Run(source, func(t *testing.T) {
			text, bindings, err := resolver.Itemize(context.Background(), source, nil)
			if err != nil {
				t.Fatal(err)
			}
			if text.Raw().Len() != uint64(len(source)) || len(bindings) != 1 || bindings[0].Start != 0 || bindings[0].End != uint64(len(source)) || bindings[0].Path != hebrew || bindings[0].Script != "Hebr" {
				t.Fatalf("lost shaping context: %+v", bindings)
			}
		})
	}
	_, bindings, err := resolver.Itemize(context.Background(), "\u00ad\u00ad", nil)
	if err != nil || len(bindings) != 1 || bindings[0].Path != latin {
		t.Fatalf("marker-only default: %+v %v", bindings, err)
	}
	_, bindings, err = resolver.Itemize(context.Background(), "אב\u00ada", nil)
	if err != nil || len(bindings) != 2 || bindings[0].End != 6 || bindings[0].Path != hebrew || bindings[1].Path != latin {
		t.Fatalf("real font/script boundary lost: %+v %v", bindings, err)
	}
	// A separately styled marker must retain its own explicit mapping. Context
	// inheritance stays inside CSS spans, even when the span has only markers.
	d, err := htmlinput.Parse("styles", strings.NewReader("<p>אב<b>\u00ad</b>גד</p>"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range d.Nodes {
		if n.Name != "p" {
			continue
		}
		e, err := d.Extract(n.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, bindings, err = resolver.Itemize(context.Background(), e.Text, e)
		if err != nil {
			t.Fatal(err)
		}
		if len(bindings) != 3 || bindings[1].Path != progressFont || bindings[1].Start != 4 || bindings[1].End != 6 {
			t.Fatalf("explicit marker style lost: %+v", bindings)
		}
	}
}
