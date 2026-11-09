# TUI and paragraph pipeline implementation plan

Build the integration laboratory described in [AGENTS.md](../AGENTS.md) through
working vertical slices. Start with synthetic paragraph layout, then add DOM/CSS
extraction and real fonts. Each slice should work through the TUI and through a
headless test, so interactive findings can become library regression cases.

## 1. Establish the pipeline and dependency setup

- Wire the sibling modules, including `uax`, using a compatible Go toolchain.
- Separate pipeline operations from the Bubble Tea model.
- Define input configuration and a result containing stage diagnostics,
  extracted text/styles, and frozen layout.

Completion criterion: a headless test encodes plain text with khipu's synthetic
backend, breaks it, and freezes the result.

Implemented in [`pipeline`](../pipeline/pipeline.go). `pipeline.Run` accepts
plain text, a positive finite width in `khipu/dimen.DU`, and a base direction
(LTR by default). It returns extracted `styled.Text`, source byte endpoints,
stage diagnostics, and frozen layout. Failures return a `StageError` and no
partial result. Each call owns fresh synthetic services. See the
[executable example](../pipeline/example_test.go).

The module requires Go 1.25.6 and uses relative `go.mod` replacements for sibling
cords, khipu, and uax checkouts. OpenType and w3c-dom replacements are also present
for later integration, but those modules are not yet required or imported.
Run `go test ./pipeline -count=1` for the headless integration checks.

## 2. Build a minimal paragraph workbench

- Replace the demo interaction with plain-text input, paragraph width, a run
  action, and a result pane.
- Show lines, source ranges, widths, and fit diagnostics.
- Run pipeline work through Bubble Tea commands, serialize owner operations,
  and discard stale results after input changes.

Completion criterion: users can change text or width and inspect a fresh layout
without blocking the interface.

Implemented in `main.go`, `help.go`, and `info_view.go`. Ctrl+R runs the headless
pipeline through a Bubble Tea command. Tab cycles text, width (in big points),
and scrollable results. Editing invalidates old output and cancels active work;
requests are serialized and only the latest pending request is retained.
Revision and run IDs prevent stale completions from replacing current output.
Results include logical source previews, byte ranges, line geometry, fit
diagnostics, and pipeline stages. See [README.md](../README.md) for controls.

## 3. Add structural visual output

- Export frozen results with `debug.WriteFrozenSVG`.
- Add firstfit/Knuth–Plass selection.
- Preserve each frozen result before running another breaker, which may change
  selected discretionary variants on the elastic input.

Completion criterion: TUI diagnostics and exported SVG describe the same layout.
Actual glyph rendering remains a later decision.

Implemented: `pipeline.Input.Breaker` selects `FirstFit` (the default) or
`KnuthPlass`, each with its own resolved default geometry used for both breaking
and freezing. Every run has a fresh owner and publishes only after freezing.
`pipeline.WriteSVG` exports a completed result through `debug.WriteFrozenSVG`.
The TUI uses F2 to switch breakers, F3 to browse retained completed results, and
Ctrl+S to export the displayed snapshot to a unique `paragraph-*.svg` file.
Export captures that result even if another layout completes before writing.
Tests check SVG geometry/source diagnostics, retained-result stability, selection
during active runs, write failures, and file preservation.

## 4. Add HTML loading and DOM selection

- Parse complete HTML documents and construct the styled DOM.
- Present a navigable tree with text previews and property inspection.
- Initially support selecting a subtree containing inline text; report nested
  blocks and unsupported mandatory breaks clearly.
- Preserve a mapping from extracted text ranges to DOM nodes.

Completion criterion: selecting a node produces reproducible extracted text and
a DOM-to-text source map.

Implemented in `htmlinput/` and `browser.go`. F4 opens an asynchronous local-file
loader, navigable DOM tree, and scrollable property/text inspector. Enter selects
an eligible inline-text subtree and retains detached node/byte provenance with
pipeline results. Unsupported subtree structures and mandatory breaks report
explicit errors. Step 5 adds CSS translation as described below.
See [DOM integration notes](DOM_INTEGRATION.md) for the extraction policy,
stylesheet subset, and the required local w3c-dom declaration-preservation fix.

## 5. Preserve CSS styles through paragraph extraction

- Introduce an immutable application style implementing `styled.Style`.
- Define a small supported subset: font family, size, weight/style, language,
  direction, and whitespace handling. Retain color as display metadata.
