package pipeline

import (
	"fmt"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/typography"
	typestyle "github.com/npillmayer/typography/style"
	"golang.org/x/text/language"
)

// M projects document values before they enter styled.Text. The synthetic face
// has no family/weight variants; size, language and spacing affect this bridge.
type syntheticStyleResolver struct{}

func (syntheticStyleResolver) ResolveStyle(value styled.Style, defaults contracts.ResolvedRunStyle) (contracts.ResolvedRunStyle, error) {
	result, err := (helpers.DefaultStyleResolver{}).ResolveStyle(nil, defaults)
	if err != nil {
		return result, err
	}
	if value == nil {
		return result, nil
	}
	var css typestyle.Typography
	switch v := value.(type) {
	case typestyle.Style: // Used by M while resolving the input projection.
		css = v.Typography()
	case typography.TypographyStyle:
		css = v.Typography
	default:
		return contracts.ResolvedRunStyle{}, fmt.Errorf("unknown paragraph style %T", value)
	}
	result.EmSize = css.EmSize
	result.Language, err = language.Parse(css.Language)
	if err != nil {
		return contracts.ResolvedRunStyle{}, err
	}
	result.Script, _ = result.Language.Script()
	if css.WhiteSpace == "pre-wrap" {
		result.Spacing = contracts.SpacingFixed
	}
	return result, nil
}
func syntheticConfig() khipu.Config {
	return khipu.Config{Streaming: &khipu.StreamingConfig{
		NewShaper: func() (contracts.StreamingShaper, error) { return helpers.NewSyntheticStreamingShaper(0) },
		Resolver:  syntheticStyleResolver{},
		Defaults:  contracts.ResolvedRunStyle{Font: helpers.NewSyntheticFont(), EmSize: 10 * dimen.PT, Spacing: contracts.SpacingElastic},
	}}
}
