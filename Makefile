# Copyright (c) 2026 AO Cyber Systems
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in
# all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#
# SPDX-License-Identifier: MIT

.PHONY: build vet test check-no-chromedp check-cli-imports export-test check-chrome-export export-binary-smoke

# build / vet / test mirror the CI gates for local convenience.
build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

# check-no-chromedp enforces API-02: the public render path (press/) and
# everything it composes (chase/, profiles/) must carry NO chromedp dependency.
# See scripts/check-no-chromedp.sh; wired into .github/workflows/ci.yml beside
# the addlicense header check.
check-no-chromedp:
	bash scripts/check-no-chromedp.sh

# check-cli-imports enforces CLI-04 (04-08-TRD.md): cmd/eden-press's OWN
# source must import ONLY press/ from the engine -- never chase/, profiles/,
# or chromedp directly (a transitive scan legitimately shows chase/profiles
# VIA press; this checks .Imports, the CLI's own DIRECT imports, only). See
# scripts/check-cli-imports.sh; wired into .github/workflows/ci.yml beside
# check-no-chromedp.
check-cli-imports:
	bash scripts/check-cli-imports.sh

# CHROME_VERSION is the SINGLE pinned chromedp/headless-shell tag eden-press's
# export path is validated against -- NEVER "latest" (05-RESEARCH Pitfall
# A/11: two independently-documented Chrome regressions hit the PDF export
# path ONLY, so a silent version drift can regress PDF export while every
# PNG/screenshot-path test keeps passing). This is the canonical value;
# .github/workflows/ci.yml's `export` job container tag references this SAME
# version -- bump both together, then re-run `make check-chrome-export`
# (which enforces the PDF-path re-validation rule) before accepting the
# bump. See convert/EXPORT.md for the full process.
CHROME_VERSION := 151.0.7922.34

# export-test runs convert/'s export test suite (convert/chrome,
# convert/pdf, convert/png, and the 05-05 press.Render->ToPDF/ToImages
# capstone) and the CLI's live-Chrome end-to-end tests
# (cmd/eden-press-export TestEndToEnd*), expecting Chrome to be discovered
# via the EXP-04 fallback chain. Chrome-presence-gated: cleanly t.Skips
# outside a provisioned Chrome/container. CI runs it in the pinned
# headless-shell container, so the CLI tests are judged against the pinned
# Chrome, not the runner image's.
export-test:
	CHROME_VERSION=$(CHROME_VERSION) go test ./convert/... ./cmd/eden-press-export/... -v

# check-chrome-export runs scripts/check-chrome-export.sh: the same export
# test suite PLUS the mechanically-enforced EXP-04 Chrome-version-pin +
# PDF-path re-validation process rule. Wired into the pinned
# no-system-Chrome CI export job (.github/workflows/ci.yml).
check-chrome-export:
	CHROME_VERSION=$(CHROME_VERSION) bash scripts/check-chrome-export.sh

# export-binary-smoke builds cmd/eden-press-export and drives a real pdf+png
# export of a fixture deck through the compiled binary -- the turnkey CLI
# path (05.1-01/05.1-02), distinct from export-test (which exercises
# convert/ directly, not the binary's argv/file-writing surface). Self
# -gating: the binary exits 3 when no Chrome is discoverable, which the
# script treats as a clean skip, so a local run without Chrome no-ops
# instead of failing. Wired into the CI export job beside export-test.
export-binary-smoke:
	bash scripts/export-binary-smoke.sh

# check-dart-native-scaffold verifies the eden_press Flutter plugin scaffolding
# (bind/dart/android, bind/dart/ios) still agrees with the native build scripts
# that feed it. The binaries themselves are gitignored, so nothing else in the
# repo connects the two sides — a moved output path would ship a plugin with no
# native library and fail at runtime on a device. See
# scripts/check-dart-native-scaffold.sh.
check-dart-native-scaffold:
	bash scripts/check-dart-native-scaffold.sh

# dart-native-vendor copies built native artifacts into the plugin. Run after
# scripts/build-android.sh and/or scripts/build-ios.sh.
dart-native-vendor:
	bash scripts/vendor-dart-native.sh

.PHONY: check-dart-native-scaffold dart-native-vendor
