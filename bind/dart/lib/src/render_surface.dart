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

/// The JS-free rendering surface (DART-04): builds a Flutter widget tree
/// directly from `Output.Model`'s schema-v2 [Block]s -- never from
/// `Output.html` (see model.dart's library doc / 07-05-TRD.md anti_patterns,
/// Pitfall 3: presentation MathML carries no TeX annotation and chroma spans
/// are lossy DOM soup, so HTML is not a viable source for these two blocks).
///
/// - `math` blocks render via `flutter_math_fork`'s `Math.tex` from the raw
///   TeX in [Block.text], with [Block.display] selecting [MathStyle.display]
///   vs [MathStyle.text].
/// - `code` blocks render via `flutter_highlighting`'s `HighlightView` from
///   the raw source in [Block.text] and the fenced info-string in
///   [Block.language].
/// - `heading` blocks render via [FitText] (08-07-TRD.md, RESOLVED DECISION
///   #4's Flutter half): a native `TextPainter` measure-then-binary-search
///   SHRINK-ONLY auto-fit, so an oversized heading shrinks to its allotted
///   slide width -- the Flutter-native equivalent of Marp's `<!--fit-->`
///   shrink, with zero JavaScript.
/// - `paragraph`/`list`/unknown blocks render as plain Flutter text -- not
///   this TRD's focus, but rendered so the surface is complete.
///
/// No HTML/DOM-parsing package, no embedded browser surface, no JavaScript
/// is imported or invoked anywhere in this file.
library;

import 'package:flutter/material.dart';
import 'package:flutter_highlighting/flutter_highlighting.dart';
import 'package:flutter_highlighting/themes/github.dart';
import 'package:flutter_math_fork/flutter_math.dart';

import 'fit_text.dart';
import 'model.dart';

/// Default `flutter_highlighting` theme for `code` blocks: GitHub's, chosen
/// for neutral, high-contrast readability. [EdenPressView.highlightTheme]
/// can override it; picking a different default is a presentation decision,
/// not a DART-04 correctness concern.
const Map<String, TextStyle> defaultHighlightTheme = githubTheme;

/// Renders an [Output]'s schema-v2 [ModelDocument] natively, in document
/// order, from its raw [Block] content -- the JS-free surface DART-04 exists
/// to prove.
class EdenPressView extends StatelessWidget {
  const EdenPressView(
    this.output, {
    super.key,
    this.highlightTheme = defaultHighlightTheme,
  });

  /// The decoded render output (typically from [render]/`eden_press.dart`),
  /// carrying the schema-v2 [ModelDocument] this view walks.
  final Output output;

  /// The `flutter_highlighting` theme used for `code` blocks.
  final Map<String, TextStyle> highlightTheme;

  @override
  Widget build(BuildContext context) {
    final blocks = <Widget>[
      for (final section in output.model.sections)
        for (final block in section.blocks) _buildBlock(block),
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: blocks,
    );
  }

