package documentlayout

import (
	"context"

	"github.com/npillmayer/typography"
	"github.com/npillmayer/typography/fontresource"

	"tytui/fontselection"
)

type testServices struct {
	Loader interface {
		Load(context.Context, string, *fontresource.Resource) (typography.FaceEntry, error)
	}
	Coverage typography.Coverage
}

// loadResolver builds selected fixture faces before entering the typography engine.
func loadResolver(ctx context.Context, s fontselection.Settings, svc testServices) (*typography.Resolver, error) {
	fonts := typography.SelectedFonts{Size: s.Size}
	seen := map[string]bool{}
	load := func(path string, r *fontresource.Resource) (string, error) {
		e, err := svc.Loader.Load(ctx, path, r)
		if err != nil {
			return "", err
		}
		id := e.Info.ResourceID
		if !seen[id] {
			seen[id] = true
			fonts.Faces = append(fonts.Faces, e)
		}
		return id, nil
	}
	var err error
	fonts.Default, err = load(s.DefaultPath, s.DefaultResource)
	if err != nil {
		return nil, err
	}
	for _, rule := range s.Rules {
		id, err := load(rule.Path, rule.Resource)
		if err != nil {
			return nil, err
		}
		fonts.Rules = append(fonts.Rules, typography.SelectedRule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, ResourceID: id})
	}
	for _, p := range s.Fallbacks {
		id, err := load(p, nil)
		if err != nil {
			return nil, err
		}
		fonts.Fallbacks = append(fonts.Fallbacks, id)
	}
	for _, r := range s.FallbackResources {
		id, err := load("", r)
		if err != nil {
			return nil, err
		}
		fonts.Fallbacks = append(fonts.Fallbacks, id)
	}
	return typography.NewResolver(ctx, fonts, svc.Coverage)
}
