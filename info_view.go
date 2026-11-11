package main

import (
	"fmt"
	"strings"

	"github.com/npillmayer/khipu"
	"tytui/pipeline"
)

// This is source text inspection, not positioned glyph rendering. Quote text
// so invisible whitespace and terminal control characters remain inspectable.
func renderResult(result *pipeline.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %d lines · width %.2f bp\nProcessed text shown in logical order; ranges are UTF-8 bytes.\n\n", result.Input.Breaker, result.Frozen.LineCount(), result.Input.Width.Points())
	for i := 0; i < result.Frozen.LineCount(); i++ {
		line, _ := result.Frozen.Line(i)
		fit := "within capacity"
		switch line.Fit {
		case khipu.FitUnderfull:
			fit = "underfull"
		case khipu.FitOverfull:
			fit = "overfull"
		}
		source := result.ParagraphText[line.Source.Start:line.Source.End]
		fmt.Fprintf(&b, "Line %d · bytes [%d,%d) · %s\n%q\n", i+1, line.Source.Start, line.Source.End, fit, source)
		fmt.Fprintf(&b, "  actual %.2f / target %.2f bp · residual %+.2f bp\n  ratio %.3g · badness %d\n\n", line.Width.Points(), line.TargetWidth.Points(), line.Residual.Points(), line.Ratio, line.Badness)
	}
	if len(result.Fonts) > 0 {
		b.WriteString("OpenType faces (core/Hebrew)\n")
		for _, d := range result.FontDiagnostics {
			label := d.Stage
			if d.Substituted {
				label += " SUBSTITUTION"
			}
			fmt.Fprintf(&b, "Discovery %s · requested %q · %s: %s\n", label, d.Request.Pattern, d.Provider, d.Message)
		}
		for _, f := range result.Fonts {
			fmt.Fprintf(&b, "%q · %s %s\nSHA-256 %s\n", f.Path, f.Family, f.Subfamily, f.SHA256)
			for _, w := range f.Warnings {
				fmt.Fprintf(&b, "Warning: %s\n", w)
			}
		}
		for _, binding := range result.FontBindings {
			fmt.Fprintf(&b, "Coverage binding bytes [%d,%d) → %q · %.2f bp · %s\n", binding.Start, binding.End, binding.Path, binding.EmSize.Points(), binding.Script)
		}
		b.WriteString("SVG renders frozen glyph outlines with baselines, colors, and line geometry.\n\n")
	}
	if result.Input.Source == nil && result.Input.Text != result.ParagraphText {
		fmt.Fprintf(&b, "Original input: %q\nProcessed paragraph: %q\nPlain-text byte map (processed → original)\n", result.Input.Text, result.ParagraphText)
		for _, span := range result.TextSpans {
			fmt.Fprintf(&b, "[%d,%d) → [%d,%d) %s\n", span.Start, span.End, span.SourceStart, span.SourceEnd, span.Kind)
		}
	}
	b.WriteString("Pipeline stages\n")
	for _, d := range result.Diagnostics {
		fmt.Fprintf(&b, "  %s: %s\n", d.Stage, d.Message)
	}
	if source := result.Input.Source; source != nil {
		fmt.Fprintf(&b, "\nCSS style spans · base direction %s\n", source.Direction)
		for _, s := range source.Styles {
			fmt.Fprintf(&b, "[%d,%d): %s\n", s.Start, s.End, s.Style)
		}
		fmt.Fprintf(&b, "Logical bidi runs: %v\n", result.BidiRuns)
		if result.Input.Fonts == nil {
			b.WriteString("Synthetic face: family, weight, slant and color are retained metadata.\n")
		}
		fmt.Fprintf(&b, "\nDOM source map\nDocument %q\nSHA-256 %s\nSelected node %d: %q\nDecoded text-node bytes, not HTML file offsets.\n", source.DocumentName, source.DocumentID, source.SelectedID, source.SelectedPath)
		for _, s := range source.Spans {
			fmt.Fprintf(&b, "text [%d,%d) → node %d bytes [%d,%d) %s · %q\n", s.Start, s.End, s.NodeID, s.NodeStart, s.NodeEnd, s.Kind, s.NodePath)
		}
	}
	return b.String()
}
