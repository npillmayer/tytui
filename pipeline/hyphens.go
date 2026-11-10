package pipeline

import "github.com/npillmayer/khipu"

// Only author-supplied soft hyphens enable discretionary replacements. No
// language dictionary or automatic word splitting is inferred.
type explicitHyphens struct{}

func (explicitHyphens) Discretionaries(word string, _ *khipu.Params) ([]khipu.HyphenationFragment, error) {
	if word == "\u00ad" {
		return []khipu.HyphenationFragment{{PreBreak: "-"}}, nil
	}
	return nil, nil
}
