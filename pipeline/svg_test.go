package pipeline_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/npillmayer/khipu/dimen"
	"tytui/pipeline"
)

func svgBytes(t *testing.T, r *pipeline.Result) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := pipeline.WriteSVG(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestBreakersFreezeAndExport(t *testing.T) {
	input := pipeline.Input{Text: "éa bb cc dd ee ff gg hh", Width: 35 * dimen.BP}
	var saved []*pipeline.Result
	var proofs [][]byte
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass, pipeline.FirstFit} {
		input.Breaker = breaker
		result, err := pipeline.Run(context.Background(), input)
		if err != nil {
			t.Fatalf("%s: %v", breaker, err)
		}
		if result.Input.Breaker != breaker || result.Frozen.LineCount() < 2 {
			t.Fatalf("wrong breaker/result: %+v", result.Input)
		}
		svg := svgBytes(t, result)
		decoder := xml.NewDecoder(bytes.NewReader(svg))
		var root string
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if start, ok := token.(xml.StartElement); ok && root == "" {
				root = start.Name.Local
			}
		}
		if root != "svg" {
			t.Fatal("missing SVG root")
		}
		if !bytes.Contains(svg, []byte(breaker)) {
			t.Fatal("missing breaker metadata")
		}
		for i := 0; i < result.Frozen.LineCount(); i++ {
			line, _ := result.Frozen.Line(i)
			if line.TargetWidth != input.Width {
				t.Fatal("wrong frozen geometry")
			}
			want := fmt.Sprintf("line %d: target=%.3f actual=%.3f residual=%.3f bp", i+1, line.TargetWidth.Points(), line.Width.Points(), line.Residual.Points())
			if !bytes.Contains(svg, []byte(want)) {
				t.Fatalf("missing line diagnostic %s", want)
			}
			source := fmt.Sprintf("source bytes [%d,%d)", line.Source.Start, line.Source.End)
			if !bytes.Contains(svg, []byte(source)) {
				t.Fatalf("missing source range %s", source)
			}
		}
		saved = append(saved, result)
		proofs = append(proofs, svg)
	}
	for i, result := range saved {
		if !bytes.Equal(proofs[i], svgBytes(t, result)) {
			t.Fatal("later breaker changed retained layout")
		}
	}
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		result, err := pipeline.Run(context.Background(), pipeline.Input{Width: 100 * dimen.BP, Breaker: breaker})
		if err != nil {
			t.Fatal(err)
		}
		_ = svgBytes(t, result)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
func TestExportErrors(t *testing.T) {
	if err := pipeline.WriteSVG(io.Discard, nil); err == nil {
		t.Fatal("accepted missing result")
	}
	result, err := pipeline.Run(context.Background(), pipeline.Input{Text: "hi", Width: 100 * dimen.BP})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("writer failed")
	if err := pipeline.WriteSVG(failingWriter{sentinel}, result); !errors.Is(err, sentinel) {
		t.Fatalf("writer error lost: %v", err)
	}
	if err := pipeline.WriteSVG(nil, result); err == nil {
		t.Fatal("accepted nil writer")
	}
	if got, err := pipeline.Run(context.Background(), pipeline.Input{Width: dimen.BP, Breaker: "unknown"}); got != nil || !errors.Is(err, pipeline.ErrInvalidInput) || !strings.Contains(err.Error(), "breaker") {
		t.Fatalf("unknown breaker: %v", err)
	}
}

func TestBreakerLayoutProfiles(t *testing.T) {
	for _, breaker := range []pipeline.Breaker{pipeline.FirstFit, pipeline.KnuthPlass} {
		result, err := pipeline.Run(context.Background(), pipeline.Input{Text: "aa bb cc dd ee ff gg hh ii jj", Width: 100 * dimen.BP, Breaker: breaker})
		if err != nil {
			t.Fatal(err)
		}
		if result.Frozen.LineCount() < 2 {
			t.Fatal("fixture needs a nonfinal line")
		}
		line, _ := result.Frozen.Line(0)
		if breaker == pipeline.FirstFit && line.FillOrder != dimen.Fil {
			t.Fatalf("firstfit lost ragged-right profile: %+v", line)
		}
		if breaker == pipeline.KnuthPlass && line.FillOrder != 0 {
			t.Fatalf("Knuth-Plass lost finite-stretch profile: %+v", line)
		}
	}
}
