# Document flow and normalized input

`documentlayout` implements pipeline integration stages 3D–3F and 4C–4D. It builds typed CSS
render input from a selected styled block, lets `css-box-layout` normalize the
structure, extracts its anonymous inline groups, and connects `FlowLayout` to
typography's prepared paragraph engine. CSS core schedules and sizes nested atomic
interiors. Geometry, retained paragraphs, sources and paint are published together
after whole-document success. No glyph coverage validation or font discovery is added.

```go
document, err := documentlayout.Build(ctx, htmlDocument, selectedNodeID)
// Handle err before using document.
root := document.Root() // borrowed, immutable structural input for CSS core
for _, group := range document.Groups() {
    input, err := document.Extract(ctx, group.GroupID)
    // Handle err. input addresses exactly one normalized inline group.
    // Pass input.Text, input.Styles and input.Anchors to ItemizeInline.
    // Resolve the strut from input.Paragraph.Style, not its first text run.
}
```

To lay out a document, supply an initialized `typography.Resolver` (from
`typography.LoadSettings`) and a factory returning fresh exclusive streaming
shapers. The caller supplies fonts suitable for the text; the existing itemizer
checks remain in place pending their separate relocation.

```go
next, err := document.Layout(ctx, documentlayout.LayoutRequest{
    Width: 640, // containing-block width in CSS pixels; zero is valid
    Breaker: paragraph.KnuthPlass,
}, documentlayout.Config{Resolver: resolver, NewShaper: newShaper})
if err != nil {
    return err // retain the previous complete snapshot
}
current = next // publish only the whole successful document
```

`Result.Document()` retains the immutable normalized input. `Geometry()` returns
a detached CSS geometry map; `Groups()` lists the source-order group identities;
`Group(BoxID)` returns a detached `GroupResult` containing extracted `Input` and
the concrete `paragraph.Result`. Its `Input.OwnerID` identifies the content origin
for paragraph placement. Nested callbacks execute before containing callbacks,
but retention is keyed by group BoxID, not callback order. Every prepared atomic
BoxID is mapped through `AtomicSources` and placed exactly once, including zero
sizes. Repeated callbacks fail explicitly; speculative layout is unsupported.

Frozen paragraphs and vlists are shared immutable objects. Mutable result
envelopes, source/paint/layout slices and paragraph diagnostics are copied by
accessors. No preparation handle or partial invocation table escapes. Each call
constructs fresh working state; a failed resize cannot modify an older snapshot.
Keep the tree and font/scale services stable, and serialize layouts sharing a
resolver. Context is checked at callback entry/completion and document publication;
synchronous CSS flow, bidi and breaking retain their cancellation limits.

Stage 4C separates three invocation-local tables: completed group preparations,
accepted final paragraphs, and final-callback attempts. A private helper validates
both the normalized group node's identity and OwnerID before extracting,
itemizing and preparing it once. Cache hits perform the same identity check.
The retained extraction supplies the same anchors, source maps, paint, isolates
and inherited owner context to measurement and final layout. Markup is consumed
during preparation; CSS request views and measurement answers are not retained.

Repeated measurement with different atomic ranges recomputes widths through the
same prepared paragraph, without accepting a final callback. Final callbacks still
allow only one attempt per group, including failed attempts. Failed preparations
are not inserted and can be retried; completed preparations may survive another
callback's failure until the invocation is discarded. No private preparation is
published. Independent `Document.Layout` calls prepare afresh; there is no cache
on `Document` and no cross-invocation resize reuse. Operations sharing an invocation
or prepared handle remain serialized under stable tree/service lifetimes.

Stage 4D connects the invocation as both `InlineLayouter` and `IntrinsicMeasurer`.
`MeasureInline` gets each atomic occurrence's intrinsic border bounds and resolved
horizontal margins from CSS views. It converts each primitive independently to DU,
checks completed signed sums, and delegates to the cached prepared paragraph.
Border bounds already contain padding/borders; neither those edges nor owner edges
are added again. Only the final nonnegative intrinsic summary is converted back
to CSS pixels, with finite-domain and float32-narrowing checks.

