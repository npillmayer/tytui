# tytui

The headless [documentlayout package](documentlayout/README.md) connects
CSS-normalized inline groups to prepared paragraph layout, including fixed and auto-width
atomic boxes and retained source/paint correspondence (pipeline integration
stages 3D–3F and 4C–4D, verified through stage 5). Intrinsic callbacks reuse preparation while CSS core selects
auto widths. `Document.Layout` publishes CSS geometry and typed paragraph results
together after success. [Retained fixed/auto document proofs](documentlayout/testdata/README.md)
show nested boxes, baselines and real glyphs without remeasurement. Independent
geometry, resize/failure recovery and result lifetime are verified under both
breakers; broader CSS policies remain explicitly deferred.
The TUI exposes this path through **F7 document mode**, alongside the existing
paragraph workbench. Paragraph replay formats and fixtures remain unchanged.
See the [2026-11-15 integration review](doc/INTEGRATION_REVIEW_2026-11-15.md)
for the current boundaries, confirmed gaps and proposed next document experiments.

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

Run with Go 1.25.6 or newer and sibling `cords`, `khipu`, `uax`, `w3c-dom`, `opentype`, `tree`, `schuko`, `fontfind`, `typography`, and `css-box-layout` checkouts:

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
- **F7** switches between paragraph mode and the document experiment described
  below, including selection mode inside the DOM explorer.
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

In paragraph mode, editing clears the displayed layout and cancels active work; completed layouts
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

## Document experiment

Press **F7** for document mode. Its width, breaker, selection and result history
are separate from paragraph controls. The F5 font selection is shared; each
completed result retains its own faces and rendering services. This first document
TUI experiment requires OpenType fonts; F6 keeps its existing synthetic behavior
in paragraph mode. No synthetic substitution is made for document failures.

To reproduce the nested auto-width experiment:

1. Open **F5**. Set the default file to
   `../opentype/testdata/fonts/GentiumPlus-R.ttf` and add the mapping row
   `fallback | * | * | ../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf`.
   Press Ctrl+S to validate and apply. The controlled offline request/resource
   mode also works, subject to the selected fonts' coverage.
2. Open **F4**, load `documentlayout/testdata/nested-auto.html`, select the
   `div` with `id="root"`, then press Enter. This selects a read-only block subtree,
   including nested blocks, flow-roots and inline-blocks. Both `display:block`
   and `display:flow-root` elements are document-root candidates, including
   equivalent multi-keyword CSS spellings. Ctrl+R performs full profile
   validation and layout asynchronously; the browser only checks root candidacy.
3. Run with **420 CSS px**, then **100 CSS px**. Enter from the width field also
   runs. CSS pixels differ from paragraph mode's big points; zero is permitted.
   Terminal resizing only changes the viewport, not the layout constraint.
4. Use **F2** to change breaker and **F3** to browse completed document results.
   The inspector identifies each snapshot's width/breaker independently of the
   current controls. Tab switches between width and results; arrows/page keys
   scroll the retained DOM → BoxID → group/owner → line/atomic associations.
5. **Ctrl+S** exports the displayed snapshot to a unique `document-*.svg`.
   The export captures its result and font services at request time. It does not
   reopen sources/fonts, shape, measure or lay out again.

Document edits, reruns and failures retain the previous completed result for
inspection/export. Failures appear above that result. Runs in both modes share
one serialized scheduler with only the latest pending request; edits, selection,
font/breaker changes and mode switches cancel stale work. F7 restores paragraph
text, source mapping, width, breaker and retained paragraph results. Reloaded DOMs
become input only after selecting a node again.

The inspector shows the bounded profile, box geometry in CSS pixels, paragraph
placements in big points, overfull/underfull diagnostics, decoded byte mappings,
separate paint, retained HTML, font identities and ordered mapping configuration.
The profile remains normal line-height, NoHyphenation, bottom-margin-edge atomic
alignment and zero side skips. Authored soft hyphens under CSS manual are rejected;
`hyphens:none` explicitly disables them. See the
[document subset](documentlayout/README.md) for the other limits.

Try `documentlayout/testdata/rectangle.html` for explicitly sized non-text leaves.
Marked diagnostic images become rectangles through the existing atomic boundary;
local PNG/JPEG images use `data-tytui-leaf="image" src="images/photo.png"` with
explicit CSS width/height. Relative paths use the HTML file directory. Image bytes
are retained and embedded in SVG, so deleting a file cannot change earlier proofs.
See the document subset for format, size and sizing limits.

These are diagnostic document proofs, not pagination or full CSS painting.
Document replay remains deferred; Ctrl+S exports SVG, and the existing headless
paragraph replay uses experimental version 4. The
[TUI integration tests](document_workbench_test.go) reproduce all four existing
[nested-auto proofs](documentlayout/testdata/README.md) byte-for-byte.

## HTML and DOM selection

