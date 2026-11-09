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

The default inline subset is a, abbr, b, cite, code, del, dfn, em, i, ins, kbd,
mark, q, s, samp, small, span, strong, sub, sup, time, u, and var. Additional roots
are p, div, li, dt, dd, h1–h6, blockquote, figcaption, and pre. Explicit non-inline
display on descendants is rejected, including inline-block. Hidden ancestors
are checked even when selecting a descendant directly. Default tag policy is
explicit because the library's UA fallback calls several standard inline tags
block elements.

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

Embedded stylesheets and style attributes are processed by w3c-dom. The loader
extracts style text itself, including empty and nested style elements, avoiding
the convenience extractor's unchecked child accesses. Linked resources are not
loaded. Unsupported declarations, at-rules, and invalid selectors are rejected
before worker execution. Supported declarations are those backed by library
defaults, its recognized shorthands, and the additional font inspection defaults
(serif, medium size, normal weight/style, normal line height).

Property inspection walks public local property maps and parents with nil checks;
it does not call the unsafe inherited-property convenience traversal. Font
properties are treated as inherited in this inspection subset. These values are
diagnostics of the draft library and application defaults, not a claim of browser
CSS conformance. Attributes remain visible alongside them. Real font selection and color rendering remain future work. Resolved styles,
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

Other observed limitations are contained at the adapter boundary rather than
silently treated as correct CSS: missing font defaults caused a panic in the old
property-forking path (the local-group fix also removes that dependency),
inherited-property traversal dereferences a nil parent at the root, and
the convenience style-element extractor assumes nonempty nodes. Broader cascade,
specificity, shorthand, and inline-style parsing correctness remains upstream
work beyond the supported subset; step 5 fixes duplicate-property truncation and
inline priority (see CSS_EXTRACTION.md). The cords builder investigations remain separate.

## Verification

The HTML fixture in `testdata/html/paragraphs.html` exercises valid selections,
embedded styles, nested blocks, and mandatory breaks. Tests cover Unicode and
entities, exact source-map coverage, stable locators, inherited properties,
empty documents/style elements, explicit display values, unsupported CSS,
selection through frozen layout, detached provenance, navigation, scrolling,
file-load failure/staleness, and small terminal layouts.
