# Document pipeline whitespace implementation plan

Status: **implementation deferred**. This document records the proposed work;
API names below are proposals, not implemented interfaces.

## Objective and current boundary

Support the remaining CSS `white-space` modes in the document pipeline. Enable
each mode only after transformation, breaking, intrinsic measurement and freezing
agree. This requires changes in khipu and typography as well as CSS core;
extraction changes alone would produce incorrect wrapping and sizing.

The current CSS core has typed `layout.WhiteSpace` values and readable text input
through `BuildOptions.ReadText`. It derives anonymous-block suppression from text,
computed whitespace mode and inline edges. Document extraction currently supports
`normal`; typography's CSSLineHeight assembly suppresses lines for empty processed
text. The standalone paragraph workbench retains its separate behavior.

## Target behavior

| Mode | Segment breaks | Spaces/tabs | Soft wrapping | Trailing spaces |
| --- | --- | --- | --- | --- |
| `normal` | Collapse | Collapse | Yes | Remove |
| `nowrap` | Collapse | Collapse | No | Remove |
| `pre-line` | Preserve | Collapse | Yes | Remove |
| `pre` | Preserve | Preserve | No | Preserve |
| `pre-wrap` | Preserve | Preserve | Yes | Hang under CSS rules |
| `break-spaces` | Preserve | Preserve | Yes, including after preserved spaces | Retain; participate in wrapping |

