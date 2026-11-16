# Simplify replay for an experimental integration laboratory

Completed 2026-11-16: all four steps are implemented. Replay
now supports only experimental version 4. Legacy shaping/color policies have been removed from typography and tytui.
The current behavior remains documented in [OFFLINE_REPLAY.md](OFFLINE_REPLAY.md).
Implementation should recheck the evolving sibling checkouts and preserve unrelated
work, including the current migration to application-owned `fontselection`.

## Goal and scope

Keep reproducible paragraph cases with exact offline fonts and verifiable layout
evidence. Support one current experimental format, without a promise to execute
historical shaping and coloring behavior indefinitely. Retire the legacy
style-boundary policy; color continues to bypass the typesetter and is assigned
using the current glyph/source-anchor policy. A ligature remains one colored glyph.

This does not remove immutable resources, font identities, retained source maps,
frozen results, vlists, rendering services or TUI history. It does not add document
replay, pagination, page-break work, a migration framework or new TUI controls.

The current replay subsystem contains three production files (704 lines) and ten
test files (859 lines), counted including comments and blanks in the inspected
checkout. Those numbers exclude policy branches in pipeline/typography. Reducing
historical behavior combinations matters more than achieving a line-count target.

## Target contract

| Concern | Proposed contract |
| --- | --- |
| Format | Introduce version 4, since versions 1–3 already have meanings. Once the transition is complete, Save writes and Load accepts only that current version. Future incompatible changes increment the version and explicitly replace support. |
| Old or unknown input | Reject with a clear version error before font acquisition or layout. Never reinterpret an old case as the current policy. |
| Shaping and paint | One current behavior: font/shaping attributes determine runs; color does not. Rendering uses separate logical paint ranges and retained glyph/source correspondence. No serialized selector for legacy behavior. |
| Vertical layout | One schema handles the existing ProofCompatibility and CSSLineHeight assembly modes. Record the selected mode explicitly, its applicable values, and placement evidence for both. Removing legacy paint does not silently change paragraph geometry. |
| Font resources | Pin configured defaults, mappings and fallback candidates, including unused candidates. Preserve immutable bytes, SHA-256, provenance-scoped ResourceID, ordering and selection evidence. Paths/descriptor requests are provenance, not instructions for rediscovery. |
| Execution | Load validates pinned resources and reruns the normal current pipeline. Frozen evidence verifies the result; it is not an executable replacement for shaping. |
| Evidence | Compare processed text, breaks, bindings, frozen geometry/glyphs, retained placements and glyph proofs where available. A mismatch fails with a useful category/location; it never updates expectations automatically. |
| Provenance | Retain source HTML/locator or plain input, extracted styles/maps, revision and runtime evidence. Revision metadata describes the recorded environment; replay does not reconstruct that environment. Diagnostic prose remains informational, not a reason to preserve obsolete pipeline behavior. |

Keep explicit wire records separate from runtime faces, resolver/services and
provider objects. Simplify obsolete fields and conversions, but do not serialize
runtime structs wholesale just to shorten the code. Keep strict decoding,
bounded reads, hash/reference checks, cancellation and non-overwriting publication.
The existing limits bound individual resources, not total case size.

Experimental does not mean undetected drift: unchanged cases must still compare
successfully. A deliberate algorithm change requires reviewing its evidence and
updating affected fixtures explicitly. A format or semantic-contract change also
requires a version change; there is no enduring multi-version reader requirement.

## 1. Establish current-behavior regression cases

**Implemented (2026-11-15).** See the
[regression inventory](REPLAY_REGRESSION_INVENTORY.md) for the test classification,
historical fixture successors and independent correctness oracles. New direct-input
tests capture 16 current OpenType SVG/placement controls and hand-calculated
synthetic placements under both breakers/vertical policies. They verify replay
and retained rendering after removing source font files, including unused font
selection retention. Historical fixtures and tests remain unchanged.

Inventory replay tests and classify their purpose: current correctness, resource
lifetime/integrity, source correspondence, or historical compatibility alone.
Identify each legacy fixture's integration question before removing any test.

Retain or add focused current-policy cases for both breakers and both synthetic
and OpenType backends where applicable:

