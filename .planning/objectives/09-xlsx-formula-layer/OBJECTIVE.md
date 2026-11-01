---
work: feature
requirements: [XLSX-F01, XLSX-F02, XLSX-F03, XLSX-F04, XLSX-F05, XLSX-F06]
depends_on: []
---
# Objective 9: Excel Formula Layer for Eden Spreadsheets

## Goal

Grow the live spreadsheet formula layer from today's tiny Dart subset
(`+ - * /`, parentheses, `A1` / `Sheet!A1`, `SUM` / `AVERAGE` / `MIN` / `MAX`)
toward the largest practical Excel-compatible surface that still fits eden-press
constraints: Chrome-free Go OOXML, Flutter-only grid evaluation, MIT, and a
published conformance corpus.

Excel remains the authority for any formula the grid cannot yet evaluate. The
grid shows `#NAME?` (or a documented error) for unsupported functions; the
stored formula still exports as SpreadsheetML `<f>` for Excel / LibreOffice to
calculate.

## Decision gates (resolve during research / Wave 0)

1. **Engine home:** grow `bind/eden_ui` Dart evaluator (recommended) vs adopt a
   Go formula engine vs dual Go+Dart twins. Research locks this before TRD 01.
2. **Corpus authority:** new `conformance/corpus/xlsxformula` is the single
   pass/fail gate; Excel / LibreOffice oracle fixtures when available.
3. **"As much as possible" ceiling:** Phase C (~200–300 functions) is the
   stated ambition; Microsoft 365's full ~500+ catalog and LAMBDA/dynamic-array
   family are deferred hard outs, not silent stretch goals.

## Requirements

XLSX-F01 … XLSX-F06 (see `.planning/REQUIREMENTS.md`)

## Success criteria (what must be TRUE)

1. A language-neutral formula corpus exists under
   `conformance/corpus/xlsxformula` with cases for grammar, functions by
   category, errors, and typing interaction with `xlsxtyping`. The Dart
   evaluator is driven by that corpus (not a private inline table).
2. Grammar Wave 0 lands: comparisons, `&` concat, string/bool literals,
   absolute refs (`$A$1`), and Excel-shaped errors — without breaking the
   sparse-range / display-cache invariants already locked by PR #11.
3. Function coverage reaches the Phase A "daily driver" set (≥40 named
   functions across Logical / Math / Text / Information / basic Statistical),
   then Phase B "analyst core" (≥100 including Lookup and Date), with Phase C
   tracked as a coverage dashboard against a pinned Microsoft category list —
   never claimed as 100% of Excel.
4. Unsupported functions still round-trip through `Workbook` → `.xlsx` →
   `Open` as formulas; the grid surfaces `#NAME?`; Excel recalculates on open
   (`fullCalcOnLoad` when needed). No stale cached `<v>` is invented by a
   partial evaluator.
5. `go list -deps` on `./convert/xlsx/...`, `./press/...`, `./chase/...`,
   `./cmd/eden-press/...` still contains no `chromedp`, and no AGPL / commercial
   OOXML stack (`unioffice` and forks) is introduced.
6. AODex / host consumers keep a single typing rule via `xlsxtyping`; formula
   cells remain a distinct class from number/text (decision D6 preserved).

## Plans (proposed TRD waves)

- [x] 09-00-TRD.md — Decision lock + corpus scaffold + coverage inventory (Wave 0) — SUMMARY: 09-00-SUMMARY.md
- [x] 09-01 — Grammar: comparisons, strings, bools, `&`, `$A$1`, `^`, quoted sheets (Wave 1) — SUMMARY: 09-01-SUMMARY.md
- [x] 09-02 / 09-03 — Phase A Logical/Math/Text/Info (74 named functions) (Wave 2) — SUMMARY: 09-02-SUMMARY.md
- [x] 09-04 / 09-05 / 09-06 — Phase B Date + Lookup + conditional aggregates (110 named functions) (Wave 3) — SUMMARY: 09-04-SUMMARY.md
- [~] 09-07 — Export calcPr landed; LO openability + Go twin deferred (Wave 4) — SUMMARY: 09-07-SUMMARY.md
- [~] 09-08 — Phase C first tranche (154 named functions; dashboard/capstone open) (Wave 4) — SUMMARY: 09-08-SUMMARY.md

> **Internal parallelism:** 09-02 and 09-03 share only the Wave-0 grammar and
> can execute as sibling jobs. 09-04/05/06 similarly fan out after Phase A.
> 09-07/08 are join points.

## Out of scope (this objective)

- Realtime collaboration, comments, sharing (held in eden-editor surface spec)
- Docs/slides rich-text editing
- VBA / macros / ActiveX / external workbook links / Cube / WEBSERVICE
- Full LAMBDA + dynamic-array spill model (FILTER/UNIQUE/SORT as first-class)
- Replacing hand-rolled `convert/xlsx` with excelize / werkbook / unioffice
- Evaluating formulas inside Markdown → `ToXLSX` document export (tables stay
  values unless a future TRD opts in)

---
*Created: 2026-10-30 (plan for Excel formula layer post PR #11 workbook/UI seam)*
