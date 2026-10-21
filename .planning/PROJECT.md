---
kind: library
github_repo: AOCyberAI/eden-press
org_project: PVT_kwDOFEhtQc4BnqOy
---

# Eden Press

## What This Is

Eden Press is a **Go (+ Dart/Flutter client) framework and CLI for generating documents from Markdown**, Markdown-compatible with [Marp](https://marp.app) but implemented clean-room with **zero JavaScript in the backend**. It is a developer-facing library first (`press.Render(md)` embedded in any Go service), a CLI second (`eden-press`), and part of the **Eden open-source platform** (`eden-platform`, `eden-ui`, …). It is distinct from **Eden Docs** (the end-user Collabora-fork productivity suite) — Eden Press is a library/tool, not an editor.

The v1 target is **Marp-compatible slide decks**; the architecture is deliberately format-agnostic (an output-**profile** abstraction) so paged documents/reports, single-page articles, and EPUB can follow as additional profiles without a rewrite. The full architecture review, JS-free stack decisions, two completed feasibility spikes, and the phasing this roadmap is derived from live in **`PROPOSAL.md`** at the repo root — read it as the primary design source.

## Core Value

**Render Marp-compatible documents from Markdown inside a Go service or CLI with no JavaScript runtime, no Node, and no browser for HTML/structured output** — while emitting the document as structured data, not just HTML.

## Requirements

### Validated

(None yet — greenfield; ship to validate.)

### Active

<!-- v1 hypotheses. Detailed REQ-IDs live in REQUIREMENTS.md. -->

- [ ] Parse Marp/Marpit Markdown (directives: global/local/spot; front-matter + HTML-comment syntax) into a document model
- [ ] Slide splitting (`---`, `headingDivider`), `.marpit`/`section` containers, inline-SVG mode, background-image syntax
- [ ] Theme-CSS engine: parse `/* @theme */`/`@size`/`@auto-scaling`, scope selectors, pagination, advanced backgrounds (native Go CSS, no PostCSS)
- [ ] Batteries: the 3 bundled themes (verbatim, credited), GFM + heading slugs, allow-list sanitization, native emoji, `chroma` highlighting, `latex2mathml`→MathML math (SVG/PNG fallback), fit markers
- [ ] Importable Go API (`press.Render`) with **structured document-model output** (JSON AST/outline/notes/metadata) alongside HTML+CSS
- [ ] Output-**profile** abstraction (slides profile in v1; extensible to paged/article/EPUB)
- [ ] CLI (`eden-press`): convert to HTML; watch; serve+live-reload; preview
- [ ] Raster export via headless Chrome (`chromedp`): PDF, PNG/JPEG
- [ ] PPTX export (native OOXML; aim for **editable** text boxes, not image-per-slide)
- [ ] Dart/Flutter client: Go→WASM (web) + FFI (Android `.so` / iOS `.a`); JS-free client (flutter_math_fork, pure-Dart highlight)
- [ ] Conformance corpus (from Marp's own Jest fixtures) as the acceptance gate for every layer
- [ ] Attribution shipped from day one (LICENSE + NOTICE/CREDITS + per-file MIT headers on reused assets)

### Out of Scope

- **Embedded JS interpreter (goja) / Node / npm at runtime** — hard constraint; the entire point is a JS-free backend.
- **Interactive/reactive components (Slidev/Vue-style)** — Eden Press is server-side/static; templating not client reactivity.
- **A WYSIWYG editor / end-user app** — that is Eden Docs' role; Eden Press is a library + CLI.
- **Reimplementing TeX layout or a Markdown grammar from scratch** — use native libs (goldmark, latex2mathml) instead.
- **Forking Marp source** — clean-room reimplementation of the dialect + theme format; reuse only the MIT assets (themes, browser script) with attribution.

## Context

- **Prior work this session:** a full review + proposal (`PROPOSAL.md`) reviewed Marpit/Marp Core/Marp CLI (v4.x, MIT) and made all stack decisions. Two feasibility spikes were run and both de-risked the hard unknowns:
  - **Math (§11):** pure-Go `latex2mathml`→MathML rendered in real headless Chrome; 8/20 rendered wrong but **100% were converter bugs, not engine limits** (proven by rendering corrected MathML). Viable v1 default after a bounded converter-hardening pass; SVG/PNG fallback for heavy math.
  - **Parser (§12):** goldmark vs markdown-it over a 32-case corpus, DOM-normalized; **31/32 (97%) match**, only `<s>` vs `<del>` differed. Base-parser risk retired.
- **Ecosystem:** Eden platform (privacy-first, self-hosted, developer-first). Eden Press will be consumable by Eden-Biz, AOCore, etc., for server-side branded document generation.
- **Differentiation (not a 1-1 port), PROPOSAL §13:** (1) a "press" = one source → pluggable output profiles; (2) library-first & server-native; (3) output-as-data (structured AST). Layered later: design-token theming (multi-tenant brand), safe-for-untrusted rendering, deterministic/reproducible output, editable-PPTX + tagged/PDF-A export.

## Constraints

- **Tech stack**: Go source-of-truth; goldmark (Markdown), `tdewolff/parse` css (theme-CSS scoping), `chroma` (highlight), `latex2mathml` + `go-latex/latex` (math), native emoji, `cobra` (CLI), `chromedp` (raster export). Dart client via Go→WASM + `dart:ffi`. — Because the goal is a JS-free, single-static-binary backend embeddable in Go services.
- **Compatibility**: ingest Marp/Marpit Markdown + theme CSS; validate against a conformance corpus of Marp's own fixtures. — Migration path + ecosystem reuse are features.
- **Security**: allow-list HTML sanitization matching Marp's `xss` policy; capability-gated (no implicit network/asset fetch) for untrusted-content rendering. — Multi-tenant SaaS safety (Eden/AOCyber ethos).
- **Licensing/attribution**: MIT; credit Marp/Marpit/Marp CLI + deps; per-file MIT headers preserving Marp copyright on verbatim-reused assets (3 themes + browser fit/polyfill script); README "inspired by, not affiliated/endorsed." — Obligation + goodwill.
- **External dependency**: headless Chrome/Chromium required only for raster export (PDF/PPTX/PNG) — same as upstream; not JS in our code.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Name **Eden Press** (over "Eden Weave") | bare `weave` owned by active same-space dev tool (W&B Weave); `press` only dead/niche squatters, free on pub.dev | ✓ Good |
| GitHub org **AO-Cyber-Systems** (`AO-Cyber-Systems/eden-press`) | consistent with EdenDocs and Eden platform OSS | — Pending |
| **Zero JavaScript in the backend** (no goja/Node/npm) | core motivation; native Go leaf libs replace every JS dep | ✓ Good (spikes validated) |
| **goldmark** as Markdown base | 97% parity with markdown-it on Marp-relevant corpus (spike §12) | ✓ Good |
| **latex2mathml → native MathML** for math | pure-Go; browser renders MathML natively incl. Chrome PDF path; SVG/PNG fallback | ⚠️ Revisit (needs converter-hardening pass; spike §11) |
| **Go single source of truth; Dart via WASM/FFI** | avoids a second engine; one conformance pass | — Pending |
| **Not a 1-1 port** — profile abstraction + library API + structured-AST from day one | earns its own identity beyond "Marp in Go" | — Pending |
| **Library** project kind, work types vary | consumed by other code first; objectives mix foundation/port/feature | — Pending |

---
*Last updated: 2026-07-20 after initialization*
