package replay

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/fontresource"
)

func TestWireProvenanceCodesAndFieldNames(t *testing.T) {
	for _, tc := range []struct {
		style  fontresource.Slant
		weight fontresource.Weight
		want   string
	}{
		{fontresource.Normal, fontresource.Thin, `{"Provider":"offline","Name":"Example","Location":"example.ttf","Request":{"Pattern":"Example","Style":0,"Weight":-3}}`},
		{fontresource.Oblique, fontresource.Black, `{"Provider":"offline","Name":"Example","Location":"example.ttf","Request":{"Pattern":"Example","Style":2,"Weight":5}}`},
		{99, 99, `{"Provider":"offline","Name":"Example","Location":"example.ttf","Request":{"Pattern":"Example","Style":99,"Weight":99}}`},
	} {
		origin := fontresource.Origin{Provider: "offline", Name: "Example", Location: "example.ttf", Request: fontresource.Request{Pattern: "Example", Style: tc.style, Weight: tc.weight}}
		data, err := json.Marshal(originWire(origin))
		if err != nil || string(data) != tc.want {
			t.Fatalf("wire fields/codes changed: %s %v", data, err)
		}
		var wire wireOrigin
		if err = json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.native() != origin {
			t.Fatal("unknown provenance codes lost")
		}
	}
	diagnostics := []discovery.FontDiagnostic{{Stage: "resolved", Request: fontresource.Request{Pattern: "A", Style: fontresource.Italic, Weight: fontresource.Bold}, Substituted: true}}
	if !reflect.DeepEqual(diagnosticsNative(diagnosticsWire(diagnostics)), diagnostics) {
		t.Fatal("diagnostics changed")
	}
	info := typography.Info{Warnings: []string{"retained"}}
	detached := infoWire(info)
	info.Warnings[0] = "mutated"
	if detached.Warnings[0] != "retained" {
		t.Fatal("wire aliases runtime metadata")
	}
	if bindingsWire(nil) != nil || diagnosticsWire(nil) != nil || diagnosticsNative(nil) != nil {
		t.Fatal("nil became empty in wire evidence")
	}
}
