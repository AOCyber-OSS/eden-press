# Eden Press — a Go/Dart document-generation framework (Marp-compatible): Review & Proposal

*Name: **Eden Press** — a printing-press metaphor for a framework that produces finished documents of many kinds from markup. Part of the Eden open-source platform (`eden-platform`, `eden-ui`, …), distinct from **Eden Docs** (the end-user productivity suite). Repo **`AOCyber-OSS/eden-press`**; Go module `go.aocyber.ai/eden-press`; Go package `press` (`press.Render(md)`); CLI `eden-press`. Optional layer split mirroring Marp: **`chase`** (framework, ~Marpit — the letterpress frame that locks the type) → **`press`** (batteries, ~Marp Core) → **`eden-press`** (CLI); flat `press/...` packages are a fine alternative if the metaphor reads too obscure. Throughout this doc, "the engine" = Eden Press.*

*Positioning: a **developer-facing framework for generating documents of various types from markup.** v1 target is **Marp-compatible slide decks** (the scope reviewed below); the architecture is deliberately format-agnostic so other document types (paged HTML/PDF, etc.) can follow. It is a library + CLI, not an editor.*

*Scope (v1): reimplement the three upstream Marp packages — **Marpit** (framework), **Marp Core** (batteries), **Marp CLI** (tool) — as a Go library + CLI, with a Dart/Flutter client-side story. All three upstream packages are MIT-licensed. Source reviewed: `@marp-team/marpit`, `@marp-team/marp-core`, `@marp-team/marp-cli` (v4.x) manifests + READMEs.*

---

## 1. Executive summary

Marp is three thin layers stacked on a small number of heavy leaf libraries:

- **Marpit** — a pure structural transform: `Markdown + theme CSS → { html, css }`. It's a set of plugins on top of **markdown-it**, plus a **PostCSS**-based theme-CSS scoper. This layer is almost entirely *deterministic string/AST manipulation* and is the highest-value, most portable target.
- **Marp Core** — Marpit + "batteries": three bundled themes (just CSS), GFM niceties, emoji (twemoji), math (KaTeX/MathJax), syntax highlighting (highlight.js), and an auto-scaling ("fit") feature.
- **Marp CLI** — argument/config parsing, watch/server modes, and export to **PDF/PPTX/PNG/JPEG** by driving **headless Chrome** (puppeteer-core).

**The core finding:** the *structural* work (slide splitting, the directive system, theme-CSS scoping, the SVG container, background images, pagination) has **no hard dependency on JavaScript** and ports cleanly to Go. The JS/browser touchpoints are a small, well-bounded set of **leaf libraries** — KaTeX, highlight.js, twemoji, and one tiny browser-side runtime script — plus **Chrome** for rasterizing exports. None of those force the *backend* to be Node.

