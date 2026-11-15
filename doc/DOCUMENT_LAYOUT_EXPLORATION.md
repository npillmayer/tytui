# From a styled document to positioned paragraph boxes

Living discussion document, started 2026-11-12. Direction agreed: explore document
layout from the top down, meeting the existing paragraph/vlist API. Detailed
paragraph improvements, including [DOM line-height extraction](DOM_LINE_HEIGHT_PLAN.md),
are parked unless a concrete document-layout experiment requires them.

Tytui remains an experimental integration laboratory. The immediate outcome is a
small, inspectable document-layout example and evidence about the participating
modules. **Re-evaluated 2026-11-15:** the headless experiment now includes fixed
and auto-width nested inline-blocks, intrinsic measurement, retained paragraph
results and real-font proofs. See [documentlayout](../documentlayout/README.md),
[fixed/auto fixtures](../documentlayout/testdata/README.md), and the
[current integration review and next steps](INTEGRATION_REVIEW_2026-11-15.md).
The new khipu prepare/measure/bind lifecycle is already integrated through P.
F7 document mode now selects block subtrees, runs this path asynchronously and
inspects/exports retained results alongside paragraph mode. Full painting,
finite-height composition and document replay remain separate work. The original
numbered steps below are historical.

## Working picture

```text
HTML + CSS
    │ w3c-dom: styled document and CSS services
    ▼
Document adapter: source identities + supported computed style values
    │
    ▼
css-box-layout: construct/normalize box tree
    │ anonymous boxes, inline groups, block containers
    ▼
Resolve used dimensions from containing-block constraints
    │
    ▼
CSS flow: measure auto contents, choose widths, complete nested atomics
    │
    ├── Inline-group adapter: collect text/styles/source correspondence
    │       │ M prepares typography; paint remains separate
    │       ▼
    │   typography/paragraph: prepare ↔ measure; bind → break → freeze → vlist
    │       │ retained paragraph-local line geometry and glyphs
    │       ▼
    └── Overall extent + atomic border origins (return to block flow)
    ▼
Document result: box geometry + typed group-to-paragraph table
    │ group BoxID → owner BoxID + retained paragraph result
    │ source correspondence and paint retained separately
    │
    ▼
Diagnostic tree / document proof renderer
```

A paragraph vlist is a leaf of the **outer positioning problem**. Internally it
still contains lines and frozen glyph material. CSS layout determines its available
width; the paragraph engine determines its natural height and line baselines.
The outer engine positions the result without stacking its lines again.

The unit of paragraph work should be an inline formatting group produced by box
normalization, not simply each HTML p element. A div containing text, a nested
block, then more text can produce multiple groups. Conversely, nested inline spans
belong to one group and must not become independent shaping requests.

## Agreed boundary: extent return and typed retention

Decision agreed with css-box-layout: block flow needs the overall layout extent
of a paragraph vlist, not individual line summaries or an opaque paragraph payload.
The adapter returns that extent in box-layout units. For the present normal-flow
experiment, the essential returned dimension is consumed height; available content
width is already supplied. Layout extent includes agreed trailing spacing and is
not merely glyph ink bounds. The concrete return is InlineResult{Extent, Atomics}: atomic border origins are
relative to the owner content origin, with IDs for core-owned prepared boxes.

The document integration layer owns a typed table scoped to one layout invocation:

```text
group BoxID -> owner BoxID + concrete paragraph result
```

Use P's concrete retained result type (or a typed reference to it); do not introduce
`any`, runtime type assertions, or a dependency on P's internal types into
css-box-layout. The implementation is Result.Group(BoxID), returning a detached GroupResult
with Input InlineGroup and Paragraph paragraph.Result. Paragraph-local lines, baselines and precise
glyph geometry remain inside the retained paragraph result for rendering and
inspection; they do not travel through the block-flow return value.

The inline group B and owner A already have BoxIDs from tree construction. The
vlist does not create another core box or receive BoxIDs for its internal boxes.
Its extent sets A's content height; rendering derives B's origin from A's content
geometry and composes ancestor transforms once. Preserve the group-to-owner
association explicitly, including anonymous owners.

