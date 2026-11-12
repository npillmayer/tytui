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
	"github.com/npillmayer/typography"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	parlayout "github.com/npillmayer/typography/paragraph"
	"github.com/npillmayer/typography/render"
	typestyle "github.com/npillmayer/typography/style"
	"github.com/npillmayer/uax/bidi"

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
// to left-to-right.
type Input struct {
	PaintPolicy typography.PaintPolicy `json:",omitempty"`
	// LineLayout opts into CSS-inspired line boxes. Nil preserves proof geometry.
	// Runs address normalized paragraph bytes; font/size strut uses backend defaults.
	// Values bypass shaping styles and are retained independently for replay.
	LineLayout *parlayout.LineLayout `json:",omitempty"`
	// Styles optionally supplies complete byte spans in normalized paragraph text.
	// Nil uses defaults. Mutually exclusive with Source. Direction is paragraph-wide;
	// DOM isolates still enter through Source. Only normal whitespace is supported.
	Styles    []typestyle.Span          `json:",omitempty"`
	Fonts     *fontdiscovery.Resolution // Nil selects synthetic; nonnil always uses OpenType.
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
	lineLayout      *parlayout.LineLayout       // Detached policy input; nil means compatibility.
	vlist           *parlayout.VList            // Retained placement; never serialized or recalculated by rendering.
	paintPolicy     typography.PaintPolicy      // Retained rendering policy, independent of inspection/replay Input.
	paint           []typography.PaintSpan      // Detached rendering input; never passed to layout.
	Identity        typography.IdentityProvider `json:"-"`
	Renderer        render.Backend              `json:"-"`
	FontSnapshot    *typography.Settings        // All configured fonts pinned at layout time.
	FontDiagnostics []fontdiscovery.FontDiagnostic
	ParagraphText   string               // Processed text addressed by frozen ranges and glyph clusters.
	TextSpans       []paragraphtext.Span // Plain input: processed bytes to Input.Text bytes; HTML uses Source.Spans.
	Fonts           []typography.Info
	FontBindings    []typography.Binding
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
func (r *Result) VList() *parlayout.VList {
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
func (r *Result) PaintSpans() []typography.PaintSpan {
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
// Each call owns fresh services and parsed fonts, so calls do not share an encoder.
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
	if input.PaintPolicy == "" {
		input.PaintPolicy = typography.GlyphBoundary
	}
	if input.PaintPolicy != typography.GlyphBoundary && input.PaintPolicy != typography.LegacyStyleBoundary {
		return fail(StageInput, fmt.Errorf("%w: unknown paint policy", ErrInvalidInput))
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
	var paint []typography.PaintSpan
	for _, span := range spans {
		paint = append(paint, typography.PaintSpan{Start: span.Start, End: span.End, Paint: span.Style.Paint()})
	}
	config := syntheticConfig()
	backend := "synthetic"
	var identity typography.IdentityProvider
	var renderer render.Backend
	var fonts []typography.Info
	var fontSnapshot *typography.Settings
	var fontDiagnostics []fontdiscovery.FontDiagnostic
	var bindings []typography.Binding
	if input.Fonts != nil {
		input.Fonts = input.Fonts.Clone()
		fontDiagnostics = append([]fontdiscovery.FontDiagnostic(nil), input.Fonts.ResolutionDiagnostics...)
		resolver, err := typography.LoadSettings(ctx, input.Fonts.Settings, typography.Services{Loader: otadapter.Services{}, Coverage: otadapter.Services{}})
		if err != nil {
			return fail(StageFonts, err)
		}
		text, bindings, err = resolver.ItemizePolicy(ctx, paragraphText, spans, input.PaintPolicy)
		if err != nil {
			return fail(StageFonts, &fontdiscovery.ResolutionError{Stage: "coverage", Err: err})
		}
		fonts = resolver.Infos()
		fontSnapshot = resolver.Snapshot()
		config = khipu.Config{Streaming: &khipu.StreamingConfig{
			NewShaper: func() (contracts.StreamingShaper, error) { return otadapter.NewStream(64) },
			Resolver:  resolver, Defaults: resolver.Defaults(),
		}}
		identity, renderer = otadapter.Services{}, otadapter.Services{}
		backend = "OpenType (core/Hebrew)"
	}
	if input.Fonts == nil && spans != nil {
		shaping, err := typography.ProjectStyles(paragraphText, spans, syntheticStyleResolver{}, config.Streaming.Defaults, input.PaintPolicy)
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
		vlist: completed.VList, lineLayout: lineLayout, paintPolicy: input.PaintPolicy,
		paint: paint, Identity: identity, Renderer: renderer, FontSnapshot: fontSnapshot, FontDiagnostics: fontDiagnostics, ParagraphText: paragraphText, TextSpans: textSpans, Fonts: fonts, FontBindings: bindings, Input: input, Text: text, BreakPositions: completed.BreakPositions, Frozen: completed.Frozen, BidiRuns: completed.BidiRuns,
		Diagnostics: []Diagnostic{
			// Preserve the successful stage transcript serialized by replay v1/v2.
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
