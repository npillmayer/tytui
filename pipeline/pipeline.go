// Package pipeline runs paragraph integration independently of the TUI.
// Backends are deterministic synthetic shaping and explicit local OpenType fonts.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/npillmayer/cords/styled"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"github.com/npillmayer/khipu/dimen"
	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/glyphing"
	otadapter "github.com/npillmayer/typography/glyphing/adapter/opentype"
	parlayout "github.com/npillmayer/typography/glyphing/paragraph"
	"github.com/npillmayer/typography/layout"
	"github.com/npillmayer/typography/render"
	renderot "github.com/npillmayer/typography/render/adapter/opentype"
	typestyle "github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"

	"tytui/fontselection"
	"tytui/htmlinput"
	"tytui/paragraphtext"
)

// Breaker selects the algorithm and its matching default geometry profile.
type Breaker string

const (
	FirstFit   Breaker = "firstfit"
	KnuthPlass Breaker = "knuth-plass"
)

// Input describes one explicit paragraph. Plain input whitespace is collapsed
// before shaping; results retain the original Input and a byte map. Width must
// be a positive finite layout dimension, not terminal cells. Direction defaults
// to left-to-right. Fonts must already be loaded; Run performs no discovery or I/O.
type Input struct {

	// LineLayout opts into CSS-inspired line boxes. Nil preserves proof geometry.
	// Runs address normalized paragraph bytes; font/size strut uses backend defaults.
	// Values bypass shaping styles and are retained independently for replay.
	LineLayout *parlayout.LineLayout `json:",omitempty"`
	// Styles optionally supplies complete byte spans in normalized paragraph text.
	// Nil uses defaults. Mutually exclusive with Source. Direction is paragraph-wide;
	// DOM isolates still enter through Source. Only normal whitespace is supported.
	Styles    []typestyle.Span         `json:",omitempty"`
	Fonts     *fontselection.Selection // Nil selects synthetic; nonnil always uses OpenType.
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
	StageAssemble  Stage = "assemble"
)

// Diagnostic describes a completed stage. Messages are for display, not parsing.
type Diagnostic struct {
	Stage   Stage
	Message string
}

// Result is published only after freezing and assembly succeed. Text contains
// typography-only styles (unstyled for plain synthetic input). Paint is retained separately from
// the typesetter; Input preserves document values for inspection/replay.
// BreakPositions are paragraph-relative UTF-8 byte endpoints. Frozen contains
// line fit diagnostics and retained glyphs.
// No mutable encoder or elastic paragraph is exposed.
type Result struct {
	lineLayout *parlayout.LineLayout // Detached policy input; nil means compatibility.
	vlist      *layout.VList         // Retained placement; never serialized or recalculated by rendering.

	paint           []typestyle.PaintSpan     // Detached rendering input; never passed to layout.
	Identity        fontmeta.IdentityProvider `json:"-"`
	Renderer        render.Backend            `json:"-"`
	fontProvenance  fontselection.Provenance
	FontSnapshot    *glyphing.SelectedFonts // All configured fonts pinned at layout time.
	FontDiagnostics []fontselection.FontDiagnostic
	ParagraphText   string               // Processed text addressed by frozen ranges and glyph clusters.
	TextSpans       []paragraphtext.Span // Plain input: processed bytes to Input.Text bytes; HTML uses Source.Spans.
	Fonts           []fontmeta.Info
	FontBindings    []glyphing.FontTextRun
	Input           Input
	Text            styled.Text
	BreakPositions  []uint64
	Frozen          *khipu.FrozenKhipu
	Diagnostics     []Diagnostic
	BidiRuns        []bidi.Segment // Logical UTF-8 byte ranges before line reordering.
}

// VList returns a detached handle to the retained paragraph placements for either
// backend. Its geometry accessors return copies; Frozen shares immutable content.
// Replacing the returned handle or changing its returned values cannot alter r.
func (r *Result) VList() *layout.VList {
	if r == nil || r.vlist == nil {
		return nil
	}
	v := *r.vlist
	return &v
}

// LineLayout returns the detached layout policy used to assemble this result.
// Nil means ProofCompatibility, including old replay cases.
func (r *Result) LineLayout() *parlayout.LineLayout {
	if r == nil || r.lineLayout == nil {
		return nil
	}
	return r.lineLayout.Clone()
}