The adapter records P's result in invocation-local state during LayoutInline.
This side effect is explicit integration bookkeeping, not a global registry.
Publish the typed table, box geometry, source correspondence and separate paint
as one complete document snapshot only after layout succeeds. A failed or later
run must not alter the previous snapshot; retained objects and backing storage
must remain valid and immutable for that snapshot's lifetime.

There is one accepted final result per group per invocation. Intrinsic measurement
is a separate callback that reuses private preparation and publishes no paragraph.
Before speculative final layout or repeated final requests are introduced, specify
how the accepted result is selected; blindly overwriting the table is insufficient.

This is implemented wiring. CSS core owns recursive nested atomic preparation;
final callbacks receive read-only PreparedAtomicsView and UsedValuesView; measurement
receives intrinsic bounds and resolved edges. No external
AtomicSizer, []LineBox return, opaque paragraph payload or mutable node geometry
mirror is used. Speculative/repeated final callbacks are explicitly rejected;
repeated measurements are supported within the serialized invocation.

## What exists today

Current implemented boundaries (2026-11-15):

| Boundary | Current surface | Ownership |
| --- | --- | --- |
| Styled document → render input | documentlayout.Build supplies typed styles and stable text source references to RenderNode. | DOM policy and source tables stay in tytui; no glyph coverage policy is added. |
| Render input → box tree | BuildLayoutTree normalizes anonymous groups and split/hoisted inlines. | CSS core retains NodeID/BoxID distinctions and transported styles. |
| Box tree → used dimensions | ResolveUsedValues resolves definite values and marks pending auto widths. | CSS core owns used widths, shrink-to-fit and descendant re-resolution. |
| CSS sizing → paragraph measurement | MeasureInline supplies nested atomic intrinsic bounds and resolved edges to Prepared.MeasureIntrinsic. | Private preparation is reused; measurements publish no final layout. |
| Block flow → paragraph request | LayoutInline(InlineRequest) receives Group, OwnerID, ContentWidth, Used and completed Atomics. | The adapter extracts/itemizes/prepares once per invocation, then binds and lays out; no atomic-interior recursion. |
| Paragraph result → block flow | InlineResult returns Extent plus BoxID-mapped atomic border origins. | Paragraph lines/glyphs/source/paint stay in typed GroupResult records. |
| Flow → document output | Result publishes detached geometry accessors and typed retained groups after whole-flow success. | Geometry uses explicit RelativeTo content parents; old snapshots survive resize/failure. |
| Document → proof | BuildProof/WriteSVG compose each owner-content transform once around retained glyph/line placements. | Generic paragraph walking lives in typography/render; CSS/DOM associations stay in tytui. |

The sibling's authoritative design is
[CSS-Algo-Overview.md](../../css-box-layout/doc/CSS-Algo-Overview.md); its
[Current-Checklist.md](../../css-box-layout/doc/Current-Checklist.md) records gaps.
The implementation currently supports a much narrower surface than a complete CSS
engine. Its outer-layout contract is LTR; paragraph bidi support does not establish
RTL block-layout support.

## Next experiment

The [review](INTEGRATION_REVIEW_2026-11-15.md)'s three immediate findings are now
addressed. Inline CSS trailing-semicolon validation is fixed locally; horizontal
auto margins are fixed in css-box-layout for LTR normal-flow blocks, including
late containing widths. The fixed document NoHyphenation policy now rejects
authored soft hyphens under effective CSS manual with a source-located error;
`hyphens:none` is the explicit opt-out. Focused regressions cover these boundaries,
including both breakers and intrinsic/final layout consistency.