- A color boundary inside `fi` does not split shaping; the ligature receives the
  color associated with its retained source anchor.
- Combining marks, pointed Hebrew, repeated cluster anchors and mixed LTR/RTL
  preserve visual ordering and source-based colors.
- Selected and unbroken discretionary replacements retain their own cluster
  conventions and original-source correspondence.
- Feature ranges retain their scalar origins, including around atomics in the
  typography integration. Removing legacy boundaries must not merge feature domains.
- Both existing vertical policies preserve baselines and placement evidence.
- Saving/rendering remains possible after original font paths disappear or change;
  loading uses pinned bytes without discovery. Unused configured fonts stay pinned.
- Source/style maps survive round trips; missing/corrupt resources, invalid
  references and tampered evidence fail explicitly.

Use existing independent glyph/geometry expectations, not only Save→Load agreement
or freshly regenerated snapshots. Capture representative current SVG and placement
controls before changing APIs. Record historical fixture provenance in
`replay/testdata/README.md`; Git retains the old bytes and implementation.

**Completion:** each useful historical case has a current-policy regression or an
explicit explanation that it tests only retired behavior. No production semantics
change in this step.

## 2. Replace version dispatch with the current format

**Implemented (2026-11-15).** Version 4 is the only reader/writer format. It fixes
glyph-boundary semantics, omits a paint selector, and requires explicit assembly
mode plus placement evidence for both modes/backends. Save rejects retained legacy
results before I/O, even if inspection metadata was edited. Unsupported versions
fail before current schema decoding/font loading. Requested-path provenance now
survives load/save without becoming an executable discovery request.

The two new current-fi manifests match step 1's unchanged proof/placement controls
and pass load/save/load checks. Historical fixture bytes remain unchanged but now
test rejection; their useful integration cases are covered by current regressions.
No permanent converter or legacy reader was added. See
[OFFLINE_REPLAY.md](OFFLINE_REPLAY.md) for the implemented contract.

Implement version 4 in `tytui/replay`: one schema, explicit assembly mode, current
paint semantics, and placement evidence for every result. Remove version selection
based on paint policy or presence of line-height input. Keep the font store and
immutable selection/resource machinery.

Prepare replacement checked-in fixtures before dropping versions 1–3. For an old
case, recover its input and pinned fonts using the pre-change checkout, run that
input under the current glyph policy, inspect the resulting evidence, and save a
new case with current revision provenance. This is a deliberate new expectation,
not a claim that the historical output was reproduced. The red-f/blue-i example
will intentionally become a single red ligature with a supporting font.

Use a temporary developer conversion script or the old checkout if needed; do
not ship an enduring legacy importer. Do not overwrite old fixture bytes in place
without documenting their replacement. Handwritten regression inputs may be clearer
than retaining every serialized historical fixture.

Switch normal Load to current-version-only and add rejection tests for all three
old versions and unknown versions. During this intermediate step, Save must reject
an explicitly legacy-policy result rather than label its evidence as current.
No legacy execution path is needed by the replay package after this step.

**Completion:** current round trips and load/save/load succeed with stable fonts,
colors and geometry. Unsupported versions fail before resource loading; corruption
and evidence mismatch tests remain. The replay package no longer needs historical
layout/color output to pass its suite.

## 3. Retire legacy policies in their owning modules

**Implemented (2026-11-15).** Removed PaintPolicy/constants, ItemizePolicy,
policy parameters on ProjectStyles/ItemizeInline, ParagraphInput's selector, and
pipeline input/result policy state. Replay's intermediate legacy-result guard and
legacy-only tests are removed. Current-policy branches are now the sole paths;
feature domains, object barriers, resource identities and both assembly modes are
unchanged. Renderer failure precedence is preserved without knot-based coloring.
The sibling consumer search found these API users only in typography and tytui;
no deprecated wrappers or changes to khipu/css-box-layout were required.


Coordinate the typography API removal with tytui call-site updates in one passing
cross-module change. Search all sibling consumers before removing exported symbols;
update affected callers rather than leaving deprecated forwarding wrappers.

In `typography`, remove `LegacyStyleBoundary` and its shaping/itemization branches
from style projection, `runStyle` equality and mixed-input processing. Remove the
old knot-start coloring path from rendering. With one behavior remaining, remove
the paint-policy selector type/arguments/fields where they serve no other purpose,
and collapse policy-specific entry points into the normal APIs.

