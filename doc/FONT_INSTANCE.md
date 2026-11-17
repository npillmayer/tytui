# Retained font instances

Font selection still supplies loaded faces; style resolution supplies a face and
physical em size. Khipu binds them with contracts.NewFontInstance during run
preparation. ShapeParams.Font retains the immutable instance throughout shaping,
discretionary replacements, binding and freezing.

FrozenTextContext now contains Input and Params only. Consumers obtain the face,
size and conversion from Context.Params.Font.Face(), .EmSize() and .Scale().
There are no legacy EmSize or Scale fields on the context. SVG export and replay
evidence read the retained instance without reconstructing it or reshaping.

Replay serializes resource identity and em size as data, never instance/service
handles. Its wire format and existing glyph/placement evidence are unchanged.
The loaded face remains independent of size; changing CSS size creates a new
instance sharing the face, not another parsed font.