The interactive part of original step 4 is also implemented: F7 document mode
selects a block subtree, varies CSS-pixel width and breaker, inspects source/box/
group associations, compares retained results and exports proofs. F5 OpenType
settings feed fresh per-run services; failed runs keep the previous document.
The [TUI tests](../document_workbench_test.go) reproduce all four nested-auto
proof controls and preserve paragraph mode/replay. See the
[walkthrough](../README.md#document-experiment).

An explicitly sized diagnostic rectangle now exercises the non-text atomic
boundary. Explicitly sized local PNG/JPEG payloads now use the same boundary,
with immutable retained bytes and SVG embedding. Bounded-height composition is
the next separate fragmentation/design question. The proposed
[finite-height composition plan](FINITE_HEIGHT_COMPOSITION_PLAN.md) begins with
viewport overflow diagnostics on retained results, before introducing pagination.

## Ownership to preserve

| Owner | Responsibility |
| --- | --- |
| w3c-dom | DOM/CSS representation, cascade and reusable CSS value services. General CSS fixes belong here. |
| css-box-layout | Box generation, anonymous/split structure, formatting contexts, used box dimensions and block positioning. Own fixes to these algorithms here. |
| Document/inline adapters | Translate supported document values and text identities, assemble one inline group's paragraph input, convert units, and retain associations. Start with thin integration code; extract reusable adapters when the contracts settle. |
| Typography M and A | Font selection/itemization and contractual shaping/metrics services. No document-tree layout. |
| Paragraph P | Horizontal typography and internal line stacking into a boxed vlist. No containing-block positioning or margin collapse. |
| Tytui | Compose the experiment, inspect boundaries, preserve failures and visualize results. Avoid implementing a second box-layout engine. |

Document input, layout tree and rendered output are distinct representations.
Preserve NodeID for source association and BoxID for generated/split box identity;
one DOM node may generate multiple boxes. Document-to-text correspondence must
survive normalization and paragraph extraction rather than being reconstructed
from displayed strings afterward.

## Initial experiment (historical): a bounded continuous document

The original recommended starting fixture was a block root with a fixed available
width, two paragraphs, and one nested block containing another paragraph. Add basic padding
and borders after the zero-edge case works. Use normal-flow LTR block layout,
explicitly supported widths, and additive margins only where the upstream contract
supports them. No pagination, floats, positioned elements, flex/grid, margin
collapse, inline-blocks, images or intrinsic sizing in this first experiment.

Use deterministic fake inline results first: known layout extents at two known
widths. This isolates document structure and positioning from typography. Then
replace the fake with the existing synthetic paragraph backend, followed by one
real-font example. Existing paragraph spacing can remain an explicitly named
compatibility policy; do not present the experiment as full CSS line-height layout.

Success means we can trace a source node through box generation, available width,
paragraph request, returned vlist and final placement, with independently specified
coordinates. A narrower parent must trigger a fresh paragraph layout and update
following block positions. Prior document results must remain inspectable.

## Original exploration sequence (historical)

The steps below record the initial review, before the current upstream contracts.
Its implementation concerns are resolved by the boundaries above and the staged
[khipu integration plan](../../khipu/doc/PIPELINE_INTEGRATION.md). The original
proposals are retained as context, not as descriptions of the current API.

### 1. Establish the document/box boundary

Build one minimal w3c-dom fixture and inspect its proposed render-input projection.
Record the supported style-value subset, source IDs and document/root conventions.
Characterize the real BuildLayoutTree API before designing a broader adapter.

First upstream priority: preserve styles through box construction, including split
fragments and anonymous-box rules. Give text leaves stable source references and
source-node associations; current source-0 local ranges cannot identify distinct
text nodes reliably. Test these in css-box-layout, with a small tytui integration
case. Do not compensate by rebuilding the box tree in the workbench.

### 2. Prove box positioning with fake paragraph results

Exercise build → used values → flow with fixed inline results. Test three levels
of nesting, differing widths, padding/borders and following-sibling positions.
Resolve coordinate conventions with owning regressions: the contract says parent
content-relative coordinates, while current flow adds content offsets and mutates
geometry mirrors. This is a review target, not yet a confirmed defect.

Check empty block normalization and typed retention for every inline group.
Current flow stores line tables only for non-anonymous owners; the agreed
adapter-owned table retains all groups independently of that policy. Implement
the upstream extent-only result contract and preserve group-to-owner identities.
Do not lose paragraphs that happen not to have a named block owner.

### 3. Meet the paragraph input API

Collect the normalized inline group, not an arbitrarily reselected DOM subtree.
Reuse X's normalization and source-map behavior where valid, but its current
single-subtree selection/eligibility checks cannot simply define all document
paragraph boundaries. Preserve inline font/style changes and zero-byte bidi markup;
keep paint separate. Prepare text through M and call paragraph.Run at the width
supplied by box flow.

Implement the agreed typed group-to-paragraph table and return only overall
layout extent to block flow. Preserve paragraph-local baseline conventions inside
the retained result and total vlist extent, including compatibility-policy trailing
spacing. No baseline translation is needed for the block-flow return. Avoid dummy
lines, inflated last-line heights, and applying line spacing twice.

Unit bridge: box used values are currently pixels/float32; paragraph geometry is
fractional big points/float64 with DU inputs. Specify conversion and checked
rounding at the adapter boundary. Retain precise paragraph-local geometry; do not
narrow all glyph coordinates to float32. Audit the sibling's float32 versus
float64 design discrepancy before treating either as permanent.

### 4. Retain and inspect a complete document result

Publish geometry, typed paragraph associations, source correspondence and paint
together after a successful invocation. Test anonymous groups, empty results,
trailing extent, resize, and failed runs without changing earlier snapshots.
Compose parent transforms once to place each paragraph's local output. Render
block frames and paragraph bounds first, then reuse frozen glyph outlines and
separate paint. Keep document rendering out of the paragraph engine.

Add source-to-box/paragraph selection to the TUI only when it helps inspect this
fixture. Existing single-paragraph workbench and replay remain useful controls.
Do not extend paragraph manifests to document replay implicitly; specify a separate
versioned document contract later if reproducible document persistence is needed.

### 5. Let the experiment select the next extension

After the first document works, choose one exposed limitation: mixed inline/block
normalization, computed-style context, DOM line-height, or atomic inline content.
Inline-blocks require the upstream width/descendant-resolution fixes and an honest
intrinsic-measurement API; a very-wide paragraph run is not automatically a correct
intrinsic width. Images require non-text inline material. Do not pull either into
the first experiment merely because callback types already exist.

## Original discussion questions (historical)

| Question | Working proposal |
| --- | --- |
| What is the first outer layout target? | A fixed-width continuous document with natural height; pages/columns later. |
| What owns a paragraph request? | A normalized inline group identified by BoxID, with its originating DOM associations. |
| Where should the first adapter live? | Thin experimental tytui integration code; general fixes upstream. Revisit package placement after the fixture exposes the contract. |
| What do we inspect first? | Box-tree structure and width/height handoffs with fake paragraphs, before real glyph rendering. |
| What remains deliberately parked? | Paragraph refinements, full CSS line-height extraction and new font discovery features unless required by this path. |

The extent-only return and invocation-scoped typed retention are agreed above;
the remaining questions in this table are working proposals. Decisions about
css-box-layout's core contracts should be reconciled with its existing roadmap,
not maintained as a competing specification here. Update this document as our
understanding changes, distinguishing observed behavior from agreed choices.

## Evidence and verification

For each experiment keep the HTML/CSS fixture, expected tree/IDs, paragraph-call
widths and independent geometry expectations. Cover errors and unsupported features
explicitly. Run owning-package tests and affected integration suites for each
implementation; run race/build/vet checks when retaining shared results or moving
boundaries. Preserve paragraph replay fixtures and rendering-bypass tests.

The initial 2026-11-12 planning pass was source inspection only. The 2026-11-15
review rechecked actual code, ordinary/race suites, builds and vet in tytui,
typography, khipu and css-box-layout, plus the ordinary w3c-dom suite. All passed;
temporary targeted probes exposed the policy/input gaps above. See the review
for exact scope, checkout state and remaining limits. No implementation changes
were made during this re-evaluation.
