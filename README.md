# tytui

An interactive paragraph workbench built with [Bubble Tea](https://charm.land/).
The pipeline supports synthetic and OpenType fonts, Latin/Hebrew mixed-direction
paragraphs, and firstfit or Knuth–Plass line breaking. The TUI displays text and
layout diagnostics; SVG exports render positioned glyph outlines for OpenType.

Run with Go 1.25.6 or newer and sibling `cords`, `khipu`, `uax`, `w3c-dom`, and `opentype` checkouts:

```sh
go run .
```

- Edit the paragraph, then press **Ctrl+R** to run layout.
- **F4** (or Ctrl+O) opens the HTML / DOM explorer. Enter a local filename and
  press Enter to load it. Try `testdata/html/paragraphs.html`.
- **F2** switches the breaker for the next run. The title shows that selection;
  each result identifies the breaker actually used.
- **F3** cycles backward through completed layouts retained for this session.
  Viewing a saved layout does not change the input controls.
- **Ctrl+S** exports the displayed layout as a uniquely named `paragraph-*.svg`
  in the current directory. The status line reports the filename or write error.
- **Tab / Shift+Tab** moves between text, width, and results.
- Width is in **bp** (big points, 1/72 inch), independently of terminal width.
  Decimal values are accepted. Press Enter in the width field to run.
- Enter in the text editor inserts a newline. Layout collapses whitespace,
  including blank lines, to spaces and trims paragraph edges. The editor retains
  the original input. Each run lays out one explicit paragraph; empty paragraphs
  are valid.
- With results focused, use arrows or Page Up / Page Down to scroll.
- **F1** toggles help; **Ctrl+C** quits. A terminal of at least 40 × 14 is needed.

Editing clears the displayed layout and cancels active work; completed layouts
remain available through F3. Run requests are serialized;
if a run is still stopping, only the latest requested input is queued. Completed
results for older input are discarded. Layout work runs outside the TUI update
loop, though the backend may finish a synchronous stage before noticing cancellation.

Results show logical source text, UTF-8 byte ranges, actual and target widths,
residuals, fit classification, ratio, badness, and completed pipeline stages.
Quotes expose whitespace in the source preview. Underfull and overfull lines can
be successful layouts with diagnostics.

Each run uses fresh encoder services and freezes before publishing its result.
Firstfit uses its ragged-right default profile; Knuth–Plass uses its own profile
with a finite-stretch right skip. Breaking and freezing always share the same
resolved geometry. Algorithm comparisons therefore also reflect these profiles.

SVG export uses that exact frozen result, without rerunning layout. It shows
positioned glyph outlines, colors, and line geometry for OpenType results, or
structural boxes and fit diagnostics for synthetic results. No font binaries are
embedded. Existing
files are never overwritten; failed exports remove their incomplete output.
History is held in memory for the session; export any proofs you want to keep.

## HTML and DOM selection

In the explorer, Tab / Shift+Tab cycles between the filename, DOM tree, and
property inspector. Up / Down moves through visible tree nodes; Left collapses
a node or moves to its parent; Right expands a node or enters its first child.
Home, End, and Page Up / Page Down navigate longer trees. With the inspector
focused, arrows and page keys scroll attributes, decoded text previews, library
property values, selection eligibility, and source spans. F4 or Escape returns
to the paragraph workbench without changing its input.

Press Enter on an eligible tree node to use its text as paragraph input, then
Ctrl+R to lay it out. File loading is asynchronous; an unsuccessful load keeps
the previous DOM, and changing the filename discards an outstanding load result.

Selections resolve inherited CSS styles into explicit UTF-8 byte spans. Only
white-space:normal is supported, with transformed DOM provenance retained.
Newline-preserving modes (pre, pre-line, pre-wrap, break-spaces) are rejected.
Bidi paragraphs honor base direction and supported inline isolates. Synthetic
layout uses size, language, and spacing; font family/weight/slant can select real faces through F5 mappings. Supported colors are rendered in OpenType proofs. Nested blocks, br/hr, images, and hidden
content remain unsupported. Text the editor cannot preserve is also rejected.
See [CSS extraction](doc/CSS_EXTRACTION.md) for the supported subset and limits.

Embedded and inline CSS feed the DOM inspector. Linked resources are not fetched.
The inspector exposes the library's property values, not browser-complete computed
CSS. It supplies explicit font inspection defaults, safely traverses inherited
property maps, and rejects unsupported CSS properties, invalid selectors, and
at-rules before invoking the current library. These restrictions make the
inspection subset explicit while the library's CSS support evolves. See
[DOM integration notes](doc/DOM_INTEGRATION.md) for scope and upstream findings.

Each HTML selection carries a document SHA-256, selected-node locator, and a map
from paragraph byte ranges to decoded bytes in DOM text nodes. These are **not
HTML-file byte offsets**. The result pane lists this map, and frozen history
retains independent copies. Width/breaker changes preserve provenance; editing
the extracted text detaches it. Reloading a document does not alter older results.

The [headless pipeline example](pipeline/example_test.go) and
[implementation plan](doc/IMPLEMENTATION_PLAN.md) describe the API and next steps.
`Result.ParagraphText` is the processed paragraph; frozen ranges, glyph clusters,
and font bindings address this text. For plain input, `Result.Input.Text` retains
the original and `Result.TextSpans` maps processed bytes back to it. HTML results
retain the DOM map in `Result.Input.Source.Spans`. See
[whitespace and provenance](doc/CSS_EXTRACTION.md#whitespace-and-provenance).

```sh
go test ./... -count=1
go build ./...
go vet ./...
```

## OpenType fonts

F5 opens font-file selection, plain-text size, and ordered style-to-font mappings.
Ctrl+S validates and applies the draft; F6 explicitly returns to synthetic shaping.
Parser errors and missing glyphs are reported without synthetic fallback. Frozen
results retain font identities, glyphs, and mapped byte spans. Try the sibling
`../opentype/testdata/fonts/GentiumPlus-R.ttf` with plain text such as `office`.
See [OpenType integration](doc/OPENTYPE_INTEGRATION.md) for mapping syntax,
headless use, diagnostics, fixture provenance, and current shaping limits.

## Multilingual glyph proofs

Step 7 adds Latin/Hebrew script itemization, whole-grapheme font fallback, and
soft-hyphen replacements. Add `fallback | * | * | /path/to/Hebrew.ttf` in F5.
OpenType SVG exports now render frozen outlines, colors, baselines, and line
geometry. Try `testdata/html/multilingual.html`; see
[policies and proof verification](doc/MULTILINGUAL_PROOFS.md).
