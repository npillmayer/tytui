# Finite-height document composition

Proposed plan, 2026-11-15. This expands step 4 of the
[integration review](INTEGRATION_REVIEW_2026-11-15.md). No implementation changes
are part of this document.

Start by observing the existing continuous document layout through a finite
viewport. Implement steps 1–3 below, inspect the evidence, and then decide which
pagination experiment to pursue. A viewport does not change line breaking,
vertical stacking, or the retained document. Pagination will require an explicit
composition contract, rather than clipping the same picture into page-sized strips.

## Current foundation and gaps

| Implemented boundary | What it provides | What remains missing |
| --- | --- | --- |
| `documentlayout.LayoutRequest` | Containing width in CSS pixels and breaker selection | No viewport height or retained constraint record in `Result` |
| CSS `FlowLayout` and geometry table | Continuous block flow; border/content rectangles relative to parent content origins | No page sequence, fragment continuations, or legal vertical break protocol |
| `paragraph.VList` | Immutable frozen content, placed lines, atomic placements, and `Materials()` containing line boxes and fixed kerns | No target-height packing or fragment-boundary spacing policy |
| `documentlayout.Result` | Source correspondence, paint, retained paragraphs and CSS geometry | No headless overflow assessment |
| `BuildProof` / `WriteSVG` | Absolute document proof, glyphs and retained image payloads | Proof bounds mix several kinds of geometry; export expands to the whole proof rather than showing a viewport |
| F7 document workbench | Independent document runs/history and captured exports | Width only; the terminal's scrolling inspector is not a physical layout viewport |

Source references: [layout](../documentlayout/layout.go),
[retained result](../documentlayout/result.go),
[proof composition](../documentlayout/proof.go),
[SVG export](../documentlayout/svg.go), and
[document workbench](../document_workbench.go).

The upstream paragraph box's `Extent` is its total consumed vertical extent.
`Height()` is the signed distance above its reference baseline, **not** total
height. `Materials()` describes an already placed natural vlist; the existence of
line/kern entries does not establish fragmentation semantics. Likewise, the CSS
context's rectangular containing block does not establish pagination support.

Keep three measurements distinct:

- **Consumed flow extent:** the space used by continuous stacking, including
  P-owned trailing spacing and the CSS core's current edge/margin treatment.
- **Layout overflow:** retained box, line and atomic geometry protruding beyond
  a viewport. Descendants may protrude even when their ancestor fits.
- **Paint overflow:** glyph outlines or other rendered content extending outside
  layout geometry. This requires renderer evidence and is optional initially.

Neither a union of proof bounds nor a sum of descendant heights measures consumed
flow. Current CSS stacking does not collapse margins; negative margins and
overlaps also prevent treating geometry sorted by Y as document flow order.

## 1. Assess viewport overflow headlessly

Add a read-only assessment of a completed `documentlayout.Result`, with a separate
viewport constraint. Keep `LayoutRequest` width-driven at this stage. A height
change must not call `Build`, `Layout`, font loading, shaping, breaking, or image I/O.

The initial viewport starts at document origin `(0, 0)`, uses the containing width
captured for that result, and accepts an optional positive finite height in CSS
pixels. An absent height means unbounded vertical observation; zero, negative,
NaN and infinite heights are invalid. Preserve the existing zero-width layout
contract. This is an experiment constraint, not CSS `height`, `max-height`, or
`overflow` support. Scrolling origins and resizing an independent viewport width
can wait.

Retain the original containing width with the headless result, or in an explicit
immutable result/constraint envelope. Do not infer it from the root frame: an
explicit root width can differ from its containing width. Do not consult current
TUI controls when assessing an older result.

Extract the pure parent-content coordinate composition from `BuildProof` for
reuse by the analyzer and renderer. Preserve its missing-parent/cycle checks and
operation order. Compose CSS geometry in fractional big points (`1px = 0.75bp`)
and translate P's existing big-point placements once. Avoid integer DU rounding
or querying font outlines to obtain layout diagnostics.

The detached report should contain:

- The viewport and the result/constraint association used to produce it.
- Per-edge protrusion distances and classifications: inside, crossing an edge,
  or wholly outside. Exact edge contact fits. Keep raw distances; document and
  test any numerical tolerance, accounting for upstream float32 geometry without
  using tolerance to conceal meaningful overflow.
- Box identity and source locator; for lines, group/owner identity and original
  `paragraph.Line.Index`; for atomics, group-local `AtomicID` and mapped `BoxID`.
  BoxIDs are snapshot-local and are not persistent document identities.
