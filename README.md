# Eden Press

Eden Press is a **Go (+ Dart/Flutter client) framework and CLI for generating documents
from Markdown**, Markdown-compatible with [Marp](https://marp.app) but implemented
clean-room with **zero JavaScript in the backend**. It is a developer-facing library first
(`press.Render(md)` embedded in any Go service), a CLI second (`eden-press`), and part of
the **Eden open-source platform**. The v1 target is Marp-compatible slide decks; the
architecture is deliberately format-agnostic (an output-**profile** abstraction) so paged
documents/reports, single-page articles, and EPUB can follow as additional profiles without
a rewrite. See [`PROPOSAL.md`](./PROPOSAL.md) for the full architecture and design source.

> **Core value:** render Marp-compatible documents from Markdown inside a Go service or CLI
> with no JavaScript runtime, no Node, and no browser for HTML/structured output — while
> emitting the document as structured data, not just HTML.

## Status

**v1 complete.** All 9 roadmap objectives (0–8) plus two insertions (4.1 CLI Agent
Interface, 5.1 Export Binary) are built and verified. `go build ./...` and `go test ./...`
are green.

Shipped:

| Piece | What it is |
|---|---|
| `press.Render` | Chrome-free rendering to HTML + CSS + a structured document model |
| `chase/` | The engine: Marpit/Marp-Core semantics in Go — directives, slide split, theme scoping, containers |
| `chase/model` | The versioned document IR (`eden-press.model/v3`) |
| `profiles/slides`, `profiles/paged` | Two output profiles behind the `chase/profile` interface |
| `eden-press` | CLI: `convert`, `watch`, `serve`, `preview` — CI-proven chromedp-free |
| `eden-press-export` | Separate binary, the only one permitted to touch Chrome (PDF/PNG) |
| `convert/pptx`, `convert/docx`, `convert/xlsx` | Native OOXML writers, hand-rolled — no Chrome, no LibreOffice |
| `bind/` | Dart/Flutter binding: C-ABI (`.so`/`.a`) + WASM, with a JS-free native Flutter surface |
| `conformance/` | Golden corpus + CommonMark/GFM spec sweeps, with HTML-DOM and CSS-AST diffing |

Not built: the `article` and `epub` profiles named as future work in `PROPOSAL.md`.

## Quick start

**As a library:**

```go
out, err := press.Render(markdown, press.Options{})
// out.HTML   — rendered deck HTML (zero JavaScript)
// out.CSS    — packed theme CSS
// out.Model  — *model.Document, the structured IR
// out.Meta   — deck-level front-matter
```

**As a CLI:**

```bash
eden-press convert deck.md -o deck.html
eden-press convert deck.md --format json      # the document model, for programmatic use
eden-press convert deck.md --format pptx      # native OOXML, no Chrome
eden-press watch deck.md                      # rebuild on change
eden-press preview deck.md                    # local preview server
eden-press-export deck.md -o deck.pdf         # PDF/PNG (this binary uses Chrome)
```

## Architecture notes

**The document model is a first-class output, not a by-product.** `chase/model.Document` is
a versioned, JSON-serializable IR: `Sections` of typed `Block`s
(`paragraph|list|code|math|heading|table|image|quote`) with stable IDs and an outline. It is
built by a **read-only walk of the same finalized AST the HTML renderer consumes** — never a
second parse, never reverse-engineered from rendered HTML — and that invariant is enforced by
a test that counts parses through a seam. Raw code is retained pre-highlighting and raw TeX
pre-MathML, because neither is recoverable from the rendered form.

**The Flutter client renders from the model, not from HTML.** `bind/dart` builds a native
widget tree from `Output.Model`: math via `flutter_math_fork` from raw TeX, code via
`flutter_highlighting` from raw source, headings auto-fit via native `TextPainter`
measurement. No DOM, no embedded browser, no JavaScript.

**Chrome is quarantined by CI, not by convention.** `make check-no-chromedp` proves `press/`,
`chase/`, `profiles/` and `bind/` carry no chromedp import; `make check-cli-imports` proves
the CLI depends only on `press/`. Chrome lives behind `eden-press-export` alone, against a
pinned version.

## Testing

```bash
make test               # go test ./...
make check-no-chromedp  # architectural gate: engine is Chrome-free
make check-cli-imports  # architectural gate: CLI depends only on press/
make export-test        # Chrome-dependent export tests (self-skip without a browser)
```

Fidelity is held by a golden conformance corpus under `conformance/corpus/cases/` (each case
an `input.md` + `options.json` + `expected.html`, originally extracted from an npm Marp
oracle), plus full CommonMark and GFM spec sweeps. Diffs are structural: `htmldiff` compares
normalized DOM, `cssdiff` compares a parsed CSS AST rather than text.

## Acknowledgments

Eden Press is inspired by and Markdown-compatible with [Marp](https://marp.app)
(Marpit, Marp Core, Marp CLI), implemented clean-room in Go with zero JavaScript in the
backend. Eden Press is **not affiliated with, endorsed by, or sponsored by the Marp team**.
See [NOTICE](./NOTICE) for full third-party credits.

The vendored `internal/latex2mathml` fork (via a `go.mod` replace) carries its own upstream
license and copyright; see [NOTICE](./NOTICE) and per-file headers.

## License

[MIT](./LICENSE) — Copyright (c) 2026 AO Cyber Systems. Verbatim-reused third-party assets
retain their original license and copyright; see [NOTICE](./NOTICE) and per-file headers.
