# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-20)

**Core value:** Render Marp-compatible documents from Markdown inside a Go service or CLI with no JavaScript runtime, no Node, and no browser for HTML/structured output — while emitting the document as structured data, not just HTML.
**Current focus:** Objective 9 in progress — Excel formula layer. Wave 0–2 done (corpus, grammar, Phase A = 74 functions); Phase B/C remaining.

## Current Position

Objective: 9 — Excel Formula Layer for Eden Spreadsheets
Job: Waves 0–2 complete on branch `cursor/xlsx-formula-layer-plan-6283`. Next: Phase B (dates, lookups, conditional aggregates).
Status: Live Dart evaluator covers grammar + Phase A daily-driver set; `xlsxformula` corpus is the acceptance gate; `calcPr fullCalcOnLoad` when formulas present; Excel still authority for unsupported functions.

Progress: v1 [##########] 100% · Obj 9 [#####-----] ~55% (Phase A done; Phase B/C open)

## Accumulated Context

> Decisions and performance metrics are logged in STATE_ARCHIVE.md.

### Pending Todos

- Phase B: Date & Time, Lookup, SUMIF/COUNTIF family
- Phase C breadth + coverage dashboard
- LibreOffice formula openability smoke; optional Go twin spike

### Blockers/Concerns

- None. Keep omitting cached `<v>`; preserve sparse range walks and xlsxtyping D6.

## Session Continuity

Last activity: 2026-10-30 - Implemented Objective 9 Waves 0–2 (corpus, grammar, Phase A) + calcPr
Stopped at: Phase A green; Phase B not started
Resume file: `.planning/objectives/09-xlsx-formula-layer/OBJECTIVE.md`
