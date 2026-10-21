#!/usr/bin/env bash
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

# check-cli-imports.sh is the mechanical enforcement of CLI-04's boundary
# gate (04-08-TRD.md): cmd/eden-press's OWN source may import press/ (and
# its subpackages, e.g. press/themes, press/math) plus stdlib/third-party
# deps, but must NEVER directly import chase/, profiles/, or chromedp. This
# is the CLI analogue of API-02's check-no-chromedp.sh gate, one level down
# at the "CLI is a consumer of press/ only" boundary (the Objective-3
# consumer boundary).
#
# This checks .Imports -- the package's OWN DIRECT imports -- NOT a
# transitive `go list -deps` scan, which would legitimately show
# chase/profiles VIA press (allowed; the boundary is about the CLI's own
# source, not what press/ composes underneath it).
#
# Wired into CI (.github/workflows/ci.yml) beside check-no-chromedp.sh and
# runnable locally via `make check-cli-imports`.
set -euo pipefail

echo "check-cli-imports: scanning direct imports of ./cmd/eden-press/..."

# .Imports is the package's OWN direct imports (transitive would show
# chase/profiles via press -- allowed). Match engine-internal roots only
# (/chase|/profiles|chromedp) so press subpackages (press/themes,
# press/math) never trip this gate.
offending="$(go list -f '{{range .Imports}}{{println .}}{{end}}' ./cmd/eden-press/... \
	| sort -u | grep -E 'AOCyber-OSS/eden-press/(chase|profiles)|chromedp' || true)"

if [ -n "$offending" ]; then
	echo "FAIL: cmd/eden-press directly imports engine internals (must import only press/):" >&2
	echo "$offending" | sed 's/^/  /' >&2
	exit 1
fi

echo "PASS: cmd/eden-press imports only press/ (no direct chase/ profiles/ chromedp)."
