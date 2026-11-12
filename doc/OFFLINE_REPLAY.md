# Font manifests and offline paragraph replay

The headless `tytui/replay` package reads versions 1–3 and uses a separate local
font content store. Compatibility layouts save as version 1 or 2; explicit
line-height layouts save as version 3. It supports OpenType and synthetic results.
This is an API for export/import; the TUI's existing Ctrl+S still exports SVG.

```go
// Capture alongside the layout, before editing any source checkout.
revisions, err := replay.CaptureRevisions(ctx, workspaceRoot)
// Handle err, then run the paragraph pipeline and retain its completed result.
err = replay.Save(ctx, result, "case.json", "font-store", revisions)
// Handle err. The original fonts/HTML and font-discovery providers are no longer needed.
reproduced, err := replay.Load(ctx, "case.json", "font-store")
```

CaptureRevisions reads Git HEAD and tracked/untracked dirty status for tytui and
its nine sibling dependencies. Save requires explicit revision evidence; callers
may supply equivalent records for their build environment. Capture at layout time:
these records describe source provenance, not proof that a running binary was
built from that checkout. A dirty flag does not archive the patch. The manifest
also records the exporting Go runtime version. Replay verifies outputs under the
current executable rather than checking out or executing recorded revisions.

## What is pinned

Every configured face is retained at pipeline completion in `Result.FontSnapshot`,
including unused mappings and fallback candidates. Saving never reopens original
font paths. The resolver snapshot shares private immutable bytes; public readers
do not expose the backing slice.

The manifest stores original plain input or original HTML plus the selected DOM
locator, extracted text, styles, source maps and bidi isolates. Replay uses the
retained extraction, not a new DOM parse or network stylesheet fetch. Source HTML
is checked against its original document hash. Plain whitespace normalization is
rerun and the processed paragraph is compared with the recorded result.

The rendering bypass does not change this format. Document colors remain in
Input.Source/Styles on the wire. Run/Load derive color-free typesetter styles and
a separate renderer color snapshot; Result.PaintSpans() returns a detached copy.
Glyph proofs use that snapshot rather than looking up colors in retained input.
Version-1 boundary behavior and version-2 glyph-anchor colors remain unchanged.

Font entries record parsed family/subfamily, origin/request, byte size, SHA-256,
and provenance-scoped resource ID. Selection records default/rule/ordered fallback
references, original file requests, descriptor requests, semantic provider order
and generic mappings, and diagnostics. Provider implementations, environment,
credentials, and API configuration are not serialized. Application-supplied names,
source content, and diagnostic messages are preserved as supplied; callers should
not put secrets in those fields.

Frozen line/knot geometry and run evidence retain feature selections, language,
script, direction, size, glyph positions/clusters, and replacement-local text and
font identities. OpenType cases also retain a structural representation of the
glyph proof. These are verification evidence, not instructions to bypass shaping.
Replay invokes the normal pipeline with pinned resources, then compares paragraph,
breaks, font bindings, frozen runs/geometry, and glyph proofs. A behavioral change
produces an error instead of silently claiming the same result. Initializing a new
randomized shaping feature is outside this version's supported input surface.

## Storage and failure behavior

Save writes font-store/<lowercase SHA-256> objects and a new JSON manifest. Files
are published atomically without replacing existing files; the store deduplicates
content and rejects existing corrupt objects. Incomplete saves may leave valid,
unreferenced font objects. The manifest and each font read are limited to 64 MiB.
This bounds individual files, not the total size of a case with many fonts.

Load accepts only the supported schema/version and units. It verifies every
font's hash, byte size, parsed identity, and references before layout. Missing,
corrupt, or mismatched fonts fail with identifying errors. It does not open recorded
paths, consult registry caches, try another face, or contact providers. Changing
an installed font or provider policy cannot change a pinned replay.

A JSON manifest alone is descriptive when its font objects are unavailable. For
portability, copy the referenced hash-named objects to a local store at the receiving
end. This change does not automatically distribute fonts or produce license-cleared
bundles. Include license/provenance when authorized to share font bytes; otherwise
the receiving end must provide matching external bytes. No automatic garbage
collection or remote download is implemented.

## Tests

`replay/replay_test.go` checks both breakers at narrow/wide soft-hyphen widths,
Hebrew coverage and discretionary identities, unused font retention, file replacement
and deletion, source/style/map round trips, synthetic replay, discovery-policy and
diagnostic retention without provider calls, and rejection of missing/tampered
bytes, hashes, references, versions, source hashes, and proof evidence. Existing
pipeline and immutable-resource regressions remain active.