CSS core recursively measures atomic interiors, selects shrink-to-fit widths and
resolves descendants before final callbacks. Tytui does not recurse or choose
widths inside its callbacks. Fixed-width flow makes no intrinsic callbacks and
still prepares on demand. Failed measurement publishes no document; completed
private preparations are discarded with the failed invocation. Intrinsic widths
describe natural break opportunities, not breaker fit guarantees: the existing
zero-skip Knuth–Plass policy can select overfull output even at min-content width.

The callback uses normal `CSSLineHeight`, the explicit owner's typography for the
strut, `NoHyphenation`, `AlignBottomMarginEdge` and zero horizontal side skips for
both breakers. Fixed-pixel and auto-width inline-blocks are supported. Positive
overfull output is valid; unsupported negative completed line widths are errors.
Empty atomic interiors currently reserve their normal paragraph strut.

Convert each CSS border dimension and margin separately with
`round(px * 49152)` (ties away from zero), checking the finite DU domain before
casting. Derive advances only after conversion. `dimen.PX` is not a CSS-pixel unit.
Paragraph geometry stays float64 big points. Only CSS-facing extents and border
origins use `points * 4/3`, checked before and after float32 narrowing. The error
budget is half a DU per primitive plus float32 rounding at the CSS boundary.
CSS core retains its original prepared border dimensions, without a DU round trip.

Paragraph and atomic origins are relative to the owner's **content** origin.
CSS geometry uses `RelativeTo` content-coordinate parents. A future document
renderer must compose those origins once and use retained line/material positions;
it must not add owner edges twice, restack lines, or remeasure text. The diagnostic proof below implements those transforms; it is not a full CSS
painting or stacking-context model.

`Group` contains the actual `GroupID`, `OwnerID` and inherited paragraph
`NodeSource`. Anonymous owners inherit the nearest source block's paragraph
context. Split inline boxes retain one source `NodeID` but have distinct BoxIDs.
DOM IDs and paths are locators, not normalized box identities. CSS NodeIDs use
DOM preorder ID + 1, reserving zero for anonymous boxes; callers use `NodeSource`
and `TextSource` lookups rather than depending on that encoding.

`InlineGroup` contains normalized UTF-8 `Text`, complete document `Styles`,
separate `Paint`, typed `LineLayout`, zero-byte `Isolates`, atomic `Anchors`,
`TextSources`, and `AtomicSources`. Every range uses the same emitted byte stream.
Each inline-block emits one U+FFFC OBJECT REPLACEMENT CHARACTER; its entire subtree
is skipped by the containing extraction. Its own normalized groups appear
separately in `Groups()`, including empty and nested atomic interiors. Atomic IDs
are nonzero occurrence IDs local to one group, assigned in emission order.
`AtomicSource` supplies the explicit AtomicID-to-BoxID association and original
element locator. Unattached literal U+FFFC remains ordinary source text.

Text provenance addresses **decoded DOM text-node bytes**, including entity
decoding, not offsets in HTML markup. `TextSpan` also identifies its normalized
text BoxID. Normal whitespace is collapsed across nodes and trimmed at group
edges; atoms count as content. Collapsed space uses its first source unit's style,
paint and bidi scope, matching `htmlinput.Extract`. Several source spans may map
to one collapsed output space; omitted whitespace has zero output width. Atomic
marker ranges are covered only by `AtomicSources`, never invented text-node bytes.
Use both tables when accounting for output coverage.

`htmlinput.Document.ResolvedStyles` supplies existing typography, relative font
sizes, language and direction. `paragraphtext.Normalize` supplies the whitespace
transform, and `htmlinput` supplies isolate markup construction. The legacy
single-paragraph extractor and replay formats are unchanged.

Initial policy is bounded:

- Select a supported `display:block` element. Descendants may be supported block,
  inline or inline-block elements. Hidden content, explicit breaks, replaced
  elements, tables, list-item and other display modes are rejected.
- Inline-block and block widths may be definite nonnegative px or auto.
  Box margins, padding and visible border widths use px (unitless zero is
  accepted); margins may be signed/auto. Font sizing retains htmlinput's existing
  relative and absolute-unit support. CSS geometry remains in CSS pixels; the
  callback performs checked px/DU conversion at the paragraph boundary.
- Normal whitespace, normal line-height, normal/isolate bidi, static flow and
  ordinary spacing/wrapping are supported. Height/min/max constraints, floats,
  automatic hyphenation and richer policies are rejected. Border width/style
  longhands and their supported four-side shorthands work; the existing DOM
  parser does not support the combined `border` shorthand.
