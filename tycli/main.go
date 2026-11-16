// tycli runs the document integration pipeline without a terminal UI.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/typography"
	ot "github.com/npillmayer/typography/adapter/opentype"
	"github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	"github.com/thatisuday/commando"
	"gopkg.in/yaml.v3"
	"tytui/documentlayout"
	"tytui/fontselection"
	"tytui/htmlinput"
)

type options struct {
	input, css, fonts, root, output, width, breaker, log string
	showGlyphs, hideBoxes                                bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	registry := commando.NewCommandRegistry().SetExecutableName("tycli").SetVersion("0.1.0").SetDescription("Lay out HTML/CSS and export positioned CSS box outlines.")
	cmd := registry.Register(nil).AddArgument("html", "Local HTML input", "")
	for _, f := range []struct{ name, desc, value string }{
		{"css", "Additional author stylesheet (after embedded CSS)", ""},
		{"fonts", "YAML font mapping file (required)", ""},
		{"root", "Element ID to lay out; empty selects body", ""},
		{"output,o", "SVG path, or - for stdout", "boxes.svg"},
		{"width,w", "Containing width in CSS pixels", "800"},
		{"breaker", "firstfit or knuth-plass", "firstfit"},
		{"log", "quiet, info, or trace (stderr)", "info"},
	} {
		cmd.AddFlag(f.name, f.desc, commando.String, f.value)
	}
	// Commando treats empty string defaults as required; these flags are optional.
	for _, name := range []string{"css", "root", "fonts"} {
		cmd.Flags[name].IsRequired = false
	}
	cmd.AddFlag("show-glyphs", "Overlay final glyph outlines", commando.Bool, nil)
	cmd.AddFlag("hide-boxes", "Show glyphs without box or fragment outlines", commando.Bool, nil)
	cmd.SetAction(func(args map[string]commando.ArgValue, flags map[string]commando.FlagValue) {
		value := func(k string) string { s, _ := flags[k].GetString(); return s }
		showGlyphs, _ := flags["show-glyphs"].GetBool()
		hideBoxes, _ := flags["hide-boxes"].GetBool()
		err := run(ctx, options{input: args["html"].Value, css: value("css"), fonts: value("fonts"), root: value("root"), output: value("output"), width: value("width"), breaker: value("breaker"), log: value("log"), showGlyphs: showGlyphs, hideBoxes: hideBoxes}, os.Stdout, os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tycli:", err)
			os.Exit(1)
		}
	})
	registry.Parse(nil)
}

type fontMap struct {
	Default   string   `yaml:"default"`
	Fallbacks []string `yaml:"fallbacks"`
	Rules     []struct {
		Family string `yaml:"family"`
		Weight int    `yaml:"weight"`
		Slant  string `yaml:"slant"`
		Path   string `yaml:"path"`
	} `yaml:"rules"`
}

func loadFonts(path string) (*fontselection.Request, error) {
	if path == "" {
		return nil, fmt.Errorf("--fonts is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m fontMap
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("font mapping: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("font mapping must contain exactly one YAML document")
	}
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(filepath.Dir(path), p)
	}
	r := &fontselection.Request{Settings: fontselection.Settings{DefaultPath: resolve(m.Default), Size: 12 * dimen.BP}}
	if m.Default == "" {
		return nil, fmt.Errorf("font mapping requires default")
	}
	for _, p := range m.Fallbacks {
		if p == "" {
			return nil, fmt.Errorf("empty fallback path")
		}
		r.Fallbacks = append(r.Fallbacks, resolve(p))
	}
	for _, rule := range m.Rules {
		r.Rules = append(r.Rules, fontselection.Rule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, Path: resolve(rule.Path)})
	}
	return r, nil
}

