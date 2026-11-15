package fontselection

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/npillmayer/khipu/dimen"
	ot "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/fontresource"
)

// FontRequest preserves ordered CSS families and exact supported CSS style/weight.
// Generic families require an explicit session mapping. Zero weight means 400.
type FontRequest struct {
	Families          []string
	Weight            int
	Slant             string
	AllowSubstitution bool
}

// FontDiagnostic is detached evidence of a lookup, substitution, read, or parse.
// Coverage selection remains independently recorded in pipeline FontBindings.
type FontDiagnostic struct {
	Stage, Provider, Message, ResourceID, SHA256 string
	Request                                      fontresource.Request
	Substituted                                  bool
}

// ResolutionError identifies the request, lookup, read or parse stage and
// preserves its underlying cause for errors.Is and errors.As.
type ResolutionError struct {
	Stage string
	Err   error
}

// Error formats the stage and underlying failure.
func (e *ResolutionError) Error() string { return e.Stage + ": " + e.Err.Error() }

// Unwrap returns the underlying failure.
func (e *ResolutionError) Unwrap() error { return e.Err }

type cachedResolution struct {
	resource    *fontresource.Resource
	diagnostics []FontDiagnostic
}

// Discovery owns a session cache and fixed provider policy. Create a new session
// when policy changes. Providers must be offline, concurrency-safe, and honor ctx.
// Calls are serialized; published resources and diagnostic values are immutable.
type Discovery struct {
	gate      chan struct{}
	search    *providerSearch
	loader    loadEntry
	providers []string
	generics  map[string]string
	cache     map[string]cachedResolution
}

// New creates a session with controlled offline providers, normal OpenType
// validation and copied generic
// family mappings. Keys are case-insensitive; empty mapped names are rejected.
// Provider names are snapshotted in order. Create a new session for policy changes;
// keep provider closures stable and usable for the session lifetime.
func New(providers []OfflineProvider, generics map[string]string) (*Discovery, error) {
	search, err := newProviderSearch(providers)
	if err != nil {
		return nil, err
	}
	loader := loadEntry((ot.Services{}).Load)

	d := &Discovery{gate: make(chan struct{}, 1), search: search, loader: loader, providers: append([]string(nil), search.Names()...), generics: map[string]string{}, cache: map[string]cachedResolution{}}
	for k, v := range generics {
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("empty generic mapping")
		}
		d.generics[strings.ToLower(k)] = v
	}
	return d, nil
}

// Descriptors converts ordered, trimmed CSS families into neutral requests.
// Weight must be 100..900 in steps of 100 (zero means 400); supported slants are
// normal, italic and oblique (empty means normal). It converts weights to legacy
// identity codes without resolving generic families or accessing providers.
func Descriptors(request FontRequest) ([]fontresource.Request, error) {
	weight := request.Weight
	if weight == 0 {
		weight = 400
	}
	if weight < 100 || weight > 900 || weight%100 != 0 {
		return nil, fmt.Errorf("unsupported CSS weight %d: use 100..900 in steps of 100", weight)
	}
	slant := fontresource.Normal
	switch request.Slant {
	case "", "normal":
	case "italic":
		slant = fontresource.Italic
	case "oblique":
		slant = fontresource.Oblique
	default:
		return nil, fmt.Errorf("unsupported font style %q", request.Slant)
	}
	if len(request.Families) == 0 {
		return nil, fmt.Errorf("font request needs ordered family names")
	}
	result := make([]fontresource.Request, len(request.Families))
	for i, family := range request.Families {
		family = strings.TrimSpace(family)
		if family == "" {
			return nil, fmt.Errorf("empty family name")
		}
		result[i] = fontresource.Request{Pattern: family, Style: slant, Weight: fontresource.Weight(weight/100 - 4)}
	}
	return result, nil
}
func genericFamily(name string) bool {
	switch strings.ToLower(name) {
	case "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui", "ui-serif", "ui-sans-serif", "ui-monospace", "ui-rounded", "emoji", "math", "fangsong":
		return true
	}
	return false
}

