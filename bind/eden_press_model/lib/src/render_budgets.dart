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

// Render budgets for eden_docs (eden-press#32). EdenDocsView paints a block in
// one frame, so a table, formula or code block a model or another user wrote
// could freeze it: a 20000x20 table never finished, 3000 nested \frac
// overflowed the stack, and 400 KB of JavaScript took 123 s to highlight. Each
// budget below is where the view stops doing the expensive thing and does a
// cheap, honest one instead. They live here, in the pure-Dart model package,
// so a host app (AODex) or a server can apply the same numbers before a
// document reaches the view.

/// Table rows painted inline. A longer table paints this many and offers
/// "Show all N rows", which pages the rest through a lazily built list.
const int maxTableRowsPainted = 100;

/// Table columns painted. Wider tables paint this many, in fixed-width cells
/// that scroll horizontally, and say how many columns are not shown.
const int maxTableColumnsPainted = 50;

/// TeX longer than this (UTF-16 code units) is shown as its source, not
/// typeset.
const int maxMathTexLength = 2048;

/// TeX whose braces nest deeper than this is shown as its source, not
/// typeset: flutter_math_fork recurses per level and overflowed the stack at
/// 3000 nested \frac.
const int maxMathBraceDepth = 32;

/// Code longer than this is shown as plain monospace text, not highlighted.
const int maxHighlightedCodeLength = 20000;

/// Plain code text painted. Longer code shows this much and says how many
/// characters are not shown: one 400 KB paragraph is itself slow to lay out.
const int maxPlainCodeLength = 50000;
