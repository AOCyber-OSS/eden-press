# Pattern mapping — eden-editor

Donor: the embeddable `EdenView` already in `bind/eden_ui` (document list, slide `PageView`, spreadsheet grid). There is no external mockup. This page is the redesign map the Surface Spec is written against.

| Donor | Eden pattern | What stays | What changes |
|---|---|---|---|
| `SpreadsheetView` formula `TextField`, key `eden-formula-bar` | `form/input` | Key `eden-formula-bar`. Raw cell text, including a leading `=`. Commit still emits `EdenCellEdit`. | Semantics identifier `sheet.formula`. `EdenInput`, not a raw `TextField`. Address in JetBrains Mono. No hint used as the label. |
| Sheet-name `TextButton`s | `navigation/tabs` | Names, order, and the rule that the last sheet cannot disappear by itself. | Semantics identifier `sheet.tab`. Selected tab uses `colorScheme.primary`, not `#0B57D0`. The strip scrolls on its own axis. |
| Add-sheet `IconButton`, key `eden-add-sheet`, tooltip `Add sheet` | `feedback/empty-state` on an empty book; `navigation/icon-button` otherwise | Key `eden-add-sheet`. Tooltip string `Add sheet`. `EdenSheetAdded` with `freshSheetName()`. | Semantics identifier `sheet.add`. 48dp target. On an empty book the same control is the empty-state action labeled `Add a sheet`, and the tab strip is absent. |
| Cell `GestureDetector` in a nested scroll view | `data/grid` | Sparse cells, header-row weight, formula result via `displayCell`, minimum grid on a non-empty sheet. | Semantics identifier `sheet.grid`. Hairline `outline` borders. Selection ring from `colorScheme.primary`. Cell text ellipsizes. The grid scrolls inside itself. Narrow rows grow to 48dp. |
| `_DocumentView` `ListView` of `EdenPressView` | `content/document-viewer` | One section after another. Host still owns the bytes. No rich-text edit event in this surface. | Measure capped at 640. Tokens for padding. Empty copy is a state of this surface, not a blank list. |
| `_SlidesView` `PageView`, key `eden-slides`, label `Slide n of N` | `navigation/pager` | Key `eden-slides`. Label wording `Slide n of N`. One slide visible at a time. | Semantics identifiers `slides.previous` and `slides.next`. Keyboard as well as drag. Disabled on the ends, with the reason shown. Reduced motion drops the page transition. |
| Hardcoded fills `#F7F7F7`, `#F0F0F0`, `#0B57D0`, `Colors.white` | `EdenTheme` gold | None of the hex values. | `EdenTheme.light()` / `EdenTheme.dark()`. One accent, the gold preset. |
