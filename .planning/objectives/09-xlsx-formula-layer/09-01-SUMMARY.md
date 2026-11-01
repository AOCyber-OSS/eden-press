# 09-01 Summary — Grammar wave

**Status:** complete
**Date:** 2026-10-30

## Delivered

Extended `bind/eden_ui` formula parser:

- Comparisons (`= <> < > <= >=`)
- `&` concatenation and `"..."` string literals (with `""` escape)
- `TRUE`/`FALSE` literals
- `^` exponentiation
- Absolute/mixed refs (`$A$1`) — ignored for lookup value
- Quoted sheet names (`'My Sheet'!A1`)
- Richer return values: `num` | `String` | `bool` (throws `FormulaError`)

Corpus cases under `phase: grammar` drive the suite.

## Verification

`flutter test test/formula_test.dart` (includes corpus driver) green.
