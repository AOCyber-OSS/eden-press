# 09-04 / 09-05 / 09-06 Summary — Phase B analyst core

**Status:** complete
**Date:** 2026-10-30

## Delivered

Phase B lands as one wave (Date + Lookup + conditional aggregates) on top of
the Wave-2 Dart evaluator:

- **FormulaRange** — multi-cell (and bare cell) function args stay rectangular
  so INDEX/MATCH/VLOOKUP/SUMIF can address by position; aggregates flatten via
  sparse `visitStored` (blanks still omitted for SUM/COUNT).
- **Date & time** — `DATE`, `DATEVALUE`, `TIME`, `TODAY`, `NOW`,
  `YEAR`/`MONTH`/`DAY`/`HOUR`/`MINUTE`/`SECOND`, `WEEKDAY`, `WEEKNUM`, `DAYS`,
  `EDATE`, `EOMONTH`, `YEARFRAC`. Excel serial epoch `1899-12-30`. Test clock
  via `formulaNowOverride` + corpus `now` field.
- **Lookup** — `INDEX`, `MATCH`, `VLOOKUP`, `HLOOKUP`, `XLOOKUP` (exact lite),
  `CHOOSE`, `COLUMN`/`COLUMNS`/`ROW`/`ROWS`, `ADDRESS`.
- **Conditional aggregates** — `SUMIF`/`SUMIFS`, `COUNTIF`/`COUNTIFS`,
  `AVERAGEIF`/`AVERAGEIFS`, `MAXIFS`/`MINIFS` with comparison and `*`/`?`
  criteria.

**110** named functions registered (≥100 Phase B cumulative target). Corpus
grew to 49 cases (16 Phase B).

## Verification

- `flutter test` (full `bind/eden_ui`) green — corpus + perf
- `go generate` + `go test ./conformance/corpus/xlsxformula/...` green
- `make check-no-chromedp check-cli-imports` green
