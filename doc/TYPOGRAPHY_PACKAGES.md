# Typography package branches

The module separates glyphing/layout execution from rendering, without aliases
at the former root, paragraph or adapter/opentype paths.

| Old API | Current owner |
| --- | --- |
| Root resolver, selected fonts, itemization and projections | typography/glyphing |
| Paragraph execution and policies | typography/glyphing/paragraph |
| VList, Line, Box, Material, AtomicPlacement | typography/layout |
| Info, IdentityProvider, FontSource | typography/font (Source replaces FontSource) |
| PaintSpan | typography/style |
| Combined OpenType adapter | glyphing/adapter/opentype and render/adapter/opentype |

Fontselection prepares glyphing.SelectedFonts with the glyphing adapter.
Pipelines compose shaping and rendering services independently. Rendering uses
retained resources, frozen FontInstance bindings, completed placements and
separate paint. Original source files may be removed after preparation.

The glyphing adapter no longer supplies Identify, Outline or VisualOrder.
Create rendering services with render/adapter/opentype.New(). Its independent
outline cache lasts as long as the service; do not copy it after use.

ItemizeWithMetadata returns shaping metadata only. Use style.ProjectPaint on
validated document spans. Replay wire values, identity encoding and static
SVG/placement controls are unchanged.

See [typography's overview](../../typography/README.md). Earlier extraction plans
describe historical paths; this document describes current package ownership.
