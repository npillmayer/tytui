# Integration review TODO — updated 2026-11-11

Review of steps 1–7 against IMPLEMENTATION_PLAN.md. All vertical slices exist,
but the supported subsets are not fully validated. The tytui race-enabled suite
passed during review; temporary targeted probes exposed the issues below.
Prioritize correctness before adding scripts. Status entries distinguish fixes
from observations; unchecked items remain open.

## Confirmed correctness issues

1. **P1 — Unbroken soft hyphen reverses Latin fragments in an RTL paragraph.**
   `a\u00adb`, RTL base, with enough width for one line, freezes as physical `b,a`
   instead of `a,b`, including with synthetic shaping. Before encoding, bidi
   segments assign RTL to the marker between LTR pieces. Owner: uax bidi
   resolution; verify its integration through khipu freezing. Fix must preserve
   original UTF-8 offsets and invisible-marker provenance, not reorder SVG output.
   Status: fixed in the sibling uax checkout (`bidi/scanner.go`). BN characters
   are excluded from resolution before W1–W3 while original byte offsets remain.
   `uax/bidi/soft_hyphen_test.go` covers logical/visual ordering, multibyte text,
   repeated/edge markers, weak characters, combining marks, and isolates.
   `khipu/frozen_soft_hyphen_test.go` verifies both breakers, physical glyph order,
   scalar clusters, and retained source through freezing. Both focused regressions
   failed before the fix and pass afterward. No tytui workaround was added.
2. **P1 — Fallback disables valid discretionary breaks.**
   At 12 bp and 20 bp width, `אב\u00adגד` breaks into two lines with Noto Sans
   Hebrew primary. With Gentium primary and Noto fallback, both breakers instead
   produce one overfull line. The exempt marker selects the primary font and
   introduces an artificial style boundary. Owner: tytui itemization, with khipu
   integration coverage. Glyphless controls need surrounding shaping context.
   Status: fixed in tytui `fontbackend/itemize.go`. Soft hyphens defer face
   selection and inherit the preceding face inside the same CSS span, or the
   following face at the span's start. Marker-only spans use their mapped primary
   face. Real font/script transitions and explicit CSS boundaries are retained.
   `fontbackend/soft_hyphen_test.go` covers interior, repeated, leading, trailing,
   marker-only, and explicitly styled markers. `pipeline/fallback_hyphen_test.go`
   compares direct Noto and fallback under both breakers at narrow and wide widths,
   checking line geometry, selected replacements, glyphs, clusters, and source.
   The integration regression failed before the fix and passes afterward.
3. **P1 — OpenType newlines fail as missing glyphs.**
   `a\nb` fails under both breakers with `.notdef` at scalar 1. Coverage exempts
   controls, but shaping passes them to the font and rejects the output.
   Previously accepted pre-line content could not reliably reach real-font layout;
   CR from `&#13;` also remained unnormalized in that mode.
   Status: fixed by an explicit paragraph-input policy in tytui. Source newlines
   collapse with whitespace before bidi construction, font itemization, and
   shaping. Blank lines do not split paragraphs; DOM selection or one pipeline
   invocation defines the paragraph. Newline-preserving CSS modes are rejected
   until supported explicitly. This does not add forced-break support upstream.
   `paragraphtext` retains processed-to-original byte spans for plain input;
   HTML retains transformed DOM spans and styles. Frozen ranges and clusters
   address `Result.ParagraphText`, while `Input.Text` keeps original plain input.
   Regressions cover CRLF/CR/LF, Unicode separators, blank/empty paragraphs,
   multibyte text, NBSP, DOM mappings, CSS rejection, and TUI previews. Both
   breakers run through freezing with synthetic and OpenType fonts.
4. **P1 — pre-wrap silently loses edge spaces.**
   `<p style="white-space:pre-wrap">  a  </p>` and `a` have the same visible glyph
   origin (x=0) and knot count. Source bytes survive, but layout does not preserve
   the spaces. SpacingFixed controls elasticity, not discardability; khipu still
   trims glue. Add upstream preservation semantics, or reject unsupported cases
   explicitly until available. The documented limitation does not make silently
   changed layout correct. Status: guarded by rejecting pre-wrap during extraction
   (issue 3 policy). Upstream preservation semantics remain deferred until
   preserved-whitespace layout is added to the supported subset.
