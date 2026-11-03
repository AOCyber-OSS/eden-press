# AGENTS.md — eden-press agent interface

`eden-press` is a pure-Go CLI: it converts a Marp-compatible Markdown deck to
static output with **no browser, no Node, and no JavaScript runtime**. This
document is the scriptable contract an AI agent (or any non-interactive
caller) needs to inspect, create, edit, and export documents without reading
the Go source.

**Source of truth for live documents is the docmodel** (`eden-press.model/v7`
JSON), not Markdown. Markdown remains the import/export prose format
(`doc import` / `doc export`, `--format html`/`pptx`/…). Agents should create
and edit via `eden-press doc` ops against JSON pages; do not rewrite Markdown
as the primary edit path. There is **no separate database product** — use
workbooks + `sheet` embed blocks. Design/layout tooling is **out of Press**;
use `figure` blocks for SVG/asset interchange only.

**Source of truth for live sheets is the workbook snapshot**
(`eden-press.workbook/v3` JSON). `.xlsx` is interchange only — relation refs
and typed columns survive in the JSON snapshot; xlsx export flattens
relations to display text (lossy).

The whole surface described here is CI-enforced chromedp-free:
`scripts/check-no-chromedp.sh` scans `./cmd/...`'s transitive dependency
closure (alongside `./press/...`, `./chase/...`, `./profiles/...`,
`./bind/...`) and fails the build if `chromedp` ever appears in it.

## Docmodel CRUD (`eden-press doc`)

Agent write surface (CLI first; an MCP server can wrap these later). Always
prints a JSON error envelope on stderr on failure (even without `--format json`).

| Command | Purpose |
|---|---|
| `doc create [--title T] [-o page.json]` | Empty single-section page JSON (`eden-press.model/v7`) |
| `doc get <page.json\|->` | Validate / pretty-print; assigns missing block/list-item ids |
| `doc apply <page.json\|-> --ops ops.json [--dry-run] [-o out.json]` | Apply ops (`--dry-run` validates without mutating) |
| `doc import <in.md\|-> [-o page.json]` | Markdown → docmodel JSON via `press.Render` (**always assigns block ids**; unchanged, and not lossless: it sanitizes and drops what the renderer drops. The lossless import is `press.MarkdownToDocJSON`, see Eden Markdown) |
| `doc export <page.json\|-> [-o out.md]` | Docmodel → Eden Markdown: **lossless and deterministic** (every kind, ids, comments and properties; see Eden Markdown) |

`apply` ops JSON may be a bare array or `{"ops":[...]}`.

### Block id policy

- Import (`doc import`) and `doc get` / `doc apply` always run `EnsureBlockIDs`:
  depth-first over section blocks and nested `children`, assigning `b_N` when
  `id` is missing. Existing ids are preserved.
- `Build`/raw Markdown render paths used by `--format html|json|pptx|xlsx`
  still omit ids; only the `doc` edit surface guarantees them.
- Agents should address blocks by `blockId` (and optional `parentBlockId` for
  nest insert/move).

### Document ops (`doc apply`)

| `op` | Required fields | Notes |
|---|---|---|
| `insert_block` | `sectionId` **or** `parentBlockId`, `block` | Optional `index` (0-based). Auto-assigns `block.id` when omitted |
| `update_block` | `blockId`, `block` | Full replace (keeps id). Synced: mutate the **source**, not the instance |
| `patch_block` | `blockId`, `block` | Merge fields/runs/checked/sheetEmbed; `clearChecked`/`clearCollapsed` clear bools |
| `delete_block` | `blockId` | Removes the block (and its children) |
| `move_block` | `blockId` | Optional `sectionId` / `parentBlockId` / `index`. Cycle into own descendant is rejected |
| `replace_text` | `old`, `new` | Optional `blockId`, `all`. Operates on plain `text` (runs re-flattened; marks may drop) |
| `insert_text` | `blockId`, `offset`, `text` | UTF-8 byte offset into plain text |
| `insert_section` | | Optional `index`, `attrs`, `notes`. Section ids are renumbered 1..N after structural ops |
| `delete_section` | `sectionId` | Cannot delete the last section |
| `move_section` | `sectionId`, `index` | Renumbers afterward |
| `set_section_attrs` | `sectionId` | `attrs` map (omit/empty clears) |
| `set_section_notes` | `sectionId` | `notes` string array |
| `set_meta_directives` | `directives` | Merges into `meta.directives` unless `replaceDirectives: true` |
| `set_properties` | `properties` | Page property schema; `replaceProperties` replaces entirely |
| `set_property_value` | `propertyId`, `value` | Sets one property value string |
| `add_comment` | `comment` | `{blockId, body, start?, stop?, suggestion?}`; assigns `c_N` id |
| `resolve_comment` | `commentId` | Marks resolved |
| `apply_suggestion` | `commentId` | Applies suggestion text/runs to the anchored block and resolves |

