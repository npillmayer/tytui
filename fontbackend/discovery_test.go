package fontbackend

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/npillmayer/fontfind"
	"golang.org/x/image/font"
)

func discoveryFixture(t *testing.T) fontfind.ScalableFont {
	t.Helper()
	data, err := os.ReadFile("../../opentype/testdata/fonts/GentiumPlus-R.ttf")
	if err != nil {
		t.Fatal(err)
	}
	sf := fontfind.ScalableFont{Name: "Gentium"}
	sf.SetFS(fstest.MapFS{"font.ttf": &fstest.MapFile{Data: data}}, "font.ttf")
	return sf
}
func TestDescriptorTranslation(t *testing.T) {
	for weight := 100; weight <= 900; weight += 100 {
		for _, style := range []struct {
			name  string
			value font.Style
		}{{"normal", font.StyleNormal}, {"italic", font.StyleItalic}, {"oblique", font.StyleOblique}} {
			ds, err := descriptors(FontRequest{Families: []string{"A", "B"}, Weight: weight, Slant: style.name})
			if err != nil || len(ds) != 2 || ds[0].Pattern != "A" || ds[1].Pattern != "B" || ds[0].Weight != font.Weight(weight/100-4) || ds[0].Style != style.value {
				t.Fatalf("%v %v", ds, err)
			}
		}
	}
	for _, r := range []FontRequest{{}, {Families: []string{""}}, {Families: []string{"A"}, Weight: 450}, {Families: []string{"A"}, Slant: "sideways"}} {
		if _, err := descriptors(r); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
}
func TestDiscoveryOrderingCacheAndSubstitution(t *testing.T) {
	sf := discoveryFixture(t)
	var calls []string
	providers := []OfflineProvider{
		{"missing", func(ctx context.Context, d fontfind.Descriptor) (Match, error) {
			calls = append(calls, "missing:"+d.Pattern)
			return Match{}, errors.New("absent")
		}},
		{"catalog", func(ctx context.Context, d fontfind.Descriptor) (Match, error) {
			calls = append(calls, "catalog:"+d.Pattern)
			if d.Pattern == "Gentium" {
				return Match{Font: sf, Exact: true}, nil
			}
			return Match{Font: sf, Reason: "explicit alternate"}, errors.New("requested family absent")
		}},
	}
	session, err := NewDiscovery(providers, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := FontRequest{Families: []string{"Unknown", "Gentium"}}
	resource, diags, err := session.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 || resource.origin.Request.Pattern != "Gentium" || len(diags) < 5 {
		t.Fatalf("%v %+v", calls, diags)
	}
	original := append([]FontDiagnostic(nil), diags...)
	diags[0].Message = "mutated"
	cached, again, err := session.Resolve(context.Background(), request)
	if err != nil || cached != resource || len(calls) != 4 || !reflect.DeepEqual(again, original) {
		t.Fatal("cache lost evidence or ownership")
	}
	strict := FontRequest{Families: []string{"Unknown"}}
	if r, _, err := session.Resolve(context.Background(), strict); r != nil || err == nil {
		t.Fatal("strict accepted substitution")
	}
	strict.AllowSubstitution = true
	_, ds, err := session.Resolve(context.Background(), strict)
	if err != nil || !ds[2].Substituted {
		t.Fatalf("substitution: %+v %v", ds, err)
	}
	_, again, err = session.Resolve(context.Background(), strict)
	if err != nil || !reflect.DeepEqual(ds, again) {
		t.Fatal("cached substitution lost")
	}
}
func TestDiscoveryFailuresAndCancellation(t *testing.T) {
	sf := discoveryFixture(t)
	for _, stage := range []string{"read", "parse"} {
		broken := fontfind.ScalableFont{Name: "broken"}
		if stage == "parse" {
			broken.SetFS(fstest.MapFS{"bad": &fstest.MapFile{Data: []byte("not a font")}}, "bad")
		}
		second := false
		d, _ := NewDiscovery([]OfflineProvider{{"broken", func(context.Context, fontfind.Descriptor) (Match, error) {
			return Match{Font: broken, Exact: true}, nil
		}}, {"next", func(context.Context, fontfind.Descriptor) (Match, error) {
			second = true
			return Match{Font: sf, Exact: true}, nil
		}}}, nil)
		_, ds, err := d.Resolve(context.Background(), FontRequest{Families: []string{"A"}})
		var failure *ResolutionError
		if !errors.As(err, &failure) || failure.Stage != stage || second || ds[len(ds)-1].Stage != stage {
			t.Fatalf("%v %+v", err, ds)
		}
	}
	entered := make(chan struct{})
	d, _ := NewDiscovery([]OfflineProvider{{"blocking", func(ctx context.Context, _ fontfind.Descriptor) (Match, error) {
		close(entered)
		<-ctx.Done()
		return Match{}, ctx.Err()
	}}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := d.Resolve(ctx, FontRequest{Families: []string{"A"}}); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestDiscoveryGenericPolicyAndKeys(t *testing.T) {
	sf := discoveryFixture(t)
	var requests []fontfind.Descriptor
	d, _ := NewDiscovery([]OfflineProvider{{"catalog", func(_ context.Context, desc fontfind.Descriptor) (Match, error) {
		requests = append(requests, desc)
		return Match{Font: sf, Exact: true}, nil
	}}}, map[string]string{"serif": "Gentium"})
	for _, req := range []FontRequest{{Families: []string{"serif"}}, {Families: []string{"A.ttf"}, Weight: 600, Slant: "italic"}, {Families: []string{"A.ttf"}, Weight: 700, Slant: "italic"}, {Families: []string{"A.ttf"}, Weight: 700, Slant: "oblique"}} {
		if _, _, err := d.Resolve(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 4 || requests[0].Pattern != "Gentium" {
		t.Fatal(requests)
	}
	if _, ds, err := d.Resolve(context.Background(), FontRequest{Families: []string{"sans-serif"}}); err == nil || len(ds) != 1 {
		t.Fatal("missing generic policy accepted")
	}
}

func TestDiscoveryQueuedCancellationAndConcurrentCache(t *testing.T) {
	sf := discoveryFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	d, _ := NewDiscovery([]OfflineProvider{{"catalog", func(context.Context, fontfind.Descriptor) (Match, error) {
		calls++
		close(entered)
		<-release
		return Match{Font: sf, Exact: true}, nil
	}}}, nil)
	request := FontRequest{Families: []string{"A"}}
	first := make(chan error, 1)
	go func() { _, _, err := d.Resolve(context.Background(), request); first <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := d.Resolve(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { _, _, err := d.Resolve(context.Background(), request); done <- err }()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("provider invoked %d times", calls)
	}
}

func TestDescriptorSettingsSnapshot(t *testing.T) {
	sf := discoveryFixture(t)
	d, _ := NewDiscovery([]OfflineProvider{{"catalog", func(context.Context, fontfind.Descriptor) (Match, error) { return Match{Font: sf, Exact: true}, nil }}}, nil)
	input := DescriptorSettings{Default: FontRequest{Families: []string{"A"}}, Size: 12 * 65536, Rules: []DescriptorRule{{Weight: 700, Request: FontRequest{Families: []string{"B"}}}}, Fallbacks: []FontRequest{{Families: []string{"C"}}}}
	s, _, err := d.ResolveSettings(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	cloned := s.Clone()
	input.Default.Families[0] = "changed"
	s.DescriptorInput.Rules[0].Request.Families[0] = "changed"
	s.DescriptorInput.Fallbacks[0].Families[0] = "changed"
	if cloned.DescriptorInput.Default.Families[0] != "A" || cloned.DescriptorInput.Rules[0].Request.Families[0] != "B" || cloned.DescriptorInput.Fallbacks[0].Families[0] != "C" {
		t.Fatal("descriptor snapshot aliased caller")
	}
}
