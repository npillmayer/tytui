# Replay evidence status (CSS-only migration, 2026-11-17)

Version 5 is the only supported format. All checked-in version-1/version-4
manifests below are now rejection cases; their bytes are unchanged.
current-controls.json is unchanged: its eight CSS controls remain executable,
and its eight compatibility controls are historical. Current version-5 resaves
are covered by CSS replay tests, including source removal and pinned resources.
No test automatically regenerates expected evidence.

The notes below describe when the historical cases were captured.

These version-1 manifests were captured from tytui before typography extraction
step 5 (2026-11-11), using both breakers, mixed Latin/pointed Hebrew, a soft hyphen,
and retained font resources. They are historical evidence, not golden output
to regenerate after implementation changes. Version 4 rejects them before loading
fonts; tests no longer execute their historical behavior. No font bytes are stored here.
Tests supply GentiumPlus-R.ttf and NotoSansHebrew-Regular.ttf from the sibling
opentype fixtures into a temporary content-addressed font store. Paths in the
manifest are provenance labels; replay must not reopen them or call providers.

pre-step6-paint.json was captured before the glyph-boundary policy. It has red f
and blue i in separate shaping spans. Its historical two-color, two-glyph output
is intentionally unsupported by current replay. New current-policy runs form a
single red fi ligature with this font.

## Current-behavior controls (2026-11-15)

`current-controls.json` is a schema-independent regression capture, not a replay
manifest. It records 16 SVG SHA-256 hashes and numeric vlist box/line placements
for `TestCurrentBehaviorReplayControls`. Inputs are written directly in that test;
no historical manifest is loaded to obtain them. Both breakers and compatibility/
CSS vertical policies are represented. The test never regenerates the file.
Intentional changes require inspecting the affected SVG/geometry and reviewing a
separate fixture edit; round-trip agreement alone does not justify an update.

The capture used these checkout HEADs (the only tytui changes at capture were the
new regression test/control file):

- tytui: `57a874c1ac1fa7956b3d24be20b5e06060029931`
- typography: `77bef3655721fa0a4667bcd9abff6cf94adc31a4`
- khipu: `d4f0a084af9f8a6b8f4675b8635274fc33a69a63`
- opentype: `b16d15181fd1a9b744eb24248d46f02599dbe57d`

The font inputs remain the sibling fixtures, with no binary copies added:

| Font | SHA-256 |
| --- | --- |
| GentiumPlus-R.ttf | `fa09788adcc651d77eba7d96f8cc3a600d6484c58e3b4ff79c2b2464dbfea6f8` |
| NotoSansHebrew-Regular.ttf | `a7fa16fffb27bedb060a0866267c29e9859aeb9c21cc33f5b3aaf6eb062eca85` |

See [the regression inventory](../../doc/REPLAY_REGRESSION_INVENTORY.md) for each
historical fixture's current-policy successor and independent correctness tests.
The controls were captured in step 1 and remain unchanged through step 2.

## Current version-4 manifests

`current-fi-firstfit.json` and `current-fi-knuth-plass.json` are new direct-input
cases captured in step 2, not relabelled historical output. Both use split red-f /
blue-i document styles, current glyph-boundary behavior and compatibility assembly.
They produce one red glyph and match the corresponding step-1 SVG/placement
control exactly. Tests supply Gentium bytes by hash, verify load/save/load,
selection provenance and retained-assembly behavior after editing inspection input.

Their recorded revision evidence comes from CaptureRevisions during capture and
marks the tytui checkout dirty; it does not archive that patch. The input is `fi`,
12bp Gentium Plus, 100bp width, English, serif/400/normal, byte ranges [0,1) red
and [1,2) blue. The capture used the unchanged pipeline and the new v4 Save API.
Font bytes remain external with the hashes above. No regeneration tool is shipped.

Step 3 removes the shaping/color selectors from typography and pipeline. These
manifest bytes and current-controls.json remain unchanged; only current behavior
is executable. No paint-policy field or legacy Save guard remains.

Final cleanup (2026-11-16) preserves all six JSON files byte-for-byte. For a new
case or intentional expectation change, follow the explicit review workflow in
[OFFLINE_REPLAY.md](../../doc/OFFLINE_REPLAY.md#creating-and-reviewing-a-fresh-case).
Tests never regenerate these fixtures automatically.
