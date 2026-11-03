// Copyright (c) 2026 AO Cyber Systems
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
// SPDX-License-Identifier: MIT

// Package press is Eden Press's public API (API-03): the ONE package a
// consumer imports to render a complete Marp-compatible deck from Markdown
// -- HTML + packed CSS + the JSON-serializable document model -- with no
// JavaScript runtime, no Node, and no browser (OBJECTIVE.md success
// criterion 4).
//
// press.Render (defined in wave 3, TRD 03-09) is a SIBLING composition to
// chase.Render, not a wrapper: it drives its OWN battery-laden goldmark
// engine -- emoji (CORE-06), syntax highlighting via chroma (CORE-04/05),
// LaTeX-to-MathML (CORE-07/08), and an always-on bluemonday sanitize pass
// (CORE-05) -- through the SAME two-phase, one-parse flow chase already
// uses, via the additive chase/markdown.ParseWithEngine seam this objective
// introduces (TRD 03-01). It never touches chase.Render or chase/chase.go;
// every existing chase caller stays byte-for-byte unaffected.
//
// press.RenderModel (and its byte form RenderModelJSON) is the same pipeline
// entered from a v7 docmodel instead of Markdown: it returns the same Output
// -- same theme/profile CSS, one unit element per Section, sanitized HTML --
// so PDF, PNG and themed HTML can be produced from a document that is stored
// as a model. See render_model.go.
//
// This file (doc.go) carries only the package documentation. The public
// surface itself lives in options.go: the Options input struct and the
// Output result struct -- the frozen API-03 contract every wave-2 battery
// TRD and the wave-3 compose TRD (03-09) consume, and the shape Objective
// 7's Dart binding serializes over. Those types are defined once, here in
// press/, so the downstream binding never has to chase per-battery API
// churn.
//
// Invariant: press/ never imports a headless-browser driver. `go list -deps
// ./press/...` must never contain chromedp -- Eden Press renders HTML and
// structured data without ever launching a browser, and that promise is
// enforced from the very first file in this package.
package press