Editable block kinds: `paragraph`, `heading`, `list`, `code`, `math`, `table`,
`image`, `quote`, `toggle`, `synced`, `todo`, `callout`, `divider`, `file`,
`sheet`, `figure`.

Schema v7 additions:

- `runs[]` — rich text `{text, marks?, link?}`; plain `text` is the search/replace authority
- `todo` — `checked` bool + optional `children`
- `callout` — `text`/`runs` + optional `icon`/`tone`
- `divider` — thematic break
- `file` — attachment ref: `src`, `name?`, `mime?`, `size?` (bytes in Docs storage)
- `sheet` — embed via `sheetEmbed: {pageId?, workbookHint?, sheet, range?, snapshot?}`; snapshot is sparse workbook-shaped JSON for offline export
- `figure` — `src` and/or inline `svg` + `alt`/`title` (no design editor in Press)
- `properties` / `propertyValues` — typed page properties (`text|number|select|multi_select|date|checkbox|url|email|relation`)
- `comments[]` — review anchors + optional suggestions (data only)
- List items may carry `id` (`li_N`) and `runs`

Schema v6 (still valid): `children`, `toggle`, `synced` / `syncRef`. Cross-page
`pageId` resolution is a Docs concern.

```bash
# Create → edit → read (agent flow):
eden-press doc create --title "Q3" -o page.json
cat > ops.json <<'EOF'
[
  {"op":"insert_block","sectionId":1,"block":{"id":"b_title","kind":"heading","level":1,"text":"Q3 plan"}},
  {"op":"insert_block","sectionId":1,"block":{"id":"b_p","kind":"paragraph","text":"Ship the spike."}},
  {"op":"insert_block","sectionId":1,"block":{"id":"b_tog","kind":"toggle","text":"Details","children":[
    {"kind":"paragraph","text":"Nested body"}
  ]}}
]
EOF
eden-press doc apply page.json --ops ops.json -o page.json
eden-press doc get page.json | jq '.outline'
eden-press doc export page.json -o page.md

# Import existing Markdown into the docmodel (ids assigned):
eden-press doc import deck.md -o page.json
```

## Eden Markdown (lossless codec)

`doc export` (`press.ExportMarkdownJSON`, `model.ToMarkdown`) writes **Eden
Markdown**: Marp-compatible Markdown that carries the whole docmodel. It is
**lossless and deterministic**:

- For a document in canonical form (limits below) `FromMarkdown(ToMarkdown(d)) == d`,
  block, list-item and comment ids included (they ride in a trailing comment, or
  come from `Prev`).
- `ToMarkdown(d)` is byte-identical on every call (map keys are sorted), and
  `ToMarkdown(FromMarkdown(ToMarkdown(d))) == ToMarkdown(d)`.
- Reading never drops content silently. The strict reader refuses what the model
  cannot carry exactly (`*model.UnrepresentableError`); the lenient reader applies
  a fixed mapping.

This changed `doc export` output for the v7 kinds: before, a callout was a quote
with an emoji, a toggle was `<details>`, a sheet was a table of literal text, and
a synced block was replaced by its source text. A pure Marp renderer shows the
`:::kind` lines of the directives below as text; the HTML comments are invisible.

