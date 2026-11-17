# Current experimental paragraph replay

`tytui/replay` writes and reads **version 5 only**, using a separate local font
content store. Versions 1–4 and unknown versions fail explicitly before current
schema decoding, font acquisition or pipeline execution. There is no migration
reader and no promise of indefinite behavioral compatibility. This is a headless
paragraph API; TUI Ctrl+S still exports SVG. Document replay remains deferred.

```go
revisions, err := replay.CaptureRevisions(ctx, workspaceRoot)
// Handle err, then run the pipeline using the captured checkout state.
err = replay.Save(ctx, result, "case.json", "font-store", revisions)
// Handle err. Original font files and discovery providers are no longer needed.
reproduced, err := replay.Load(ctx, "case.json", "font-store")
```

## One format, current paint, explicit assembly

Version 5 retains glyph-boundary paint behavior. Color bypasses typesetter styles;
rendering uses separate logical paint ranges and retained source/cluster mapping.
A ligature is one colored glyph. There is no `Input.PaintPolicy` wire field;
strict decoding rejects one even if it names the current policy.

`Input.Assembly` is required:

| Mode | Required input | Placement evidence |
| --- | --- | --- |
| css-line-height-v1 | LineLayout present, with default and optional byte-range overrides | Required |

CSS is the only assembly mode. ProofCompatibility is removed. New paragraph
runs with no line-height option use explicit CSS normal; Save records that
retained value. Load does not silently supply omitted values. Evidence.Layout
records box geometry and line placements without the obsolete Gap field.
Unknown/retired modes, missing line-height or missing evidence fail explicitly.

The pipeline and typography now have only current glyph-boundary behavior:
PaintPolicy selectors, legacy shaping/color branches and the intermediate Save
guard are removed. Assembly values still come from `Result.LineLayout()`, not
mutable inspection metadata. Source paint stays separate and rendering consumes
retained glyph/source correspondence. This semantic/schema change advances the experimental format to version 5.
Version-4 manifests (including CSS cases) remain historical rejection cases;
there is no compatibility reader or automatic conversion.

## What remains pinned

`Result.FontSnapshot` retains immutable resources for configured defaults, rules
and ordered fallback candidates, including unused candidates. Save reads those
bytes, never original paths. Font records retain SHA-256, byte size, parsed names,
provenance-scoped ResourceID and origin/request evidence. Load validates all stored
fonts and gives only referenced entries to the resolver. Resource content and
selection ordering remain unchanged by this format transition.

Selection provenance includes requested paths, descriptor requests, provider
names, generic mappings and diagnostics. Load/save preserves these records but
never executes them as discovery requests. Provider implementations, runtime
faces/services, credentials and environment settings are not serialized.

The manifest retains plain input or original HTML and selected DOM locator,
extracted text/styles, source maps and bidi isolates. HTML hashes are checked;
retained extraction is used without reparsing HTML or fetching stylesheets.
Plain whitespace normalization runs normally and its processed text is verified.
Wire styles retain document colors; pipeline execution rebuilds color-free
shaping input and separate paint. Frozen output and placements remain immutable
rendering inputs.

CaptureRevisions records HEAD and dirty status for tytui and its nine paragraph
pipeline sibling dependencies; unversioned module directories are labelled.
Capture at layout time. This metadata and the recorded Go runtime describe the
environment, not an archived patch, reproducible binary or executable checkout
instruction. Explicit equivalent revision records may be supplied by a caller.

## Verification and failures

Load reruns the normal pipeline with pinned resources. It compares processed text,
break positions, font bindings, placements, frozen lines/knots, shaped runs and
OpenType glyph proof evidence. Category-specific evidence errors expose drift;
Load never updates expected output. Diagnostic prose is retained as information,
not compared as a layout contract.

Save publishes a new manifest without overwriting existing files and deduplicates
font objects by lowercase SHA-256. Existing corrupt objects fail. Partial saves
may leave valid unreferenced objects. Manifest and individual font reads are
bounded to 64 MiB; this is not a total-case size limit. Missing/tampered bytes,
invalid references, unknown fields, invalid units and mismatched identities fail
without substitution or provider access. Cancellation and retained-resource
lifetime contracts remain in effect.

To share a case, provide its referenced hash-named objects subject to font
licenses. JSON alone cannot reproduce missing fonts. No automatic font download,
redistribution, garbage collection or discovery is introduced.

## Regression cases and deliberate changes

See [the regression inventory](REPLAY_REGRESSION_INVENTORY.md) and
[fixture notes](../replay/testdata/README.md). The original 16 step-1 SVG/placement controls
remain byte-for-byte unchanged. The eight CSS controls remain executable; the
eight compatibility controls are historical. Version-4 current-fi fixtures now
test rejection alongside the older fixtures. Current version-5 tests exercise
normal/factor CSS, pinned fonts, source removal, load/save/load and tampering.

For an old case that needs continued investigation, use its pre-change checkout
to recover input and pinned fonts, then deliberately run the current policy and
review new evidence. Save a new case with current provenance; do not claim that
its historical output was reproduced. No permanent converter is provided.

An intentional algorithm change requires reviewing affected evidence and explicitly
updating fixtures. An incompatible format/semantic-contract change increments the
version and replaces support. Tests never automatically regenerate expectations.
The [simplification plan](REPLAY_SIMPLIFICATION_PLAN.md) is complete. Historical
milestone documents link here; their old format promises are superseded.

## Creating and reviewing a fresh case

1. Preserve the failing input, selected node/extraction, width, breaker, assembly
   values and pinned font resources. Record checkout revisions at execution time;
   a dirty flag does not preserve the patch itself.
2. Reproduce the finding through the normal pipeline. For a historical manifest,
   use an appropriate old checkout to recover input/resources if necessary, then
   construct a new current input. Do not edit its version number to bypass rejection.
3. Inspect the frozen glyph/source correspondence and numeric placements. Compare
   a retained SVG proof and the relevant independent expectations (for example,
   the HarfBuzz or hand-calculated geometry tests). Explain any intentional change
   in glyphs, colors, breaks or baselines before accepting new evidence.
4. Call Save with a new manifest path and the retained result/resources. Verify
   Load and load/save/load using the local store, including after removal of the
   original font files. Review the manifest/fixture diff and record the input,
   font hashes, provenance and reason for the expectation in the fixture README.
5. Run `go test ./replay ./pipeline ./documentlayout -count=1`, then the affected
   modules' suites. Keep the unchanged controls as controls; update only evidence
   justified by the reviewed behavior change. Load never blesses its own mismatch.

There is no blanket fixture regeneration command. Font licenses/provenance still
apply to any shared bytes. JSON field ordering alone is not a compatibility
promise, while resource identity codes, selected order, source correspondence and
the recorded layout evidence have concrete regression checks.
