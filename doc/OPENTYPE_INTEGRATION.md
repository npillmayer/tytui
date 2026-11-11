# OpenType fonts and shaping (step 6)

This documents the current font integration, including step 7 script itemization,
fallback, and glyph export. See [Multilingual proofs](MULTILINGUAL_PROOFS.md) for
detailed shaping, rendering, and coverage policies.

## Workbench

Press F5 to open the font panel. Enter a local font file and a plain-text size in
big points (1/72 inch). Tab moves between the path, size, and mapping editor.
Ctrl+S validates all files asynchronously and applies the configuration. Escape
or F5 closes without applying the draft; editing or closing invalidates pending
validation. Failures preserve the previous configuration; full error text is
available in the results pane after Escape. F6 explicitly selects synthetic
shaping. Ctrl+R runs the selected backend; changing backend or mappings cancels
and invalidates the current layout without altering saved results.

Each optional mapping row is:

```
family | weight | slant | /path/to/font.ttf
```

For example:

```
serif | 700 | normal | /fonts/Bold.ttf
serif | * | italic | /fonts/Italic.ttf
* | * | * | /fonts/Regular.ttf
```

Rows match in order. Asterisk means any value. Weight is an exact integer from
1 to 1000; slant is normal, italic, or oblique. Family matches a case-insensitive
name in the extracted CSS family list; simple quoted names are accepted. This
is an explicit mapping policy, not browser font matching or system discovery.
Unmatched styles and plain text start with the chosen default file. Ordered
explicit fallback files may supply a missing grapheme.
CSS sizes override the panel size, which is the plain-text default. No synthetic
bold/italic or system-font discovery is performed. Add
`fallback | * | * | /path/to/font.ttf` rows for explicit ordered fallback. All configured files are
validated, even if a rule is not used in the current paragraph.

The result inspector lists file paths, names, SHA-256 hashes, parser warnings,
and processed UTF-8 byte spans mapped to each face and size. SVG export renders
positioned outlines, supported CSS colors, baselines, and line geometry from the
exact frozen result. Font binaries are not embedded. `WriteStructuralSVG` remains
available for box diagnostics; `WriteSVG` chooses glyph output for OpenType and
structural output for synthetic results.

## Headless use and ownership

`pipeline.Input.Fonts == nil` selects synthetic shaping. Set it explicitly to
request OpenType; a failure never switches backends:

```go
result, err := pipeline.Run(ctx, pipeline.Input{
    Text: "office", Width: 200 * dimen.BP,
    Fonts: &fontbackend.Settings{
        DefaultPath: "../opentype/testdata/fonts/GentiumPlus-R.ttf",
        Size: 12 * dimen.BP,
    },
})
```

For styled HTML, also pass the matching `Source` extraction and configure
`Settings.Rules`. Its empty Family/Slant and zero Weight are wildcards.
`Settings.Fallbacks` holds the ordered fallback font paths. Input
settings and source slices are cloned in the result. Every run loads private font
bytes and fresh parsed faces; subsequent file changes affect subsequent runs,
not saved glyph snapshots. Font hashes identify the actual bytes used at layout
time, which may differ from an earlier panel validation. Files are local regular
files, capped at 64 MiB. No fonts are fetched or discovered automatically.

`fontbackend.Loader`, `Face`, `Scale`, `Resolver`, and `Stream` implement the khipu
contracts. Metrics preserve signed font units. Scaling caches the factor from
`otquery.FontScale` and retains fractional DU until khipu rounds geometry. The
bounded stream adapts the synchronous public OpenType shaper directly to a
context-aware channel sink; it starts no goroutines. The producer calls Shape,
while a consumer drains Glyphs, then the producer calls Finish. Errors close the
stream and remain available through Err. Cancellation interrupts reads between
calls and blocked sends; concurrent Shape/Finish/SetInitialCluster operations
are rejected while shaping. Glyph cluster anchors remain original Unicode scalar
indexes, including across calls and explicit SetInitialCluster overrides.
Features and buffering options are translated without changing their coordinates.
All glyph advances and offsets are final shaped font units, never nominal metrics
added a second time. Lazy backend access is serialized per face; metrics and
scale queries use cached immutable values.

## Diagnostics and supported scope

Font loading and coverage errors carry pipeline stage `fonts`. The loader uses
ordinary `ot.Parse`, never the relaxed test-font option. Returned parser errors
and accumulated font errors reject the file; warnings remain visible. The current
parser requires both GSUB and GPOS, even for otherwise valid simple fonts.
Malformed lazy cmap queries are distinct from absent glyph mappings.

