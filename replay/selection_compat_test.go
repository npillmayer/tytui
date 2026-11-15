package replay

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	ot "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/fontresource"

	"tytui/fontselection"
	"tytui/pipeline"
)

func TestSelectionWireShapeAndEmptyNormalization(t *testing.T) {
	for _, empty := range []bool{false, true} {
		p := &fontselection.DiscoveryPolicy{}
		d := &fontselection.DescriptorSettings{}
		if empty {
			p.Providers = []string{}
			p.Generics = map[string]string{}
			d.Default.Families = []string{}
			d.Rules = []fontselection.DescriptorRule{}
			d.Fallbacks = []fontselection.FontRequest{}
		}
		data, _ := json.Marshal(policyWire(p))
		if string(data) != `{"Providers":null,"Generics":{}}` {
			t.Fatal(string(data))
		}
		data, _ = json.Marshal(descriptorsWire(d))
		if string(data) != `{"Default":{"Families":null,"Weight":0,"Slant":"","AllowSubstitution":false},"Rules":null,"Fallbacks":null,"Size":0}` {
			t.Fatal(string(data))
		}
		if !reflect.DeepEqual(policyWire(policyWire(p).native()), policyWire(p)) || !reflect.DeepEqual(descriptorsWire(descriptorsWire(d).native()), descriptorsWire(d)) {
			t.Fatal("normalization changed on roundtrip")
		}
	}
	data, _ := json.Marshal(inputWire(pipeline.Input{Fonts: &fontselection.Selection{}}))
	if string(data) != `{"Fonts":null,"Text":"","Width":0,"Direction":0,"Breaker":"","Source":null}` {
		t.Fatal(string(data))
	}
	for _, raw := range []string{`{}`, `[]`, `false`, `"font"`} {
		if !(wireInput{Fonts: json.RawMessage(raw)}).hasFonts() {
			t.Fatal("accepted unpinned fonts")
		}
	}
	if policyWire(nil) != nil || descriptorsWire(nil) != nil {
		t.Fatal("nil provenance changed")
	}
}
func TestSelectionResaveNormalizesRequestedPaths(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "fonts")
	selected := selectFonts(t, &fontselection.Request{Settings: fontselection.Settings{DefaultPath: latin, Size: 12 * dimen.BP, Rules: []fontselection.Rule{{Weight: 700, Path: latin}}, Fallbacks: []string{hebrew, hebrew}}})
	result, err := pipeline.Run(t.Context(), pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: selected})
	if err != nil {
		t.Fatal(err)
	}
	// Inspection metadata cannot change the provenance retained at layout time.
	result.Input.Fonts.Provenance.RequestedPath = "mutated"
	result.Input.Fonts.Provenance.RequestedRulePaths[0] = "mutated"
	first := filepath.Join(dir, "first.json")
	if err = Save(t.Context(), result, first, store, revisions()); err != nil {
		t.Fatal(err)
	}
	read := func(path string) Manifest {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var m Manifest
		if err = json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	original := read(first)
	if original.Selection.RequestedPath != latin || original.Selection.Rules[0].RequestedPath != latin || !reflect.DeepEqual(original.Selection.RequestedFallbackPaths, []string{hebrew, hebrew}) {
		t.Fatal("new save lost requested paths")
	}
	replayed, err := Load(t.Context(), first, store)
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "second.json")
	if err = Save(t.Context(), replayed, second, store, revisions()); err != nil {
		t.Fatal(err)
	}
	resaved := read(second)
	want := *original.Selection
	want.RequestedPath = ""
	want.Rules = append([]Rule(nil), want.Rules...)
	want.Rules[0].RequestedPath = ""
	want.RequestedFallbackPaths = nil
	if !reflect.DeepEqual(*resaved.Selection, want) {
		t.Fatalf("normalized resave changed: %+v want %+v", resaved.Selection, want)
	}
	if !reflect.DeepEqual(resaved.Fonts, original.Fonts) || !reflect.DeepEqual(resaved.Evidence, original.Evidence) || !reflect.DeepEqual(resaved.Bindings, original.Bindings) {
		t.Fatal("resave changed evidence")
	}
}
func TestReplayValidatesUnreferencedFontsWithoutSelectingThem(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "fonts")
	path := filepath.Join(dir, "case.json")
	result, err := pipeline.Run(t.Context(), pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: selectFonts(t, &fontselection.Request{Settings: fontselection.Settings{DefaultPath: latin, Size: 12 * dimen.BP}})})
	if err != nil {
		t.Fatal(err)
	}
	if err = Save(t.Context(), result, path, store, revisions()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var m Manifest
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(latin)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := fontresource.Read(bytes.NewReader(data), fontresource.Origin{Provider: "unused", Location: "not-an-executable-path", Request: fontresource.Request{Style: 99, Weight: 99}})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := (ot.Services{}).Load(t.Context(), "", resource)
	if err != nil {
		t.Fatal(err)
	}
	m.Fonts = append(m.Fonts, Font{Info: infoWire(entry.Info), Bytes: int64(len(data))})
	write := func(name string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	got, err := Load(t.Context(), write("extra.json"), store)
	if err != nil || len(got.FontSnapshot.Faces) != 1 {
		t.Fatal("unreferenced legacy font became selection", err)
	}
	m.Fonts[1].Info.Family = "tampered"
	if _, err = Load(t.Context(), write("tampered.json"), store); err == nil {
		t.Fatal("unreferenced entry escaped validation")
	}
}
