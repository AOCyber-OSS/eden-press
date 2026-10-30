---
schema_version: 1
surface: eden-editor
patterns:
  - form/input
  - navigation/tabs
  - navigation/icon-button
  - navigation/pager
  - data/grid
  - feedback/empty-state
  - content/document-viewer
references:
  mockup: refs/eden-editor/mockup/
  donor: refs/eden-editor/pattern-mapping.md
  locked: refs/eden-editor/locked/
design_read: "Reading this as: an embeddable Docs, Sheets, and Slides workspace for people who already live in those tools, dense and conventional, on EdenTheme gold tokens with minimal motion."
mode: redesign
routes:
  - id: editor
    path: /
    entry: [embed]
    root: true
    reachable_from_nav: false
    justification: "A host application places EdenView. This surface has no navigator entry of its own."
    title: "{file.name}"
controls:
  - id: sheet.formula
    kind: input
    visible_in: [populated, long-content, narrow, dark]
    does: "Writes the raw value or formula into the selected cell and leaves that cell selected."
    effect: [submit]
    must_not: ["change route", "lose selection", "steal focus", "fire twice per activation"]
    activation: {pointer: true, keyboard: [Enter]}
    a11y: {role: textbox, announces: [value]}
    hit_rect:
      disjoint_from: [sheet.grid, sheet.tab, sheet.add]
  - id: sheet.grid
    kind: button
    visible_in: [populated, long-content, narrow, dark]
    does: "Selects the activated cell and loads its raw value into the formula bar."
    effect: [select]
    must_not: ["change route", "lose selection", "fire twice per activation", "cover sibling hit rects"]
    activation: {pointer: true, keyboard: [Enter, Space]}
    a11y: {role: grid, announces: [selected]}
    hit_rect:
      disjoint_from: [sheet.formula, sheet.tab, sheet.add]
  - id: sheet.tab
    kind: button
    visible_in: [populated, long-content, narrow, dark]
    does: "Selects that sheet and shows its grid."
    effect: [select]
    must_not: ["change route", "fire twice per activation", "cover sibling hit rects"]
    activation: {pointer: true, keyboard: [Enter, Space]}
    a11y: {role: tab, announces: [selected]}
    hit_rect:
      disjoint_from: [sheet.formula, sheet.grid, sheet.add]
  - id: sheet.add
    kind: button
    visible_in: [populated, empty, long-content, narrow, dark]
    does: "Appends a sheet with the next fresh name and selects it."
    effect: [select]
    must_not: ["change route", "fire twice per activation", "cover sibling hit rects"]
    activation: {pointer: true, keyboard: [Enter, Space]}
    a11y: {role: button, announces: [selected]}
    hit_rect:
      max: "56x56"
      disjoint_from: [sheet.formula, sheet.grid, sheet.tab]
  - id: slides.previous
    kind: button
    visible_in: [slides]
    does: "Shows the previous slide and updates the slide count label."
    effect: [select]
    must_not: ["change route", "fire twice per activation", "cover sibling hit rects"]
    activation: {pointer: true, keyboard: [ArrowLeft, Enter, Space]}
    disabled_when: {condition: "the first slide is showing", reason_shown: "First slide"}
    a11y: {role: button, announces: [selected]}
    hit_rect:
      max: "56x56"
      disjoint_from: [slides.next]
  - id: slides.next
    kind: button
    visible_in: [slides]
    does: "Shows the next slide and updates the slide count label."
    effect: [select]
    must_not: ["change route", "fire twice per activation", "cover sibling hit rects"]
    activation: {pointer: true, keyboard: [ArrowRight, Enter, Space]}
    disabled_when: {condition: "the last slide is showing", reason_shown: "Last slide"}
    a11y: {role: button, announces: [selected]}
    hit_rect:
      max: "56x56"
      disjoint_from: [slides.previous]
states:
  - id: populated
    seed: workbook-metrics
    content:
      must_show: ["Metrics", "550"]
      must_not_show: ["Add a sheet", "Workbook unavailable", "Could not open this file"]
  - id: empty
    seed: workbook-no-sheets
    content:
      must_show: ["Add a sheet"]
      must_not_show: ["Workbook unavailable", "Could not open this file", "550"]
  - id: error
    seed: workbook-metrics
    fault: snapshot-parse-failed
    content:
      must_show: ["Could not open this file"]
      must_not_show: ["Add a sheet", "Workbook unavailable", "550"]
  - id: outage
    seed: workbook-metrics
    fault: press-library-unavailable
    content:
      must_show: ["Workbook unavailable"]
      must_not_show: ["Add a sheet", "Could not open this file", "550"]
  - id: long-content
    seed: workbook-long-names
    content:
      rule: "A 31-character sheet name stays on one line inside the tab strip. Cell text ellipsizes. The formula bar shows the full raw value."
  - id: narrow
    viewport: "390x844"
    seed: workbook-metrics
    content:
      must_show: ["Metrics", "550"]
      rule: "The grid scrolls inside itself. The page does not scroll sideways. Cell rows are at least 48dp tall."
  - id: dark
    theme: dark
    seed: workbook-metrics
    content:
      must_show: ["Metrics", "550"]
      rule: "Surfaces come from EdenTheme.dark. No white cell fill and no light formula bar."
  - id: document
    seed: doc-two-paragraphs
    content:
      must_show: ["Alpha", "Beta"]
      must_not_show: ["Slide 1 of 2", "Add a sheet"]
  - id: slides
    seed: deck-two-slides
    content:
      must_show: ["Alpha", "Slide 1 of 2"]
      must_not_show: ["Beta", "Add a sheet"]
