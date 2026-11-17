# convert/ — EXP-04 export surface: Chrome discovery, fonts, versioning

This document is the operator-facing surface for `convert/`'s Chrome-driven
raster export path (`convert/pdf.ToPDF`, `convert/png.ToImages`, both riding
on `convert/chrome.Session`). It covers four things a deployer/operator of a
custom export environment needs to know: how Chrome is located, why the
bundled STIX font matters, how the pinned Chrome version is managed, and what
"deterministic" actually means for this path.

`convert/` is the ONE package tree in this module that imports chromedp
(see `convert/doc.go`); `press/`, `chase/`, and `profiles/` never do, and
`scripts/check-no-chromedp.sh` mechanically enforces that boundary in CI.
Everything below is scoped to `convert/` and its subpackages only.

**`cmd/eden-press-export`** is the turnkey CLI consumer of this surface —
the operator-facing `eden-press-export <deck.md> -o out.pdf --format pdf`
command that drives exactly the discovery chain, STIX font bundling, and
version-pin process documented below, without requiring a caller to write
any Go. It is a SEPARATE, opt-in binary from the core `eden-press` CLI
(which stays chromedp-free); see AGENTS.md's "Export (separate binary)"
section for its flags, output convention, and exit codes.
`cmd/eden-press-export` itself contains no rasterization logic of its own —
it is a thin cobra wrapper around `convert/pdf.ToPDF` and
`convert/png.ToImages` — so `convert/` remains the SOLE chromedp boundary
this document describes.

## Chrome discovery chain

`convert/chrome.Discover` (`convert/chrome/discover.go`) resolves a
Chrome/Chromium executable in this strict order, stopping at the first tier
that resolves:

1. **Explicit override** — `convert.Options.BrowserPath` (equivalently
   `chrome.DiscoverOptions.BrowserPath`). Highest precedence; always wins if
   non-empty.
2. **`CHROME_PATH` environment variable** — read manually (it is NOT a
   chromedp built-in). This is an eden-press convention matching the
   `CHROME_PATH` variable Lighthouse and `marp-cli` already use, so operators
   coming from either tool get the behavior they expect for free.
3. **PATH auto-detection** — `Discover` confirms a known Chrome/Chromium
   binary name (`google-chrome`, `google-chrome-stable`, `chromium-browser`,
   `chromium`, `google-chrome-beta`, `google-chrome-unstable`,
   `/usr/bin/google-chrome`) exists on `PATH` via `exec.LookPath`, then hands
   back an EMPTY exec path on purpose — chromedp's own `ExecAllocator`
   performs its own (more exhaustive, platform-aware) auto-detection when no
   `ExecPath` option is supplied, so `Discover` just confirms "something is
   there" and steps aside.
4. **Nothing found** — `Discover` returns `ErrChromeNotFound`, whose message
   documents the remaining, deliberately-not-automated remedy: pin a
   `chromedp/headless-shell` container image (see below), or download a
   known-good build from Chrome for Testing's
   `https://googlechromelabs.github.io/chrome-for-testing/` JSON API and
   point `CHROME_PATH` (or `convert.Options.BrowserPath`) at its executable.
   Inside a `chromedp/headless-shell` image the binary is at
   `/headless-shell/headless-shell` — it does NOT match any of tier 3's
   candidate names, so a headless-shell-only environment resolves via tier 2
   (`CHROME_PATH`), never tier 3. A Chrome-for-Testing download's own
   archived executable is named plain `chrome`, also not on tier 3's list,
   for the same reason.

A custom export environment (a different container base, a bare-metal host,
etc.) must land Chrome/Chromium somewhere this chain can find it — either by
setting `CHROME_PATH`/`BrowserPath` explicitly (recommended, since it is
unambiguous) or by ensuring one of the tier-3 candidate names resolves on
`PATH`.

## STIX Two Math — a required export-environment asset

MathML rendered by `press/math`'s native backend (03-06 CORE-08) needs
STIX Two Math glyphs to display real characters instead of "tofu" (empty
boxes) for math operators/symbols outside the basic Latin range — a
documented Chrome/headless-shell font-availability gap (Pitfall 6) that has
nothing to do with the correctness of the emitted MathML itself.

`convert/chrome/fonts.go` bundles `convert/chrome/fonts/STIXTwoMath-Regular.otf`
(vendored, embedded via Go's `embed`) and injects it as a base64 `@font-face`
data URI into every composed export document (both `convert/pdf.ToPDF` and
`convert/png.ToImages` route through the same `chrome.ComposeCSS` helper).
This means:

- The font is **self-contained in the Go binary** — no filesystem font
  install, no system font package, no network fetch is required for MathML
  to render correctly in ANY export environment (dev, CI, container,
  production).
- A custom export environment (e.g. a from-scratch Docker image, a different
  `chromedp/headless-shell` derivative) does **not** need to install this
  font itself — it ships inside `convert/chrome`'s compiled binary. The one
  thing such an environment must NOT do is strip/rebuild `convert/chrome`
  without this embedded asset present; if you fork or vendor `convert/chrome`,
  keep `convert/chrome/fonts/STIXTwoMath-Regular.otf` intact.
- The real pixel-level "did this glyph actually render, and does it look
  right" verification (a MATH-table smoke test comparing rendered glyphs
  pixel-for-pixel) is Objective 8's scope, along with any final sourcing
  decision for the font asset. This capstone (05-05) only sanity-checks a
  byte-size floor on the exported PDF as a light tofu-blank guard — it is not
  a substitute for Objective 8's pixel check.

