# Deferred proposal: paragraph annotation API contract

**Status: deferred, 2026-11-11. Not an implementation contract or extraction
prerequisite.** No APIs in this document have been implemented.

The selected approach is now the simpler
[rendering bypass](type-integr.md#style-projections-and-the-rendering-bypass):
only typography styles enter the paragraph typesetter. Rendering properties stay
on the input/application side and reach the renderer separately, associated with
glyphs through existing source correspondence. See
[the current extraction follow-up](TYPOGRAPHY_EXTRACTION_PLAN.md#follow-up--simplify-rendering-property-wiring).

The remainder is retained as an inactive design exploration. Its annotation
package, payload schemas, whole-cluster decoration/hit policies, acceptance criteria,
implementation increments, and replay version 3 are **not adopted or scheduled**.
Revisit them only if concrete consumers demonstrate a need that the simpler bypass
cannot meet. Existing color behavior and version-1/version-2 replay remain unchanged.

## Decisions proposed

Keep complete document styles and independent logical-range annotations. Font
selection, shaping, and layout consume explicit projections; rendering and
interaction consume retained annotations and frozen correspondence. A difference
in annotations alone must not select another font, split shaping, or change breaks.

Use an immutable annotation set with ordered entries, stable entry identities,
versioned value kinds, and half-open UTF-8 byte ranges. Support overlapping and
discontinuous annotations. Do not require new resolver fields for every annotation.
The initial consumers are foreground color, underline, links, and hover actions;
this is not a CSS cascade, event system, or live editor API.

For decoration and hit regions, propose whole-cluster coverage on any nonempty
logical overlap. This is deliberately coarse when a range covers only part of a
ligature. Preserve the exact logical range and report that coarseness. Color keeps
its existing anchor-based whole-glyph rule; these are separate consumer policies.

## Ownership and dependencies

| Owner | Contract |
| --- | --- |
| X, currently `htmlinput` | Resolve supported document semantics, normalize text, construct complete styles, annotations, and extraction source maps. Resolve CSS cascade before publishing effective values. |
| `typography/style` | Font requests and shaping/layout value vocabulary. Full style equality remains independent of resolved shaping equivalence. |
| Proposed `typography/annotation` | Immutable ranges, annotation values/sets, and neutral extraction source correspondence. Initially standard-library dependencies only; no DOM, cords, khipu, UI, or concrete font types. |
| M, `typography` | Resolve projected font requests and shaping inputs through existing contracts. Does not interpret annotation payloads. |
| P, currently `pipeline` | Own the paragraph envelope and retain original content, annotations, source maps, and frozen output together. Apply paragraph layout settings separately from shaping keys. |
| `khipu` and shaping/hyphenation producers | Own glyph/cluster ranges and material provenance. Any new general replacement-to-source information belongs with its producer and must survive freezing. No annotation dependency is added to khipu. |
| `typography/render` and future interaction consumer | Adapt frozen material to output correspondence, decoration geometry, and hit candidates. Do not inspect DOM nodes or reshape. |
| Host application | Bind link/action identities to behavior, select interaction policy, and schedule layout again if an interaction changes typography or geometry. |

One new small package is proposed, not a new Go module. The initial envelope stays
with P until the paragraph engine is extracted. Original HTML and node locators
remain optional application provenance, outside M and the annotation core.

## Logical ranges and immutable values

The following Go declarations describe proposed public signatures. Constructors
validate and copy inputs; private fields in `Set` and `SourceMap` prevent mutation.
Exact exported names may change before implementation; the semantics below should
be treated as the acceptance contract.

```go
// Package annotation.
type ByteRange struct { Start, End uint64 }
type TextID string // SHA-256 of the exact processed UTF-8 paragraph bytes.
type ID string     // Nonempty, unique within one annotation set.
type Kind string   // Namespaced, versioned schema, e.g. "typography/foreground/v1".

type Value struct {
    Kind    Kind
    Payload string // UTF-8 JSON object; inert data, never executable behavior.
}
type Entry struct {
    ID      ID
    Ranges  []ByteRange
    Value   Value
    Origins []SourceRange // Optional exact pre-normalization annotated ranges.
}
type Set struct { /* private immutable storage */ }

func NewSet(text string, entries []Entry) (Set, error)
func (s Set) TextID() TextID
func (s Set) Entries() []Entry
func (s Set) At(byteOffset uint64) ([]Entry, error)
func (s Set) Overlapping(ByteRange) ([]Entry, error)
```

- Ranges address the processed paragraph, never raw HTML, UTF-16 positions, scalar
  indexes, or glyph indexes. Both ends must be valid UTF-8 scalar boundaries and
  satisfy `0 <= Start <= End <= len(text)`. Grapheme boundaries are not required.
  The text must be valid UTF-8. Paragraph boundaries remain explicit.
- Each entry has at least one range. Nonempty ranges within an entry are sorted,
  disjoint, and canonicalized by joining adjacency. Different entries may overlap
  freely. One ID can describe a discontinuous link or decoration without creating
  multiple logical annotations.
- Empty `[p,p)` ranges are allowed as retained locations for annotations whose
  text was omitted by normalization. They have no paint, decoration, hit area, or
  shaping effect. They are not caret instructions or zero-width bidi markup.
  Deduplicate equal empty ranges; retain their provenance separately even if an
  adjacent nonempty range of the same entry exists.
- `At(p)` selects nonempty ranges containing the scalar beginning at `p`.
  At paragraph end it returns no entries. `Overlapping([a,b))` uses nonempty
  intersection; an empty query returns no entries. Invalid bounds or a split
  UTF-8 scalar are errors. `Entries` still exposes retained empty ranges.
- Entry order is significant and preserved across queries and replay. IDs identify
  entries; their lexical order has no priority meaning. Duplicate IDs are errors.
  Constructors copy nested ranges; accessors return detached values. An immutable
  set is safe for concurrent reads and remains valid after caller input changes.
- Optional `Origins` preserves the exact originally annotated source ranges, not
  just every contributor to a collapsed output space. `NewSet` copies these values
  and validates range ordering/bounds structurally; P validates source identities
  and UTF-8 boundaries against the source catalog. Missing origins mean no such
  extraction evidence was supplied, not that the annotation has no logical range.
- A set is bound to exact text bytes by `TextID`. P rejects a mismatched text/set
  pair. TextID does not identify styles, a document, or a layout result. Repeated
  text in different source nodes can share TextID but has distinct source identity.
  Nil/absent annotations mean an empty set bound to the input text; do not infer
  coordinate validity from a zero-valued set.

Use typed helpers for known kinds rather than JSON at every call site. Initial
schemas contain a resolved foreground color, a supported underline specification,
a link target/identity, or a hover action identity. Helpers reject unsupported
values; they do not accept CSS expressions needing a DOM or layout context.
Underline may request `currentColor`, resolved by the rendering consumer against
retained foreground annotations, without involving M.

Validate JSON syntax, unique object keys, and each known schema. Payload strings
and unknown kinds round-trip unchanged; JSON spelling is not shaping equivalence
or annotation identity. Unknown kinds are retained as opaque metadata and reported
as unsupported by a consumer asked to interpret them. They are never treated as
font or layout inputs. This extension mechanism is only for rendering/interaction;
new geometry-affecting semantics require an explicit shaping/layout API extension.
No callback, parsed font, DOM pointer, or service instance enters a payload.

## Overlap and consumer policies

The set is storage and query machinery, not an inheritance or cascade engine.
X publishes effective CSS values; non-DOM callers provide equivalent values.

| Consumer | Proposed initial policy |
| --- | --- |
| Foreground | At each retained glyph's original logical anchor, the last matching foreground entry wins. With none, use the explicit renderer default. No outline splitting; repeated anchors share color. Independently anchored marks may differ. |
| Underline | Each entry selects all output clusters whose logical coverage overlaps its ranges. Draw over selected cluster advance intervals on each line, using line/font metrics. Deduplicate clusters within an entry; distinct decorations remain distinct. No partial-ligature clipping or invented caret positions. |
| Link/hover | Select the same whole-cluster coverage, plus eligible source-bearing spacing boxes. Return every matching entry and logical range in retained order; the host decides activation priority. Do not silently choose a link by glyph color or ID order. |

Geometry is computed from placed output, not from raw logical range lengths. Use
advance intervals and line ascent/descent for initial hit rectangles; zero-advance
marks share their containing cluster's region. If an independently anchored mark
forms a zero-advance cluster of its own, use the positioned clusters in its logical
grapheme as the coarse region; do not manufacture a character advance. This grouping
uses retained text boundaries and geometry, not reshaping. Return separate regions per line
and visual fragment, never one rectangle bridging unrelated bidi content. Exclude
side skips, paragraph fill, and other source-free layout material. Source-bearing
spaces can be decorated/hit; trimmed or silent material has no fabricated region.
Merge adjacent regions only when the annotation, line, and consumer policy permit.

These choices intentionally allow an annotation on only `i` in `fi` to select the
whole ligature for underline/hit testing, while the glyph's color still comes from
its `f` anchor. Hit results retain the original `i` range and a cluster-coverage
flag; they do not claim an exact character position. An exact caret/partial-cluster
selection API, ink-aware decoration skipping, and more sophisticated hit testing
are later additions. They must not silently alter this policy or split shaping.

## Style projections and paragraph preparation

The envelope has one canonical processed text and keeps original semantics
separate from the text projected for khipu. Schematically, in P:

```go
type ParagraphContent struct {
    Text        styled.Text       // Complete supported document styles.
    Annotations annotation.Set
    Sources     annotation.SourceMap
    // Existing base direction, bidi boundaries, paragraph/layout settings.
}
```

The constructor verifies that every component addresses `Text.Raw()` and snapshots
mutable input. No preprocessing stage may substitute normalized text while keeping
stale annotation/style coordinates. Plain non-DOM clients may supply an identity
source map. M receives the relevant style projection, not this entire envelope.

| Projection | Inputs | Equality / boundary rule |
| --- | --- | --- |
| Font request | Ordered families, weight, slant, configured selection/fallback policy | Select an opaque face through M. Equal requests need not imply equal fallback results; different requests may resolve to the same face. |
| Shaping | Resolved face, size, script, language, features with explicit domain, spacing and bidi context | Use existing resolved equivalence and context checks. Equivalent resolved typography can coalesce despite document/annotation boundaries. |
| Paragraph layout | Width, breaking/alignment, line height, paragraph direction and other supported geometry settings | P consumes these at their appropriate scope; not every geometry difference is a font or shaping boundary. |
| Rendering/interaction | The independent annotation set and source correspondence | Never part of font matching or shaping-run equality. Consumers interpret it after freezing. |

Keep full document-style equality for persistent storage. Do not redefine
`Style.Equals` to mean font equality or add arbitrary annotation fields to
`ShapingKey`. Nor should a CSS property be classified solely by its name: a hover
action changing weight needs a new typography/layout operation, while hover
sensitivity alone does not. Direction/isolation stays in bidi analysis rather than
being added to khipu's `StyleResolver` return value.

Feature ranges keep explicit scalar origins and are clipped/rebased only under
the existing contract. Until absolute feature-domain composition is implemented,
retain the current conservative separation of feature-bearing spans. Unsupported
actual typography boundaries inside graphemes remain errors; annotation boundaries
inside those same graphemes are valid.

During migration, convert legacy `Style.Color`/`PaintSpan` into foreground entries
once at the input adapter. Reject simultaneously supplied legacy color and explicit
foreground entries until an explicit override mode exists; avoid two authorities.
Preserve legacy style values for inspection/replay. New preparation uses only the
canonical annotation set for rendering. Older replay modes retain their existing
style-boundary or glyph-boundary implementations unchanged.

## Extraction source correspondence

Separate the exact logical annotation range from its origin in input content.
Source maps are relations, not a single invertible offset function:

```go
// Package annotation. IDs refer to immutable source snapshots in a catalog.
type SourceID string
type SourceRange struct {
    Source SourceID
    Bytes  ByteRange
}
type MappingKind string // "copy", "collapse", "omit", "generated"
type SourceSegment struct {
    Text    ByteRange     // Processed paragraph bytes.
    Origins []SourceRange // Ordered contributions; may span several text nodes.
    Kind    MappingKind
}
type SourceMap struct { /* private immutable segments and source catalog */ }

type SourceSnapshot struct {
    ID     SourceID
    Domain string // e.g. "plain-input-utf8" or "dom-decoded-text-utf8".
    Text   string // Immutable bytes used for validation and retained provenance.
}

func NewSourceMap(text string, sources []SourceSnapshot, segments []SourceSegment) (SourceMap, error)
func (m SourceMap) TextID() TextID
func (m SourceMap) Sources() []SourceSnapshot
func (m SourceMap) Segments() []SourceSegment
func (m SourceMap) Origins(ByteRange) ([]SourceSegment, error)
```

Its constructor receives the processed text, source catalog, and segments; it
validates UTF-8 boundaries against the supplied immutable source text snapshots.
The catalog gives each source a stable snapshot identity, byte length, content
digest, and coordinate-domain tag. DOM text uses decoded text-node bytes, with a
document-snapshot/node locator in application metadata. Plain input uses original
input bytes. Neither is silently relabeled as raw HTML byte offsets. Raw HTML
locations need a separate parser map if later supported.

- `copy`: one equal-length origin with identical bytes, permitting exact offset
  translation and clipping of subrange queries.
- `collapse`: nonempty output with one or more source contributions; preserve all
  contributing whitespace, including across nodes. Any nonempty overlap queries
  the whole relation rather than pretending it is one-to-one.
- `omit`: empty output with nonempty origins, retaining trimmed whitespace or other
  explicitly removed input. Enumerate it through `Segments`; it has no visual extent.
- `generated`: nonempty output with explicit cause origins, or no origin when
  truly source-free. Never claim an identity mapping for inserted text.

Nonempty target segments partition processed text without gaps/overlap; combine
all contributors to a collapsed space in one segment. Empty omission segments can
occur at boundaries. Source ranges can be disjoint; they are not assumed to cover
every byte of a source document. Synthetic bidi boundaries remain separate metadata
and do not introduce fake text bytes. `Origins` returns clipped copy relations and
whole noninvertible relations for nonempty overlaps; it does not include omissions
merely because they share a boundary. All returned slices are detached.

Annotations enter P in processed coordinates. If X projects pre-normalization
ranges, copies translate exactly; any overlap with collapsed whitespace selects
the resulting space; wholly omitted portions become empty ranges at the omission
position. Preserve exact pre-normalization ranges in `Entry.Origins` and all
contributing source ranges in the source map. Distinct annotations may
consequently overlap a collapsed space; the consumer rules above settle rendering,
without discarding source evidence. This
projection policy must be recorded, not silently inferred during replay.

## Frozen output correspondence

Retain two independent relations with each result:

1. Processed paragraph bytes to original source snapshots, as above.
2. Output material to processed paragraph bytes, including selected replacements.

Consumers compose these relations. Layout order must never rewrite logical ranges.
Use `(line, knot, glyph index in CopyText)` as result-local glyph references, or
equivalent opaque handles. They are scoped to one frozen result, not stable across
reflow. VisualOrder only changes traversal of a complete owned fragment; it does
not change these identities or the original annotation order.

The proposed retained output adapter exposes detached records of this shape:

```go
// In typography/render, not annotation or khipu/contracts.
type GlyphRef struct { Knot, Index int } // Scoped to the retained result.
type Coverage struct {
    Ranges []annotation.ByteRange
    Basis  string // "cluster", "producer", "source-free", "unavailable"
}
type GlyphAssociation struct {
    Glyph    GlyphRef
    Anchor   *uint64 // Processed-byte anchor for color; nil if unmapped.
    Coverage Coverage
}
```

Associate glyphs and source-bearing nonglyph material before physical traversal;
placed geometry can then attach line/box positions to those references. Both
coverage and anchors are needed: the color anchor cannot describe a full link
range, and cluster coverage cannot decide a glyph's color. Coverage is a set of
logical ranges, not necessarily one contiguous interval. Unknown is distinct from
known source-free material; consumers return explicit unavailable-correspondence
diagnostics rather than inventing ownership. Color fails if its required source
anchor is unavailable; source-free glyphs use the explicit renderer default.

The existing `FrozenCluster.Input` and glyph intervals provide ordinary cluster
coverage. Khipu derives some cluster ends from the next anchor/run end; omitted
scalars may be included. This is a coarse cluster association, not proof of exact
per-character glyph contribution. Leading silent source and completely silent
runs may have no glyph association. Preserve that distinction in hit/decor results;
do not invent glyphs or caret positions to fill gaps. Repeated anchors map to the
same source anchor and do not imply multiple logical characters.

Replacement-local coordinates must first pass through an explicit replacement map
to processed paragraph bytes. That map records copied, removed, changed, and
inserted material and any anchor attribution. For the supported explicit soft
hyphen, generated `-` takes the selected marker's anchor and coverage; surrounding
replacement text maps to its own source. An unbroken marker retains provenance
without generating a standalone visible region. A synthetic insertion with a
source cause must declare its anchor policy or explicitly declare no anchor.

Current `render.PaintSources` reconstructs only the supported soft-hyphen producer
and validates replacement text; it is not a general replacement map. A first
implementation can adapt that supported case and reject unsupported replacements.
General changed-spelling hyphenation requires producer-supplied maps retained by
khipu freezing, with owning-package tests. Do not infer them from matching glyph
IDs, adjacent knots, or replacement-local anchors alone.

## Persistence, errors, and incremental implementation

New annotation-aware saves should introduce replay version 3 with explicit
annotation schema, normalization/projection policy, color policy, and
decoration/hit-region policy identifiers. Preserve set order, IDs, exact ranges,
opaque payloads, source identities, and mapping kinds. Font resource identities
remain unchanged. Validate decoded data through the same constructors before
publishing a replay result. Unknown data kinds can be retained; unknown required
layout/consumer policies prevent reproducing that result and must report an error.

Version 1 keeps legacy style-boundary shaping; version 2 keeps current glyph-color
semantics. Loading or resaving either must not add new annotation behavior or
upgrade it implicitly. Explicit conversion to the new format is a separate action.
Retain stable action identities as inert data; replay does not dispatch actions or
fetch link targets. JSON field names and canonical evidence encoding are to be
specified with the version-3 implementation, not derived from Go struct JSON by
accident. No wire-format or resource-ID change is made by this proposal.

Invalid ranges, duplicate IDs, malformed known values, text/snapshot mismatches,
and invalid source relations fail construction with entry/segment context. A
frozen result is published only when its required correspondence is valid. Optional
unsupported annotation consumers can report diagnostics while preserving data;
strict proof/replay verification must not report an unrendered required annotation
as reproduced. Keep diagnostics structured and separate from user-facing text.

Implement in reviewable increments:

1. Add immutable ranges/values/sets and source-map validation in typography, with
   typed foreground helpers and round-trip tests. Keep the existing runtime path.
2. Add P's retained annotation envelope and legacy color conversion; preserve
   current glyph output and replay modes. Test font/shaping projection invariance.
3. Add output correspondence for ordinary clusters, source-bearing spacing, and
   the supported discretionary producer. Report coarse/unavailable coverage
   explicitly. Add upstream replacement maps only when a producer requires them.
4. Add underline and link/hover candidate consumers using the proposed policies;
   keep action dispatch in the host. Add version-3 persistence and fixed fixtures
   before exposing saved annotation cases as reproducible.

Acceptance cases must include scalar-aligned boundaries inside graphemes, invalid
UTF-8 splits, empty paragraphs/ranges, overlapping and discontinuous entries,
detached ownership, unknown-kind round trips, and input mutation after publication.
For both breakers, compare annotation-only variants for identical selected fonts,
glyphs, positions, and breaks. Exercise `fi`, independently/shared anchored marks,
pointed Hebrew, mixed LTR/RTL, multi-line ranges, source-bearing spaces, normalization
across DOM nodes, omitted text, selected/unselected soft hyphens, and unsupported
replacement diagnostics. Use fixed expected correspondence/regions, not merely
comparisons of two paths through the same implementation. Preserve existing
version-1/version-2 replay fixtures and font identity tests unchanged.

Validation of this proposal: checked current shared styles/projections,
`paragraphtext.Normalize`, HTML extraction spans, frozen glyph/cluster coordinate
contracts, and `render.PaintSources`. No production code or test expectations
were changed; this document is the proposed implementation contract, not a claim
that general annotation consumers or replay version 3 already exist.
