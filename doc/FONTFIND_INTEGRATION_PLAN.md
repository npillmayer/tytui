# Incremental fontfind integration plan

Source review: 2026-11-11, refreshed against `1b6583b`, sibling `../fontfind` (module
`github.com/npillmayer/fontfind`). Steps 1–4 are implemented; later steps remain proposed. Public API and implementation references below are relative to that
checkout unless prefixed with `tytui/`. The step 1 implementation wires the sibling dependency for offline resource adapters.

## Ownership update (2026-11-12)

The original milestones below use their implementation-time `fontbackend` names.
Typography extraction steps 1–7 have since moved resources to typography/fontresource,
neutral discovery to typography/discovery, and provider conversion to
typography/adapter/fontfind. Tytui's forwarding package is removed; F5 and pipeline
compose those APIs directly with injected OpenType services. Existing offline scope,
resource IDs, diagnostics and replay formats are preserved. Current imports and
constructors are documented in [OPENTYPE_INTEGRATION.md](OPENTYPE_INTEGRATION.md)
and [the final contracts](TYPOGRAPHY_API_CONTRACTS.md#implemented-step-7-api).

## API inventory and boundaries

| API | Contract and integration consequence |
| --- | --- |
| `fontfind.Descriptor{Pattern, Style, Weight}` | Pattern-based request; style/weight use `golang.org/x/image/font` enums, not CSS numeric weights. Translate explicitly and retain the original CSS request. |
| `fontfind.ScalableFont` | Resource descriptor with public Name/Style/Weight and private filesystem/path. It is not a parsed or shaped font. |
| `(*ScalableFont).SetFS(fs.FS, string)` | Supports injected/embedded resources. Useful for deterministic tests. |
| `(*ScalableFont).Path()` | Path inside the resource filesystem, not necessarily an OS path or globally unique identity. Never pass it blindly to `os.Open`. |
| `(*ScalableFont).ReadFontData()` | Reads bytes through `fs.ReadFile`; currently no context, streaming interface, or size bound. |
| `NullFont`, `FallbackFont()` | Zero-value unresolved marker and packaged Go-Regular resource, respectively. A fallback resource is not a guarantee of parser compatibility or character coverage. |
| `locate.FontLocator` | `func(Descriptor) (ScalableFont, error)`. |
| `locate.FontLocatorWithContext` | `func(context.Context, Descriptor) (ScalableFont, error)`. |
| `ResolveFontLoc`, `ResolveFontLocWithContext` | Asynchronous lookup using the global registry, with legacy or context-aware providers respectively. |
| `NewResolverPipeline(reg, ...FontLocatorWithContext)` | Client-owned registry and ordered providers; nil registry selects the global singleton. Prefer an explicit per-session registry. |
| `ResolverPipeline.Resolve(ctx, desc)` | Returns `FontPromise`; cache lookup precedes provider calls. Successful hits are cached; exhaustion returns a registry fallback plus an error. |
| `FontPromise.Font()`, `FontWithContext(ctx)` | Blocking await; wait cancellation is distinct from cancelling provider work. Repeated/concurrent awaits retain the same completed result; see lifecycle details below. |
| `locate.FontRegistry` | GetFont(string), StoreFont(string, ScalableFont), FallbackFont() contracts. Resource lookup cache, not parsed-face or content storage. |
| `fontregistry.New`, `GlobalRegistry`, `NormalizeFontname` | New isolated registry or singleton. StoreFont does not replace an existing key. Keys now preserve the exact pattern and numeric style/weight. |
| `systemfont.Find(appkey, IO)` / `FindLocalFont` | Legacy locator over fontconfig-list lookup or system directory search; nil IO uses host I/O. Directory search does not honor style/weight. |
| `fallbackfont.Find()` / `FindFallbackFont` / `Default()` | Embedded locator and explicit default resource. Unmatched lookup now returns NullFont plus an error; Default explicitly selects the packaged default. |
| `googlefont.Find(conf, IO)` / `FindGoogleFont` | Legacy remote/cache provider; `SimpleConfig(appkey)` supplies app-key configuration. Live directory access requires GOOGLE_FONTS_API_KEY. Defer until offline integration works. |
| `Matches`, `ClosestMatch`, `GuessStyleAndWeight` | Filename/variant matching helpers; not authoritative metadata, glyph coverage, or a full CSS font-matching algorithm. |

Read `font.go`, `locate/{locator,resolve}.go`, `fontregistry/registry.go`, and
provider implementations as well as ReadMe.md. There is no general structured
font-enumeration API suitable for a complete TUI family browser. Start with a
request form and resolved-result preview. `ListGoogleFonts` is not such an API.

Tytui already has `fontbackend.Loader.LoadFont(io.Reader)`, bounded to 64 MiB,
which parses bytes, retains them privately, and computes SHA-256. `LoadFile`
adds file validation and path metadata. `Settings`, `Rule`, resolver face maps,
`Binding`, and itemized styles currently identify resources by path. Extend
these together; changing only LoadFile would leave embedded resources ambiguous.
OpenType retains parsing/shaping ownership, khipu retains layout/freezing, and
VisualOrder rendering remains unchanged.

## Upstream prerequisites and observed gaps

Status refreshed against commits `74b3d69`, `b424366`, and `1f042c5`.
Keep general remaining fixes in fontfind, not TUI workarounds.

1. **Promise lifecycle: resolved.** Search completion stores a result and closes
   a notification channel without blocking on waiters. Repeated/concurrent awaits
   return the completed result. Wait cancellation affects only that waiter;
   the context passed to Resolve controls cooperative search cancellation.
   Observed completion wins over cancellation; nil wait contexts are supported.
   `locate/promise_lifecycle_test.go` covers these contracts. Legacy providers
   still do not gain cancellable I/O through adaptation.
2. **Truthful match/fallback results: partially resolved.** Embedded lookup now
   returns NullFont plus an error on no match, allowing later providers to run;
   `Default` remains an explicit default selection. Focused fallback regressions
   cover this fix. Matching remains filename-based. System directory search still
   ignores requested weight/style and returned metadata repeats the request.
   The resolver still discards individual provider errors. Match quality, actual
   metadata, provider provenance, and failure reasons remain gaps for strict
   general discovery. Nil error is not proof of an exact family/style match.
3. **Cache keys: resolved; policy isolation remains a client responsibility.**
   NormalizeFontname now preserves the pattern verbatim and exact numeric style
   and weight in an opaque versioned key, distinct from the reserved fallback key.
   Old lossy keys must be rebuilt. Use a fresh registry when provider order,
   configuration, or fallback policy changes, as the pipeline now documents.
   A cache key identifies a request, not font bytes or resolution provenance.
4. **Concurrency: repaired; bounded/cancellable I/O remains open.** Registry
   logging snapshots the map under lock and no longer mutates tracer levels.
   System and Google provider state is isolated in provider instances, with
   concurrency/state regressions. Custom IO and shared tracers must still be
   concurrency-safe. ReadFontData still uses unbounded fs.ReadFile, and Google IO
   still exposes HTTPGet without context. Checking size afterward does not bound
   the allocation. Add bounded opening/reading and cancellable I/O before remote
   integration; a cancelled promise wait alone does not stop provider work.

The first headless byte-loading slice can proceed independently of these fixes.
Do not block explicit file loading on discovery work.

## Incremental implementation

### 1. Resource loading and immutable identity, offline

Status: implemented. `fontbackend/resource.go` adds immutable bounded-reader and
resolved-fontfind snapshots; Settings supports resource defaults/rules/fallbacks
alongside paths. Content SHA256 and provenance-scoped ResourceID reach Info and
Binding; resolver maps and itemized styles no longer key on display paths.
Existing file UI is unchanged. Fontfind ReadFontData remains unbounded before our
size check; its adapter is restricted to trusted offline resources. Packaged
Gentium works with normal parsing; other packaged faces are not guaranteed.
Full tytui ordinary/race tests, build, and vet pass. See OPENTYPE_INTEGRATION.md.


Add a detached resource-selection model alongside existing file paths. Distinguish
explicit path, descriptor request, and resolved resource. Preserve existing F5
file workflows and first-match mapping rules. Feed resource bytes into the existing
Loader through a reader, preserving normal parser errors, warnings, missing-glyph
checks, and the current size limit. Do not weaken parser requirements to accept
an embedded default; test which packaged faces actually work.

Introduce a resource ID separate from display location in Info, Binding, resolver
maps, and itemized style identity. Use the hash of the exact bytes parsed as the
content identity; retain requested descriptor, resolved name, provider, location,
and parser-derived family/subfamily separately. Never treat a mutable path or
registry key as content identity. Keep private owned bytes for frozen rendering.

Acceptance: explicit paths remain compatible; injected fs.FS and embedded fixtures
produce equivalent layout for identical bytes; identical relative paths in two
filesystems do not collide; changed bytes change identity. Exercise read errors,
invalid/oversized fonts, parser diagnostics, and export after the original resource
is removed. Use fixed fixtures, not installed system fonts.

### 2. Headless descriptor resolution and explicit diagnostics

Status: implemented for controlled offline providers in fontbackend/discovery.go.
NewDiscovery owns fixed policy and a cache of immutable resources plus diagnostics;
ResolveSettings produces normal resource-backed Settings for pipeline.Run.
Providers explicitly attest exact matches; substitution is opt-in and retained
on cache hits. Requests, attempts, identities, and parser warnings are preserved;
read/parse errors stop resolution. CSS family order and common weight/style
translation are tested. General system/remote matching remains gated on upstream
metadata work. See OPENTYPE_INTEGRATION.md for supported API and limitations.


The local fontfind dependency is already wired with `replace => ../fontfind` by
step 1. Preserve that wiring and avoid copying unrelated replacements.
With lifecycle and cache-key fixes available, construct an explicit session
registry and ordered providers. Keep general system discovery gated on the remaining
match-quality/provenance work in item 2. Begin with local discovery and explicitly selected embedded resources.
Translate CSS weight/style to x/image enums with a tested table; preserve CSS
family-list order rather than sending a comma-separated list as one pattern.
Keep generic-family policy explicit and unsupported requests diagnostic.

Represent diagnostics as structured stages: lookup failure/substitution, resource
read failure, parser failure/warning, and missing-glyph coverage. Preserve original
request, actual identity, provider attempts, and reason. Default to strict matching;
allow discovery substitution only under an explicit user-selected policy, then
show it even when layout succeeds. A returned fallback plus error must never be
silently treated as success. Nil error currently is insufficient evidence of a
true match, hence the remaining detailed-result prerequisite for strict general discovery.

Keep discovery substitution separate from existing ordered grapheme coverage
fallback. Resolve every configured coverage candidate once, then let the existing
itemizer choose among parsed faces. Preserve soft-hyphen shaping context and
explicit CSS boundaries. Do not replace a parser failure with another font
silently. Freeze diagnostic and identity snapshots with each completed result.

Acceptance: injected providers test ordered search/cache behavior, exact versus
substituted results, exhaustion, cancellation, key distinctions, and diagnostics
surviving cache hits. Both breakers cover Hebrew fallback and selected/unbroken
soft hyphens. No network or machine-specific discovery in ordinary tests.

### 3. TUI request selection and preview

Status: implemented with an explicit packaged Gentium Plus Regular catalog.
F5 supports file/request modes, per-target family/style/weight requests, ordered
fallbacks, opt-in substitution, and asynchronous preview-before-apply. No general
font enumeration or remote discovery is enabled. Existing file mode is retained
for fonts outside the small catalog. See OPENTYPE_INTEGRATION.md for controls.


Extend F5 with file versus family-request selection, style/weight, source policy,
and ordered coverage-fallback requests. Apply the same choices to default face
and mapping rows. Preview requested versus resolved face, parsed metadata, source,
SHA-256, and any substitution/parser diagnostics before applying the selection.
Do not promise a full installed-font browser until a structured listing API exists.

Run lookup/loading outside Update using existing asynchronous command patterns.
Attach request/revision IDs; cancel superseded work and discard stale completions.
Do not change the displayed frozen result when new selection is pending or fails.
Distinguish discovery substitution from missing-character fallback in the result
inspector, including the affected byte ranges for the latter. Synthetic mode
remains an explicit choice, never a recovery path.

Acceptance: model tests with controllable fake providers cover pending, cancel,
failure, substitution policy, stale completion, mapping edits, and preserved
history. Tests must not depend on a local font installation or timing sleeps.

### 4. Reproducible font manifest and offline replay

Status: implemented as headless replay.Save/Load with a versioned JSON case and
separate hash-addressed local font store. Pipeline results pin all configured fonts
at completion; source HTML/extraction, original requests, semantic provider policy,
revisions, diagnostics, and frozen proof evidence are retained. No provider is
called on replay. Sharing license-cleared bundled font archives is optional future
packaging; missing external bytes fail explicitly. See OFFLINE_REPLAY.md for APIs,
validation, ownership, and revision-capture limitations. TUI Ctrl+S remains SVG.


Add a versioned font manifest to the planned case export/import format. Include:

- Original descriptor or explicit-file request, ordered provider/fallback policy,
  CSS mapping rules, actual parsed metadata, origin, byte size, and SHA-256.
- Stable content IDs referenced by bindings, including coverage-selected faces
  and discretionary replacements; size, features, script, and language belong
  in run/layout configuration rather than the font-byte identity.
- Relevant module revisions and dirty-state indication, alongside the existing
  paragraph source, styles/maps, direction, breaker, widths, and diagnostics.

Store exact font bytes in a content-addressed local store, and optionally include
redistributable fonts in case bundles with license/provenance. Otherwise require
an external resource with the recorded hash. Replay pins content, bypasses discovery,
verifies hashes, and fails explicitly if unavailable or mismatched. Never silently
resolve a newer face. Exclude API keys and transient credentials from manifests.

Acceptance: offline round trips through both breakers preserve bindings and proofs;
changing a path, installed font, provider order, or cache contents cannot alter a
pinned replay. Tampering and unavailable fonts produce actionable errors. A manifest
without accessible exact bytes is descriptive, not a guarantee of replayability.

### 5. Optional remote discovery

Only after cancellation, bounded reads, provenance, and replay are established,
add opt-in Google Fonts selection and explicit download/cache state. Extend upstream
context-aware I/O as needed; waiting with a deadline alone cannot cancel HTTPGet.
Keep ordinary tests offline through injected IO and fixed directory/font fixtures.
Test cache corruption, network failures, cancellation, and changed remote bytes.
Remote results enter the same parsing, diagnostic, and content-pinning path.

## Verification and priority

For each implementation slice, run focused regressions, tytui's full tests, build,
and vet; run race tests for asynchronous/cache changes. Upstream API fixes require
focused reproductions and fontfind's ordinary/race suites. Preserve independent
HarfBuzz and VisualOrder integration tests when changing font selection/loading.

Slices 1–4 are complete for controlled offline resources, providers, TUI
requests/previews, and headless replay. Remote discovery remains optional step 5. Address remaining system-match/provenance diagnostics
before claiming strict automatic discovery; defer remote I/O to slice 5.
Independent mixed-direction physical-layout fixtures remain the separate next
correctness priority in TODO.md; discovery does not close that gap. This plan
requires no shaping-engine or frozen-glyph ordering changes.

Validation of the refreshed plan: public source and regression tests inspected;
`go test ./... -count=1` and `go test -race ./... -count=1` passed in fontfind during the plan review. Step 1 subsequently passed tytui’s
full ordinary/race tests, build, and vet. Live remote discovery was not exercised.

Step 2 validation: focused discovery and pipeline regressions pass, along with
full tytui `go test ./... -count=1`, `go test -race ./... -count=1`, build, and vet.
No live system or remote discovery is exercised.

Step 3 validation: full tytui ordinary and race suites, build, and vet pass.
No installed-font enumeration or live remote lookup was used.

Step 4 validation: replay round trips and rejection tests pass; full tytui ordinary
and race suites, build, and vet pass. Tests use local fixtures and do not resolve
or download fonts during replay.
