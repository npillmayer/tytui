# Integration review and next document experiments

Reviewed against the local checkouts on 2026-11-15. This is a source-and-test
assessment, not a proposal to implement another layout engine. It supersedes the
pending-work descriptions in the original
[document exploration](DOCUMENT_LAYOUT_EXPLORATION.md); that document retains the
original sequence for context.

## Where we are

**The document-to-paragraph loop is implemented headlessly.** We have crossed the
boundary that the original exploration set out to find:

```text
styled DOM → normalized CSS boxes and inline groups
           → itemized text with attached atomic occurrences
           → prepared paragraphs ↔ intrinsic measurement
           → CSS chooses widths and completes nested atomic boxes
           → bind advances → break → freeze → assemble boxed vlists
           → CSS positions boxes → publish document snapshot → glyph proof
```

Nested fixed-width and auto-width inline-blocks, source correspondence, separate
paint, both breakers, real fonts, and immutable completed results are exercised
together. The remaining work is about the supported policies and making this
integration inspectable. It is no longer about inventing the basic handoff.

This is a **continuous document at a bounded width and natural height**. It is
not yet composition into a rectangle with bounded height, page fragmentation,
or a general CSS renderer. The interactive workbench now also exposes the bounded
document experiment via F7; persisted replay cases still operate on paragraphs.

## What changed in khipu, and why it matters

The old `EncodeParagraph → break → Freeze` route remains useful for the existing
paragraph pipeline. The new document route separates work that needs a font from
work that needs a final atomic width:

| Operation | Current responsibility | Integration consequence |
| --- | --- | --- |
| `PrepareParagraph` | Prepare bidi/text/shaping and attach identified atomic slots. | Shape ordinary text before the final dimensions of nested boxes are known. |
| `MeasureIntrinsic` | Compute natural min/max widths using supplied atomic intrinsic bounds. | Auto sizing does not require pretending that a very wide line break is a measurement. |
| `BindParagraph` | Supply one advance per atomic occurrence and create fresh elastic state. | Reuse preparation without sharing breaker selections or mutable line state. |
| Break and `Freeze` | Select line material and retain detached output, including atomic IDs. | Rendering and vertical assembly use the selected result, not prepared or elastic state. |

An attached object occupies a U+FFFC in the logical text plus an explicit
`contracts.AtomicAnchor`. An unattached U+FFFC remains ordinary text. Atomic IDs
identify occurrences within a paragraph; they are not CSS BoxIDs or DOM IDs.
Khipu owns horizontal advances, Unicode boundaries, breaking and frozen identity.
It does not need a DOM, CSS dimensions, an image payload, or OpenType font types.

`typography.Resolver.ItemizeInline` preserves the text/feature coordinate domains
around these slots and avoids resolving a glyph for an attached object.
`typography/paragraph.Prepare` wraps the khipu lifecycle;
`Prepared.MeasureIntrinsic` and `Prepared.Layout` are the document adapter's
entry points. P adds vertical atomic geometry and line assembly. The adapter maps
AtomicID back to BoxID and returns border origins to CSS flow.

This division already resolves the apparent sizing cycle. CSS recursively measures
nested contents, chooses an auto width, resolves descendants and completes the
atomic border box. Only then does the containing paragraph receive final advances
and vertical sizes. The inline callback does not recurse into atomic interiors.

Important limits of the new contracts:

- Preparation handles and their owner are serialized, with stable text and font
  services. The document adapter reuses them only within one invocation. Resize
  starts fresh preparation; there is no document-wide cache or invalidation model.
- Measurements do not publish paragraphs or consume the single final callback.
  A second final callback attempt is rejected, including after a failed attempt.
  Speculative layout needs an acceptance contract before it can be added.
- Intrinsic bounds describe natural break opportunities. They do not guarantee
  that either breaker will choose a fitting result at min-content width. Existing
  zero-side-skip Knuth–Plass experiments can legitimately return overfull lines.