- Build `styled.Text` with explicit byte spans and construct the bidi paragraph.
- Resolve the text-builder issues below before relying on its convenience APIs.

Completion criterion: fixtures verify nested styles, inherited properties,
whitespace, and multibyte source mappings.

Implemented in `htmlinput/styles.go`, `htmlinput/extraction.go`, and the pipeline
style resolver. Extraction retains computed style spans, transformed byte maps,
and bidi isolates; the synthetic pipeline applies size/language/spacing and
exposes logical bidi runs. Both breakers are covered by frozen-output tests.
See [CSS extraction](CSS_EXTRACTION.md) for supported values, focused sibling
cords/CSSOM fixes, and remaining upstream limitations.

## 6. Integrate OpenType fonts and shaping

- Implement and test `FontLoader`, `FontFace`, `FontScale`, and
  `StreamingShaper` adapters independently of the UI.
- Start with explicit font-file selection and one Latin run.
- Add a style-to-font mapping editor and multiple styled runs.
- Expose parser errors and missing glyphs instead of silently substituting
  synthetic output.

Completion criterion: real-font paragraphs preserve glyph IDs, cluster anchors,
final advances, and source correspondence through freezing.

Implemented in `fontbackend/`, the headless pipeline, and `fonts.go`. F5 selects
local files and edits ordered family/weight/slant mappings; F6 selects synthetic.
Real runs retain fresh parsed faces, font identities, shaped glyphs, and mappings
through freezing. Parser/coverage/shaping failures are explicit and never trigger
synthetic fallback. See [OpenType integration](OPENTYPE_INTEGRATION.md) for the
supported subset, diagnostics, lifecycle, and independent reference tests.

## 7. Expand multilingual coverage and visual proofs

- Add mixed LTR/RTL cases, Hebrew engine selection, combining marks, ligatures,
  and discretionary breaks.
- Decide explicit policies for script itemization and font fallback.
- Choose a glyph renderer using frozen output, including baseline placement
  and colors.

Completion criterion: structural diagnostics and positioned glyph proofs agree
for a small, independently verified fixture set.

## Integration gaps and upstream priorities

Keep upstream work narrowly tied to the slices that need it. The source concerns
recorded in AGENTS.md require focused reproductions before treating them as
test-confirmed defects.

| Priority | Issue or gap | Action |
| --- | --- | --- |
| First | Styled builder appears to lose styles and length bookkeeping | Reproduce `AppendString` and `Text()` behavior in cords tests; fix upstream before adopting the builder. Explicit assigned `Text.Style` calls can support early work. |
| First | DOM helper nil accesses and untrimmed `Children()` results | Add focused reproductions for empty style elements, incomplete trees, and mixed element/text children; fix confirmed defects before exposing unrestricted DOM exploration. |
| Early | CSS precedence and error reporting | Test embedded, supplied, and inline styles against the intended subset. Prefer the error-returning CSSOM path for integration. |
| Implemented in step 6 | OpenType-to-khipu adapter | `fontbackend` supplies local loading, mapping, metrics, scaling, and streaming with regression coverage. |
| Before real-font rollout | Streaming lifecycle and coordinate conversion | Test backpressure, cancellation, terminal errors, scalar anchors, and font-unit scaling. These errors can corrupt plausible-looking output. |
| As encountered | OpenType parser and shaping limitations | Select compatible baseline fonts and report unsupported ones. Turn observed failures into focused upstream cases; avoid a broad shaping repair campaign. |

## Reproducibility and validation

Save reproducible cases containing HTML/CSS or plain-text input, the selected
node, extracted text and styles, font identity, style mapping, width and units,
breaker configuration, and diagnostics. Preserve font provenance and licenses
for checked-in fixtures.

Keep synthetic tests separate from real-font tests. Verify source/cluster
correspondence and geometry as well as visual output. Use independent shaping
expectations, following the existing OpenType fixture workflow. Add focused
tests in the owning package for upstream fixes and integration tests here for
the boundaries between packages.

## First milestone

Deliver plain text → synthetic khipu → frozen lines → TUI diagnostics and SVG,
alongside focused reproductions of the cords builder defects. This establishes
the application structure without making progress depend on CSS or real-font
shaping correctness.

Steps 1–6 are implemented. The later adapters and final glyph rendering remain
implementation work. The separate cords builder investigations remain outstanding.
