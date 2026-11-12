# DOM/CSS line-height integration plan

Proposed 2026-11-12. This is an implementation plan, not an API already delivered.
It follows [the paragraph engine](PARAGRAPH_ENGINE_PLAN.md),
[DOM integration](DOM_INTEGRATION.md), and [offline replay](OFFLINE_REPLAY.md).
The purpose is an integration experiment: establish which CSS semantics the
participating modules can represent, expose gaps with reproducible cases, and
keep reusable fixes in their owning packages.

## Current evidence and gaps

| Finding from the current checkouts | Consequence / priority |
| --- | --- |
| `w3c-dom/style/defaults.go` registers line-height as inherited, initially normal. Tytui validates declaration names against that registry. | A declaration can pass loading without affecting layout. Make its extraction/support status visible. |
| `htmlinput/styles.go` computes font sizes and inherited styles but does not read line-height. `PropertyMap.Property` performs no inheritance; inspection uses `css.GetProperty`. | Need computed values with ancestor context, not a number parsed from an inherited raw string at the leaf. |
| `htmlinput/extraction.go` emits normalized bytes and source mappings; collapsed whitespace uses the first pending text unit's style. | Layout ranges must follow this same emission and whitespace ownership, independently of typography-span merging. |
| `paragraph.LineLayout` already represents normal, factors, lengths and optional byte overrides. These do not enter styled.Text. | Reuse this narrow boundary; no annotation system or additional shaping-style field is needed. |
| CSSLineHeight currently takes its strut from `Streaming.Defaults.Font/EmSize`. | Backend defaults may differ from the selected paragraph's computed font/size, particularly when it contains only differently styled children. Resolve the paragraph's own typography explicitly. |
| Replay v3 persists typed line-height and numeric placements; v1/v2 preserve compatibility geometry. | Preserve old meanings. DOM-derived strut selection may need a new wire version rather than silently changing v3. |

These are source-inspection findings. Missing upstream capabilities below are
candidates to characterize with tests, not assertions that new upstream bugs have
already been reproduced.

## Ownership

| Component | Responsibility |
| --- | --- |
| w3c-dom | CSS syntax, cascade winners, shorthand expansion, property metadata, CSS-wide keywords and reusable contextual computed-value services. Keep general CSS fixes here, without importing typography or khipu. |
| X: tytui/htmlinput initially | Select explicit paragraph context, normalize supported content, map computed values onto emitted UTF-8 bytes, retain DOM provenance, and adapt to P's typed layout values. Extract X later when its boundary is established. |
| M: typography | Resolve the selected paragraph's font request as well as run typography through existing font contracts. Do not choose a strut from the first glyph or a fallback run. |
| P: typography/paragraph | Resolve metric-dependent line extents, strut minimums, half-leading and natural stacking. No CSS strings, DOM locators or cascade logic. |
| A: typography/adapter/opentype | Supply contractual face metrics and scaling. Fix incorrect metrics here; do not compensate in X. |
| Tytui pipeline/TUI/replay | Explicit experiment selection, service composition, diagnostics, retained policy and versioned reproducibility. Paint continues on the separate rendering path. |

Prefer a small w3c-dom computed-line-height service using explicit font-size/root
context over a new universal computed-style framework. X converts its neutral
value into P's value and units. Reuse existing absolute-unit parsing. Keep any
application-only supported-subset checks in X.

## Proposed contract and policy

