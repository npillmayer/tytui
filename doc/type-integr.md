# From structured content to paragraph boxes

## Purpose and status

This document proposes an architecture; it does not describe a completed package
or freeze public Go APIs. The immediate goal is to extract reusable responsibilities
from tytui while keeping it an integration workbench with thin application wiring.
Package names and ownership of new shared interfaces remain to be decided.

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
- An injected typography resolver mapping styles to faces, sizes, shaping features,
  and paint attributes, with explicit font selection and coverage-fallback policies.
- Shaping and hyphenation services adapted to khipu's public contracts.

The engine owns this sequence:

1. Resolve typography and project document styles into shaping-relevant runs.
2. Itemize runs by resolved font, script, language, direction, and shaping needs.
3. Encode horizontal paragraph material through khipu.
4. Break into lines and freeze the selected result before another breaker runs.
5. Establish line metrics, baselines, and interline spacing.
6. Assemble the line boxes and vertical spacing into a vlist, then box it.

The result exposes dimensions, line placements, baselines, and source correspondence.
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
- Producing `styled.Text` with document-style annotations, source mappings, and
  bidi markup boundaries. These may be separate fields in the paragraph envelope;
  not all metadata belongs in character styles.
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

## Document styles, shaping runs, and paint spans

Keep three representations distinct:

| Representation | Purpose |
| --- | --- |
| Document-style spans | Preserve semantic properties and source provenance. |
| Shaping runs | Group text by resolved typography and necessary shaping context. |
| Paint spans | Retain color and other rendering attributes associated with shaped output. |

Their boundaries need not coincide. Color-only changes should not automatically
break shaping. Conversely, a strong/bold annotation need not create a boundary if
it ultimately resolves to the same face and shaping parameters. Actual changes
in face, size, features, script, language, or direction require the appropriate
run treatment; adjacent runs can only be merged when their shaping context permits.

The typography service resolves family/variant sets (regular, italic, bold, etc.)
and coverage fallback, then exposes shaping-relevant parameters separately from
paint. Fontfind locates resources; it does not own CSS inheritance or paragraph
coverage selection. Existing style-to-font mapping should evolve into this reusable
service rather than grow inside the workbench.

A paint boundary at a grapheme boundary can still fall inside a ligature spanning
several graphemes. We must choose a policy for such glyphs: assigning one paint,
clipping outlines, or deliberately preventing the ligature are different behaviors.
This document does not select one. Simply removing color from style equality would
lose information unless paint spans and their source-to-glyph correspondence survive.
Combining sequences and fallback boundaries require equally explicit treatment.

## Participating modules and adapters

| Module/entity | Responsibility and boundary |
| --- | --- |
| `w3c-dom` | Construct HTML/CSS DOMs; own cascade, property semantics, and HTML defaults. General CSS/HTML fixes belong here. Computed typography needs an explicit context/API beyond raw property lookup. |
| `cords` and an HTML adapter | Hold persistent styled text; adapt a selected DOM paragraph into text, annotations, source maps, and paragraph metadata. Adapter placement is proposed, not a claim that the complete conversion exists today. |
| Typography service, potentially a new package | Resolve family/variant selection, typographic values, coverage fallback, and shaping-run itemization through small style/service interfaces. Keep DOM types out of its generic boundary. |
| `fontfind` | Discover/load font resources from configured providers; expose lookup diagnostics. Do not decide document styles or line layout. |
| `opentype` | Parse fonts, expose metrics/properties, and shape character runs into positioned glyph records. |
| OpenType–khipu adapter | Implement khipu font/shaping contracts using OpenType. Keep representation conversion separate from typography policy. |
| `uax` | Unicode boundaries and bidi levels/order, used through appropriate adapters. |
| `hyphenate` and its khipu adapter | Find word break opportunities and expose them to khipu's hyphenation contract. |
| `khipu` | Encode paragraph material using injected services, measure, break lines, and freeze results. Retain provenance needed to associate paint with output. |
| Paragraph layout engine | Orchestrate typography, khipu breaking/freezing, line metrics, and assembly of a boxed vlist. |
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
| Shared style definitions | A small typography vocabulary independent of DOM and concrete font implementations; compatible with cords' style annotations. |
| X: DOM adapter | `w3c-dom`, `cords`, and shared style definitions. |
| M: typography resolver | `cords`, shared style definitions, `khipu/contracts`, and injected selection/coverage services. |
| A: OpenType adapter | `ot`, `otquery`, `otshape`, `khipu/contracts`, and any M service interfaces it implements. |
| Font discovery adapter | `fontfind`, M's selection interfaces, and A for constructing usable font faces. |
| P: paragraph layout engine | `khipu`, M, and shared box/material definitions; font and shaping services supplied through contracts. |
| Application or typographic engine | X, P, A, and discovery configuration; constructs and connects the concrete services. |

The existing dependency `khipu → cords + uax` remains. Khipu stays decoupled from
`otshape` and `hyphenate` through interfaces. The future CSS box engine consumes P
and the shared box interfaces; P does not depend on CSS block layout, X, or the TUI.

### Shared style vocabulary between X and M

`styled.Text` provides styled storage, but does not by itself define what an opaque
style means. X and M must share definitions for font requests, size values,
language, features, and the separation of shaping and paint attributes. Place
these definitions in a small typography package rather than duplicate conventions
in the two adapters. Non-DOM clients construct the same values directly.

X emits font requests, not parsed fonts. M resolves those requests to faces and
typographic parameters. Source/provenance and bidi metadata remain part of the
paragraph envelope where they are not character styles. This shared vocabulary
does not make X responsible for resource discovery or physical shaping boundaries.

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

## First architectural work

Define style projection and boundary semantics before moving whole directories.
Use focused tests to establish that:

1. Color-only changes preserve shaping context while retaining paint information.
2. Actual face/size changes create required boundaries.
3. Equivalent resolved typography does not split unnecessarily.
4. Ligatures and combining marks crossing paint boundaries follow an explicit,
   agreed policy.
5. Soft-hyphen context, coverage fallback, bidi markup, and source mappings survive
   the separation and freezing.

Then extract the typography service and adapters against those contracts, and add
line-to-vlist assembly over a small shared box model. Package names, paint-boundary
policy, computed-style context, and detailed vertical packing rules remain open
choices. The goal is reusable ownership with thin tytui wiring, not relocation of
existing coupling into a differently named package.
