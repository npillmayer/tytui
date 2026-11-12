These version-1 manifests were captured from tytui before typography extraction
step 5 (2026-11-11), using both breakers, mixed Latin/pointed Hebrew, a soft hyphen,
and retained font resources. They are compatibility evidence, not golden output
to regenerate after implementation changes. No font bytes are stored here.
Tests supply GentiumPlus-R.ttf and NotoSansHebrew-Regular.ttf from the sibling
opentype fixtures into a temporary content-addressed font store. Paths in the
manifest are provenance labels; replay must not reopen them or call providers.

pre-step6-paint.json was captured before the glyph-boundary policy. It has red f
and blue i in separate shaping spans. Loading it must still produce the original
two-color, two-glyph result; new version-2 runs may form a single red fi ligature.