- Separate box, line and atomic observations. Use retained target line boxes and
  occupied horizontal extents as separately labelled evidence; retain the existing
  breaker fit diagnostics. Do not sum overlapping ancestor/descendant reports or
  describe their count as the number of lost items.

Include all four edges, even though finite height motivates the work. Report
zero-area objects' positions without counting them as positive-area overflow.
An empty document remains a valid assessment. No implicit clipping, source-range
rewriting or “number of pages required” estimate belongs in this report.

**Acceptance:** small independently calculated geometry cases cover exact fit,
partial crossing, wholly below, fractional units, negative origins/margins,
overlapping descendants, overfull horizontal lines, and zero/tall atomic leaves.
Assessing several heights preserves the same frozen content and placements and
works without renderer services. Missing/cyclic geometry is an error, not an
incomplete successful report. Tests must not derive every expected coordinate
through the same transform helper under test.

## 2. Show the viewport in retained SVG proofs

Add an opt-in diagnostic export path accepting the captured viewport/assessment.
Keep the existing `WriteSVG` default and fixture output unchanged. Draw the
viewport boundary and distinguish outside/crossing material; keep the complete
document visible by taking the union of the viewport and proof for the SVG
viewBox. Labels should say “viewport overflow,” not “page break.”

The geometry-only assessment remains usable if outline rendering is unavailable.
If paint overflow is added, expose it separately using the renderer's available
evidence; do not relabel existing proof bounds as exact ink bounds. A later
clipped preview may be useful, but it is a display option with the same retained
result, not a composition operation.

**Acceptance:** verify viewport coordinates and overflow markers numerically,
including fractional conversion and negative positions. Existing unannotated
SVG fixtures remain byte-identical. Both breakers, mixed-direction paragraphs,
rectangles and local images retain their current geometry and colors. Export
after deleting/changing source image files must still use retained bytes.

## 3. Expose height as a document observation control

Add a document-mode viewport-height input: blank means unbounded, otherwise a
positive CSS-pixel value. Keep this independent of terminal rows and paragraph
mode. Show the captured viewport, vertical overflow amount, and inspectable
source-associated observations alongside the existing document diagnostics.

Editing height reassesses the displayed retained result. Width, source, fonts and
breaker changes retain their current layout lifecycle. Keep the previous usable
assessment on invalid height input and explain which viewport it describes.
History entries keep their captured observation settings; an explicit height edit
may create a new observation of that result without changing the saved layout.
Exports capture result plus viewport/report together when requested. If assessment
is asynchronous, reject completions for a superseded result or height revision.

**Acceptance:** show the same result fitting, crossing and lying partly below
three chosen heights without a new layout invocation or resource acquisition.
Cover history selection, failed edits, pending layout, stale completions, export
capture, and switching between paragraph/document modes. Preserve paragraph
replay v1/v2/v3 and the existing document scheduler's serialization rules.

At this milestone we have a complete finite-viewport experiment. Use a checked-in
HTML fixture with several blocks, nested content and an explicitly sized leaf to
record what actually exceeds the viewport. Richer CSS line-height extraction,
margin collapse and baseline propagation remain separate work unless these
observations make one necessary.

## 4. Settle the fragmentation contract before paginating

Review the viewport cases and choose a deliberately small supported page model:
equal-width/equal-height pages, normal block flow, and indivisible atomic objects.
There is no need for a new general annotation API or a generic object framework.
Document these decisions before adding a page-producing API:

| Decision | Recommended initial direction |
| --- | --- |
| Composition owner | CSS-aware document composition owns block traversal, page allocation and continuation boxes. Establish the boundary in `css-box-layout`; introduce a separate general-purpose compositor only if a concrete reusable contract warrants it. |
| Paragraph contribution | P owns line/vlist fragment metrics and legal line-boundary material slices. Reuse frozen lines at the same width; do not rerun paragraph layout for each page. |
| Break opportunities | Begin between supported sibling blocks. Later add between retained paragraph lines. Never split glyphs or atomic objects at an arbitrary Y coordinate. |
| Vertical spacing | Specify which interline kerns/gaps survive each boundary, first/last-fragment extent and baseline, and trailing spacing. Natural-vlist placement alone cannot answer this. |
| CSS edges and margins | Specify boundary margin treatment and decoration continuation. Initially restrict cases whose semantics have not been implemented; do not invent margin collapse in tytui. |
| Oversized material | Place an indivisible oversized item once on an otherwise empty page and report overflow. Guarantee progress rather than generating empty pages indefinitely. |
| Continuation identity | Retain original box/group/source identity plus page and fragment ordinal, original line indexes and page-relative transforms. Never mutate shared frozen ranges or rebase logical source offsets as if pages were new paragraphs. |
| Rendering and resources | Share immutable frozen content, fonts, paint/source maps and image bytes. Render selected retained material through a fragment-aware boundary. Color continues to bypass typesetter styles. |

