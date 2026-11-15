package fontselection

import (
	"bytes"
	"context"
	"fmt"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/fontfind/fontregistry"
	"github.com/npillmayer/fontfind/locate"
	"github.com/npillmayer/typography/fontresource"
	"golang.org/x/image/font"
)

// Match makes trust explicit: controlled providers must attest an exact match.
// A nil error alone is not evidence of matching. A font plus error is a substitution.
type Match struct {
	Font   fontfind.ScalableFont
	Exact  bool
	Reason string
}

// OfflineProvider supplies a named, trusted offline lookup. Find must honor
// cancellation and be concurrency-safe. It must attest exact matches through Match;
// a nil error alone is insufficient. Names must be nonempty and unique.
type OfflineProvider struct {
	Name string
	Find func(context.Context, fontfind.Descriptor) (Match, error)
}

// noFallbackRegistry prevents fontfind's implicit packaged fallback from becoming
// an accidental success. Discovery caches complete immutable resolution evidence.
type noFallbackRegistry struct{ *fontregistry.Registry }

// FallbackFont rejects implicit packaged fallback at the provider boundary.
func (noFallbackRegistry) FallbackFont() (fontfind.ScalableFont, error) {
	return fontfind.NullFont, fmt.Errorf("implicit fallback disabled")
}

// providerSearch executes an ordered, fixed list of controlled providers.
// Each search owns a private registry with implicit fallback disabled.
type providerSearch struct{ providers []OfflineProvider }

// newProviderSearch copies the list, rejecting empty/duplicate names and nil Find
// functions. Provider closures remain shared; no loader or default provider is added.
func newProviderSearch(providers []OfflineProvider) (*providerSearch, error) {
	seen := map[string]bool{}
	for _, p := range providers {
		if p.Name == "" || p.Find == nil || seen[p.Name] {
			return nil, fmt.Errorf("providers need unique names and nonnil functions")
		}
		seen[p.Name] = true
	}
	return &providerSearch{providers: append([]OfflineProvider(nil), providers...)}, nil
}

// Names returns a detached list in provider search order.
func (s *providerSearch) Names() []string {
	var out []string
	for _, p := range s.providers {
		out = append(out, p.Name)
	}
	return out
}

// Descriptor converts neutral legacy request codes to provider enum values.
// It does not validate codes or convert numeric CSS weights.
func Descriptor(r fontresource.Request) fontfind.Descriptor {
	return fontfind.Descriptor{Pattern: r.Pattern, Style: font.Style(r.Style), Weight: font.Weight(r.Weight)}
}

// ResourceRequest converts provider enums to neutral legacy identity codes.
// It does not validate the descriptor or produce numeric CSS weights.
func ResourceRequest(d fontfind.Descriptor) fontresource.Request {
	return fontresource.Request{Pattern: d.Pattern, Style: fontresource.Slant(d.Style), Weight: fontresource.Weight(d.Weight)}
}

// ReadResource snapshots provider bytes. ReadFontData allocates before the bound
// check, so this adapter is restricted to trusted offline resources.
func ReadResource(sf fontfind.ScalableFont, provider string, request fontresource.Request) (*fontresource.Resource, error) {
	data, err := sf.ReadFontData()
	if err != nil {
		return nil, fmt.Errorf("font resource %q: %w", sf.Name, err)
	}
	return fontresource.Read(bytes.NewReader(data), fontresource.Origin{Provider: provider, Name: sf.Name, Location: sf.Path(), Request: request})
}

// Search tries providers in order using mapped for lookup and original for
// diagnostics/provenance. Unattested or error-bearing matches require allow=true.
// Returned evidence belongs to this call and resource bytes are snapshotted; read
// failures are terminal. Context cancellation is preserved. No implicit fallback
// or session cache is installed here; discovery owns session policy and caching.
func (s *providerSearch) Search(ctx context.Context, mapped, original fontresource.Request, allow bool) (searchResult, error) {
	if ctx == nil {
		return searchResult{}, fmt.Errorf("nil context")
	}
	selectedProvider := ""
	var diagnostics []FontDiagnostic
	var resource *fontresource.Resource
	substituted := false
	result := func() searchResult {
		return searchResult{Resource: resource, Provider: selectedProvider, Substituted: substituted, Diagnostics: diagnostics}
	}
	providers := make([]locate.FontLocatorWithContext, 0, len(s.providers))
	for _, provider := range s.providers {
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
			accepted := match.Font.Name != "" && (!substitute || allow)
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
	sf, err := locate.NewResolverPipeline(registry, providers...).Resolve(ctx, Descriptor(mapped)).Font()
	if ctx.Err() != nil {
		return result(), ctx.Err()
	}
	if err != nil {
		return result(), err
	}
	resource, err = ReadResource(sf, selectedProvider, original)
	if err != nil {
		diagnostics = append(diagnostics, FontDiagnostic{Stage: "read", Request: original, Provider: selectedProvider, Message: err.Error()})
		return result(), &ResolutionError{Stage: "read", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return result(), err
	}
	return result(), nil
}