func run(ctx context.Context, o options, stdout, stderr io.Writer) error {
	if o.log != "quiet" && o.log != "info" && o.log != "trace" {
		return fmt.Errorf("invalid --log %q", o.log)
	}
	width, err := strconv.ParseFloat(o.width, 32)
	if err != nil || math.IsNaN(width) || math.IsInf(width, 0) || width < 0 {
		return fmt.Errorf("--width must be a finite nonnegative CSS pixel width")
	}
	if o.breaker != string(paragraph.FirstFit) && o.breaker != string(paragraph.KnuthPlass) {
		return fmt.Errorf("unknown breaker %q", o.breaker)
	}
	var mu sync.Mutex
	log := func(s string) { mu.Lock(); defer mu.Unlock(); fmt.Fprintln(stderr, s) }
	info := func(s string) {
		if o.log != "quiet" {
			log(s)
		}
	}
	source, err := os.Open(o.input)
	if err != nil {
		return err
	}
	defer source.Close()
	var css []byte
	if o.css != "" {
		css, err = os.ReadFile(o.css)
		if err != nil {
			return err
		}
	}
	info("read HTML/CSS: " + o.input)
	input, err := htmlinput.ParseWithCSS(o.input, source, string(css))
	if err != nil {
		return err
	}
	for _, w := range input.Warnings {
		info("warning: " + w)
	}
	selected := -1
	for _, n := range input.Nodes {
		match := o.root == "" && n.Name == "body"
		for _, a := range n.Attributes {
			if o.root != "" && a.Key == "id" && a.Val == o.root {
				match = true
			}
		}
		if match {
			if selected != -1 {
				return fmt.Errorf("root selection is ambiguous")
			}
			selected = n.ID
		}
	}
	if selected < 0 {
		return fmt.Errorf("root %q not found (default: body)", o.root)
	}
	doc, err := documentlayout.Build(ctx, input, selected)
	if err != nil {
		return err
	}
	request, err := loadFonts(o.fonts)
	if err != nil {
		return err
	}
	info("load fonts: " + o.fonts)
	selection, err := fontselection.Prepare(ctx, request)
	if err != nil {
		return err
	}
	resolver, err := typography.NewResolver(ctx, selection.Fonts, ot.Services{})
	if err != nil {
		return err
	}
	config := documentlayout.Config{Resolver: resolver, NewShaper: func() (contracts.StreamingShaper, error) {
		s, err := ot.NewStream(64)
		if err != nil {
			return nil, err
		}
		if o.log == "trace" {
			return &traceShaper{StreamingShaper: s, log: log}, nil
		}
		return s, nil
	}}
	if o.log == "trace" {
		config.Trace = log
	}
	info(fmt.Sprintf("layout: root=%s width=%g CSS px breaker=%s", input.Nodes[selected].Path, width, o.breaker))
	result, err := doc.Layout(ctx, documentlayout.LayoutRequest{Width: float32(width), Breaker: paragraph.Breaker(o.breaker)}, config)
	if err != nil {
		return err
	}
	for _, group := range result.Groups() {
		g, _ := result.Group(group.GroupID)
		f := g.Paragraph.Frozen
		info(fmt.Sprintf("group=%d owner=%d lines=%d knots=%d", group.GroupID, group.OwnerID, f.LineCount(), f.KnotCount()))
		if o.log == "trace" {
			for i := 0; i < f.LineCount(); i++ {
				l, _ := f.Line(i)
				log(fmt.Sprintf("group=%d line=%d %+v", group.GroupID, i, l))
			}
			for i := 0; i < f.KnotCount(); i++ {
				k, _ := f.Knot(i)
				log(fmt.Sprintf("group=%d knot=%d %+v", group.GroupID, i, k))
			}
		}
	}
	var svg bytes.Buffer
	if err := documentlayout.WriteLayoutSVG(ctx, &svg, result, render.Services{Backend: ot.Services{}, Identity: ot.Services{}}, documentlayout.LayoutSVGOptions{ShowGlyphs: o.showGlyphs, HideBoxes: o.hideBoxes}); err != nil {
		return err
	}
	if o.output == "-" {
		_, err = io.Copy(stdout, &svg)
	} else {
		err = writeOutput(o.output, svg.Bytes())
	}
	if err != nil {
		return err
	}
	info("wrote SVG: " + o.output)
	return nil
}

// Replace the destination only after a complete successful write.
func writeOutput(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tycli-*.svg")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

type traceShaper struct {
	contracts.StreamingShaper
	log func(string)
}

func (s *traceShaper) Shape(ctx context.Context, p contracts.ShapeParams, r contracts.RuneReader, b contracts.ShapeBufferOptions) error {
	s.log(fmt.Sprintf("shape: script=%s language=%s direction=%v features=%v", p.Script, p.Language, p.Direction, p.Features))
	err := s.StreamingShaper.Shape(ctx, p, r, b)
	s.log(fmt.Sprintf("shape complete: error=%v", err))
	return err
}
