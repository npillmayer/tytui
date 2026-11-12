package pipeline

import (
	"context"
	"testing"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/linebreak"
	"github.com/npillmayer/khipu/linebreak/firstfit"
	typestyle "github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"
)

func TestBuiltStylesReachFrozenLayout(t *testing.T) {
	b := styled.NewTextBuilder()
	style := typestyle.Style{Family: "serif", EmSize: 12 * dimen.BP, Weight: 400, Slant: "normal", Color: "red", Language: "en", Direction: bidi.LeftToRight, WhiteSpace: "normal", UnicodeBidi: "normal"}
	if err := b.AppendString("é ", style); err != nil {
		t.Fatal(err)
	}
	style.EmSize = 24 * dimen.BP
	if err := b.AppendString("abc", style); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 6 {
		t.Fatalf("builder byte length=%d", b.Len())
	}
	text := b.Text()
	// Empty style selections at the start, a multibyte boundary, and the end
	// must preserve both sizes before paragraph construction and freezing.
	for _, pos := range []uint64{0, 2, b.Len()} {
		var err error
		text, err = text.Style(nil, pos, pos)
		if err != nil {
			t.Fatalf("empty style selection at %d: %v", pos, err)
		}
	}
	par, err := styled.ParagraphFromText(&text, 0, b.Len(), bidi.LeftToRight, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := khipu.NewKhipukamayuq(syntheticConfig())
	encoded, err := owner.EncodeParagraph(context.Background(), par)
	if err != nil {
		t.Fatal(err)
	}
	layout := firstfit.DefaultLayout(linebreak.RectangularParShape(200 * dimen.BP))
	breaks, err := firstfit.BreakParagraph(encoded, layout, nil)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := owner.Freeze(context.Background(), encoded, breaks, layout)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.SourceLen() != 6 || frozen.LineCount() != 1 {
		t.Fatal("lost built paragraph")
	}
	sizes := map[dimen.DU]bool{}
	for k := 0; k < frozen.KnotCount(); k++ {
		if tx, ok := frozen.CopyText(k); ok {
			sizes[tx.Context.EmSize] = true
		}
	}
	if !sizes[12*dimen.BP] || !sizes[24*dimen.BP] {
		t.Fatalf("builder styles lost through freezing: %v", sizes)
	}
}
