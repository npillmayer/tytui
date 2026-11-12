# tytui

An experimental typography integration laboratory, with an interactive paragraph
workbench built with [Bubble Tea](https://charm.land/). Tytui is a practical example
of progress in combining typographic functionality from other modules. Its purpose
is to explore their strengths, limitations, and interoperability, and to turn
findings into reproducible tests, clearer API boundaries, and upstream fixes.

Development is guided by integration experiments rather than an end-user
application roadmap. The TUI makes those experiments accessible: it lets us vary
inputs, compare results, inspect diagnostics, and export visual proofs. Reusable
functionality belongs in the participating libraries; tytui provides the wiring
and evidence needed to exercise and improve them.

The pipeline supports synthetic and OpenType fonts, Latin/Hebrew mixed-direction
paragraphs, and firstfit or Knuth–Plass line breaking. The TUI displays text and
layout diagnostics; SVG exports render positioned glyph outlines for OpenType.

Run with Go 1.25.6 or newer and sibling `cords`, `khipu`, `uax`, `w3c-dom`, `opentype`, `tree`, `schuko`, `fontfind`, and `typography` checkouts:

```sh
go run .
```

Cords, tree, w3c-dom, and tytui each replace schuko with `../schuko` to use
the local tracing race fixes. Keep these checkouts as siblings. Main-module
replacements do not propagate from dependencies, so tytui retains its own
replacement. Cords can run `go test -race ./styled/... -count=1` directly.

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

Headless offline fonts can also use immutable `fontresource.Resource` snapshots
through `Settings.DefaultResource`, `Rule.Resource`, and `FallbackResources`.
Existing F5 file selection remains unchanged. The local dependency layout now
also requires `../fontfind`; see [offline resources](doc/OPENTYPE_INTEGRATION.md).

In F5, Ctrl+D switches file paths to controlled offline requests such as
`Gentium Plus ; 400 ; normal`. The packaged catalog currently contains Gentium
Plus Regular only; use file mode for other fonts. Ctrl+T opts into explicit
substitution. Ctrl+P previews requested/resolved faces and diagnostics; Ctrl+S
applies a successful preview. Ctrl+E edits again; arrows/Page keys scroll details.
See [font requests](doc/OPENTYPE_INTEGRATION.md) for mapping syntax and limits.

Headless reproducible cases use `replay.Save`/`Load` with pinned font hashes and a
local content store; see [offline replay](doc/OFFLINE_REPLAY.md). TUI Ctrl+S remains SVG.

Shared styles now live in `github.com/npillmayer/typography/style` (local
`../typography`). DOM extraction and headless input use `style.Style` and
`style.Span` directly; the former htmlinput aliases have been removed.
Headless callers may supply `pipeline.Input.Styles` without a DOM: spans cover
processed paragraph bytes after `paragraphtext.Normalize`, not original source
bytes. `Input.Source` and `Input.Styles` are mutually exclusive; `TextSpans` retains
the plain-source mapping. See [typography contracts](doc/TYPOGRAPHY_API_CONTRACTS.md).

Immutable resource storage and the OpenType backend now live in
`typography/fontresource` and `typography/adapter/opentype`. The workbench
`fontbackend` forwarding package has been removed. Parser, identity, fallback and proof
behavior are unchanged. The backend-independent paragraph engine is
`typography/paragraph.Run`; pipeline.Run wires the selected backend and prepares
its input. The former `pipeline/internal/layout` package is removed.

The typography resolver is now extracted too: mapping, itemization, coverage policy
and pinned snapshots live in the root `typography` package behind explicit loader
and coverage services. Application wiring supplies `opentype.Services` explicitly.
`pipeline.Input.Fonts` accepts `*discovery.Resolution`, embedding `typography.Settings`
with request/policy diagnostics. `Result.FontSnapshot` retains pinned settings;
request metadata remains in the detached input. No generic constructor selects a backend.

Discovery is split between neutral `typography/discovery` session policy and the
controlled `typography/adapter/fontfind` provider adapter. F5's offline catalog is
unchanged. Frozen proof and replay consumers now use injected rendering/identity
services; the OpenType adapter owns outlines and visual glyph ordering. Version-1
and version-2 replay compatibility is verified against static pre-migration manifests.
Explicit serialization records in `replay/wire.go` preserve the existing wire format.

New layouts use glyph-boundary colors: paint changes preserve shaping, ligatures
remain one color, and independently anchored marks may have their own color.
Original paint spans are retained. New replay cases use version 2; version-1 cases
retain their original shaping and paint behavior rather than silently changing.

Color now bypasses the paragraph typesetter. M's `typography.ProjectStyles` owns
the projection into color-free typesetter styles; the renderer receives a separate
snapshot of logical color spans. `Result.PaintSpans()` returns a detached copy.
Document colors remain in the existing inspection/replay fields, so the replay
formats and glyph-color policy are unchanged.

The headless `typography/paragraph.Run` API consumes prepared `styled.Text`, base
direction and zero-byte bidi markup, with explicit width/breaker/assembly parameters
and injected `khipu.Config` services. It prepares bidi levels, encodes, breaks,
freezes and assembles a boxed vertical list. `paragraph.Assemble` remains available
for callers that already have frozen lines. Its compatibility
policy preserves proof geometry. Completed results expose detached placements via
`Result.VList()`; glyph rendering consumes those placements without remeasuring
line metrics. Colors remain separate, and replay versions 1–2 rebuild placements
without new wire fields. See the
[paragraph-engine plan](doc/PARAGRAPH_ENGINE_PLAN.md).


Headless callers can opt into CSS-inspired line boxes with
`pipeline.Input.LineLayout = &paragraph.LineLayout{Default:
paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}}`.
The policy uses a font/size strut, half-leading and natural stacking without a
trailing gap. Normal and resolved-length values are also supported; optional
byte ranges override line-height without splitting shaping or carrying paint.
TUI defaults remain unchanged. CSS declaration extraction and the CSS box-engine
bridge remain future work. Explicit line-height cases use replay version 3 with
placement evidence; compatibility layouts continue to save as version 1 or 2.
