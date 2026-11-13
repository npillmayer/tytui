# Incremental typography extraction plan

Proposed 2026-11-11, following [type-integr.md](type-integr.md). This plan extracts
shared style definitions, typography resolution (M), and the OpenType adapter (A)
from tytui. It does not implement the paragraph engine's boxed vlist, CSS block
layout, new scripts, or remote font discovery. Steps 2–5 implement shared styles,
resources, injected resolution/discovery, and OpenType/fontfind adapters in typography.
Step 6 implements glyph-boundary color with explicit legacy replay. The rendering
bypass follow-up is also implemented: M projects typography-only typesetter styles,
and application wiring supplies colors separately to the renderer. The general
annotation API is deferred. Step 7 is implemented (2026-11-12): runtime compatibility
wrappers are removed; consumers use the extracted packages directly. Steps 1–7
are complete. The step-by-step statuses below record their historical milestones;
the step-7 contract supersedes references to temporary facades.

## Target boundaries

Use one new sibling integration module initially, with several packages rather
than several new modules. `typography` is now the local sibling module `github.com/npillmayer/typography`;
its root resolver, style, fontresource, discovery, render and adapter packages are implemented. No module publication is implied:

| Proposed package | Role and allowed dependencies |
| --- | --- |
| `typography/style` | Shared font requests and shaping/layout values. Document color fields remain for compatibility; M projects color-free typesetter values and rendering properties bypass layout. Depends on cords only as needed to satisfy its style interface, plus existing unit/language types; no DOM, UI callbacks, fontfind, or concrete OpenType types. |
| `typography` | M: mapping rules, typography projection, itemization, coverage policy, bindings, and diagnostics. Uses cords, shared styles, khipu/contracts, and injected face/coverage services. Does not interpret decorations or interaction behavior. |
| `typography/fontresource` | Immutable byte resources, content identity, and descriptive provenance, independent of the parser and discovery provider. No fontfind.Descriptor in its core types. |
| `typography/adapter/opentype` | A: loader, face, metrics/scaling, streaming shaper, and backend outline access. Uses ot/otquery/otshape and khipu/contracts. Concrete faces may exist here but do not appear in M/P public contracts. |
| `typography/adapter/fontfind` | Maps neutral requests to fontfind descriptors; wraps controlled providers and parser services. Owns fontfind-specific result interpretation. Depends on M's interfaces and resource types; concrete loader wiring belongs in the application. |

X initially remains `tytui/htmlinput`, consuming shared styles. Extracting the full
DOM adapter into cords or a separate adapter is subsequent work. P initially
remains `tytui/pipeline`, consuming M and khipu. The application supplies A and
provider instances. The shared box model and line-to-vlist assembly follow later.

### Typography projection and rendering bypass

X does not translate every CSS property into a style for the typesetter. M owns
that projection (`ProjectStyles`); X computes CSS values and application wiring
uses the projection. Paragraph-layout inputs outside font/shaping remain P's
responsibility.

| Category | Examples | Responsibility |
| --- | --- | --- |
| Font selection | Family, weight, slant, fallback preferences | X supplies requests; M resolves faces. |
| Shaping/layout | Size, language, features, direction, spacing, line height | X supplies relevant text/paragraph settings; shaping, bidi analysis, and P consume them. |
| Rendering/interaction | Color, underline, links, mouse sensitivity | The input/application side sends effective properties and logical ranges directly to the appropriate consumer. |

Only typography and necessary context determine shaping runs. A color-only DOM
boundary must not prevent `fi` from becoming a ligature. Properties that change
geometry still belong in typography/layout even if they do not select a new face.

P returns frozen output with source correspondence. The application pairs it with
separately retained rendering properties; the renderer uses the correspondence to
associate logical text with glyphs. Existing source maps and retained snapshots
are the starting point. No capsule styles or general annotation package are needed.
The renderer can receive resolved values without interpreting CSS.

