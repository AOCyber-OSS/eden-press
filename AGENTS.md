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
| `doc import <in.md\|-> [-o page.json]` | Markdown → docmodel JSON via `press.Render` (**always assigns block ids**) |
| `doc export <page.json\|-> [-o out.md]` | Docmodel → Marp-compatible Markdown (nested/sheet/figure best-effort; some features lossy) |

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
