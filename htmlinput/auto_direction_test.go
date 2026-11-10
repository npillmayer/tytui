package htmlinput

import (
	"testing"

	"github.com/npillmayer/uax/bidi"
)

func TestAutoDirectionSkipsDirectedDescendants(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		want       bidi.Direction
	}{
		{"rtl child", `<p dir="auto"><span dir="rtl">אב</span>abc</p>`, bidi.LeftToRight},
		{"ltr child", `<p dir="auto"><span dir="ltr">abc</span>אב</p>`, bidi.RightToLeft},
		{"auto child", `<p dir="auto"><span dir="auto">אב</span>abc</p>`, bidi.LeftToRight},
		{"case insensitive", `<p dir="auto"><span dir="RTL">אב</span>abc</p>`, bidi.LeftToRight},
		{"implicit bdi", `<p dir="auto"><bdi>אב</bdi>abc</p>`, bidi.LeftToRight},
		{"nested child", `<p dir="auto"><em><span dir="rtl">אב</span></em>abc</p>`, bidi.LeftToRight},
		{"ordinary descendant counts", `<p dir="auto">123<em>אב</em>abc</p>`, bidi.RightToLeft},
		{"root still scanned", `<p dir="auto">אב abc</p>`, bidi.RightToLeft},
		{"no eligible strong uses parent", `<div dir="rtl"><p dir="auto"><span dir="ltr">abc</span>123</p></div>`, bidi.RightToLeft},
		{"default fallback", `<p dir="auto"><span dir="rtl">אב</span>123</p>`, bidi.LeftToRight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := extractP(t, tc.html)
			if e.Direction != tc.want {
				t.Fatalf("direction = %v, want %v", e.Direction, tc.want)
			}
		})
	}
}