Reference: [CSS Text white-space property](https://www.w3.org/TR/css-text-3/#white-space-property).
In particular, `pre-wrap` and `break-spaces` need different fitting and
intrinsic-sizing behavior.

## Stage 1: Computed properties and inline whitespace input

Keep `layout.WhiteSpace`, but replace the document pipeline's hard-coded `normal`
projection with explicit mapping of all six computed values.

Proposed CSS-core types:

- `text.WhiteSpaceRun`: source byte range and computed mode.
- `text.InlineInput`: text runs, inline ancestry and atomic boundaries.
- `layout.TabSize`: a tagged nonnegative number or resolved length.

Preserve ancestry: wrapping at a boundary can depend on the nearest common
ancestor's `white-space`, rather than either adjacent text run alone. See
[CSS Text line-breaking details](https://www.w3.org/TR/css-text-3/#line-break-details).

In w3c-dom/htmlinput, validate accepted values, inheritance, `initial` and `unset`;
add `tab-size` resolution and its default. Audit `<pre>` defaults so author CSS can
override them normally. Separate style resolution from the standalone paragraph
extractor's supported-mode checks.

Required tests:

- All keywords, inherited and overridden modes.
- Invalid declarations and cascade fallback.
- `<pre>` defaults and author overrides.
- Mixed nested modes.
- `tab-size` numbers, lengths, zero and invalid values.

## Stage 2: Two-phase CSS text processing

Replace whole-group normalization with a proposed
`text.PrepareInline(InlineInput) (ProcessedInline, error)` API in css-box-layout's
text package. Its result should contain:

- Processed text and complete source-byte mappings.
- Preserved segment-break and tab occurrences.
- Whitespace behavior ranges.
- Derived wrapping boundaries and the structural information needed to resolve
  them.

Collapse across text-node and inline-element boundaries where required. Preserve
boundaries around atomic content. Do not independently trim every style run.

Move line-dependent trimming and trailing-space handling to line selection;
extraction cannot decide those correctly before the breaks are known. Follow
[CSS Text's processing phases](https://www.w3.org/TR/css-text-3/#white-space-rules).
Keep `NormalizeNormal` as a convenience only where its contract remains valid.

Required tests:

- Adjacent DOM nodes and nested mode changes.
- Leading/trailing whitespace and CRLF/CR/LF preprocessing.
- Whitespace surrounding preserved breaks.
- NBSP, Unicode separators and atomic boundaries.
- Complete UTF-8 provenance, including omitted bytes and transformed newlines.

## Stage 3: Neutral breaking and whitespace primitives in khipu

Extend `PrepareOptions` beyond its current atom-only boundary overrides. Proposed
additions describe typography operations without importing CSS:

- General source-byte boundary constraints: allow, prohibit and force.
- Preserved-space behavior, distinct from ordinary discardable glue.
- Explicit forced-break occurrences with source provenance.
- Line-edge trimming and hanging behavior translated from CSS by the integration
  layer.

Preparation must validate boundaries against UTF-8, shaping clusters and atomic
occurrences. Forced breaks need an explicit representation even when a newline
produces no glyph. Preserve the prepare → measure → bind → break → freeze lifecycle.

Update both firstfit and Knuth–Plass. Disabling soft wrapping must also apply
during intrinsic measurement, not just final breaking.

Required tests:

- `nowrap` overflow without discretionary wrapping.
- Mandatory breaks under both breakers, including consecutive, leading and
  terminal breaks.
- Breaks beside atomics and invalid cluster boundaries.
- Preserved spaces surviving freeze.
- Consistent intrinsic and final widths.

Delivery gate: enable **`nowrap` and `pre-line`** once document integration passes.

## Stage 4: Preserved tabs and position-dependent advances

Tabs need actual tab stops, not replacement with a fixed number of spaces. CSS
defines their positioning relative to the containing block and resolved
`tab-size`: [CSS Text tab sizing](https://www.w3.org/TR/css-text-3/#tab-size-property).

Add a neutral tab occurrence to khipu's prepared material. Typography resolves
font-dependent tab intervals into native units. Introduce one shared
candidate-line advance evaluator used by:

- Firstfit.
- Knuth–Plass.
- Intrinsic measurement.
- Final freezing.

This is the main algorithmic risk: tab widths depend on their position in a
candidate line. Adapter-side repeated layouts would bypass the intended
measurement architecture.

Required tests:

- Tabs at line start and after text; exact and near tab stops.
- Consecutive tabs; zero and fractional sizes; changed font sizes.
- Line wrapping before tabs and RTL placement.
- Intrinsic sizing inside fixed and auto inline-blocks.

Delivery gate: enable **`pre`** after forced breaks, preserved spaces and tabs are
complete.

## Stage 5: Preserved-space wrapping, hanging and empty-line assembly

Implement the remaining `pre-wrap` and `break-spaces` rules. Extend frozen line
diagnostics to distinguish actual advances, width used for fitting and hanging
extents where necessary. Rendering must consume these retained values without
recomputation.

Replace the current empty-source special case with line-level content accounting
in CSS assembly. Distinguish:

- An empty paragraph with no line.
- A blank line caused by a preserved break.
- A line containing preserved spaces or tabs.
- A line containing atomic content.

Do not derive blank-line counts with `strings.Split`; terminal breaks and
consecutive breaks need explicit semantics. Check bidi paragraph boundaries and
line reordering independently.

Required tests:

- Whitespace-only content in every mode; repeated and terminal newlines.
- Trailing-space hanging and a space sequence wider than the container.
- Opportunities between preserved spaces.
- Empty versus zero-width atomic content.
- Line heights and margin-collapse permission.

Delivery gate: enable **`pre-wrap` and `break-spaces`** when fitting, intrinsic
measurement, freezing and assembly agree.

## Stage 6: Document integration and conformance controls

Carry the processed whitespace plan through `InlineGroup`, typography preparation,
intrinsic measurement and final layout. Keep CSS policy out of font/shaping styles.
Update retained diagnostics and SVG bounds for preserved and hanging whitespace.

Run a shared fixture matrix through both breakers:

- All six modes at wide, narrow and zero widths.
- Mixed modes within one paragraph.
- Nested auto inline-blocks and flow-roots.
- Latin/Hebrew text, combining marks and source mappings.
- Resize, cancellation, failure isolation and repeated export.
- Empty-line effects on parent/child and sibling margin collapse.

Use deterministic font fixtures and numeric geometry assertions, supplemented by
relevant Web Platform Tests and browser reference measurements. Update old
expectations when they conflict with the rules. Complete ordinary and race suites,
builds and vet checks in affected modules with the local dependency wiring.

## Delivery order and scope

1. Property/input model.
2. Text processing.
3. `nowrap` and `pre-line`.
4. Tabs and `pre`.
5. `pre-wrap` and `break-spaces`.
6. Full integration audit.

`<br>`, richer inline decoration and additional CSS Text features remain separately
scoped unless explicitly added. Preserved-newline support should make those later
extensions easier. Unsupported behavior must remain explicit until its delivery
gate is satisfied.

Implementation is deferred; this plan does not authorize starting these stages.
