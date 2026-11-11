package fontbackend

import (
	"context"
	"fmt"
	"strings"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/fontfind/fontregistry"
	"github.com/npillmayer/fontfind/locate"
	"github.com/npillmayer/khipu/dimen"
	"golang.org/x/image/font"
)

// FontRequest preserves ordered CSS families and exact supported CSS style/weight.
// Generic families require an explicit session mapping. Zero weight means 400.
type FontRequest struct {
	Families          []string
	Weight            int
	Slant             string
	AllowSubstitution bool
}

// Match makes trust explicit: controlled providers must attest an exact match.
// A nil error alone is not evidence of matching. A font plus error is a substitution.
type Match struct {
	Font   fontfind.ScalableFont
	Exact  bool
	Reason string
}
type OfflineProvider struct {
	Name string
	Find func(context.Context, fontfind.Descriptor) (Match, error)
}

// FontDiagnostic is detached evidence of a lookup, substitution, read, or parse.
// Coverage selection remains independently recorded in pipeline FontBindings.
type FontDiagnostic struct {
	Stage, Provider, Message, ResourceID, SHA256 string
	Request                                      fontfind.Descriptor
	Substituted                                  bool
}
type ResolutionError struct {
	Stage string
	Err   error
}

func (e *ResolutionError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *ResolutionError) Unwrap() error { return e.Err }

type cachedResolution struct {
	resource    *Resource
	diagnostics []FontDiagnostic
}

// Discovery owns a session cache and fixed provider policy. Create a new session
// when policy changes. Providers must be offline, concurrency-safe, and honor ctx.
// Calls are serialized; published resources and diagnostic values are immutable.
type Discovery struct {
	gate      chan struct{}
	providers []OfflineProvider
	generics  map[string]string
	cache     map[string]cachedResolution
}

func NewDiscovery(providers []OfflineProvider, generics map[string]string) (*Discovery, error) {
	d := &Discovery{gate: make(chan struct{}, 1), providers: append([]OfflineProvider(nil), providers...), generics: map[string]string{}, cache: map[string]cachedResolution{}}
	seen := map[string]bool{}
	for _, p := range providers {
		if p.Name == "" || p.Find == nil || seen[p.Name] {
			return nil, fmt.Errorf("providers need unique names and nonnil functions")
		}
		seen[p.Name] = true
	}
	for k, v := range generics {
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("empty generic mapping")
		}
		d.generics[strings.ToLower(k)] = v
	}
	return d, nil
}

