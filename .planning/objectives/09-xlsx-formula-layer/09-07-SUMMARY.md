# 09-07 Summary — calcPr fullCalcOnLoad (partial)

**Status:** partial — export calcPr landed; LibreOffice openability + Go twin spike deferred

**Date:** 2026-10-30

## Delivered

- `workbookXML` emits `<calcPr fullCalcOnLoad="1"/>` when any sheet has formulas
- Value-only workbooks omit `calcPr` (byte-identical to prior exports)
- Locked decision: still omit cached `<v>` from the partial Dart evaluator

## Deferred

- LibreOffice headless openability smoke for formula workbooks
- Go twin evaluator spike
