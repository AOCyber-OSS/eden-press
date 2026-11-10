# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

eden-press is a Go engine that generates documents from Markdown. It is Marp-compatible but a
clean-room implementation — explicitly **not affiliated with Marp**. The backend has **zero
JavaScript**. It is library-first, CLI second (`kind: library`), which means strict test-first
posture per the global by-kind playbook: write the test before the implementation.

Packages: `bind/` (dart, wasm, capi bindings), `chase/`, `press/`, `profiles/`, `convert/`,
`conformance/`, `cmd/eden-press/`, `tools/`.

## Commands

```bash
go build ./...
go vet ./...
go test ./...          # includes conformance/... except conformance/consumer
make consumer-test     # conformance/consumer: eden-press as a downstream module sees it
make check-no-chromedp
make check-cli-imports
```

CI: `ci.yml` (build/vet/test/license-header), `dart-native.yml`, `upstream-drift.yml` (auto-files an
issue when upstream Marp releases exceed the pins in `UPSTREAM-VERSIONS.txt`; current pins:
marpit v3.2.2, marp-core v4.4.0, marp-cli v4.5.0).

## Hard Rules (load-bearing — do not violate)

1. **Never run `go mod tidy`.** `go.mod`/`go.sum` are hand-authored and version-pinned. Adding a
   dependency is additive-only: `go mod download <module>` + manual edit. `tidy` prunes requires that
   sibling worktree branches legitimately still need.
2. **License headers are enforced by `addlicense -check` in CI**, and there are two templates:
   - Original files: MIT / AO Cyber Systems — `addlicense -l mit -s -c "AO Cyber Systems" -y 2026`.
   - The 3 vendored Marp theme files (default, gaia, uncover) plus the browser-fit script keep
     **Marp's original copyright and year** — never relabel these to AO Cyber Systems.
3. **`make check-no-chromedp`** — the public render path (`press/`, `chase/`, `profiles/`) must carry
   **zero** chromedp dependency. chromedp is only permitted in the `convert/` export path.
4. **`make check-cli-imports`** — `cmd/eden-press` may import **only** `press/` directly.
5. **Chrome/PDF export uses a pinned `CHROME_VERSION`** in the Makefile (currently `151.0.7922.34`).
   Never bump this to "latest" — two documented Chrome regressions have hit the PDF path.

## DevFlow Routing

The DevFlow plugin (`devflow@aocyber`) is installed on this repo. When a request fits a DevFlow
workflow, prefer the matching `/devflow:` skill (build, plan-objective, execute-objective, verify-work,
debug) over ad-hoc edits — it enforces atomic commits, state tracking, and verification. Run
`/devflow:help` to list all commands.