flows:
  - id: commit-cell
    steps:
      - click: sheet.grid
        expect:
          route: editor
      - click: sheet.formula
        expect:
          route: editor
  - id: add-sheet
    steps:
      - click: sheet.add
        expect:
          route: editor
  - id: advance-slide
    steps:
      - click: slides.next
        expect:
          route: editor
          content:
            must_show: ["Slide 2 of 2", "Beta"]
            must_not_show: ["Alpha"]
scope_rules:
  - on: logout
    reset: [selected-cell, selected-sheet, slide-index]
---

## Intent

`EdenView` is the embed a host drops into its own tree when it wants one surface for a flowing document, a paged deck, and a workbook. The host keeps the bytes. The widget applies a spreadsheet edit locally and reports `EdenCellEdit` or `EdenSheetAdded`. Docs and slides still render `ModelDocument` through `EdenPressView`.

People who already live in Docs, Sheets, and Slides should recognise the jobs (write, present, calculate) without the surface dressing itself as those products. The accent is Eden gold via `EdenTheme`, not the donor's `#0B57D0`.

## Design read

Reading this as: an embeddable Docs, Sheets, and Slides workspace for people who already live in those tools, dense and conventional, on EdenTheme gold tokens with minimal motion.

Dials, read off the donor first (EXPRESSION 2, MOTION 1, DENSITY 6):

- **EXPRESSION 4.** Overhaul is the donor plus 2. Left-aligned formula bar and cell text, one quiet title from `{file.name}`, no hero and no feature row.
- **MOTION 3.** Overhaul is the donor plus 2. Pressed and selected states only. The slide change is user-driven. `MediaQuery.disableAnimations` removes the page transition instead of shortening it.
- **DENSITY 8.** The derivation table's instrument band, not the overhaul increment. Desktop cells stay hairline and short. Narrow cells grow to the 48dp target. Document text measure stays at 640.

Colour lock: `EdenColors.gold` only. Shape lock: `EdenRadii` (inputs `md`, the slide page `lg`). Theme lock: one theme per capture, light or `EdenTheme.dark()`. Icon lock: Material Symbols, one weight.

## Audit of the donor

What exists today in `lib/src/eden_view.dart` and `lib/src/spreadsheet_view.dart`:

- Three kinds chosen by `kind` or by inference. A workbook forces the spreadsheet. `marp: true` forces slides. A table-only model becomes a sheet. A `profile: article` model stays a flowing document.
- Spreadsheet chrome is a gray formula bar, text tabs, an add icon, and a white grid. Selection and the selected tab are `#0B57D0`. Spacing and type are raw numbers.
- Slides are a `PageView` plus the literal `Slide n of N`. Drag changes the page. There is no previous or next control.
- The document is a padded list of sections. Both paragraphs of an article are on screen together.
- No empty, error, outage, long-content, narrow, or dark state is built. A zero-sheet book renders an empty box.
- Widget keys the tests already use: `eden-formula-bar`, `eden-add-sheet`, `eden-slides`. Those keys stay. The semantics identifiers in this spec are new and are what a probe looks up.

Patterns to keep: host-owned bytes, `eden-press.workbook/v1` as the edit snapshot, formulas stored for Excel and evaluated in Dart for the grid, the slide-count sentence, one slide visible at a time, sheet names from `freshSheetName()`.

Patterns to retire: every hardcoded color, the raw `TextField` / `TextButton` / `GestureDetector` chrome, the centered-spinner-shaped absence of an empty state, and a slide deck that cannot be moved from the keyboard.

## Walkthrough

**commit-cell.** On the Metrics book, activating the cell that shows `550` selects it and puts the raw value in the formula bar. Submitting the bar writes that cell and the selection stays put. The route stays `editor`.

**add-sheet.** Activating `sheet.add` appends the next fresh sheet and selects it. On the empty book the same control is the sentence `Add a sheet`.

**advance-slide.** From the first slide (`Alpha`, `Slide 1 of 2`), next shows `Beta` and `Slide 2 of 2`. Previous is disabled on the first slide with the reason `First slide`. Next is disabled on the last slide with the reason `Last slide`.

## Held outside this surface

Realtime collaboration, comments, sharing, and a full Excel formula language are not controls here. Docs and slides do not emit rich-text edits yet. A product switcher inside the widget is not a control either: the host passes `kind` or a workbook. Those are later surfaces, not deltas on this one.

## Seeds

| Seed | What the capture builds |
|---|---|
| `workbook-metrics` | Sheet `Metrics`, header row `Metric` / `Q3`, body `p95` / `550`. |
| `workbook-no-sheets` | A workbook with no sheets. |
| `workbook-long-names` | One sheet whose name is 31 characters, and one cell whose text is longer than the cell. |
| `doc-two-paragraphs` | `profile: article`, two sections, text `Alpha` and `Beta`, both visible. |
| `deck-two-slides` | `marp: true`, headings `Alpha` then `Beta`. Capture starts on slide 1. |

`snapshot-parse-failed` is a bad workbook snapshot. `press-library-unavailable` is the render library failing to load. Logout clears the selected cell, the selected sheet, and the slide index because the in-memory copy belongs to the host session.
