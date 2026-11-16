# Replay regression inventory

Steps 1–4 of [the simplification plan](REPLAY_SIMPLIFICATION_PLAN.md), completed 2026-11-16.
Step 1 established the controls below without changing behavior. Step 2 now accepts
only replay version 4; the controls and historical fixture bytes remain unchanged.

## Current behavior to retain

`replay/current_behavior_test.go` constructs inputs directly, without loading an
old manifest or selecting a legacy policy. `TestCurrentBehaviorReplayControls`
checks 16 OpenType cases: both breakers, both existing vertical modes, and four
inputs (split-color fi, mixed Latin/pointed Hebrew with a combining accent, selected
RTL soft hyphen, unbroken RTL soft hyphen).

The checked-in `current-controls.json` captures SVG SHA-256 and inspectable numeric
box/line placements. These are current-output controls, not a new replay schema
or independent proof of shaping correctness. Handwritten assertions additionally
require one red fi glyph, the expected discretionary colors, and replacement paint
correspondence to the original soft-hyphen byte position. Existing independent
shaping and geometry tests below remain the correctness oracles.

Each case removes both source font files before Save, preserves configured unused
mappings/fallbacks, and loads from the content store. It compares source/style maps
and font bindings, verifies the loaded SVG/placements against the static controls,
then removes inspection-source styles and verifies retained rendering again.
The original result must also remain unchanged after saving and rendering.

`TestCurrentSyntheticReplayPlacements` checks both breakers and vertical modes
against hand-calculated two-line geometry, before and after replay without a font
store. An explicit 10bp synthetic style gives compatibility baselines 8/20bp and
24bp extent; factor-2 CSS line-height gives baselines 13/33bp and 40bp extent.
This deliberately does not assume the default synthetic font uses big points.

## Step-1 test classification (before reader retirement)

| Tests | Purpose and disposition |
| --- | --- |
| `TestOfflineRoundTrip` | Current correctness/resource lifetime: both breakers, selected/unbroken Hebrew discretionaries, replacement font identity, unused primary/mapping retention, replaced/deleted source font. Retain. |
| `TestHTMLAndSyntheticReplay`, `TestSharedStylesOfflineReplay` | Source/style/map correspondence and synthetic/non-DOM inputs. Retain. |
| `TestDescriptorReplayBypassesDiscovery` | Offline loading, detached discovery provenance and no provider calls. Retain. |
| `TestReplayRejectsMissingAndTamperedResources`, `TestReplayCancellation` | Resource integrity, invalid references/version/evidence/source hash, cancellation. Retain and adapt format assertions in step 2. |
| `TestReplayValidatesUnreferencedFontsWithoutSelectingThem` | Validate extra resources without changing selected faces. Retain. |
| `TestCaptureUnversionedModules` | Revision provenance for local development. Retain. |
| `TestRenderingBypassReplayKeepsWireColorsAndPolicies` | Current paint bypass, source-domain projection and retained placement checks mixed with legacy-policy coverage. Keep current cases; remove legacy combinations in step 3. |
| `TestLineHeightReplayVersion3` | Current vertical policy, placements and tamper checks mixed with format/legacy assertions. Retain current correctness; adapt schema checks in step 2. |
| `TestPaintPolicyVersions` | Primarily historical policy dispatch/resave. The modern fi assertion is independently covered by the new control and pipeline tests; retire historical execution after step 2. |
| `TestPreStep5VersionOneFixtures`, `TestLegacyPlacementEvidenceSurvivesResave` | Historical fixture loading/resaving. Useful mixed-script/resource/placement questions have current coverage; historical wire/output promises alone can retire in step 2. |
| `TestWireProvenanceCodesAndFieldNames`, `TestSelectionWireShapeAndEmptyNormalization`, `TestSelectionResaveNormalizesRequestedPaths` | Mostly historical wire shape/normalization. Retain selection order, provenance and empty-value correctness as applicable to the new schema; numeric-code/old-field commitments can retire. |

No test is removed in step 1. `selected_fonts_test.go` supplies setup helpers, not
an independent historical requirement.

## Integration oracles outside replay

| Boundary | Existing coverage retained and verified |
| --- | --- |
| Ligatures/combining marks/pointed Hebrew | Pipeline `TestGlyphBoundaryPaintPreservesShaping` compares colored and uncolored shaping; `TestIndependentGlyphProofs` checks glyphs and positions against upstream HarfBuzz fixtures. |
| Repeated anchors and visual ordering | Pipeline `TestFrozenGlyphProofVisualOrderCompatibility` checks both breakers, pointed Hebrew, repeated anchors and selected/unbroken replacements, including snapshot immutability. This checks helper integration, not independent shaping correctness. |
| Mixed-script paint and source geometry | `TestMixedScriptsFallbackAndProof`, `TestRTLFallbackAndSoftHyphenPaint`, and `TestIndependentRetainedLinePlacements` preserve bidi, fallback, logical-source lookup and hand-calculated synthetic coordinates. |
| Feature origins | Pipeline `TestPaintCoalescingPreservesRelativeFeatureDomains`; typography `TestRenderingBypassKeepsFeatureDomainsWithoutColor`, `TestItemizeInlineFeatureDomains`, `TestItemizeInlineNonzeroFeatureOrigin`, `TestItemizeInlinePaintFeatureDomains`; OpenType adapter `TestItemizeInlineFeatureLigatureAndSourceCoordinates` and `TestPreparedInlineFeatureDomainsAndReusableLayout`. Preserve current-policy cases when deleting legacy branches. |
| Retained services/geometry | Pipeline `TestRenderingDoesNotRemeasureRetainedLines`, `TestRetainedPlacementsSurviveInputMutationAndSourceRemoval`, and font selection/resource lifetime tests; typography resource/retained-render tests. These are ordinary rendering contracts, not replay-only machinery. |