func descriptors(request FontRequest) ([]fontfind.Descriptor, error) {
	weight := request.Weight
	if weight == 0 {
		weight = 400
	}
	if weight < 100 || weight > 900 || weight%100 != 0 {
		return nil, fmt.Errorf("unsupported CSS weight %d: use 100..900 in steps of 100", weight)
	}
	slant := font.StyleNormal
	switch request.Slant {
	case "", "normal":
	case "italic":
		slant = font.StyleItalic
	case "oblique":
		slant = font.StyleOblique
	default:
		return nil, fmt.Errorf("unsupported font style %q", request.Slant)
	}
	if len(request.Families) == 0 {
		return nil, fmt.Errorf("font request needs ordered family names")
	}
	result := make([]fontfind.Descriptor, len(request.Families))
	for i, family := range request.Families {
		family = strings.TrimSpace(family)
		if family == "" {
			return nil, fmt.Errorf("empty family name")
		}
		result[i] = fontfind.Descriptor{Pattern: family, Style: slant, Weight: font.Weight(weight/100 - 4)}
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

// noFallbackRegistry prevents fontfind's implicit packaged fallback from becoming
// an accidental success. Discovery caches complete immutable resolution evidence.
type noFallbackRegistry struct{ *fontregistry.Registry }

func (noFallbackRegistry) FallbackFont() (fontfind.ScalableFont, error) {
	return fontfind.NullFont, fmt.Errorf("implicit fallback disabled")
}

// Resolve returns diagnostics even on failure. Only successful read/parse results
// are cached. Strict matching is the default; opt-in substitution is always reported.
func (d *Discovery) Resolve(ctx context.Context, request FontRequest) (*Resource, []FontDiagnostic, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("nil context")
	}
	descs, err := descriptors(request)
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
		selectedProvider := ""
		substituted := false
		providers := make([]locate.FontLocatorWithContext, 0, len(d.providers))
		for _, provider := range d.providers {
			providers = append(providers, func(ctx context.Context, desc fontfind.Descriptor) (fontfind.ScalableFont, error) {
				match, err := provider.Find(ctx, desc)
				if ctx.Err() != nil {
					return fontfind.NullFont, ctx.Err()
				}
				substitute := !match.Exact || err != nil
				message := match.Reason
				if err != nil {
					if message == "" {
						message = err.Error()
					} else {
						message = fmt.Sprintf("%s: %v", message, err)
					}
				}
				if message == "" {
					if substitute {
						message = "provider did not attest an exact match"
					} else {
						message = "exact match"
					}
				}
				accepted := match.Font.Name != "" && (!substitute || request.AllowSubstitution)
				diagnostics = append(diagnostics, FontDiagnostic{Stage: "lookup", Provider: provider.Name, Request: original, Message: message, Substituted: accepted && substitute})
				if !accepted {
					return fontfind.NullFont, fmt.Errorf("request not satisfied: %s", message)
				}
				selectedProvider = provider.Name
				substituted = substitute
				return match.Font, nil
			})
		}
		// Each search has an explicit private registry. The session-level cache above
		// also retains provider attempts and substitutions, which fontfind cannot store.
		registry := noFallbackRegistry{fontregistry.New()}
		sf, err := locate.NewResolverPipeline(registry, providers...).Resolve(ctx, desc).Font()
		if ctx.Err() != nil {
			return nil, append([]FontDiagnostic(nil), diagnostics...), ctx.Err()
		}
		if err != nil {
			continue
		}
		resource, err := FontfindResource(sf, selectedProvider, original)
		if err != nil {
			diagnostics = append(diagnostics, FontDiagnostic{Stage: "read", Request: original, Provider: selectedProvider, Message: err.Error()})
			return nil, diagnostics, &ResolutionError{"read", err}
		}
		if err := ctx.Err(); err != nil {
			return nil, diagnostics, err
		}
		face, err := LoadResource(resource)
		if err != nil {
			diagnostics = append(diagnostics, FontDiagnostic{Stage: "parse", Request: original, Provider: selectedProvider, Message: err.Error()})
			return nil, diagnostics, &ResolutionError{"parse", err}
		}
		if err := ctx.Err(); err != nil {
			return nil, diagnostics, err
		}
		info := face.Info()
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

type DescriptorRule struct {
	Family  string
	Weight  int
	Slant   string
	Request FontRequest
}
type DescriptorSettings struct {
	Default   FontRequest
	Rules     []DescriptorRule
	Fallbacks []FontRequest
	Size      dimen.DU
}

// ResolveSettings resolves every candidate before itemization. No parser failure
// silently selects another candidate. Coverage fallback remains the itemizer's job.
func (d *Discovery) ResolveSettings(ctx context.Context, input DescriptorSettings) (*Settings, []FontDiagnostic, error) {
	if input.Size <= 0 || input.Size >= dimen.Fil {
		return nil, nil, &ResolutionError{"request", fmt.Errorf("font size must be positive and finite")}
	}
	policy := &DiscoveryPolicy{Generics: map[string]string{}}
	for _, p := range d.providers {
		policy.Providers = append(policy.Providers, p.Name)
	}
	for k, v := range d.generics {
		policy.Generics[k] = v
	}
	s := &Settings{DiscoveryPolicy: policy, Size: input.Size, DescriptorInput: input.Clone()}
	var diagnostics []FontDiagnostic
	resolve := func(request FontRequest) (*Resource, error) {
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
