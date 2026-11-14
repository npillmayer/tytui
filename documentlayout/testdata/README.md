# Nested fixed-width document proof

`nested.html` exercises text/object/text, nested fixed-width inline-blocks,
anonymous groups around a block, adjacent and object-only material, signed margins,
padding/borders, separate paint and pointed Hebrew. `nested-firstfit.svg` is a
diagnostic generated from retained document geometry and frozen glyphs at 420 CSS px.
Solid blue boxes are borders, dashed gray boxes are content, pale lines are line
boxes/baselines, and dots identify precise paragraph atomic border origins.
The SVG is demonstration output, not the independent correctness oracle.

From the tytui module root:

```sh
go run documentlayout/testdata/proof.go -out /tmp/nested.svg
go run documentlayout/testdata/proof.go -breaker knuth-plass -width 260 -out /tmp/nested-narrow.svg
```

The example explicitly supplies sibling OpenType test fonts: Gentium Plus Regular
for Latin and Noto Sans Hebrew Regular as fallback. No font files are copied here,
embedded in the proof or discovered automatically. Their provenance/licenses remain
in `opentype/testdata/fonts`, including `NotoSansHebrew-README.md` and its OFL license.
Proof glyph records identify the retained font by SHA-256. Suitable font coverage
is the caller's responsibility; this example adds no validation policy.

Tests independently check manually calculated synthetic border origins, line
heights, ancestor offsets, multiline baselines and bidi positions. Real glyph IDs,
clusters and offsets are compared with OpenType's existing HarfBuzz captures for
`latin_office_liga_on` and `hebrew_shalom_pointed`, including font hashes. Tests also
check nested source/paint association and identical proof output after source DOM
removal or subsequent layouts. Existing paragraph replay fixtures remain untouched.

Current policies allow positive overfull lines: narrowing need not produce a fit.
The proof deliberately displays actual overflow. It does not claim full CSS
painting order, internal inline-block baseline alignment, replaced-image layout,
intrinsic sizing, pagination or a replay format.
