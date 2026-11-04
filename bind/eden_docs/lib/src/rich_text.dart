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

import 'package:eden_press_model/eden_press_model.dart';
import 'package:flutter/material.dart';

/// Builds a [TextSpan] tree from schema-v7 [TextRun]s (or plain [Block.text]).
InlineSpan richSpanFor(Block block, {TextStyle? style}) {
  final base = style ?? const TextStyle(fontSize: 16, height: 1.45);
  if (block.runs.isEmpty) {
    return TextSpan(text: block.text, style: base);
  }
  return TextSpan(
    style: base,
    children: [for (final run in block.runs) _spanForRun(run, base)],
  );
}

/// Builds a [TextSpan] tree for one table cell from its [runs], styled exactly
/// as [richSpanFor] styles paragraph runs (they share [_spanForRun]). [text] is
/// the plain fallback used when [runs] is empty. A `math` run has no special
/// style here, as in paragraphs: it renders as its TeX text.
///
/// [style] is the cell's base style. It is deliberately not defaulted to the
/// paragraph style (16 px, 1.45 line height): a table cell inherits the
/// ambient text style, so a styled cell must not stand out from a plain one.
InlineSpan richSpanForRuns(
  List<TextRun> runs,
  String text, {
  TextStyle? style,
}) {
  final base = style ?? const TextStyle();
  if (runs.isEmpty) {
    return TextSpan(text: text, style: base);
  }
  return TextSpan(
    style: base,
    children: [for (final run in runs) _spanForRun(run, base)],
  );
}

TextSpan _spanForRun(TextRun run, TextStyle base) {
  var style = base;
  for (final mark in run.marks) {
    switch (mark) {
      case 'bold':
        style = style.copyWith(fontWeight: FontWeight.w700);
      case 'italic':
        style = style.copyWith(fontStyle: FontStyle.italic);
      case 'code':
        style = style.copyWith(
          fontFamily: 'monospace',
          backgroundColor: const Color(0xFFF0F0F0),
        );
      case 'strike':
        style = style.copyWith(decoration: TextDecoration.lineThrough);
    }
  }
  if (run.link.isNotEmpty) {
    style = style.copyWith(
      color: const Color(0xFF0B57D0),
      decoration: TextDecoration.underline,
    );
  }
  return TextSpan(text: run.text, style: style);
}
