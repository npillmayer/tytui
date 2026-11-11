package fontbackend

import (
	"context"
	"fmt"
	"strings"
	"tytui/htmlinput"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"golang.org/x/text/language"
)

// Settings is a detached request. Rules use first-match order; zero/empty fields
// are wildcards. Select exactly one default file or immutable resource.
// Unmatched styles use that default, never synthetic. Path fallbacks precede resources.
type Settings struct {
	DiscoveryPolicy       *DiscoveryPolicy
	DescriptorInput       *DescriptorSettings
	ResolutionDiagnostics []FontDiagnostic
	DefaultPath           string
	DefaultResource       *Resource
	FallbackResources     []*Resource
	Size                  dimen.DU
	Rules                 []Rule
	Fallbacks             []string
}
type Rule struct {
	Family      string
	Weight      int
	Slant, Path string
	Resource    *Resource
}

func (s *Settings) Clone() *Settings {
	if s == nil {
		return nil
	}
	c := *s
	c.DiscoveryPolicy = s.DiscoveryPolicy.Clone()
	c.DescriptorInput = s.DescriptorInput.Clone()
	c.ResolutionDiagnostics = append([]FontDiagnostic(nil), s.ResolutionDiagnostics...)
	c.FallbackResources = append([]*Resource(nil), s.FallbackResources...)
	c.Fallbacks = append([]string(nil), s.Fallbacks...)
	c.Rules = append([]Rule(nil), s.Rules...)
	return &c
}

// Binding records the chosen face and exact processed-text byte range.
type Binding struct {
	Start, End   uint64
	Path, SHA256 string
	ResourceID   string
	Origin       Origin
	EmSize       dimen.DU
	Script       string
}
type Resolver struct {
	settings    Settings
	faces       map[string]*Face
	infos       []Info
	defaultID   string
	ruleIDs     []string
	fallbackIDs []string
}

func LoadSettings(ctx context.Context, s Settings) (*Resolver, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if s.Size <= 0 || s.Size >= dimen.Fil {
		return nil, fmt.Errorf("font size must be positive and finite")
	}
	r := &Resolver{settings: *s.Clone(), faces: make(map[string]*Face)}
	load := func(path string, resource *Resource) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if (strings.TrimSpace(path) == "") == (resource == nil) {
			return "", fmt.Errorf("select exactly one font file or resource")
		}
		var f *Face
		var err error
		if resource != nil {
			f, err = LoadResource(resource)
		} else {
			f, err = LoadFile(path)
		}
		if err != nil {
			return "", err
		}
		id := f.Info().ResourceID
		if r.faces[id] == nil {
			r.faces[id] = f
			r.infos = append(r.infos, f.Info())
		}
		return id, nil
	}
	var err error
	r.defaultID, err = load(s.DefaultPath, s.DefaultResource)
	if err != nil {
		return nil, err
	}
	for i, rule := range s.Rules {
		if rule.Weight < 0 || rule.Weight > 1000 || (rule.Slant != "" && rule.Slant != "normal" && rule.Slant != "italic" && rule.Slant != "oblique") {
			return nil, fmt.Errorf("invalid font mapping row %d", i+1)
		}
		id, err := load(rule.Path, rule.Resource)
		if err != nil {
			return nil, fmt.Errorf("font mapping row %d: %w", i+1, err)
		}
		r.ruleIDs = append(r.ruleIDs, id)
	}
	for _, path := range s.Fallbacks {
		id, err := load(path, nil)
		if err != nil {
			return nil, err
		}
		r.fallbackIDs = append(r.fallbackIDs, id)
	}
	for _, resource := range s.FallbackResources {
		id, err := load("", resource)
		if err != nil {
			return nil, err
		}
		r.fallbackIDs = append(r.fallbackIDs, id)
	}
	return r, ctx.Err()
}
func (r *Resolver) Infos() []Info {
	infos := make([]Info, len(r.infos))
	for i, v := range r.infos {
		infos[i] = v
		infos[i].Warnings = append([]string(nil), v.Warnings...)
	}
	return infos
}
func (r *Resolver) Defaults() contracts.ResolvedRunStyle {
	return contracts.ResolvedRunStyle{Font: r.faces[r.defaultID], EmSize: r.settings.Size, Language: language.English, Script: language.MustParseScript("Latn"), Spacing: contracts.SpacingElastic}
}
func familyMatches(list, want string) bool {
	for _, family := range strings.Split(list, ",") {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(family), "\"'"), want) {
			return true
		}
	}
	return false
}
func (r *Resolver) ResolveStyle(value styled.Style, defaults contracts.ResolvedRunStyle) (contracts.ResolvedRunStyle, error) {
	if item, ok := value.(runStyle); ok {
		var css styled.Style
		if !item.Plain {
			css = item.CSS
		}
		result, err := r.ResolveStyle(css, defaults)
		if err != nil {
			return result, err
		}
		result.Font = r.faces[item.ResourceID]
		result.Script = language.MustParseScript(item.Script)
		return result, nil
	}
	result := defaults
	result.Features = append([]contracts.ShapeFeature(nil), defaults.Features...)
	if value == nil {
		return result, nil
	}
	css, ok := value.(htmlinput.Style)
	if !ok {
		return result, fmt.Errorf("unknown style %T", value)
	}
	path := r.defaultID
	for i, rule := range r.settings.Rules {
		if (rule.Family == "" || familyMatches(css.Family, rule.Family)) && (rule.Weight == 0 || rule.Weight == css.Weight) && (rule.Slant == "" || rule.Slant == css.Slant) {
			path = r.ruleIDs[i]
			break
		}
	}
	result.Font = r.faces[path]
	result.EmSize = css.EmSize
	var err error
	result.Language, err = language.Parse(css.Language)
	if err != nil {
		return result, err
	}
	result.Script, _ = result.Language.Script()
	result.Spacing = contracts.SpacingElastic
	if css.WhiteSpace == "pre-wrap" {
		result.Spacing = contracts.SpacingFixed
	}
	return result, nil
}

// CheckText validates itemization and coverage without retaining the styled text.
func (r *Resolver) CheckText(ctx context.Context, text string, source *htmlinput.Extraction) ([]Binding, error) {
	_, bindings, err := r.Itemize(ctx, text, source)
	return bindings, err
}

// Snapshot pins every configured face, including unused rules/fallbacks. It never
// reopens original paths and preserves provenance for offline replay.
func (r *Resolver) Snapshot() *Settings {
	s := r.settings.Clone()
	resource := func(id string) *Resource { f := r.faces[id]; return &Resource{data: f.data, origin: f.info.Origin} }
	s.DefaultPath = ""
	s.DefaultResource = resource(r.defaultID)
	for i := range s.Rules {
		s.Rules[i].Path = ""
		s.Rules[i].Resource = resource(r.ruleIDs[i])
	}
	s.Fallbacks = nil
	s.FallbackResources = nil
	for _, id := range r.fallbackIDs {
		s.FallbackResources = append(s.FallbackResources, resource(id))
	}
	return s
}
