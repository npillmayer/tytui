package documentlayout

import (
	"errors"
	"testing"

	"github.com/npillmayer/khipu/contracts"
	ot "github.com/npillmayer/typography/glyphing/adapter/opentype"
	"github.com/npillmayer/typography/glyphing/paragraph"
)

func TestDocumentRejectsIncompatibleShaperBeforePublishing(t *testing.T) {
	i := boundaryInvocation(t, `<div id="root">ab<span style="display:inline-block;width:10px">cd</span>ef</div>`)
	cfg := i.config
	var stream *ot.Stream
	cfg.NewShaper = func() (contracts.StreamingShaper, error) {
		var err error
		stream, err = ot.NewStream(8)
		return stream, err
	}
	result, err := i.document.Layout(t.Context(), i.request, cfg)
	var stage *paragraph.Error
	if result != nil || !errors.Is(err, contracts.ErrUnsupportedFontFace) || !errors.As(err, &stage) || stage.Stage != paragraph.StagePrepare {
		t.Fatal(result, err)
	}
	if stream == nil || stream.Err() != nil {
		t.Fatal("preflight changed terminal state")
	}
	select {
	case _, ok := <-stream.Glyphs():
		if ok {
			t.Fatal("rejected stream shaped")
		}
	default:
		t.Fatal("rejected stream left open")
	}
	// A separate invocation remains usable with the original compatible services.
	if result, err = i.document.Layout(t.Context(), i.request, i.config); err != nil || result == nil {
		t.Fatal(result, err)
	}
}