5. **P2 — Incorrect CSS specificity.**
   `<style>p#x {color:red} .c {color:blue}</style><p id="x" class="c">a</p>`
   resolves blue rather than red. w3c-dom counts IDs only when a selector component
   starts with `#`, and adds source order to its specificity score. Replace the
   heuristic with specificity tuples and separate source-order comparison.
   Owner: w3c-dom/style/cssom. Status: fixed in the sibling checkout's
   `style/cssom/cssom.go`. Cascadia's parsed specificity tuples replace the
   heuristic; source order is a separate tie breaker. Selector lists use the
   strongest branch that actually matches each node, and inline/important
   precedence remains separate from specificity. The parsed-selector cache is
   safe for concurrent tree traversal; node-specific match weights are not cached.
   `style/cssom/specificity_test.go` covers compound/escaped IDs, multiple classes,
   attributes, pseudo-classes, negation, universals, combinators, long rule lists,
   selector-list branches, inline/important precedence, and ordering across sheets.
   `pipeline/specificity_test.go` verifies extraction, inherited font size,
   freezing, and glyph-proof colors with OpenType under both breakers. Regressions
   fail before the fix and pass afterward. No tytui cascade workaround was added;
   broader CSS cascade and parser limitations remain outside this fix. Subsequent
   upstream fixes and the three adapter migrations now also enable duplicate
   declarations and !important; see the completed migration entries below.
6. **P2 — dir=auto includes explicitly directed descendants.**
   `<p dir="auto"><span dir="rtl">אב</span>abc</p>` incorrectly resolves RTL.
   The outer direction scan must exclude the nested directional subtree.
   Owner: htmlinput/styles.go firstStrong traversal. Status: fixed. The scan
   excludes descendant subtrees with dir=ltr/rtl/auto (case insensitive) and
   bdi elements, while retaining the scan root and ordinary descendants. With
   no eligible strong character, the existing parent-direction fallback remains.
   `htmlinput/auto_direction_test.go` covers both directions, nested boundaries,
   auto children, implicit bdi, and fallback. `pipeline/auto_direction_test.go`
   compares bidi runs and physical frozen knots with explicitly directed input
   under both breakers, retaining isolate byte boundaries and DOM mappings.
   Both regression groups failed before the fix and pass afterward.
   Validation: `go test -race ./... -count=1`, `go build ./...`, and
   `go vet ./...` pass in tytui.

## Remaining integration and test gaps

- [x] Delegate property inspection and recursive stylesheet extraction to w3c-dom.
  `Document.Property` uses `css.GetProperty`, with text-node parent inspection
  and explicit absent-property errors. Built-in font metadata replaces local
  default registrations. `ExtractStyleElementsWithError` supplies recursive,
  ordered sheets and node-specific errors; any error rejects the load rather
  than styling partial results. Tytui retains supported-CSS validation and
  linked-resource policy. `htmlinput/upstream_dom_test.go` covers inheritance,
  initial/unset, nested/empty sheets, source maps, and partial-result rejection.
- [x] Remove duplicate-declaration and !important rejection. Upstream CSSOM owns
  importance, specificity, source order, and shorthand/longhand competition.
  `htmlinput/cascade_test.go` covers inline/stylesheet precedence and quoted
  punctuation; `pipeline/priority_test.go` verifies inherited sizes and proof
  colors under both breakers. Inline validation adds a terminator to avoid
  douceur truncating a final multi-token value; styling sees the original
  attribute. Unsupported properties, selectors, and at-rules remain rejected.