## Historical fixtures and their successors

| Fixture | Original integration question | Current coverage / intentionally historical part |
| --- | --- | --- |
| `pre-step5-firstfit.json` | Typography extraction must preserve font identity, mixed Latin/pointed Hebrew, soft-hyphen evidence and geometry under firstfit. | New firstfit controls plus `TestOfflineRoundTrip`, source-map tests and independent pipeline oracles cover these questions. Exact v1 JSON and historical output are compatibility-only. |
| `pre-step5-knuth-plass.json` | Same boundaries under Knuth–Plass. | Corresponding Knuth–Plass controls and the same independent/integrity tests. Exact v1 execution is compatibility-only. |
| `pre-step6-paint.json` | Old red-f/blue-i style boundaries must survive legacy loading. | Current `ligature` controls explicitly require one red glyph. Two differently colored glyphs are intentionally historical behavior, not an expectation to carry forward. |

New controls preserve the integration questions; they are not claimed to be a
byte-for-byte conversion of those manifests. Historical fixtures remain unchanged
as rejection cases in step 2. Their origin and font hashes are in the fixture README.

## Step-2 current-format coverage

| Current tests | Coverage replacing historical dispatch |
| --- | --- |
| `TestHistoricalFixturesRejectedBeforeResources` | All three historical fixtures fail with a version error using an absent font store. |
| `TestUnsupportedVersionsBeforeCurrentSchemaAndResources` | Versions 0, 1, 2, 3 and unknown future versions fail before current-schema/invalid-font processing. |
| `TestCurrentFormatValidation` | Both assembly modes require placement evidence; unknown/missing/inconsistent modes, changed box/baseline evidence, units, unpinned font fields and a serialized paint selector fail. |
| `TestSaveRejectsRetainedLegacyPaint` | Both vertical modes reject a retained legacy result despite editing its inspection policy; no case or font store is created. |
| `TestCurrentFixturesSurviveResave` | New v4 fi fixtures for both breakers match step-1 controls, preserve font/selection evidence and placements through load/save/load, and use retained policies despite inspection edits. |
| `TestCSSAssemblyReplay`, `TestRenderingBypassReplayKeepsWireColors` | Former multi-policy cases now exercise the current glyph policy in version 4, retaining line-height/paint/source checks. |
| `TestSelectionResavePreservesRequestedPaths` | Original request paths and ordered fallback provenance now survive load/save as inert evidence. Font identities and output remain unchanged. |

Historical load/resave tests were replaced by these cases; there is no version-1
execution remaining in replay tests. At the end of step 2 the independent typography/pipeline legacy
branches remained; step 3 below removes them. The new current fixtures
are captures with explicit provenance, not an importer or automatic migration.

## Verification (2026-11-15)

Focused tytui replay/pipeline/documentlayout tests passed. Full ordinary and race
suites, `go build ./...`, and `go vet ./...` passed in both tytui and typography.
No sibling code or existing fixture was changed. These checks include the
upstream typography feature/atomic/resource cases listed above; they are not a
claim that every sibling module's independent suite was rerun.

Step 2 repeated focused replay/pipeline tests, full ordinary/race suites, build
and vet in both modules: all pass. All 16 step-1 SVG/placement controls and original
historical JSON bytes are unchanged. No upstream code or paragraph geometry/paint
algorithm changed; khipu's independent suite was not rerun.

## Step 3: one shaping/color behavior

Removed all production paint-policy selectors and legacy branches in typography
and tytui. Current-policy test combinations remain; feature-domain/atomic tests
now call the simplified APIs directly. The retained renderer still validates paint
before ordering services, preserving the tested error/cancellation behavior.
`TestSaveRejectsRetainedLegacyPaint` was an intermediate step-2 guard and is now
removed: the pipeline cannot produce a legacy-policy result. Historical version
and forbidden wire-selector rejection tests remain. `TestCurrentFixturesSurviveResave`
still checks retained assembly after inspection edits. No static control or
manifest evidence was regenerated.

Step 3 verification (2026-11-15): full ordinary and race suites, build and vet
passed in both typography and tytui. Khipu and css-box-layout full ordinary suites
also passed; their race suites were not rerun. The API consumer search across the
sibling Go sources found only typography and tytui callers. Resource/identity code,
assembly modes, source maps and checked-in replay evidence remain unchanged.

## Step 4: final cleanup (2026-11-16)

`TestWireProvenanceIdentityAndOwnership` replaces whole-JSON-string expectations
with independent fixed numeric identity codes, decoded provenance round trips,
detached warnings and nil evidence checks. `TestSelectionProvenanceNormalization`
checks normalized provenance and the prohibition on serializing runtime selections;
it no longer freezes incidental field order or null/empty spelling. Selection
resave/resource-validation tests reuse the shared manifest test helpers.

No unused production compatibility adapter remained: each wire conversion is
needed by current input, selection/resource evidence or provenance. Historical
rejection tests and all current correctness/resource tests remain. The current
fixtures and historical rejection fixture bytes were hash-checked against the
pre-cleanup snapshot; none changed. Documentation links distinguish dated
milestones from the current-only contract and explain deliberate case review.

Full ordinary/race suites, build and vet passed in typography and tytui. Focused
replay checks also passed. No upstream implementation/resource code changed during
this step; no additional khipu or css-box-layout suite run was needed.
