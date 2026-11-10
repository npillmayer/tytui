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
// are wildcards. Unmatched styles deliberately use DefaultPath, never synthetic.
type Settings struct {
	DefaultPath string
	Size        dimen.DU
	Rules       []Rule
	Fallbacks   []string
}
type Rule struct {
	Family      string
	Weight      int
	Slant, Path string
}

func (s *Settings) Clone() *Settings {
	if s == nil {
		return nil
	}
	c := *s
	c.Fallbacks = append([]string(nil), s.Fallbacks...)
	c.Rules = append([]Rule(nil), s.Rules...)
	return &c
}

// Binding records the chosen face and exact processed-text byte range.
type Binding struct {
	Start, End   uint64
	Path, SHA256 string
	EmSize       dimen.DU
	Script       string
}
type Resolver struct {
	settings Settings
	faces    map[string]*Face
	infos    []Info
}

func LoadSettings(ctx context.Context, s Settings) (*Resolver, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if strings.TrimSpace(s.DefaultPath) == "" {
		return nil, fmt.Errorf("select a default font file")
	}
	if s.Size <= 0 || s.Size >= dimen.Fil {
		return nil, fmt.Errorf("font size must be positive and finite")
	}
	r := &Resolver{settings: *s.Clone(), faces: make(map[string]*Face)}
	paths := []string{s.DefaultPath}
	for i, rule := range s.Rules {
		if rule.Path == "" || rule.Weight < 0 || rule.Weight > 1000 || (rule.Slant != "" && rule.Slant != "normal" && rule.Slant != "italic" && rule.Slant != "oblique") {
			return nil, fmt.Errorf("invalid font mapping row %d", i+1)
		}
		paths = append(paths, rule.Path)
	}
	paths = append(paths, s.Fallbacks...)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if r.faces[path] != nil {
			continue
		}
		f, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		r.faces[path] = f
		r.infos = append(r.infos, f.Info())
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
	return contracts.ResolvedRunStyle{Font: r.faces[r.settings.DefaultPath], EmSize: r.settings.Size, Language: language.English, Script: language.MustParseScript("Latn"), Spacing: contracts.SpacingElastic}
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
		result.Font = r.faces[item.Path]
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
	path := r.settings.DefaultPath
	for _, rule := range r.settings.Rules {
		if (rule.Family == "" || familyMatches(css.Family, rule.Family)) && (rule.Weight == 0 || rule.Weight == css.Weight) && (rule.Slant == "" || rule.Slant == css.Slant) {
			path = rule.Path
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
