package replay

import (
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/fontresource"
)

// These records freeze the version-1/version-2 font wire contract. They are
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
func diagnosticsWire(ds []discovery.FontDiagnostic) []wireDiagnostic {
	if ds == nil {
		return nil
	}
	out := make([]wireDiagnostic, len(ds))
	for n, d := range ds {
		out[n] = wireDiagnostic{d.Stage, d.Provider, d.Message, d.ResourceID, d.SHA256, requestWire(d.Request), d.Substituted}
	}
	return out
}
func diagnosticsNative(ds []wireDiagnostic) []discovery.FontDiagnostic {
	if ds == nil {
		return nil
	}
	out := make([]discovery.FontDiagnostic, len(ds))
	for n, d := range ds {
		out[n] = discovery.FontDiagnostic{Stage: d.Stage, Provider: d.Provider, Message: d.Message, ResourceID: d.ResourceID, SHA256: d.SHA256, Request: d.Request.native(), Substituted: d.Substituted}
	}
	return out
}
