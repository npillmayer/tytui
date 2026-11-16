# fontselection

Application font selection for tytui. This package owns editable file/resource
requests, controlled offline fontfind providers, generic-family mappings,
substitution policy, session caches and detached diagnostic evidence. Typography
receives only already loaded faces and explicit role references.

## File and resource inputs

Build a `Request{Settings: Settings{...}}` and call `Prepare(ctx, request)` before
`pipeline.Run`. A default or rule must select exactly one path or immutable
resource. File fallbacks precede resource fallbacks. Prepare uses OpenType
`Services.Load`, normal parsing, exact retained identity validation, and no discovery.
Nil requests select synthetic layout. Failures publish no partial selection.

`Selection.Fonts` is a `typography.SelectedFonts`: resource IDs select the default,
ordered first-match rules and explicit fallback list. The face table is deduplicated
by identity, not content hash, and includes configured roles unused by this text.
`Selection.Provenance` separately retains requested paths, policy, descriptors and
diagnostics. Clone detaches slices/maps and shares only stable faces/resources.
Keep those handles valid for all retained results. Never serialize Selection.

The TUI keeps Request values editable; each preview or layout prepares its own
loaded selection. A completed selection and proof survive original-file removal.
A later run of an editable file request reads the current file again.

## Controlled family requests

`New(providers, generics)` creates a fixed-policy session using fontfind's private
ResolverPipeline and normal OpenType validation. `ResolveSettings` resolves all
configured roles into a resource-backed Request; then Prepare builds the loaded
Selection. The successful-resource cache remains per session; fresh parsing per
layout is intentional. There is no public Searcher or implicit provider backend.

Keep family/provider order and explicit generic mappings. Providers must attest
`Match.Exact`; a font plus error or an unattested match needs AllowSubstitution.
A private registry disables implicit packaged fallback. Diagnostics preserve
attempt order and substitution evidence on cache hits. Read/parse failures are
terminal and never cached. Calls are serialized with cancellation-aware waiting;
providers must honor cancellation and be safe to share. Create a new session when
policy changes.

`ResourceRequest` and `Descriptor` translate legacy provenance enums, not CSS
weights. `Descriptors` validates CSS request values before translating them.
`ReadResource` snapshots an already selected font; upstream ReadFontData allocates
before the resource size check, so providers must be trusted and offline.

Replay owns versioned DTOs and reconstructs loaded entries from its local store.
It never executes requested paths or descriptors. Owning selection/provider tests
run with `go test ./fontselection` and `go test -race ./fontselection`; application
and replay compatibility require the full suites in tytui and typography.

Preparation validates representation compatibility through the resolver's
`Coverage.CheckFace`, without querying glyphs or shaping. Real preparation pairs
normal OpenType loading with OpenType coverage. Khipu checks actual shaper
instances later during paragraph/document execution and lazy replacements.
No discovery, shaping probe or replay wire change is added to font-only previews.