// PaintSpans returns detached logical-byte color ranges for rendering/inspection.
// Their boundaries need not coincide with the typography styles or glyphs.
func (r *Result) PaintSpans() []typestyle.PaintSpan {
	if r == nil {
		return nil
	}
	return slices.Clone(r.paint)
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
// the selected breaker's default layout, freezes and assembles retained placements.
// Each call owns fresh encoding state; input faces are already loaded and retained.
// Empty text is valid.
//
// Context must be nonnil. Cancellation is checked at stage boundaries and by
// encoding/freezing/assembly; paragraph preparation and breaking cannot be interrupted
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
	if input.Source != nil && input.Styles != nil {
		return fail(StageInput, fmt.Errorf("%w: Styles and Source are mutually exclusive", ErrInvalidInput))
	}
	if input.Source != nil {
		if err := input.Source.Validate(input.Text); err != nil {
			return fail(StageInput, fmt.Errorf("%w: %v", ErrInvalidInput, err))
		}
		input.Source = input.Source.Clone()
		input.Direction = input.Source.Direction
	}

	paragraphText := input.Text
	var textSpans []paragraphtext.Span
	if input.Source == nil {
		paragraphText, textSpans = paragraphtext.Normalize(input.Text)
	}
	input.Styles = slices.Clone(input.Styles)
	if err := typestyle.ValidateSpans(paragraphText, input.Styles); err != nil {
		return fail(StageInput, fmt.Errorf("%w: %v", ErrInvalidInput, err))
	}
	for _, span := range input.Styles {
		if span.Style.WhiteSpace != "normal" || span.Style.UnicodeBidi != "normal" {
			return fail(StageInput, fmt.Errorf("%w: shared spans require normal whitespace and unicode-bidi", ErrInvalidInput))
		}
	}
	spans := input.Styles
	text := styled.TextFromString(paragraphText)
	var markup bidi.OutOfLineBidiMarkup
	var err error
	paragraphDiagnostic := "Prepared an unstyled bidi paragraph"
	if spans != nil {
		paragraphDiagnostic = fmt.Sprintf("Prepared %d shared style spans", len(spans))
	}
	if input.Source != nil {
		spans = input.Source.Styles
		markup, err = input.Source.BidiMarkup()
		if err != nil {
			return fail(StageParagraph, err)
		}
		paragraphDiagnostic = fmt.Sprintf("Prepared %d CSS style spans; base direction %s", len(input.Source.Styles), input.Direction)
	}
	// Document values remain available for replay, but only typography reaches
	// the typesetter. Rendering owns an independent copy of the original colors.
	var paint []typestyle.PaintSpan
	for _, span := range spans {
		paint = append(paint, typestyle.PaintSpan{Start: span.Start, End: span.End, Paint: span.Style.Paint()})
	}
	config := syntheticConfig()
	backend := "synthetic"
	var identity fontmeta.IdentityProvider
	var renderer render.Backend
	var fonts []fontmeta.Info
	var fontSnapshot *glyphing.SelectedFonts
	var fontProvenance fontselection.Provenance
	var fontDiagnostics []fontselection.FontDiagnostic
	var bindings []glyphing.FontTextRun
	if input.Fonts != nil {
		input.Fonts = input.Fonts.Clone()
		fontProvenance = input.Fonts.Provenance.Clone()
		fontDiagnostics = append([]fontselection.FontDiagnostic(nil), input.Fonts.Provenance.ResolutionDiagnostics...)
		resolver, err := glyphing.NewStyleMapper(ctx, input.Fonts.Fonts, otadapter.Services{})
		if err != nil {
			return fail(StageFonts, err)
		}
		text, bindings, err = resolver.Itemize(ctx, paragraphText, spans)
		if err != nil {
			return fail(StageFonts, &fontselection.ResolutionError{Stage: "coverage", Err: err})
		}
		fonts = resolver.Infos()
		fontSnapshot = resolver.Snapshot()
		config = khipu.Config{Streaming: &khipu.StreamingConfig{
			NewShaper: func() (contracts.StreamingShaper, error) { return otadapter.NewStream(64) },
			Resolver:  resolver, Defaults: resolver.Defaults(),
		}}
		renderServices := renderot.New()
		identity, renderer = renderServices, renderServices
		backend = "OpenType (core/Hebrew)"
	}
	if input.Fonts == nil && spans != nil {
		shaping, err := glyphing.ProjectStyles(paragraphText, spans, syntheticStyleResolver{}, config.Streaming.Defaults)
		if err != nil {
			return fail(StageParagraph, err)
		}
		text = styled.TextFromString(paragraphText)
		for _, span := range shaping {
			text, err = text.Style(span.Style, span.Start, span.End)
			if err != nil {
				return fail(StageParagraph, err)
			}
		}
	}
	config.Hyphenator = explicitHyphens{}
	assembly := parlayout.Options{Policy: parlayout.ProofCompatibility, DefaultEmSize: config.Streaming.Defaults.EmSize}
	var lineLayout *parlayout.LineLayout
	if input.LineLayout != nil {
		lineLayout = input.LineLayout.Clone()
		input.LineLayout = lineLayout.Clone()
		assembly = parlayout.Options{Policy: parlayout.CSSLineHeight}
	}
	completed, err := parlayout.Run(ctx, parlayout.Input{Text: text, Direction: input.Direction, Markup: markup}, parlayout.Params{
		Constraints: parlayout.Constraints{Width: input.Width}, Breaker: parlayout.Breaker(input.Breaker),
		Assembly: assembly, LineLayout: lineLayout,
	}, config)
	if err != nil {
		var stageErr *parlayout.Error
		if errors.As(err, &stageErr) {
			return fail(Stage(stageErr.Stage), stageErr.Err)
		}
		return fail(StageEncode, err)
	}

	return &Result{
		vlist: completed.VList, lineLayout: lineLayout,
		fontProvenance: fontProvenance, paint: paint, Identity: identity, Renderer: renderer, FontSnapshot: fontSnapshot, FontDiagnostics: fontDiagnostics, ParagraphText: paragraphText, TextSpans: textSpans, Fonts: fonts, FontBindings: bindings, Input: input, Text: text, BreakPositions: completed.BreakPositions, Frozen: completed.Frozen, BidiRuns: completed.BidiRuns,
		Diagnostics: []Diagnostic{
			// Assembly failures have their own StageAssemble error; no partial result
			// is published. VList exposes successful assembly independently.
			{StageInput, fmt.Sprintf("Accepted %d UTF-8 bytes", len(input.Text))},
			{StageParagraph, paragraphDiagnostic},
			{StageEncode, fmt.Sprintf("Encoded %d knots with the %s backend", completed.EncodedKnots, backend)},
			{StageBreak, fmt.Sprintf("Selected %d %s line breaks", completed.BreakCount, input.Breaker)},
			{StageFreeze, fmt.Sprintf("Froze %d lines", completed.Frozen.LineCount())},
		},
	}, nil
}

// FontProvenance returns detached application selection evidence used by replay.
func (r *Result) FontProvenance() fontselection.Provenance {
	if r == nil {
		return fontselection.Provenance{}
	}
	return r.fontProvenance.Clone()
}