Preserve coalescing based on resolved shaping values, grapheme handling, bidi
boundaries, soft-hyphen context, and the feature-domain machinery. In particular,
`TypographyStyle.domain` also protects feature origins: it is not purely legacy
state and must not be deleted indiscriminately. Keep `PaintSpan` and separate
renderer paint input; do not move color back into typesetter styles.

Update `pipeline`, document adapters, proof construction and tests to use the
single behavior. Remove replay-driven restrictions on successful diagnostic prose.
Keep ordinary error/stage contracts. Do not remove `ProofCompatibility` as part of
this change: it remains the paragraph path's geometry policy, independently of
the obsolete paint behavior.

No replay-specific change is expected in khipu. Its frozen material, provenance,
and prepared lifecycle remain ordinary integration contracts. Do not move format
knowledge or compatibility switches into khipu or the OpenType adapter.

**Completion:** legacy-policy branches and selectors have no live consumers.
Current-policy proof/placement controls are unchanged, and focused feature-domain,
atomic, bidi, discretionary and rendering-bypass tests pass in their owning modules.

## 4. Finish the documentation and test cleanup

**Implemented (2026-11-16).** Current guidance now consistently documents version 4
and the sole glyph-boundary behavior. Historical extraction/review plans link to
the superseding contract. OFFLINE_REPLAY.md gives a concrete fresh-case and evidence
review workflow, with no automatic fixture regeneration.

Replay tests no longer freeze incidental JSON field ordering or whole-record
null/empty spelling. They still check fixed resource identity codes, provenance,
selection order, immutable ownership, strict rejection and exact retained output.
Duplicate test-local manifest readers/writers were replaced by existing helpers.
All production wire adapters were audited: they remain used for current resource,
selection, source or provenance data, so no additional adapter removal was warranted.
The earlier steps already removed historical execution tests and legacy selectors.
No production behavior or fixture bytes changed in this final cleanup.

Remove tests whose sole purpose is executing/resaving old versions. Retain concise
version-rejection tests and the replacement correctness cases. Remove unused wire
compatibility helpers; retain resource and source evidence with a concrete use.
Do not introduce a generic schema registry or migration abstraction.

Update `OFFLINE_REPLAY.md`, replay fixture notes, relevant typography API/rendering
documentation and AGENTS guidance. Historical extraction plans may keep their
dated implementation record, but link to the superseding current-only policy.
Explain how developers create and review a fresh case after an intentional change;
there should be no automatic golden-file update on Load failure.

**Completion:** documentation promises current experimental reproduction and
explicit mismatch detection, not perpetual behavioral compatibility. No TUI,
document replay or pagination feature has been added along the way.

## Validation and ownership

`tytui/replay` owns serialization, content-store I/O, provenance and verification;
`fontselection` owns application discovery/request evidence. Typography owns
shared font resources, style projection/itemization and retained rendering.
Khipu owns horizontal material and freezing, with no replay format dependency.

For each implementation step, run focused tests first, then the affected modules'
ordinary suites. For the coordinated policy removal, run full ordinary and race
suites, build and vet in both tytui and typography. Preserve paragraph/document
proof controls and retained-resource lifetime tests. Run affected sibling consumer
checks when the API inventory identifies them; khipu changes require their own
focused upstream regressions rather than a tytui workaround.

Step 1 validation passed: focused `go test ./replay ./pipeline ./documentlayout
-count=1` in tytui; full `go test ./... -count=1`, `go test -race ./... -count=1`,
`go build ./...` and `go vet ./...` in both tytui and typography. No sibling code
was changed. Khipu's independent suite was not rerun; its integration is exercised
through these suites. Step 2 verification is recorded in the regression inventory.
Step 3 verification is recorded in the regression inventory. Legacy readers and
typography/pipeline policy selectors are retired. Step 4 verification (2026-11-16):
full ordinary/race suites, build and vet passed in both modules; focused replay
checks passed as well. All fixture JSON hashes match the pre-cleanup snapshot.
The plan is complete; no document replay or pagination work is scheduled by it.
