package fontbackend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	"github.com/npillmayer/opentype/otshape"
	"golang.org/x/text/language"
)

const progressFont = "../../opentype/testdata/fonts/ShaperTestProgress.ttf"

func fixture(t *testing.T) *Face {
	t.Helper()
	f, err := LoadFile(progressFont)
	if err != nil {
		t.Fatal(err)
	}
	if f.Info().SHA256 != "6b4bb3ea60b769f0b385eeed74dc79ab50e61628f458c1a1368548acd59293be" {
		t.Fatal("fixture changed")
	}
	return f
}
func TestFaceAndParser(t *testing.T) {
	f := fixture(t)
	m, err := f.Metrics()
	if err != nil || m.UnitsPerEm != 1000 || m.Ascent != 800 || m.Descent != -200 {
		t.Fatalf("metrics %+v %v", m, err)
	}
	scale, err := f.Scale(12 * dimen.BP)
	if err != nil || scale.ToDU(-500) != -6*float64(dimen.BP) {
		t.Fatal("signed scale", err)
	}
	if _, err = f.Scale(0); err == nil {
		t.Fatal("zero size")
	}
	if _, err = (Loader{}).LoadFont(strings.NewReader("bad font")); err == nil {
		t.Fatal("accepted bad font")
	}
	if _, err = LoadFile("missing.ttf"); err == nil {
		t.Fatal("accepted missing file")
	}
	if _, err = LoadFile("../../opentype/testdata/fonts/ShaperTestHebrew-NoGPOS.ttf"); err == nil {
		t.Fatal("accepted font lacking required GPOS")
	}
}
func TestStreamReferenceAndClusters(t *testing.T) {
	f := fixture(t)
	s, _ := NewStream(1)
	params := contracts.ShapeParams{Font: f, Script: language.MustParseScript("Latn"), Language: language.English}
	done := make(chan error, 1)
	go func() {
		if err := s.SetInitialCluster(4); err != nil {
			done <- err
			return
		}
		if err := s.Shape(context.Background(), params, strings.NewReader("aa"), contracts.ShapeBufferOptions{}); err != nil {
			done <- err
			return
		}
		err := s.Shape(context.Background(), params, strings.NewReader("a"), contracts.ShapeBufferOptions{})
		if err == nil {
			err = s.Finish()
		}
		done <- err
	}()
	var got []contracts.ShapedGlyph
	for g := range s.Glyphs() {
		got = append(got, g)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Independent HarfBuzz baseline: progress font's aa kern is -20.
	want := []contracts.ShapedGlyph{{GlyphID: 1, Cluster: 4, XAdvance: 580}, {GlyphID: 1, Cluster: 5, XAdvance: 600}, {GlyphID: 1, Cluster: 6, XAdvance: 600}}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("glyph %d: %+v want %+v", i, got[i], want[i])
		}
	}
	if s.Err() != nil || s.Finish() != nil {
		t.Fatal("terminal success lost")
	}
	if err := s.SetInitialCluster(0); !errors.Is(err, otshape.ErrStreamClosed) {
		t.Fatal(err)
	}
}
func TestStreamFailureAndCancel(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		s, _ := NewStream(1)
		p := contracts.ShapeParams{Font: fixture(t), Script: language.MustParseScript("Latn"), Language: language.English}
		ctx, cancel := context.WithCancel(context.Background())
		if foreign {
			p.Font = helpers.NewSyntheticFont()
		} else {
			cancel()
		}
		err := s.Shape(ctx, p, strings.NewReader("a"), contracts.ShapeBufferOptions{})
		cancel()
		if err == nil || s.Err() == nil {
			t.Fatal("missing terminal error")
		}
		if _, ok := <-s.Glyphs(); ok {
			t.Fatal("output not closed")
		}
	}
}

func TestStreamBackpressureCancellation(t *testing.T) {
	s, _ := NewStream(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	params := contracts.ShapeParams{Font: fixture(t), Script: language.MustParseScript("Latn"), Language: language.English}
	done := make(chan error, 1)
	go func() {
		done <- s.Shape(ctx, params, strings.NewReader(strings.Repeat("a", 100)), contracts.ShapeBufferOptions{})
	}()
	<-s.Glyphs() // Production is active and cannot finish with a one-glyph queue.
	if err := s.Finish(); !errors.Is(err, otshape.ErrStreamBusy) {
		t.Fatal(err)
	}
	if err := s.SetInitialCluster(0); !errors.Is(err, otshape.ErrStreamBusy) {
		t.Fatal(err)
	}
	cancel()
	for range s.Glyphs() {
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !errors.Is(s.Err(), context.Canceled) {
		t.Fatal(s.Err())
	}
}

func TestFeatureRangesAndMissingOutput(t *testing.T) {
	for _, missing := range []bool{false, true} {
		s, _ := NewStream(2)
		p := contracts.ShapeParams{Font: fixture(t), Script: language.MustParseScript("Latn"), Language: language.English, Features: []contracts.ShapeFeature{{Tag: [4]byte{'t', 'e', 's', 't'}, Enabled: true, Start: 2, End: 4}}}
		text := "fifi"
		if missing {
			text = "X"
		}
		done := make(chan error, 1)
		go func() {
			err := s.Shape(context.Background(), p, strings.NewReader(text), contracts.ShapeBufferOptions{Flush: contracts.ShapeFlushCluster})
			if err == nil {
				err = s.Finish()
			}
			done <- err
		}()
		var glyphs []contracts.ShapedGlyph
		for g := range s.Glyphs() {
			glyphs = append(glyphs, g)
		}
		err := <-done
		if missing {
			if err == nil || !strings.Contains(err.Error(), "missing glyph") {
				t.Fatal(err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(glyphs) != 3 || glyphs[0].GlyphID != 4 || glyphs[1].GlyphID != 5 || glyphs[2].GlyphID != 6 || glyphs[2].Cluster != 2 {
			t.Fatalf("feature range: %+v", glyphs)
		}
	}
}
