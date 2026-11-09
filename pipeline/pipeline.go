// Package pipeline runs paragraph integration independently of the TUI.
// Backends are deterministic synthetic shaping and explicit local OpenType fonts.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	"github.com/npillmayer/khipu/linebreak"
	"github.com/npillmayer/khipu/linebreak/firstfit"
	"github.com/npillmayer/khipu/linebreak/knuthplass"
	"github.com/npillmayer/uax/bidi"
	"tytui/fontbackend"
	"tytui/htmlinput"
)

// Breaker selects the algorithm and its matching default geometry profile.
type Breaker string

const (
	FirstFit   Breaker = "firstfit"
	KnuthPlass Breaker = "knuth-plass"
)

// Input describes one plain-text paragraph. Text is preserved without whitespace
// folding or normalization. Width must be a positive finite layout dimension,
// not terminal cells. Direction defaults to left-to-right.
type Input struct {
	Fonts     *fontbackend.Settings // Nil selects synthetic; nonnil always uses OpenType.
	Text      string
	Width     dimen.DU
	Direction bidi.Direction
	Breaker   Breaker               // Empty selects FirstFit; results contain the resolved value.
	Source    *htmlinput.Extraction // Optional detached DOM provenance, matching Text.
}

// Stage identifies an operation for diagnostics and errors.
type Stage string

const (
	StageInput     Stage = "input"
	StageFonts     Stage = "fonts"
	StageParagraph Stage = "paragraph"
	StageEncode    Stage = "encode"
	StageBreak     Stage = "break"
	StageFreeze    Stage = "freeze"
)

// Diagnostic describes a completed stage. Messages are for display, not parsing.
type Diagnostic struct {
	Stage   Stage
	Message string
}

// Result is published only after freezing succeeds. Text contains the extracted
// text/styles (unstyled for plain input). BreakPositions are paragraph-relative
// UTF-8 byte endpoints. Frozen contains line fit diagnostics and retained glyphs.
// No mutable encoder or elastic paragraph is exposed.
type Result struct {
	Fonts          []fontbackend.Info
	FontBindings   []fontbackend.Binding
	Input          Input
	Text           styled.Text
	BreakPositions []uint64
	Frozen         *khipu.FrozenKhipu
	Diagnostics    []Diagnostic
	BidiRuns       []bidi.Segment // Logical UTF-8 byte ranges before line reordering.
}

// ErrInvalidInput identifies invalid pipeline configuration or text.
var ErrInvalidInput = errors.New("invalid paragraph input")

// StageError identifies the failed operation and preserves the underlying error
// for errors.Is and errors.As, including context cancellation and backend errors.
type StageError struct {
	Stage Stage
	Err   error
}

func (e *StageError) Error() string { return fmt.Sprintf("pipeline %s: %v", e.Stage, e.Err) }
func (e *StageError) Unwrap() error { return e.Err }

