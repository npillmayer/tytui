package replay

import (
	"path/filepath"
	"strings"
	"testing"
)

// Preserve the original bytes as rejection cases, not runnable compatibility cases.
func TestHistoricalFixturesRejectedBeforeResources(t *testing.T) {
	for _, name := range []string{"pre-step5-firstfit.json", "pre-step5-knuth-plass.json", "pre-step6-paint.json", "current-fi-firstfit.json", "current-fi-knuth-plass.json"} {
		t.Run(name, func(t *testing.T) {
			result, err := Load(t.Context(), filepath.Join("testdata", name), filepath.Join(t.TempDir(), "missing-store"))
			if result != nil || err == nil || !strings.Contains(err.Error(), "unsupported replay version ") {
				t.Fatalf("historical input was not rejected by version: %v", err)
			}
		})
	}
}
