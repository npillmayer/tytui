package replay

import (
	"encoding/json"
	"reflect"
	"testing"

	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/fontresource"

	"tytui/fontselection"
)

func TestWireProvenanceIdentityAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		style                 fontresource.Slant
		weight                fontresource.Weight
		wantStyle, wantWeight int
	}{
		{fontresource.Normal, fontresource.Thin, 0, -3},
		{fontresource.Oblique, fontresource.Black, 2, 5},
		{99, 99, 99, 99},
	} {
		origin := fontresource.Origin{Provider: "offline", Name: "Example", Location: "example.ttf", Request: fontresource.Request{Pattern: "Example", Style: tc.style, Weight: tc.weight}}
		data, err := json.Marshal(originWire(origin))
		if err != nil {
			t.Fatal(err)
		}
		var encoded struct{ Request struct{ Style, Weight int } }
		if err := json.Unmarshal(data, &encoded); err != nil {
			t.Fatal(err)
		}
		// These codes are part of immutable resource identity, not CSS weights.
		if encoded.Request.Style != tc.wantStyle || encoded.Request.Weight != tc.wantWeight {
			t.Fatal("resource identity codes changed", string(data))
		}
		var wire wireOrigin
		if err = json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.native() != origin {
			t.Fatal("unknown provenance codes lost")
		}
	}
	diagnostics := []fontselection.FontDiagnostic{{Stage: "resolved", Request: fontresource.Request{Pattern: "A", Style: fontresource.Italic, Weight: fontresource.Bold}, Substituted: true}}
	if !reflect.DeepEqual(diagnosticsNative(diagnosticsWire(diagnostics)), diagnostics) {
		t.Fatal("diagnostics changed")
	}
	info := fontmeta.Info{Warnings: []string{"retained"}}
	detached := infoWire(info)
	info.Warnings[0] = "mutated"
	if detached.Warnings[0] != "retained" {
		t.Fatal("wire aliases runtime metadata")
	}
	if bindingsWire(nil) != nil || diagnosticsWire(nil) != nil || diagnosticsNative(nil) != nil {
		t.Fatal("nil became empty in wire evidence")
	}
}