Shared-style extraction preserves existing version-1 style field names and IDs.
Non-DOM styled cases use optional `Input.Styles` (processed UTF-8 byte coordinates);
plain and DOM cases omit it and retain their old shape. Current readers accept old
cases. Older readers with strict unknown-field checks cannot read new shared-style
cases; use the recorded revisions when replaying them. Revision capture also lists
the local typography checkout (directories without .git are explicitly recorded as `unversioned`, dirty;
missing modules and Git errors still fail).


Typography extraction step 5 preserves manifest version 1. Explicit serialization records
remain the wire boundary; neutral discovery/render services are not serialized.
Frozen evidence obtains identity from Result.Identity, and glyph proofs use the
retained Renderer. Save reads pinned bytes through FontSource; Load validates the
content store through the configured backend without invoking providers or reopening
provenance paths. Static pre-step-5 manifests in replay/testdata exercise both
breakers and mixed Latin/pointed Hebrew with external fixture fonts. Treat these
files as compatibility inputs, not regenerated golden output.


Step 6 introduces a semantic format version: new cases record version 2 and
`Input.PaintPolicy="glyph-boundary-v1"`. Version-1 cases omit that field and load
with legacy style-boundary shaping and knot paint. Resaving a legacy result keeps
version 1 and omits the field again; there is no automatic migration. To deliberately
rerun under the new policy, change Input.PaintPolicy and run the pipeline afresh.
Unknown or inconsistent version/policy combinations fail before layout. Font IDs
and DTO encoding are unchanged. The static pre-step6-paint.json fixture protects
old red-f/blue-i output; new output has a single red fi glyph when the font ligates.


Typography extraction step 7 removes runtime `fontbackend` wrappers. Font records
in `replay/wire.go` explicitly retain the version-1/version-2 JSON field names and
numeric provenance codes; these records are independent of runtime adapter types.
Save reads pinned `typography.Settings` from Result.FontSnapshot and discovery
metadata from Result.Input.Fonts (`discovery.Resolution`). Load reconstructs that
resolution from validated local resources without discovery. It preserves legacy
policy and resource identities. Wire tests and unchanged static fixtures cover this
boundary; moving types introduced no manifest version change.

Rendering colors still bypass the typesetter: replay input retains document styles,
while Run rebuilds color-free typesetter styles and a detached renderer snapshot.
Proof generation uses that snapshot and retained services, not mutable replay input.


Paragraph-engine step 2 retains the compatibility vlist in completed results.
Load rebuilds it through pipeline.Run; Save continues to write only the existing
v1/v2 records and proof evidence. No placement or vertical-policy field is added.
The glyph proof reads retained baselines/line extents, paint spans and paint policy,
so changing inspection Input does not reflow or recolor an existing proof. To change
layout or paint semantics, construct new input and run the pipeline again before
saving. Static legacy manifests now also have load/resave/reload coverage for
identical geometry, colors, identities and versions. New v2 round trips compare
rebuilt placements. Successful diagnostic records retain their existing shape;
assembly failures return StageAssemble without producing a replayable result.


Paragraph-engine step 3 moves bidi preparation and encode/break/freeze/assembly
into the public `typography/paragraph.Run` API. Pipeline.Run prepares input and
injects services, then maps the neutral result into the existing application
records. Replay still reaches this same path through pipeline.Run: v1/v2 wire
values, identities, successful diagnostic messages, ProofCompatibility geometry
and separate paint snapshots are unchanged. No fixtures were regenerated.


## Explicit line-height cases (version 3)

`pipeline.Input.LineLayout` opts into the CSS-inspired baseline line-box policy.
Save selects `LineHeightVersion` (3) for these results, using the detached policy
retained at layout time. `Version` (2) remains the default compatibility format;
legacy paint with compatibility geometry still saves as version 1. Old records
and their static fixture evidence are unchanged.

Version 3 requires Input.LineLayout with a Default line-height and optional Runs
(ordered normalized UTF-8 byte ranges), an explicit glyph-boundary or legacy paint
policy, and Evidence.Layout with box and line placement numbers. Line-height kinds
are `normal`, `factor` and `length`; lengths use DU, factors are unitless. This
version fixes `normal` to ascent+descent+max(0,font gap) and uses half-leading with
baseline alignment and no trailing kern. Strut metrics come from the same pinned
backend font/size defaults used for layout. It adds no provider calls or live fonts.

Versions 1/2 must omit LineLayout and Layout evidence; version 3 must include both.
The loader reconstructs placements and compares evidence even for synthetic cases
without a glyph proof. Invalid or missing policy values, incompatible versions,
changed metrics/placements and tampered line-height values fail explicitly. Both
paint policies are independent of the spacing policy and retain their old meaning.
Round-trip/resave regressions cover both breakers and backends. No VList or service
objects are serialized; rendering still reads separate retained paint and geometry.
