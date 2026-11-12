# Paragraph engine: extraction review and boxed-vlist plan

Proposed 2026-11-12, following [type-integr.md](type-integr.md) and the completed
[typography extraction plan](TYPOGRAPHY_EXTRACTION_PLAN.md). This document records
the review findings and implementation sequence. Steps 1–3 and step 4's
line-height policies are implemented:
`typography/paragraph.Run` consumes prepared styled text and produces a boxed vlist;
tytui retains its placements for rendering. `Assemble` remains available for frozen
input. CSS-inspired baseline line boxes are an explicit opt-in. The CSS adapter
and box-engine bridge remain future work. Existing replay formats are unchanged;
new line-height cases use an explicit version-3 extension.

The first deliverable is **frozen lines → boxed vlist**. Once that is independently
tested, compose it with paragraph preparation and the existing encode/break/freeze
core to provide **styled paragraph → boxed vlist**. Preserve the rendering bypass
and existing replay cases throughout.

## Review of the typography extraction

Steps 1–7 satisfy their extraction scope. Shared styles, immutable resources,
typography resolution, discovery adaptation and OpenType services have owning
packages in the sibling typography module. Tytui consumes them directly; its
fontbackend wrappers and htmlinput style aliases are removed. Generic dependency
audits, adapter regressions, both-breaker integration tests, rendering-bypass tests
and static replay fixtures cover the resulting boundaries.

The original review reran `go test ./... -count=1` successfully in both modules.
Implementation verification for steps 1–4 also includes full race, build and vet
checks, recorded below. Passing extraction tests alone does not establish a
complete paragraph-layout engine or independent vertical-layout correctness.

| Remaining gap | Evidence and consequence | Priority/owner |
| --- | --- | --- |
| Vertical geometry was calculated during rendering | Resolved in step 2: Run retains assembled placements; [pipeline/proof.go](../pipeline/proof.go) consumes them without querying line metrics. The compatibility policy preserves the original minimum gap and trailing spacing. | Completed; step 4 adds explicit CSS-inspired spacing. |
| Public engine boundary was absent | Resolved in step 3: typography/paragraph.Run consumes prepared styled.Text and injected services and returns a boxed vlist plus frozen content. [pipeline.Input](../pipeline/pipeline.go) remains the application envelope; the local engine is removed. | Completed; CSS-inspired line-height is also implemented. |
| Independent physical-placement coverage is bounded | Step 2 adds manually specified synthetic absolute coordinates for mixed LTR/RTL, isolates/digits, multiline and discretionary cases. Existing HarfBuzz fixtures cover real glyphs; real mixed-size/fallback cases exercise retained placements. | Broader independently reviewed real-font paragraph geometry remains useful. |
| Replay captures proof geometry | [replay/evidence.go](../replay/evidence.go) records the full GlyphProof. Changing line spacing or rounding can change old cases even if shaping is identical. | Before renderer migration: preserve the existing placement policy and wire evidence. |
| General typography/source semantics remain bounded | Feature-bearing spans retain their original domains. [PaintSources](../../typography/render/paint_sources.go) supports explicit soft-hyphen transformations, not arbitrary generated-text mappings. | Later: extend in owning packages when a concrete consumer requires it. |

These are follow-on integration tasks, not reasons to reopen the completed
extraction. Latin/Hebrew scope, computed-style context, general hyphenation,
underline/interaction behavior and the deferred annotation API remain separate.
The independent mixed-direction fixtures recommended in [TODO.md](TODO.md) should
be included in this work rather than replaced by self-generated proof snapshots.

## Ownership and scope

Use the existing typography module initially. `typography/paragraph` now owns
bidi preparation, encode/break/freeze orchestration, line measurement and assembly.
Keep the small geometry/material vocabulary separate from paragraph-specific
frozen-content references, using a small package if sharing requires it. Exact Go
names are implementation decisions; this plan does not require a new module or
a universal layout framework.