- [x] Delegate absolute-unit parsing and HTML display defaults. Font sizes in
  px/pt/in/mm/cm use w3c-dom's checked `dimen.ParseAbsolute`, with explicit
  conversion to khipu DU and positive/finite checks. Descendant display checks
  now use resolved properties, including inherit/initial/unset and upstream
  HTML defaults. `htmlinput/units_display_test.go` covers conversions and retained
  eligibility constraints; `pipeline/css_units_test.go` checks equivalent units
  plus relative child sizing through frozen proofs under both breakers.
  Relative sizes, inherited computed sizes, keywords, paragraph/tag eligibility,
  hidden-content rejection, directionality, and byte/source maps remain local.
- [x] Remove the visual-order API mismatch between OpenType and frozen khipu
  glyphs. `otshape.VisualOrder[G any]` now accepts client-defined record slices
  directly. `pipeline.BuildGlyphProof` applies it in place to each detached
  `CopyText` glyph fragment with its retained shaping direction, then walks the
  result forwards. Frozen data and physical knot order remain unchanged; no
  record conversion or client-owned RTL reversal is needed. Replacement-local
  anchors and glyph positions move unchanged with their records.
  `pipeline/visual_order_test.go` covers pointed Hebrew, repeated anchors,
  selected/unbroken discretionaries, LTR, both breakers, and repeated rendering
  without frozen-data mutation. Upstream tests cover client-defined records,
  copying/in-place ownership, and the existing direction/order contract.
- [ ] Strengthen independent physical-layout oracles. Mixed-direction tests check
  script presence, colors, and geometry copied from the same frozen result, not
  expected physical word order. The single-run oracle derives its starting pen
  from actual output. Add absolute origins, expected knot order, and combinations
  of controls, styles, fallback, directions, and both breakers. Newline-collapse
  tests now cover real-font and synthetic paths. VisualOrder compatibility tests
  now accumulate expected positions from frozen line skips and knot widths,
  rather than deriving the first pen from a rendered glyph. However, they share
  the renderer's ordering helper and frozen geometry, so they do not independently
  establish correct bidi run arrangement, line origins, or line breaking. The
  independent HarfBuzz single-run captures remain valuable; broader physical
  oracles remain open.
- [x] Implement headless reproducible case export/import: original HTML/CSS or text,
  selection, extracted styles/map, font identities/hashes, mappings, units,
  direction, breaker configuration, diagnostics, and sibling repository revisions.
  Implemented in replay.Save/Load with pinned local font objects, retained HTML
  and extraction, semantic request/provider policy, source revisions and dirty
  status, and comparison of frozen runs/proofs. See OFFLINE_REPLAY.md. TUI case
  import/export controls and license-cleared archive packaging remain future work.
- [x] Resolve cords TextBuilder defects in `../cords/styled/builder.go`.
  AppendString and Append now record byte lengths and style spans consistently;
  empty appends create no spans and rejected UTF-8 leaves the builder unchanged.
  Text retains the styled value returned by Style, checks errors, and caches its
  finalized snapshot. The existing Text() signature is unchanged; an internal
  span-application failure panics with context instead of silently losing styles.
  `styled/builder_regression_test.go` covers both append APIs, multibyte text,
  long strings spanning chunks, nil styles, zero-value builders, empty/rejected
  appends, repeated Text calls, persistent snapshots, and append-after-finalize.
  `pipeline/builder_test.go` verifies builder styles reach frozen font sizes using
  tytui's synthetic resolver. Production DOM extraction still uses explicit spans.
- [x] Fix cords TestStyleNoOpOnEmptySpan in `styled/styles.go` and
  `styled/run_ops.go`. Text.Style and Runs.Style treat [p,p) as a no-op when
  p <= the text's UTF-8 byte length, including [0,0) on empty text, nil styles,
  and positions inside multibyte encodings. No styles are inspected or allocated.
  Reversed/out-of-range spans and nonempty spans clamped to empty still return
  ErrIllegalArguments; nonempty end-clamping remains supported. Text.Style also
  returns its original value on error. `styled/empty_span_test.go` covers initial
  styling, existing runs, boundaries, byte offsets, invalid ranges, and clipping.
  Tytui's builder-to-freeze regression checks that empty style selections preserve
  font sizes. No tytui workaround was introduced.
