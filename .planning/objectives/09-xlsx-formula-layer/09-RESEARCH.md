# Objective 9: Excel Formula Layer — Research

**Researched:** 2026-10-30
**Domain:** Excel formula language (ECMA-376 SpreadsheetML §18.17) as consumed by
eden-press editable workbooks — Go OOXML store/export + Flutter live evaluation
**Confidence:** HIGH for current eden-press seams (direct code inspection of
`convert/xlsx` + `bind/eden_ui` after PR #11). HIGH for SpreadsheetML `<f>` /
optional `<v>` / `calcPr` semantics (Microsoft Learn OOXML docs). MEDIUM for
exact Microsoft 365 function cardinality (category pages drift; treat ~440–520
as the denominator band, pin a dated inventory in Wave 0). HIGH that adopting
excelize/unioffice as the *writer* conflicts with the hand-rolled MIT OOXML
posture already locked for pptx/docx/xlsx.

<phase_requirements>
## Objective Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| XLSX-F01 | Published formula corpus as acceptance gate | Architecture Pattern 1 (corpus-first, mirrors `xlsxtyping`) |
| XLSX-F02 | Grammar toward Excel expression language | Pattern 2 (parser rewrite before function sprawl); Pitfalls 1–3 |
| XLSX-F03 | Phased named-function coverage A→B→C | Pattern 3 (category waves); Standard Stack alternatives |
| XLSX-F04 | Unsupported → `#NAME?` in grid, still export `<f>` | Pattern 4 (dual authority: Dart preview / Excel truth) |
| XLSX-F05 | No chromedp / no AGPL OOXML; convert/xlsx stays hand-rolled | Alternatives Considered; Pitfall 6 |
| XLSX-F06 | Preserve `xlsxtyping` + formula-vs-value distinction (D6) | Pitfall 4; current `Workbook.Set` / `formulaXML` |
</phase_requirements>

## Summary

PR #11 (`372b696`) shipped the spreadsheet *seam*: `convert/xlsx.Workbook`
stores formulas (leading `=` stripped for SpreadsheetML `<f>`, restored on
`Get`), packs/opens `.xlsx`, and snapshots `eden-press.workbook/v2`.
`bind/eden_ui` evaluates a **deliberately tiny** subset in Dart so the grid can
show results while editing. The surface spec explicitly holds "a full Excel
formula language" outside that change.

The load-bearing architecture is already correct for a long march toward Excel:

1. **Store the author's formula** for Excel (Go never invents a cached `<v>`).
2. **Evaluate a growing subset in Dart** for the live grid.
3. **Leave Excel / LibreOffice as the authority** for everything the subset
   does not cover.

"Support as much of the Excel formula layer as possible" therefore means
maximizing (2) under eden-press constraints — not claiming Microsoft 365
parity, and not replacing the OOXML writer with a formula-rich third-party
stack.

**Primary recommendation:** Keep evaluation in Dart (`bind/eden_ui`), rebuild
the parser once for real Excel grammar (Wave 0/1), drive every function add
from a published `conformance/corpus/xlsxformula` corpus (mirror
`xlsxtyping` / decision D6), phase functions by editor usefulness
(daily-driver → analyst → breadth), and emit `workbook.xml` `calcPr
fullCalcOnLoad="1"` when any formula lacks a trusted cache so Excel always
recomputes. Reject excelize/unioffice/werkbook as the packaging path; optionally
*study* their function lists as coverage inventories only.

## Current state (inspected 2026-10-30)

### Go — `convert/xlsx`

| Capability | Status |
|---|---|
| Store formula cells (`Set("=SUM(A1:A3)")`) | Done — `storedCell.formula`, body without `=` |
| Emit `<c r="…"><f>…</f></c>` | Done — `formulaXML`; **no** `<v>` cache |
| Open `.xlsx` formulas back through `Get` | Done — `open.go` prefers `<f>` over cached `<v>` |
| Workbook JSON v2 `{r,c,f}` / `{r,c,v}` | Done |
| Evaluate formulas in Go | **Not present** — package docs: "Formulas are stored, not evaluated" |
| `calcPr` / `calcChain` | **Not present** |
| Markdown `ToXLSX` path marks formulas | **No** — table cells that look like `=…` stay text (intentional) |

### Dart — `bind/eden_ui`

| Capability | Status |
|---|---|
| Arithmetic `+ - * /`, unary `-`, parentheses | Done |
| Cell refs `A1`, sheet refs `Sheet!A1` | Done (case-insensitive sheet names) |
| Ranges `A1:A3` for aggregate args | Done — sparse `visitStored` only |
| Functions | **Only** `SUM`, `AVERAGE`, `MIN`, `MAX` |
| Display cache + invalidation via read spans | Done |
| Errors | `#ERR`, `#NUM!`, `#DIV/0!`, `#NAME?`, `#REF!`, `#VALUE!`, `#CYCLE!` |
| Strings, comparisons, `&`, `$A$1`, booleans | **Missing** |
| IF / COUNT / ROUND / lookups / dates | **Missing** (`POW` already asserts `#NAME?`) |

Doc comment in `formula.dart` is explicit: *"This is not Excel's formula
language. The stored formula is still what an `.xlsx` export asks Excel to
calculate."*

## Excel formula layer — what "as much as possible" means

Microsoft's public "Excel functions (by category)" inventory for Microsoft 365
/ Excel 2024 sits in a **~440–520 named-function** band depending on the dated
snapshot (third-party lists cite 442 / 480 / 512). Categories that matter for
an editable Sheets-like surface:

| Category | Role for eden-ui | Priority |
|---|---|---|
| Logical | IF, AND, OR, NOT, IFERROR, IFS, SWITCH | Phase A |
| Math & Trig (common) | ABS, ROUND*, INT, MOD, POWER, SQRT, SUM already | Phase A |
| Statistical (common) | COUNT, COUNTA, COUNTBLANK, AVERAGE* already, MEDIAN | Phase A |
| Text | LEFT/RIGHT/MID, LEN, TRIM, UPPER/LOWER, CONCAT/&, TEXT | Phase A |
| Information | ISBLANK, ISNUMBER, ISTEXT, IFERROR companion | Phase A |
| Date & Time | DATE, TODAY, YEAR/MONTH/DAY, EDATE, NETWORKDAYS | Phase B |
| Lookup & Reference | INDEX, MATCH, VLOOKUP, HLOOKUP, XLOOKUP, INDIRECT† | Phase B |
| Conditional aggregates | SUMIF(S), COUNTIF(S), AVERAGEIF(S) | Phase B |
| Financial / Engineering / Database / Cube | Niche | Phase C / never (Cube) |
| Dynamic array + LAMBDA family | FILTER, UNIQUE, SORT, LAMBDA, MAP, … | **Hard out** for this objective |
| Web / Cube / RTD / external links | Network / OLAP | **Never** (SAFE-01 conflict) |

† `INDIRECT` is powerful and also a classic injection / volatility footgun —
gate it behind an explicit Options flag or defer to Phase C with a security
note.

### Coverage targets (planning numbers, not marketing)

| Milestone | Named functions (approx.) | Grid UX meaning |
|---|---|---|
| Today (PR #11) | 4 | Totals row demos |
| Phase A — daily driver | ≥ 40 | Budgets, IF logic, text cleanup |
| Phase B — analyst core | ≥ 100 | Lookups + dates + conditional sums |
| Phase C — breadth | 200–300 | Approaches excelize/werkbook class engines |
| Excel 365 full catalog | ~440–520 | **Not a v1 claim** |

Werkbook advertises ~450; excelize's `CalcCellValue` documents 200+ with
explicit gaps (iterative calc, some array/table formulas). Those numbers are
**reference ceilings**, not dependencies to import.

## Standard Stack

### Core (recommended)

| Piece | Role | Why |
|---|---|---|
| Hand-grown Dart evaluator in `bind/eden_ui` | Live grid results | Already shipped; Flutter-only; no JS; matches UI ownership |
| `conformance/corpus/xlsxformula` (new) | Language-neutral cases | Same pattern as `xlsxtyping` that closed D6 drift |
| Hand-rolled `convert/xlsx` (existing) | Persist `<f>`, open, JSON | Already MIT, chromedp-free, deterministic |

### Supporting

| Piece | When |
|---|---|
| LibreOffice headless (CI-only) | Openability of formula workbooks; optional CSV of *values* after recalc if LO cooperates |
| Microsoft Learn OOXML formula docs | Spec authority for `<f>` / `<v>` / `calcPr` |
| Pinned Excel function category dump (Wave 0 artifact) | Coverage denominator for Phase C dashboard |

### Alternatives considered

| Instead of growing Dart | Could use | Tradeoff | Verdict |
|---|---|---|---|
| `xuri/excelize` CalcCellValue | 200+ Go functions | Pulls excelize as dep; tempts replacing hand-rolled writer; BSD-3 ok but architecture conflict | **Reject as writer/engine host**; may cite function list only |
| `jpoz/werkbook` | ~450 funcs, MIT, zero-deps Go | Young (2026); would own packaging; dual UI+Go engines still needed for Flutter | **Reject as packaging**; study FORMULAS.md as inventory |
| `unioffice` / forks formula pkg | Mature-ish Go formula | **AGPLv3 / commercial key** — forbidden by Obj 6 decision | **Reject** |
| `xuri/efp` (parse only) | AST from Excel formulas | Parse ≠ eval; Dart still needs eval; extra dep | Optional reference grammar only |
| Port engine to Go + expose via capi/wasm | One engine for CLI hosts too | Large; Flutter already has Dart engine; dual-bind risk | **Defer** — Wave 4 spike gate only |
| Emit formulas only, never evaluate in grid | Zero engine work | Grid shows raw `=…` or blank — fails Sheets competition | **Reject** as primary UX |

**Installation:** none for the recommended path — continue additive Dart +
stdlib Go. If Wave 4 spike chooses a Go twin, add deps only via
`go mod download` + hand edit of `go.mod` (**never** `go mod tidy`).

## Architecture Patterns

### Pattern 1: Corpus-first function adds (mirror `xlsxtyping`)

**What:** Every grammar rule and named function lands as rows in
`conformance/corpus/xlsxformula/cases.json` (schema
`eden-press.xlsxformula/v1`) with `{expr, sheetFixture, expect, expectError,
category, excelNote}`. Dart tests consume the corpus; optional Go twin later
consumes the same file.

**Why:** The D6 lesson — two green suites disagreeing about `"+3"` — applies
doubly to formula semantics (`AVERAGE` of blanks, `SUM` of text, `IF` truthiness).

### Pattern 2: Grammar wave before function sprawl

**What:** Replace / extend the current identifier-driven `_Parser` so it can
represent Excel expressions: string literals (`"..."`), booleans, comparisons
(`= <> < > <= >=` inside formulas), concatenation `&`, absolute/mixed refs
(`$A$1`, `A$1`, `$A1`), and structured error propagation.

**Why:** IF, COUNTIF, and text functions are blocked on strings and
comparisons. Adding ROUND without strings is fine; adding LEFT without strings
is impossible. Do grammar once.

### Pattern 3: Dual authority (Dart preview / Excel truth)

**What:**
- Grid: evaluate known subset; unknown function → `#NAME?` (already).
- File: always write `<f>body</f>` with **no** guessed `<v>` unless a future
  trusted full evaluator exists.
- Workbook: set `<calcPr fullCalcOnLoad="1"/>` when formulas are present so
  Excel recomputes even if some viewers show blank.

**Why:** Spec-valid (Microsoft Learn: omitting `<v>` is allowed; reader must
compute). Avoids lying caches — already the documented reason
`formulaXML` skips `<v>`.

### Pattern 4: Sparse evaluation stays sacred

**What:** Range walks continue to use `visitStored` / occupied cells only.
`SUM(A1:A100000)` with three occupied cells must stay O(occupied).

**Why:** PR #11 perf tests already lock `<25ms` for that case. A naive dense
loop would make Phase B conditional aggregates unusable on large sheets.

### Pattern 5: Function registry, not a growing switch

**What:** `_call` becomes a map of `name → impl` with arity metadata and
category tags, exported for a coverage listing (`wb`-style `formula list` can
be a Dart unit that dumps the registry into the corpus README).

**Why:** Phase C is hundreds of functions; a switch statement does not scale
reviewably.

### Recommended package shape (additive)

```
conformance/corpus/xlsxformula/
  cases.json              # language-neutral fixtures
  corpus.go               # Go loader (even if Go eval is later)
  README.md               # schema + coverage policy

bind/eden_ui/lib/src/
  formula.dart            # façade: evaluateFormula / displayCell
  formula/
    parser.dart           # grammar
    value.dart            # number | string | bool | error
    registry.dart         # function table
    functions_*.dart      # per-category impls
    errors.dart

convert/xlsx/
  workbook.go             # unchanged store semantics
  parts_static.go         # additive calcPr when formulas present
  # no Go evaluator in Wave 1–3 unless spike says otherwise
```

## Common Pitfalls

1. **Cached `<v>` from a partial engine.** Writing a Dart-computed cache into
   OOXML makes LibreOffice/Excel show a value that may not match Excel's own
   semantics for edge cases. **Avoid:** keep omitting `<v>`; use
   `fullCalcOnLoad`.
2. **Treating `=` table cells in Markdown export as formulas.** `ToXLSX` from
   docmodel currently leaves `=` as text on purpose. Flipping that without an
   explicit Options flag will surprise document exporters.
3. **Dense range iteration.** Re-breaking the sparse visit invariant when
   implementing COUNTIF/SUMIF.
4. **Breaking `xlsxtyping` / D6.** Formula detection must remain
   `strings.HasPrefix(value, "=")` at Set-time, not a ParseFloat side effect.
   `"=SUM(A1:A5)"` must never become an inlineStr of the literal including
   confused numeric coercion.
5. **Sheet-name quoting.** Excel uses `'My Sheet'!A1` for names with spaces.
   Today's parser accepts `Notes!A1` only. Grammar wave must add quoting.
6. **Pulling AGPL OOXML or replacing the writer** to "get formulas free."
   Conflicts with Obj 6 / EPD-R4 posture.
7. **Claiming Excel parity in UI copy.** Surface spec and AGENTS.md must keep
   "subset evaluated live; Excel calculates the file."
8. **Volatile functions (`NOW`, `TODAY`, `RAND`) without a clock seam.**
   Inject a `FormulaHost.now()` for tests; document that export still has no
   cached value.
9. **Circular refs.** Already `#CYCLE!`. Keep; do not add iterative-calc mode
   in this objective.
10. **Array / spill formulas.** Returning multiple cells needs grid layout
    changes far beyond the formula parser. Hard out.

## Open Questions

| # | Question | Blocking? | Proposed default |
|---|---|---|---|
| 1 | Dart-only vs Go twin evaluator in this objective? | Yes for Wave 4 | Dart-only through Phase B; Go twin is an explicit spike TRD with kill criteria |
| 2 | Pin which Microsoft function list as denominator? | Yes for Phase C dashboard | Snapshot Microsoft category page on Wave 0 date into `xlsxformula/inventory.json` |
| 3 | Enable `INDIRECT` / `OFFSET`? | No | Defer; security + volatility |
| 4 | Should Markdown `ToXLSX` gain `Options.FormulasFromCells`? | No | Leave false; separate quick task if AODex asks |
| 5 | Shared formulas / `t="shared"` in OOXML? | No | Emit only ordinary `<f>` per cell; Excel may rewrite on save |
| 6 | Locale / argument separators (`;` vs `,`)? | Medium | v1 = Excel en-US `,` and `.` only; document |

## Sources

- In-repo: `bind/eden_ui/lib/src/formula.dart`, `formula_test.dart`,
  `convert/xlsx/workbook.go`, `sheet.go` (`formulaXML`), `open.go`,
  `bind/eden_ui/flutter/ui_spec/eden-editor.md` ("Held outside this surface")
- Microsoft Learn: [Working with formulas](https://learn.microsoft.com/en-us/office/open-xml/spreadsheet/working-with-formulas)
  (formula in `<f>`, optional cached `<v>`)
- Microsoft Learn: [Working with the calculation chain](https://learn.microsoft.com/en-us/office/open-xml/spreadsheet/working-with-the-calculation-chain)
  (`calcChain` optional)
- Microsoft Support: [Excel functions (by category)](https://support.microsoft.com/en-us/excel/excel-functions-by-category)
- excelize `CalcCellValue` supported-function header (coverage reference only)
- werkbook `FORMULAS.md` (~450 functions — coverage reference only)
- Prior eden-press decision: hand-rolled OOXML, reject unioffice (Obj 6 RESEARCH)

---
*Research completed: 2026-10-30*
