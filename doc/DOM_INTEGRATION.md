# HTML and DOM integration

`htmlinput.LoadFile` and `Parse` build a styled DOM through w3c-dom's public
CSSOM APIs. The TUI's F4 explorer loads files through Bubble Tea commands, keeps
the previous document on failure, and ignores completion after the filename has
changed. The DOM is read-only after loading. Source selection never mutates it.

## Extraction and coordinates

`Document.Extract` accepts supported paragraph-like elements and text nodes.
Descendant text is collected in document order and processed using the step 5
[CSS extraction policy](CSS_EXTRACTION.md). No Unicode normalization is applied. HTML parsing decodes entities; comments are absent from
the styled DOM. Nested blocks, br/hr, images and other unsupported elements,
and hidden content fail before replacing the
paragraph input. The UI also refuses a selection if the text editor would alter
its bytes. CSS whitespace, resolved styles, and bidi isolates are retained by extraction.

The default inline subset is a, abbr, b, bdi, cite, code, del, dfn, em, i, ins, kbd,
mark, q, s, samp, small, span, strong, sub, sup, time, u, and var. Additional roots
are p, div, li, dt, dd, h1–h6, blockquote, figcaption, and pre. A pre element is
eligible only when its effective white-space is explicitly normal; preserving
whitespace modes remain rejected. Resolved non-inline
display on descendants is rejected, including inherited block and inline-block. Hidden ancestors
are checked even when selecting a descendant directly. The extraction allowlist remains an explicit application policy; upstream now
provides corrected HTML display defaults.

An `Extraction` contains the original file name, SHA-256 of the original HTML,
selected node ID/path, decoded text, and `Span` records. Each span maps a half-open
paragraph UTF-8 byte interval to a half-open decoded-text byte interval in a DOM
text node. Node IDs are preorder indexes in that document; paths count siblings
in the constructed styled DOM, including text nodes. They are locators, not XPath
expressions. HTML entities and omitted markup prevent interpreting these offsets
as original-file byte positions.

`pipeline.Input.Source` optionally carries this provenance. The pipeline validates
its text/coverage and clones the spans before retaining it in a result. Width and
breaker changes retain the map. Text edits detach it, while completed history
and an export already in flight retain their original results.

## CSS inspection boundary

Embedded stylesheet extraction delegates to
`douceuradapter.ExtractStyleElementsWithError`. It scans recursively in tree order,
including nested and empty style elements. Any returned error rejects the entire
load before styling; partial successful sheets are never published. The adapter
retains the upstream node-specific error through wrapping. Tytui validates all
extracted rules and registers them as author sheets. Linked resources are not
loaded. Unsupported declarations, at-rules, and invalid selectors remain rejected;
inline validation remains, while duplicate declarations and !important are delegated
to w3c-dom’s cascade. Importance, specificity, and source order determine winners;
quoted exclamation marks are ordinary value content. Validation appends a CSS
terminator to inline input to avoid douceur truncating a final multi-token value;
the original attribute is retained for upstream styling.

Property inspection delegates to `css.GetProperty`, including inherited font
properties and explicit inherit/initial/unset. Text nodes inspect their parent;
invalid IDs and absent properties return errors. Font and unicode-bidi defaults
now come from upstream metadata rather than application registrations. This is
property resolution, not computed layout: relative font sizes, paragraph policy,
and source maps remain in tytui. Upstream's color `default` sentinel remains
visible in inspection; extraction retains its own supported color policy.
Attributes remain visible alongside them. F5 provides real font selection and style mappings; glyph SVG proofs render supported colors. Resolved styles,
whitespace processing, and byte spans are described in CSS_EXTRACTION.md.

## Upstream integration findings

The sibling w3c-dom checkout needs the focused change in
`style/cssom/cssom.go` and its regression `local_declaration_test.go`:

- Explicit declarations equal to ancestor defaults were dropped. For example,
  span with display:block incorrectly fell back to its inline UA display when
  inspected, allowing an invalid subtree to be selected.
- CSSOM now creates a local property group linked to the ancestor and preserves
  the explicit declaration. This also prevents reuse of a shared group for later
  local declarations. The regression checks local display/visibility and unchanged
  ancestor defaults. Tytui covers both inline and stylesheet display declarations.

The previous unsafe ancestor lookup and empty-style extraction findings are
resolved upstream, as are font metadata and HTML display defaults. Commits
`ff87854` and `e9ab7fa` also correct CSS absolute units. Extraction now delegates absolute parsing to
`dimen.ParseAbsolute` and descendant display checks to upstream property resolution. Tytui still
owns computed font-size resolution and HTML directionality. The earlier specificity,
duplicate-declaration and priority fixes are now used by the adapter. Cords TextBuilder defects are fixed (TODO.md).

## Verification

The HTML fixture in `testdata/html/paragraphs.html` exercises valid selections,
embedded styles, nested blocks, and mandatory breaks. Tests cover Unicode and
entities, exact source-map coverage, stable locators, inherited properties,
empty documents/style elements, explicit display values, unsupported CSS,
selection through frozen layout, detached provenance, navigation, scrolling,
file-load failure/staleness, and small terminal layouts.

Migration regressions in `htmlinput/upstream_dom_test.go` check non-inherited
unset, font inheritance/initial/defaults, text-node inspection, absent properties,
nested stylesheet source order, empty styles, preserved extraction maps, and
rejection of partial stylesheet extraction. Existing pipeline tests verify the
resulting styles still reach frozen layout.

`htmlinput/cascade_test.go` covers duplicate/important precedence, inline versus
stylesheet priority, shorthand conflicts, and quoted punctuation.
`pipeline/priority_test.go` verifies winning inherited sizes and colors through
freezing and glyph proofs with both breakers. Unsupported properties, selectors,
and at-rules remain rejected.

`htmlinput/units_display_test.go` checks absolute conversions, invalid sizes,
resolved display keywords, and retained element eligibility. Existing relative-size
tests remain in force. `pipeline/css_units_test.go` verifies equivalent absolute
units followed by a relative child size produce identical frozen glyph proofs
under both breakers.