Coverage is intentionally strict: every non-control/non-format scalar must have a
cmap glyph in the mapped face. Missing coverage reports the codepoint, processed
UTF-8 byte interval, file, and hash. Variation selectors are exempt from this
preflight; the shaper must still succeed. This policy can reject a character
that normalization could decompose into supported glyphs. Explicit fallback
checks whole graphemes in configured order; normalization-aware coverage and
automatic system-font discovery remain deferred. A shaped .notdef glyph also
fails explicitly with its original scalar anchor. No partial frozen result is
published on any failure.

The adapter registers `otcore` and `othebrew`. Text-based, grapheme-safe
itemization selects Latin/Hebrew scripts; CSS lang supplies language metadata.
Soft hyphens support discretionary replacements through both breakers. Arabic
and broader scripts, variable/collection font selection, and normalization-aware
fallback remain unsupported. Existing OpenType correctness limits still apply.

## Fixtures and verification

No font binaries were copied. Tests use the required sibling OpenType checkout:

- `testdata/fonts/GentiumPlus-R.ttf`, SHA-256
  `fa09788adcc651d77eba7d96f8cc3a600d6484c58e3b4ff79c2b2464dbfea6f8`.
  SIL Open Font License 1.1; the sibling's `ShaperTestLatin-LICENSE.txt` contains
  its embedded license (see `testdata/fonts/NORMALIZATION.md`).
- `testdata/fonts/ShaperTestProgress.ttf`, SHA-256
  `6b4bb3ea60b769f0b385eeed74dc79ab50e61628f458c1a1368548acd59293be`.
  Authored by the OpenType project under its BSD 3-Clause license; provenance and
  generator are in `testdata/fonts/STREAMING.md`.

Frozen `office` glyph IDs, scalar anchors, and advances are checked against the
existing HarfBuzz 15 capture in
`opentype/internal/hbcmp/testdata/text/streaming.json` (`stream_ligatures`, first
word). Progress-font tests check kerning, feature ranges, metrics, and rebasing.
Both breakers, mapped styled runs, detached settings, parser failures, absent
files, missing multibyte glyphs, cancellation under backpressure, stream lifecycle,
panel failure/staleness, backend switching, and small terminal sizes are covered.
No upstream source changes were required for step 6.

## Offline resources and identities

File-path loading and the F5 file mode remain supported. Headless callers may select
exactly one of DefaultPath/DefaultResource, and Path/Resource for each mapping rule.
Fallbacks retains its existing file ordering; FallbackResources follows those
files in slice order. First-match style rules are unchanged.

`ReadResource(reader, Origin)` copies at most 64 MiB plus one detection byte and
returns a Resource with private immutable bytes. `FontfindResource(sf, provider,
request)` reads an already resolved `fontfind.ScalableFont` through ReadFontData,
not by opening Path as an OS path. Its allocation is not bounded upstream before
the size check: use it only for trusted offline resources; use the reader API for
bounded input. Neither API performs discovery. Context-free resource snapshotting
must happen before publishing a request to an asynchronous command.

LoadResource uses the normal Loader parser, preserving parser failures, warnings,
and coverage checks. Packaged Gentium is verified through shaping and proof export;
other embedded fonts, including Go-Regular, are not promised parser compatibility.
No relaxed parser flags or silent fallback are introduced.

Info and Binding separate display Path, Origin (provider, resolved name, location,
requested descriptor), SHA256 of the exact bytes, and ResourceID. SHA256 is the
content identity. ResourceID hashes content plus provenance, allowing identical
bytes from different origins to remain distinguishable. Different bytes at the
same relative path cannot collide in face maps or itemized styles. These IDs are
not yet a versioned replay manifest format. Parsed family/subfamily remain in Info;
provider descriptions are not authoritative font metadata.

Settings.Clone copies selection slices; Resource pointers safely share private
immutable snapshots. Each pipeline run parses owned bytes; frozen faces retain
those bytes and never reread the filesystem during proof export. Resources are now persisted by the separate step 4 replay API; see
[offline replay](OFFLINE_REPLAY.md) for manifests, storage, and verification.

`fontbackend/resource_test.go` covers bounded/read/parser errors, snapshot ownership,
selection ambiguity and cloning. `pipeline/resource_test.go` covers injected fs.FS,
packaged Gentium, identical relative locations with different fonts, mapping and
coverage fallback, and file/resource proof equivalence under both breakers after
source removal. Existing file, missing-glyph, and parser-warning tests remain active.

## Controlled offline descriptor resolution

Step 2 adds `NewDiscovery([]OfflineProvider, genericMappings)` and
`Discovery.ResolveSettings(ctx, DescriptorSettings)`. The result is ordinary
resource-backed Settings suitable for `pipeline.Run`; F5 now uses this layer
for the request mode described below.
There is no automatic system lookup, download, or implicit packaged default.