// Run encodes with the selected backend (synthetic by default), breaks with
// the selected breaker's default layout, and freezes with the same geometry. Each call
// owns fresh services and parsed fonts, so calls do not share an encoder. Empty text is valid.
//
// Context must be nonnil. Cancellation is checked at stage boundaries and by
// encoding/freezing; paragraph preparation and breaking cannot be interrupted
// mid-call. Errors return a nil result, never a partially completed layout.
func Run(ctx context.Context, input Input) (*Result, error) {
	fail := func(stage Stage, err error) (*Result, error) {
		return nil, &StageError{Stage: stage, Err: err}
	}
	if ctx == nil {
		return fail(StageInput, fmt.Errorf("%w: nil context", ErrInvalidInput))
	}
	if err := ctx.Err(); err != nil {
		return fail(StageInput, err)
	}
	if input.Width <= 0 || input.Width >= dimen.Fil {
		return fail(StageInput, fmt.Errorf("%w: width must be positive and finite", ErrInvalidInput))
	}
	if input.Direction != bidi.LeftToRight && input.Direction != bidi.RightToLeft {
		return fail(StageInput, fmt.Errorf("%w: unsupported direction", ErrInvalidInput))
	}
	if !utf8.ValidString(input.Text) {
		return fail(StageInput, fmt.Errorf("%w: text must be valid UTF-8", ErrInvalidInput))
	}
	if input.Breaker == "" {
		input.Breaker = FirstFit
	}
	if input.Breaker != FirstFit && input.Breaker != KnuthPlass {
		return fail(StageInput, fmt.Errorf("%w: unknown breaker %q", ErrInvalidInput, input.Breaker))
	}
	if input.Source != nil {
		if err := input.Source.Validate(input.Text); err != nil {
			return fail(StageInput, fmt.Errorf("%w: %v", ErrInvalidInput, err))
		}
		input.Source = input.Source.Clone()
		input.Direction = input.Source.Direction
	}

	text := styled.TextFromString(input.Text)
	var markup bidi.OutOfLineBidiMarkup
	var err error
	paragraphDiagnostic := "Prepared an unstyled bidi paragraph"
	if input.Source != nil {
		text, err = input.Source.StyledText()
		if err != nil {
			return fail(StageParagraph, err)
		}
		markup, err = input.Source.BidiMarkup()
		if err != nil {
			return fail(StageParagraph, err)
		}
		paragraphDiagnostic = fmt.Sprintf("Prepared %d CSS style spans; base direction %s", len(input.Source.Styles), input.Direction)
	}
	paragraph, err := styled.ParagraphFromText(&text, 0, text.Raw().Len(), input.Direction, markup)
	if err != nil {
		return fail(StageParagraph, err)
	}
	var bidiRuns []bidi.Segment
	for segment := range paragraph.BidiLevels().LogicalSegments() {
		bidiRuns = append(bidiRuns, segment)
	}
	config := syntheticConfig()
	backend := "synthetic"
	var fonts []fontbackend.Info
	var bindings []fontbackend.Binding
	if input.Fonts != nil {
		input.Fonts = input.Fonts.Clone()
		resolver, err := fontbackend.LoadSettings(ctx, *input.Fonts)
		if err != nil {
			return fail(StageFonts, err)
		}
		bindings, err = resolver.CheckText(ctx, input.Text, input.Source)
		if err != nil {
			return fail(StageFonts, err)
		}
		fonts = resolver.Infos()
		config = khipu.Config{Streaming: &khipu.StreamingConfig{
			NewShaper: func() (contracts.StreamingShaper, error) { return fontbackend.NewStream(64) },
			Resolver:  resolver, Defaults: resolver.Defaults(),
		}}
		backend = "OpenType (otcore)"
	}
	owner := khipu.NewKhipukamayuq(config)
	encoded, err := owner.EncodeParagraph(ctx, paragraph)
	if err != nil {
		return fail(StageEncode, err)
	}
	if err := ctx.Err(); err != nil {
		return fail(StageBreak, err)
	}
	shape := linebreak.RectangularParShape(input.Width)
	var layout khipu.Params
	var breaks []int
	switch input.Breaker {
	case FirstFit:
		layout = firstfit.DefaultLayout(shape)
		breaks, err = firstfit.BreakParagraph(encoded, layout, nil)
	case KnuthPlass:
		layout = knuthplass.DefaultLayout(shape)
		breaks, err = knuthplass.BreakParagraph(encoded, layout, nil)
	}
	if err != nil {
		return fail(StageBreak, err)
	}
	positions, err := encoded.BreakpointPositions(breaks)
	if err != nil {
		return fail(StageBreak, err)
	}
	frozen, err := owner.Freeze(ctx, encoded, breaks, layout)
	if err != nil {
		return fail(StageFreeze, err)
	}
	if err := ctx.Err(); err != nil {
		return fail(StageFreeze, err)
	}
	return &Result{
		Fonts: fonts, FontBindings: bindings, Input: input, Text: text, BreakPositions: positions, Frozen: frozen, BidiRuns: bidiRuns,
		Diagnostics: []Diagnostic{
			{StageInput, fmt.Sprintf("Accepted %d UTF-8 bytes", len(input.Text))},
			{StageParagraph, paragraphDiagnostic},
			{StageEncode, fmt.Sprintf("Encoded %d knots with the %s backend", len(encoded.Kind), backend)},
			{StageBreak, fmt.Sprintf("Selected %d %s line breaks", len(breaks), input.Breaker)},
			{StageFreeze, fmt.Sprintf("Froze %d lines", frozen.LineCount())},
		},
	}, nil
}
