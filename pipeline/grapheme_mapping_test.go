package pipeline_test

import (
	"context"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
)

func TestGraphemeFallbackPolicy(t *testing.T) {
	settings := typography.Settings{DefaultPath: "../../opentype/testdata/fonts/ShaperTestProgress.ttf", Size: 12 * dimen.BP, Fallbacks: []string{"../../opentype/testdata/fonts/GentiumPlus-R.ttf"}}
	r, err := typography.LoadSettings(context.Background(), settings, typography.Services{Loader: otadapter.Services{}, Coverage: otadapter.Services{}})
	if err != nil {
		t.Fatal(err)
	}
	_, bindings, err := r.Itemize(context.Background(), "a\u0301a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 || bindings[0].Start != 0 || bindings[0].End != 3 || bindings[0].Path != settings.Fallbacks[0] || bindings[1].Path != "../../opentype/testdata/fonts/ShaperTestProgress.ttf" {
		t.Fatalf("grapheme split or wrong priority %+v", bindings)
	}
	settings.Fallbacks[0] = "missing.ttf"
	if _, _, err = r.Itemize(context.Background(), "a\u0301a", nil); err != nil {
		t.Fatal("resolver aliases settings", err)
	}
	strict, err := typography.LoadSettings(context.Background(), typography.Settings{DefaultPath: "../../opentype/testdata/fonts/ShaperTestProgress.ttf", Size: 12 * dimen.BP}, typography.Services{Loader: otadapter.Services{}, Coverage: otadapter.Services{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = strict.Itemize(context.Background(), "a\u0301", nil); err == nil {
		t.Fatal("implicit fallback")
	}
	if _, _, err = r.Itemize(context.Background(), "العربية", nil); err == nil {
		t.Fatal("unsupported script silently shaped")
	}
}
