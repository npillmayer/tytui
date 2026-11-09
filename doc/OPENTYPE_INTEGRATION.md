# OpenType fonts and shaping (step 6)

## Workbench

Press F5 to open the font panel. Enter a local font file and a plain-text size in
big points (1/72 inch). Tab moves between the path, size, and mapping editor.
Ctrl+S validates all files asynchronously and applies the configuration. Escape
or F5 closes without applying the draft; editing or closing invalidates pending
validation. Failures preserve the previous configuration; full error text is
available in the results pane after Escape. F6 explicitly selects synthetic
shaping. Ctrl+R runs the selected backend; changing backend or mappings cancels
and invalidates the current layout without altering saved results.

Each optional mapping row is:

```
family | weight | slant | /path/to/font.ttf
```

For example:

```
serif | 700 | normal | /fonts/Bold.ttf
serif | * | italic | /fonts/Italic.ttf
* | * | * | /fonts/Regular.ttf
```

Rows match in order. Asterisk means any value. Weight is an exact integer from
1 to 1000; slant is normal, italic, or oblique. Family matches a case-insensitive
name in the extracted CSS family list; simple quoted names are accepted. This
is an explicit mapping policy, not browser font matching or system discovery.
Unmatched styles use the chosen default file. Plain text always uses that file.
CSS sizes override the panel size, which is the plain-text default. No synthetic
bold/italic or automatic glyph fallback is performed. All configured files are
validated, even if a rule is not used in the current paragraph.

The result inspector lists file paths, names, SHA-256 hashes, parser warnings,
and processed UTF-8 byte spans mapped to each face and size. Color remains
metadata. SVG export still draws structural boxes from the exact frozen result;
it does not render glyph outlines or embed fonts.

## Headless use and ownership

`pipeline.Input.Fonts == nil` selects synthetic shaping. Set it explicitly to
request OpenType; a failure never switches backends:

```go
result, err := pipeline.Run(ctx, pipeline.Input{
    Text: "office", Width: 200 * dimen.BP,
    Fonts: &fontbackend.Settings{
        DefaultPath: "../opentype/testdata/fonts/GentiumPlus-R.ttf",
        Size: 12 * dimen.BP,
    },
})
```

For styled HTML, also pass the matching `Source` extraction and configure
`Settings.Rules`. Its empty Family/Slant and zero Weight are wildcards. Input
settings and source slices are cloned in the result. Every run loads private font
bytes and fresh parsed faces; subsequent file changes affect subsequent runs,
not saved glyph snapshots. Font hashes identify the actual bytes used at layout
time, which may differ from an earlier panel validation. Files are local regular
files, capped at 64 MiB. No fonts are fetched or discovered automatically.

`fontbackend.Loader`, `Face`, `Scale`, `Resolver`, and `Stream` implement the khipu
contracts. Metrics preserve signed font units. Scaling caches the factor from
`otquery.FontScale` and retains fractional DU until khipu rounds geometry. The
bounded stream adapts the synchronous public OpenType shaper directly to a
context-aware channel sink; it starts no goroutines. The producer calls Shape,
while a consumer drains Glyphs, then the producer calls Finish. Errors close the
stream and remain available through Err. Cancellation interrupts reads between
calls and blocked sends; concurrent Shape/Finish/SetInitialCluster operations
are rejected while shaping. Glyph cluster anchors remain original Unicode scalar
indexes, including across calls and explicit SetInitialCluster overrides.
Features and buffering options are translated without changing their coordinates.
All glyph advances and offsets are final shaped font units, never nominal metrics
added a second time. Lazy backend access is serialized per face; metrics and
scale queries use cached immutable values.

## Diagnostics and supported scope

Font loading and coverage errors carry pipeline stage `fonts`. The loader uses
ordinary `ot.Parse`, never the relaxed test-font option. Returned parser errors
and accumulated font errors reject the file; warnings remain visible. The current
parser requires both GSUB and GPOS, even for otherwise valid simple fonts.
Malformed lazy cmap queries are distinct from absent glyph mappings.

Coverage is intentionally strict: every non-control/non-format scalar must have a
cmap glyph in the mapped face. Missing coverage reports the codepoint, processed
UTF-8 byte interval, file, and hash. Variation selectors are exempt from this
preflight; the shaper must still succeed. This policy can reject a character
that normalization could decompose into supported glyphs. Automatic fallback and
normalization-aware coverage decisions are deferred. A shaped .notdef glyph also
fails explicitly with its original scalar anchor. No partial frozen result is
published on any failure.

Step 6 registers `otcore` only, with a Latin/English plain-text default. CSS lang
supplies language and inferred script. This is not text-based script itemization;
nonmatching engines or unsupported shaping cases return errors. Dedicated Hebrew
and Arabic engines, font fallback, variable/collection font selection, and glyph
rendering remain later work. Existing OpenType correctness limits still apply.

## Fixtures and verification

No font binaries were copied. Tests use the required sibling OpenType checkout:

- `testdata/fonts/GentiumPlus-R.ttf`, SHA-256
  `fa09788adcc651d77eba7d96f8cc3a600d6484c58e3b4ff79c2b2464dbfea6f8`.
  SIL Open Font License 1.1; the sibling's `ShaperTestLatin-LICENSE.txt` contains
  its embedded license (see `testdata/fonts/NORMALIZATION.md`).
- `testdata/fonts/ShaperTestProgress.ttf`, SHA-256
  `6b4bb3ea60b769f0b385eeed74dc79ab50e61628f458c1a1368548acd59293be`.
  Authored by the OpenType project under its BSD 3-Clause license; provenance and
  generator are in `testdata/fonts/STREAMING.md`.

Frozen `office` glyph IDs, scalar anchors, and advances are checked against the
existing HarfBuzz 15 capture in
`opentype/internal/hbcmp/testdata/text/streaming.json` (`stream_ligatures`, first
word). Progress-font tests check kerning, feature ranges, metrics, and rebasing.
Both breakers, mapped styled runs, detached settings, parser failures, absent
files, missing multibyte glyphs, cancellation under backpressure, stream lifecycle,
panel failure/staleness, backend switching, and small terminal sizes are covered.
No upstream source changes were required for step 6.
