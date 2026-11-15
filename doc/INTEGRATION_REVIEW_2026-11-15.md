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
or a general CSS renderer. The interactive workbench and persisted replay cases
still operate on individual paragraphs.

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
  adapter currently selects `NoHyphenation` regardless of the CSS manual value.

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
| 4. Retain and inspect a document result | Retention and headless proofs implemented; interactive document inspection is missing. | Wire a small document experiment into the TUI; document replay is separate. |
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

Confirmed by a temporary integration probe. `style="font-size:20px;"` reaches
`htmlinput.Parse`, which unconditionally appends `"\n;"` before calling douceur.
The doubled terminator is rejected. This is an adapter defect, not a missing box
or khipu feature. A common spelling of otherwise supported input cannot enter
either experiment.

Owner: [htmlinput/document.go](../htmlinput/document.go). Make declaration-list
termination work with and without an existing terminator, including trailing
whitespace/comments and quoted semicolons. Keep the original attribute for actual
styling. Add focused parsing/extraction tests and one document-layout integration
case; do not weaken unsupported-property checks or patch the cascade to compensate.

### 2. Accepted automatic margins do not distribute horizontal space

Confirmed by source and a temporary document probe: a 40px block inside a 100px
parent with both horizontal margins `auto` is accepted and positioned at x=0.
`resolveEdges` computes the auto flags, but `resolveUsedValues` discards them.
The CSS engine's own [checklist](../../css-box-layout/doc/Current-Checklist.md)
already records this gap. The document adapter's acceptance currently exceeds
the implemented outer-layout policy.

Owner: `css-box-layout` for distribution; tytui for a truthful accepted subset.
Until the upstream behavior is implemented and tested, reject block horizontal
auto margins in the experiment. Distinguish normal blocks from atomic items and
vertical margins; do not prohibit all uses of `auto`, including the working
auto-width path. The eventual upstream fix needs definite/auto widths, one/two
auto margins, edges and overflow cases, then a small document regression.

### 3. CSS manual hyphenation and the document profile differ

`boxStyle` accepts `hyphens:manual` and `none`, while preparation always uses
`NoHyphenation`. A probe with `ab&shy;cd`, 20px text and a 30px content width
produces one overfull line under both breakers with omitted, manual or none
declarations. This is an intentional bounded profile in the implementation record,
but CSS acceptance alone does not communicate the override to a caller.

Owner: document policy/adaptation first; khipu only for new measurement semantics.
Keep the existing default profile while making the override explicit in experiment
diagnostics. Until such diagnostics exist, reject authored soft-hyphen cases that
would otherwise claim manual behavior. A later explicit-hyphenation experiment
can use the existing bounded API. Do not enable it indiscriminately: mixed
per-span policies and multiple splits are not covered by that contract. Measurement
and final layout must always use the same chosen policy.

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

The TUI imports the paragraph pipeline, not `documentlayout`; its existing DOM
selector enforces paragraph eligibility. Reusing it unchanged cannot select a
mixed block subtree. A document experiment needs a separate selection/run path
and an explicit CSS-pixel width label. Font services can be reused; serialize work
sharing a resolver and preserve stale-result/cancellation safeguards.

Document SVG is a proof, not a page renderer. Border frames are diagnostic;
backgrounds, border appearance, stacking, clipping and interaction are not modeled.
Outer block flow remains LTR even when a contained paragraph is RTL. Rendering
extensions must keep logical color/source lookup independent of visual order.

There is no document replay format. Existing paragraph replay v1/v2/v3 must retain
its meanings. A document case would additionally need root selection, the document
profile, containing constraints, object resources and geometry/group evidence.
BoxIDs alone are not identities across changed source or arbitrary rebuilds.

## Recommended incremental sequence

1. **Harden the existing input/profile boundary.** Fix the terminator defect;
   guard unsupported block auto margins; make the no-hyphenation override explicit
   for authored markers. Preserve the working fixed/auto fixtures and both breakers.
   Acceptance: every targeted input either exercises its advertised behavior or
   reports a precise limitation; no silent geometry workaround in tytui.
2. **Expose one document experiment in the TUI.** Select a supported block subtree,
   choose CSS-pixel containing width and breaker, run asynchronously and retain a
   completed `documentlayout.Result`. Inspect DOM locator → BoxID → group/owner →
   paragraph/atomic placement; show the chosen profile and overfull diagnostics;
   export the retained proof. Reuse the existing nested-auto fixture at wide/narrow
   widths. Acceptance: compare both breakers, resize, fail a later run and still
   inspect/export the previous result. Keep paragraph mode and replay as controls.
3. **Use that experiment to validate a non-text leaf.** Recommended next structural
   extension: one explicitly sized replaced item, initially a diagnostic rectangle,
   then a caller-supplied local image. CSS owns its border box; P receives the same
   opaque occurrence/advance and vertical geometry; the renderer owns its payload.
   Reuse the existing atomic attachment boundary. Avoid inventing another generic
   object framework or implementing image discovery in khipu. Test source/identity,
   mixed direction, zero size, retained payload lifetime and both breakers.
4. **Then introduce finite height as a separate composition question.** First show
   overflow against a viewport without claiming pagination. If pages are the next
   target, specify legal vertical break opportunities, continuation/source identity,
   margins at boundaries and how retained line material is partitioned. P can
   supply line/vlist material; document composition owns pages and columns. Neither
   the current `InlineExtent` callback nor `VList.Box()` alone is a fragmentation
   API. Avoid treating clipping or rerunning each paragraph as page composition.

The next implementation request should be item 1; the next visible integration
milestone should be item 2. Baseline propagation, margin collapse and richer
line-height are candidates selected by observations, not prerequisites to repeat
the completed document experiment. Add document replay when we need to exchange
these cases beyond checked-in fixtures; specify it independently before changing
the existing replay package. Global caching, parallel owner calls, broader script
coverage and automatic intrinsic hyphenation remain separate investigations.

## Verification and reproducibility

Fresh commands run during this review, using the existing local replacements:

| Module | `go test ./... -count=1` | `go test -race ./... -count=1` | `go build ./...` | `go vet ./...` |
| --- | --- | --- | --- | --- |
| tytui | Pass | Pass | Pass | Pass |
| typography | Pass | Pass | Pass | Pass |
| khipu | Pass | Pass | Pass | Pass |
| css-box-layout | Pass | Pass | Pass | Pass |
| w3c-dom | Pass | Not rerun | Not rerun | Not rerun |

Temporary Go test overlays additionally exercised the three policy cases above;
no production files or proof/replay fixtures were changed. These probes expose
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
