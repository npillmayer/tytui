# tycli

A CLI companion to the paragraph/document TUI. It runs the implemented document
pipeline: styled HTML → normalized CSS boxes → font itemization → shaping and
khipu preparation → intrinsic measurement and CSS flow → frozen paragraphs.
The SVG contains colored CSS box outlines and light-gray text-fragment outlines. Fonts still
matter: their shaped advances and line metrics determine the box geometry.

From the tytui module root:

```sh
go run ./tycli documentlayout/testdata/nested.html \
  --root root --fonts tycli/examples/fonts.yaml \
  --width 420 --breaker firstfit --output /tmp/boxes.svg --log trace

go build -o /tmp/tycli ./tycli
/tmp/tycli --help
```

All existing sibling checkouts are required, as for tytui. The example mapping
uses licensed fixtures already in the sibling OpenType repository.

- `--css FILE`: additional author stylesheet, after embedded stylesheets.
  Inline declarations keep their normal precedence. Linked CSS is not fetched.
- `--fonts FILE`: required YAML mapping. Local fonts only; no discovery/downloads.
- `--root ID`: select an element by its HTML ID (without `#`); defaults to `body`.
  The selected element must satisfy documentlayout's block-root profile:
  `display:block` or `display:flow-root`, including their normalized multi-keyword
  forms `block flow` and `block flow-root`.
- `--width N`: containing width in CSS pixels (default 800; zero is allowed).
- `--breaker firstfit|knuth-plass`: default firstfit.
- `--output FILE` / `-o FILE`: default `boxes.svg`; `-` writes SVG to stdout.
  Files are replaced only after successful layout and SVG serialization.
- `--log quiet|info|trace`: stderr verbosity, default info. Quiet suppresses
  progress and warnings, but failures still report an error. Info reports stages
  and group line/knot counts. Trace additionally reports CSS flow, group
  preparation, shaping calls, and retained line/knot records. Knot widths and
  source ranges use native DU/byte units; containing widths use CSS pixels.
  Trace is an integration-stage report, not an internal OpenType lookup debugger.

Font mapping example:

```yaml
default: fonts/regular.ttf
fallbacks:
  - fonts/hebrew.ttf
rules:
  - family: serif
    weight: 700
    slant: normal
    path: fonts/bold.ttf
  - slant: italic
    path: fonts/italic.ttf
```

Relative font paths resolve against the YAML directory. Rules are ordered,
first-match, using CSS family, weight and slant. Omitted match fields are
wildcards. Unmatched styles use `default`; `fallbacks` provide missing coverage.
CSS determines font sizes. Unknown YAML fields, duplicate keys and multiple YAML
documents are rejected.

Box colors distinguish ordinary blocks (indigo), block flow-roots (teal), and
inline-blocks, including replaced leaves (orange). Anonymous block wrappers use
indigo; a supplied ordinary block root stays indigo despite root containment.
Solid rectangles mark border boxes; dashed rectangles mark content boxes, using
the same color. Flow-root outlines are three times thicker (1.5 big points versus
0.5 for other boxes). Each pair also has a `data-kind` attribute.

Thin light-gray rectangles mark frozen text fragments, with their final advance
width and containing line's height. They follow line breaks and visual bidi order,
including text inside inline-blocks. These are diagnostic fragment extents, not
glyph ink bounds or exact CSS inline-element boxes; spaces and atomic boxes do
not get text outlines. No shaping or layout is repeated during export.

SVG coordinates are big points (CSS px × 0.75), with downward Y. Each
pair carries `data-box` and `data-parent` attributes; parent IDs identify the
content-coordinate reference used to compose absolute positions. These are the
CSS engine's retained geometry entries, not rectangles for every DOM node.
No glyph paths, text labels, images, baselines or CSS painting are emitted.

The supported CSS/layout subset and explicit failure policies are those of
[documentlayout](../documentlayout/README.md). Unsupported inputs and font errors
fail without synthetic fallback. Ctrl+C cancels through the existing pipeline's
cancellation boundaries. Processing errors exit with status 1. Commando 1.0.4
itself prints CLI syntax errors to stdout and exits 0; this upstream parser
behavior also applies to tycli (help/version exit 0 normally).