  Widget _buildBlock(Block block) {
    switch (block.kind) {
      case BlockKind.math:
        return Math.tex(
          block.text,
          mathStyle: block.display ? MathStyle.display : MathStyle.text,
        );
      case BlockKind.code:
        return HighlightView(
          block.text,
          languageId: block.language,
          theme: highlightTheme,
        );
      case BlockKind.heading:
        return FitText(block.text, style: _headingStyle(block.level));
      case BlockKind.list:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            for (final item in block.items)
              Text(
                '${'  ' * item.level}${block.ordered ? '-' : '•'} ${item.text}',
              ),
          ],
        );
      case BlockKind.table:
        return _buildTable(block);
      case BlockKind.quote:
        // A quote is only distinguishable from prose because schema v3
        // (EPD-R1) gave it its own kind; before that it arrived as an
        // indistinguishable paragraph and could not be styled as a quote.
        return Container(
          margin: const EdgeInsets.symmetric(vertical: 6),
          padding: const EdgeInsets.only(left: 12),
          decoration: const BoxDecoration(
            border: Border(
              left: BorderSide(color: Color(0xFFBDBDBD), width: 3),
            ),
          ),
          child: Text(
            block.text,
            style: const TextStyle(fontStyle: FontStyle.italic),
          ),
        );
      case BlockKind.image:
        // The render path performs NO I/O -- it never fetches a remote URL --
        // so the alt text is what must survive. Rendering nothing would drop
        // the content silently, which is exactly the failure schema v3 exists
        // to stop.
        return Text(
          '[${block.text.isEmpty ? block.src : block.text}]',
          style: const TextStyle(
            fontStyle: FontStyle.italic,
            color: Color(0xFF757575),
          ),
        );
      case BlockKind.toggle:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              block.text,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            Padding(
              padding: const EdgeInsets.only(left: 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  for (final child in block.children) _buildBlock(child),
                ],
              ),
            ),
          ],
        );
      case BlockKind.synced:
        final ref = block.syncRef?['blockId'] ?? '';
        return Text(
          '[synced:$ref]',
          style: const TextStyle(
            fontStyle: FontStyle.italic,
            color: Color(0xFF757575),
          ),
        );
      case BlockKind.todo:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('${block.checked ? '☑' : '☐'} ${_plain(block)}'),
            for (final child in block.children) _buildBlock(child),
          ],
        );
      case BlockKind.callout:
        return Container(
          margin: const EdgeInsets.symmetric(vertical: 6),
          padding: const EdgeInsets.all(8),
          color: const Color(0xFFF5F5F5),
          child: Text(
            block.icon.isEmpty ? _plain(block) : '${block.icon} ${_plain(block)}',
          ),
        );
      case BlockKind.divider:
        return const Divider();
      case BlockKind.file:
        final label = block.name.isNotEmpty ? block.name : block.src;
        return Text(
          '[$label]',
          style: const TextStyle(color: Color(0xFF757575)),
        );
      case BlockKind.sheet:
        return Text(
          '[sheet:${block.sheetEmbed?['sheet'] ?? ''}]',
          style: const TextStyle(fontStyle: FontStyle.italic),
        );
      case BlockKind.figure:
        final alt = block.text.isNotEmpty ? block.text : (block.title.isNotEmpty ? block.title : 'figure');
        return Text(
          '[$alt]',
          style: const TextStyle(fontStyle: FontStyle.italic, color: Color(0xFF757575)),
        );
      case BlockKind.paragraph:
      case BlockKind.unknown:
        return Text(_plain(block));
    }
  }

  String _plain(Block block) {
    if (block.runs.isNotEmpty) {
      return block.runs.map((r) => r.text).join();
    }
    return block.text;
  }

  /// Renders a `table` block. Its payload is in [Block.headers]/[Block.rows],
  /// never in [Block.text] -- falling through to the paragraph branch would
  /// render an empty widget and silently lose the whole table.
  ///
  /// Rows are padded/truncated to the column count: Flutter's Table asserts
  /// that every row has equal length, and chase/model deliberately reports
  /// rows exactly as authored, so normalizing is the consumer's job.
  Widget _buildTable(Block block) {
    final int columns =
        block.headers.isNotEmpty
            ? block.headers.length
            : block.rows.fold<int>(0, (m, r) => r.length > m ? r.length : m);
    if (columns == 0) {
      return const SizedBox.shrink();
    }

    TextAlign alignOf(int i) {
      if (i >= block.aligns.length) return TextAlign.start;
      switch (block.aligns[i]) {
        case 'right':
          return TextAlign.right;
        case 'center':
          return TextAlign.center;
        default:
          return TextAlign.start;
      }
    }

    TableRow row(List<String> cells, {required bool header}) => TableRow(
      children: List<Widget>.generate(columns, (i) {
        final text = i < cells.length ? cells[i] : '';
        return Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
          child: Text(
            text,
            textAlign: alignOf(i),
            style: header ? const TextStyle(fontWeight: FontWeight.bold) : null,
          ),
        );
      }),
    );

    return Table(
      border: TableBorder.all(color: const Color(0xFFBDBDBD), width: 0.5),
      defaultVerticalAlignment: TableCellVerticalAlignment.middle,
      children: [
        if (block.headers.isNotEmpty) row(block.headers, header: true),
        for (final r in block.rows) row(r, header: false),
      ],
    );
  }

  TextStyle _headingStyle(int level) {
    final double size = switch (level) {
      1 => 28.0,
      2 => 24.0,
      3 => 20.0,
      _ => 18.0,
    };
    return TextStyle(fontSize: size, fontWeight: FontWeight.bold);
  }
}