- [x] Replace cords TestConsoleFixedWidth's unconditional failure with exact
  output assertions in `styled/formatter/fmt_test.go`. The test captures output
  in a buffer, uses explicit control codes and ReorderNone, checks paragraph and
  formatting errors, and creates a fresh paragraph for each width. Width 40 must
  wrap between the Hebrew words; width 80 must retain the whole paragraph on one
  line. Expectations include whitespace, punctuation, and final newline markers.
  The test no longer depends on terminal environment variables or writes stdout.
- [x] Fix the schuko global-selector race in `tracing/tracing.go`. Select now
  acquires selectorMutex before every read of the global selector, including the
  nil check; SetTraceSelector already writes under the same lock. Nil selection
  still returns the no-op tracer, and selector callbacks retain their existing
  locking behavior. `tracing/selector_race_test.go` covers concurrent readers,
  replacements between distinct selectors, and nil teardown. It reproduces the
  original read/write race and passes with the fix. This addresses the race
  observed when cords bidi scanning overlapped trace2go.Teardown, separately
  from the earlier testing-adapter instance fixes.
- [x] Fix the tree dependency's `CalcRank` race exposed by w3c-dom's
  `TestW3CTextContent1`. Multiple child notifications could execute the same
  ready parent concurrently. `../tree/tree.go` now claims each ready node once
  per BottomUp traversal, preserving child-completion synchronization. No change
  to the public Rank field or application-level locking is needed.
  `bottomup_rank_test.go` reproduces duplicate parent notifications and checks
  exact-once actions and subtree ranks through repeated public traversals.
  Both w3c-dom and tytui replace the tree dependency with `../tree`.
- [x] Resolve tree's tracing/test races. In sibling schuko, the testing
  tracer now synchronizes levels and prefix consumption, snapshots state before
  formatting/logging, and protects the deprecated redirect target. `P` returns
  an independent contextual tracer with a level snapshot, so concurrent callers
  cannot steal prefixes. In tree, traversal test callbacks protect shared counters
  and result slices; worker concurrency remains enabled. New schuko regressions
  cover concurrent logging/level changes, context independence, and legacy
  redirection. The full race suites now pass (validation below).
- [x] Refresh implementation/dependency/export documentation. The plan and
  DOM/CSS/OpenType guides now describe implemented steps 1–7, explicit fallback,
  whitespace/source coordinates, and current rendering limits. AGENTS.md lists
  all seven sibling dependencies. WriteSVG dispatches to glyph outlines for
  OpenType and structural boxes for synthetic results; the explicit export APIs
  and completed upstream fixes are documented consistently.

## Ownership and next priorities

Issues 1–3 are fixed in uax, the itemizer, and paragraph input processing,
respectively. Issue 4 is guarded by explicit rejection; full preservation is
deferred. Issues 5 and 6 are fixed in w3c-dom and htmlinput, respectively.
Save each reproduction as a focused owning-package
regression plus a tytui integration case where appropriate. The recorded cords
cleanup and dependency wiring are complete. The generic VisualOrder integration
resolves the glyph-type mismatch, but does not resolve paragraph bidi or layout
validation gaps.

The three w3c-dom migrations are complete against the updated sibling checkout.
The former font metadata, recursive stylesheet extraction, absolute-unit, and
display-default obstacles no longer block this adapter. Remaining local paragraph
policy and computed font-size handling are intentional responsibilities, not
unfinished removal of those workarounds. These migrations do not add preserved
whitespace, general CSS layout, or a computed-style engine.

**Recommended next gap: independent mixed-direction physical-layout fixtures.**
Start with a small, manually reviewed LTR/RTL paragraph pair containing Latin,
pointed Hebrew, punctuation, and digits. Specify expected physical word/knot
order and absolute line origins independently of the frozen result and
VisualOrder. Then add explicit directional isolates, a style/font fallback
boundary, and narrow/wide soft-hyphen cases under both breakers. Reuse independent
HarfBuzz glyph expectations for individual runs; do not generate expected run
order or geometry from the implementation under test. Check original source
provenance and replacement-local anchors alongside rendering. Put any exposed
defect in its owning package with a focused regression. This closes the most
important remaining correctness blind spot; the new replay cases can now retain
these fixtures without expanding script support.

