package replay

import (
	"bytes"
	"encoding/json"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"

	"tytui/fontselection"
	"tytui/htmlinput"
	"tytui/pipeline"
)

// These records describe font provenance independently of runtime types. They are
// serialization values only, never runtime wrappers around extracted services.
// Request codes are provenance codes (-3..5 weights), not CSS numeric weights.
type wireRequest struct {
	Pattern       string
	Style, Weight int
}
type wireOrigin struct {
	Provider, Name, Location string
	Request                  wireRequest
}
type wireInfo struct {
	ResourceID                      string
	Origin                          wireOrigin
	Path, Family, Subfamily, SHA256 string
	Warnings                        []string
}
type wireBinding struct {
	Start, End               uint64
	Path, SHA256, ResourceID string
	Origin                   wireOrigin
	EmSize                   dimen.DU
	Script                   string
}
type wireDiagnostic struct {
	Stage, Provider, Message, ResourceID, SHA256 string
	Request                                      wireRequest
	Substituted                                  bool
}

func requestWire(r fontresource.Request) wireRequest {
	return wireRequest{r.Pattern, int(r.Style), int(r.Weight)}
}
func (r wireRequest) native() fontresource.Request {
	return fontresource.Request{Pattern: r.Pattern, Style: fontresource.Slant(r.Style), Weight: fontresource.Weight(r.Weight)}
}
func originWire(o fontresource.Origin) wireOrigin {
	return wireOrigin{o.Provider, o.Name, o.Location, requestWire(o.Request)}
}
func (o wireOrigin) native() fontresource.Origin {
	return fontresource.Origin{Provider: o.Provider, Name: o.Name, Location: o.Location, Request: o.Request.native()}
}
func infoWire(i typography.Info) wireInfo {
	return wireInfo{i.ResourceID, originWire(i.Origin), i.Path, i.Family, i.Subfamily, i.SHA256, append([]string(nil), i.Warnings...)}
}
func bindingsWire(bindings []typography.Binding) []wireBinding {
	if bindings == nil {
		return nil
	}
	out := make([]wireBinding, len(bindings))
	for n, b := range bindings {
		out[n] = wireBinding{b.Start, b.End, b.Path, b.SHA256, b.ResourceID, originWire(b.Origin), b.EmSize, b.Script}
	}
	return out
}
func diagnosticsWire(ds []fontselection.FontDiagnostic) []wireDiagnostic {
	if ds == nil {
		return nil
	}
	out := make([]wireDiagnostic, len(ds))
	for n, d := range ds {
		out[n] = wireDiagnostic{d.Stage, d.Provider, d.Message, d.ResourceID, d.SHA256, requestWire(d.Request), d.Substituted}
	}
	return out
}
func diagnosticsNative(ds []wireDiagnostic) []fontselection.FontDiagnostic {
	if ds == nil {
		return nil
	}
	out := make([]fontselection.FontDiagnostic, len(ds))
	for n, d := range ds {
		out[n] = fontselection.FontDiagnostic{Stage: d.Stage, Provider: d.Provider, Message: d.Message, ResourceID: d.ResourceID, SHA256: d.SHA256, Request: d.Request.native(), Substituted: d.Substituted}
	}
	return out
}

// wireInput freezes the paragraph input shape; Fonts must remain null on disk.
// Runtime selections, faces and services must never be decoded from a manifest.
type wireInput struct {
	Assembly   paragraph.SpacingPolicy
	LineLayout *paragraph.LineLayout `json:",omitempty"`
	Styles     []style.Span          `json:",omitempty"`
	Fonts      json.RawMessage
	Text       string
	Width      dimen.DU
	Direction  bidi.Direction
	Breaker    pipeline.Breaker
	Source     *htmlinput.Extraction
}

func inputWire(in pipeline.Input) wireInput {
	return wireInput{LineLayout: in.LineLayout, Styles: in.Styles, Text: in.Text, Width: in.Width, Direction: in.Direction, Breaker: in.Breaker, Source: in.Source}
}
func (in wireInput) native() pipeline.Input {
	return pipeline.Input{LineLayout: in.LineLayout, Styles: in.Styles, Text: in.Text, Width: in.Width, Direction: in.Direction, Breaker: in.Breaker, Source: in.Source}
}
func (in wireInput) hasFonts() bool {
	return len(in.Fonts) > 0 && !bytes.Equal(bytes.TrimSpace(in.Fonts), []byte("null"))
}

type wirePolicy struct {
	Providers []string
	Generics  map[string]string
}
type wireFontRequest struct {
	Families          []string
	Weight            int
	Slant             string
	AllowSubstitution bool
}
type wireDescriptorRule struct {
	Family  string
	Weight  int
	Slant   string
	Request wireFontRequest
}
type wireDescriptors struct {
	Default   wireFontRequest
	Rules     []wireDescriptorRule
	Fallbacks []wireFontRequest
	Size      dimen.DU
}

func policyWire(p *fontselection.DiscoveryPolicy) *wirePolicy {
	if p == nil {
		return nil
	}
	c := p.Clone()
	return &wirePolicy{c.Providers, c.Generics}
}
func (p *wirePolicy) native() *fontselection.DiscoveryPolicy {
	if p == nil {
		return nil
	}
	return (&fontselection.DiscoveryPolicy{Providers: p.Providers, Generics: p.Generics}).Clone()
}
func fontRequestWire(r fontselection.FontRequest) wireFontRequest {
	return wireFontRequest{append([]string(nil), r.Families...), r.Weight, r.Slant, r.AllowSubstitution}
}
func (r wireFontRequest) native() fontselection.FontRequest {
	return fontselection.FontRequest{Families: append([]string(nil), r.Families...), Weight: r.Weight, Slant: r.Slant, AllowSubstitution: r.AllowSubstitution}
}
func descriptorsWire(d *fontselection.DescriptorSettings) *wireDescriptors {
	if d == nil {
		return nil
	}
	d = d.Clone()
	out := &wireDescriptors{Default: fontRequestWire(d.Default), Size: d.Size}
	for _, r := range d.Rules {
		out.Rules = append(out.Rules, wireDescriptorRule{r.Family, r.Weight, r.Slant, fontRequestWire(r.Request)})
	}
	for _, r := range d.Fallbacks {
		out.Fallbacks = append(out.Fallbacks, fontRequestWire(r))
	}
	return out
}
func (d *wireDescriptors) native() *fontselection.DescriptorSettings {
	if d == nil {
		return nil
	}
	out := &fontselection.DescriptorSettings{Default: d.Default.native(), Size: d.Size}
	for _, r := range d.Rules {
		out.Rules = append(out.Rules, fontselection.DescriptorRule{Family: r.Family, Weight: r.Weight, Slant: r.Slant, Request: r.Request.native()})
	}
	for _, r := range d.Fallbacks {
		out.Fallbacks = append(out.Fallbacks, r.native())
	}
	return out
}