## Chrome version pinning + PDF-path re-validation process

`CHROME_VERSION` is pinned to an explicit `chromedp/headless-shell` tag in
exactly two places that must be bumped TOGETHER:

- `Makefile`'s `CHROME_VERSION` variable.
- `.github/workflows/ci.yml`'s `export` job (`env.CHROME_VERSION`, which
  parameterizes the job's `chromedp/headless-shell:${CHROME_VERSION}` build
  stage).

**Never pin to `latest`.** Two INDEPENDENTLY-DOCUMENTED Chrome regressions
have hit the PDF export path specifically, with no corresponding PNG/
screenshot-path symptom (05-RESEARCH Pitfall A):

- SVG-in-PDF rendering issues at Chrome >=108.
- A print-pipeline regression around Chrome ~125.

Because of this, **a PNG-path-only test pass does NOT imply the PDF path
still works** on any Chrome/headless-shell version change. The enforced
process rule (`scripts/check-chrome-export.sh`, wired into
`make check-chrome-export` and the CI `export` job) is:

1. Decide on a new `CHROME_VERSION` tag.
2. Bump it in BOTH the Makefile and `ci.yml`'s `export` job `env` block.
3. Re-run the PDF-path re-validation set — mechanically required to have run
   by `scripts/check-chrome-export.sh`, which fails the check if either did
   not execute:
   - `convert/pdf.TestToPDFInlineSVGFixture` (05-03 Test-list case 4 — the
     inline-SVG PDF conformance fixture, the direct regression smoke for
     Pitfall A).
   - `convert.TestCapstoneExportEndToEnd` (05-05 Task 1 — the full
     `press.Render` -> `pdf.ToPDF` + `png.ToImages` capstone).
4. Only accept the bump once both are green (or, outside a Chrome-provisioned
   environment, cleanly `t.Skip`ped — the CI `export` job is where they run
   for real, against the newly-pinned version, inside the no-system-Chrome
   container).

## Determinism scope: pixel-diff, not byte-identical

The Chrome-driven raster export path's acceptance bar is **pixel-diff under
a threshold**, not byte-identical output. This is a deliberate, explicit
scope distinction from the pure-Go `press/` render path (which targets
byte-identical HTML/CSS/Model output): Chrome's own headless-rendering team
has publicly acknowledged PRNG-based non-determinism in its rendering
pipeline with no committed deterministic-mode ship date (05-RESEARCH
Pitfall C). `convert/pdf`'s and `convert/png`'s own tests reflect this bar
directly — they assert PDF page count and `/MediaBox` dimensions match
across repeated runs, and PNG pixel sampling within a small per-channel
tolerance, but never raw byte-for-byte equality of the exported artifact.
Any consumer building a regression/golden-file check against this export
path should budget for the same pixel-diff-under-threshold bar, not bytewise
comparison.

## Untrusted documents: no network (eden-press#42)

Every tab a `Session` opens is put behind CDP Fetch-domain interception
before a document is loaded into it. Every network request the page makes is
paused and refused (`BlockedByClient`) unless `convert.Options.AllowRequest`
returns true for its URL; with no `AllowRequest`, all are refused. An export
of an attacker-supplied document therefore cannot make the export host fetch
internal services or cloud metadata (`169.254.169.254`) on its behalf.

The export path needs no network of its own: the document is injected with
`SetDocumentContent` into `about:blank`, and fonts are `data:` URIs. What
does change: a deck's remote `<img src="https://...">`, and the gaia theme's
`@import` of its web font, no longer load unless the caller allows them.
Allow only origins you trust, and compare parsed URLs, not string prefixes.

If interception cannot be enabled, `NewTab` fails closed: the tab's context
is cancelled and the caller's first `chromedp.Run` fails.

## Container hardening (Pitfall 11)

Every `convert/chrome.Session` (`convert/chrome/session.go`) launches with
these flags baked in as DEFAULTS, not opt-ins:

- **The sandbox stays on.** `--no-sandbox` is an explicit per-deployment
  opt-out (`convert.Options.NoSandbox`, or `EDEN_PRESS_CHROME_NO_SANDBOX=1` in
  the environment), for a container where Chrome cannot launch sandboxed
  (root, or no user-namespace support) and the container supplies the
  isolation instead: non-root user, no privileges, no route to anything the
  export should not reach (eden-press#42).
- `--disable-dev-shm-usage` — container `/dev/shm` is often too small for
  Chrome's default shared-memory usage; disabling it avoids `BUS_ADRERR`
  crashes. The CI `export` job additionally sizes the container's shared
  memory generously (`--shm-size=1g`) as defense in depth.
- A **unique `--user-data-dir` per Session**, created via `os.MkdirTemp` and
  removed on `Close()` — avoids cross-run profile-directory contention if
  multiple Sessions exist concurrently, and guarantees every CI run starts
  from a clean profile.
- Fixed device-scale-factor, locale, and timezone (`chrome.ApplyDeterminism`)
  — independent of the host's display density/system locale/TZ, feeding the
  pixel-diff-under-threshold determinism bar above.

The pinned no-system-Chrome CI `export` job (`.github/workflows/ci.yml`)
layers non-root execution on top (`docker run --user <uid>:<uid>` against a
dedicated unprivileged container user) — proving the whole stack (discovery
+ launch hardening + the exporters themselves) works end-to-end in a
container that has no Chrome/Chromium anywhere in its filesystem other than
the one pinned `chromedp/headless-shell` binary copied in.
