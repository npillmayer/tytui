# From HTML Input to Type

## Components

Module `w3c-dom`:
  - create an HTML/CSS DOM from input source
  - hold CSS property values for every node

Module `opentype` and sub-packages:
  - load OpenType fonts
  - allow access to OT font properties
  - shape input character runs into glyph positioning info

Module `fontfind`:
  - locate a font given some font-properties

Module `cords` and sub-packages:
  - Given a paragraph-node in an HTML/CSS DOM, create styles.Text

Module `uax`:
  - Provide splitting info for various UAX
  - Provide bidi handling (levels and ordering)

Module `hyphenate`:
  - Find breakpoints in words

Module `khipu`:
  - Given a styled.Text, create a khipu from it
  - needs a text shaper
  - has a line-breaker, which needs a hyphenator
  - relies on `uax` for splitting and bidi


## Integration

To wire everything up into something like `tytui`, some additional integration effort is necessary:

### Adapters

  - Adapter for text shaping: otshape -> khipu
  - Adapter for hyphenation: hyphenate -> khipu
  - Translation of CSS properties to styled.Text styles
  - Translation of styles to fonts and font-changes

### From Style to Font-Selection

I see a hole in the overall responsibility chain. Let's focus on paragraph-like nodes in the
HTML/CSS input. Users select CSS styles for runs of characters (spans). These styles may or may not be related to font selection: let's use the `<strong>` tag as being related to font-selection, and a hypothetical `<color>` tag as unrelated. Now we are at the stage of translating the DOM node into a `styled.Text` for the paragraph. Right now, all style changes are transferred to `styled.Text`. In the khipu stage, runs are split at style-change boundaries to satisfy the text-shaping interface semantics. However, only `<strong>` runs should be split, not `<color>` runs, as the latter do influence shaping. (Actually, for color this is debatable. We want color changes between graphemes only, not within.)

We need an instance carrying the responsibilities:

  - translate CSS properties into font-selection operations
  - assemble "font families" (not in the OpenType sense): different fonts / font weights comprising a set for switching between "normal", "italics", "bold" etc.
  - Interact with the construction of styled.Text from DOM nodes to signal font-switches / font size switches  for style-boundaries
  - khipus have to be fed scaled fonts, i.e. "Helvetica 11.0pt". A khipu-textbox is set for a specific scaled font in order to be able to measure its width and height




