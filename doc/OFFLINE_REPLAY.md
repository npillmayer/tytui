# Font manifests and offline paragraph replay

The headless `tytui/replay` package implements version 1 of a case manifest and a
separate local font content store. It supports OpenType and synthetic results.
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
its eight sibling dependencies. Save requires explicit revision evidence; callers
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
