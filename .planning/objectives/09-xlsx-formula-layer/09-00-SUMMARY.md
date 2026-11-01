# 09-00 Summary — Decision lock + corpus + inventory

**Status:** complete
**Date:** 2026-10-30

## Delivered

- `conformance/corpus/xlsxformula` with schema `eden-press.xlsxformula/v1`
- Wave-0 decisions embedded in every `cases.json` (Dart through Phase B; omit `<v>`; hand-rolled OOXML)
- `inventory.json` pinned 2026-10-30 from excelize v2.10.0 CalcCellValue header (reference only; 454 names; Phase A ≥40 tagged)
- Baseline + grammar + Phase A cases (33) as Go source; `go generate` projection
- Anti-drift tests mirroring `xlsxtyping`

## Verification

`go test ./conformance/corpus/xlsxformula/` green.
