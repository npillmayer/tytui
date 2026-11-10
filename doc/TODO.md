# Integration review TODO — 2026-11-10

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
   broader CSS cascade and parser limitations remain outside this fix.
6. **P2 — dir=auto includes explicitly directed descendants.**
   `<p dir="auto"><span dir="rtl">אב</span>abc</p>` incorrectly resolves RTL.
   The outer direction scan must exclude the nested directional subtree.
   Owner: htmlinput/styles.go firstStrong traversal. Status: open.

## Remaining integration and test gaps

- [ ] Strengthen independent physical-layout oracles. Mixed-direction tests check
  script presence, colors, and geometry copied from the same frozen result, not
  expected physical word order. The single-run oracle derives its starting pen
  from actual output. Add absolute origins, expected knot order, and combinations
  of controls, styles, fallback, directions, and both breakers. Newline-collapse
  tests now cover real-font and synthetic paths; broader physical oracles remain.
- [ ] Implement reproducible case export/import: original HTML/CSS or text,
  selection, extracted styles/map, font identities/hashes, mappings, units,
  direction, breaker configuration, diagnostics, and sibling repository revisions.
  Session history and SVG alone cannot replay failures. This remains an unmet
  requirement of the implementation plan's reproducibility section.
- [ ] Resolve cords builder defects upstream. Review probe: AppendString("é",
  style) leaves Len() at zero and the built text has no accessible assigned style.
  AppendString bypasses style/length bookkeeping; Text ignores the returned styled
  value/error. Current explicit span construction avoids this path, so it is less
  urgent than active pipeline failures, but the planned builder investigations
  are unfinished.
- [ ] Resolve the existing cords TestStyleNoOpOnEmptySpan failure. The full styled
  suite still fails because an empty range returns ErrIllegalArguments. Settle
  and test the public empty-span contract; do not hide it in tytui.
- [ ] Investigate the tree dependency's `CalcRank` race exposed by w3c-dom's
  `TestW3CTextContent1` (`dom_test.go:84`). Reads/writes at `tree.go:534,537`
  race during BottomUp traversal. Reproduced with the pre-issue-5 CSSOM code;
  this is separate from selector specificity. The CSSOM race tests pass.
- [ ] Keep documentation current: old step descriptions still say OpenType and
  w3c-dom are unused or WriteSVG is structural-only, despite later additions.

## Ownership and next priorities

Issues 1–3 are fixed in uax, the itemizer, and paragraph input processing,
respectively. Issue 4 is guarded by explicit rejection; full preservation is
deferred. Issue 5 is fixed in w3c-dom. Next address HTML auto direction (6).
Save each reproduction as a focused owning-package
regression plus a tytui integration case where appropriate. Expand layout oracles
alongside these fixes, then add replayable case bundles and finish cords cleanup.

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
- w3c-dom's full `go test -race ./... -count=1` fails on the existing
  `tree.CalcRank` race described above. Running `TestW3CTextContent1` with
  the original CSSOM restored through a Go overlay reproduces that race.