| Component | Responsibility |
| --- | --- |
| Shared box/material values | Dimensions, reference baseline, placements and vertical spacing; no DOM, colors, font discovery or glyph representation. |
| Paragraph package P | Measure frozen lines through font contracts, apply explicit spacing policy, assemble a boxed vlist, retain line/source references and orchestrate paragraph layout. |
| Khipu | Encode, break and freeze horizontal material; retain source and glyph provenance. |
| Typography M and OpenType adapter A | Continue their existing resolution, itemization, metrics and shaping responsibilities through injected contracts. |
| DOM/input adapter X | Normalize explicit paragraph content, compute supported style values, supply direction/bidi markup and source maps. Keep rendering values on the separate consumer path. |
| Renderer | Use retained placements and glyphs; apply separately retained colors and visual ordering within existing directional fragments. |
| Tytui | Compose services, inspect results, export proofs and replay cases. |
| Future CSS adapter | Translate parent constraints and paragraph boxes; own CSS integration, including padding and margin-collapse context outside P. |

The first implementation is horizontal writing with natural-height vertical
assembly. Vertical kern is fixed spacing; glue contributes its natural size until
a later packing policy implements stretch/shrink. Do not silently accept a target
height or flexible-packing request as if it had been honored. Page/column breaking,
vertical writing, inline images, CSS block formatting and margin collapse are
outside this first engine slice.

## CSS-inspired typographic parameters

P is governed by explicit typed parameters, analogous in role to khipu's Params.
Step 4 implements the baseline-aligned line-height subset below, with CSS semantics
guiding the model. ProofCompatibility remains the default migration policy; it
does not define the new opt-in CSSLineHeight policy.

### Compact CSS line-building primer

- Available width and wrapping rules determine which inline content fits a line.
- `line-height` supplies each text run's vertical contribution. Extra leading is
  divided above and below its font ascent/descent; leading may be negative.
- The paragraph's font and line-height provide a zero-width **strut**, establishing
  minimum above/below-baseline extents for each line.
- `vertical-align` positions inline contributions. Their combined extents,
  including the strut, determine the line box; successive boxes are stacked.
- Thus `line-height` is the closest counterpart to `\baselineskip`, but baseline
  distance is an outcome and may vary with mixed content.