- Khipu/P support bounded explicit soft-hyphen measurement: at most one eligible
  split position and one break-permitting variant per textbox. Automatic
  intrinsic hyphenation and successive splits remain unsupported. The document
  adapter uses a fixed `NoHyphenation` profile and now rejects authored U+00AD
  under effective `hyphens:manual`; `hyphens:none` explicitly disables the marker.

References: [khipu prepared contract](../../khipu/doc/PREPARED_PARAGRAPHS.md),
[completed cross-module implementation record](../../khipu/doc/PIPELINE_INTEGRATION.md),
[P preparation](../../typography/paragraph/prepared.go),
[document preparation](../documentlayout/preparation.go), and
[measurement adapter](../documentlayout/measure.go).

## Reassessment of the original five steps

| Exploration step | Status now | What remains |
| --- | --- | --- |
| 1. Establish document/box boundary | Implemented for the bounded subset. `Build` supplies typed values and source identities; CSS owns normalization and anonymous boxes. | Policy completeness and broader computed values, not a replacement boundary. |
| 2. Prove box positioning with fake paragraphs | Implemented upstream and surpassed by independent synthetic integration oracles. Parent-content coordinates, nested edges and atomic origins are tested. | Preserve these oracles when expanding CSS behavior. |
| 3. Meet the paragraph API | Implemented through prepared P, including itemized atomic slots, intrinsic callbacks, owner struts and unit conversion. | Broader hyphenation/alignment/inline policies. Do not switch this path back to repeated `paragraph.Run` calls. |
| 4. Retain and inspect a document result | Retention, proofs and F7 document TUI inspection are implemented. | Document replay is separate; full painting remains deferred. |
| 5. Select the next extension | Fixed and auto inline-block integration has already been selected and completed through stage 5 of the khipu integration record. | Choose the next evidence-producing experiment from the gaps below. |

The original ownership decisions still hold: normalized groups define paragraphs;
P owns lines and vlist geometry; CSS owns containing widths and outer positions;
the adapter retains concrete paragraphs; paint bypasses typesetter styles.
The old pending obstacles—style transport, stable text references, anonymous-owner
retention, coordinate conventions and an intrinsic API—should not be reopened as
unimplemented design tasks.

## Evidence and current boundaries

| Area | Implemented evidence | Remaining boundary |
| --- | --- | --- |
| DOM → normalized groups | [extraction tests](../documentlayout/extract_test.go): split/hoist, anonymous owners, whitespace/entities, isolates, source maps and detached input. | Selected root must be a supported block; hidden content, explicit breaks and replaced elements are rejected. |
| Atomic typography | [M tests](../../typography/inline_test.go), [P tests](../../typography/paragraph/prepared_test.go), [OpenType adapter tests](../../typography/adapter/opentype/inline_test.go). | Text still uses the existing coverage/fallback checks. No new coverage policy was added; relocation remains separate. |
| Intrinsic sizing | [auto-width tests](../documentlayout/auto_test.go), [intrinsic tests](../documentlayout/measure_test.go), [nested geometry](../documentlayout/nested_geometry_test.go). | Document box lengths are px/zero and supported auto widths. Percentage/relative box sizing is not enabled. |
| Retained output | [lifetime tests](../documentlayout/lifetime_test.go), [preparation tests](../documentlayout/preparation_test.go). | No cross-invocation preparation cache; borrowed structural trees and shared services remain immutable. |
| Document rendering | [proof tests](../documentlayout/proof_test.go), [auto proofs](../documentlayout/auto_proof_test.go), [retained SVG fixtures](../documentlayout/testdata/README.md). | Source-group diagnostic order, not CSS paint/stacking order; no background/border painting or images. |
| Existing paragraph experiment | Full pipeline and replay suites, including independent HarfBuzz controls and retained placements. | These remain controls; document support does not change their geometry or wire versions. |

