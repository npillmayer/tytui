package paragraphtext

import (
	"reflect"
	"testing"
)

func TestNormalizeByteSpans(t *testing.T) {
	got, spans := Normalize("\r\né\r\n世\n")
	want := []Span{{0, 0, 0, 2, "omitted"}, {0, 2, 2, 4, ""}, {2, 3, 4, 6, "collapsed"}, {3, 6, 6, 9, ""}, {6, 6, 9, 10, "omitted"}}
	if got != "é 世" || !reflect.DeepEqual(spans, want) {
		t.Fatalf("%q %+v", got, spans)
	}
}
func TestWhitespacePolicy(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"", ""}, {" \r\n\t\n", ""}, {"a\r\n\r\nb", "a b"}, {"a\tb\vc\fd\u0085e\u2028f\u2029g", "a b c d e f g"}, {" a\u00a0  b ", "a\u00a0 b"}, {"é a", "é a"}} {
		got, spans := Normalize(tc.raw)
		if got != tc.want {
			t.Fatalf("%q → %q", tc.raw, got)
		}
		var sourceEnd, outEnd uint64
		for _, s := range spans {
			if s.SourceStart != sourceEnd || s.Start != outEnd {
				t.Fatal("map gap")
			}
			sourceEnd, outEnd = s.SourceEnd, s.End
			if s.Kind == "" && tc.raw[s.SourceStart:s.SourceEnd] != got[s.Start:s.End] {
				t.Fatal("identity changed")
			}
		}
		if sourceEnd != uint64(len(tc.raw)) || outEnd != uint64(len(got)) {
			t.Fatal("incomplete mapping")
		}
		again, _ := Normalize(got)
		if again != got {
			t.Fatal("not idempotent")
		}
	}
}