| Task | Use |
|---|---|
| Docmodel to Markdown | `eden-press doc export`, `press.ExportMarkdownJSON(doc)` |
| Markdown to docmodel, lossless | `press.MarkdownToDocJSON(md, prevDoc, press.MarkdownImport{Lenient, Spans})`; pass the stored document as `prevDoc` to keep ids and comment anchors across an edit |
| The Markdown an editor shows, and its outline with spans | `press.MarkdownSourceViewJSON(doc)`: `md` equals `ExportMarkdownJSON`; `md[o.Span.Start:o.Span.Stop]` is heading `o` (byte offsets; a UTF-16 client converts) |
| Will a stored document survive Markdown? | `press.VerifyMarkdownRoundTripJSON(doc)`: nil, or an error naming the first differing JSON path |

`doc import` still goes through `press.Render` (unchanged). Go callers that may
import `chase/model` use `model.FromMarkdown(md, model.FromMarkdownOptions{Prev, Spans, Lenient})`.

### Spellings, one per kind

| Kind | Eden Markdown |
|---|---|
| `paragraph` | text lines; a hard break is a backslash and a newline; a soft break (plain newline) reads as a space. Rich text: `**bold**`, `*italic*`, `~~strike~~`, `` `code` ``, `[text](href)`, `$tex$` (a run marked `math`). Where CommonMark's delimiter rules would not read `**`/`~~` back, the block uses `<strong>`, `<em>`, `<del>` (the only HTML read) |
| `heading` | `#` to `######` |
| `list` | `-` bullets or `1.` numbers, children indented by the parent's marker width; adjacent lists alternate `-`/`*` (or `.`/`)`) so they never merge; numbering must start at 1 |
| `code` | a fence one backtick longer than any backtick run inside; the language is the one info word; `Text` is exactly what is between the fences (a trailing newline shows as a blank line before the closing fence) |
| `math` | display: `$$`, the TeX, `$$` on their own lines; a paragraph of only `$tex$` is a non-display math block |
| `table` | GFM pipe table; delimiter row `---`, `:---`, `:---:`, `---:`; cells are plain text, `\|` escaped |
| `image` | `![alt](src "title")`, `<src>` when it has spaces; a title may span lines |
| `quote` | `> ` lines: its leading paragraphs are `Text`, the rest are `Children` |
| `divider` | `***` (a `---` rule is a section break, never a divider) |
| `todo` | `- [ ] text` or `- [x] text`; children indented under it; consecutive childless todos are one tight list |
| `toggle`, `callout`, `file`, `sheet`, `figure`, `synced` | container directives, below |

