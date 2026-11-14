# Retained nested document proofs

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
general intrinsic sizing, pagination or a replay format.


## Nested auto sizing

`nested-auto.html` adds auto inside auto, fixed inside auto, a fixed RTL item and
an empty auto item. It retains the same explicit fonts and supported normal-line,
bottom-margin-edge, zero-side-skip, no-hyphenation profile. Its six atomic
occurrences remain separate from the containing paragraphs. The existing
`nested.html` and `nested-firstfit.svg` are preserved.

| Breaker | Wide: 420 CSS px | Narrow: 100 CSS px |
| --- | --- | --- |
| First-fit | [SVG](nested-auto-firstfit-wide.svg) | [SVG](nested-auto-firstfit-narrow.svg) |
| Knuth–Plass | [SVG](nested-auto-knuth-plass-wide.svg) | [SVG](nested-auto-knuth-plass-narrow.svg) |

Widths are the containing-block widths passed to `Document.Layout`; root edges
reduce its content width. The narrow case changes the innermost auto width.
The wide outputs agree for this fixture, while narrow line choices differ.
Overflow is deliberately visible, including the fixed Hebrew item and the
existing zero-skip Knuth–Plass policy; intrinsic bounds do not guarantee a fit.

Reproduce from the tytui module root with the unchanged generator:

```sh
go run documentlayout/testdata/proof.go -input documentlayout/testdata/nested-auto.html -breaker firstfit -width 420 -out /tmp/nested-auto-firstfit-wide.svg
go run documentlayout/testdata/proof.go -input documentlayout/testdata/nested-auto.html -breaker firstfit -width 100 -out /tmp/nested-auto-firstfit-narrow.svg
go run documentlayout/testdata/proof.go -input documentlayout/testdata/nested-auto.html -breaker knuth-plass -width 420 -out /tmp/nested-auto-knuth-plass-wide.svg
go run documentlayout/testdata/proof.go -input documentlayout/testdata/nested-auto.html -breaker knuth-plass -width 100 -out /tmp/nested-auto-knuth-plass-narrow.svg
```

`auto_proof_test.go` checks structured proofs and byte-identical SVG re-rendering
of both retained sizes after later layouts, failures/cancellation, original DOM
release and accessor mutation. A recovered wide layout reproduces its original
proof. Real-service guards reject loading, coverage queries, stream construction,
shaping, metric queries and scale acquisition while rendering, but allow retained
scale conversion, identity and outlines. The renderer consumes completed frozen
lines and vlists; it has no preparation/breaker configuration. Source assertions
check decoded DOM anchors, paint, ligatures, Hebrew and separate atomic ownership.
Numeric synthetic tests remain the independent geometry oracle. All four SVGs
were XML-validated and visually inspected; they add no CSS painting or replay API.