Use the newly accepted CSS priority and inherited display behavior in a follow-up
fixture, while keeping the first mixed-direction fixture small enough to review
independently. Passing cascade and unit-equivalence tests do not establish correct
physical bidi arrangement. Headless replay is now implemented; TUI case controls
and portable archive packaging remain tooling follow-ups after these fixtures. Fontfind integration has resumed: steps 1–4 (offline resources, descriptor resolution, TUI requests/previews, and
headless replay) are complete; optional remote discovery remains a later step
in FONTFIND_INTEGRATION_PLAN.md.

Steps 1–3 provide the strongest foundation: headless operation, asynchronous TUI
lifecycle, fresh owners, freezing, history, and export. Step 4's constrained DOM
explorer is implemented. Steps 5–7 have working vertical slices but the findings
above qualify their completion claims.

Broader font/script support is subsequent work: optional GSUB/GPOS parsing,
normalization-aware fallback, complete script itemization, additional engines,
and CSS line-height. Mandatory GPOS is a documented OpenType parser scope policy,
not a newly discovered regression. Prefer focused scope decisions and independent
fixtures over a broad shaping repair campaign.

## Issue 1 validation

- uax: `go test -vet=off -race ./bidi -count=1` passes. Without `-vet=off`,
  pre-existing nonconstant `t.Logf` format strings in `bidi/bidi_test.go:74,108`
  block the test command. Those unrelated test diagnostics were not changed.
- tytui: `go test -race ./... -count=1` and `go build ./...` pass.
- khipu: the focused freezing regression passes for firstfit and Knuth–Plass;
  `go test -race ./... -count=1` also passes for the full suite.

## Issue 3 validation

- tytui: `go test ./... -count=1`, `go test -race ./... -count=1`,
  `go build ./...`, and `go vet ./...` pass.
- No sibling source changes were needed: whitespace policy belongs at the
  paragraph-input boundary, before the libraries receive processed text.

## Issue 5 validation

- The new upstream cascade regressions and tytui integration regression fail
  against the original CSSOM and pass with the fix.
- tytui: `go test -race ./... -count=1`, `go build ./...`, and `go vet ./...`
  pass, including the OpenType proof integration under both breakers.
- w3c-dom: `go test ./... -count=1`, `go build ./...`, and `go vet ./...` pass.
  CSSOM and its adapter also pass under the race detector in the full run.
- At the issue 5 review, w3c-dom's full `go test -race ./... -count=1` failed on the existing
  `tree.CalcRank` race described above. Running `TestW3CTextContent1` with
  the original CSSOM restored through a Go overlay reproduces that race.

## Tree rank race validation

- The deterministic queued-notification regression fails before the fix with
  two parent actions and a race on Rank, and passes afterward.
- tree: both new regressions pass with `-race -count=10`; full non-race tests
  and `go vet ./...` pass. The former full-suite race limitation is now fixed (see below).
- w3c-dom and tytui: full `go test -race ./... -count=1` suites pass using the
  sibling tree replacement. Tytui build and vet also pass.

## Tracing/test race validation

- schuko's new `tracing/gotestingadapter/concurrency_test.go` regressions fail
  against the original adapter with races/context leakage and pass with the fix.
- `go test -race ./... -count=1` passes in schuko, tree, w3c-dom, and tytui.
  Tree's TopDown/BottomUp callback tests now synchronize their own bookkeeping.
- `go vet ./...` and `go build ./...` pass in all four modules.
- Cords, tree, w3c-dom, and tytui now replace schuko with `../schuko`, so tests and normal
  runs use the owning-package fix rather than cached releases.
- Global tracer configuration/teardown remains serialized between tests. Tracer
  instances support concurrent calls; workers must finish before their testing.T
  completes. This does not make concurrent global QuickConfig sessions supported.

## TextBuilder validation

- Focused length and style regressions fail against the original builder and
  pass with the fix, including `go test -race ./styled -run TestTextBuilder -count=1`.
