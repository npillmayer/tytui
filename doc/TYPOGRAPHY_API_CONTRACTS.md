# Typography extraction: proposed API contracts

Step 1 of [the extraction plan](TYPOGRAPHY_EXTRACTION_PLAN.md), 2026-11-11.
Steps 1–7 are implemented. The original sketches and milestone sections below
record the migration history; the final step-7 contract supersedes temporary
aliases and forwarding APIs.
Step 6 implements the agreed glyph-boundary policy, with legacy replay preserved.
The [rendering bypass](type-integr.md#style-projections-and-the-rendering-bypass)
is implemented for color: M projects typography styles for the paragraph
typesetter; colors reach the renderer separately through application wiring and
source correspondence. See the implemented bypass and step-7 contracts below.
The [general annotation API proposal](ANNOTATION_API_CONTRACT.md) is deferred, including
its package, consumer policies, and proposed replay version. It does not block
extraction or require a change to existing color/replay behavior.
The implementation uses one sibling checkout
`../typography`, module `github.com/npillmayer/typography`, with the packages below.
Steps 2–5 created the root resolver, style/resource/discovery/render packages and adapters;
the module has not been published.

## Dependency contract

- `typography/style`: shared immutable style values; no DOM, fontfind, or OpenType
  types. X and M both use this vocabulary. Cords' style interface and existing
  language/unit types may be used.
- `typography/fontresource`: immutable bytes, neutral request/provenance values,
  content hashes, and resource identities. No parsing or discovery dependency.
- `typography`: M; depends on style, fontresource, cords, and khipu/contracts.
  Selection/loading/coverage services are injected. No import of htmlinput,
  w3c-dom, or the concrete backend adapter.
- `typography/adapter/opentype`: A; depends on OpenType and khipu contracts and
  implements M's supplemental services. Concrete faces stay inside this boundary.
- `typography/adapter/fontfind`: converts neutral requests and adapts providers.
  It may depend on M and fontfind, but receives loading/validation services rather
  than requiring M to import A.

Application composition installs A and discovery policy. X initially remains in
htmlinput, P in pipeline. Neither shared styles nor M imports P. Khipu continues
its existing cords/uax dependencies without a reverse dependency on typography.
A single module may list all backend requirements; generic packages need not
import them. No new universal font representation or font-table API is proposed.

## Values shared by X and M

The following is the original step-1 sketch, retained to explain the extraction
history. Its combined DocumentStyle/Paint is not the chosen target for the
typesetter input: rendering properties now bypass the typesetter. The implemented
milestones below describe intermediate APIs, including temporary compatibility
wrappers. Step 7 removes those Go APIs while preserving explicit replay DTOs.

```go
// Schematic typography/style values.
type FontRequest struct {
    Families []string       // Ordered names, not an unsplit CSS declaration.
    Weight   int            // CSS numeric weight, not x/image/font.Weight.
    Slant    string          // Validated supported style vocabulary.
}
type Typography struct {
    Font     FontRequest
    EmSize   dimen.DU       // Positive computed size, in scaled big points.
    Language language.Tag
    Features []contracts.ShapeFeature
}
type Paint struct { Color string }
type DocumentStyle struct {
    Typography Typography
    Paint      Paint
    // Retain supported whitespace/direction semantics through conversion.
}
type Span struct {
    Start, End uint64       // Half-open UTF-8 bytes in processed paragraph text.
    Style      DocumentStyle
}
```

This is not permission to discard current whitespace, direction, or unicode-bidi
fields. Step 2 preserves every supported value through the compatibility layer;
the eventual envelope/property split must account for each explicitly. Size
expressions are resolved with the appropriate CSS parent/root context before
becoming EmSize. Inherited computed sizes are not evaluated a second time.

DocumentStyle must preserve paint in equality. Slices require value comparison
and defensive copies, not pointer equality. Expose constructors/accessors or
copy-on-publication semantics so caller mutation cannot change stored styles.
The exact representation can remain comparable during the first migration.

The paragraph envelope carries styled.Text, base direction, zero-byte bidi
boundaries, paragraph settings, and source associations. DOM node locators remain
X/application metadata; M needs text intervals, not DOM nodes or filenames.
Direction comes from paragraph analysis and is not added to StyleResolver's result.

## Font and resource boundary

Reuse these khipu contracts unchanged:

- FontFace: Metrics and Scale. A loaded face is size-independent.
- FontScale: signed font units to fractional layout DU; no axis inversion and no
  early integer rounding. One face may serve several sizes concurrently.
- FontLoader: reader to contractual face with parser diagnostics/errors.
- StreamingShaper: logical-order glyph stream, scalar anchors, explicit lifecycle,
  cancellation/backpressure and terminal errors.
- StyleResolver/ResolvedRunStyle: complete style configuration, not a delta.
  Defaults and feature slices must not be mutated. Bidi analysis and scaling
  happen outside style resolution.

Supplement those contracts with narrowly scoped services, schematically:

```go
// M-facing sketch: none of these signatures mentions ot.Font or adapter.Face.
type Identity struct {
    ResourceID, SHA256 string
    Family, Subfamily string
    Origin            fontresource.Origin
}
type FaceEntry struct {
    Face     contracts.FontFace
    Identity Identity
    Resource fontresource.Resource
}
type FaceLoader interface {
    Load(context.Context, fontresource.Resource) (FaceEntry, []Diagnostic, error)
}
type Coverage interface {
    GlyphIndex(contracts.FontFace, rune) (uint32, error)
}
```

Identity/resource are immutable values or handles with detached metadata access.
Resource supplies fresh bounded readers over owned bytes, not mutable backing
slices. Preserve the current 64 MiB read limit and explicitly document any upstream
read-before-limit adapter. SHA256 identifies bytes; ResourceID also identifies
provenance. Neutral Origin includes requested family/style/weight, provider name,
resolved resource name, and location; it must not import fontfind.Descriptor.

Coverage queries nominal mapping only: a nonzero cmap result is not proof that
shaping will succeed. M retains grapheme-level candidate selection and configured
ordering; A still rejects missing shaped output. Backend adapters reject unsupported
face implementations with errors. Do not let M/P downcast faces to obtain metrics,
identity, bytes, or coverage. Thread-safe immutable font services may share backend
storage, with internal synchronization for lazy caches.

A may expose a backend-specific rendering adapter for outlines. A generic proof
walker can accept an outline service with a backend-neutral path vocabulary, or
A can own the backend-specific walker. Choose the smaller implementation in step 5;
do not add outline methods to FontFace simply for the workbench. Identity access
for replay must be independent of outline access. Generic P does not import sfnt
or assert an OpenType face.

## M's input and result

M accepts styled.Text with the shared style vocabulary and paragraph metadata,
plus explicit selection, coverage and loading services. Its preparation result
contains:

- Styled text projected for khipu and a contracts.StyleResolver.
- Resolved face entries/resources and binding intervals.
- Source correspondence needed to associate output with logical text. Under the
  chosen bypass, the input/application side retains rendering properties separately;
  M does not need to return them. Current Itemization.Paint is compatibility API.
- Detached structured diagnostics and the policy/request snapshot.

A possible preparation method is:

```go
Prepare(ctx context.Context, paragraph ParagraphInput, policy Policy) (Prepared, error)
```

ParagraphInput/Prepared are semantic sketches, not an additional encoder API.
Prepared may initially preserve today's segmentation. Separating shaping and paint
storage does not itself authorize coalescing spans. M's rule selection remains
first-match, family lists remain ordered, and glyph fallback remains distinct from
opt-in discovery substitution. Read/parse errors never silently select a substitute.

Keep feature coordinate domains explicit: in ResolvedRunStyle, ranges address
original style-span-relative Unicode scalars, including omitted controls. Khipu
clips/rebases them for shaping runs. Emitted clusters use the adapter's configured
scalar base. Coalescing spans later must translate features deliberately; copying
Start/End unchanged into a merged span is incorrect. Source/paint spans remain
UTF-8 byte based. Replacement clusters remain replacement-local.

## Discovery and serialization

Neutral requests use CSS weight/style values; the fontfind adapter performs tested
conversion and rejects unsupported requests. Generic-family mappings, ordered
providers, substitution permission, reasons, and failures remain explicit. Cache
hits preserve diagnostic evidence. No implicit system/network provider is installed.

Keep replay version 1 as a wire contract separate from internal Go types. The
current resource ID hashes a quoted sequence of provider, name, path, request
pattern, numeric fontfind style/weight, and content hash. Moving to neutral request
values must retain a version-1 encoder, or deliberately version the identity and
manifest format. Do not hash a new struct's JSON and call it the same identity.
No credentials or provider closures belong in serialized policy.

## Agreed paint-boundary policy (step 6)

Color changes occur at glyph boundaries only. They do not split shaping runs.
A ligature is never clipped or split into colors: it takes the paint at its first
contributing logical source anchor. A combining-mark glyph can use its own paint
when it retains an independent source anchor; records sharing an anchor inherit
that cluster's paint. Do not guess mark ownership from visual order. No special
paint-conflict diagnostic is needed for these rare cases.

Original paint spans remain unchanged. Separate shaping projections merge only
when font identity, size, language/script, spacing and bidi context permit. Relative
feature domains remain separate when nonempty; document Style equality still
includes every field. Explicit soft-hyphen replacement '-' takes the marker's paint,
while surrounding replacement text keeps its own source paint.

Replay version 2 records glyph-boundary policy. Version-1 manifests retain the old
style-boundary shaping and knot paint through an explicit legacy path. No old case
is silently reinterpreted. See the concrete step-6 contract below.

## Baselines and validation scope

Original step-1 tests (their current owners are recorded under step 7):

- `fontbackend/extraction_contract_test.go`: fixed version-1 identity values,
  contractual face scaling at two sizes, first-match style mapping, feature-storage
  ownership, paint-preserving document equality, and scalar feature ranges with
  a nonzero output cluster base. Feature glyph expectations use the existing
  ShaperTestProgress fixture, not output recaptured from the new code.
- `pipeline/extraction_contract_test.go`: multibyte byte spans versus scalar
  clusters, actual size changes, retained paint, and detached provenance through
  freezing/proofs under both breakers. It does not claim an independent physical
  layout oracle or prescribe paint-only run segmentation.

Retain existing `fontbackend/soft_hyphen_test.go`,
`pipeline/fallback_hyphen_test.go`, `pipeline/visual_order_test.go`, the independent
HarfBuzz cases in `pipeline/multilingual_test.go`, and `replay/replay_test.go`.
They already cover explicit marker styles, selected/unbroken discretionary
replacement context, detached snapshots, and pinned offline proof replay. Avoid
copying those tests merely to rename them as extraction tests.

No production types or algorithms are moved in step 1. Passing these baselines
establishes a migration checkpoint, not correctness of the unresolved paint policy
or completion of the new dependency architecture.

Validation: focused baselines and full `go test ./... -count=1`, `go build ./...`,
and `go vet ./...` pass. The new fontbackend/pipeline baselines also pass under
`-race`. No sibling source or production implementation was changed.


## Implemented step-2 API

`github.com/npillmayer/typography/style` owns the existing flat document `Style`
fields and `Span{Start, End uint64; Style Style}`. `Style.Typography()` returns the
non-paint fields; `Style.Paint()` returns Color. Both are comparable value projections,
not resolved shaping keys. Features remain in resolver defaults for now. Full
Style equality, String output, numeric CSS weights, computed dimen.DU sizes and
JSON field names are unchanged. htmlinput exports type aliases until step 7.

`ValidateSpans(text, spans)` checks UTF-8, complete ordered byte coverage and finite
positive sizes. Nil requests defaults; nonnil empty only covers empty text. It
neither computes CSS values nor decides grapheme boundaries. Resolver.Itemize and
CheckText accept `[]style.Span` directly, retain grapheme validation and default
feature/fallback behavior, and no longer import htmlinput.

`pipeline.Input.Styles` is optional and exclusive with Source. Coordinates address
normalized text, so callers may use paragraphtext.Normalize to prepare spans. Run
retains original text and its TextSpans map, clones styles, and applies them to both
synthetic and OpenType layout. Only normal whitespace and unicode-bidi are accepted;
Input.Direction supplies base direction, not individual Style.Direction fields.
Explicit isolates continue to use Source's markup. Frozen glyph proofs retain paint.
Replay stores this optional field without changing old plain/DOM cases or resource
IDs; old strict readers cannot read cases containing the new field.

Verification: shared-package equality/projection, span validation and fixed JSON
regressions; both breakers with synthetic/OpenType non-DOM inputs; frozen size,
byte/scalar coordinates, detached paint and normalized source mappings; offline
shared-style replay. Existing DOM/source-map, identity, feature, discretionary and
VisualOrder compatibility tests remain unchanged except for the Itemize call shape.

Validation completed for step 2: `go test ./...`, `go test -race ./...`,
`go build ./...` and `go vet ./...` in typography and tytui. Dependency inspection
confirms style has no DOM/discovery/OpenType imports and fontbackend no longer
imports htmlinput. Revision capture labels the new unversioned module explicitly.


## Implemented step-3 API

`typography/fontresource` uses only the standard library. `Read(io.Reader, Origin)`
snapshots at most 64 MiB plus a detection byte; errors preserve reader failures.
Resource.Open returns a fresh reader without exposing its byte slice. Origin,
SHA256 and ID return detached provenance and identities. Request contains Pattern
and independent Slant/Weight values: Normal/Italic/Oblique are 0/1/2;
Thin through Black use -3 through 5, with Regular=0. These are stable resource
request codes, **not CSS numeric weights**. Shared document Style.Weight remains
CSS-valued. Unknown numeric provenance values round-trip without reinterpretation;
request selection/validation remains the resolver's responsibility.

Version-1 resource ID encoding is SHA-256 of the UTF-8 string:
`%q|%q|%q|%q|%d|%d|%s` with provider, name, location, pattern, slant, weight and the
lower-case content SHA-256. The result has a `sha256:` prefix. Go `%q` escaping and
field order are part of the contract. Fixed default and italic/bold regressions
preserve the old encoding; content identity remains independent of provenance.

`typography/adapter/opentype` owns Loader.LoadFont, LoadFile, LoadResource, Face,
Scale and NewStream. Faces retain the original Resource and parser-owned bytes;
Face.Resource does no I/O. Info returns detached metadata/warnings. Metrics and
Scale satisfy khipu contracts; GlyphIndex and Outline serialize lazy backend access
with shaping. Outline's sfnt paths are intentionally backend-specific, in font
units with downward Y. No parsed ot.Font accessor is exposed. The normal parser,
missing-glyph checks, logical glyph order, scalar features/clusters, cancellation,
bounded output and terminal stream lifecycle are unchanged.

`tytui/fontbackend` retains Resource/Origin/Info/Face/Loader/Stream compatibility
facades and Scale as an alias. Origin still has fontfind.Descriptor so existing
callers and version-1 replay DTOs keep their shape. Explicit conversion to/from
neutral request values lives here; FontfindResource still performs no discovery and
still reads provider bytes before applying the size bound. The facade Stream only
unwraps a facade Face; all production and synchronization happen in A. Snapshot
uses retained Resource handles instead of reaching into private font data.

At this extraction milestone, `pipeline/internal/layout.Run` accepted a styled paragraph, width, breaker and
khipu.Config, then encodes, breaks and freezes with matching geometry. It has no
OpenType, fontfind, typography adapter or DOM dependency, and its synthetic test
runs both breakers without an adapter. Pipeline.Run is still the application
composition entry point; it chooses services, preserves stage diagnostics and
source maps, and calls this generic core. This was not yet the boxed-vlist engine; paragraph-engine step 3 later moved
this code to `typography/paragraph.Run` and removed the local package.
Step 4 replaces the resolver maps with contractual entries. Step 5 migrates proof/replay identity and rendering consumers to injected services.

Validation: full `go test ./...`, `go test -race ./...`, `go build ./...` and
`go vet ./...` in both modules. Owning tests cover bounded/read/parser failures,
fixed identities, snapshot isolation, removed-file outlines, concurrent coverage
and outlines, signed scaling, independent glyph expectations, foreign-face rejection,
features, missing output, backpressure/cancellation and stream lifecycle. Tytui
retains compatibility tests and both-breaker file/resource/frozen-proof/replay
integration coverage. Dependency inspection confirms the generic core and resource
package do not acquire backend/discovery/DOM imports.


## Implemented step-4 API

The root `typography` package now owns mapping and coverage policy. It imports
cords, khipu/contracts and dimen, shared style/resource values, Unicode segmentation
and language types. It imports no DOM, fontfind, concrete OpenType adapter, or tytui.

```go
func LoadSettings(context.Context, Settings, Services) (*Resolver, error)

type Services struct {
    Loader   FaceLoader
    Coverage Coverage
}
type FaceLoader interface {
    Load(context.Context, string, *fontresource.Resource) (FaceEntry, error)
}
type Coverage interface {
    GlyphIndex(contracts.FontFace, rune) (uint32, error)
}
type FaceEntry struct {
    Face     contracts.FontFace
    Info     Info
    Resource *fontresource.Resource
}
```

`LoadFunc` and `CoverageFunc` adapt explicit callbacks to these interfaces. There
is no default service constructor or implicit OpenType installation. The loader
receives exactly one path/resource and must return a usable face with a retained
immutable resource. M verifies content/provenance IDs, Origin and Path against that
resource, and rejects substitution of a supplied snapshot. It copies metadata and
policy slices before publication. Loader errors and cancellation publish no resolver.
Coverage returns zero for missing glyphs; query errors terminate instead of silently
falling back. Service implementations own backend synchronization and must remain
valid for the resolver/result lifetime; nominal coverage does not guarantee shaping.

`Settings`/`Rule` retain first-match family/weight/slant rules, default selection,
and ordered path-then-resource fallbacks, without any discovery-specific fields.
`Info`/`Binding` use neutral Origin. Resolver.Defaults and ResolveStyle implement the
existing khipu contract, preserving scalar feature ranges and copying feature slices.
Itemize/CheckText accept complete processed UTF-8 style spans (nil uses defaults).
Grapheme integrity, Latin/Hebrew restrictions, Common/Inherited adoption, coverage
exemptions and soft-hyphen neighbor choice remain unchanged. Face selection never
crosses original style spans for discretionary context.

Snapshot returns neutral Settings pinned to every loaded resource, including unused
rules and fallbacks. It does not reopen files or call the injected loader. Returned
policy/metadata slices are detached; immutable resources and contractual faces remain
shared by design. Resource identities and binding byte ranges preserve the prior
workbench contract.

`ItemizeWithMetadata` additionally returns Itemization with Text, Bindings, Shaping
and Paint projections. ShapingSpan has byte endpoints and a ShapingKey containing
resource identity, size, script/language, features and spacing. Feature origins are
the retained styled-run starts, and feature endpoints are scalar coordinates.
These are **pre-bidi projections**, not a complete final run-equivalence test:
paragraph direction/isolation and feature rebasing remain external. PaintSpan stores
the original byte span and paint value. Full document equality remains authoritative
for actual run segmentation; equal typography across different paints still produces
separate runs. No ligature/combining-mark paint policy is chosen by this API.

Tytui's fontbackend.Resolver is a facade: it converts legacy Settings/Info/Binding,
injects explicit loading/coverage callbacks using its compatibility Face, and
reattaches discovery policy, original requests and diagnostics to neutral snapshots.
Existing public methods and version-1 JSON stay intact. The facade is the application
composition boundary; concrete face assertions occur there, not in M. Step 5 migrates discovery and proof/replay consumers as described below.

Validation: fake services exercise mapping precedence, quoted/case-insensitive
families, sizes/features, metadata ownership, pinned snapshots, fallback order,
script/grapheme boundaries, soft-hyphen context and marker-only spans, missing/cmap
errors, cancellation, absent services and inconsistent/substituted resources. Paint
projection tests explicitly retain old boundaries. Tytui tests verify compatibility
faces, style resolution, detached discovery diagnostics and identical replay binding
JSON; existing both-breaker, frozen glyph proof, discretionary, identity and replay
regressions pass. Full ordinary/race tests, build and vet pass in both modules.


## Implemented step-5 API

`typography/discovery` owns FontRequest (ordered CSS families, weight/slant and
explicit substitution choice), DescriptorSettings, DiscoveryPolicy, diagnostics,
Resolution and the session cache. `New(Searcher, typography.FaceLoader, generics)`
requires explicit services and copies session policy. `Resolve` and `ResolveSettings`
return immutable resources, detached diagnostics and pinned neutral Settings;
Resolution.Clone also detaches original requests and policy. Only successful
read/validation results are cached. Cancellation while queued or during providers
is preserved; cache hits preserve substitution evidence. Neither this package nor
M imports fontfind or a concrete backend.

Searcher accepts mapped and original neutral fontresource.Request values and an
explicit substitution flag. It returns a SearchResult with retained resource,
provider, substitution flag and attempt diagnostics. Read failures are terminal
ResolutionErrors; ordinary lookup misses proceed to the next requested family.
The injected loader validates resources before publication; parser failures never
silently select another candidate. CSS-to-neutral request conversion is independent
of provider enums. The adapter translates the neutral values to fontfind.Descriptor.

`typography/adapter/fontfind.New([]OfflineProvider)` validates/copies named controlled
providers and implements Searcher with a fresh private ResolverPipeline registry.
Implicit packaged fallback is disabled. A provider's nil error is not an exact-match
attestation; a non-exact result or font-plus-error requires explicit substitution
permission. Attempts remain visible, in order. ReadResource translates an already
resolved ScalableFont into a neutral immutable snapshot; its upstream ReadFontData
allocation still precedes the size check, so only trusted offline providers belong
here. The adapter installs no parser and performs no system/network enumeration.

F5's packaged provider now uses this adapter API. The fontbackend.Discovery facade
composes it with neutral policy and the existing OpenType loader, while preserving
legacy settings/diagnostic JSON and stable cached Resource pointer identity. Its
concurrent handle map contains no resolution policy. File mode, preview/apply,
cancellation and stale-result handling stay in the TUI unchanged.

`typography.IdentityProvider.Identify(contracts.FontFace)` supplies neutral Info.
`typography/render.Backend` supplies Outline(face, gid) and VisualOrder(glyphs,
direction). Outline contains an SVG path and detached control points in font units
with downward Y; it exposes no sfnt type. OpenType Services implements both
interfaces from retained bytes, rejects foreign faces, and calls generic
`otshape.VisualOrder` on one complete owned frozen fragment. Compatibility wiring
unwraps old Face facades; the generic proof walker never asserts concrete faces.
Result.Identity and Result.Renderer retain selected services and are excluded from
JSON. The walker still owns geometry/paint and SVG assembly; no reshaping or
line breaking occurs on export.

Replay evidence uses Result.Identity independently of outlines. FontSource exposes
only Open() io.ReadCloser for pinned bytes. Resource validation goes through the
application's InspectResource facade, without exposing parsed faces to replay.
Existing version-1 workbench Origin/Info/Binding and selection DTOs are deliberately
retained. Descriptor aliases have the same JSON fields and numeric request values.
No semantic format change or version bump is required, and provider closures/services
are never serialized. Load uses only the content store and explicitly wired backend;
it never invokes discovery.

Validation includes static pre-step-5 firstfit and Knuth–Plass manifests in
replay/testdata, captured before this migration. They include mixed Latin/pointed
Hebrew, a discretionary marker, both fonts, requests/policy, identity and frozen
proof evidence. Tests supply font bytes separately; do not regenerate those JSON
files to mask differences. New owning tests exercise injected validation without
real fonts, terminal read/parse failures, cache isolation/ownership, actual fontfind
attestation/cancellation/ordering, and retained rendering/identity with repeated
cluster anchors. Tytui tests verify injected proof services and old-format replay;
existing provider-free replay, TUI file/request/preview/stale-result and both-breaker
shaping suites pass. Full ordinary/race suites, build and vet pass in both modules.


## Implemented step-6 API

`typography.PaintPolicy` defines GlyphBoundary (`glyph-boundary-v1`) and
LegacyStyleBoundary (`style-boundary-v1`). Resolver.Itemize defaults to the new
policy; ItemizePolicy selects legacy behavior explicitly. CoalesceStyles resolves
adjacent spans before grouping: same opaque face identity and equivalent resolved
parameters can share a shaping span even if document requests or colors differ.
It does not inspect backend font state. Direction/unicode-bidi remain significant,
and actual isolate boundaries still enter cords' bidi intersection machinery.

Document Style.Equals remains complete equality. Itemized run equality uses
selected resource/script and relevant typography rather than paint or unmatched
font requests. Original paint spans remain in Itemization.Paint and pipeline's
retained Input.Source/Styles. Color-only boundaries inside graphemes are accepted;
unsupported actual typography boundaries inside graphemes still fail. Fallback and
soft-hyphen context operate on the combined shaping spans, so changing marker paint
does not disable a valid discretionary break.

Feature-bearing spans retain their domains conservatively: identical relative
ranges beginning at different source offsets are not interchangeable. The document
Style API currently has no per-style feature field; resolver defaults/features still
use the retained shaping span's scalar origin. CoalesceStyles does not concatenate
or silently reinterpret nonempty feature lists. Khipu's existing range clipping and
bidi rebasing remain responsible for those spans. A both-breaker fixture exercises
repeated [2,4) feature domains with anchors 0,1,2 and 4,5,6.

`render.PaintSources(text, knot, snapshot)` maps frozen scalar anchors to original
processed UTF-8 bytes before visual ordering. Ordinary anchors are paragraph-relative;
repeated anchors map to the same paint. Ligatures take their first source anchor,
and separately anchored marks may differ. The helper maps generated pre/post-break
text for the supported explicit-soft-hyphen producer, including multibyte source,
removed earlier markers and the selected marker emitted as '-'. It checks the
reconstructed text against ReplacementText and rejects unsupported transformations
instead of inventing provenance. No khipu owning-package change was needed for this
supported producer; arbitrary future hyphenation replacements need explicit maps.

`pipeline.Input.PaintPolicy` defaults to GlyphBoundary and is retained in results.
Synthetic and OpenType paths coalesce compatible paint spans; the OpenType proof
walker assigns paint per glyph, not per knot, through original source offsets.
No outline clipping or additional paint-conflict diagnostics are introduced.
Source maps, style spans and glyph/cluster records remain detached snapshots.

Replay Version is now 2. Save records the new policy explicitly. Load accepts version
1 only with its original absent policy field and selects LegacyStyleBoundary;
version 2 requires GlyphBoundary. A loaded legacy result saves as version 1 with the
policy field omitted. Unknown/ambiguous combinations fail. Resource IDs and font
DTOs are unchanged. Static pre-step6-paint.json proves that old red-f/blue-i output
retains two glyphs while a new run shapes one red fi ligature. New cases round-trip
with their effective glyph paint, and existing pre-step-5 fixtures are unchanged.

Verification: owning coalescing/source-map/legacy/feature-domain tests; integration
proofs for Latin ligatures and marks, pointed Hebrew with shared anchors, RTL
fallback and selected/unselected soft hyphens under both breakers; original paint
span ownership; static legacy and new-policy replay tests. Full ordinary/race tests,
build and vet pass in typography and tytui.


## Implemented rendering bypass

M owns the projection from computed document values to typesetter values:

```go
func ProjectStyles(text string, spans []style.Span,
    resolver contracts.StyleResolver, defaults contracts.ResolvedRunStyle,
    policy PaintPolicy) ([]TypographySpan, error)

type TypographySpan struct {
    Start, End uint64 // Processed UTF-8 byte coordinates.
    Style TypographyStyle
}
// TypographyStyle implements styled.Style with a color-free style.Typography
// value and a private run-domain identity. Use returned values unchanged.
```

The shared `Style.Typography()` vocabulary defines the retained properties; X
computes their CSS values, while M owns projection/coalescing. TypographyStyle
contains no Color/Paint field. Itemize/ItemizePolicy use ProjectStyles internally
and retain only these values plus selected resource/script in their run styles.
ResolveStyle accepts both projected and legacy document styles. Synthetic layout
uses the same projection with its injected resolver. Paragraph-level properties
outside font/shaping remain P's responsibility.

Modern equivalent typography coalesces across color differences. Nonempty relative
feature domains retain explicit boundaries even when their colors are identical.
Legacy replay preserves the original document-style boundaries using private
domain identities; no color values enter the typesetter even on this path.
Identical adjacent legacy document styles still share a run. Document Style.Equals
and serialized document fields remain unchanged. CoalesceStyles remains a low-level
compatibility grouping API; use ProjectStyles to build typesetter input safely.

Pipeline.Run retains an independent `[]typography.PaintSpan` snapshot for rendering.
Result.PaintSpans returns detached logical-byte spans. BuildGlyphProof reads this
snapshot, not Input.Source/Styles, so changing inspection data cannot recolor frozen
output. Color validation, black defaults for unstyled input, logical-anchor selection,
RTL visual traversal, and supported soft-hyphen replacement mapping are unchanged.
Original Input.Source/Styles remain the inspection and replay representation, not
typesetter styles. Extraction.StyledText remains a document-style compatibility
helper; the pipeline no longer uses it to prepare typesetter input.

Replay still writes version 2 and preserves version-1 legacy saves. Load rebuilds
the separate renderer snapshot from the existing document colors; no new wire
field, resource identity, or version is needed. Callers must continue treating a
completed result's retained replay Input as read-only when saving a case.

Focused regressions cover projection without color fields, ordinary/legacy run
boundaries, relative feature domains, both synthetic/OpenType paths and breakers,
DOM/non-DOM normalization, detached paint ownership, and wire-color/replayed-proof
compatibility. Existing fixtures cover ligatures, combining marks, pointed Hebrew,
RTL fallback, discretionary replacements, and static pre-migration replay cases.

Validation: full `go test ./... -count=1`, `go test -race ./... -count=1`,
`go build ./...`, and `go vet ./...` pass in typography and tytui. Existing static
replay fixtures were not modified.


## Implemented step-7 API

Runtime compatibility facades are removed. This intentionally changes Go import
paths and composite literals; it does not change glyph output or replay semantics.

| Former workbench API | Current owner/API |
| --- | --- |
| `htmlinput.Style`, `StyleSpan` | `typography/style.Style`, `Span` |
| `fontbackend.Settings` at `pipeline.Input.Fonts` | `*discovery.Resolution`, embedding `typography.Settings` and discovery metadata |
| Resolver, Rule, Info, Binding | Root `typography` package |
| Resource, Origin, bounded reading | `fontresource.Resource`, `Origin`, `Read` |
| LoadFile, LoadResource, Loader, Face, Scale, Stream | `typography/adapter/opentype` |
| Discovery policy/session/request values | `typography/discovery` |
| Controlled provider adaptation | `typography/adapter/fontfind` |

Application composition installs services explicitly (here `otadapter` is the
OpenType adapter import):

```go
resolver, err := typography.LoadSettings(ctx, selection.Settings, typography.Services{
    Loader: otadapter.Services{},
    Coverage: otadapter.Services{},
})
```

`otadapter.Services` implements FaceLoader and Coverage, as well as IdentityProvider
and render.Backend. It validates exactly one file/resource source and rejects
foreign coverage faces. It owns all concrete face assertions; generic M and the
layout core use khipu contracts. Its loading method honors cancellation before and
after synchronous parsing. The application composes controlled provider search with
`findadapter.New` and `discovery.New(searcher, otadapter.Services{}, generics)`.
Neither neutral resolver nor discovery silently installs a parser or provider.

`pipeline.Input.Fonts` is nil for synthetic shaping or a detached Resolution.
File-only callers fill its embedded Settings; request callers use ResolveSettings.
`Result.FontSnapshot` is `*typography.Settings`, pinning all selected resources.
Original descriptor input, discovery policy and resolution diagnostics remain in
`Result.Input.Fonts`; displayed diagnostics are detached separately. Generic
paragraph orchestration has subsequently moved to `typography/paragraph.Run`:
prepared styled.Text, direction and optional zero-byte markup enter P with explicit
width/breaker/assembly parameters and khipu.Config services. It returns frozen
content, a boxed vlist and neutral diagnostics. The local internal/layout package
is removed. See [paragraph-engine step 3](PARAGRAPH_ENGINE_PLAN.md#step-3--expose-styled-paragraph--boxed-vlist).

The rendering bypass is unchanged: `ProjectStyles` owns color-free typesetter
projection, and the renderer reads the separate retained paint snapshot. Document
styles retain color only for inspection, source correspondence and replay input.
No capsule style, general annotation API or version-3 manifest is introduced.

`replay/wire.go` freezes font identity, origin, binding and diagnostic records at
the serialization boundary. Neutral provenance codes convert explicitly to the
existing numeric JSON values, including unknown codes. These records are values,
not runtime service wrappers. Version 1 retains legacy style boundaries; version 2
retains glyph-boundary colors. Existing pre-step-5/pre-step-6 fixtures are unchanged.

Test ownership is now explicit:

- Typography owns resources/identities, parser/stream behavior, resolver policy and
  discovery adaptation. `adapter/opentype/loading_test.go` covers direct injected
  loading/coverage and the original feature-coordinate/nonzero-cluster baseline.
- `pipeline/dom_font_mapping_test.go` retains DOM plus real-font marker/mapping
  regressions; `pipeline/grapheme_mapping_test.go` retains real coverage integration.
  Existing both-breaker freezing, independent HarfBuzz, rendering bypass and replay
  tests stay in tytui. Duplicate facade-only tests have been removed.
- Typography's `boundaries_test.go` audits transitive dependencies of M, style,
  fontresource and render. Tytui's `boundary_test.go` audits the generic layout core
  and forbids backend facade imports. These checks inspect package dependencies,
  not module-wide requirements.
- `replay/wire_test.go` fixes JSON field names/numeric provenance and tests detached
  diagnostics. Static replay cases protect earlier layout and color behavior.

Validation (2026-11-12): full `go test ./... -count=1`,
`go test -race ./... -count=1`, `go build ./...` and `go vet ./...` pass in both
local modules. Local replacements remain required; no package publication or
boxed-vlist implementation is implied.
