# Roadmap: Eden Press

## Overview

Eden Press ships as a strict build-order: prove the acceptance gate first (Objective 0), then
build the framework layer bottom-up (`chase/markdown`+`directive`+`theme` in Objective 1,
`chase/model`+`chase/profile`+`profiles/slides` in Objective 2 — kept deliberately separate so
"structured document model" and "output-profile abstraction" are first-class deliverables, not
prose folded into "port Marpit"). Objective 3 turns that framework into the importable
`press.Render()` public API with all Marp-Core batteries wired in, and is the one gate every
downstream consumer (CLI, exporters, Dart) depends on. From there the graph fans out into four
genuinely independent workstreams that all depend on Objective 3 alone and not on each other —
the CLI (4), chromedp-based PDF/PNG export (5), the Chrome-free native-OOXML PPTX exporter (6),
and the Dart/Flutter binding (7) — eligible for parallel execution via `/devflow:workstreams`.
Objective 8 closed the v1 roadmap as a deliberate last-by-necessity hardening pass: math-fidelity
tuning and auto-fit resolution had nothing to tune until the rest of the pipeline rendered
something real. Objective 9 is a post-v1 insertion: grow the PR #11 spreadsheet formula subset
toward a practical Excel-compatible layer (live Dart eval + OOXML store; Excel remains authority
for unsupported functions).

## Objectives

