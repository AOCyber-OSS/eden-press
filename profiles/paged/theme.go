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

package paged

import "go.aocyber.ai/eden-press/chase/profile"

// DocumentThemeName is the name the paged profile's default theme registers
// under, and the name a caller selects it by.
const DocumentThemeName = "document"

// DocumentThemeCSS is the paged profile's default theme (D17): type for a
// printed document, where the bundled Marp themes are slide themes (29px type,
// 78.5px padding, 22 lines on an A4 page).
//
// It sets type only. The page geometry is not here: the scaffold gives every
// page its 20 mm margin as padding, and press appends the page width and
// height rules after the theme. So the unit rule deliberately has no width,
// height or padding (a test pins that).
//
// It lives in Go, not in a .css file under themes/: that directory holds the
// vendored Marp CSS only (excluded from the license-header check, listed in
// NOTICE), and a .css file carrying this repository's license header would not
// load, because theme.Load requires the `@theme` block to be the LEADING
// comment. The string must therefore start with that comment.
const DocumentThemeCSS = `/*!
 * eden-press document theme: the paged profile's default.
 *
 * @theme document
 */

section {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif;
  font-size: 11pt;
  line-height: 1.5;
  color: #1f2328;
  background: #fff;
}

section > :first-child {
  margin-top: 0;
}

h1, h2, h3, h4, h5, h6 {
  font-weight: 600;
  line-height: 1.25;
  margin: 1.2em 0 0.4em;
}

h1 {
  font-size: 20pt;
}

h2 {
  font-size: 16pt;
}

h3 {
  font-size: 13pt;
}

h4 {
  font-size: 11pt;
}

h5, h6 {
  font-size: 10pt;
}

p, ul, ol, dl, blockquote, table, pre, figure {
  margin: 0 0 0.6em;
}

ul, ol {
  padding-left: 1.6em;
}

a {
  color: #0969da;
}

blockquote {
  color: #59636e;
}

hr {
  border: 0;
  border-top: 1px solid #d0d7de;
  margin: 1em 0;
}

img {
  max-width: 100%;
}

table {
  font-size: 10pt;
}

th {
  background: #f6f8fa;
  font-weight: 600;
}

code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
  font-size: 0.9em;
  background: #eff1f3;
  padding: 0.1em 0.3em;
  border-radius: 4px;
}

/* Wide code wraps instead of running off the page: a printed page cannot
   scroll. */
pre {
  background: #f6f8fa;
  padding: 0.75em 1em;
  border-radius: 6px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

pre code {
  background: none;
  padding: 0;
  border-radius: 0;
}

header, footer {
  font-size: 9pt;
  color: #59636e;
}

.hljs-comment, .hljs-quote {
  color: #59636e;
  font-style: italic;
}

.hljs-keyword, .hljs-selector-tag, .hljs-type {
  color: #cf222e;
}

.hljs-string, .hljs-regexp {
  color: #0a3069;
}

.hljs-number, .hljs-literal, .hljs-built_in {
  color: #0550ae;
}

.hljs-title, .hljs-section {
  color: #8250df;
}
`

// The paged profile supplies its own default theme.
var _ profile.DefaultThemer = paged{}

// DefaultTheme returns the document theme (profile.DefaultThemer). press
// registers it before any caller theme, so a caller theme also named
// `document` replaces it, and selects it only when the document names no
// theme (no Options.Theme, no `theme` directive).
func (paged) DefaultTheme() (name, css string) {
	return DocumentThemeName, DocumentThemeCSS
}
