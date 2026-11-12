package pipeline_test

import (
	"context"
	"testing"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/render"

	"tytui/pipeline"
)

type proofIdentity struct {
	typography.IdentityProvider
	calls int
}

func (s *proofIdentity) Identify(f contracts.FontFace) (typography.Info, error) {
	s.calls++
	i, e := s.IdentityProvider.Identify(f)
	i.SHA256 = "injected-identity"
	return i, e
}

type proofOutlines struct {
	render.Backend
	calls int
}

func (s *proofOutlines) Outline(contracts.FontFace, uint32) (render.Outline, error) {
	s.calls++
	return render.Outline{Path: "M0 0 L1 1 ", Points: []render.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}}, nil
}
func TestFrozenProofUsesInjectedServices(t *testing.T) {
	r, err := pipeline.Run(context.Background(), pipeline.Input{Text: "office", Width: 100 * dimen.BP, Fonts: &fontdiscovery.Resolution{Settings: typography.Settings{DefaultPath: gentium, Size: 12 * dimen.BP}}})
	if err != nil {
		t.Fatal(err)
	}
	identity := &proofIdentity{IdentityProvider: r.Identity}
	renderer := &proofOutlines{Backend: r.Renderer}
	r.Identity = identity
	r.Renderer = renderer
	proof, err := pipeline.BuildGlyphProof(r)
	if err != nil {
		t.Fatal(err)
	}
	if identity.calls == 0 || renderer.calls != len(proof.Glyphs) || renderer.calls == 0 {
		t.Fatal("services bypassed")
	}
	for _, g := range proof.Glyphs {
		if g.FaceSHA256 != "injected-identity" || g.Path != "M0 0 L1 1 " {
			t.Fatal("backend type leaked into generic walker")
		}
	}
	r.Renderer = nil
	if _, err = pipeline.BuildGlyphProof(r); err == nil {
		t.Fatal("silently installed a renderer")
	}
}