Start with normal, nonnegative numbers, absolute lengths already supported by X
(px, pt, in, mm, cm), percentages, em and rem, plus inherit/initial/unset. A number
remains a multiplier; a percentage or em length computes against the declaring
element's computed font size, then inherits as a length. Rem uses the document
root's computed size, even when selecting a nested subtree. Normal remains a
keyword until P measures fonts. These distinctions follow
[CSS line-height computed values](https://www.w3.org/TR/CSS2/visudet.html#propdef-line-height).

For example, a 10bp parent with line-height 1.5 and a 20bp child passes factor 1.5
to both; 150% on that parent computes to 15bp and the child inherits 15bp. A fresh
150% declaration on the child computes to 30bp. Zero is valid; negative and
nonfinite values are not. Specify conversion/rounding once at the CSS-to-DU
boundary, reject overflow before conversion, and preserve fractional geometry in P.

Initially leave calc(), var(), viewport/font-relative units beyond em/rem,
revert/revert-layer, and font shorthand outside the accepted experiment unless
owning tests establish their correct support. Distinguish invalid CSS declarations
(which a conforming cascade ignores) from valid unsupported features. Never let a
rejected winning value silently erase an earlier valid declaration: either let the
upstream cascade handle validity or reject the experiment with an explicit limit.
Tytui may deliberately reject unsupported cases rather than emulate full browsers.

Keep computed layout data beside Extraction. Do not add line-height to
style.Style/TYPOGRAPHY equality or M's shaping key. Proposed extraction data consists
of paragraph default height, separate normalized byte overrides, and the selected
paragraph's typography request for its strut. Exact Go field names follow the
baseline tests. Adjacent equal layout values may merge without merging source maps.

For an inline/text selection, explicitly use the selected element (or text node's
parent) as the experimental paragraph context. Report that choice; it is not a
claim to reconstruct its containing CSS block. Empty inline boxes and display:contents
need explicit limits: text ranges cannot represent all of their box contributions.
Do not fabricate ranges for empty elements or silently claim full CSS conformance.

## Incremental implementation

### 1. Characterize CSS winners and computation upstream

Add focused w3c-dom tests for inline versus stylesheet declarations, specificity,
importance, duplicate declarations, missing values and inherit/initial/unset.
Verify whether the winner API exposes a local declaration versus inherited/default
values reliably. Then cover factor/percentage/em/rem inheritance across three
levels, subtree selection, and declaration order when font-size also changes.
Compute font-size before line-height, regardless of textual declaration order.

Implement the smallest missing neutral parsing/computation API in w3c-dom. Keep
ancestry traversal or explicit parent/root context visible in its contract. Add
an X integration test using that API; do not duplicate the cascade in tytui.

Acceptance: independently specified computed values, including 1.5 versus 150%,
are correct before any shaping occurs. Unsupported syntax is diagnosed clearly.
This is the first recommended implementation step.

### 2. Extract layout ranges alongside existing text/source maps

Extend X's computed-node state and text-unit emission with separate line-height
values. Emit ranges only for output bytes, using the existing first-unit policy
for collapsed spaces. Preserve many-to-one and omitted source mappings. Keep
zero-byte bidi events independent. Validate complete effective coverage, ordered
nonoverlapping overrides, UTF-8 boundaries, cloning and text agreement.

Test nested spans, entities, multibyte/combining text, whitespace across nodes,
trimmed edges, and a line-height boundary within fi. Assert unchanged extracted
text, typography styles and bidi/source maps. Retain provenance for diagnostics
without passing DOM locators to P. For unsupported empty-element contributions,
report the limitation rather than discarding meaningful layout intent silently.

Acceptance: equivalent DOM and manually supplied typed values compare equal;
changing only line-height cannot split a shaping run or modify paint spans.

### 3. Resolve the paragraph strut independently of runs

Use M to resolve the selected paragraph's computed font family, weight, slant and
size against the pinned settings. Prefer an optional explicit strut font/size
contract in P, with the existing Streaming defaults remaining the fallback for
current callers. Keep it separate from encoder defaults: changing those defaults
could alter shaping, spaces and fallback. Do not add concrete OpenType types.

Test an empty paragraph, a paragraph containing only a larger child, a paragraph
font absent from its visible glyphs, mixed fallback metrics, missing selected faces,
and line-height normal. Record explicit resolution diagnostics. Ensure a strut-only
font is pinned even when no glyph uses it. Freeze/retain all resulting geometry.

Acceptance: paragraph metrics are independent of the first glyph and do not alter
breakpoints or glyph identities. Synthetic and OpenType paths obey the same contract.

### 4. Wire an explicit headless DOM-layout experiment and replay

Keep ordinary extraction and old pipeline defaults compatible. Introduce an explicit
DOM line-height extraction/composition mode; requesting it with no declaration
still means normal, not ProofCompatibility. Define input precedence: explicit
manual LineLayout and DOM-derived layout should be mutually exclusive rather than
silently overriding one another. Preserve provenance when changing width/breaker;
text edits detach DOM-derived layout along with source maps.

Replay stores computed typed values and sufficient pinned strut selection evidence;
load must not rerun a newly changed CSS cascade. Do not serialize a font service.
If the strut extension changes v3's interpretation, introduce v4 with explicit
selection and geometry evidence. V1/v2 remain compatible; v3 continues to mean
its original backend-default strut. Do not infer a new mode from OriginalHTML or
upgrade legacy cases while loading/resaving. Add static pre-change v3 fixtures
before extending the wire format, alongside existing v1/v2 fixtures.

Acceptance: DOM and non-DOM inputs produce equivalent placements under both
breakers; offline replay works after source HTML/fonts disappear and rejects
missing strut resources or tampered values. Ligature/mark colors stay unchanged.

### 5. Expose the experiment for inspection

Add a small explicit TUI choice between compatibility and DOM line-height, with
inspection of computed default/override values, chosen strut and unsupported cases.
Reuse existing cancellation, history and frozen-proof behavior. Distinguish logical
line boxes from overlapping glyph ink and structural debug SVG spacing.

Add a compact HTML fixture set covering inheritance, mixed sizes, fallback, RTL,
negative leading and boundary-crossing ligatures. Record strengths and remaining
limitations in DOM_INTEGRATION.md and AGENTS.md. This is an investigation aid,
not a broader end-user settings or browser-layout project.

## Verification and deferred work

At each step run focused owning-package tests and affected integration suites.
For changes to CSS computation verify w3c-dom; for strut/metrics verify typography.
At completion run ordinary/race suites, build and vet in w3c-dom, typography and
tytui. Keep independent numeric expectations, unchanged glyph/source evidence,
retained-data mutation tests, and legacy replay fixtures; do not regenerate old
proofs to accommodate regressions.

No khipu/cords change is expected for baseline text line-height: values bypass
shaping styles. Investigate any missing frozen/source provenance with a minimal
owning regression before proposing an upstream API change. Vertical alignment,
inline images, empty inline box material, full display:contents semantics, block
margins/padding, fragmentation and the css-box-layout bridge remain separate work.
The general annotation API remains deferred.

This plan was checked against local source; no runtime implementation or test
execution was performed for this documentation change.