The retained result is intentionally split: CSS receives
`InlineResult{Extent, Atomics}`; `documentlayout.Result.Group(BoxID)` retains
`InlineGroup` and `paragraph.Result`. Lines, baselines and glyph arrays do not
travel through `css-box-layout`. Ordinary inline/text nodes do not all have
independent positioned geometry entries. Use retained source maps and group owners
for inspection; a missing text-node rectangle is not automatically a lost box.

The proof composes each `RelativeTo` content-origin transform once. Generic frozen
paragraph traversal now lives in `typography/render.BuildParagraphProof`, shared
with the original paragraph proof. It consumes retained placements and uses
`otshape.VisualOrder` through the OpenType rendering service. It does not remeasure,
reshape, rediscover fonts or restack lines. Colors still come from separate paint
spans and retained logical source correspondence.

The unit boundary is explicit: CSS uses float32 pixels; input lengths become DU
by rounding each primitive independently with `px * 49152`. Paragraph placements
retain float64 big points; CSS summaries convert with `points * 4/3`. Do not replace
this with khipu's differently defined `dimen.PX` or round a sum of CSS primitives
instead of the independently converted values.

## Gaps to prioritize

### 1. Inline CSS validation rejects a trailing semicolon

**Status: fixed in tytui (2026-11-15).** The original probe found that
`style="font-size:20px;"` failed because `htmlinput.Parse` unconditionally appended
`"\n;"` before calling douceur, producing a doubled terminator.

[htmlinput/document.go](../htmlinput/document.go) now validates a temporary
declaration block (`"{\n" + attribute + "\n}"`). The closing brace completes an
unterminated final value without adding an empty declaration after an existing
semicolon. Empty styles, trailing whitespace/comments and quoted punctuation
work; the original attribute still reaches w3c-dom unchanged. No sibling change
or new declaration parser was needed, and unsupported-property checks remain.

[Focused regressions](../htmlinput/inline_validation_test.go) cover parsing,
inspection/extraction, cascade priority, final shorthand values, original source
retention and malformed/unsupported input. The
[document regression](../documentlayout/inline_css_test.go) checks nested inline
styles through normalization, atomic placement and freezing with both breakers,
including independent geometry, font size, separate paint and source mapping.
The regressions failed before the fix and pass afterward.
Follow-up verification passed: tytui's full ordinary and race suites, build and
vet, plus the full ordinary w3c-dom and css-box-layout suites. Paragraph replay
and proof fixtures were unchanged.

### 2. Accepted automatic margins do not distribute horizontal space

