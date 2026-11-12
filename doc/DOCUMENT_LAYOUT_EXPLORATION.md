# From a styled document to positioned paragraph boxes

Living discussion document, started 2026-11-12. Direction agreed: explore document
layout from the top down, meeting the existing paragraph/vlist API. Detailed
paragraph improvements, including [DOM line-height extraction](DOM_LINE_HEIGHT_PLAN.md),
are parked unless a concrete document-layout experiment requires them.

Tytui remains an experimental integration laboratory. The immediate outcome is a
small, inspectable document-layout example and evidence about the participating
modules—not a browser, page compositor, or end-user document editor.

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
Flow layout: position blocks; request inline layout at an available width
    │
    ├── Inline-group adapter: collect text/styles/source correspondence
    │       │ M prepares typography; paint remains separate
    │       ▼
    │   typography/paragraph: shape → break → freeze → boxed vlist
    │       │ retained paragraph-local line geometry and glyphs
    │       ▼
    └── Line-box summaries + retained paragraph reference
    ▼
Document result: box geometry + paragraph placements + source references
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

## What exists today

Inspected the sibling sources and their design notes; these are observations, not
claims that the full document pipeline has been validated.

| Boundary | Current surface | Integration question |
| --- | --- | --- |
| Styled document → render input | w3c-dom provides styled nodes and property services. css-box-layout defines StyNodeView, but BuildLayoutTree currently accepts concrete RenderNode. | Which minimal adapter is actually needed? Do not mistake inherited property strings for fully computed CSS values. |
| Render input → box tree | BuildLayoutTree implements anonymous wrapping and split/hoist machinery. | Constructors currently omit style transport; text leaves use source 0 and node-local byte ranges. Identity/style plumbing must precede meaningful document integration. |
| Box tree → used dimensions | ResolveUsedValues accepts containing-block and font-size context. | Establish a supported computed-value subset, including unresolved percentages versus definite lengths. |
| Block flow → paragraph request | InlineLayouter.LayoutInline receives an anonymous inline root, float32 maxWidth, and AtomicSizer. | Adapt the complete ordered group, preserve source correspondence, and specify units and errors. |
| Paragraph result → block flow | InlineLayouter returns LineBox records with Frame, Baseline and opaque Payload; flow derives content height from line extents. | Preserve vlist height, reference baselines and retained payloads exactly once. |
| Flow → document output | LayoutResult has geometry and line tables; geometry is also written to nodes. | Clarify coordinate ownership, immutable snapshots and retention for every paragraph group. |

The sibling's authoritative design is
[CSS-Algo-Overview.md](../../css-box-layout/doc/CSS-Algo-Overview.md); its
[Current-Checklist.md](../../css-box-layout/doc/Current-Checklist.md) records gaps.
The implementation currently supports a much narrower surface than a complete CSS
engine. Its outer-layout contract is LTR; paragraph bidi support does not establish
RTL block-layout support.

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

## First experiment: a bounded continuous document

Recommended starting fixture: a block root with a fixed available width, two
paragraphs, and one nested block containing another paragraph. Add basic padding
and borders after the zero-edge case works. Use normal-flow LTR block layout,
explicitly supported widths, and additive margins only where the upstream contract
supports them. No pagination, floats, positioned elements, flex/grid, margin
collapse, inline-blocks, images or intrinsic sizing in this first experiment.

Use deterministic fake inline results first: known height/baselines at two known
widths. This isolates document structure and positioning from typography. Then
replace the fake with the existing synthetic paragraph backend, followed by one
real-font example. Existing paragraph spacing can remain an explicitly named
compatibility policy; do not present the experiment as full CSS line-height layout.

Success means we can trace a source node through box generation, available width,
paragraph request, returned vlist and final placement, with independently specified
coordinates. A narrower parent must trigger a fresh paragraph layout and update
following block positions. Prior document results must remain inspectable.

## Proposed sequence, open for revision

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

Check empty block normalization and line retention for anonymous blocks. Current
flow stores line tables only for non-anonymous owners; document rendering still
needs access to results for anonymous inline groups. Agree an upstream result
contract or an explicit adapter-owned result table keyed by group BoxID. Do not
lose paragraphs that happen not to have a named block owner.

### 3. Meet the paragraph input API

Collect the normalized inline group, not an arbitrarily reselected DOM subtree.
Reuse X's normalization and source-map behavior where valid, but its current
single-subtree selection/eligibility checks cannot simply define all document
paragraph boundaries. Preserve inline font/style changes and zero-byte bidi markup;
keep paint separate. Prepare text through M and call paragraph.Run at the width
supplied by box flow.

Define a small retained association: inline-group BoxID → paragraph result plus
source mapping and rendering data. Return line summaries with references to that
result. Declare baseline convention explicitly and preserve total vlist extent,
including any compatibility-policy trailing gap. Avoid encoding the vlist height
as an extra visible line or applying line spacing twice.

Unit bridge: box used values are currently pixels/float32; paragraph geometry is
fractional big points/float64 with DU inputs. Specify conversion and checked
rounding at the adapter boundary. Retain precise paragraph-local geometry; do not
narrow all glyph coordinates to float32. Audit the sibling's float32 versus
float64 design discrepancy before treating either as permanent.

### 4. Retain and inspect a complete document result

Snapshot the geometry and paragraph associations needed by a diagnostic renderer.
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

## Questions for our next discussion

| Question | Working proposal |
| --- | --- |
| What is the first outer layout target? | A fixed-width continuous document with natural height; pages/columns later. |
| What owns a paragraph request? | A normalized inline group identified by BoxID, with its originating DOM associations. |
| Where should the first adapter live? | Thin experimental tytui integration code; general fixes upstream. Revisit package placement after the fixture exposes the contract. |
| What do we inspect first? | Box-tree structure and width/height handoffs with fake paragraphs, before real glyph rendering. |
| What remains deliberately parked? | Paragraph refinements, full CSS line-height extraction and new font discovery features unless required by this path. |

These are proposals for discussion, not frozen upstream decisions. Decisions about
css-box-layout's core contracts should be reconciled with its existing roadmap,
not maintained as a competing specification here. Update this document as our
understanding changes, distinguishing observed behavior from agreed choices.

## Evidence and verification

For each experiment keep the HTML/CSS fixture, expected tree/IDs, paragraph-call
widths and independent geometry expectations. Cover errors and unsupported features
explicitly. Run owning-package tests and affected integration suites for each
implementation; run race/build/vet checks when retaining shared results or moving
boundaries. Preserve paragraph replay fixtures and rendering-bypass tests.

This initial document is based on source inspection. No implementation changes
or runtime verification were performed for this planning pass.
