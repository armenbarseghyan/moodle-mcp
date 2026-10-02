# Local patches

Vendored copy of github.com/ledongthuc/pdf v0.0.0-20260907135840-6c8c28e0e8a0
(BSD-3-Clause, see LICENSE), used through a `replace` directive in moodle-mcp's go.mod.

1. `Page.Content`: restore the text encoder together with the font on `Q`, and ignore
   an unbalanced `Q`. Before, text drawn after a `q … /F2 Tf … Q` block was decoded with
   F2's encoding. Word-exported PDFs draw table checkboxes (☐/☒) in a Type0 CID font
   inside such blocks, so whole table rows after them were lost. Regression test:
   `TestContentRestoresEncoderOnQ` (page_qq_test.go).