**Status: fixed in css-box-layout (2026-11-15).** The original probe placed a
40px block with two auto margins at x=0 inside a 100px parent; it now starts at
x=30. [Used-value resolution](../../css-box-layout/layout/used_values.go) applies
the LTR normal-flow width constraint to normal and anonymous blocks. Free space
goes to one auto margin or equally to two, after padding and borders. Auto width
keeps auto margins zero. Overflow and all-fixed constraints solve the right
margin, which may be negative or differ from its computed declaration. See
[CSS width rules](https://www.w3.org/TR/CSS22/visudet.html#blockwidth).

Inline-block/ordinary-inline auto margins and vertical auto margins remain zero.
The root's existing (0,0) coordinate convention is unchanged. Block margins below
an unresolved auto-width ancestor remain EdgesPending, even when the block's own
width is definite. They are resolved on the private used-values table after
intrinsic sizing; intrinsic contributions do not include distributed free space.
No margin algorithm or coordinate compensation was added to tytui.

[Upstream regressions](../../css-box-layout/layout/auto_margins_test.go) cover
one/two auto margins, widths/units, signed margins, edges, zero/overflow, fractional
rounding, late sizing, callback values and immutable inputs/results.
[Document regressions](../documentlayout/auto_margin_test.go) cover both breakers,
independent geometry, retained paragraph widths/paint/provenance, resize and blocks
inside auto-width atomics. Both regression sets exposed the old behavior before
the fix. The upstream checklist and contract now record the supported LTR policy;
outer RTL and other excluded CSS sizing behavior remain separate work.
Follow-up verification passed in both css-box-layout and tytui:
`go test ./... -count=1`, `go test -race ./... -count=1`, `go build ./...` and
`go vet ./...`. Existing fixed/auto proofs and paragraph replay fixtures remain
unchanged. Numeric regressions also cover a large discarded right margin without
losing the finite residual through cancellation.

### 3. CSS manual hyphenation and the document profile differ

Status: **fixed by explicit document-policy validation** in tytui (2026-11-15).
Previously `ab&shy;cd` at 20px text and 30px width silently produced one overfull
line with omitted, manual or none declarations. The document experiment retains
its fixed `NoHyphenation` profile; it now distinguishes these inputs before
normalization, shaping or sizing:

- Effective `hyphens:manual` (including the CSS default) with an authored U+00AD
  rejects the whole `Build` with `khipu.ErrUnsupportedPolicy`. The error names the
  profile, DOM text-node path and zero-based decoded byte offset. This applies
  even at wide widths and inside nested atomic interiors.
- `hyphens:none` accepts the marker, retaining its bytes and source map while
  disabling discretionary breaks. Normal wrapping remains enabled.
- Manual/default without U+00AD remains accepted; `hyphens:auto` remains rejected.

The adapter obtains effective values from w3c-dom's property service, including
inheritance outside the selected root, initial/unset, and cascade priority. It
checks only selected content and does not mutate input or add a font/style field.
The same prepared paragraph still supplies intrinsic measurement and final layout.
The separate paragraph pipeline, paint bypass and replay formats are unchanged.

[Focused integration regressions](../documentlayout/hyphenation_test.go) reproduce
the old silent acceptance, verify source-located rejection and CSS resolution,
and check fixed-width and shrink-to-fit layout under both breakers. They retain
the disabled marker's frozen source, invisibility, glyph source anchors and color.
The original paragraph extractor remains a separate acceptance control.
Follow-up verification passed in tytui: `go test ./... -count=1`,
`go test -race ./... -count=1`, `go build ./...` and `go vet ./...`, including
the existing paragraph/replay suites and retained document proof fixtures.

Owner: document policy/adaptation; no upstream fix is needed for this guard.
A later explicit-hyphenation experiment can use the bounded khipu/P API, but mixed
per-span policies and multiple splits still need a deliberate contract. Do not
enable manual breaks only during final layout while measuring with NoHyphenation.

### 4. Baselines, empty inlines and CSS layout values are still bounded

The current atomic policy is `AlignBottomMarginEdge`, not the last text baseline
of the nested inline-block. Empty atomic interiors reserve a normal paragraph
strut. Metric-bearing empty ordinary inlines are rejected. Ordinary inline edges
must be zero. These are explicit limitations, not missing atomic-ID machinery.

For full inline-block baselines, CSS should choose and transport the nested box's
baseline/fallback rule; P should align a supplied baseline and return placements.
Khipu still needs only the horizontal advance. Add this as a separate experiment
when the diagnostic proof makes the current difference relevant.

DOM line-height remains normal-only, although P already accepts typed factors and
lengths. The document adapter **does resolve the owner's typography for the strut**;
that part of the parked [line-height plan](DOM_LINE_HEIGHT_PLAN.md) is no longer
missing on this path. General CSS computation/inheritance belongs in w3c-dom,
conversion and source ranges in X, line geometry in P. Do not reopen the parked
plan merely to complete document wiring.

### 5. Inspection, painting and persistence have distinct gaps

The TUI now has separate paragraph and document modes (F7), with distinct DOM
selection paths, width units, breaker controls and histories. The document mode
selects a block subtree and validates/builds/layouts asynchronously using the
existing `documentlayout` boundary. It uses F5's OpenType file/resource settings;
synthetic remains available in paragraph mode. Both modes share one serialized
layout scheduler. Each document command loads fresh services, and edits/mode
switches cancel stale work. Previous document results survive edits and failures.
See the [walkthrough](../README.md#document-experiment) and
[TUI integration tests](../document_workbench_test.go).

Inspection connects DOM locators, normalized BoxIDs, groups/owners, retained lines
and atomic placements. It labels CSS-pixel box geometry and big-point paragraph
coordinates, exposes profile/fit diagnostics, and retains HTML, font identities
and ordered mappings. SVG export captures the displayed result and its services;
font changes and source reloads cannot change earlier proofs. This closes the
interactive inspection gap; document replay and broader painting remain separate.

Document SVG is a proof, not a page renderer. Border frames are diagnostic;
backgrounds, border appearance, stacking, clipping and interaction are not modeled.
Outer block flow remains LTR even when a contained paragraph is RTL. Rendering
extensions must keep logical color/source lookup independent of visual order.

There is no document replay format. Existing paragraph replay v1/v2/v3 must retain
its meanings. A document case would additionally need root selection, the document
profile, containing constraints, object resources and geometry/group evidence.
BoxIDs alone are not identities across changed source or arbitrary rebuilds.

## Recommended incremental sequence

1. **Harden the existing input/profile boundary (complete for findings 1–3).**
   The terminator and horizontal auto-margin defects are fixed; authored soft
   hyphens under CSS manual now report the document profile limitation. Preserve
   the fixed/auto fixtures and both breakers. Every targeted input either
   exercises its advertised behavior or reports a precise limitation; no silent
   geometry workaround was added to tytui.
2. **Expose one document experiment in the TUI (implemented 2026-11-15).** F7
   selects document mode; F4 selects a block; F5 supplies OpenType services; Ctrl+R
   runs at the CSS-pixel width; F2/F3 compare breakers and retained results;
   Ctrl+S exports `document-*.svg`. Tests reproduce the existing nested-auto
   proofs byte-for-byte at 420/100 CSS px under both breakers. They cover failures
   followed by inspection/export, mode/selection/font changes, cancellation,
   captured settings, stale completions, serialization and terminal sizing.
   Paragraph controls/history and replay v1/v2/v3 remain controls. No new layout,
   rendering or replay algorithm was added to the TUI.
3. **Use that experiment to validate a non-text leaf (implemented).**
   Opt-in rectangles and explicitly sized local PNG/JPEG images now exercise
   the full atomic boundary with retained payloads. CSS owns its border box; P receives the same
   opaque occurrence/advance and vertical geometry; the renderer owns its payload.
   Reuse the existing atomic attachment boundary. Avoid inventing another generic
   object framework or implementing image discovery in khipu. Test source/identity,
   mixed direction, zero size, retained payload lifetime and both breakers.
4. **Observe finite viewport overflow on continuous layout.** The proposed
   [finite-height composition plan](FINITE_HEIGHT_COMPOSITION_PLAN.md) covers
   headless overflow assessment, retained SVG viewport overlays and a TUI height
   control. Existing lines and document placements remain unchanged. Pagination
   and all page-break activities, including fragmentation contract design and
   prototypes, are postponed; the eventual approach is undecided. Completing the
   viewport experiment does not automatically resume that work.

Items 1–3 are complete for this bounded experiment. Finite-viewport observation
is the next experiment (item 4). Baseline propagation, margin collapse and richer
line-height are candidates selected by observations, not prerequisites to repeat
the completed document experiment. Add document replay when we need to exchange
these cases beyond checked-in fixtures; specify it independently before changing
the existing replay package. Global caching, parallel owner calls, broader script
coverage and automatic intrinsic hyphenation remain separate investigations.

## Verification and reproducibility

Document TUI follow-up (2026-11-15): tytui's full ordinary and race suites,
`go build ./...` and `go vet ./...` pass, including the new TUI tests and unchanged
paragraph/replay controls. No upstream modules or proof/replay fixtures were
changed for this step. The table below records the earlier cross-module review.

Fresh commands run during this review, using the existing local replacements:

| Module | `go test ./... -count=1` | `go test -race ./... -count=1` | `go build ./...` | `go vet ./...` |
| --- | --- | --- | --- | --- |
| tytui | Pass | Pass | Pass | Pass |
| typography | Pass | Pass | Pass | Pass |
| khipu | Pass | Pass | Pass | Pass |
| css-box-layout | Pass | Pass | Pass | Pass |
| w3c-dom | Pass | Not rerun | Not rerun | Not rerun |

Temporary Go test overlays additionally exercised the three policy cases above
during the original review; that pass changed no production files or proof/replay
fixtures. These probes exposed
coverage gaps despite the passing maintained suites. Full opentype, cords, uax,
fontfind, tree and schuko suites were not independently rerun; their integration
through the tested modules is a narrower claim.

Reviewed heads: tytui `5cb2da4`, khipu `00d3074`, css-box-layout `d54bce9`,
w3c-dom `e9ab7fa`, typography `cf9d9ad`. **Typography has substantial pre-existing
tracked edits and untracked implementation/test files**, including `inline.go`,
`paragraph/prepared.go`, `paragraph/measure.go`, `paragraph/atomic.go` and
`render/paragraph.go`. The successful results describe that working checkout,
not typography's HEAD alone. Its integrated changes need a reviewed commit before
these HEADs can be used as a reproducible checkpoint. No sibling source or
dependency configuration was edited or committed during this review.

Tytui currently replaces ten sibling modules, including css-box-layout. Preserve
that local wiring. The API guidance in older AGENTS/architecture documents is
historical where it contradicts the prepared/atomic interfaces; consult actual
declarations and the owning tests before implementation. In particular,
typography's older AGENTS exclusion of atomic inline boxes needs reconciliation
when that sibling is next updated. Tytui's local AGENTS guidance was refreshed,
but that file is git-ignored; this tracked review and the exploration document
carry the durable integration status.

## Explicit non-text leaf follow-up

`<img data-tytui-leaf="rectangle" alt="label" style="width:36px;height:18px">`
is accepted only in document mode. Width/height must be finite nonnegative px
(or unitless zero); inline/inline-block display is required. No intrinsic image
sizes, aspect-ratio sizing, image URLs, HTML dimension attributes or fallback
image substitution are inferred. Unmarked images remain rejected.

CSS core owns `ReplacedContent`, copied into a childless atomic inline box. It
resolves width/edges, supplies fixed intrinsic bounds and constructs final border
geometry without invoking a paragraph callback for the leaf. Khipu and P receive
the existing opaque atomic ID/advance/geometry. Tytui retains the diagnostic label
by source identity and paints the content rectangle from retained geometry;
source/paint bypass and paragraph replay contracts are unchanged.

`documentlayout/leaf_test.go` covers LTR/RTL, both breakers, auto containers,
zero dimensions, edges, source/occurrence identity and retained proof lifetime.
Upstream `layout/replaced_test.go` checks copying, intrinsic widths, geometry and
invalid dimensions. `TestDocumentWorkbenchDiagnosticLeaf` exercises TUI proof
export; try `documentlayout/testdata/rectangle.html` in F7 mode. Existing nested
SVG and paragraph replay fixtures are not regenerated.

Explicit-leaf validation (2026-11-15): full ordinary/race suites, build and vet
passed in tytui and css-box-layout; paragraph replay and previous SVG fixtures
remain unchanged.


Local-image follow-up: `data-tytui-leaf="image"` supports explicit local PNG/JPEG
resources. Tytui owns bounded loading/full decode and immutable byte snapshots;
CSS core and typographic APIs are unchanged. Relative paths resolve from the HTML
filename directory. SVG embeds retained bytes and stretches them to the CSS
content rectangle; no file access or decoding occurs during export. Read/format
failures remain explicit and preserve the last TUI result. See
[image policy and tests](../documentlayout/README.md#explicit-local-image-payloads).
Paragraph mode, replay versions and previous SVG controls remain unchanged.

Local-image follow-up verification: `go test ./... -count=1`,
`go test -race ./... -count=1`, `go build ./...` and `go vet ./...` pass in tytui.