**Objective Numbering:**
- Integer objectives (1, 2, 3): Planned milestone work
- Decimal objectives (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [x] **Objective 0: Conformance Corpus, Acceptance Gate & Attribution Bootstrap** - Golden corpus (Marp's own MIT fixtures + full CommonMark/GFM sweep), DOM-normalized HTML diff runner, new CSS-AST diff tooling, upstream-drift CI check, and day-one LICENSE/NOTICE/attribution. (completed 2026-07-20)
- [x] **Objective 1: chase/markdown + chase/directive + chase/theme (Marpit-in-Go)** - Directive resolution, slide-splitting, background-image syntax, inline-SVG mode, and the theme-CSS scoping pipeline with its own tested selector-rewriter. (completed 2026-07-21)
- [x] **Objective 2: chase/model + chase/profile + profiles/slides** - The structured JSON document model and output-profile interface as first-class packages, with `profiles/slides` as profile #1. (completed 2026-07-21)
- [x] **Objective 3: press/ Batteries + Public API** - Embedded themes, GFM/slug/sanitize/emoji/highlight/math batteries, and the stable `press.Render()` API with a CI-enforced zero-chromedp boundary. (9/9 TRDs complete — 03-09 capstone: press.Render one-parse-two-sinks compose + no-chromedp gate.)
- [x] **Objective 4: CLI (cmd/eden-press)** - `eden-press` convert/watch/serve/preview, theme loading, config file + stdin input. (8/8 TRDs complete — 04-08 capstone: preview via pkg/browser + full-stack integration test + CLI-imports CI gate.)
- [x] **Objective 4.1: CLI Agent Interface (AI-usability, Chrome-free)** [INSERTED] - `--format json` structured Output + Chrome-free `--format pptx` + machine-readable errors/exit-codes + AGENTS.md; CLI stays chromedp-free. (2/2 TRDs complete — 04.1-01 json format + error contract, 04.1-02 pptx export wiring + CI-enforced chromedp-free ./cmd/... gate + AGENTS.md.) (completed 2026-07-22; pending orchestrator reconcile at merge)
- [x] **Objective 5: convert/pdf + convert/png (chromedp raster export)** - PDF and PNG/JPEG export via headless Chrome, robust Chrome discovery, deterministic/CI-hardened export. (5/5 TRDs complete — 05-01 discovery/Session, 05-02 determinism substrate, 05-03 PDF (EXP-01), 05-04 PNG/JPEG (EXP-02), 05-05 CI hardening capstone: pinned no-system-Chrome container + PDF re-validation process (EXP-04 complete).) (completed 2026-07-21)
- [x] **Objective 5.1: CLI Raster Export Binary (eden-press-export)** [INSERTED] - separate Chrome-permitting `eden-press-export --format pdf|png` binary; core `eden-press` stays chromedp-free (2/2 TRDs complete — 05.1-01 binary + gate re-scope, 05.1-02 CI smoke + docs). (completed 2026-07-22)
- [x] **Objective 6: convert/pptx (native OOXML)** - Hand-rolled, Chrome-free, editable-text-box PPTX export consuming the docmodel directly — sibling to Objective 5, not sequential after it. (5/5 TRDs complete; verified passed 2026-07-21)
- [x] **Objective 7: Dart/Flutter Binding (bind/capi + bind/dart)** - One Go core exposed via C-ABI, built three ways (Android `.so`, iOS `.a`, Web `.wasm`), gated only on Objective 3's API stability. (5/5 TRDs complete; verified passed 2026-07-21)
- [x] **Objective 8: Math-Fidelity Hardening + Auto-Fit Resolution** - The five math-converter root-cause fixes, a concrete MathML fallback-trigger rule, bundled STIX Two Math, and the final auto-fit mechanism decision. (7/7 TRDs complete: latex2mathml fork + all 8 spike cases at KaTeX-parity, structural-ceiling fallback rule, STIX WOFF2 + MATH-table smoke, Flutter-only auto-fit / zero-JS HTML). (completed 2026-07-22)
- [ ] **Objective 9: Excel Formula Layer for Eden Spreadsheets** - Grow the PR #11 Dart formula subset toward the largest practical Excel-compatible surface (corpus-first, phased A/B/C coverage), keeping Go OOXML as store/export and Excel as authority for unsupported functions.

## Objective Details

### Objective 0: Conformance Corpus, Acceptance Gate & Attribution Bootstrap
**Goal**: Establish the acceptance gate every other objective is validated against, and get licensing/attribution right from day one — before any vendored asset or engine code exists.
**Depends on**: Nothing (first objective)
**Requirements**: CONF-01, CONF-02, CONF-03, CONF-04, LIC-01, LIC-02, LIC-03, LIC-04
**Success Criteria** (what must be TRUE):
  1. A golden corpus of Markdown→HTML/CSS cases exists in-repo, seeded from Marp's own MIT-licensed Jest fixtures plus the full CommonMark (~600 examples) and GFM (~672 examples) spec sweeps, with pass/fail tracked per spec section (not just an aggregate percentage).
  2. The test runner renders every case and compares DOM-normalized HTML via an explicit, narrow allow-list of cosmetic differences (`<br>` vs `<br/>`, whitespace, attribute order) — proven not to mask real bugs by a negative test (a deliberately broken `<pre>`/code-span case is still caught).
  3. A CSS-AST diff comparator (new tooling, not a repurposed HTML diff) exists and can detect an intentionally-broken theme-CSS output as a failing case.
  4. `LICENSE` (MIT), `NOTICE`/`CREDITS` (crediting Marpit/Marp Core/Marp CLI + goldmark/chroma/latex2mathml/go-latex/latex with their licenses), and README "inspired by, not affiliated/endorsed" language exist in-repo, with a standing per-PR checklist requiring a NOTICE update any time a new vendored/verbatim asset is added.
  5. A scheduled CI job exists that checks for drift against Marp's latest upstream tag (a mechanism, not a manual reminder).
**Plans**: 6 TRDs in 3 waves
- [x] 00-01-TRD.md — Repo scaffolding, go.mod, LICENSE/NOTICE/README, attribution + drift CI (LIC-01..04) [wave 1]
- [x] 00-02-TRD.md — Corpus format + DOM-normalized HTML diff runner + negative test (CONF-02) [wave 2]
- [x] 00-03-TRD.md — CSS-AST diff spike: normalized model + grammar-stream builder (CONF-03) [wave 2]
- [x] 00-04-TRD.md — CommonMark + GFM full spec sweep, per-section pass/fail (CONF-04) [wave 3]
- [x] 00-05-TRD.md — Marp golden-corpus extraction via npm oracle + corpus runner (CONF-01) [wave 3]
- [x] 00-06-TRD.md — CSS-AST diff comparator + theme negative/order tests (CONF-03) [wave 3]

### Objective 1: chase/markdown + chase/directive + chase/theme (Marpit-in-Go)
**Goal**: Reimplement Marpit's actual value-add — the directive system, slide-splitting, background-image syntax, inline-SVG mode, and the theme-CSS scoping pipeline — as goldmark extensions plus a standalone CSS-scoping engine, validated against Objective 0's corpus.
**Depends on**: Objective 0
**Requirements**: PARSE-01, PARSE-02, PARSE-03, PARSE-04, PARSE-05, PARSE-06, PARSE-07, THEME-01, THEME-02, THEME-03, THEME-04
**Success Criteria** (what must be TRUE):
  1. Markdown is parsed via goldmark's explicit two-phase `Parser().Parse()` + `Renderer().Render()` call (never the one-shot `Convert()`), so the finalized, post-transform AST is inspectable — proven by a test that hooks the AST between phases.
  2. Global, local, and spot directives (both YAML front-matter and HTML-comment syntax) resolve with correct carry-forward state across a multi-slide deck, matching Objective 0's corpus.
  3. A deck splits into `<section>`-wrapped slides on `---`/`headingDivider` (with the setext-H2 trap resolved correctly), `![bg ...]` syntax resolves to CSS backgrounds or the advanced-background SVG layer, and inline-SVG mode (`<svg><foreignObject><section>`) renders correctly both as an HTML string AND rasterized (not string-diff alone).
  4. Theme CSS runs through the full ordered scoping pipeline (meta parse → `:root` remap/specificity fix-up → selector-scope → `@import`/`@import-theme` resolve → render-time pagination/background injection) and passes Objective 0's CSS-AST diff gate for all 3 bundled themes plus a synthetic theme stress-testing `:is()`/`:where()`/native CSS nesting.
  5. The selector-rewriter exists as its own independently unit-tested subsystem (dedicated test suite, not folded into theme-loading integration tests) — reflecting that it has no upstream Go analogue.
**Plans**: 8 TRDs in 5 waves
- [x] 01-01-TRD.md — chase/theme/selector: standalone selector-rewriter (THEME-04) [wave 1]
- [x] 01-02-TRD.md — chase/directive: directive state machine + front-matter/comment syntaxes (PARSE-02, PARSE-03) [wave 1]
- [x] 01-03-TRD.md — chase/theme: Stylesheet model + @theme/@size/@auto-scaling metadata (THEME-01, THEME-02) [wave 1]
- [x] 01-04-TRD.md — chase/theme: ordered two-tier scoping pipeline (Load + Pack) (THEME-03) [wave 2]
- [x] 01-05-TRD.md — chase/markdown: two-phase seam + comment wiring + slide-split + container (PARSE-05) [wave 2]
- [x] 01-06-TRD.md — chase/markdown: directive application + directive-set materialization (PARSE-04) [wave 3]
- [x] 01-07-TRD.md — chase/markdown: background images + inline-SVG advanced backgrounds (PARSE-06, PARSE-07) [wave 4]
- [x] 01-08-TRD.md — integration: two-phase seam + Marp corpus + cssdiff gates + inline-SVG raster check (PARSE-01) [wave 5]

> **Planning note (internal parallelism):** `chase/directive` (pure carry-forward state machine, zero goldmark import) and `chase/theme` (CSS `Stylesheet` model over `tdewolff/parse/v2/css`, zero dependency on the Markdown/AST side) share no dependency edge and can be planned/executed as separate jobs within this objective; `chase/markdown` (goldmark Extenders wiring directive resolution, slide-splitting, `![bg]`, inline-SVG) depends on `chase/directive`'s output via `parser.Context` and should be sequenced after it.

### Objective 2: chase/model + chase/profile + profiles/slides
**Goal**: Ship the structured document model and the output-profile abstraction as first-class packages from day one — the single most consequential architectural decision in the whole project, since retrofitting either after a v1 profile hardens means a rewrite, not an extension.
**Depends on**: Objective 1
**Requirements**: MODEL-01, MODEL-02, MODEL-03, MODEL-04
**Success Criteria** (what must be TRUE):
  1. A structured `Document{Meta, Sections, Outline}` model is built by a direct recursive walk of the exact same finalized AST that produces HTML — not a second parse, not reverse-engineered from rendered HTML — proven by a test that only touches the docmodel builder and confirms HTML output is byte-identical before/after.
  2. One call to the shared entrypoint returns both rendered HTML+CSS and the JSON-serializable Model from a single parse pass (one parse, two sinks), not two separate render calls.
  3. `chase/profile` exists as its own package exposing a `Profile` interface (boundary detection, container wrapping, size table, pagination rules, profile-only directives) validated bottom-up — it is exactly what `profiles/slides` needs, not a speculative superset built before a second profile exists to test it against.
  4. `profiles/slides` is the only registered `profile.Profile` implementation, fully reproduces Marp-compatible slide behavior (16:9 default, `section` container, `paginate` semantics) against Objective 0's corpus, and neither `chase/model` nor `chase/theme` contains slide-specific naming or hardcoded assumptions (`Slide` type names, a hardcoded `section` selector) — grep-verifiable, confirming the anti-pattern research flags most strongly was avoided.
**Plans**: 4 TRDs in 3 waves
- [x] 02-01-TRD.md — chase/model: structured docmodel builder via direct finalized-AST walk (MODEL-01) [wave 1]
- [x] 02-02-TRD.md — chase/profile: Profile interface + registry + exported-vs-internal decision (MODEL-03) [wave 1]
- [x] 02-03-TRD.md — profiles/slides (only impl) + de-hardcode chase/theme + grep-proof (MODEL-04) [wave 2]
- [x] 02-04-TRD.md — chase.go one-parse-two-sinks entrypoint: HTML+CSS+Model (MODEL-02) [wave 3]

> **Decision gate to resolve in this objective:** `chase/*` internal vs. exported Go — decide and document explicitly whether `chase/` gets an `internal/` prefix (forcing every consumer through `press/`) or stays independently importable for advanced consumers (e.g. a future `profiles/paged` built by someone other than Eden Press). Apply the decision consistently before Objective 3 builds the public API on top of it.
>
> **Resolved (during Objective 2 planning):** `chase/*` stays **EXPORTED** (no `internal/` prefix) — independently importable for advanced consumers / future profiles, per the library-first thesis; documented in `chase/profile/doc.go` (TRD 02-02).

### Objective 3: press/ Batteries + Public API
**Goal**: Deliver the Marp-Core-equivalent batteries and the stable, importable `press.Render()` API — the point at which Eden Press becomes embeddable in any Go service with zero Chrome dependency, and the gate every downstream consumer (CLI, exporters, Dart) waits on.
**Depends on**: Objective 2
**Requirements**: CORE-01, CORE-02, CORE-03, CORE-04, CORE-05, CORE-06, CORE-07, CORE-08, CORE-09, API-01, API-02, API-03
**Success Criteria** (what must be TRUE):
  1. `press.Render(md, opts)` returns `{HTML, CSS, Model, Comments, Meta}` for a deck exercising all 3 bundled themes (default/gaia/uncover, embedded verbatim via `go:embed` with preserved MIT headers), `size`/`math` global directives, GFM tables + strikethrough (rendered as `<s>` to match Marp) + hard-breaks, heading slugs, native shortcode/unicode emoji, chroma-highlighted code styled correctly by the bundled themes' `.hljs-*`-shaped CSS, and math (`$…$`/`$$…$$`) rendered as MathML with construct-detection routing heavy constructs (`\tag`, `\label`, complex `aligned`) to the `codeberg.org/go-latex/latex` SVG/PNG fallback, plus emitted auto-fit markers (`<!--fit-->`).
  2. `go list -deps ./press/...` contains no `chromedp` — enforced as an automated CI check, not a documented promise.
  3. HTML sanitization matches Marp's `xss` allow-list *behaviorally* (strip-vs-escape semantics documented and deliberately chosen, not assumed identical) via an adversarial round-trip test suite, explicitly including the GFM disallowed-raw-HTML-tag filter (`<script>`, `<iframe>`, `<style>`, `<textarea>`, etc.) that goldmark's GFM extension does not provide automatically, and the always-on directive/comment-parsing code path is validated as its own trust boundary.
  4. `Options`/`Output` types are documented and stable enough that a consumer only ever imports `press/` — never reaches into `chase/`/`profiles/` directly — to render a complete deck; this is the explicit gate at which Objective 7 (Dart binding) may begin.
**Plans**: 9 TRDs in 3 waves
- [x] 03-01-TRD.md — chase/markdown.ParseWithEngine seam + press Options/Output + deps + emoji-compat spike (API-03) [wave 1]
- [x] 03-02-TRD.md — compiled-CSS theme extraction + go:embed + name-keyed ThemeSet (CORE-01) [wave 1]
- [x] 03-03-TRD.md — strikethrough <s> override + GFM/hard-break/slug verify (CORE-03, CORE-04) [wave 2]
- [x] 03-04-TRD.md — emoji: goldmark-emoji Twemoji + bespoke unicode-literal parser (CORE-06) [wave 2]
- [x] 03-05-TRD.md — chroma highlight + CSS-grounded chroma→.hljs remap (CORE-07) [wave 2]
- [x] 03-06-TRD.md — math baseline: $/$$→MathML + construct-detect + PNG fallback (CORE-08) [wave 2]
- [x] 03-07-TRD.md — size/math global directives + auto-fit markers (CORE-02, CORE-09) [wave 2]
- [x] 03-08-TRD.md — sanitize: bluemonday Marp-parity policy + adversarial suite (CORE-05) [wave 2]
- [x] 03-09-TRD.md — press.Render compose (API-01) + no-chromedp CI gate (API-02) + capstone [wave 3]

> **Decision gate (baseline, hardened in Objective 8):** a first-pass acceptable MathML-quality threshold before auto-invoking the SVG/PNG fallback is decided here; the full converter-hardening pass and final fallback-trigger rule land in Objective 8.

### Objective 4: CLI (cmd/eden-press)
**Goal**: Ship the `eden-press` command-line tool as a thin, standard-Go consumer of `press/`.
**Depends on**: Objective 3
**Requirements**: CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, CLI-06
**Success Criteria** (what must be TRUE):
  1. `eden-press <in.md>` produces zero-JS static `bare`-style HTML output by default.
  2. Watch mode (`fsnotify`, non-recursive directory walk with atomic-save-safe filtering) rebuilds output automatically when the source file changes.
  3. Server mode serves local files with live-reload on request, and preview mode opens the rendered output in the user's default browser.
  4. `--theme`/`--theme-set` flags load custom themes, a YAML/JSON/TOML config file (via koanf) can supply the same options, and stdin (`-`) works as an input source.
**Plans**: 8 TRDs in 5 waves
- [x] 04-01-TRD.md — press.Options.ThemeCSS additive extension + BrowserFitJS re-export (Wave-0 enabler for CLI-05) [wave 1]
- [x] 04-02-TRD.md — cobra skeleton + go.mod deps + flag→Options surface + stdin/file input [wave 1]
- [x] 04-03-TRD.md — htmldoc bare-style zero-JS assembly + convert pipeline (CLI-01) [wave 2]
- [x] 04-04-TRD.md — koanf config loading: .marprc.* + precedence flags>env>file (CLI-06) [wave 2]
- [x] 04-05-TRD.md — --theme/--theme-set loading into press.Options.ThemeCSS (CLI-05) [wave 2]
- [x] 04-06-TRD.md — watch mode: scoped fsnotify + debounce + SSE reload channel (CLI-02) [wave 3] — commits da9ab7b/ec6df0a (reload Hub), 99c912b/2d48136 (runWatch)
- [x] 04-07-TRD.md — serve mode: static + convert-on-request + traversal guard + reuse SSE (CLI-03) [wave 4] — commit 65ad5a5
- [x] 04-08-TRD.md — preview (pkg/browser) + integration test + CLI-imports CI gate (CLI-04) [wave 5] — commits d6097c6/062f625

### Objective 5: convert/pdf + convert/png (chromedp raster export)
**Goal**: Deliver PDF and PNG/JPEG export via headless Chrome, isolated as the only Chrome-touching code in the module, with CI-hardened determinism.
**Depends on**: Objective 3
**Requirements**: EXP-01, EXP-02, EXP-04
**Success Criteria** (what must be TRUE):
  1. A rendered deck exports to PDF via `chromedp`'s `Page.PrintToPDF` (invoked inside an `ActionFunc`) with fixed viewport, pinned timezone/locale, and animations disabled for deterministic output.
  2. The same deck exports to per-slide PNG/JPEG via chromedp screenshots.
  3. Chrome discovery falls back correctly: system Chrome → `--browser-path`/`CHROME_PATH` env var → a documented pinned-download path — tested in CI against a container with no system Chrome pre-installed; STIX Two Math font-provisioning is documented as a required export-environment asset.
  4. CI runs export tests against a pinned `chromedp/headless-shell` version with `--disable-dev-shm-usage`, non-root execution, and a unique `--user-data-dir` per run, and PDF export specifically (not just PNG) is re-validated whenever the pinned Chrome version bumps.
  5. `go list -deps` on `chase/`, `press/`, and `profiles/` still shows zero `chromedp` after this objective adds `convert/` — the CI check from Objective 3 remains green.
**Plans**: 5 TRDs in 4 waves
- [x] 05-01-TRD.md — convert bootstrap (chromedp provisioned, gate stays green) + Chrome discovery chain + one-browser-many-tabs Session (EXP-04) [wave 1]
- [x] 05-02-TRD.md — shared determinism recipe (fixed viewport/UTC/locale/animation-kill) + SetDocumentContent loader + STIX Two Math font provisioning (EXP-04) [wave 2]
- [x] 05-03-TRD.md — convert/pdf: PrintToPDF via ActionFunc + CSS @page sizing + print-backgrounds + inline-SVG PDF smoke (EXP-01) [wave 3]
- [x] 05-04-TRD.md — convert/png: per-slide screenshots (PNG/JPEG) viewport-pinned + inline-SVG capture smoke (EXP-02) [wave 3]
- [x] 05-05-TRD.md — CI hardening capstone: no-system-Chrome pinned headless-shell container + PDF re-validation process + no-chromedp/addlicense gates stay green (EXP-04) [wave 4]

### Objective 6: convert/pptx (native OOXML)
**Goal**: Deliver editable-text-box PPTX export directly from the structured document model, with no Chrome dependency at all — a sibling workstream to Objective 5, not sequential after it.
**Depends on**: Objective 3
**Requirements**: EXP-03
**Success Criteria** (what must be TRUE):
  1. A `.pptx` file is generated directly from `chase/model` (via `press.Output.Model`) — not from rendered HTML, and with zero `chromedp`/browser process invoked anywhere in the code path.
  2. Text content renders as real, editable OOXML text-box shapes (`<p:sp>` with actual text runs) — not one screenshot image dropped per slide.
  3. Generated PPTX position/size values are programmatically verified against expected EMU conversions via a dedicated, independently unit-tested EMU-conversion utility, including at least one grouped-shape (`chOff`/`chExt`) case.
  4. The generated file opens correctly in PowerPoint/LibreOffice with elements in their expected positions, verified on both a 16:9 and a 4:3 slide size.
**Plans**: 5 TRDs in 4 waves
- [x] 06-01-TRD.md — chase/model schema-v2: per-section Blocks (paragraph/list/code/math/heading) + press/math raw-TeX accessor — SHARED PREREQUISITE, unblocks Obj-7 DART-04 [wave 1]
- [x] 06-02-TRD.md — convert/pptx EMU-conversion utility + 16:9/4:3 slide-size constants + chOff/chExt group-transform, independently unit-tested (EXP-03) [wave 1] — complete 2026-07-21 (commits c5eb144, 6de5619; SUMMARY: 06-02-SUMMARY.md)
- [x] 06-03-TRD.md — deterministic OPC zip packager + complete static part graph (12-attr clrMap, 3-entry fmtScheme) + trivial-deck openability on 16:9/4:3 (EXP-03) [wave 2] — complete 2026-07-21 (commits eb2ada9, 5a98584, b209398, 95fe34c; SUMMARY: 06-03-SUMMARY.md)
- [x] 06-04-TRD.md — ToPPTX(model.Document, Options): Section→slide, Blocks/Outline→editable <p:sp> text boxes + grouped shape, no Chrome (EXP-03) [wave 3]
- [x] 06-05-TRD.md — speaker notes (Section.Notes→notesSlideN) + comprehensive openability/position verification on 16:9 + 4:3 (EXP-03) [wave 4]

> **Decision gate (must be confirmed before this objective's design doc is written):** hand-rolled OOXML (stdlib `archive/zip` + `encoding/xml`) is the confirmed approach — `unioffice` and any of its forks are explicitly rejected (AGPLv3/commercial-license-key requirement, incompatible with Eden Press's MIT/embeddable positioning). Re-confirm at planning time that no new permissive Go PPTX library has emerged since the research date.

### Objective 7: Dart/Flutter Binding (bind/capi + bind/dart)
**Goal**: Expose `press.Render()` to Flutter clients via a single Go core compiled three ways, gated only on Objective 3's public API being stable — not on the CLI or any exporter existing.
**Depends on**: Objective 3
**Requirements**: DART-01, DART-02, DART-03, DART-04, DART-05
**Success Criteria** (what must be TRUE):
  1. A single Go C-ABI core (`PressRender`/`PressFree`, JSON-in/JSON-out — not mirrored Dart/Go structs) builds three ways: `-buildmode=c-shared` (Android `.so`), `-buildmode=c-archive` (iOS `.a`), and `GOOS=js GOARCH=wasm` (Web) — with no `gomobile bind` anywhere in the build pipeline.
  2. Android and iOS builds are verified independently in CI as two separately-toolchained pipelines, specifically including a run on an Apple-Silicon runner with Android NDK also present (the confirmed toolchain-panic case) — both succeed.
  3. The Flutter package renders a deck's math via `flutter_math_fork` and code highlighting via `highlighting`/`flutter_highlighting` — no JavaScript anywhere in the Dart rendering surface.
  4. A shared subset of Objective 0's conformance corpus runs against the compiled capi/wasm artifact through the same JSON entrypoint the Dart code uses (not just the Go-native test run) and passes.
**Plans**: 5 TRDs in 3 waves
- [x] 07-01-TRD.md — DART-01: C-ABI core — RenderJSON pure-Go JSON boundary + cgo PressRender/PressFree [wave 1]
- [x] 07-02-TRD.md — DART-03: web binding — GOOS=js/wasm syscall/js shim + version-pinned wasm_exec.js [wave 2]
- [x] 07-03-TRD.md — DART-02: native builds — Android c-shared .so/ABI, iOS c-archive→xcframework + two-pipeline CI [wave 2]
- [x] 07-04-TRD.md — DART-05: boundary conformance — corpus subset through compiled wasm + host capi via same JSON entrypoint [wave 3]
- [x] 07-05-TRD.md — DART-04: JS-free Dart surface — flutter_math_fork + flutter_highlighting; consumes Obj6/06-01 schema-v2 [wave 3]

> **Decision gate (resolve before writing any WASM-specific code):** standard Go vs. TinyGo for the WASM target — decide based on a compatibility audit of goldmark, `yaml.v3` front-matter parsing, and the JSON-AST emitter against TinyGo's partial reflection/`encoding/json` support (a functional-correctness risk, not merely a binary-size/perf tradeoff). If TinyGo is chosen, pin its bundled `wasm_exec.js` to the exact compiler version.

### Objective 8: Math-Fidelity Hardening + Auto-Fit Resolution
**Goal**: Close the gap between "math renders without crashing" (Objective 3's baseline) and "math renders at KaTeX-parity quality with a concrete, tested fallback rule" — and resolve the one remaining browser-side-JS holdout (auto-fit). Last by necessity, not by low priority: there is nothing to tune until the rest of the pipeline renders something real.
**Depends on**: Objective 3, Objective 7
**Requirements**: None new — this objective hardens CORE-08 and CORE-09 (delivered in Objective 3) to production quality; it owns no v1 requirement IDs of its own.
**Success Criteria** (what must be TRUE):
  1. All 8 previously-wrong math-spike cases (big-operator limit stacking, `\binom`/`pmatrix` shared-fence bug, `\sqrt[n]` argument parsing, `aligned`→`mtable` conversion, `mathvariant`→Unicode-codepoint mapping) render at KaTeX-parity quality and are promoted into the permanent conformance-corpus regression set — not left as spike-scratch artifacts.
  2. A concrete, testable fallback-trigger detector correctly auto-routes `\tag`/`\label`/complex-multi-column-`aligned` constructs to the `go-latex/latex` SVG/PNG path, covered by a corpus test (not manual inspection) — reflecting the permanent Chromium MathML-Core structural ceiling (no `<mlabeledtr>`, limited `<mtable>` attributes), not a bug awaiting a fix.
  3. STIX Two Math is bundled from the STIX-fonts-project's own OTF/WOFF2 release files (never a Google Fonts CDN copy, which has been reported to strip MATH-table data), and a CI smoke test renders+pixel-checks a known formula to confirm MATH-table presence — catching tofu regressions before production.
  4. The auto-fit mechanism is resolved per the decision gate below and implemented with no remaining silent viewer-side JavaScript dependency.
**Plans**: 7 TRDs in 4 waves
  - [x] 08-01-TRD.md - Fork + vendor latex2mathml into internal/latex2mathml (go.mod replace directive, license/NOTICE) [wave 1] — commits 66fc2cf/1d6a38f (verbatim copy, behavior-identical; 5 converter patches deferred to 08-02/08-03)
  - [x] 08-02-TRD.md - Converter patches A: big-operator limit stacking (munderover tag-switch, Open Q1) + sqrt[n] radicand loss, structural MathML-DOM regression tests (criterion 1) [wave 2] — commits 113ce04/c77264e/063acb7; 4/8 spike cases (\sum/\prod/\lim/\sqrt[3]) at KaTeX-parity
  - [x] 08-03-TRD.md - Converter patches B: binom/pmatrix matched sized fence + aligned-to-mtable (in-fork MATRICES registration) + mathvariant-to-Unicode-codepoint (tokenizer font-drop fix + setFont) + TestSpikeCorpus all-8 lock (criterion 1 closed) [wave 3] — commits 7d6055c/feaf2b3/d786e78; all 8 spike cases at KaTeX-parity
  - [ ] 08-04-TRD.md - Finalize the fallback-trigger detector to the structural ceiling + routing corpus test (criterion 2) [wave 4]
  - [x] 08-05-TRD.md - STIX Two Math WOFF2 companion + Chrome-gated MATH-table pixel-check smoke (criterion 3) [wave 2] — commits 41f224f/69e8941 (official stipub v2.13 WOFF2, MATH-table survival verified two ways, Chrome-gated pixel-check smoke)
  - [x] 08-06-TRD.md - Remove viewer-side JS auto-fit from the HTML/CLI path (flag + BrowserFitJS + browser-fit.js) (criterion 4, web half) [wave 3] — commits 391bfe6/66477ec/53d8238/6e65a95; zero-<script> gate proven, CORE-09 markers confirmed unchanged
  - [x] 08-07-TRD.md - Native Flutter TextPainter auto-fit (shrink-only) for headings (criterion 4, Flutter half) [wave 1]

> **Decision gates:** (1) the concrete MathML fallback-trigger rule — which exact TeX constructs (confirmed candidates: `\tag`, `\label`, complex multi-column `aligned`) auto-route to the SVG/PNG fallback, as a testable detection function, not a vague heuristic; (2) the final auto-fit mechanism — native Flutter `TextPainter` fit (client-side, Objective 7's binding) vs. a CSS-only `cqw`/SVG-text spike for browser/PDF output (Objective 5's export path) vs. dropping auto-fit entirely if neither pixel-matches acceptably.

### Objective 9: Excel Formula Layer for Eden Spreadsheets
**Goal**: Maximize practical Excel formula coverage in the live grid evaluator while keeping `convert/xlsx` as a formula store/exporter (Excel remains authority for unsupported functions).
**Depends on**: PR #11 workbook + `bind/eden_ui` formula seam (already on main); no dependency on Objectives 0–8 beyond shipped packages.
**Requirements**: XLSX-F01, XLSX-F02, XLSX-F03, XLSX-F04, XLSX-F05, XLSX-F06
**Success Criteria** (what must be TRUE):
  1. `conformance/corpus/xlsxformula` exists and drives Dart evaluator tests (corpus-first, like `xlsxtyping`).
  2. Expression grammar supports comparisons, strings/bools, `&`, absolute/mixed refs, quoted sheet names.
  3. Named-function coverage hits Phase A (≥40) then Phase B (≥100), with Phase C (200–300) tracked against a pinned Microsoft category inventory — never marketed as full Excel 365 parity.
  4. Unsupported functions → `#NAME?` in grid; `.xlsx` still carries `<f>`; no stale `<v>` from a partial engine; `calcPr fullCalcOnLoad` when formulas exist.
  5. No chromedp in formula path; no AGPL/commercial OOXML adoption; `xlsxtyping` / D6 formula-vs-value distinction preserved.
**Plans**: 9 TRDs in 5 waves (see `objectives/09-xlsx-formula-layer/OBJECTIVE.md`)
- [x] 09-00 — Decision lock + corpus scaffold + coverage inventory [wave 0]
- [x] 09-01 — Grammar wave [wave 1]
- [x] 09-02 / 09-03 — Phase A functions (74 named) [wave 2]
- [ ] 09-04-TRD.md — Phase B Date & Time [wave 3]
- [ ] 09-05-TRD.md — Phase B Lookup [wave 3]
- [ ] 09-06-TRD.md — Phase B Conditional aggregates [wave 3]
- [~] 09-07 — calcPr fullCalcOnLoad landed; LO/Go-twin deferred [wave 4]
- [ ] 09-08-TRD.md — Phase C breadth + coverage dashboard / capstone [wave 4]

> **Decision gates:** (1) Dart-only evaluator through Phase B vs Go twin (research default: Dart-only; Go twin is a kill-criterial spike); (2) pin the Microsoft function inventory snapshot used as Phase C denominator; (3) hard outs stay hard — LAMBDA/dynamic-array family, Cube/Web, VBA, iterative circular calc.

## Progress

**Execution Order:**
Objectives 0 → 8 completed. Objective 9 is a post-roadmap insertion on the editable workbook / eden_ui seam (PR #11) and does not reopen Objectives 0–8.

**Parallel workstreams (via `/devflow:workstreams`):** Objectives 4, 5, 6, and 7 all depended on Objective 3 alone (not on each other) and could execute concurrently. Objective 8 was the v1 join point. Objective 9 Wave 2/3 TRDs (02∥03, 04∥05∥06) are eligible for parallel execution after Wave 0/1.

| Objective | Jobs Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 0. Conformance Corpus, Acceptance Gate & Attribution Bootstrap | 6/6 | Complete    | 2026-07-20 |
| 1. chase/markdown + chase/directive + chase/theme | 8/8 | Complete    | 2026-07-21 |
| 2. chase/model + chase/profile + profiles/slides | 4/4 | Complete    | 2026-07-21 |
| 3. press/ Batteries + Public API | 9/9 | Complete    | 2026-07-21 |
| 4. CLI (cmd/eden-press) | 8/8 | Complete    | 2026-07-21 |
| 5. convert/pdf + convert/png (chromedp) | 5/5 | Complete    | 2026-07-21 |
| 6. convert/pptx (native OOXML) | 5/5 | Complete    | 2026-07-21 |
| 7. Dart/Flutter Binding | 5/5 | Complete    | 2026-07-21 |
| 8. Math-Fidelity Hardening + Auto-Fit Resolution | 7/7 | Complete    | 2026-07-22 |
| 9. Excel Formula Layer for Eden Spreadsheets | 3.5/9 | In progress | — |

---
*Roadmap created: 2026-07-20*
*Last updated: 2026-10-30 — Objective 9 (Excel formula layer) planned*
*Depth: comprehensive (9 v1 objectives + Obj 9 post-v1 formula layer)*
