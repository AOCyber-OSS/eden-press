# xlsxformula corpus

Published acceptance gate for the eden-press spreadsheet formula layer
(Objective 9 / XLSX-F01).

## Shape

| Artifact | Role |
|---|---|
| `corpus.go` | Source of truth: decisions, cases, helpers |
| `cases.json` | Generated projection for Dart (`go generate`) |
| `inventory.json` | Pinned coverage denominator (dated reference list) |

Never hand-edit `cases.json`. Regenerate:

```bash
go generate ./conformance/corpus/xlsxformula
```

`TestCorpusJSONIsCurrent` fails if the committed file drifts.

## Decisions (locked Wave 0)

- **Evaluator through Phase B:** Dart (`bind/eden_ui`)
- **Go twin:** Wave-4 spike only
- **OOXML writer:** hand-rolled `convert/xlsx` (no excelize/unioffice/werkbook)
- **Cached `<v>`:** omit; Excel recalculates (`fullCalcOnLoad` when formulas present)

## Inventory attribution

`inventory.json` pins the **excelize v2.10.0** `CalcCellValue` supported-function
header as a *coverage denominator reference*. That list is BSD-3-Clause upstream
documentation; eden-press does **not** import or vendor excelize. Phase tags
(`A`/`B`/`C`/`out`) are ours.

## Case schema

Each case has:

- `id` — stable name for failures
- `phase` — `shipped` | `A` | `B` | `C` | `grammar`
- `category` — grammar / logical / math / …
- `sheet` — map of sheet name → map of A1 → raw cell text (formulas keep `=`)
- `fromSheet` — evaluation context sheet
- `expr` — formula body **without** leading `=`
- `expect` — number, string, or bool when success
- `expectError` — Excel-shaped error code when failure (`#NAME?`, …)
- `reason` — why the case exists (load-bearing)