// Resolve returns diagnostics even on failure. Only successful read/parse results
// are cached. Strict matching is the default; opt-in substitution is always reported.
func (d *Discovery) Resolve(ctx context.Context, request FontRequest) (*fontresource.Resource, []FontDiagnostic, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("nil context")
	}
	descs, err := Descriptors(request)
	if err != nil {
		return nil, nil, &ResolutionError{"request", err}
	}
	select {
	case d.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	defer func() { <-d.gate }()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	key := fmt.Sprintf("%q/%d/%s/%t", request.Families, request.Weight, request.Slant, request.AllowSubstitution)
	if cached, ok := d.cache[key]; ok {
		return cached.resource, append([]FontDiagnostic(nil), cached.diagnostics...), nil
	}
	var diagnostics []FontDiagnostic
	for _, original := range descs {
		desc := original
		if genericFamily(desc.Pattern) {
			mapped, ok := d.generics[strings.ToLower(desc.Pattern)]
			if !ok {
				diagnostics = append(diagnostics, FontDiagnostic{Stage: "lookup", Request: original, Message: "generic family has no explicit mapping"})
				continue
			}
			desc.Pattern = mapped
		}
		found, err := d.search.Search(ctx, desc, original, request.AllowSubstitution)
		diagnostics = append(diagnostics, found.Diagnostics...)
		if ctx.Err() != nil {
			return nil, diagnostics, ctx.Err()
		}
		if err != nil {
			var stage *ResolutionError
			if errors.As(err, &stage) && stage.Stage == "read" {
				return nil, diagnostics, err
			}
			continue
		}
		resource, selectedProvider, substituted := found.Resource, found.Provider, found.Substituted

		if err := ctx.Err(); err != nil {
			return nil, diagnostics, err
		}
		entry, err := d.loader(ctx, "", resource)
		if err == nil && (resource == nil || entry.Face == nil || entry.Resource == nil || entry.Info.ResourceID != resource.ID() || entry.Info.SHA256 != resource.SHA256() || entry.Resource.ID() != resource.ID()) {
			err = fmt.Errorf("loader identity does not match discovered resource")
		}
		if err != nil {
			diagnostics = append(diagnostics, FontDiagnostic{Stage: "parse", Request: original, Provider: selectedProvider, Message: err.Error()})
			return nil, diagnostics, &ResolutionError{"parse", err}
		}
		if err := ctx.Err(); err != nil {
			return nil, diagnostics, err
		}
		info := entry.Info
		diagnostics = append(diagnostics, FontDiagnostic{Stage: "resolved", Request: original, Provider: selectedProvider, ResourceID: info.ResourceID, SHA256: info.SHA256, Message: info.Family + " / " + info.Subfamily, Substituted: substituted})
		for _, warning := range info.Warnings {
			diagnostics = append(diagnostics, FontDiagnostic{Stage: "parse-warning", Request: original, Provider: selectedProvider, Message: warning, ResourceID: info.ResourceID, SHA256: info.SHA256})
		}
		d.cache[key] = cachedResolution{resource, append([]FontDiagnostic(nil), diagnostics...)}
		return resource, diagnostics, nil
	}
	err = fmt.Errorf("no controlled provider satisfied families %q", request.Families)
	return nil, diagnostics, &ResolutionError{"lookup", err}
}

// DescriptorRule pairs style-match criteria with an ordered discovery request.
// Family, Weight and Slant become SelectedRule criteria after resolution.
type DescriptorRule struct {
	Family  string
	Weight  int
	Slant   string
	Request FontRequest
}

// DescriptorSettings supplies discovery requests for the default, ordered rules
// and fallbacks, plus the resolved paragraph font size in DU.
type DescriptorSettings struct {
	Default   FontRequest
	Rules     []DescriptorRule
	Fallbacks []FontRequest
	Size      dimen.DU
}

// ResolveSettings resolves every candidate before itemization. No parser failure
// silently selects another candidate. Coverage fallback remains the itemizer's job.
func (d *Discovery) ResolveSettings(ctx context.Context, input DescriptorSettings) (*Request, []FontDiagnostic, error) {
	if input.Size <= 0 || input.Size >= dimen.Fil {
		return nil, nil, &ResolutionError{"request", fmt.Errorf("font size must be positive and finite")}
	}
	policy := &DiscoveryPolicy{Generics: map[string]string{}}
	for _, p := range d.providers {
		policy.Providers = append(policy.Providers, p)
	}
	for k, v := range d.generics {
		policy.Generics[k] = v
	}
	s := &Request{Settings: Settings{Size: input.Size}, DiscoveryPolicy: policy, DescriptorInput: input.Clone()}
	var diagnostics []FontDiagnostic
	resolve := func(request FontRequest) (*fontresource.Resource, error) {
		r, ds, err := d.Resolve(ctx, request)
		diagnostics = append(diagnostics, ds...)
		return r, err
	}
	var err error
	s.DefaultResource, err = resolve(input.Default)
	if err != nil {
		return nil, diagnostics, err
	}
	for _, rule := range input.Rules {
		r, err := resolve(rule.Request)
		if err != nil {
			return nil, diagnostics, err
		}
		s.Rules = append(s.Rules, Rule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, Resource: r})
	}
	for _, request := range input.Fallbacks {
		r, err := resolve(request)
		if err != nil {
			return nil, diagnostics, err
		}
		s.FallbackResources = append(s.FallbackResources, r)
	}
	s.ResolutionDiagnostics = append([]FontDiagnostic(nil), diagnostics...)
	return s, diagnostics, nil
}

// Clone detaches the original descriptor requests retained for later replay.
func (s *DescriptorSettings) Clone() *DescriptorSettings {
	if s == nil {
		return nil
	}
	c := *s
	cloneRequest := func(r FontRequest) FontRequest { r.Families = append([]string(nil), r.Families...); return r }
	c.Default = cloneRequest(s.Default)
	c.Rules = append([]DescriptorRule(nil), s.Rules...)
	for i := range c.Rules {
		c.Rules[i].Request = cloneRequest(c.Rules[i].Request)
	}
	c.Fallbacks = append([]FontRequest(nil), s.Fallbacks...)
	for i := range c.Fallbacks {
		c.Fallbacks[i] = cloneRequest(c.Fallbacks[i])
	}
	return &c
}

// DiscoveryPolicy records semantic configuration only, never provider closures,
// credentials, or host configuration. Replay uses pinned resources instead.
type DiscoveryPolicy struct {
	Providers []string
	Generics  map[string]string
}

// Clone detaches provider names and generic mappings. A nil receiver returns nil.
func (p *DiscoveryPolicy) Clone() *DiscoveryPolicy {
	if p == nil {
		return nil
	}
	c := &DiscoveryPolicy{Providers: append([]string(nil), p.Providers...), Generics: map[string]string{}}
	for k, v := range p.Generics {
		c.Generics[k] = v
	}
	return c
}

// searchResult carries the selected immutable resource and ordered diagnostic
// evidence, including whether the provider explicitly substituted a match.
type searchResult struct {
	Resource    *fontresource.Resource
	Provider    string
	Substituted bool
	Diagnostics []FontDiagnostic
}