See [CSS line-height and leading](https://www.w3.org/TR/CSS2/visudet.html#leading).
For inheritance, a unitless line-height factor remains a factor applied to the
receiving element's font size; a percentage computes to a length. Preserve that
distinction while resolving document values.

### Parameter ownership

| Category | Values and role |
| --- | --- |
| Paragraph defaults | Line-height (`normal`, a factor, or a resolved length) and default font/size metrics for the strut. `normal` now uses ascent + descent + max(0, font gap), independently of the compatibility 0.2em gap. |
| Per-run layout values | Effective line-height overrides and, later, vertical alignment. Resolve these with each run's size/metrics and retain their logical correspondence through itemization and freezing. |
| Assembly constraints | Available inline width; natural block extent initially. Future height budgets and packing/overflow policies are explicit constraints, not line-height settings. Unsupported packing requests must fail. |

X owns CSS parsing, cascade, inheritance and computed-value handling. M owns font
selection and shaping projection. P receives typed defaults, run layout values
and constraints, resolving metric-dependent layout with the injected font
contracts. Non-DOM callers supply the same values directly. Step 4 uses a narrow
LineLayout value with logical LineHeightSpan ranges, separately from styled.Text;
no generic annotation framework is introduced.

A line-height or alignment boundary must remain available to line construction
without automatically splitting a shaper input run. These values can affect
geometry without changing font selection. Color and other rendering-only values
continue to bypass the typesetter and assembler entirely.

CSS-inspired parameters determine line boxes; the vlist places those boxes.
Kern/glue may express internal material without becoming caller-facing TeX
registers. Padding and margins belong to the surrounding box engine; margin
collapse requires block context. See the
[CSS box model](https://www.w3.org/TR/CSS2/box.html#collapsing-margins).

### Compatibility rule

Steps 2–3 keep ProofCompatibility geometry, precision and trailing spacing. Both
existing manifest versions retain that vertical policy independently of their
different paint policies. An absent new parameter must never silently opt an old
case into CSS line-height behavior. Step 4 introduces CSSLineHeight explicitly
and persists it only in version 3. Old fixtures remain unchanged, as does the
rendering bypass.

## Step 1 — Assemble frozen lines into a boxed vlist

Status: implemented on 2026-11-12. The headless API is:

```go
vlist, err := paragraph.Assemble(ctx, frozen, paragraph.Options{
    Policy:        paragraph.ProofCompatibility,
    DefaultEmSize:  12 * dimen.BP,
})
```

`Assemble` returns `*paragraph.VList`. `Box()` returns logical width, total
`Extent`, the first-line `Baseline` and `HasBaseline`. `Height()` and `Depth()`
on that box describe distances above/below the reference baseline; `Extent`
remains authoritative for floating-point arithmetic. `Lines()` and `Materials()`
return detached slices. `Frozen()` retains the original immutable khipu result.
Each line's `Horizontal` record preserves skips, width/target/residual, fit and
source ranges; its `Index` identifies the corresponding frozen line. Line `X`
is zero, with content starting at `Horizontal.LeftSkip.Points()`.

The compatibility policy retains float64 fractional big-point coordinates and
the old proof's calculation order without integer rounding. It computes maximum
scaled ascent, negated signed descent and the larger of each run's line gap or
0.2em. Each line exposes ascent/descent separately from its gap; `Height` includes
both. Material contains a line box followed by a fixed `Kern`, including after
the final line. No flexible glue or target-height request is exposed yet.

When measured ascent plus descent is zero, the supplied DefaultEmSize provides
0.8em ascent, 0.2em descent and 0.2em gap, matching the existing proof fallback.
A zero-line frozen result produces an empty box without a baseline; nil frozen
input is rejected. Options require a known policy and positive finite default
size. Invalid services, zero units-per-em, non-finite conversions and dimension
overflow return errors, never partial output. Coordinates must remain below
`dimen.Fil` expressed in big points. Signed font extents retain the legacy
nonnegative maxima; negative font line gaps still use the 0.2em minimum.

Box width encloses both target and occupied widths, including overfull lines.
Ink bounds are deliberately absent: assembly never requests glyph outlines.
Geometry/material values are separate from paragraph-specific frozen references
inside the package; no additional shared module was introduced.

Owning tests in `typography/paragraph/assemble_test.go` cover exact synthetic
baselines, mixed sizes, blank and empty output, trailing space, side skips,
overfull widths, invalid metrics/scales, cancellation, accumulated overflow,
detached accessors and concurrent assembly. The generic dependency audit now
includes paragraph. [pipeline/paragraph_box_test.go](../pipeline/paragraph_box_test.go)
originally compared mixed-size pointed Hebrew/Latin with fallback against the old
proof path. After step 2 it checks the retained-placement/rendering boundary under
both breakers, narrow/wide widths and both replay paint policies.
It checks exact geometry, retained glyph/source data and unchanged paint/proofs.

At the step-1 checkpoint, application rendering and replay were unchanged. Step 2
now retains placements in the application result and replaces the proof calculation.

Validation: `go test ./... -count=1`, `go test -race ./... -count=1`,
`go build ./...` and `go vet ./...` pass in typography and tytui, including the
new assembly/dependency tests, rendering bypass and existing static replay cases.
No fixture regeneration, font-resource change or replay version change was needed.

The original scope and acceptance criteria follow:

Implement a headless assembler in typography. Its input is frozen khipu output,
an explicit line-metric/spacing policy and default metrics for lines lacking text
metrics. Its output contains box dimensions, line placements and vertical material,
with stable references to the retained frozen lines. It requires no renderer,
DOM, provider, resource loading or concrete OpenType face.

Introduce only the values needed for this operation:

- Line dimensions: width, ascent/descent relative to its baseline, and separately
  identified leading/spacing. Preserve available width, actual occupied width,
  side skips and overflow information from frozen output.
- Placements: child origin and baseline in paragraph-local coordinates, with a
  documented downward-positive Y convention. Preserve horizontal physical order.
- Vertical material: line boxes and explicit fixed/natural spacing. Retain enough
  distinction to add genuine glue packing later without pretending it exists now.
- Enclosing box: width, total vertical extent and an explicit reference baseline.
  Use the first line's baseline initially; an empty list has zero extent and no
  baseline. State how baseline-relative height/depth relate to the total extent.

First provide a named compatibility policy reproducing the current proof's metric
maxima, minimum gap, fallback metrics and trailing spacing. The default size must
be supplied as a value, not obtained from mutable TUI input inside the assembler.
Do not call this policy CSS `line-height: normal` or TeX baseline glue.

Before finalizing numeric fields, document units, fractional accumulation,
rounding and overflow checks. Reuse existing unit conventions; do not introduce
an unrelated font scaling model. Preserve the legacy proof's calculation order
and precision where replay requires it. Rounding to integer DU is not a harmless
refactor if it changes recorded proof coordinates.

Define and test empty output, a line without retained text metrics, mixed sizes,
fallback fonts, signed descent, overfull width and invalid/non-finite metrics.
Distinguish logical layout dimensions from glyph ink bounds: ink may extend
beyond the box, and measuring the box must not require extracting outlines.

Acceptance:

- Synthetic metric fixtures establish exact baselines, total extent, spacing and
  first/last-line behavior independently of the assembler's output.
- Both breakers feed frozen output into the same assembler; assembly never
  reshapes, rebreaks, changes source coordinates or mutates the frozen result.
- Real mixed-size Latin/Hebrew fixtures exercise contract metrics and fallback.
- The assembler can be called without importing tytui, DOM, discovery or A.
- Existing application rendering/replay remains unchanged at this stage.

## Step 2 — Retain placements and make rendering consume them

Status: implemented. `pipeline.Run` assembles every completed synthetic/OpenType
result with ProofCompatibility immediately after freezing. It uses the configured
backend's default em size for fallback metrics. Assembly errors return a nil result
with `StageAssemble`; successful diagnostic records retain the v1/v2 transcript.

`Result.VList()` returns a detached handle: replacing it or modifying its returned
Box/Lines/Materials cannot alter the retained placement. Its Frozen content is the
same immutable result exposed by Result.Frozen. BuildGlyphProof requires matching
placements and frozen content; it does not silently reassemble missing data.
The renderer reads retained baselines/line extents, keeps glyph ink bounds separate,
and no longer calls font Metrics. VisualOrder still runs per directional fragment.

Paint spans and paint policy are retained separately from Input and from the
vlist. Glyph proofs and WriteSVG dispatch no longer depend on inspection Input's
font selection or paint policy. Clearing that input cannot change already retained
geometry/colors. The structural SVG exporter remains a debug advance-box view
with its own diagnostic spacing, not a depiction of the vlist's vertical geometry.

Replay still serializes the same v1/v2 records and proof evidence. Load rebuilds
placements through Run; no vlist, service object or vertical-policy wire field is
added. Legacy paint and glyph-boundary paint retain their original semantics.

Focused tests:

- `pipeline/retained_placements_test.go`: hand-calculated 10bp synthetic cells
  establish absolute glyph positions, line origins, baselines and physical word
  order for LTR/RTL, isolates/digits, two lines and selected/unbroken soft hyphens
  under both breakers. Original byte correspondence is checked, including the
  selected marker. Real-font proofs survive source-file removal and mutations to
  original/inspection input, placement handles/slices and paint copies. Height
  overflow fails at assembly without publishing a partial result.
- `pipeline/retained_metrics_test.go`: font metrics become unavailable after
  assembly; rendering still succeeds from retained geometry. Missing or mismatched
  placements fail explicitly.
- `pipeline/paragraph_box_test.go`: mixed-size pointed Hebrew/Latin, real font
  fallback, both breakers and both paint policies check rendering from placements.
  Existing independent HarfBuzz glyph/mark tests remain active.
- `replay/retained_placements_test.go`: unchanged pre-step-5/pre-step-6 fixtures
  load, resave and reload with identical evidence, bindings, font records and wire
  versions. Rendering-bypass replay tests also compare rebuilt placements for
  both versions and breakers. No fixtures were regenerated.

Validation: full `go test ./... -count=1`, `go test -race ./... -count=1`,
`go build ./...` and `go vet ./...` pass in tytui and typography after this
migration. Typography's assembler API and implementation did not need changes.

The original scope and acceptance criteria follow:

Wire assembly into tytui after freezing. Retain the assembled result alongside
frozen glyphs and expose detached placement values. Move vertical metric and
baseline calculation out of BuildGlyphProof; there must be one owning calculation.
The glyph renderer continues to determine ink bounds and SVG framing, while the
paragraph result determines line placement. Structural proofs may retain their
diagnostic presentation, clearly distinguished from actual paragraph geometry.

Keep typography-only styled.Text and the separate renderer paint snapshot intact.
Do not attach color to box styles or pass paint into the assembler. Retain logical
source correspondence and directional fragment boundaries; placing lines must
not alter VisualOrder usage, cluster anchors or discretionary replacements.

Preserve replay versions 1 and 2 without adding fields to their wire records for
this behavior-preserving move. Old cases rebuild placements under the compatibility
policy and produce the same proof evidence. Do not serialize service objects or
the new in-memory result automatically. Preserve IDs, glyph colors, version-1
style-boundary behavior and version-2 glyph-boundary behavior independently of
vertical assembly.

Acceptance:

- Static pre-step-5 and pre-step-6 cases load and resave under their existing
  versions without regenerated expectations or changed proof geometry.
- Mutating original input, returned placement copies or paint copies cannot
  change an already retained proof. Rendering works after source fonts disappear.
- Add manually specified LTR/RTL fixtures with absolute line origins, baselines
  and physical word order, then isolates, fallback and selected/unbroken soft
  hyphens under both breakers. Keep independent HarfBuzz run expectations.
- Check geometry and colors directly as well as SVG validity; screenshots alone
  are insufficient evidence of correct placement.

## Step 3 — Expose styled paragraph → boxed vlist

Status: implemented on 2026-11-12. The public API is in
`typography/paragraph/layout.go`. It composes bidi preparation, encoding, breaking,
freezing and assembly; tytui's duplicate `pipeline/internal/layout` package is
removed. `pipeline.Run` retains input normalization, M preparation, service
composition, source maps and separate renderer paint snapshots.

The prepared-text boundary is explicit:

```go
// M prepares normalized paragraph bytes with the matching resolver and policy.
text, bindings, err := resolver.ItemizePolicy(ctx, content, spans, paintPolicy)
if err != nil {
    return err
}
// Retain bindings and rendering paint in the application, outside P.
_ = bindings
out, err := paragraph.Run(ctx, paragraph.Input{
    Text: text, Direction: direction, Markup: markup,
}, paragraph.Params{
    Constraints: paragraph.Constraints{Width: width},
    Breaker:     paragraph.FirstFit, // or paragraph.KnuthPlass
    Assembly: paragraph.Options{
        Policy: paragraph.ProofCompatibility, DefaultEmSize: resolver.Defaults().EmSize,
    },
}, khipu.Config{
    Streaming: &khipu.StreamingConfig{
        NewShaper: newShaper, Resolver: resolver, Defaults: resolver.Defaults(),
    },
    Hyphenator: hyphenator,
})
```

`Input` is one prepared `styled.Text`, base `Direction` and optional
`bidi.OutOfLineBidiMarkup` callback. No DOM types are required: a non-DOM caller
can supply zero-byte isolate events in paragraph byte coordinates directly. Text
is not normalized, projected or itemized again. Its exact styles, including M's
private relative-feature domains and legacy boundaries, reach the resolver intact.
Text/style values and markup must remain stable during Run. X/application owns
normalization, paragraph selection and any mapping back to DOM or original text.
No separate prepared-Paragraph overload is needed for this first public boundary.

`Params` separates available inline `Constraints.Width`, the selected `Breaker`
and `Assembly` defaults. Both breaker and assembly policy/default size are explicit.
`khipu.Config` supplies existing resolver/defaults, shaper-factory and hyphenation
contracts; a zero config uses khipu's synthetic backend. Non-nil Streaming supplies
all its services, with a fresh exclusive shaper per factory call. Set
Assembly.DefaultEmSize to the backend's default for metricless lines. Config.Params
controls encoder policy; line geometry uses the selected breaker's rectangular
default profile. This step introduces no CSS line-height, vertical-align or target
height behavior. Typed paragraph defaults and run-layout values remain step 4.

`Result` exposes `Frozen`, `VList`, `BreakPositions`, `BidiRuns`, `EncodedKnots`
and `BreakCount`. Diagnostics are neutral data, not application message strings.
Frozen content is shared immutably with VList; diagnostic slices and geometry
accessors are detached from engine state. A fresh owner performs each invocation;
no elastic owner escapes. A later or concurrent breaker cannot mutate prior
results. Font/service implementations must honor their retained-lifetime contracts.
P never acquires fonts, discovers providers, extracts outlines or handles paint.

`*paragraph.Error` identifies input, paragraph, encode, break, freeze or assemble
stage and unwraps its cause. Failures return no partial result. Cancellation is
checked between stages and inside encoding/freezing/assembly; synchronous bidi
preparation and breaking cannot be interrupted in the middle of their calls.
Pipeline.Run maps stages to the existing application errors and preserves the
successful diagnostic transcript used by replay. Both replay versions still
rebuild with ProofCompatibility; colors, IDs, wire records and static fixtures
are unchanged. No engine/service objects are serialized.

Focused tests:

- Typography's `paragraph/layout_test.go` exercises non-DOM zero-byte isolates,
  opaque prepared styles, injected hyphenation and both breakers, empty input,
  fresh concurrent owners, retained results, validation, cancellation and backend
  error causes. No DOM, discovery or OpenType dependency enters these tests.
- Tytui's `pipeline/paragraph_engine_test.go` independently constructs shared
  spans and isolate events, prepares them with M, and compares public engine
  output against DOM workbench input. Mixed sizes, RTL/Hebrew with LTR Latin,
  fallback and soft hyphens are covered at narrow/wide widths under both breakers
  and paint policies. Boxes, source bytes, glyph/cluster data and font bindings
  match. `pipeline/paint_features_test.go` retains its fixed relative-feature
  oracle through the public API.
- Dependency audits cover the public paragraph engine transitively and prohibit
  reintroducing a local engine or backend facade. Existing independent placement,
  rendering-bypass, metrics-unavailable and static replay regressions remain active.

Validation: full `go test ./... -count=1`, `go test -race ./... -count=1`,
`go build ./...` and `go vet ./...` pass in typography and tytui. No fixture,
resource identity or replay format changes were needed.

The original scope and acceptance criteria follow:

Promote the generic encode/break/freeze orchestration into the paragraph package
and compose it with the assembler. Keep tytui's public application envelope as
application wiring rather than importing its types into P.

Define a small neutral input around typography-styled `styled.Text`, base direction,
out-of-line bidi markup, width and paragraph policy. A prepared `styled.Paragraph`
may be a lower-level entry point. Non-DOM callers must be able to express isolates
without manufacturing an htmlinput.Extraction. DOM locator maps remain on the
input/application side; P retains paragraph-relative text-to-output correspondence.
Keep paragraph defaults, per-run layout values and assembly constraints distinct
as described above; do not make CSS strings or a combined document-style object
the engine's parameter contract.

Make M's preparation boundary explicit: its current Itemize API accepts text plus
shared document spans and returns typesetter text. Reuse that operation before P
as the initial composition path. P must accept the resulting text and matching
resolver without reconstructing document styles, losing private feature-domain
identities, or re-itemizing already prepared runs. A convenience preparation API
can follow only if needed; it must not introduce a second CSS-property filter.

Supply resolver/defaults, shaper factory and hyphenation through existing contracts.
Font paths, discovery settings and replay storage do not enter P. Preserve fresh
per-run state, cancellation/error stages and freezing before another breaker can
alter material. Return the boxed vlist, retained frozen content and diagnostics.

Acceptance:

- DOM and non-DOM callers reach the same engine with matching text, typography
  and bidi input; both produce equivalent boxes and source correspondence.
- Synthetic and OpenType services use the same public path; P imports no concrete
  adapter, discovery package, DOM or TUI.
- Tytui and replay use the public engine without duplicate assembly algorithms.
- Extend dependency audits and run both modules' full ordinary/race suites,
  build and vet; keep all existing replay and rendering-bypass regressions.

## Step 4 — Add new spacing policies and a CSS bridge when needed

Status: the requested CSS-inspired line-height engine is implemented. CSS string
extraction and the `css-box-layout` bridge remain separate follow-up work; this
step adds no TUI controls or implicit change to DOM/TUI defaults.

### Public API and opt-in

`paragraph.Run` accepts `Params.LineLayout` with
`Assembly: paragraph.Options{Policy: paragraph.CSSLineHeight}`. The existing
`Streaming.Defaults.Font` and `EmSize` supply the paragraph strut. CSS mode requires
explicit streaming services/defaults; omit the compatibility-only DefaultEmSize.
The existing frozen-only `Assemble` entry remains ProofCompatibility: it has no
prepared style/resolver context for spaces without retained glyph runs.

```go
params.Assembly = paragraph.Options{Policy: paragraph.CSSLineHeight}
params.LineLayout = &paragraph.LineLayout{
    Default: paragraph.LineHeight{Kind: paragraph.FactorHeight, Factor: 1.5},
    Runs: []paragraph.LineHeightSpan{
        {Start: 4, End: 8, Height: paragraph.LineHeight{
            Kind: paragraph.LengthHeight, Length: 24 * dimen.BP,
        }},
    },
}
// paragraph.Run(ctx, preparedInput, params, streamingConfig)
```

Tytui's headless `pipeline.Input.LineLayout` accepts the same value and selects
CSSLineHeight. Nil continues to select ProofCompatibility. Pipeline font settings
supply the strut font/size, or synthetic defaults when fonts are absent. Ranges
address normalized paragraph UTF-8 bytes, exactly as frozen source coordinates do.
`Result.LineLayout()` returns a detached snapshot used by replay; mutating the
request or inspection input cannot change retained placements or saved policy.

### Values and geometry

- `NormalHeight`: scaled ascent + positive descent + max(0, scaled font line gap).
  This is our deterministic font-metric policy for CSS-inspired `normal`; it has
  no 0.2em floor and does not promise browser-specific metric-table choices.
- `FactorHeight`: a nonnegative unitless factor multiplied by each receiving run's
  em size. Inheritance preserves the factor.
- `LengthHeight`: a nonnegative resolved DU length. Inheritance preserves the
  length; an adapter must compute percentages and relative lengths in the defining
  element's context before passing them to P. The engine does not parse CSS.

For each contribution, half-leading is `(lineHeight - ascent - descent) / 2`.
The effective above/below-baseline extents include that half-leading; the line
uses the maximum of each side across the strut and contributing runs. Boxes stack
at natural height. Gap is zero, material contains only LineBox entries, and there
is no final kern. A smaller run cannot shrink the strut. A mixed-size line can be
taller than any one inherited fixed length because its baseline extents differ.

Negative leading is allowed; zero line-height is valid. A baseline can lie outside
its logical box, so Line.Ascent/Descent and Box.Height()/Depth() are signed extents.
Glyph ink may overlap adjacent line boxes; the renderer still frames actual ink.
Geometry remains fractional big points, with finite-range and accumulated-overflow
checks. Unknown kinds, negative/nonfinite values, extra fields for the wrong kind,
overlapping/out-of-range spans and non-UTF-8 boundaries are errors. Missing CSS
services/options and CSS values under ProofCompatibility are rejected explicitly.
The engine preserves khipu's blank line for empty input and gives it the strut;
a genuinely zero-line frozen result remains an empty box in Assemble.

### Source and shaping boundary

LineHeightSpan ranges are ordered, nonoverlapping half-open byte ranges; gaps use
Default. They never enter styled.Text or change M's style projection/itemization.
The assembler intersects them with frozen knot provenance, including selected
soft-hyphen replacements, and uses the actual retained font/scale for each glyph
run. Every intersecting range contributes even inside one ligature/mark sequence;
the glyph is never split or recolored. Surviving spaces use the matching prepared
source style's resolved font metrics; trimmed spaces and silent replacements do
not enlarge a line. The source-style scan reads range metadata without flattening
or reconstructing the cord. Font metrics are consumed before publishing output,
and rendering continues to consume placements and separate paint snapshots only.

This is baseline-aligned text layout. There is no vertical-align, inline image,
empty-element box, target-height packing, padding, margin collapse or new generic
annotation API. Those require additional content/context contracts rather than
fabricated geometry. A future DOM adapter should compute and flatten inherited
line-height values into these records; the current DOM extractor does not yet
translate the CSS `line-height` declaration.

### Replay and verification

Versions 1/2 continue to omit LineLayout and placement evidence and retain their
original compatibility geometry and paint semantics. Version 3 requires explicit
LineLayout, explicit glyph-boundary or legacy paint policy, and numeric layout
evidence, including for synthetic results. It pins the same font resources and
rebuilds with the same pipeline offline. Save uses retained layout values, and
load checks the recomputed evidence. No font service or VList object is serialized.
Legacy resaves keep their original version; no fixture regeneration was needed.

Owning tests in `paragraph/lineheight_test.go` and `lineheight_metrics_test.go`
cover exact synthetic baselines/heights, factor versus computed-length inheritance,
strut minimums, normal font gaps, zero/negative leading, fractional zero-height
boxes, mixed sizes, surviving/trimmed spaces, discretionary provenance, untouched
horizontal output, detached inputs, invalid metrics/ranges, cancellation and
accumulated overflow under both breakers. Tytui's `pipeline/lineheight_test.go`
covers mixed RTL/LTR, pointed Hebrew, real fallback, sizes, paint policies and
an override inside a colored fi ligature. The retained-metrics test also exercises
CSS mode with metrics disabled before rendering. `replay/lineheight_test.go`
covers v3 round trips/resaves with both backends/breakers/paint policies and rejects
inconsistent versions, missing policy/evidence and tampered layout values.

Validation: full ordinary and race suites, build and vet pass in typography and
tytui. Existing static v1/v2 fixtures and independent glyph/placement expectations
remain unchanged.

The original scope and future bridge requirements follow:

Once retained placement is stable, implement the CSS-inspired parameter model:
paragraph strut/defaults, effective run line-heights, half-leading and mixed-run
extents, with baseline alignment initially. Add other vertical alignments only
when supported explicitly. Define first/last-line extents from the line boxes;
the compatibility policy's trailing kern is not a CSS default. Test unitless
versus length inheritance, mixed sizes/fallback, negative leading and strut
behavior independently of the legacy proof convention. Keep constrained-height
packing separate. Do not silently change replay's interpretation of old defaults.
If new policy selection must be persisted, define a versioned wire extension with
an explicit migration path; existing versions retain their old meaning. No new
manifest version is needed merely to complete steps 1–3.

The current css-box-layout InlineLayouter expects line boxes with frames and
baselines. A later adapter should translate constraints, units and source/payload
references to that interface. Recheck its evolving API before implementing the
bridge. Keep css-box-layout out of P's dependencies; its float32 geometry is not
a reason to reduce paragraph-engine precision. Atomic inline items and intrinsic
measurement require separate scope rather than fabricated results.

Padding belongs to the surrounding box; CSS margin collapse requires neighboring
block/ancestor context and remains with CSS layout. Generic vertical packing,
height budgets and fragmentation can follow a demonstrated need. The general
annotation API stays deferred.

## Rollout and verification

Steps 1–3 and step 4's line-height policies are complete, with owning tests in
typography and focused tytui DOM, rendering and replay integration. Next is a
DOM line-height adapter when needed, followed by an explicit css-box-layout bridge.
Preserve the old compatibility checkpoint and the explicit new-policy opt-in.
Each stage leaves the workbench usable and retains the existing
source/paint/replay guarantees. Do not regenerate independent fixtures from the
implementation or broaden scripts/discovery as part of this work.

Run focused owning tests and full ordinary tests, build and vet in both modules
for each implementation stage. Run full race suites when retaining new shared
results or moving orchestration, and at completion. Keep source-inspection findings,
proposed contracts and implemented behavior distinct in documentation.