Source identity must survive bidi visual ordering, collapsed whitespace and
discretionary replacements. A fragment's correspondence may consist of multiple
source spans; a visual slice is not necessarily one contiguous original HTML
range. Preserve the existing distinction between decoded DOM bytes and HTML bytes.

The current one-final-callback-per-group integration cannot simply be called again
for each page. Define an explicit retained fragment result instead. Neither
`InlineExtent` nor `VList.Box()` is that result. Variable page widths, floats,
columns, break constraints such as widows/orphans, and CSS page styling remain
explicitly unsupported until separately designed.

## 5. Prototype whole-block page composition

Start with a flat sequence of supported blocks, zero external vertical margins,
and fixed equal page widths. Consume blocks in normalized tree flow order, moving
a whole block to the next page when it does not fit. Keep paragraphs and atomic
leaves indivisible. Produce retained page/fragment placements and explicit
oversized-item diagnostics; do not create pages by cropping a continuous SVG.

This deliberately limited prototype tests page identity, progress and payload
lifetime before paragraph continuation. Nested fragmentation and nonzero boundary
margins must report the limitation until their policies exist.

**Acceptance:** exact fits, multiple pages, an oversized block, empty input and
deterministic ordering. Every input occurrence appears once; no accidental empty
pages or duplicated resources. Retained paragraph content and legacy proofs/replay
remain unchanged. General composition tests belong in the owning library, with
tytui tests exercising adapters and source correspondence.

## 6. Add paragraph continuation, then nested block continuation

In P, add the smallest immutable fragment-view/measurement contract needed to
select retained lines and their owned vertical material. Settle gap and edge
policies from step 4 in focused upstream tests. Keep original frozen line indexes
and atomic associations. Rendering should consume the same view, rather than
having tytui splice private vlist arrays or duplicate the placement algorithm.

Integrate these opportunities into document composition. Only after flat paragraph
continuation works should CSS core build nested ancestor continuation boxes and
apply the agreed margin/decoration policy. Test that each line and atomic
occurrence appears once, fragment extents match rendering, and both breakers'
retained source/paint correspondence survives page changes. Use pointed Hebrew,
mixed direction and available discretionary fixtures at the appropriate upstream
boundary without relaxing the document profile's hyphenation restrictions.

## Ownership and validation

| Module | Responsibility in this plan |
| --- | --- |
| `w3c-dom` | Parse/cascade CSS; add break-related property semantics only when the supported composition policy requires them. No page placement. |
| `css-box-layout` | CSS geometry, flow order, block edges/margins and ancestor continuation; reusable document-composition fixes belong here or in an explicitly agreed compositor. |
| `typography/paragraph` (P) | Immutable line/vlist geometry, fragment measurements and vertical-material policies. No DOM, page allocation or paint. |
| `khipu` | Horizontal breaking, frozen glyph/atomic material and provenance. No viewport or page policy. |
| `typography/render` / document renderer | Retained fragment rendering, paint bypass and resource lifetime; no reflow. |
| `tytui/documentlayout` and TUI | Join upstream geometry to source/diagnostic records, provide experiment controls and reproducible proofs. Keep reusable fragmentation algorithms upstream. |

For steps 1–3, run focused geometry/export/workbench tests, then tytui's ordinary
and race suites, build and vet checks. Preserve existing proof fixtures and replay
tests as compatibility controls. For later steps, add focused owner-package tests
and run the corresponding typography/css-box-layout suites alongside integration
tests. Do not broaden dependencies or change paragraph replay to store a viewport.

There is still no document replay format. Checked-in source fixtures, captured
width/height/profile/breaker/font identities and retained export tests suffice for
the first milestone. Specify document replay separately when portable document
cases are needed; it must eventually include fragment constraints and immutable
leaf identities as well as fonts. This plan records proposed work, not new test
results or a claim of CSS fragmentation conformance.
