package fontbackend

import (
	"bytes"
	"context"
	"errors"
	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/khipu/dimen"
	"io"
	"os"
	"strings"
	"testing"
)

type failingResourceReader struct{}

func (failingResourceReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type zeroResourceReader struct{}

func (zeroResourceReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestResourceErrorsAndOwnership(t *testing.T) {
	for _, r := range []io.Reader{nil, failingResourceReader{}, io.LimitReader(zeroResourceReader{}, maxFontBytes+1)} {
		if _, err := ReadResource(r, Origin{}); err == nil {
			t.Fatal("accepted invalid resource")
		}
	}
	if _, err := FontfindResource(fontfind.NullFont, "test", fontfind.Descriptor{}); err == nil {
		t.Fatal("accepted missing resource")
	}
	invalid, err := ReadResource(strings.NewReader("not a font"), Origin{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResource(invalid); err == nil {
		t.Fatal("suppressed parser error")
	}
	data, err := os.ReadFile("../../opentype/testdata/fonts/GentiumPlus-R.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ReadResource(bytes.NewReader(data), Origin{Provider: "test", Location: "same"})
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	f, err := LoadResource(r)
	if err != nil {
		t.Fatal("caller mutated snapshot:", err)
	}
	if f.Info().ResourceID == "" || f.Info().SHA256 == "" {
		t.Fatal("missing identities")
	}
	for _, s := range []Settings{{DefaultPath: "../../opentype/testdata/fonts/GentiumPlus-R.ttf", DefaultResource: r, Size: 12 * dimen.BP}, {Size: 12 * dimen.BP}} {
		if _, err := LoadSettings(context.Background(), s); err == nil {
			t.Fatal("accepted ambiguous/absent selection")
		}
	}
	settings := Settings{DefaultResource: r, FallbackResources: []*Resource{r}, Rules: []Rule{{Resource: r}}}
	cloned := settings.Clone()
	settings.FallbackResources[0] = nil
	settings.Rules[0].Resource = nil
	if cloned.FallbackResources[0] == nil || cloned.Rules[0].Resource == nil {
		t.Fatal("clone aliases settings slices")
	}
}
