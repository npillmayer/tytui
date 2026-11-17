//go:build ignore

// Run from the tytui module root; fonts are explicit local test fixtures.
// go run documentlayout/testdata/proof.go -out /tmp/nested.svg
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography/glyphing"
	ot "github.com/npillmayer/typography/glyphing/adapter/opentype"
	"github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/render"
	renderot "github.com/npillmayer/typography/render/adapter/opentype"

	"tytui/documentlayout"
	"tytui/fontselection"
	"tytui/htmlinput"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	out := flag.String("out", "/tmp/nested.svg", "output SVG path")
	inputPath := flag.String("input", "documentlayout/testdata/nested.html", "HTML fixture path")
	breaker := flag.String("breaker", string(paragraph.FirstFit), "firstfit or knuth-plass")
	width := flag.Float64("width", 420, "containing width in CSS pixels")
	flag.Parse()
	ctx := context.Background()
	source, err := os.ReadFile(*inputPath)
	if err != nil {
		return err
	}
	input, err := htmlinput.Parse("nested.html", bytes.NewReader(source))
	if err != nil {
		return err
	}
	selected := -1
	for _, n := range input.Nodes {
		for _, a := range n.Attributes {
			if a.Key == "id" && a.Val == "root" {
				selected = n.ID
			}
		}
	}
	doc, err := documentlayout.Build(ctx, input, selected)
	if err != nil {
		return err
	}
	s := ot.Services{}
	fonts, err := fontselection.Prepare(ctx, &fontselection.Request{Settings: fontselection.Settings{DefaultPath: "../opentype/testdata/fonts/GentiumPlus-R.ttf", Fallbacks: []string{"../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf"}, Size: 15 * dimen.BP}})
	if err != nil {
		panic(err)
	}
	resolver, err := glyphing.NewResolver(ctx, fonts.Fonts, s)
	if err != nil {
		return err
	}
	result, err := doc.Layout(ctx, documentlayout.LayoutRequest{Width: float32(*width), Breaker: paragraph.Breaker(*breaker)}, documentlayout.Config{Resolver: resolver, NewShaper: func() (contracts.StreamingShaper, error) { return ot.NewStream(64) }})
	if err != nil {
		return err
	}
	var svg bytes.Buffer
	if err := documentlayout.WriteSVG(ctx, &svg, result, render.Services{Backend: renderot.New(), Identity: renderot.New()}); err != nil {
		return err
	}
	return os.WriteFile(*out, svg.Bytes(), 0644)
}
