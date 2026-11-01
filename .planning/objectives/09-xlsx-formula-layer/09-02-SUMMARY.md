# 09-02 / 09-03 Summary — Phase A functions

**Status:** complete
**Date:** 2026-10-30

## Delivered

Function registry in `bind/eden_ui/lib/src/formula.dart` with **74** named
functions spanning Logical, Math, Statistical, Text, and Information —
exceeding the Phase A ≥40 target.

Highlights: `IF`/`AND`/`OR`/`NOT`/`IFERROR`/`IFS`/`SWITCH`, `ROUND*`/`ABS`/
`POWER`/`SQRT`/`PI`/`PRODUCT`/`GCD`/`LCM`, `COUNT`/`COUNTA`/`MEDIAN`,
`LEFT`/`RIGHT`/`MID`/`LEN`/`TRIM`/`UPPER`/`CONCAT`/`TEXTJOIN`,
`ISBLANK`/`ISNUMBER`/`ISTEXT`/`NA`.

Preserved sparse `visitStored` range walks and xlsxtyping D6 (`007` stays text
under numeric coercion).

## Verification

- Corpus-driven Dart test green (all cases including Phase A)
- `flutter test` (full `bind/eden_ui`) green
- Perf: `SUM` over sparse 100k span still under budget