- Tytui's full `go test -race ./... -count=1`, build, and vet pass.
  Cords' styled packages also build and pass vet.
- At the TextBuilder review, the broader cords styled suite retained the
  TestStyleNoOpOnEmptySpan failure (now fixed above) and TestConsoleFixedWidth's unconditional `t.Fail()` (now fixed above) in
  `styled/formatter/fmt_test.go:90`. At that review, cords' pinned schuko retained the old tracer races; validation with
  a temporary module file selecting the fixed sibling schuko removes those races
  and leaves those two unrelated test failures. No dependency files were changed in cords.

## Empty style-span validation

- New regressions fail against the original implementation and pass with the fix.
  `TestStyleNoOpOnEmptySpan` now passes unchanged.
- The full cords styled and inline packages pass under the race detector with
  the fixed sibling schuko selected via a temporary module file:
  `go test -modfile <validation.mod> -race ./styled ./styled/inline -count=1`.
  No cords dependency files changed in that step. The local replacement is now
  permanent; see the dependency wiring validation below.
- Tytui: full `go test -race ./... -count=1`, build, and vet pass. Cords' styled
  packages build and pass vet. At that review, the formatter's unconditional
  `t.Fail()` was still a separate limitation; it is now fixed above.

## Console formatter test validation

- `go test ./styled/... -count=1` passes in cords, including the complete styled,
  formatter, and inline packages.
- The focused replacement test passes under `-race`. With the fixed sibling
  schuko selected through a temporary module file, formatter and inline also
  pass in full under `-race`; the broader styled race run exposed the separate
  global-selector race recorded above (now fixed). No dependency files were changed.
- Only the owning-package test changed; formatter production behavior is unchanged.

## Global-selector race validation

- The focused schuko regression fails under `-race` against the original code;
  it passes ten consecutive runs with the fix.
- Schuko and tytui: full `go test -race ./... -count=1`, `go build ./...`, and
  `go vet ./...` pass.
- Cords: all styled packages pass with
  `go test -modfile <validation.mod> -race ./styled/... -count=1` using a temporary
  replacement for the fixed sibling schuko. This temporary validation has now
  been superseded by the permanent local replacement below.

## Cords dependency wiring validation

- [x] Cords' go.mod now replaces `github.com/npillmayer/schuko` with
  `../schuko`. `go list -m github.com/npillmayer/schuko` confirms the local
  checkout is selected. No temporary module file is needed.
- Cords: `go test -race ./styled/... -count=1` passes, including styled,
  formatter, and inline. Tytui: `go test -race ./... -count=1` passes.
- README.md and AGENTS.md document the sibling layout and the independent
  main-module replacements required by Go.

## Documentation refresh validation

- Cross-checked the current guides against go.mod, pipeline/svg.go,
  pipeline/pipeline.go, fontbackend/itemize.go, fontbackend/stream.go, and the TUI
  controls. Historical validation entries remain labeled as prior observations.
- Checked documentation diffs and relative Markdown links. This refresh changes
  documentation only; runtime tests were not rerun.

## Generic VisualOrder integration validation

- At implementation, opentype and tytui both passed `go test ./... -count=1`,
  `go build ./...`, and `go vet ./...`. Tytui also passed the full
  `go test -race ./... -count=1` suite.
- Focused VisualOrder and renderer compatibility tests passed, as did the
  independent HarfBuzz proof tests. Compatibility is not a substitute for the
  independent mixed-direction layout fixtures recommended above.
- This TODO review updates documentation only; those validation results are
  from the integration implementation, not a new test run.

## W3c-dom migration validation

- Each of the three migrations passed tytui's full `go test ./... -count=1`,
  `go test -race ./... -count=1`, `go build ./...`, and `go vet ./...` checks.
- The focused tests named above exercise the public upstream APIs through the
  adapter; pipeline tests cover frozen results and glyph proofs with both breakers.
  No sibling implementation changes were needed for these migrations.
- This status review is documentation-only. Validation above records the completed
  implementation runs; runtime suites were not rerun for this edit.
