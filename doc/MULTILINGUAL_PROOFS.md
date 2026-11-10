# Multilingual paragraphs and frozen glyph proofs (step 7)

## Workbench workflow

In F5, choose a default Latin font and add ordered fallback rows to the mapping
editor, for example:

```
fallback | * | * | ../opentype/testdata/fonts/NotoSansHebrew-Regular.ttf
```

Existing family/weight/slant mapping rows still select the primary face. Fallback
rows are a separate ordered list and require wildcard weight/slant. Ctrl+S applies
the font configuration. Select an HTML paragraph with F4, then Ctrl+R lays it out.
Ctrl+S in the workbench now exports glyph outlines for OpenType results and the
existing structural boxes for synthetic results. Saved layouts export their own
retained fonts and glyphs, even after the configuration or files change.

`pipeline.WriteStructuralSVG` explicitly preserves the structural diagnostic
export for either backend. `WriteGlyphSVG` requires OpenType, and `WriteSVG`
chooses based on the frozen result's backend. `BuildGlyphProof` exposes positioned
glyphs and line geometry for independent inspection without writing SVG.

## Itemization and fallback policies

Text is itemized before bidi paragraph construction, retaining its original UTF-8
bytes and the DOM source map. Extended grapheme clusters are never split by a
font or script boundary. CSS style boundaries inside a grapheme are rejected.
Within each original CSS span, Latin and Hebrew characters identify script;
Common/Inherited graphemes adopt the preceding strong script, or the following
one at the span's beginning. A span with no strong script uses Latin. Other
strong scripts fail explicitly. This is a deliberately limited script policy,
not complete Unicode Script_Extensions or paired-bracket script resolution.

Script comes from text; language remains the HTML lang value or the plain-text
English default. `otcore` and `othebrew` are registered explicitly. Paragraph
base direction still comes from HTML dir/CSS direction or the headless input;
inline isolates remain zero-byte bidi markup. LTR and RTL bases are both tested.

For each grapheme, try the mapped primary face followed by configured fallbacks.
Choose the first face covering every required scalar, including combining marks.
Soft hyphens defer face selection: they inherit the preceding face within the
original CSS span, or the following face at the span's beginning. Marker-only
spans retain their mapped primary face. This keeps a fallback word contiguous
without overriding real script/font transitions or explicit style boundaries.
The same face shapes the entire grapheme; adjacent equal script/face/style items
are merged to retain ligature and kerning context. A switch between graphemes may
still break cross-grapheme ligatures; no whole-word or language-specific fallback
heuristic is implied. No system font discovery, implicit fallback, or synthetic
substitution occurs. Cmap errors remain errors. Control/format characters and
variation selectors remain exempt from nominal coverage preflight; shaping must
still succeed. Normalization-aware coverage remains deferred. Diagnostics include
processed byte ranges, chosen script, font identity, and size.

Explicit U+00AD soft hyphens supply a `-` pre-break replacement through khipu's
hyphenation service. Both breakers use the owning discretionary provider. There
is no dictionary hyphenation. Replacement text is shaped in its retained run
context; a missing replacement glyph fails explicitly. Frozen source provenance
continues to refer to original text, while replacement cluster anchors are local
to replacement text. Proofs use the retained replacement glyphs directly.

## Positioned SVG renderer

The renderer uses public khipu frozen snapshots and x/image/font/sfnt for unhinted
outlines from the face's retained bytes. It never calls shaping or line breaking
and never reloads the file path. There are no embedded font binaries, external
resources, SVG text elements, scripts, or browser font substitutions.

Frozen knots are already physical left-to-right. Their final widths, including
adjusted glue, advance the knot pen. Within each RTL knot, logical glyph records
are visited in reverse visual order, matching the OpenType proof convention and
independent Hebrew captures (including marks sharing a cluster). LTR records stay
in logical order. Glyph advances and offsets are converted once using the retained
font scale. SVG's downward Y axis negates font Y advances/offsets; sfnt outlines
already use downward Y coordinates. Knot order is never reversed again.

Each line's baseline uses the largest scaled ascent; its descent and spacing use
the largest descent and max(font line gap, 20% em). Empty lines use the configured
size. These are explicit proof-layout choices, not CSS line-height layout.
Source styles supply fill colors: CSS named colors, transparent, and hex colors
are supported; other values return errors. Soft-hyphen replacements use the source
knot's style. Structural overlays show target line rectangles and actual-width
baselines. The viewBox includes metric extents and all outline/control-point
bounds, with margins to preserve overhangs. A white background makes proof colors
consistent. Unsupported outline formats or glyph loads return errors.

## Verification and reproducibility

`pipeline/multilingual_test.go` compares glyph IDs, scalar anchors, and positioned
origins against the sibling OpenType project's HarfBuzz 15 captures for Latin
ligatures, decomposed accents, Hebrew letters and pointed Hebrew. It exercises
mixed LTR/RTL bases, both breakers, explicit fallback, colors, and discretionary
replacement proofs. Adapter tests check atomic grapheme fallback and explicit
failure without configured coverage. Existing stream cancellation/race and frozen
history tests remain in place.

The font identities and licenses from OPENTYPE_INTEGRATION.md still apply. Hebrew
uses the sibling's unmodified `NotoSansHebrew-Regular.ttf`, SHA-256
`a7fa16fffb27bedb060a0866267c29e9859aeb9c21cc33f5b3aaf6eb062eca85`, under SIL OFL 1.1;
its license and source are recorded in that checkout's
`testdata/fonts/NotoSansHebrew-LICENSE.txt` and `NotoSansHebrew-README.md`.
No font binaries were copied and no upstream source changes were needed.

`testdata/proofs/multilingual.svg` is a visually inspected sample: firstfit,
180 bp wide, 24 bp CSS size, Gentium primary and Noto Sans Hebrew fallback.
Its source is `testdata/html/multilingual.html`. The SVG contains per-glyph font
hashes, cluster IDs, knot indexes, and line indexes. These scalar clusters are not
UTF-8 byte offsets; use the retained source map for DOM correspondence.
