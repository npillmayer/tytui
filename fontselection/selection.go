// Package fontselection selects and loads explicit fonts for tytui. Provider
// policy, session caches and request provenance stay outside the typography engine.
package fontselection

import (
	"context"
	"fmt"
	"strings"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"
	ot "github.com/npillmayer/typography/glyphing/adapter/opentype"
)

// Settings is a detached request. Rules use first-match order; zero/empty fields
// are wildcards. Select exactly one default file or immutable resource.
// Unmatched styles use that default, never synthetic. Path fallbacks precede resources.
type Settings struct {
	DefaultPath       string
	DefaultResource   *fontresource.Resource
	FallbackResources []*fontresource.Resource
	Size              dimen.DU
	Rules             []Rule
	Fallbacks         []string
}

// Rule selects a font for the first matching family, CSS weight and slant.
// Empty family/slant and zero weight are wildcards. Select exactly one Path or
// Resource; family matching is case-insensitive against the style family list.
type Rule struct {
	Family      string
	Weight      int
	Slant, Path string
	Resource    *fontresource.Resource
}

// Clone copies the settings and their slices, sharing immutable resources.
// A nil receiver returns nil.
func (s *Settings) Clone() *Settings {
	if s == nil {
		return nil
	}
	c := *s
	c.FallbackResources = append([]*fontresource.Resource(nil), s.FallbackResources...)
	c.Fallbacks = append([]string(nil), s.Fallbacks...)
	c.Rules = append([]Rule(nil), s.Rules...)
	return &c
}

// Request retains editable file/resource selections and optional discovery evidence.
// Paths are loaded only by Prepare, before the typography engine is called.
type Request struct {
	Settings
	DiscoveryPolicy       *DiscoveryPolicy
	DescriptorInput       *DescriptorSettings
	ResolutionDiagnostics []FontDiagnostic
}

// Clone detaches request/evidence slices and shares immutable resources.
func (r *Request) Clone() *Request {
	if r == nil {
		return nil
	}
	return &Request{Settings: *r.Settings.Clone(), DiscoveryPolicy: r.DiscoveryPolicy.Clone(), DescriptorInput: r.DescriptorInput.Clone(), ResolutionDiagnostics: append([]FontDiagnostic(nil), r.ResolutionDiagnostics...)}
}

// Provenance is display/replay evidence, never an executable font request.
type Provenance struct {
	RequestedPath          string
	RequestedRulePaths     []string
	RequestedFallbackPaths []string
	DiscoveryPolicy        *DiscoveryPolicy
	DescriptorInput        *DescriptorSettings
	ResolutionDiagnostics  []FontDiagnostic
}

// Clone detaches all provenance records and slices.
func (p Provenance) Clone() Provenance {
	p.RequestedRulePaths = append([]string(nil), p.RequestedRulePaths...)
	p.RequestedFallbackPaths = append([]string(nil), p.RequestedFallbackPaths...)
	p.DiscoveryPolicy = p.DiscoveryPolicy.Clone()
	p.DescriptorInput = p.DescriptorInput.Clone()
	p.ResolutionDiagnostics = append([]FontDiagnostic(nil), p.ResolutionDiagnostics...)
	return p
}

// Selection contains already loaded fonts and separate application evidence.
// Faces/resources remain shared and stable for retained results. Never serialize it.
type Selection struct {
	Fonts      glyphing.SelectedFonts
	Provenance Provenance
}

// Clone detaches metadata, sharing immutable resources and retained faces.
func (s *Selection) Clone() *Selection {
	if s == nil {
		return nil
	}
	return &Selection{Fonts: *s.Fonts.Clone(), Provenance: s.Provenance.Clone()}
}

// Prepare loads a closed set of explicitly selected files/resources using normal
// OpenType parsing and coverage acceptance checks. It performs no glyph queries,
// shaping or discovery. Nil selects the synthetic pipeline.
func Prepare(ctx context.Context, request *Request) (*Selection, error) {
	return prepare(ctx, request, (ot.Services{}).Load, ot.Services{})
}

type loadEntry func(context.Context, string, *fontresource.Resource) (glyphing.FaceEntry, error)

func prepare(ctx context.Context, request *Request, loadEntry loadEntry, coverage glyphing.Coverage) (*Selection, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, nil
	}
	request = request.Clone()
	s := request.Settings
	if s.Size <= 0 || s.Size >= dimen.Fil {
		return nil, fmt.Errorf("font size must be positive and finite")
	}
	fonts := glyphing.SelectedFonts{Size: s.Size}
	seen := map[string]bool{}
	load := func(path string, resource *fontresource.Resource) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if (strings.TrimSpace(path) == "") == (resource == nil) {
			return "", fmt.Errorf("select exactly one font file or resource")
		}
		entry, err := loadEntry(ctx, path, resource)
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if entry.Face == nil || entry.Resource == nil {
			return "", fmt.Errorf("font loader returned incomplete entry")
		}
		if entry.Info.ResourceID != entry.Resource.ID() || entry.Info.SHA256 != entry.Resource.SHA256() || entry.Info.Origin != entry.Resource.Origin() || entry.Info.Path != entry.Info.Origin.Location {
			return "", fmt.Errorf("font loader identity does not match retained resource")
		}
		if resource != nil && resource.ID() != entry.Info.ResourceID {
			return "", fmt.Errorf("font loader substituted the selected resource")
		}
		id := entry.Info.ResourceID
		if !seen[id] {
			seen[id] = true
			fonts.Faces = append(fonts.Faces, entry)
		}
		return id, nil
	}
	var err error
	fonts.Default, err = load(s.DefaultPath, s.DefaultResource)
	if err != nil {
		return nil, err
	}
	provenance := Provenance{RequestedPath: s.DefaultPath, RequestedFallbackPaths: append([]string(nil), s.Fallbacks...), DiscoveryPolicy: request.DiscoveryPolicy, DescriptorInput: request.DescriptorInput, ResolutionDiagnostics: request.ResolutionDiagnostics}
	for i, rule := range s.Rules {
		if rule.Weight < 0 || rule.Weight > 1000 || (rule.Slant != "" && rule.Slant != "normal" && rule.Slant != "italic" && rule.Slant != "oblique") {
			return nil, fmt.Errorf("invalid font mapping row %d", i+1)
		}
		id, e := load(rule.Path, rule.Resource)
		if e != nil {
			return nil, fmt.Errorf("font mapping row %d: %w", i+1, e)
		}
		fonts.Rules = append(fonts.Rules, glyphing.SelectedRule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, ResourceID: id})
		provenance.RequestedRulePaths = append(provenance.RequestedRulePaths, rule.Path)
	}
	for _, path := range s.Fallbacks {
		id, e := load(path, nil)
		if e != nil {
			return nil, e
		}
		fonts.Fallbacks = append(fonts.Fallbacks, id)
	}
	for _, resource := range s.FallbackResources {
		id, e := load("", resource)
		if e != nil {
			return nil, e
		}
		fonts.Fallbacks = append(fonts.Fallbacks, id)
	}
	r, err := glyphing.NewStyleMapper(ctx, fonts, coverage)
	if err != nil {
		return nil, err
	}
	return &Selection{Fonts: *r.Snapshot(), Provenance: provenance.Clone()}, nil
}