**Recommended stance (zero JavaScript in the backend — hard requirement):**
1. **Native Go** for 100% of the structural transform (the Marpit + Marp-Core logic).
2. Every former JS leaf library is replaced by a **native** one — **no `goja`, no embedded JS interpreter, no Node, no npm**:
   - Syntax highlighting → **[`chroma`](https://github.com/alecthomas/chroma)** (pure Go).
   - Math → **[`~mekyt/latex2mathml`](https://git.sr.ht/~mekyt/latex2mathml)** (pure Go LaTeX→MathML); the browser renders MathML natively ([Chrome/Edge 109+, Safari 10+, Firefox](https://mathml.igalia.com/)), including the Chromium used for PDF export. Fallback for math-dense decks: **[`go-latex/latex`](https://github.com/go-latex/latex)** renders formulas to self-contained SVG/PNG in pure Go.
   - Emoji → native shortcode table + unicode regex.
3. **Chrome** is still required for PDF/PPTX/PNG (a *binary* the tool drives via `chromedp`; not JS in our code — same external as upstream). The **only** possible residual browser-side JS is the optional **auto-fit** helper, and even that runs in the *viewer's* browser, never the backend — and is replaceable per target (native Dart on Flutter, or a CSS-only technique) or droppable (§3).
4. **Go is the single source of truth.** For the Dart client, prefer **Go→WASM** (Flutter Web) and **FFI to the Go core** (Flutter native — `.so` on Android, static `.a` on iOS) over a second full reimplementation. Native Dart math via **[`flutter_math_fork`](https://pub.dev/packages/flutter_math_fork)** and highlighting via the pure-Dart **[`highlight`](https://pub.dev/packages/highlight)** package keep the client JS-free too.
5. **Fidelity is guaranteed by a conformance corpus** extracted from Marp's own Jest snapshot fixtures — built *before* the engine, and the acceptance test for every layer.

This keeps AO/Eden backends **fully JS-free** (the stated goal) without reinventing the genuinely hard leaf algorithms (TeX layout, tokenizer grammars) — those are covered by mature *native* Go/Dart libraries rather than a JS runtime. The one honest tradeoff is math *rendering quality*: native MathML is weaker than KaTeX in Chromium for heavy layouts (§9), which the corpus quantifies.

---

## 2. Architecture review

### 2.1 Marpit — the framework (`@marp-team/marpit`)

**Contract:** `marpit.render(markdown) → { html, css, comments }`. Ships *no* themes; it is "just a framework."

**Runtime dependencies (all portable):**
| Dep | Role | Port target (Go) |
|---|---|---|
| `markdown-it` ^14 | Markdown → token stream | **goldmark** (extensions) *or* custom |
| `markdown-it-front-matter` | YAML front-matter block | goldmark frontmatter ext / custom |
| `js-yaml` | parse directive YAML | `gopkg.in/yaml.v3` |
| `postcss` ^8 | CSS AST + transforms | `tdewolff/parse/v2/css` + selector rewriter |
| `postcss-nesting` | CSS nesting → flat | native (or rely on modern-browser nesting) |
| `@csstools/postcss-is-pseudo-class` | `:is()` down-level | usually **droppable** (target modern engines) |
| `lodash.kebabcase` | directive name casing | trivial native |
| `cssesc` | escape CSS identifiers | trivial native |

**What Marpit actually does (the pipeline):**
1. **Front-matter + comment scan.** Parses the top YAML block and inline `<!-- key: value -->` HTML comments into a directive stream.
2. **Slide splitting.** A markdown-it *core ruler* breaks the token stream at thematic breaks (`---`) — and optionally at a heading level (`headingDivider`) — wrapping each run in a `<section>`. The whole deck is wrapped in a `.marpit` container.
3. **Directive resolution.** Directives are applied to a per-slide state that *carries forward*:
   - **Global** (whole deck): `theme`, `style`, `headingDivider` (Marp Core adds `size`, `math`).
   - **Local** (this slide onward): `paginate`, `header`, `footer`, `class`, `color`, `backgroundColor`, `backgroundImage`, `backgroundPosition/Repeat/Size/Split`.
   - **Spot** (this slide only): any local directive prefixed with `_` (e.g. `_class`, `_paginate`, `_backgroundColor`).
   Two syntaxes carry them: **YAML front-matter** (deck-level) and **HTML-comment** directives (inline).
4. **Background images.** `![bg](url)` / `![bg fit right:40%](url)` image syntax → either CSS backgrounds or, in advanced mode, a separate SVG/`foreignObject` background layer.
5. **Theme CSS scoping.** The `ThemeSet` parses a theme's required `/* @theme name */` metadata (plus `@size`, `@auto-scaling`), then runs the CSS through PostCSS to: scope selectors to the container, map `:root`→ slide `section`, resolve `@import`/`@import-theme`, and inject slide-size, pagination (`section::after` from `data-marpit-pagination`), and advanced-background rules.
6. **Container/SVG rendering.** Default HTML wrapping, or **inline-SVG mode**: each slide becomes `<svg viewBox="0 0 W H"><foreignObject><section>…</section></foreignObject></svg>` for CSS-only pixel-perfect scaling, with a companion SVG layer for advanced backgrounds.

**Portability verdict:** ~95% deterministic Go. The only sharp edge is **markdown-it → goldmark semantic parity** (§4.1).

### 2.2 Marp Core — the batteries (`@marp-team/marp-core`)

Extends Marpit. Runtime deps: `marpit`, `marpit-svg-polyfill`, `highlight.js`, `katex`, `mathjax-full`, `postcss-selector-parser`, `xss`.

| Feature | Nature | Port target |
|---|---|---|
| **Default / Gaia / Uncover themes** | pure CSS assets (MIT) | **copy verbatim**, embed via `go:embed` |
| **GFM tables, strikethrough, line-break→`<br>`** | parser config | native in goldmark & Dart `markdown` |
| **Heading slugification** (`id` on `h1`–`h6`) | deterministic | native (GitHub-style slugger) |
| **HTML allow-list sanitization** (`xss`) | security filter | Go `bluemonday` policy; Dart custom |
| **Emoji** (`:smile:` + unicode → twemoji SVG) | shortcode table + unicode regex + asset URL map | native table + regex; assets by CDN/embed |
| **Syntax highlighting** (highlight.js + `{1-3}` line highlight) | tokenizer | **`chroma`** (pure Go) + small theme-CSS class reconciliation |
| **Math** (`$…$`, `$$…$$`; KaTeX or MathJax) | TeX layout | **`latex2mathml` → MathML** (pure Go, browser-native render); SVG/PNG fallback via `go-latex/latex` |
| **Auto-scaling / "fit"** (`# <!--fit-->`, code/KaTeX shrink) | **browser-measured** | emit markers; **reuse upstream browser script** |

**Critical nuance — what needs a browser vs. what doesn't:**
- **Auto-scaling is inherently browser-side.** It measures a *rendered* element and applies a scale transform. Upstream ships this as a tiny injected script (`@marp-team/marp-core/browser`), which also carries the WebKit SVG polyfill. **This script is language-agnostic — it runs in the client browser no matter what generated the HTML.** So the port does **not** reimplement fit logic; it emits the same markers (`data-marp-fitting`, the SVG container, `data-auto-scaling`) and **reuses the existing script as a static asset.** Major de-risk.
- **Everything else is a static transform** producing final HTML/CSS at render time: themes, `size`, emoji, math (when rendered server-side), slugs, sanitization, tables.

### 2.3 Marp CLI — the tool (`@marp-team/marp-cli`)

Runtime deps: `marp-core`, `marpit`, `puppeteer-core`, `chokidar`, `cosmiconfig`, `serve-index`, `ws`, `tmp`. (`yargs`, `pptxgenjs`, `express`, `chrome-launcher` are bundled at build time.)

| CLI capability | Port target (Go) |
|---|---|
| Arg parsing (`yargs`) | `cobra` / `urfave/cli` |
| Config (`cosmiconfig`: `.marprc`, YAML/JSON/JS) | `koanf`/`viper` — support YAML/JSON/TOML; **drop JS-config** (or gate behind goja) |
| Input: file / stdin / dir / URL | `net/http` + `os` |
| Watch mode (`chokidar`) | `fsnotify` |
| Server + live-reload (`ws`) | `net/http` + `nhooyr/websocket` |
| **HTML output** | native (the engine) |
| **PDF** (`puppeteer` `Page.printToPDF`) | **`chromedp`** → `Page.printToPDF` |
| **PNG / JPEG** (per-slide screenshot) | `chromedp` element/clip screenshots |
| **PPTX** (`pptxgenjs`, images per slide; `--pptx-editable`→`soffice`) | build OOXML zip natively (images) + shell to `soffice` for editable |
| Chrome discovery / bundled browser | reuse detection: env, known paths, `--browser-path`; optional download |

**Portability verdict:** the CLI is the *largest* surface but the *lowest-risk* — every dependency has a mature Go analogue. The only irreducible external is **Chrome**, which upstream requires too.

---

## 3. What's left after going fully JS-free

With `chroma` (highlight), `latex2mathml`→MathML (math), and native emoji, **every leaf transform is native** — highlight.js, KaTeX/MathJax, twemoji, and any JS interpreter are all gone from the backend. Two items remain, and only one is JavaScript at all:

1. **Chrome / Chromium** — required for pixel-accurate PDF/PPTX/PNG. This is an external *binary* the tool drives via `chromedp` (Go) / `puppeteer` (Dart); **no JavaScript enters our codebase**. Same external dependency as upstream. (MathML renders natively inside Chrome during export, so math needs no in-page script.)
2. **Auto-fit** (`# <!--fit-->`, code/KaTeX auto-shrink) — the *only* feature that fundamentally needs a layout engine to measure rendered text. It is browser-side by nature, never backend. Three JS-free ways to handle it, decided per output target:
   - **Flutter client:** reimplement natively via `TextPainter` measurement — zero JS.
   - **Browser HTML / Chrome export:** a **CSS-only fit** using container-query units (`cqw`) or SVG `<text>` auto-scaling (research spike; may not pixel-match upstream), **or** drop the feature (all other layout — themes, backgrounds, pagination, whole-slide SVG scaling — is pure CSS).
   - **Last resort only:** ship Marp's ~few-KB MIT fit script as a static asset that runs in the *viewer's* browser (still nothing in the backend). Reserved for if you want browser-HTML fit *and* reject the CSS approach.

Net: a **100% JS-free backend and native mobile** are achievable. Chrome (a binary) is the only external for raster export; auto-fit is the sole feature with a genuine "solve natively / with CSS / or drop" decision.

---

## 4. Deep dive on the two hard problems

### 4.1 Markdown pipeline fidelity (the #1 risk)

Marpit doesn't just *use* markdown-it — it hooks its **rulers** (block/inline/core) to inject directives, slide splitting, and background syntax. Two viable Go approaches:

- **A. goldmark + extensions (recommended).** goldmark (the parser behind Hugo) is AST-based, CommonMark-compliant, and extensible via `parser.ASTTransformer`, custom block/inline parsers, and node renderers. We reimplement each Marpit "plugin" as a goldmark extension. Pros: mature, fast, GFM built in, actively maintained. Cons: goldmark's **AST model differs from markdown-it's token stream**, and the two disagree on edge cases (raw-HTML handling, tight/loose lists, some inline precedence). Those deltas are exactly what the conformance corpus (§6) surfaces.
- **B. Port markdown-it's algorithm directly.** Maximum fidelity, but you inherit maintaining a markdown-it clone. Only worth it if goldmark deltas prove unfixable.

**Plan:** start with **A**, treat goldmark parity gaps as corpus failures, fix via extensions/forks; fall back to **B** only for specific rules that resist. Expect this to be the single largest engineering line-item.

### 4.2 Theme-CSS engine (the #2 risk)

Marpit's PostCSS chain scopes an author's theme into the slide container and injects generated rules. Port each PostCSS pass as a CSS-AST transform over `tdewolff/parse/v2/css` (fast, well-tested) with a small selector rewriter:
- parse `/* @theme */`, `@size`, `@auto-scaling` metadata comments;
- scope/prefix selectors to `.marpit` / slide `section`; map `:root`;
- resolve `@import` / `@import-theme` across the ThemeSet;
- inject slide dimensions, pagination, and advanced-background rules.

**Scope reducer:** several PostCSS passes upstream exist purely to *down-level* modern CSS (`:is()`, `:where()`, nesting) for older browsers. Since our export path is **Chrome** and our HTML target is modern engines, we can **target native `:is/:where/nesting` and drop those passes** — smaller, simpler, faster. (Keep a compat flag if we later care about legacy Safari.)

---

## 5. Proposed architecture for the Go port

### 5.1 Module layout (single Go module, layered like upstream)

```
<module-root>/
  marpit/            # Layer 1: the framework (no themes)
    parser/          #   goldmark extensions: directives, slide-split, bg-image, inline-svg
    directive/       #   global/local/spot resolution + carry-forward state
    theme/           #   ThemeSet + CSS scoping transforms (tdewolff/css)
    render/          #   .marpit container, <section>, inline-SVG container
    marpit.go        #   Marpit{}.Render(md) (html, css, comments)
  core/              # Layer 2: batteries on top of marpit
    themes/          #   default.css, gaia.css, uncover.css (go:embed, verbatim MIT)
    emoji/           #   shortcode table + twemoji unicode map
    math/            #   goja+KaTeX (server) | client-mode passthrough
    highlight/       #   goja+highlight.js  | chroma (build tag)
    sanitize/        #   bluemonday allow-list matching upstream xss policy
    browser/         #   embedded upstream fit+polyfill script (asset)
    core.go          #   Marp{}.Render(md)
  convert/           # Layer 3a: exporters (importable, no CLI)
    chrome/          #   chromedp session mgmt + discovery
    pdf.go png.go pptx.go
  cmd/<bin>/         # Layer 3b: the CLI (cobra): convert/watch/serve/preview
  conformance/       # golden corpus + runner (see §6)
```

### 5.2 Public Go API (importable package — first-class goal)

```go
import "example.com/mod/core"

m := core.New(core.Options{
    Themes:   core.DefaultThemes(),   // or AddTheme(css)
    Math:     core.MathKaTeX,          // KaTeX | MathJax | ClientSide | Off
    Highlight: core.HighlightEmbedded, // goja+hljs | Chroma | Off
    InlineSVG: true,
})

out, err := m.Render(markdown)   // out.HTML, out.CSS, out.Comments, out.Meta
// caller assembles a page, or:

pdf, err := convert.ToPDF(ctx, out, convert.PDFOptions{ /* … */ })   // needs Chrome
png, err := convert.ToImages(ctx, out, convert.ImageOptions{Format: convert.PNG})
pptx, err := convert.ToPPTX(ctx, out, convert.PPTXOptions{Editable: false})
```

Design rules: engine (`marpit`/`core`) has **zero Chrome dependency** and is pure/deterministic; only `convert` touches Chrome. This lets AO/Eden backends embed HTML rendering with no external process, and opt into Chrome only for rasterized exports.

### 5.3 CLI parity

`<bin> deck.md` → HTML; `--pdf/--pptx/--images`; `--watch`, `--server`, `--preview`; `--theme`, `--theme-set`; config via `.marprc.{yml,json,toml}`. Behavioral compatibility with `marp-cli` flags where sensible so existing muscle-memory/CI transfers.

---

## 6. Fidelity strategy — the conformance corpus (do this FIRST)

Before writing engine code, extract a **language-neutral golden corpus** from upstream's own Jest snapshot fixtures:

```
conformance/cases/<name>/
  input.md
  options.json          # constructor/directive options
  expected.html         # normalized (attr-sorted, whitespace-collapsed)
  expected.css          # normalized
```

- Seed from Marpit's + Marp Core's snapshot tests (both MIT).
- A Go test runner renders each case and diffs against `expected.*` after normalization (parse both sides, compare DOM/CSS-AST — not raw strings — to ignore cosmetic differences).
- **This corpus is the acceptance gate for every layer and every language binding.** Re-import periodically to track upstream drift.

This converts "did we match Marp?" from a vibe into a green/red signal, and it's the only sane way to keep a Go engine *and* a Dart binding honest against a moving upstream.

---

## 7. The Dart / client-side story

The user wants a **client-side Dart** option. Reimplementing the whole engine a second time in Dart — and keeping it in fidelity-lockstep — is the expensive path. Three options, in order of preference:

1. **Go→WASM (Flutter Web) + FFI to Go (Flutter native, incl. mobile) — recommended.**
   - **Native (desktop + mobile) via `dart:ffi`:** Android links a `.so` (`-buildmode=c-shared`, cgo + NDK); **iOS** links a static `.a` (`-buildmode=c-archive`) resolved via `DynamicLibrary.process()`. This is the standard Flutter FFI-plugin pattern (already used to ship Go/Rust cores). Confirmed to cover both mobile platforms.
   - **Web via WASM:** compile the same `core` to WASM and load it from Flutter Web. (Go WASM needs a `wasm_exec.js` loader shim — but that's the Flutter-Web host runtime, not JS we author; Flutter Web already runs on a JS/WASM host.)
   - **One source of truth**, one conformance pass. The engine returns HTML+CSS+MathML; the Dart side stays JS-free — math via **`flutter_math_fork`** (native KaTeX-layout port) or native MathML, highlighting via the pure-Dart **`highlight`** package, and auto-fit via native `TextPainter` measurement.
   - Cost: Go WASM binaries are sizable (~2 MB+ gzipped) — acceptable for an app, evaluate for web-embed latency. FFI adds per-platform cross-compile/toolchain setup (well-trodden).
2. **Pure-Dart "lite" port** (`markdown` + `csslib` packages) — a dependency-free pub package.
   - Honest use case: a Flutter app that must render decks with **no WASM and no Go toolchain**. Realistically a *subset* (structural transform + client-side math/highlight), validated against a subset of the corpus.
   - Cost: a second parser/CSS implementation to maintain — accept only if (1) is ruled out.
3. **Thin client + Go service** — Dart calls a Go HTTP/render endpoint. Simplest, but not "client-side" and needs a backend.

**Recommendation:** build **(1)**. Treat **(2)** as a fallback documented up front, gated on whether WASM size/perf is unacceptable for the actual Flutter target. In all cases the **browser script and client-side KaTeX/highlight.js are shared assets**, so the Dart surface is mostly "load WASM, pass markdown, get HTML, mount it."

---

## 8. Phasing (maps to DevFlow objectives)

| # | Objective | Deliverable | Rough size |
|---|---|---|---|
| 0 | **Conformance corpus + runner** | golden cases from upstream snapshots; AST-diff test harness | S–M |
| 1 | **Marpit-in-Go** | goldmark extensions (directives, slide-split, bg-image, inline-SVG), directive engine, theme-CSS scoper, container/SVG renderer; passes Marpit corpus | **L** (biggest) |
| 2 | **Marp-Core-in-Go** | embedded themes, GFM/slug/sanitize, emoji (native), highlight (**chroma** + theme-CSS reconciliation), math (**latex2mathml→MathML**), fit markers; passes Core corpus | **M–L** |
| 3 | **Importable Go API** | stable `core.Render` + `convert` package boundary; docs; examples | S |
| 4 | **CLI-in-Go** | cobra CLI; HTML/watch/server/preview; PDF+PNG via chromedp | M |
| 5 | **PPTX + polish** | native OOXML PPTX (+ optional `soffice` editable); Chrome discovery/bundling | M |
| 6 | **Dart binding** | Go→WASM (web) + FFI (native); shared assets; subset corpus green | M |
| 7 | **Auto-fit resolution + math-fidelity tuning** | native `TextPainter` fit (Flutter) + CSS-only fit spike (`cqw`/SVG-text) for browser/PDF; MathML quality pass vs corpus, wire SVG/PNG fallback | S–M |

Objectives 1 and 2 dominate the schedule and carry all the fidelity risk; 0 must precede them. 4–5 are large-surface but low-risk. 6 depends only on 3. 7 removes the last browser-side JS holdout (auto-fit) and closes the MathML-vs-KaTeX quality gap.

---

## 9. Risks & open decisions

**Risks**
- **markdown-it ↔ goldmark edge-case deltas** (§4.1) — **spike-tested (§12): 31/32 structural match, only `<s>` vs `<del>` differed.** Downgraded from "top risk" to "manageable"; the full CommonMark + Marpit-fixture sweep (objective 0) remains the ongoing gate, and rare deltas may still force per-rule custom parsers.
- **PostCSS pass parity** — mitigated by dropping legacy down-level passes and targeting modern CSS.
- **Math rendering quality (MathML vs KaTeX)** — **spike-validated (§11):** for *correct* MathML, Chrome renders the common constructs at KaTeX quality. Residual Chromium-MathML weaknesses (numbered equations/`\tag`, very complex alignments, Safari/Firefox differences) are unexercised by the spike and still warrant the pure-Go `go-latex/latex` **SVG/PNG fallback** for heavy-math decks. *Fidelity* risk, not a JS risk.
- **Headless-Chrome math font** — native MathML needs an OpenType MATH font in the export environment or a bare server renders tofu. Mitigation: ship / point Chrome at STIX Two Math or Latin Modern Math.
- **Highlight theming reconciliation** — `chroma` token classes ≠ highlight.js `.hljs` classes, so the code-block CSS in the three themes needs a one-time remap/regeneration. Bounded.
- **`latex2mathml` coverage** — **spike-quantified (§11):** 20/20 convert without crashing, but 8/20 render wrong *as-shipped* (big-operator limits, `\binom`, `\sqrt[n]`, `pmatrix` fence, `aligned`, font-variants). **All are converter-side bugs, not engine limits** — bounded pure-Go patches (fork/vendor + contribute upstream). Budget a converter-hardening pass before math ships as a default.
- **Chrome coupling** for exports — unavoidable; same as upstream; make browser discovery robust.
- **Upstream drift** — Marp evolves; the fork tracks it via periodic corpus re-import, not manual diffing.
- **Sanitization parity** — security-sensitive; bluemonday policy must match the upstream `xss` allow-list exactly (test it).

**Resolved (per user, this round)**
- ✅ **No JavaScript in the stack** — `goja` dropped; all leaf libs are native.
- ✅ **Math:** native `latex2mathml` → browser-native MathML (SVG/PNG fallback via `go-latex/latex`). *Sub-decision remaining:* the acceptable MathML-quality threshold before auto-invoking the SVG fallback.
- ✅ **Highlighting:** native `chroma`.
- ✅ **Dart:** Go→WASM (web) + FFI (native, incl. Android + iOS) — confirmed viable on mobile.
- ✅ **Scope:** full (engine + HTML + PDF + PPTX + PNG/JPEG + watch + server + preview).

**Open decisions (need your call — not blocking the proposal)**
1. **Auto-fit handling** — the one browser-side JS holdout. Choose: native `TextPainter` fit on the Flutter client + **CSS-only fit** (`cqw`/SVG-text) for browser/PDF; or **drop auto-fit** entirely; or (last resort) ship Marp's tiny MIT fit script as a viewer-side asset. *Recommendation: native on Flutter, CSS-only spike for browser/PDF, drop if the spike can't pixel-match.*
2. **Parser base:** default **goldmark** with corpus-driven edge-case fixes, vs. a full markdown-it algorithm port for 1:1 behavior. *Recommendation: goldmark; escalate per-rule only if the corpus demands it.*
3. **Chrome delivery:** rely on system Chrome / `--browser-path`, vs. bundling/downloading a pinned Chromium for reproducible exports. *Recommendation: system-first with optional pinned download.*

**Licensing:** all upstream (Marpit, Marp Core, Marp CLI, the three themes, the browser script) is **MIT** — a Go/Dart reimplementation, and verbatim reuse of the CSS themes and browser script, are license-compatible provided MIT attribution is retained. No blocker.

---

## 10. Bottom line

A Go port is **feasible and well-shaped, with zero JavaScript in the backend**: the structural 80% is pure Go, and the former "JS 20%" is now covered by mature *native* libraries — `chroma` (highlight), `latex2mathml`→MathML (math), native emoji — with **no `goja`, no Node, no npm**. The only externals are Chrome (a binary, for raster export — same as upstream) and, at most, an optional viewer-side auto-fit script that never touches the backend and is replaceable natively per target. The schedule is dominated by two items — the Markdown pipeline and the CSS scoper — both de-risked by building the conformance corpus first. Go is the single source of truth; the Dart/Flutter client (web via WASM, mobile + desktop via FFI) consumes that same code and stays JS-free too. Net result: AO/Eden backends render Marp decks with **no JavaScript runtime at all**, from one importable Go package and one CLI, with a Flutter-friendly client path. The one honest tradeoff is native MathML's rendering quality vs. KaTeX, quantified by the corpus and backstopped by a pure-Go SVG/PNG math fallback.

---

## 11. Spike results — native math fidelity (2026-07-20)

**Goal:** determine whether the pure-Go, JS-free math path (`latex2mathml` → browser-native MathML) is good enough to be the v1 default, using the *actual* PDF-export engine (headless Google Chrome via `chromedp`) on a 20-formula battery deliberately weighted toward the constructs that break TeX→MathML converters. Artifacts in `scratchpad/math-spike/` (`compare.png` = native vs KaTeX side-by-side; `corrected.png` = fixes rendered; `variant.png` = mathvariant vs codepoint).

**Environment:** Go 1.26 (arm64), Google Chrome, `STIXTwoMath.otf` present (the OpenType MATH font MathML needs).

**Headline:** all 20 convert without crashing; **10/20 render at KaTeX quality as-shipped, 2/20 acceptable, 8/20 render wrong.** Critically, **100% of the defects are bugs in the pure-Go converter — none is a Chrome/MathML *rendering* limitation.** Hand-authored correct MathML for all 8 renders at KaTeX quality in the same Chrome.

| Result | Cases |
|---|---|
| ✅ KaTeX-quality as-shipped (10) | `E=mc²`, Pythagoras, quadratic (`\frac` **and** `\over`), Greek run, `∂u/∂t=α∇²u`, `matrix`, `bmatrix`, `cases`, `\overbrace` |
| ⚠️ Acceptable, minor (2) | Gaussian integral (smaller ∫, side limits), accents (slight offset) |
| ❌ Wrong as-shipped (8) | `\sum`/`\prod`/`\lim` limits not stacked; `\binom`; `\sqrt[3]`; `pmatrix` fence; `aligned`; `\mathbb`/`\mathbf`/`\mathcal` |

**Root cause of each ❌ (all pure-Go, bounded fixes):**
- **Big-operator limits** — converter emits `<msubsup>`; should emit `<munderover>` (movablelimits) in display mode. *(small)*
- **`\binom` & `pmatrix`** — converter emits the opening `(` as the *closing* fence too (`<mo>(</mo>…<mo>(</mo>`). Shared bug. *(small)*
- **`\sqrt[3]{x}`** — optional `[n]` argument not parsed; puts literal `[` as radicand. *(small)*
- **`aligned`** — emits literal `<mi>&</mi>` + `<mspace linebreak>`; needs `<mtable columnalign="right left">`. *(medium)*
- **`\mathbb`/`\mathbf`/`\mathcal`** — emits plain `<mi>R</mi>`. Note: `mathvariant="double-struck"` **does not work** — MathML Core dropped non-`normal` `mathvariant`; verified that the real Unicode codepoint (`&#x211D;` → ℝ) renders correctly. Fix = map letter+variant → Unicode math-alphanumeric codepoint. *(small table)*

**Verified (corrected.png):** `munderover` limits stack; stretchy fences on `\binom`/`pmatrix`; `<mroot>` cube root; `mtable` two-line `aligned` — all render at KaTeX quality. **(variant.png):** codepoint approach renders `ℝ 𝐯 ℒ d𝑥` correctly.

**Honest caveats:**
- The battery did **not** exercise Chrome's known MathML weak spots (numbered equations, `\tag`/`\label`, very complex alignment/coloring) or Safari/Firefox rendering — so "Chrome renders correct MathML at KaTeX quality" is established for *common* constructs only. Keep the `go-latex/latex` SVG/PNG fallback for heavy math and non-Chromium viewers.
- Native MathML depends on a MATH font in the export environment; bundle STIX Two Math / Latin Modern Math for server-side Chrome.
- `latex2mathml` is a single-author lib; the fixes mean fork/vendor + patch (MIT-spirit) and ideally upstream contribution.

**Verdict:** the JS-free math path is **viable as the v1 default after a bounded converter-hardening pass** (objective 7). Ceiling is the converter, not the engine — exactly the risk we wanted to retire. Fall back to pure-Go SVG/PNG rendering for math-dense decks and non-MATH-font environments.

---

## 12. Spike results — markdown parser parity (2026-07-20)

**Goal:** quantify the #1 structural risk — how far **goldmark** (the proposed Go parser base) diverges from **markdown-it** (what Marp uses). Artifacts in `scratchpad/md-spike/` (`corpus.txt`, `ref.mjs`, `main.go`).

**Method:** a 32-case corpus weighted toward Marp-relevant and known-divergence constructs, run through **both** parsers configured to match Marp Core (markdown-it `html:true, breaks:true, linkify:false` + default tables/strikethrough; goldmark `Table + Strikethrough + WithUnsafe + WithHardWraps`). Outputs compared **DOM-normalized** (via `x/net/html`) so cosmetic differences — `<br>` vs `<br/>`, inter-block whitespace, attribute order — don't count as divergence.

**Headline: 31/32 structural match (97%).** The single DIFF is cosmetic:

| Case | markdown-it | goldmark | Fix |
|---|---|---|---|
| strikethrough | `<s>gone</s>` | `<del>gone</del>` | override goldmark's Strikethrough node renderer to emit `<s>` (trivial), or target `<del>` in theme CSS. *(GFM spec says `<del>`; markdown-it uses `<s>`; Marp inherits `<s>`.)* |

**Matched — including every deliberately-planted trap:**
- **`---` after a paragraph → `<h2>` (setext)** in both — confirms Marpit's slide-splitter must special-case `---`, independent of parser choice.
- Intraword emphasis (`a*b*c` yes, `a_b_c` no); autolinks (URL + email); reference links; nested code spans; emphasis nesting & flanking.
- Tight vs loose lists; nested lists; ordered `start=3`; `)` delimiter; multi-paragraph items.
- GFM tables with/without outer pipes + alignment; fenced (info string) & indented code; nested blockquotes.
- HTML blocks, inline HTML, and **HTML comments** (the Marpit directive carrier) — identical.
- Entities (named/numeric/unknown), backslash escapes, setext headings, ATX no-space non-heading.

**Honest caveats:**
- 32 targeted cases ≠ exhaustive. CommonMark has ~650 spec examples; this is a strong *indicator*, not proof of full parity. The full sweep (CommonMark spec.json + Marpit's own snapshot fixtures) is **objective 0**.
- The normalizer intentionally ignores cosmetic whitespace/void-element/attr-order diffs; genuinely whitespace-significant divergences (rare) would be masked. `<pre>`/`<code>` text is compared verbatim and matched.
- This tests the **base parser only.** Marpit's real value-add — directive plugins, slide splitting, `![bg]` syntax, inline-SVG — are custom extensions to reimplement regardless (objective 1); this spike de-risks the *foundation* they sit on.

**Verdict:** the feared markdown-it → goldmark semantic gap **did not materialize** on Marp-relevant constructs — a single cosmetic tag-name difference across 32 cases. goldmark is a sound base; the base-parser risk is substantially retired, and objective 0's conformance corpus becomes the ongoing gate rather than an open question.

---

## 13. Differentiation from Marp — a superset, not a 1-1 port (2026-07-20)

Eden Press stays **Marp-compatible** (ingests Marp Markdown + themes — the on-ramp and ecosystem reuse) but exists for reasons Marp doesn't serve. A pure port competes with Marp on Marp's terms and forever chases upstream; these give Eden Press its own identity while keeping compatibility as a feature.

**Identity differentiators (shape v1 architecture even where the feature ships later):**
1. **A "press," not a slide tool.** One markup source → pluggable **output profiles**. v1 profile = Marp-compatible slides; the engine is designed so paged documents/reports (A4/Letter, running headers, page numbers, TOC), single-page articles, and EPUB become *profiles*, not rewrites. A press prints many things — this is the reframe that justifies the name.
2. **Library-first & server-native.** An importable Go package that renders documents *inside* a service (Eden-Biz, AOCore) with **no Node and no browser** for HTML/structured output; Chrome only for raster export. Marp is a Node CLI; Eden Press is `press.Render()` in your binary.
3. **Output as data, not just HTML.** Because we own the AST, emit a structured document model (JSON AST, outline, speaker notes, metadata) *alongside* rendered formats — for programmatic manipulation, search/indexing, accessibility trees, translation pipelines, and LLM/AOCore ingestion.

**Layered differentiators (build on the above):**
4. **Brandable by design tokens.** Keep Marp's raw-CSS themes for compat, add a token layer (JSON/TOML brand tokens → generated theme CSS) so documents are themed programmatically per tenant/brand — ties into Eden-Biz multi-tenant branding and the AO brand system.
5. **Safe for untrusted content.** A hardened, capability-gated renderer (no implicit network/asset fetch, deterministic, strict allow-list sanitization) so a multi-tenant SaaS can render user-submitted Markdown safely — an Eden/AOCyber-ethos differentiator.
6. **Deterministic & reproducible.** Zero-JS + Go determinism → byte-reproducible output, git-diffable decks, content-hashed incremental rebuilds.

**Export-quality upgrades to aim past Marp CLI:** **editable** PPTX (real OOXML text boxes vs Marp's image-per-slide) and **tagged / accessible, PDF/A-capable** output for enterprise/gov/archival.

**v1 scope discipline:** ship *Marp-compatible slides in Go with zero JS* (the immediate need) — but lay the **profile abstraction + library-first API + structured-AST output** so #1–#6 are natural extensions, not rewrites. Differentiators **1–3 are the identity**; 4–6 layer on afterward.

## 14. Attribution & credit to Marp (greenfield, inspired-by)

Eden Press is a **clean-room reimplementation inspired by and Markdown-compatible with Marp / Marpit** (© the Marp team, MIT). Credit is both an obligation and a courtesy, shipped from day one:
- **LICENSE** (MIT for Eden Press) + a **NOTICE / CREDITS** file crediting Marpit, Marp Core, and Marp CLI, plus the Go/asset dependencies (goldmark, chroma, `latex2mathml`, and the KaTeX/MathML lineage) with their licenses.
- **Per-file MIT headers preserving the original Marp copyright** on any asset reused **verbatim** — specifically the three bundled themes (default/gaia/uncover CSS) and the browser fit + Safari-SVG-polyfill script. Verbatim reuse of MIT assets *requires* retaining their copyright + license notice.
- **README acknowledgment:** "Inspired by and Markdown-compatible with [Marp](https://marp.app) — © Marp team, MIT," plus an explicit **"not affiliated with or endorsed by the Marp team"** line.
- Frame compatibility as respect: we implement their Markdown dialect and theme format; we do not fork or vendor their source. Where we do reuse MIT assets, we say so, in place.