- Ordinary inline edges must be zero. An inline with no emitted content and a
  font request different from the paragraph strut is rejected as a metric-bearing
  empty inline. Same-font empty wrappers may disappear. Coincident nested bidi
  boundaries remain unsupported by the existing zero-byte markup contract.
- `LineLayout` contains normal values over emitted ranges, including markers;
  paragraph assembly uses atomic geometry instead of applying text leading there.

Build consumes a stable `htmlinput.Document` and retains detached source/style
values, not its DOM pointers or mutable slices. The caller may release that input
after success. `Root()` and everything reachable from it must remain read-only.
Other accessors return values or detached slices; each extraction returns new
caller-owned slices. `BidiMarkup()` snapshots isolate boundaries. Cancellation or
failure returns no partial document/group; synchronous style resolution, CSS
normalization and whitespace transformation retain their cancellation limits.

Tests cover split/hoisted structure, inherited anonymous-owner contexts, nested
and object-only groups, transformed byte correspondence, literal markers,
multibyte entities, isolate boundaries, styles/paint/layout alignment, typed box
edges, source removal and detached outputs. Text-only fixtures are compared with
the established extractor. A synthetic typography/paragraph composition test
checks both breakers and explicit owner struts without adding coverage policy.
Callback tests cover nested scheduling, exact edge/margin accounting, anonymous
owner/ancestor transforms, zero-sized prepared boxes, zero available width,
overfull output, signed margins, unquantized CSS border sizes, conversion limits
and rounding budgets. Late backend failure, late cancellation, publication-time
cancellation, repeated callbacks, detached accessors and resize exercise the
transactional ownership contract. TUI and replay paths are unchanged.

## Retained document proof

```go
services := render.Services{Backend: backend, Identity: identity}
proof, err := documentlayout.BuildProof(ctx, current, services)
// Handle err. proof is detached absolute geometry in float64 big points.
err = documentlayout.WriteSVG(ctx, writer, current, services)
```

`typography/render.BuildParagraphProof` owns the reusable frozen-glyph walk,
visual fragment ordering, original UTF-8 paint anchors and outline positioning.
The existing paragraph SVG delegates to that same walker without changing its
public proof records or replay evidence. `documentlayout` adds content-parent
transforms, CSS border/content rectangles and original DOM source correspondence.
No style resolution, font loading, metric query, shaping, breaking or line stacking
occurs while constructing a proof. Caller-supplied identity/outline services must
remain usable for the retained faces. No backend is selected implicitly.

`Proof.Glyphs` contains GroupID, OwnerID, line/knot indexes, glyph/cluster IDs,
original group-relative SourceByte, font hash, paint and covering TextSpan records.
For identity spans, DOM byte = NodeStart + SourceByte - Start; collapsed spans
retain all corresponding source records and require their transform semantics.
`Proof.Atomics` retains the separate element/BoxID/AtomicID association and precise
paragraph border origin. `Proof.Boxes` preserves CSS core dimensions. Small
quantization differences between precise paragraph origins and CSS summaries
are expected under the documented conversion budget. Signed positions remain
signed; rendering never sorts items by X.

Proof construction returns no partial object on error/cancellation. SVG generation
buffers a complete proof before writing; destination write failures can still leave
partial bytes. The SVG shows diagnostic geometry and glyph outlines in source-group
order, not CSS paint order for overlapping/positioned/stacking contexts. There are no
background/border paint semantics, image layout, font embedding or document replay.

See [the fixture and reproduction commands](testdata/README.md) and
[the sample SVG](testdata/nested-firstfit.svg). Tests check a typed nested tree,
independent synthetic ancestor/multiline/bidi coordinates, both breakers, the real
nested HTML/CSS fixture, retained source/paint after DOM removal and resize, and
Latin ligature/pointed Hebrew glyphs against existing independent HarfBuzz captures.
No new glyph coverage validation is introduced; relocation of existing itemizer
checks remains deferred. Auto-width tests cover intrinsic bounds, signed margins,
primitive rounding, shrink-to-fit below/inside/above the intrinsic interval,
preparation reuse, late measurement failure/cancellation and unchanged fixed-width
flow. Broader nested-auto-sizing and lifetime evidence remains stage 5.
