# AGENTS.md — eden-press agent interface

`eden-press` is a pure-Go CLI: it converts a Marp-compatible Markdown deck to
static output with **no browser, no Node, and no JavaScript runtime**. This
document is the scriptable contract an AI agent (or any non-interactive
caller) needs to inspect, create, edit, and export documents without reading
the Go source.

**Source of truth for live documents is the docmodel** (`eden-press.model/v5`
JSON), not Markdown. Markdown remains the import/export prose format
(`doc import`, `--format html`/`pptx`/…). Agents should create and edit via
`eden-press doc` ops against JSON pages; do not rewrite Markdown as the
primary edit path.

The whole surface described here is CI-enforced chromedp-free:
`scripts/check-no-chromedp.sh` scans `./cmd/...`'s transitive dependency
closure (alongside `./press/...`, `./chase/...`, `./profiles/...`,
`./bind/...`) and fails the build if `chromedp` ever appears in it.

## Docmodel CRUD (`eden-press doc`)

Agent write surface (CLI first; an MCP server can wrap these later). Always
prints a JSON error envelope on stderr on failure (even without `--format json`).

| Command | Purpose |
|---|---|
| `doc create [--title T] [-o page.json]` | Empty single-section page JSON |
| `doc get <page.json\|->` | Validate / pretty-print page JSON |
| `doc apply <page.json\|-> --ops ops.json [-o out.json]` | Apply insert/update/delete block ops |
| `doc import <in.md\|-> [-o page.json]` | Markdown → docmodel JSON via `press.Render` |

`apply` ops JSON may be a bare array or `{"ops":[...]}`. Spike kinds:
`paragraph` and `heading` only. Each block may carry a stable string `id`
(auto-assigned as `b_N` when omitted on insert).

```bash
# Create → edit → read (agent flow):
eden-press doc create --title "Q3" -o page.json
cat > ops.json <<'EOF'
[
  {"op":"insert_block","sectionId":1,"block":{"id":"b_title","kind":"heading","level":1,"text":"Q3 plan"}},
  {"op":"insert_block","sectionId":1,"block":{"id":"b_p","kind":"paragraph","text":"Ship the spike."}}
]
EOF
eden-press doc apply page.json --ops ops.json -o page.json
eden-press doc get page.json | jq '.outline'

# Import existing Markdown into the docmodel:
eden-press doc import deck.md -o page.json
```

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

## Workbook snapshot

`Workbook.MarshalJSON` (and the Flutter `EdenWorkbook.toJson`) writes
`eden-press.workbook/v2`. Each occupied cell is `{ "r", "c", "v" }` or
`{ "r", "c", "f" }` with 0-based coordinates. `f` is the formula without
`=`. `headerRows` is a list of header-row indexes. `aligns` stays one entry
per column. Blank cells are omitted, so a value on the last Excel row does
not expand the snapshot to the whole grid.

`ParseWorkbook` and `EdenWorkbook.fromJson` still read
`eden-press.workbook/v1`. In v1, `sheets[].rows[].cells[]` is rectangular
and the array position is the coordinate. New snapshots are v2.

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
          { "kind": "quote", "text": "a blockquote" }
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
- `model` is exactly `chase/model.Document`'s own schema-v5 JSON shape,
  reused verbatim; `cmd/eden-press` itself never imports that package
  directly (it re-marshals the already-typed value through `any`), but the
  wire shape is stable and versioned independently of Marp/goldmark churn.
- Schema v5 adds optional `block.id` (string) for `doc apply` addressing.
  `Build`/Markdown import leaves `id` absent; agents assign ids on create.

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
```

No browser, no Node, no JavaScript runtime is ever invoked by any of the
above — `html` and `json` are pure `press/`, and `pptx` / `xlsx` are pure stdlib OOXML
(`convert/pptx`, `convert/xlsx`). This is a CI-enforced invariant
(`scripts/check-no-chromedp.sh` covers `./cmd/...`), not just a convention.
