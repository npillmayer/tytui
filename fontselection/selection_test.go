package fontselection

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"
)

func fakeEntry(t *testing.T, path string, r *fontresource.Resource) glyphing.FaceEntry {
	t.Helper()
	if r == nil {
		var err error
		r, err = fontresource.Read(strings.NewReader("fake bytes"), fontresource.Origin{Provider: "test", Location: path})
		if err != nil {
			t.Fatal(err)
		}
	}
	return glyphing.FaceEntry{Face: helpers.NewSyntheticFont(), Resource: r, Info: fontmeta.Info{ResourceID: r.ID(), SHA256: r.SHA256(), Origin: r.Origin(), Path: r.Origin().Location, Warnings: []string{"warning"}}}
}
func TestPrepareOrderOwnershipAndFailures(t *testing.T) {
	pinned := fakeEntry(t, "resource", nil).Resource
	req := &Request{Settings: Settings{DefaultPath: "default", Size: 12 * dimen.BP, Rules: []Rule{{Weight: 700, Path: "rule"}}, Fallbacks: []string{"path", "path"}, FallbackResources: []*fontresource.Resource{pinned}}, DiscoveryPolicy: &DiscoveryPolicy{Providers: []string{"offline"}}, ResolutionDiagnostics: []FontDiagnostic{{Message: "original"}}}
	var calls []string
	loader := func(ctx context.Context, path string, r *fontresource.Resource) (glyphing.FaceEntry, error) {
		if r != nil {
			path = r.Origin().Location
		}
		calls = append(calls, path)
		return fakeEntry(t, path, r), nil
	}
	s, err := prepareSynthetic(t.Context(), req, loader)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"default", "rule", "path", "path", "resource"}) || len(s.Fonts.Faces) != 4 || len(s.Fonts.Fallbacks) != 3 || s.Fonts.Fallbacks[0] != s.Fonts.Fallbacks[1] {
		t.Fatalf("changed selection order: %v %+v", calls, s)
	}
	req.Rules[0].Path = "changed"
	req.Fallbacks[0] = "changed"
	req.DiscoveryPolicy.Providers[0] = "changed"
	req.ResolutionDiagnostics[0].Message = "changed"
	if s.Provenance.RequestedRulePaths[0] != "rule" || s.Provenance.RequestedFallbackPaths[0] != "path" || s.Provenance.DiscoveryPolicy.Providers[0] != "offline" || s.Provenance.ResolutionDiagnostics[0].Message != "original" {
		t.Fatal("request aliases selection")
	}
	c := s.Clone()
	c.Fonts.Faces[0].Info.Warnings[0] = "changed"
	c.Provenance.RequestedRulePaths[0] = "changed"
	if s.Fonts.Faces[0].Info.Warnings[0] != "warning" || s.Provenance.RequestedRulePaths[0] != "rule" {
		t.Fatal("clone aliases selection")
	}
	for _, mode := range []string{"substitution", "face", "resource", "identity", "error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			sentinel := errors.New("loader failure")
			load := func(context.Context, string, *fontresource.Resource) (glyphing.FaceEntry, error) {
				e := fakeEntry(t, "", pinned)
				switch mode {
				case "substitution":
					e = fakeEntry(t, "replacement", nil)
				case "face":
					e.Face = nil
				case "resource":
					e.Resource = nil
				case "identity":
					e.Info.SHA256 = "bad"
				case "error":
					return e, sentinel
				case "cancel":
					cancel()
				}
				return e, nil
			}
			got, err := prepareSynthetic(ctx, &Request{Settings: Settings{DefaultResource: pinned, Size: 12 * dimen.BP}}, load)
			if got != nil || err == nil {
				t.Fatal("invalid selection published")
			}
			if mode == "error" && !errors.Is(err, sentinel) {
				t.Fatal("cause lost")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost")
			}
		})
	}
	for _, settings := range []Settings{{Size: 12 * dimen.BP}, {DefaultPath: "path", DefaultResource: pinned, Size: 12 * dimen.BP}, {DefaultPath: "path", Size: 0}} {
		called := false
		got, err := prepareSynthetic(t.Context(), &Request{Settings: settings}, func(context.Context, string, *fontresource.Resource) (glyphing.FaceEntry, error) {
			called = true
			return glyphing.FaceEntry{}, nil
		})
		if got != nil || err == nil || called {
			t.Fatal("invalid request performed loading")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := prepareSynthetic(ctx, req, loader); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled prepare")
	}
}
func TestPolicyCacheAndTerminalFailures(t *testing.T) {
	for _, mode := range []string{"success", "read", "parse"} {
		t.Run(mode, func(t *testing.T) {
			calls, loads := 0, 0
			var patterns []string
			sf := fontfind.ScalableFont{Name: "fake"}
			sf.SetFS(fstest.MapFS{"font": &fstest.MapFile{Data: []byte("fake bytes")}}, "font")
			if mode == "read" {
				sf.SetFS(fstest.MapFS{}, "missing")
			}
			providers := []OfflineProvider{{Name: "test", Find: func(_ context.Context, d fontfind.Descriptor) (Match, error) {
				calls++
				patterns = append(patterns, d.Pattern)
				if d.Pattern == "absent" {
					return Match{}, errors.New("absent")
				}
				return Match{Font: sf, Exact: true}, nil
			}}}
			generics := map[string]string{"serif": "mapped"}
			d, err := New(providers, generics)
			if err != nil {
				t.Fatal(err)
			}
			generics["serif"] = "changed"
			d.loader = func(_ context.Context, _ string, r *fontresource.Resource) (glyphing.FaceEntry, error) {
				loads++
				if mode == "parse" {
					return glyphing.FaceEntry{}, errors.New("parse failed")
				}
				return fakeEntry(t, "", r), nil
			}
			req := FontRequest{Families: []string{"absent", "serif", "next"}}
			resource, ds, err := d.Resolve(t.Context(), req)
			if mode == "success" {
				if err != nil || loads != 1 || calls != 2 || patterns[1] != "mapped" || resource.Origin().Request.Pattern != "serif" {
					t.Fatal("policy mismatch", err)
				}
				ds[0].Message = "mutation"
				again, diags, err := d.Resolve(t.Context(), req)
				if err != nil || again != resource || calls != 2 || loads != 1 || diags[0].Message == "mutation" {
					t.Fatal("cache ownership")
				}
				other, _ := New(providers, nil)
				other.loader = d.loader
				if _, _, err := other.Resolve(t.Context(), FontRequest{Families: []string{"serif"}}); err == nil {
					t.Fatal("cache crossed session")
				}
			} else {
				var stage *ResolutionError
				if !errors.As(err, &stage) || stage.Stage != mode {
					t.Fatal("failure stage", err)
				}
				if _, _, err = d.Resolve(t.Context(), req); err == nil {
					t.Fatal("failure cached")
				}
				wantLoads := 2
				if mode == "read" {
					wantLoads = 0
				}
				if calls != 4 || loads != wantLoads {
					t.Fatalf("failure fell through or cached: %d %d", calls, loads)
				}
			}
		})
	}
}

// This private seam pairs the fake loader with its own explicit coverage contract.
func prepareSynthetic(ctx context.Context, req *Request, load loadEntry) (*Selection, error) {
	return prepare(ctx, req, load, glyphing.CoverageFuncs{
		Check:  new(helpers.SyntheticStreamingShaper).CheckFace,
		Lookup: func(contracts.FontFace, rune) (uint32, error) { return 1, nil },
	})
}

// Mapper construction is context-free; the loading operation still checks
// cancellation before publishing a completed selection.
func TestPrepareCancellationBeforePublication(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	loader := func(_ context.Context, path string, resource *fontresource.Resource) (glyphing.FaceEntry, error) {
		return fakeEntry(t, path, resource), nil
	}
	coverage := glyphing.CoverageFuncs{
		Check:  func(contracts.FontFace) error { cancel(); return nil },
		Lookup: func(contracts.FontFace, rune) (uint32, error) { t.Fatal("construction queried glyphs"); return 0, nil },
	}
	got, err := prepare(ctx, &Request{Settings: Settings{DefaultPath: "default", Size: 12 * dimen.BP}}, loader, coverage)
	if got != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled font selection published", got, err)
	}
}
