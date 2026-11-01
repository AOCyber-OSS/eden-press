# 09-08 Summary — Phase C breadth (first tranche)

**Status:** in progress (first tranche shipped)
**Date:** 2026-10-30

## Delivered

First Phase C tranche on the Dart evaluator, bringing the registry to **154**
named functions (Phase B was 110; Objective target for Phase C is 200–300
cumulative).

- **Trig / hyperbolic / angle:** `SIN`/`COS`/`TAN`/`A*`/`*H`, `DEGREES`/`RADIANS`
- **Combinatorics / rounding:** `FACT`, `COMBIN`, `PERMUT`, `EVEN`, `ODD`, `MROUND`
- **Aggregates / order stats:** `SUMSQ`, `SQRTPI`, `LARGE`, `SMALL`
- **Random:** `RAND`, `RANDBETWEEN` (+ `formulaRandOverride` / corpus `rand`)
- **Statistical:** `STDEV`/`STDEV.S`/`STDEV.P`, `VAR`/`VAR.S`/`VAR.P`
- **Text:** `EXACT`, `CLEAN`
- **Financial lite:** `PMT`, `PV`, `FV`, `SLN`
- **Leftover Phase B dates / link:** `TIMEVALUE`, `DAYS360`, `NETWORKDAYS`,
  `WORKDAY`, `HYPERLINK` (display-only)
- **Parser:** dotted function names (`STDEV.S`, …)

Corpus: **61** cases.

## Still open for later Phase C slices

Financial depth, engineering/bit ops, more text (`TEXT`/`FIXED`/`ROMAN`),
`SUMPRODUCT`, percentile/rank family, `NETWORKDAYS.INTL`, coverage dashboard
vs inventory.json, LibreOffice openability (09-07).
