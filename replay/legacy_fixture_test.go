package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// These static manifests were captured before step 5. Do not regenerate their
// evidence from the implementation under test; fonts are supplied separately.
func TestPreStep5VersionOneFixtures(t *testing.T) {
	store := t.TempDir()
	for _, name := range []string{"GentiumPlus-R.ttf", "NotoSansHebrew-Regular.ttf"} {
		data, err := os.ReadFile(filepath.Join("../../opentype/testdata/fonts", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = put(store, digest(data), data); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"firstfit", "knuth-plass"} {
		t.Run(name, func(t *testing.T) {
			result, err := Load(context.Background(), filepath.Join("testdata", "pre-step5-"+name+".json"), store)
			if err != nil {
				t.Fatal(err)
			}
			if result.Input.Fonts.DescriptorInput.Default.Families[0] != "serif" || result.Input.Fonts.DiscoveryPolicy.Providers[0] != "fixture" {
				t.Fatal("legacy policy lost")
			}
			if result.Identity == nil || result.Renderer == nil {
				t.Fatal("replay did not wire retained services")
			}
		})
	}
}
