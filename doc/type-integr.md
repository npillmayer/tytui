# From structured content to paragraph boxes

## Purpose and status

This document proposes an architecture; it does not describe a completed package
or freeze public Go APIs. The immediate goal is to extract reusable responsibilities
from tytui while keeping it an integration workbench with thin application wiring.
The shared styles, immutable font resources and OpenType adapter now live in
local `typography` packages. The root resolver now accepts injected loading and
coverage services. Discovery adaptation and retained proof/identity consumers are
also extracted. The color rendering bypass is implemented: M projects document
styles into color-free typesetter styles, while application wiring retains colors
separately for the renderer. Glyph-boundary color and legacy replay behavior are
preserved. The general annotation API is deferred. `typography/paragraph` now
assembles frozen lines into a naturally sized boxed vlist under an explicit
proof-compatibility policy. Tytui retains these placements and its glyph renderer
consumes them. `typography/paragraph.Run` now composes bidi preparation,
encoding, breaking, freezing and assembly from prepared `styled.Text`, direction
and zero-byte markup. It receives explicit constraints/policies and injected
khipu services. M prepares typography before P; DOM source maps and rendering
colors stay with the application. CSS-inspired baseline line-height is now an
explicit engine opt-in through Params.LineLayout, separately from shaping styles.
Default font/size supplies the strut; typed factor/length/normal values and byte
ranges govern line boxes after freezing. The DOM line-height adapter and CSS box
bridge remain future work. See
[the extraction plan](TYPOGRAPHY_EXTRACTION_PLAN.md) and
[final extraction contracts](TYPOGRAPHY_API_CONTRACTS.md#implemented-step-7-api).
The [paragraph-engine plan](PARAGRAPH_ENGINE_PLAN.md) records the extraction review
and incremental work toward boxed vlist output, starting from frozen lines.

The eventual layout core will assemble horizontal and vertical material in bounded
rectangular space, for UIs, browsers, and typesetters. Input is structured content,
often HTML/CSS; output is a box layout containing glyphs and non-discardable items
such as images. The developing `css-box-layout` module will supply CSS box-layout
behavior, but is not yet ready for integration.

The immediate scope is narrower: a **paragraph layout engine** that consumes
`styled.Text`, depends on khipu, and produces a **boxed vertical list of lines**.
It does not yet perform page breaking, general block formatting, or image layout.

## Paragraph layout engine

The proposed contract is:

> Given styled paragraph content, typography services, and layout constraints,
> produce positioned line boxes assembled into a vertical box.

Inputs comprise:

- A paragraph input envelope containing `styled.Text`, base direction, bidi markup
  boundaries, source correspondence, and paragraph-level settings.
- Layout constraints, initially the available line width and breaking/alignment
  policy. A future parent may impose a height budget; vertical fragmentation is
  not part of this first contract.
- An injected typography resolver mapping the relevant style projection to faces,
  sizes, and shaping features, with explicit font selection and coverage-fallback
  policies. Rendering/interaction properties are not inputs to the typesetter.
- Shaping and hyphenation services adapted to khipu's public contracts.

The engine owns this sequence:

1. Resolve the supplied typography styles into shaping-relevant runs.
2. Itemize runs by resolved font, script, language, direction, and shaping needs.
3. Encode horizontal paragraph material through khipu.
4. Break into lines and freeze the selected result before another breaker runs.
5. Establish line metrics, baselines, and interline spacing.
6. Assemble the line boxes and vertical spacing into a vlist, then box it.

The result exposes dimensions, line placements, baselines, and source correspondence.
The application pairs it with separately retained rendering properties; the
paragraph engine need not carry or interpret those properties.
It can retain frozen khipu content for glyph access, proof rendering, and diagnostics.
Rendering a result must not trigger reshaping or rebreaking.

The engine should not know whether a selected face came from a file, embedded
resource, fontfind, or a replay store. Khipu already separates `FontFace`, em size,
and `FontScale`; reuse those concepts rather than inventing another scaling model.

## Boxed vlist and the future layout core

The vertical-list representation is more general than paragraph layout. A paragraph
is one producer; later, block layout and non-text content can produce material too.
The shared box/material model should be independent of HTML and of khipu's internal
representation. A line box may retain a frozen-content payload while presenting
ordinary dimensions and a baseline to its parent.

A paragraph vlist contains line boxes plus vertical kern/glue. The assembled list
is enclosed in a box with dimensions and placements that a parent layout can use.
Kern denotes fixed displacement; glue can carry flexibility where the selected
assembly policy supports it. Detailed packing/stretch semantics remain to be specified.

Interline spacing, padding, and margins are not interchangeable:

- The paragraph engine controls line height, baseline spacing, and interline
  adjustments from the resolved paragraph/typography policy.
- Padding belongs to the enclosing box.
- CSS margin collapse depends on neighboring blocks and sometimes ancestors.
  It belongs in `css-box-layout`, which translates the resolved spacing into
  vertical material. A generic vlist assembler does not implement CSS collapse.

The future CSS layout engine supplies paragraph constraints and places returned
boxes among other material. Page/column fragmentation and bounded-height packing
remain future work. Opaque inline boxes, including images and other non-discardable
items, will also require a later extension; reserve an architectural place for
such material without claiming it is supported by the initial paragraph API.

## DOM-to-styled.Text boundary

The DOM adapter ends at **semantic paragraph content**, not font selection or
physical layout. It should be a reusable adapter, likely associated with cords'
HTML integration, rather than a dependency of the paragraph engine on w3c-dom.
Non-DOM clients can construct the same paragraph input envelope directly.

The adapter is responsible for:

- Selecting and traversing the paragraph subtree in document order.
- Obtaining resolved document properties and interpreting relevant HTML semantics,
  including language and directionality, through the owning DOM/CSS facilities.
- Applying explicit paragraph-boundary and whitespace policies. Tytui currently
  collapses source newlines and rejects newline-preserving modes; these are
  selected input policies, not an instruction to infer paragraphs from newlines.
- Producing `styled.Text` with typography styles, source mappings, and bidi markup
  boundaries. Paragraph-level settings stay separate where appropriate.
- Keeping rendering properties, such as resolved color, associated with logical
  text ranges on the input/application side, for delivery directly to the renderer.
  Such properties do not become styles or opaque capsules in the typesetter's text.
- Reporting unsupported content instead of silently losing its layout semantics.

CSS inheritance must distinguish computed values from expressions. A child inheriting
its parent's `font-size:150%` inherits the computed size; it must not multiply the
size again. The DOM/CSS side should supply computed typographic values where it has
sufficient context. Font-metric-dependent or layout-dependent values need an explicit
resolution context shared with typography/layout services. Do not make the generic
paragraph engine interpret arbitrary CSS strings. The precise computed-style/context
API is still a design task; current w3c-dom property lookup alone is not that engine.

Preserve coordinate distinctions across the boundary: extracted text/source spans
use UTF-8 byte offsets, while shaping cluster anchors use Unicode scalar indexes
under the shaping contract. Synthetic bidi markup must not shift source coordinates.

## Style projections and the rendering bypass

A DOM style boundary must not automatically become a shaping boundary. M defines
which properties belong in the typesetter's `styled.Text` through `ProjectStyles`.
X supplies computed CSS values; application wiring uses M's projection rather
than maintaining its own list of shaping properties.
Other properties bypass M, P, and Khipukamayuq and go to their eventual consumers.

| Property category | Examples | Path |
| --- | --- | --- |
| Font selection | Family, weight, slant, fallback preferences | X → typography styles → M selects faces. |
| Shaping/layout | Size, language, features, direction, spacing, line height | X → appropriate text or paragraph settings → shaping/bidi analysis/P. These can affect geometry without changing the selected face. |
| Rendering/interaction | Color, underline, links, mouse sensitivity | Input/application → renderer or interaction layer, separately from the paragraph typesetter. |

For `<span>f</span><span style="color:red">i</span>`, equal typography allows the
shaper to receive `fi` continuously. Color does not enter shaping-run equality.
Conversely, actual changes in resolved face, size, features, language, or bidi
context still require the appropriate treatment. Different font requests that
resolve to equivalent typography need not split a run.

The application retains effective rendering properties and their logical text
ranges alongside the paragraph result. The renderer receives both these properties
and the frozen glyphs with source correspondence. X can supply resolved values,
so the renderer need not interpret CSS. Non-DOM clients can supply the same
consumer-specific data directly. This does not require a universal range/payload
API or opaque styles carried through the typesetter.

Preserve the existing coordinate distinctions: extracted ranges use processed
UTF-8 bytes; glyph clusters use scalar anchors; DOM maps refer to decoded text-node
bytes, not raw HTML offsets. The application and rendering adapter compose these
maps through normalization and frozen output, including supported replacements.
The typesetter owns text-to-output correspondence, not color lookup. Rendering
must not reshape, rebreak, or depend on mutable DOM state for an old result.

The agreed color policy remains one color per glyph at its original logical
anchor. An `fi` ligature is never split into colors; repeated anchors share color,
and independently anchored marks may differ. Underline and interaction behavior
can be decided when those consumers are needed; no whole-cluster hit-region policy
is adopted here.

`Style.Color` and source/style snapshots remain document and replay values.
`typography.ProjectStyles` returns color-free `TypographyStyle` values, including
explicit domain identities where legacy replay or relative features require a
boundary. Itemization retains only this typography projection in its run styles.
Both synthetic and OpenType typesetter input are color-free, including legacy runs.
`Result.PaintSpans()` exposes a detached copy of the separate renderer snapshot;
proof rendering no longer reads colors from `Result.Input.Source/Styles`.
The [general annotation proposal](ANNOTATION_API_CONTRACT.md) remains **deferred**.

## Participating modules and adapters

| Module/entity | Responsibility and boundary |
| --- | --- |
| `w3c-dom` | Construct HTML/CSS DOMs; own cascade, property semantics, and HTML defaults. General CSS/HTML fixes belong here. Computed typography needs an explicit context/API beyond raw property lookup. |
| `cords` and an HTML adapter | Hold persistent styled text; adapt a selected DOM paragraph into typography-styled text, source maps, and paragraph metadata. Keep rendering properties on a separate path to consumers. Adapter placement is proposed. |
| Typography service (`typography`) | Resolve family/variant selection, typographic values, coverage fallback, and shaping-run itemization through small style/service interfaces. Keep DOM types out of its generic boundary. |
| `fontfind` | Discover/load font resources from configured providers; expose lookup diagnostics. Do not decide document styles or line layout. |
| `opentype` | Parse fonts, expose metrics/properties, and shape character runs into positioned glyph records. |
| OpenType–khipu adapter | Implement khipu font/shaping contracts using OpenType. Keep representation conversion separate from typography policy. |
| `uax` | Unicode boundaries and bidi levels/order, used through appropriate adapters. |
| `hyphenate` and its khipu adapter | Find word break opportunities and expose them to khipu's hyphenation contract. |
| `khipu` | Encode paragraph material using injected services, measure, break lines, and freeze results. Retain source/glyph provenance; do not interpret CSS decorations or interaction behavior. |
| Paragraph layout engine | Orchestrate typography, khipu breaking/freezing, line metrics, and assembly of a boxed vlist. Preserve correspondence to output glyphs/boxes, without carrying rendering properties. |
| Rendering and host interaction consumers | Receive effective properties separately from the input/application side and associate them with output through source correspondence. Own rendering and interaction behavior. |
| Shared box/material model | Express dimensions, baselines, placements, and horizontal/vertical material without HTML-specific policy. |
| `css-box-layout` | Eventually resolve surrounding CSS block geometry, padding/margin behavior, and placement of paragraph/other boxes. |
| `tytui` | User selection/configuration, invoking services, inspecting results, case replay/proofs, and reproducible integration regressions. Avoid owning general typesetting policy or algorithms. |

## Dependency structure and public contracts

Use X for the DOM adapter, M for typography resolution, P for paragraph layout,
and A for the OpenType adapter. These are architectural roles, not chosen package
names. Commit to OpenType as the initial concrete implementation while keeping
its concrete font types out of the public boundaries of X, M, and P.

| Component | Direct dependencies and injected services |
| --- | --- |
| Shared style definitions | Font requests and shaping/layout values, independent of DOM and concrete fonts; compatible with cords' style interface. No general rendering-property contract is required here. |
| X: DOM adapter | `w3c-dom`, `cords`, and shared style definitions. |
| M: typography resolver | `cords`, shared style definitions, `khipu/contracts`, and injected selection/coverage services. |
| A: OpenType adapter | `ot`, `otquery`, `otshape`, `khipu/contracts`, and any M service interfaces it implements. |
| Font discovery adapter | `fontfind`, M's selection interfaces, and A for constructing usable font faces. |
| P: paragraph layout engine | `khipu`, M, and shared box/material definitions; font and shaping services supplied through contracts. |
| Application or typographic engine | X, P, A, discovery configuration, and rendering; connects services and pairs frozen output with separately retained rendering properties. |

The existing dependency `khipu → cords + uax` remains. Khipu stays decoupled from
`otshape` and `hyphenate` through interfaces. The future CSS box engine consumes P
and the shared box interfaces; P does not depend on CSS block layout, X, or the TUI.

### Shared style vocabulary between X and M

`styled.Text` supplies styled storage; X and M share the meaning of font requests,
computed size, language, features, and other typography inputs. X emits requests,
not parsed fonts. M resolves these to faces and shaping parameters. Source
correspondence, bidi boundaries, and paragraph settings remain separate where
appropriate. Non-DOM clients can construct the same inputs.

Full CSS styles remain on the DOM/input side. Only their typography projection
crosses into the paragraph typesetter. Color and other rendering/interaction
properties travel directly to consumers through application wiring. There is no
need to define a shared annotation vocabulary for X and M or a capsule style for
Khipukamayuq. Shared document `Style` still includes color for compatibility and
replay, but M's `TypographyStyle` and the itemized typesetter styles do not. Layout
properties outside M's font/shaping responsibility remain P's contract.

### Font contracts between M and P

Initially reuse `khipu/contracts.FontFace`, its scaling/shaping contracts, and
resolved run-style concepts. M supplies faces and sizes through those contracts;
P consumes the services without requiring an `ot.Font`. Selection and coverage
may require an additional small query interface; define that requirement explicitly
instead of exposing an entire parsed font to M.

This deliberately makes M depend on `khipu/contracts`. That is acceptable for the
immediate paragraph-layout scope. Extracting these contracts into a foundational
module should wait for another substantial consumer, rather than introduce a
second abstract font model, metrics model, or scaling system now.

### Concrete implementation ownership

A retains the parsed `ot.Font` internally and implements the font/shaping services.
Its matching shaper knows how to access the backend face. OpenType-specific outline
access likewise belongs in a rendering adapter. Backend compatibility checks and
type assertions belong inside those adapters, not scattered through M or P.

The application injects A-backed services into M and P. P therefore need not import
A, although a composed application will naturally depend transitively on OpenType.
The synthetic backend remains a useful independent implementation for tests.

An intermediate extraction may keep an OpenType-specific implementation package
alongside M, provided M's public boundaries use the contracts. Package dependencies
and Go module dependencies are distinct: adapters may share a module without making
every package import concrete OpenType types. Eliminating every transitive dependency
is not the objective; preserving meaningful public boundaries is. No universal font
table abstraction, generic parser framework, or plugin framework is proposed.

## Architectural progress and next contracts

The extraction and color specialization establish the following tested boundaries:

1. Color-only changes preserve shaping context while retaining paint information.
2. Actual face/size changes create required boundaries.
3. Equivalent resolved typography does not split unnecessarily.
4. Ligatures and combining marks crossing color boundaries follow the agreed
   glyph-boundary policy.
5. Soft-hyphen context, coverage fallback, bidi markup, and source mappings survive
   the separation and freezing.

The bypass now keeps color out of typesetter styles and supplies it separately to
the renderer. Focused regressions cover DOM and non-DOM input, both backends and
breakers, legacy boundaries, relative features, detached rendering data, and replay.
Existing glyph/mark/discretionary regressions preserve source correspondence and
color behavior. No annotation package, payload schema, or replay version was added.

Compatibility cleanup is complete: consumers use shared styles, resources, resolver
and adapters directly; only replay retains explicit wire conversion. Dependency
checks guard the generic package boundaries. Headless line-to-vlist assembly is
implemented, and application results retain its placements for rendering. The
reusable styled-paragraph entry point is implemented too; tytui uses it directly.
Paragraph-engine step 4 adds explicit baseline-aligned line-height policies;
headless pipeline callers opt in, while old replay versions retain compatibility
geometry. Version 3 persists the new values with numeric placement evidence.
Next is DOM line-height extraction and the CSS box bridge when needed.
Computed-style context and flexible vertical packing remain design work.
Underline and interaction consumers can be added when required. The general
annotation API and its proposed decoration/hit-region policies remain deferred.