**Directives.** `:::kind {json}` on one line, a body, then `:::`. The fence is three
or more colons at the start of a line; the closer is colons only and at least as
long as the opener; `ToMarkdown` makes the fence one colon longer than any colon run
at the start of a body line, so directives nest like code fences (same-length
fences do not nest). The header is one JSON object from `json.Marshal` with the
fields `icon`, `tone`, `collapsed`, `name`, `mime`, `size`, `src`, `title`, `alt`
(a figure's text), `sheetEmbed`, `syncRef`, `noText`, in that order, all omitted
when empty, with `<`, `>`, `&` escaped. The header may be left out when every field
is empty. The body is the block's text paragraph (rich text allowed) and then its
children as Markdown; `"noText":true` says the first body paragraph is a child, not
the text. A `figure`'s body is its SVG verbatim. Fields by kind: toggle `collapsed`;
callout `icon`, `tone`; file `name`, `mime`, `size`, `src`; sheet `sheetEmbed`
(snapshot, formulas and rels included); figure `src`, `title`, `alt`; synced
`syncRef` and no body. A line of prose that starts with `:::` is written `\:::`.

**Section layer.**
- Sections are separated by a `---` rule; an empty section is just its rule.
- Front matter (`Meta.Directives`): `---`, sorted `key: value` lines, `---`. Keys match
  `[A-Za-z0-9_][A-Za-z0-9_.-]*`; a value is plain, `"double"` (Go escapes) or `'single'`,
  and is quoted when it holds `:`, `#` or another YAML special.
- Section attrs: one comment first in the section, `<!-- eden:attrs {"k":"v"} -->`
  (a JSON object of strings, keys sorted).
- Section notes: after the blocks, one comment each in order,
  `<!-- eden:note "json string" -->`.
- Marp input still reads: a top-level comment whose keys are all known directives
  (global, local or `_`-prefixed spot) is attrs (a later attr of the same key in a
  section wins); any other whole top-level comment is a note.
- **The trailer.** The last thing in the file, preceded by a blank line, when the
  document has ids, properties, property values or comments:
  `<!-- eden:model {"v":1,"blocks":[...],"properties":[...],"propertyValues":{...},"comments":[...]} -->`.
  `blocks` lists every block depth first as `{"id","k","f"[,"li"]}` with a content
  fingerprint (`f`, FNV-1a, 8 hex digits) so ids survive an edit of the Markdown
  even with no `Prev`; `properties`, `propertyValues` and `comments` are the model's
  JSON. Keep it in the editor's source view, or re-attach it before reading, or the
  ids, properties and comments are lost. A document with none of that has no trailer.

**Ids across an edit.** With `Prev` (else the trailer) the reader matches blocks in
this order: identical blocks in order, then moved blocks, then blocks edited in place
between two matched neighbours; the rest are new and get ids above every id ever
used, so a deleted block's id (still named by a comment) is never reused. Comments
are never dropped or re-anchored.

### What strict mode refuses (`UnrepresentableError.Construct`)

`Offset` is the byte offset of the construct. Lenient mode never fails and applies
the mapping in the last column.

| Construct | Strict refuses | Lenient mapping |
|---|---|---|
| `raw-html` | an HTML block, inline HTML other than `<strong> <em> <del>`, or a comment that is not a whole top-level comment | a block becomes a paragraph of the literal source; inline HTML becomes literal text |
| `table-cell-formatting` | a cell holding emphasis, code, a link, an image or math | the cell is flattened to plain text |
| `list-start` | an ordered list that does not start at 1 | the start is reset to 1 |
| `list-mixed-type` | a nested list of the other type | the outer list's type wins |
| `list-mixed-todo` | a task item in a list with plain items, nested in a plain list, or in an ordered list | the item stays a list item with `[ ] ` / `[x] ` as text |
| `list-item-blocks` | a list item with a second paragraph, code, a quote or a table | the extra blocks' text is appended after a line break |
| `link-title` | `[a](u "t")` | the title is dropped |
| `empty-link` | `[](u)` or `[a]()` | an empty-text link is dropped; no destination is plain text |
| `inline-image` | an image sharing a paragraph with other content | the image becomes its alt text |
| `code-info-string` | a fence info string of more than one word | the first word is the language |
| `inline-display-math` | `$$x$$` sharing a paragraph with other content | inline math |
| `math-unclosed` | a `$$` block with no closing `$$` | the math runs to the end of its container |
| `math-trailing-text` | text after the closing `$$` on its line | the text is the paragraph after the math block |
| `front-matter` | a leading `---` block with a line that is not `key: value`, a bad quoted value or a duplicate key | bad lines are skipped, a later duplicate wins |
| `unknown-directive` | `:::name` other than the six | the whole directive, fences included, as literal paragraphs |
| `directive-json` | a bad directive header (bad JSON, an unknown key, a wrong type, trailing text), a bad `eden:attrs`/`eden:note`/`eden:model` payload, an unsupported trailer version, a second trailer | a directive stays literal text; a comment stays a note |
| `directive-unclosed` | a directive with no closing fence before the end of its container | it runs to the end of its container |
| `unsupported` | a Markdown node this codec does not read | skipped |

Read without a refusal and without a trace (they carry no content): the bullet
character, the ordered delimiter, list tightness and numbering, `*` versus `_`
emphasis, ATX versus setext headings, fence character and length, indented versus
fenced code, reference versus inline links, `<url>` autolinks, character references,
backslash escapes, an unused `[x]: url` definition, and a soft line break (a
space). Footnotes are not a feature of the model: `[^1]: Some text` stays as a
paragraph, but a one-word `[^1]: note` is a link reference definition, so it
vanishes when nothing uses it and turns a `[^1]` into a link to `note` when
something does.

### Canonical form: what the guarantee covers

`ToMarkdown` still writes something sensible for anything below, but the reader may
not give the same document back. `press.VerifyMarkdownRoundTripJSON` reports such a
document by path.

- a document with no section (use one empty section); section ids that are not 1..n;
- a text that starts or ends with a newline, or holds a blank line (a quote's
  paragraphs are the exception);
- a marked or linked run with whitespace at its edge, or two neighbouring runs with the
  same marks and link; marks other than `bold`, `italic`, `strike`, `code`, `math` (not
  written); a `math` run followed by a digit;
- a paragraph that is only one image or only one math run (those are `image` and
  `math` blocks);
- display math TeX holding an unescaped `$$`, and inline math TeX holding an unescaped
  `$`, edge whitespace or a newline (TeX reserves `$`; write `\$`);
- a list that skips a level or starts above level 0;
- a table row with more or fewer cells than the header;
- children on a block other than `quote`, `toggle`, `todo` and `callout` (dropped);
  a quote whose first child is a paragraph (it merges into the quote's text);
- a whitespace-only line in code, math or an SVG nested in a todo, and a todo whose
  child list ends in an empty item while more children follow (goldmark closes the
  todo there);
- invalid UTF-8 (JSON replaces it).

Ids, spans: spans are never part of identity; they depend on the string they index.

## Workbook CRUD (`eden-press workbook`)

| Command | Purpose |
|---|---|
| `workbook create [--title T] [-o book.json]` | Empty workbook with `Sheet1` (`eden-press.workbook/v3`) |
| `workbook get <book.json\|->` | Validate / pretty-print; ensures row ids on occupied rows |
| `workbook apply <book.json\|-> --ops ops.json [-o out.json]` | Apply workbook ops |
| `workbook import <book.xlsx\|book.json\|-> [-o book.json]` | `.xlsx` or JSON → v3 snapshot |
| `workbook export <book.json\|-> [--format xlsx\|json] [-o out]` | Snapshot → `.xlsx` (default) or JSON |

### Workbook snapshot v3

Writers emit `eden-press.workbook/v3`. Readers still accept v2 and v1.

- Occupied cells: `{ "r", "c", "v" }` or `{ "r", "c", "f" }` (formula without `=`)
- Optional `rels`: list of `{ "rowId", "pageId?", "sheet?" }` on a cell
- `headerRows`: header-row indexes; `aligns`: per-column alignment
- `columns[]`: `{ "id", "name", "type" }` where `type` is
  `text` \| `number` \| `formula` \| `relation`
- `rowIds[]`: `{ "r", "id" }` stable row ids (`r_N` when auto-assigned)

JSON is authoritative for relations. `.xlsx` export does not preserve
structured relation refs.

### Workbook ops (`workbook apply`)

| `op` | Required fields | Notes |
|---|---|---|
| `set_cell` | `sheet`, `row`, `column`, `value` | Formulas keep leading `=` |
| `add_sheet` | `sheet` | Append worksheet |
| `rename_sheet` | `sheet`, `newName` | |
| `delete_sheet` | `sheet` | Cannot delete the last sheet |
| `set_header_row` | `sheet`, `row`, `header` | |
| `set_align` | `sheet`, `column`, `align` | `left`/`right`/`center`/`` |
| `set_column` | `sheet`, `columnId`, `name`, `type` | Upsert typed property; optional `options` for select types. Types: `text\|number\|formula\|relation\|select\|multi_select\|date\|checkbox\|url\|email` |
| `set_named_range` | `rangeId`, `name`, `sheet`, `r0`..`c1` | Stable named rectangle for sheet embeds |
| `set_relation` | `sheet`, `rels` | Address with `rowId` or `row`, plus `columnId` or `column`. Same-workbook target `rowId` must exist when `pageId`/`sheet` omitted |
| `clear_relation` | `sheet` | Same addressing as `set_relation` |
| `ensure_row_id` | `sheet`, `row` | Optional `rowId`; no-op if already set |

```bash
eden-press workbook create --title "Tasks" -o book.json
cat > wb-ops.json <<'EOF'
[
  {"op":"set_cell","sheet":"Sheet1","row":0,"column":0,"value":"Alpha"},
  {"op":"set_cell","sheet":"Sheet1","row":1,"column":0,"value":"Beta"},
  {"op":"ensure_row_id","sheet":"Sheet1","row":0,"rowId":"r_a"},
  {"op":"ensure_row_id","sheet":"Sheet1","row":1,"rowId":"r_b"},
  {"op":"set_column","sheet":"Sheet1","columnId":"rel","name":"Related","type":"relation"},
  {"op":"set_relation","sheet":"Sheet1","rowId":"r_a","columnId":"rel","rels":[{"rowId":"r_b"}]}
]
EOF
eden-press workbook apply book.json --ops wb-ops.json -o book.json
eden-press workbook export book.json -o book.xlsx --format xlsx
```

Flutter mirrors: `EdenEdit` subclasses (`EdenCellEdit`, `EdenSheetAdded`,
`EdenSheetRenamed`, `EdenSheetDeleted`, `EdenHeaderRowEdit`, `EdenAlignEdit`,
`EdenColumnEdit`, `EdenRelationEdit`, `EdenDocOpsEdit`) expose `toWire()` /
`toWireOps()` matching the tables above.

## Docs rendering modes (Flutter vs HTML)

Clients choose how a document is painted:

| Client | Request | Response | Renderer |
|---|---|---|---|
| Flutter app | `--format json`, `serve ?format=json`, or `Accept: application/json` | lowercase JSON envelope (`model` = `eden-press.model/v7`) | [`bind/eden_docs`](bind/eden_docs) package (`EdenDocsView`) |
| Browser / HTML host | `--format html` (default), `serve ?format=html`, or `Accept: text/html` | standalone zero-JS HTML | Go `model.ToHTML` + DocsCSS (`doc export --format html`) |

Do **not** dump JSON into a Flutter `Text`. Include `eden_docs` in the host
app and pass the decoded `ModelDocument`. Slides still use `eden_press`'s
`EdenPressView` (FitText); Docs use `eden_docs`.

## Formats (`--format`)

`--format` is a persistent flag on every convert invocation (the bare
`eden-press <in.md>` default, and the explicit `convert` subcommand). It
selects what `emitFormat` emits; `--output`/`-o` (registered on `convert`
and `watch`) selects the destination — a file path, or stdout when unset.

| `--format` | Output | Notes |
|---|---|---|
| `html` (default) | Standalone, zero-JS `<!doctype html>` document | ALWAYS zero `<script>` tags — there is no browser-side auto-fit splice option. `<!--fit-->`/shrink markers (`data-auto-scaling="fit"`, `marp-fit-shrink`) are still emitted into the HTML but are inert on this path: auto-fit is Flutter-only, consumed by the Dart binding's native TextPainter fit |
| `json` | The full render `press.Output`, as a lowercase JSON envelope (below) | For programmatic inspection — no browser needed |
| `pptx` | An editable `.pptx` (OOXML zip), built directly from the docmodel | Binary — **always use `-o out.pptx`**, never rely on stdout in a script that also wants to read text |
| `xlsx` | An editable `.xlsx` workbook (OOXML zip). Markdown tables become worksheets; a document with no tables becomes one prose sheet | Binary — **always use `-o out.xlsx`**. This is the Sheets interchange file. Building and editing a workbook in-process uses `convert/xlsx.Workbook` (`AddSheet`, `Set`, `Open`, `MarshalJSON`); `--format xlsx` is the file export. The file stores occupied cells only |

```bash
# Inspect a deck's structure (defaults to stdout; pipe into jq/etc.):
eden-press deck.md --format json | jq '.model.outline'

# Export an editable PowerPoint deck, zero Chrome:
eden-press deck.md -o slides.pptx --format pptx

# Export an editable workbook, zero Chrome:
eden-press deck.md -o book.xlsx --format xlsx

# Export via the explicit "convert" subcommand (identical to the bare default):
eden-press convert deck.md --output slides.pptx --format pptx
```

## Workbook snapshot (convert path)

`Workbook.MarshalJSON` (and the Flutter `EdenWorkbook.toJson`) writes
`eden-press.workbook/v3`. Each occupied cell is `{ "r", "c", "v" }` or
`{ "r", "c", "f" }` with 0-based coordinates. `f` is the formula without
`=`. `headerRows` is a list of header-row indexes. `aligns` stays one entry
per column. Blank cells are omitted, so a value on the last Excel row does
not expand the snapshot to the whole grid. v3 adds `columns`, `rowIds`, and
optional cell `rels`.

`ParseWorkbook` and `EdenWorkbook.fromJson` still read
`eden-press.workbook/v2` and `eden-press.workbook/v1`. In v1,
`sheets[].rows[].cells[]` is rectangular and the array position is the
coordinate. New snapshots are v3.

PNG/PDF export is intentionally **not** part of this binary (it requires
Chrome); that lives in the separate `eden-press-export` binary — see
"Export (separate binary)" below. `preview`/`watch`/`serve` behavior is
unaffected by `--format` beyond what's documented above.

## Export (separate binary)

Raster (PDF/PNG) export lives in `cmd/eden-press-export` — a **SEPARATE,
Chrome-requiring binary**, not a flag on `eden-press`. The core `eden-press`
binary documented above stays browserless (`html`/`json`/`pptx`/`xlsx`) always;
raster export is an opt-in binary you install/build separately. This split
is CI-enforced: `scripts/check-no-chromedp.sh` proves `eden-press`'s own
dependency closure never contains `chromedp`, and separately proves
`eden-press-export` is the ONE `cmd/...` binary whose closure does.

```bash
# PDF: one file, written to -o (defaults to <stem>.pdf for a file input
# with no -o; stdin input requires -o -- there is no filename to derive a
# default from):
eden-press-export deck.md -o out.pdf --format pdf

# PNG: one file per slide, written into the -o DIRECTORY (created if
# missing; "" -> CWD) as <stem>-001.png, <stem>-002.png, ...; -o may also be
# a printf `%03d`-style pattern (e.g. -o "frame-%03d.png") for full control
# over the per-slide filename:
eden-press-export deck.md -o out-dir/ --format png
```

**Chrome discovery** (in order): `--browser-path <path>` > the `CHROME_PATH`
environment variable > PATH auto-detection of a known Chrome/Chromium
binary. If none resolves, the command exits **3** with a remedy message
naming both `--browser-path` and `CHROME_PATH` — see
[`convert/EXPORT.md`](convert/EXPORT.md) for the full discovery chain and
the pinned-`chromedp/headless-shell` fallback.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Render/runtime error (bad input path, render failure, Chrome launch failure, write failure) |
| `2` | Usage/flag error (unknown `--format` value, PDF export from stdin with no `-o`, bad/unregistered flag) |
| `3` | No Chrome/Chromium discoverable — supply `--browser-path <path>` or set `CHROME_PATH`, or install a pinned `chromedp/headless-shell` |

```bash
# Copy-paste examples:
eden-press-export deck.md -o out.pdf --format pdf && echo ok || echo "failed: exit $?"
eden-press-export deck.md -o slides/ --format png && echo ok || echo "failed: exit $?"

# No Chrome available -- exits 3 with a remedy, never a stack trace:
eden-press-export deck.md -o out.pdf --format pdf
# stderr: no Chrome/Chromium found for raster export: ...
#   supply one with --browser-path <path> or the CHROME_PATH env var,
#   or install a pinned chromedp/headless-shell (see convert/EXPORT.md)
# exit code: 3
```

## JSON envelope schema (`--format json`)

```json
{
  "html": "<section>...</section>",
  "css": "/* packed theme CSS */",
  "model": {
    "meta": { "directives": { "theme": "default", "marp": "true" } },
    "sections": [
      {
        "id": 1,
        "attrs": { "class": "lead" },
        "notes": ["speaker note text"],
        "blocks": [
          { "kind": "heading", "text": "Title", "level": 1 },
          { "kind": "paragraph", "text": "Body text." },
          { "kind": "code", "text": "fmt.Println(1)", "language": "go" },
          { "kind": "math", "text": "E=mc^2", "display": true },
          { "kind": "list", "ordered": false, "items": [
            { "text": "item one", "level": 0 }
          ] },
          { "kind": "table",
            "headers": ["Metric", "Q3"],
            "rows": [["p95 latency", "550ms"]],
            "aligns": ["left", "right"] },
          { "kind": "image", "src": "chart.png", "text": "alt text", "title": "Quarterly" },
          { "kind": "quote", "text": "a blockquote" },
          { "kind": "toggle", "text": "Details", "collapsed": false, "children": [
            { "kind": "paragraph", "text": "Nested" }
          ] },
          { "kind": "synced", "syncRef": { "blockId": "b_src" } }
        ]
      }
    ],
    "outline": [
      { "sectionId": 1, "level": 1, "text": "Title", "slug": "title" }
    ]
  },
  "comments": ["speaker note text"],
  "meta": { "directives": { "theme": "default", "marp": "true" } }
}
```

Field notes an agent must get right:

- **A code block's raw source, and a math block's raw TeX, both live in
  `block.text`** — there are **no** `source` or `tex` keys. `language`
  accompanies a `code` block (e.g. `"go"`, `""` for an unlabeled fence);
  `display` accompanies a `math` block (`true` for `$$...$$`, `false` for
  `$...$`).
- `meta` (top-level) is a convenience alias identical to `model.meta` — read
  either.
- Every field is `omitempty`: absent when unused (e.g. a paragraph block has
  no `language`/`display`/`items`; a section with no speaker notes omits
  `notes` entirely). Do not assume a key is always present — check for it.
- **A table block's payload lives in `headers` + `rows` + `aligns`**, never in
  `text`. `rows` is an array of row arrays of cell strings; `aligns` is one
  entry per column, each `"left"`/`"right"`/`"center"` or `""` when the column
  declared no alignment. Ragged rows are reported as authored — GFM pads at
  render time, but the model reports what was written. An **image** block puts
  its destination in `src`, its alt text in `text`, and its optional
  `![alt](src "title")` third argument in `title`. A **quote** block carries its
  concatenated text in `text` (multi-paragraph quotes are joined by a blank
  line; nested quotes are flattened in document order).
- `model` is exactly `chase/model.Document`'s own schema-v7 JSON shape,
  reused verbatim; `cmd/eden-press` itself never imports that package
  directly (it re-marshals the already-typed value through `any`), but the
  wire shape is stable and versioned independently of Marp/goldmark churn.
- Schema v7 adds rich `runs`, todo/callout/divider/file/sheet/figure,
  page properties, and comments. Convert/`Build` may omit `id`/`runs`;
  `doc import`/`apply`/`get` always ensure ids.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Render/runtime error (bad input path, render failure, JSON marshal failure, pptx export failure) |
| `2` | Usage/flag error (unknown `--format` value, bad/unregistered flag) |

## Error envelope (`--format json`, on failure)

When `--format json` is active, any failure prints a JSON object to
**stderr** (never stdout) instead of a plain-text message, and the process
exits with the matching code above:

```json
{ "error": { "code": 1, "message": "resolveInput: read file \"deck.md\": open deck.md: no such file or directory" } }
```

For `--format html`/`--format pptx`/`--format xlsx`, a failure prints the same message as
plain text to stderr (no JSON wrapping) with the same exit-code contract.
`doc` / `workbook` subcommands always use the JSON error envelope on stderr.

## Copy-paste examples

```bash
# Pull just the outline (table of contents) as JSON:
eden-press deck.md --format json | jq '.model.outline'

# Export an editable pptx and check the exit code:
eden-press deck.md -o slides.pptx --format pptx && echo ok || echo "failed: exit $?"

# Script-friendly failure handling — write JSON to a file, check exit status:
eden-press deck.md --format json > out.json || echo "failed: exit $?"

# A bad path fails fast with a parseable error, never a stack trace:
eden-press /no/such.md --format json
# stderr: {"error":{"code":1,"message":"resolveInput: ..."}}
# exit code: 1

# Doc import → ops → Markdown export round-trip:
eden-press doc import deck.md -o page.json
eden-press doc apply page.json --ops ops.json -o page.json
eden-press doc export page.json -o out.md

# Workbook create → ops → xlsx export:
eden-press workbook create -o book.json
eden-press workbook apply book.json --ops wb-ops.json -o book.json
eden-press workbook export book.json -o book.xlsx
```

No browser, no Node, no JavaScript runtime is ever invoked by any of the
above — `html` and `json` are pure `press/`, and `pptx` / `xlsx` are pure stdlib OOXML
(`convert/pptx`, `convert/xlsx`). This is a CI-enforced invariant
(`scripts/check-no-chromedp.sh` covers `./cmd/...`), not just a convention.
