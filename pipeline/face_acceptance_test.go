package pipeline

import (
	"errors"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/helpers"
	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"

	"tytui/fontselection"
)

func TestPipelineRejectsForeignSelectedFaceAtFontsStage(t *testing.T) {
	resource, err := fontresource.Read(strings.NewReader("test"), fontresource.Origin{Provider: "test", Location: "inert"})
	if err != nil {
		t.Fatal(err)
	}
	entry := glyphing.FaceEntry{Face: helpers.NewSyntheticFont(), Resource: resource, Info: fontmeta.Info{ResourceID: resource.ID(), SHA256: resource.SHA256(), Origin: resource.Origin(), Path: "inert"}}
	input := Input{Text: "a", Width: 100 * dimen.BP, Fonts: &fontselection.Selection{Fonts: glyphing.SelectedFonts{Faces: []glyphing.FaceEntry{entry}, Default: resource.ID(), Size: 10 * dimen.BP}}}
	result, err := Run(t.Context(), input)
	var stage *StageError
	if result != nil || !errors.Is(err, contracts.ErrUnsupportedFontFace) || !errors.As(err, &stage) || stage.Stage != StageFonts {
		t.Fatal(result, err)
	}
}