Color remains one value per glyph, selected at its original logical anchor. Keep
that policy and the existing replay formats during the wiring change. Define
underline/hit behavior only when implementing those consumers. See
[the architecture](type-integr.md#style-projections-and-the-rendering-bypass).

Do not add an OpenType-to-khipu dependency to the existing opentype module, or a
reverse dependency from khipu to this integration module. A separate integration
module avoids that cycle. Package-level isolation matters even if its go.mod
necessarily lists all backend dependencies. Prefer one tested backend plus the
existing synthetic path over a universal font framework.

## Current ownership after extraction

| Entity | Current package | Extraction status |
| --- | --- | --- |
| A — OpenType adapter | `github.com/npillmayer` `/typography/adapter/opentype` | Extracted. |
| X — DOM/content adapter | `tytui/htmlinput` | Still in tytui; standalone extraction deferred. |
| M — typography resolver and style projection | `github.com/npillmayer` `/typography` | Extracted; shared style definitions live in `typography/style`. |
| P — paragraph layout engine | `github.com/npillmayer` `/typography/paragraph` | Extracted; consumes prepared styled text and produces frozen content and a boxed vlist. |

- `htmlinput` computes DOM-specific values, whitespace, bidi markup and source maps;
  it uses `typography/style.Style` and `Span` directly, without aliases.
- `typography` owns typography projection, mapping, itemization, coverage policy,
  contractual face entries and pinned settings. The application injects services.
- `typography/fontresource` owns immutable storage and stable identity;
  `typography/adapter/opentype` owns loading, coverage, shaping and outlines.
- `typography/discovery` owns request/session policy; `adapter/fontfind` adapts
  controlled providers. `pipeline.Input.Fonts` carries a `discovery.Resolution`.
- `typography/paragraph.Run` prepares bidi, encodes, breaks, freezes and assembles
  through khipu contracts. The former `pipeline/internal/layout` is removed.
  `pipeline.Run` prepares input, composes services and retains separate rendering colors.
- `pipeline/proof.go` and `replay/evidence.go` consume retained rendering/identity
  services. `replay/wire.go` owns explicit font serialization records, preserving
  version-1/version-2 JSON and IDs. No runtime `fontbackend` package remains.

## Step 1 — Characterize boundaries and agree policy

Status: API sketches and baseline tests implemented; see
[TYPOGRAPHY_API_CONTRACTS.md](TYPOGRAPHY_API_CONTRACTS.md). The proposed sibling
location/module name is recorded there and was created in step 2. Color
policy was subsequently agreed and implemented in step 6; broader decoration and
interaction policies remain separate design decisions.
New baseline tests preserve identity, scaling, mapping, feature coordinates,
source spans, and freezing; existing discretionary/HarfBuzz/replay coverage is
retained rather than duplicated.


Before creating packages, record the intended public values and service contracts
in a small API sketch. Choose the module location and name. Reuse
`khipu/contracts.FontFace`, FontScale, StreamingShaper, and ResolvedRunStyle.
Describe additional needs as narrow services: coverage queries, immutable identity,
resource loading, and backend outline access. Do not expose ot.Font through a
convenience accessor in M.

Add focused baseline cases around current mapping, scale, source-coordinate,
resource-identity, feature-range, and discretionary behavior. Keep independent
HarfBuzz fixtures and mixed-direction physical expectations distinct from tests
comparing two paths through the same implementation.

The color decision is now one color per glyph without suppressing ligatures or
clipping outlines. Other rendering/interaction properties must not introduce
shaping boundaries merely because DOM styles differ. Underline and hit-region
behavior are outside this extraction; their design does not block it.

Acceptance: documented service/type sketch, no dependency cycle, preserved baseline
results, and an explicit color-boundary decision before step 6 changes behavior.
This decision need not block the behavior-preserving moves in steps 2–5.

## Step 2 — Extract shared style values

Status: implemented. `../typography/style` owns Style, Span, Typography and Paint;
htmlinput retains aliases. Itemize/CheckText take shared spans, and non-DOM callers
can use Input.Styles through Run, freezing, glyph proof and offline replay. Full
style equality and existing shaping boundaries were preserved at this milestone;
step 6 subsequently changes shaping projection. See the concrete
step-2 API in [the contracts](TYPOGRAPHY_API_CONTRACTS.md#implemented-step-2-api).


Move the value vocabulary out of htmlinput into the shared style package. Preserve
current fields/semantics initially. Full document-style equality remains distinct
from shaping equivalence. The later bypass follow-up supplies only typography
styles to the typesetter and sends rendering values separately to consumers; it does not
expand Typography/Paint into a general metadata API.
A document style retains complete equality for storage;
shaping equivalence will be a separate operation, not a redefinition of equality
that drops color or provenance.

Use temporary aliases or conversion wrappers in htmlinput so existing callers
continue to compile. X retains DOM traversal, computed-value inheritance and
relative font-size calculation, lang/dir interpretation, whitespace policy,
source maps, and paragraph eligibility. M accepts shared styles and byte spans,
not an Extraction containing document filenames and DOM locators. A later X
extraction can carry those locators separately in a paragraph envelope.

Keep CSS numeric weight/style requests distinct from fontfind/x/image enums.
Do not put parsed faces in document styles. Preserve byte versus scalar coordinate
contracts and distinguish explicitly resolved sizes from unresolved expressions.

Acceptance: non-DOM styled input can reach the existing pipeline; extraction and
source-map tests are unchanged; neither shared styles nor M-facing signatures
import w3c-dom or htmlinput. No layout change is expected in this step.

## Step 3 — Extract immutable resources and OpenType adapter

Status: implemented. Typography owns bounded immutable resources, fixed version-1
identity encoding, and the OpenType loader/face/scale/stream/outlines. Tytui facades
preserve existing signatures and legacy descriptor/replay shapes; discovery
conversion was subsequently moved into adapter/fontfind in step 5. Generic
encode/break/freeze orchestration initially moved to pipeline/internal/layout;
paragraph-engine step 3 subsequently moved it into typography/paragraph.Run. Pipeline.Run remains application composition with backend selection.
Full ordinary/race suites, build and vet pass in both modules; fixed identities,
request conversion, source removal, outline concurrency and stream regressions
cover the extraction. See the concrete step-3 API in the contracts document.


Move immutable byte snapshots, provenance and content hashes into fontresource.
Replace its dependency on fontfind.Descriptor with a neutral request value;
fontfind conversion belongs in the discovery adapter. Preserve SHA-256 content
identity separately from provenance-scoped resource identity. Define and test the
encoding used for existing IDs before changing any representation.

Move Loader, Face, Scale, Stream and outline access into A. Preserve the normal
parser policy, diagnostics, 64 MiB bounded-reader limit, retained bytes, lazy-data
synchronization, and cancellation/backpressure behavior. OpenType-specific face
assertions remain legal inside A's shaper and renderer adapters. M/P receive
contract interfaces and opaque identity, not A's concrete Face.

Provide temporary forwarding wrappers in tytui/fontbackend. The workbench remains
buildable after each move. Configure A from application composition code. M will
receive a loader and coverage service in step 4; avoid a default constructor in M
that silently installs OpenType.

Acceptance: file and resource proofs match existing fixtures; mutation/read/parser
failures, frozen export after source removal, scale, missing-glyph and stream tests
pass in A and through tytui. Synthetic layout builds and runs without importing A
from its generic orchestration package.

## Step 4 — Extract M behind injected services

Status: implemented. The root typography package owns Settings/Rule, Resolver,
Binding/Info, mapping and itemization policies, and snapshot assembly. LoadSettings
requires explicit Services (FaceLoader and Coverage), with opaque contracts.FontFace
entries and retained neutral resources. It validates identity against retained bytes.
Tytui retains public signatures/legacy DTOs and injects its OpenType facade; discovery
metadata stays local. ItemizeWithMetadata introduced ShapingSpan/ShapingKey and
PaintSpan without merging former boundaries at this milestone; step 6 adds
coalescing. Fake-service regressions, full ordinary/race
suites, build and vet pass in both modules. Dependency inspection confirms M imports
neither DOM/htmlinput, discovery, nor the concrete adapter.


Move mapping rules, resolved typography, script/grapheme itemization, coverage
selection, binding records, and snapshot assembly into M. Split mapping policy
from parsing: an injected service supplies usable faces, immutable identity and
coverage queries. M uses contracts.FontFace for layout and returns
contracts.ResolvedRunStyle through its resolver implementation.

Replace concrete Face maps with entries containing contractual faces and associated
identity/resource metadata. Replace htmlinput.Extraction arguments with styled text
or shared style spans plus the minimal paragraph metadata needed by itemization.
Preserve rule order, family matching, grapheme coverage, script restrictions,
soft-hyphen neighbor context, and explicitly configured fallback order.

Keep baseline run segmentation unchanged during this move. Introduce explicit
shaping-key/paint-span representations, but do not merge former boundaries until
step 6's policy and coordinate handling are implemented.

Acceptance: a fake font/coverage service exercises M without OpenType parsing;
OpenType integration still passes both breakers. Binding identities and byte ranges
match the pre-extraction fixtures, and M imports neither htmlinput nor A.

## Step 5 — Extract discovery adaptation and migrate consumers

Status: implemented. Typography/discovery owns neutral requests, generic mapping,
ordered-family policy, validation, session caching and resolution snapshots;
adapter/fontfind owns provider attestation, ResolverPipeline and descriptor/resource
conversion. Loading remains explicitly injected. F5 uses the extracted provider API
through the legacy settings facade, preserving its controlled offline catalog.

The generic proof walker uses render.Backend and IdentityProvider; OpenType services
own sfnt paths and VisualOrder. Replay evidence uses the identity service, pinned
bytes use FontSource, and legacy DTOs preserve version 1. Static pre-step-5 mixed
Latin/Hebrew manifests for both breakers pass without regeneration. Full tests/races,
build and vet pass in both modules; existing preview/cancellation and provider-free
replay tests remain active. Step 6 subsequently implements the agreed glyph-boundary policy.


Separate neutral request policy from fontfind-specific provider adaptation.
Retain exact-match attestations, opt-in substitutions, ordered families/providers,
explicit generic mappings, session cache isolation, cancellation, and diagnostics
on cache hits. The adapter translates weight/style enums and wraps fontfind's
ResolverPipeline; injected loading services validate successful resources.
Fontfind itself continues to own discovery mechanics and generic concurrency fixes.

Rewire the F5 editor to the extracted services. Keep its small controlled offline
catalog and file mode; do not expand discovery scope as part of this extraction.
Settings snapshots and original descriptor/policy records must remain detached.

Move backend-specific outline access from pipeline/proof.go into an A rendering
adapter or inject an outline service into a generic proof walker. Glyph traversal
continues to use otshape.VisualOrder at the backend/rendering boundary. Replace
replay/evidence.go's concrete face assertion with identity access through the
agreed service. Replay obtains pinned bytes through resource interfaces rather
than reopening files or rediscovering fonts.

Use explicit version-1 replay DTOs if internal types change. Test reading committed
old-format cases (with test fonts supplied separately) and preserving IDs. If a
semantic format change is unavoidable, introduce a new version and an explicit
compatibility policy instead of silently reinterpreting version 1.

Acceptance: file/request TUI tests, preview/cancellation/stale-result tests, resource
and discovery tests, and offline replay pass. Provider callbacks are never invoked
during replay. P's generic orchestration and M have no concrete OpenType imports or
assertions; backend code is selected by application wiring.

## Step 6 — Preserve shaping across color boundaries

Status: implemented for color with the agreed glyph-boundary policy. Document
styles/retained replay input keep color; the bypass follow-up below removes it from
the actual typesetter styles. New runs use
`glyph-boundary-v1`: color-only/equivalent resolved typography spans coalesce before
grapheme/script/fallback analysis. Original document styles/paint/source maps remain
separate. Each glyph receives one color from its original logical scalar anchor;
a ligature stays one color, repeated anchors share paint, and independently anchored
marks may retain their own color. No clipping or paint-conflict diagnostics are added.

Actual font/size/language/bidi-context differences remain boundaries. Feature-bearing
spans conservatively retain their original domains; khipu still clips/rebases their
ranges. Both-breaker tests verify feature glyphs and original scalar anchors. Frozen
metadata suffices for the current explicit-soft-hyphen producer: ordinary anchors
are paragraph-relative; pre/post-break replacement anchors map through retained
source bytes, with '-' taking the selected soft-hyphen marker's paint. Unsupported
generated-text transformations require an explicit mapping and are rejected.

New replay saves use version 2 with the explicit policy. Version-1 cases load using
`style-boundary-v1` and resave as version 1, preserving old shaping/painting. Static
pre-step-5 and colored pre-step-6 cases pass unchanged. Tests include ligatures,
Latin marks, pointed Hebrew, RTL fallback/discretionaries, source ownership and
both breakers. Full ordinary/race suites, build and vet pass in both modules.


The implemented scope follows the color policy agreed in step 1. Project document
spans into shaping keys
(face identity, size, features, language/script and appropriate direction/context)
and independent paint spans. Merge only when shaping context permits. Direction
remains paragraph/bidi analysis data; it must not be incorrectly moved into khipu's
StyleResolver contract, which currently does not resolve direction.

Rebase feature ranges and preserve byte/scalar/source correspondence when formerly
separate style spans are coalesced. Do not concatenate streams or reorder knots
as a shortcut. Khipu may need an owning-package change if its current style/bidi
intersection machinery cannot consume the projected spans correctly. Put the
minimal general fix and regression there; keep CSS and paint policy out of khipu.

Teach rendering to associate retained paint with clusters/glyphs under the agreed
ligature policy. Preserve paint metadata through freezing/replay without adding
DOM dependencies to those layers. Update the manifest version if evidence or paint
semantics changes require it.

Acceptance: color-only boundaries preserve shaping where the policy permits;
real face/size changes split correctly; equivalent typography avoids redundant
splits; combining marks and multi-grapheme ligatures have explicit expected paint;
soft-hyphen replacements, fallback, isolates, features and source mappings remain
correct under both breakers. Independent shaping expectations remain authoritative.

### Follow-up — Simplify rendering-property wiring

Status: implemented. M's `ProjectStyles` produces `TypographySpan` values with
color-free `TypographyStyle` styles. Itemize/ItemizePolicy use the same projection;
the synthetic path applies it directly. Explicit private domain identities preserve
legacy document boundaries and relative feature origins without carrying color.

Pipeline.Run copies original logical paint spans into a separate renderer snapshot.
`Result.PaintSpans()` returns a detached copy; glyph proofs read this snapshot,
not document/replay input. Input.Source/Styles and their wire fields stay unchanged.
The existing normalization and glyph/replacement source correspondence are reused;
no generic annotation model or new replay version is introduced. Full ordinary/race
suites, build, and vet pass in typography and tytui.

Acceptance: color-only DOM differences preserve font selection, shaping, and line
breaks; frozen proofs retain their colors under both breakers; existing source-map,
ligature/mark/discretionary and version-1/version-2 replay regressions still pass.
Do not change replay semantics merely to reorganize in-memory ownership.

The [general annotation API proposal](ANNOTATION_API_CONTRACT.md) is **deferred**.
Its package, generic payloads, whole-cluster decoration/hit policies, and proposed
replay version 3 are not part of the current plan. Underline and interaction
consumers can be designed separately when needed. This follow-up is documentation
of the completed color bypass; broader rendering consumers remain future work.

## Step 7 — Remove compatibility wiring and stabilize the boundary

Status: implemented. All TUI, pipeline and replay consumers use the extracted
packages directly. `fontbackend` forwarders and `htmlinput.Style`/`StyleSpan`
aliases are removed. `adapter/opentype.Services` implements injected face loading
and coverage as well as retained identity/rendering. `discovery.Resolution` carries
selection metadata; `Result.FontSnapshot` is a pinned `typography.Settings`.

Replay owns explicit font wire records in `replay/wire.go`, preserving numeric
provenance codes, identities, diagnostics and both existing manifest versions.
Static pre-extraction fixtures remain unchanged. The typography-only typesetter
styles and detached renderer paint snapshot retain their existing behavior.

General resource/parser/stream/resolver tests live in typography; real DOM/font
integration remains in pipeline. The feature-coordinate/nonzero-cluster baseline
now lives in the OpenType adapter. Dependency regressions audit transitive imports
for the generic typography packages and the layout core, and forbid reintroducing
the workbench backend facade. Full ordinary and race suites, build and vet pass in
both modules. See [step-7 contracts](TYPOGRAPHY_API_CONTRACTS.md#implemented-step-7-api).

Use the typography-only typesetter boundary and separate rendering path as the
target for cleanup. General annotation storage or consumers are not prerequisites.
Preserve existing rendering/replay behavior while removing compatibility wrappers.

Remove tytui aliases/forwarders once all callers use extracted packages. Keep
integration regressions in tytui and move general unit regressions to their owning
packages. Update AGENTS.md, dependency wiring, API guides, and replay documentation.
Audit imports to prevent M or shared styles reacquiring DOM/concrete backend types.

Tytui should retain UI state, user policy selection, orchestration, inspection,
and case/proof integration. Paragraph assembly into a boxed vlist is the next
architectural project, not hidden inside this cleanup. Do not extract unrelated
page layout, all DOM handling, or the whole replay subsystem merely because the
new module exists.

## Verification and rollout

Each step is a reviewable change that leaves the workbench usable. Run focused
owning-package regressions, full ordinary tests, build and vet for changed modules,
and the tytui integration suite. Run race suites when moving mutable font data,
streaming, resource/cache ownership or asynchronous discovery; finish the extraction
with full race verification. Preserve independent fixtures rather than regenerating
expectations from the extracted implementation.

Use local replacements for the new sibling module during development. Avoid
unrelated dependency upgrades or cleanup. Compile tests or import checks should
verify that shared styles/M do not import htmlinput, w3c-dom, or concrete OpenType
packages. Document which boundaries are temporary wrappers and remove them in
step 7 instead of keeping two implementations.

Next: [the paragraph-engine plan](PARAGRAPH_ENGINE_PLAN.md) has implemented its
frozen lines → boxed vlist, retained rendering placements and reusable
styled-paragraph entry point steps, plus explicit baseline line-height policies.
DOM line-height extraction and the CSS box bridge remain follow-up work.
The rendering bypass remains in place;
the general annotation API remains deferred.

Validation of this plan: inspected current styles, mapping/itemization, font and
stream APIs, discovery/resource ownership, khipu contracts, proof rendering and
replay consumers. Verification for steps 2–7 is recorded in the API contracts.
