# Styled paragraph extraction (step 5)

Later steps add real fonts and colored glyph proofs; see
[Multilingual proofs](MULTILINGUAL_PROOFS.md) for current rendering and fallback policies.

`Document.Extract` resolves inherited style values and processes whitespace before
constructing explicit half-open UTF-8 byte spans. `Extraction.StyledText` applies
these spans using assigned `Text.Style` results. It avoids the cords builder,
whose convenience API issues are still outstanding. Styles are immutable values;
result snapshots clone source, style, and isolate slices.

Supported properties are family, size, weight, slant, color, language (HTML lang),
direction, white-space, and normal/isolate unicode-bidi. Relative sizes resolve
against each element's parent; rem uses the HTML root. CSS px is 0.75 big points,
CSS pt is one big point. Supported size units are px, pt, em, rem, and percent,
plus standard size keywords. Initial medium is 16px. CSS inherit, initial, and
unset are supported. Basic bold/italic tags supply defaults. Invalid or
unsupported values on the selected subtree or its ancestors return errors.

The synthetic resolver uses size, language (with inferred script), and spacing.
With the synthetic backend, family, weight, slant, and color remain metadata.
Step 6 adds explicit OpenType font mappings; see OPENTYPE_INTEGRATION.md. Color
remains metadata and SVG remains a structural proof. Script inference from language is not full script itemization.
The explorer and result pane expose resolved spans and logical bidi runs.

## Whitespace and provenance

- Only normal is supported. Whitespace runs collapse to one space across
  text-node boundaries, and paragraph edges are trimmed.
- CRLF, CR, LF, tabs, form feeds, vertical tabs, NEL (U+0085), and Unicode line
  and paragraph separators (U+2028/U+2029) participate in this collapse. Blank
  lines do not create paragraphs or force line breaks. This is an explicit
  application policy, not a claim of browser-exact CSS whitespace processing.
- pre, pre-line, pre-wrap, break-spaces, and nowrap are rejected, including
  inherited modes and empty selected elements. They are never silently
  reinterpreted as normal.
- NBSP is never collapsed. HTML parsing decodes entities and normalizes literal
  CR/LF before extraction; no Unicode normalization is applied.

One selected DOM subtree or one plain-text pipeline invocation defines a
paragraph. Nested block elements and br/hr remain unsupported; a future forced
line break needs an explicit representation distinct from source newlines.
Whitespace normalization happens before styled spans, bidi construction,
font itemization, and shaping, with the same policy for both backends and breakers.

Source records have identity (empty Kind), collapsed, or omitted kinds. Several
nodes can contribute to the same collapsed output space. Omitted records retain
source byte intervals with zero output length. The first contributing whitespace
unit supplies the collapsed space's style. Style spans cover processed text;
DOM offsets still address decoded text nodes, never original HTML file offsets.

Plain input retains its original bytes in `Result.Input.Text`; the editor is not
rewritten. `Result.ParagraphText` and `Result.Text` hold the processed paragraph.
`Result.TextSpans` maps processed byte intervals to original input byte intervals,
including omitted edge whitespace. HTML uses its existing `Source.Spans` instead;
it is validated as already normalized, never normalized again over existing spans.
Frozen byte ranges, glyph scalar clusters, bidi runs, and font bindings refer to
the processed paragraph. The result pane shows that paragraph and its provenance.

Replaced content and hidden content remain unsupported. Khipu determines final
line-edge glue behavior; this subset does not implement preserved spaces,
tab stops, or blank-line layout. Encoding failures remain visible.

## Bidi and cascade boundaries

Base direction comes from inherited direction or HTML dir. dir=auto uses a first
strong character scan. Inline dir and bdi/unicode-bidi:isolate create zero-byte
LRI/RLI/PDI markup; source byte coordinates do not shift. Nested isolates with
distinct boundaries work; coincident nested opens/closes exceed the UAX callback
capacity and return errors. Overrides and embeddings are unsupported.

Embedded and inline declarations still use the sibling CSSOM. !important and
repeated properties within a declaration block are rejected until upstream
parsing/priority handling is reliable. The sibling CSSOM now compares parsed
specificity tuples, then source order on ties; selector lists use only matching
branches. This fixes issue 5 in TODO.md without extending the supported selector
syntax or establishing full browser CSS conformance. Linked sheets and at-rules
remain unsupported. Raw property inspection and resolved extraction styles are shown
separately because relative values differ from computed values.

## Required upstream changes and validation

The sibling cords fix in styled/run_ops.go guards merges at text boundaries;
style_boundaries_test.go covers consecutive multibyte spans, whole-text styling,
and persistence. This fixes a panic reproduced by the paragraph fixture.
The full cords styled suite still has an existing TestStyleNoOpOnEmptySpan failure
(empty spans return ErrIllegalArguments); this adapter emits no empty style spans.

In w3c-dom, CSSOM now skips a duplicate winning property without stopping all
remaining properties, and inline declarations take priority over stylesheet
selectors. cascade_selection_test.go covers these cases. The step 4 local-group
fix and local_declaration_test.go are also required. No dependency versions were
changed; local replacements use these sibling working trees.

Open `testdata/html/styles.html` in the F4 explorer for nested, RTL, and multiline-source
examples.

htmlinput/styles_test.go covers inheritance, relative units, whitespace,
multibyte maps, explicit spans, and bidi metadata. pipeline/styles_test.go checks
both breakers, frozen sizes, mixed bidi runs, collapsed newlines, empty extracted
paragraphs, and detached snapshots. Existing workbench tests cover selection,
text-edit detachment, and frozen history.
`paragraphtext/normalize_test.go` and `pipeline/newlines_test.go` cover CR/LF,
blank lines, multibyte offsets, NBSP, omitted whitespace, and frozen source text
under both breakers with synthetic and OpenType fonts.