In the explorer, Tab / Shift+Tab cycles between the filename, DOM tree, and
property inspector. Up / Down moves through visible tree nodes; Left collapses
a node or moves to its parent; Right expands a node or enters its first child.
Home, End, and Page Up / Page Down navigate longer trees. With the inspector
focused, arrows and page keys scroll attributes, decoded text previews, library
property values, selection eligibility, and source spans. F4 or Escape returns
to the current workbench without changing its input. F7 switches the selection
mode; the following paragraph-specific rules apply in paragraph mode.

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
`typography/fontresource` and the separate `typography/glyphing/adapter/opentype`
and `typography/render/adapter/opentype` packages. The workbench
`fontbackend` forwarding package has been removed. Parser, identity, fallback and proof
behavior are unchanged. The backend-independent paragraph engine is
`typography/glyphing/paragraph.Run`; pipeline.Run wires the selected backend and prepares
its input. The former `pipeline/internal/layout` package is removed.

The `glyphing.StyleMapper` in `typography/glyphing` receives already loaded fonts: `SelectedFonts`
contains a closed table of `FaceEntry` values plus default, style-rule and ordered
fallback references. `glyphing.NewStyleMapper(ctx, fonts, coverage)` validates that table and
performs no loading or discovery. OpenType parsing lives in `typography/glyphing/adapter/opentype`; retained rendering
services live in `typography/render/adapter/opentype`. Completed geometry is shared
through `typography/layout`; neither branch imports the other. Itemization records
selected fonts and processed-text byte ranges as `glyphing.FontTextRun` values;
`pipeline.Result.FontBindings` retains these runs. See the
[current API names and ownership](doc/TYPOGRAPHY_PACKAGES.md).

[`fontselection`](fontselection/README.md) owns application requests, controlled
fontfind providers, generic mappings, substitution policy and session caches.
`Prepare(ctx, request)` loads explicit selections before layout. `pipeline.Input.Fonts`
accepts its `*Selection`; nil selects synthetic. `Result.FontSnapshot` contains
`*glyphing.SelectedFonts`, and `FontProvenance()` returns detached application
evidence. The TUI keeps editable requests and prepares fresh faces per run, so
previewing a file does not pin that pathname forever.

```go
selected, err := fontselection.Prepare(ctx, &fontselection.Request{
    Settings: fontselection.Settings{DefaultPath: path, Size: 12 * dimen.BP},
})
if err != nil { return err }
result, err := pipeline.Run(ctx, pipeline.Input{
    Text: "office", Width: 200 * dimen.BP, Fonts: selected,
})
```

This replaces `discovery.Resolution` and root `LoadSettings`/`Settings`; typography's
old discovery/fontfind packages are removed. Replay's explicit input, policy,
descriptor and font records in `replay/wire.go` describe version 4.
Replay uses retained bytes and stored origins; requested paths and descriptors
never trigger discovery. Historical fixtures remain unchanged as rejection cases.

New layouts use glyph-boundary colors: paint changes preserve shaping, ligatures
remain one color, and independently anchored marks may have their own color.
Original paint spans are retained. Replay accepts only version 4; it never silently reinterprets historical cases.
Legacy shaping/color selectors have been removed from typography and pipeline.

Color now bypasses the paragraph typesetter. M's `glyphing.ProjectStyles` owns
the projection into color-free typesetter styles; the renderer receives a separate
snapshot of logical color spans. `Result.PaintSpans()` returns a detached copy.
Document colors remain in the existing inspection/replay fields, so the replay
format retains source colors independently of glyph-color assignment.

The headless `typography/glyphing/paragraph.Run` API consumes prepared `styled.Text`, base
direction and zero-byte bidi markup, with explicit width/breaker/assembly parameters
and injected `khipu.Config` services. It prepares bidi levels, encodes, breaks,
freezes and assembles a boxed vertical list. `paragraph.Assemble` remains available
for callers that already have frozen lines. Its compatibility
policy preserves proof geometry. Completed results expose detached placements via
`Result.VList()`; glyph rendering consumes those placements without remeasuring
line metrics. Colors remain separate, and replay version 4 verifies rebuilt
placements for both assembly modes. See the
[paragraph-engine plan](doc/PARAGRAPH_ENGINE_PLAN.md).


Headless callers can opt into CSS-inspired line boxes with
`pipeline.Input.LineLayout = &paragraph.LineLayout{Default:
paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5}}`.
The policy uses a font/size strut, half-leading and natural stacking without a
trailing gap. Normal and resolved-length values are also supported; optional
byte ranges override line-height without splitting shaping or carrying paint.
TUI defaults remain unchanged. The separate `documentlayout` CSS box-engine bridge
currently supports normal line height; other CSS line-height declarations remain
unsupported there. Both explicit single-paragraph line-height and compatibility
layouts use replay version 4 with explicit assembly mode and placement evidence.

## CLI box-layout companion

[`tycli`](tycli/README.md) runs the headless document pipeline with HTML/CSS,
explicit YAML font mappings, CSS-pixel width and selectable diagnostics. It
exports positioned CSS box outlines; add `--show-glyphs` to overlay final glyph
outlines or `--hide-boxes` for a clean glyph proof. The TUI is unchanged:

```sh
go run ./tycli documentlayout/testdata/nested.html --root root \
  --fonts tycli/examples/fonts.yaml --width 420 --output /tmp/boxes.svg
```

Font sizing and retained-state ownership are described in [Font instances](doc/FONT_INSTANCE.md).

See [Typography package branches](doc/TYPOGRAPHY_PACKAGES.md) for current service composition.