A FontRequest has ordered Families, CSS Weight (100..900 in steps of 100; zero
means 400), Slant (normal/italic/oblique), and AllowSubstitution (false by default).
Pass family names separately, not an unsplit CSS declaration; names are not parsed
as comma-separated patterns. Generic families require explicit session mappings.
Unsupported intermediate weights are rejected rather than silently rounded.
DescriptorSettings supplies the default, mapping rules, coverage fallback requests,
and size. Original requests are deep-copied into Settings.DescriptorInput.

An OfflineProvider has a stable unique name and a context-aware Find function
returning Match{Font, Exact, Reason}. Exact is a trust assertion by a controlled
catalog provider, not inferred from nil error or fontfind's requested metadata.
For packaged fonts, resolve an explicit known face into a catalog entry; do not
mark arbitrary filename matches or system lookup results exact without verification.
A font plus error or Exact=false is accepted only with AllowSubstitution and is
recorded as a substitution. Missing/declined matches advance through providers
in order, then families in order. No implicit registry fallback is accepted.

Each search uses fontfind's ResolverPipeline with an explicit private registry.
The session caches successful immutable resources together with all diagnostic
values, rather than relying on fontfind's resource-only cache. Provider policy
and generic mappings are fixed at construction; create a new session if they
change. Strict and substitution requests have distinct cache keys. Concurrent
calls are serialized, with cancellation while waiting for the session. Active
providers must cooperate with the search context. Resource I/O itself retains
the trusted/offline and allocation limitations described above.

Resolve/ResolveSettings return diagnostics even on failure. Stages distinguish
lookup attempts, resolved identities/substitutions, read errors, parse errors,
and parser warnings. ResolutionError supports errors.As and unwraps its cause.
Read or parse failure terminates resolution; it never silently selects another
font. Settings.ResolutionDiagnostics and Result.FontDiagnostics retain detached
copies. Existing Info.Warnings and FontBindings remain available. Itemization
failures are wrapped as ResolutionError with stage coverage; this includes the
itemizer's unsupported-script/boundary diagnostics as well as missing glyphs.

Discovery substitution is independent of grapheme fallback: all requested faces
are resolved first, then the existing itemizer chooses among configured coverage
candidates and records exact byte spans and resource IDs. Fontfind still does not
provide general exact-match/provenance diagnostics, so unrestricted providers
remain outside this integration's supported scope.

Tests in fontbackend/discovery_test.go cover descriptor translation, provider and
family order, strict/substituted results, cached diagnostics, generic policy,
read/parse failures, cancellation, and concurrent cache use. The pipeline discovery
test verifies detached diagnostics, mapped resources, Hebrew fallback and soft
hyphens under both breakers. All tests use injected offline fixtures.

## TUI descriptor requests and preview (step 3)

F5 now offers two modes; Ctrl+D switches between existing file paths and offline
requests. In request mode the default field and mapping/fallback target columns
use `families ; weight ; slant`, for example `Gentium Plus ; 400 ; normal` or
`Missing, Gentium Plus ; 400 ; normal`. Families are comma-separated names, not
full CSS string syntax; quoted names containing commas are unsupported. Mapping
predicates still use `family | weight | slant | target`. Each target has its own
requested weight/slant, independently of the rule predicate. Fallback rows retain
list order. All targets in the panel use the selected mode.

The built-in controlled catalog contains only packaged Gentium Plus Regular
(alias Gentium). The generic serif explicitly maps to this face; other generics
have no default mapping. This deliberately small catalog needs no installation or
network. Use file mode for other local fonts, including Hebrew coverage fonts.
Requests for unavailable names/variants fail in strict mode. Ctrl+T explicitly
allows substitution to the catalog's regular face; every such use is labeled
SUBSTITUTION. No bold/italic synthesis or synthetic-backend recovery occurs.

Ctrl+P resolves and validates a draft asynchronously and opens a scrollable
preview of requests, actual parsed faces, provider/location, hashes, warnings,
and substitutions. Ctrl+S in request mode also previews first; Ctrl+S from a
successful preview applies its retained settings. Ctrl+E returns to editing;
F5/Esc closes. Arrow and Page keys scroll wrapped preview text, including hashes.
File mode retains direct Ctrl+S validation/application, with optional Ctrl+P preview.

Lookup runs outside Update. Editing, switching mode/policy, closing, selecting
synthetic, or quitting cancels work. Request/revision IDs discard stale completions.
Pending/failed selections leave the current settings and displayed frozen result
unchanged; application invalidates current layout while retaining saved history.
Result inspection separately labels discovery substitutions and byte-range
coverage bindings. The file/resource limitations above still apply.

`font_requests_test.go` covers syntax and rule/fallback ordering, preview-before-
apply, packaged lookup, substitution policy, retained results, cancelled and stale
completions with controllable providers, and preview bounds. Existing file-panel
and synthetic-switch tests remain active.
