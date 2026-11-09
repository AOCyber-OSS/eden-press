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

import 'dart:math' show max, min;

import 'package:eden_press_model/eden_press_model.dart';
import 'package:flutter/material.dart';
import 'package:flutter_highlighting/flutter_highlighting.dart';
import 'package:flutter_highlighting/themes/github.dart';
import 'package:flutter_math_fork/flutter_math.dart';
import 'package:flutter_svg/flutter_svg.dart';

import 'rich_text.dart';

/// The children EdenDocsView paints under [block], which sits at [depth] (a
/// section's own blocks are at depth 1), and how many blocks of its subtree it
/// leaves out.
///
/// A stored document can nest deeper than [maxNestingDepth] (one written
/// before the Go side enforced the limit, eden-press#29), and every level used
/// to add a Column, Container and Padding: depth 2000 took 91 s per pump. So
/// the view nests no deeper than the limit. A block at depth
/// `maxNestingDepth - 1` paints the blocks of its subtree as one flat run of
/// children, in document order, and a block at the limit paints no children
/// of its own (they are already in that run). The run sits in one container
/// and is laid out in one frame, so it stops after [maxFlattenedBlocks] and
/// [hidden] counts the rest. The walk is iterative: a 4900-deep chain costs
/// no stack.
({List<Block> blocks, int hidden}) paintedChildren(Block block, int depth) {
  if (depth >= maxNestingDepth) return (blocks: const <Block>[], hidden: 0);
  if (depth < maxNestingDepth - 1 ||
      !block.children.any((c) => c.children.isNotEmpty)) {
    return (blocks: block.children, hidden: 0);
  }
  final flat = <Block>[];
  var hidden = 0;
  final stack = <Block>[...block.children.reversed];
  while (stack.isNotEmpty) {
    final b = stack.removeLast();
    if (flat.length < maxFlattenedBlocks) {
      flat.add(b);
    } else {
      hidden++;
    }
    stack.addAll(b.children.reversed);
  }
  return (blocks: flat, hidden: hidden);
}

/// The line that stands for the blocks [paintedChildren] left out.
Widget _hiddenBlocks(int hidden) => _budgetNote(
  '$hidden more blocks are not shown here: this part of the document is '
  'nested $maxNestingDepth levels deep.',
);

/// Paints one schema-v7 [Block], which sits at [depth], as a document widget
/// (never a JSON dump). [nest] paints a child one level deeper; children come
/// from [paintedChildren], which keeps the view within [maxNestingDepth].
Widget buildDocBlock(
  Block block, {
  required int depth,
  required Widget Function(Block) nest,
}) {
  // Built only by the container kinds below (quote, todo, callout, toggle);
  // the other kinds never paint children.
  List<Widget> children() {
    final painted = paintedChildren(block, depth);
    return [
      for (final child in painted.blocks) nest(child),
      if (painted.hidden > 0) _hiddenBlocks(painted.hidden),
    ];
  }

  switch (block.kind) {
    case BlockKind.heading:
      return Padding(
        padding: const EdgeInsets.only(top: 12, bottom: 8),
        child: Text.rich(richSpanFor(block, style: _headingStyle(block.level))),
      );
    case BlockKind.paragraph:
    case BlockKind.unknown:
      return Padding(
        padding: const EdgeInsets.only(bottom: 10),
        child: Text.rich(richSpanFor(block)),
      );
    case BlockKind.quote:
      return Container(
        width: double.infinity,
        margin: const EdgeInsets.symmetric(vertical: 8),
        padding: const EdgeInsets.fromLTRB(14, 8, 8, 8),
        decoration: const BoxDecoration(
          border: Border(left: BorderSide(color: Color(0xFFBDBDBD), width: 3)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text.rich(
              richSpanFor(
                block,
                style: const TextStyle(
                  fontSize: 16,
                  height: 1.45,
                  fontStyle: FontStyle.italic,
                  color: Color(0xFF5F6368),
                ),
              ),
            ),
            ...children(),
          ],
        ),
      );
    case BlockKind.list:
      return Padding(
        padding: const EdgeInsets.only(bottom: 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            for (var i = 0; i < block.items.length; i++)
              Padding(
                padding: EdgeInsets.only(
                  // An item's depth is its list's depth plus its level, so
                  // the indent stops growing at maxNestingDepth (a level of
                  // 4900 would otherwise push the text 78k px off screen).
                  left:
                      8.0 +
                      16.0 *
                          block.items[i].level.clamp(
                            0,
                            max(0, maxNestingDepth - depth),
                          ),
                  bottom: 4,
                ),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    SizedBox(
                      width: 22,
                      child: Text(
                        block.ordered ? '${i + 1}.' : '•',
                        style: const TextStyle(fontSize: 16, height: 1.45),
                      ),
                    ),
                    Expanded(
                      child: Text(
                        block.items[i].text,
                        style: const TextStyle(fontSize: 16, height: 1.45),
                      ),
                    ),
                  ],
                ),
              ),
          ],
        ),
      );
    case BlockKind.todo:
      return Padding(
        padding: const EdgeInsets.only(bottom: 6),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  block.checked
                      ? Icons.check_box
                      : Icons.check_box_outline_blank,
                  size: 22,
                  color: block.checked
                      ? const Color(0xFF1E8E3E)
                      : const Color(0xFF5F6368),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text.rich(
                    richSpanFor(
                      block,
                      style: TextStyle(
                        fontSize: 16,
                        height: 1.45,
                        decoration: block.checked
                            ? TextDecoration.lineThrough
                            : null,
                        color: block.checked
                            ? const Color(0xFF5F6368)
                            : const Color(0xFF202124),
                      ),
                    ),
                  ),
                ),
              ],
            ),
            Padding(
              padding: const EdgeInsets.only(left: 30),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: children(),
              ),
            ),
          ],
        ),
      );
    case BlockKind.callout:
      final tone = _tone(block.tone);
      return Container(
        width: double.infinity,
        margin: const EdgeInsets.symmetric(vertical: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: tone.background,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: tone.border),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(tone.icon, color: tone.foreground, size: 20),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text.rich(
                    richSpanFor(
                      block,
                      style: TextStyle(
                        fontSize: 15,
                        height: 1.4,
                        color: tone.foreground,
                      ),
                    ),
                  ),
                  ...children(),
                ],
              ),
            ),
          ],
        ),
      );
    case BlockKind.toggle:
      return Theme(
        data: ThemeData(dividerColor: Colors.transparent),
        child: ExpansionTile(
          initiallyExpanded: !block.collapsed,
          tilePadding: EdgeInsets.zero,
          childrenPadding: const EdgeInsets.only(left: 12, bottom: 8),
          title: Text.rich(
            richSpanFor(
              block,
              style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
            ),
          ),
          children: children(),
        ),
      );
    case BlockKind.divider:
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 12),
        child: Divider(thickness: 1, color: Color(0xFFE0E0E0)),
      );
    case BlockKind.code:
      return Container(
        width: double.infinity,
        margin: const EdgeInsets.symmetric(vertical: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: const Color(0xFFF6F8FA),
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: const Color(0xFFE0E0E0)),
        ),
        child: _code(block),
      );
    case BlockKind.math:
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: _math(block),
      );
    case BlockKind.table:
      // Keyed by the block, so "Show all" state never carries over to a
      // different table painted at the same place.
      return _BudgetedTable(block, key: ObjectKey(block));
    case BlockKind.image:
      return _imageCard(
        caption: block.text.isEmpty ? block.src : block.text,
        subtitle: block.src,
      );
    case BlockKind.file:
      final label = block.name.isNotEmpty
          ? block.name
          : (block.text.isNotEmpty ? block.text : block.src);
      return Card(
        margin: const EdgeInsets.symmetric(vertical: 8),
        elevation: 0,
        color: const Color(0xFFF8F9FA),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: const BorderSide(color: Color(0xFFE0E0E0)),
        ),
        child: ListTile(
          leading: const Icon(Icons.insert_drive_file_outlined),
          title: Text(label.isEmpty ? 'File' : label),
          subtitle: block.mime.isEmpty && block.src.isEmpty
              ? null
              : Text(
                  [
                    if (block.mime.isNotEmpty) block.mime,
                    if (block.src.isNotEmpty) block.src,
                  ].join(' · '),
                ),
        ),
      );
    case BlockKind.sheet:
      return _sheetEmbed(block);
    case BlockKind.figure:
      return _figure(block);
    case BlockKind.synced:
      final ref = block.syncRef?['blockId'] ?? '';
      return Chip(
        avatar: const Icon(Icons.sync, size: 16),
        label: Text(ref.isEmpty ? 'Synced block' : 'Synced · $ref'),
      );
  }
}

const _codeStyle = TextStyle(fontFamily: 'monospace', fontSize: 13);

/// A code block: highlighted up to [maxHighlightedCodeLength], plain
/// monospace text above it (flutter_highlighting took 123 s on 400 KB), and
/// cut at [maxPlainCodeLength] with a count of what is not shown, since one
/// huge paragraph is itself slow to lay out (eden-press#32).
Widget _code(Block block) {
  final text = block.text;
  if (text.length <= maxHighlightedCodeLength) {
    return HighlightView(
      text,
      languageId: block.language.isEmpty ? 'plaintext' : block.language,
      theme: githubTheme,
      textStyle: _codeStyle,
    );
  }
  if (text.length <= maxPlainCodeLength) return Text(text, style: _codeStyle);
  // Never end on the first half of a surrogate pair: the text would not be
  // well-formed UTF-16 and would not paint.
  var cut = maxPlainCodeLength;
  final last = text.codeUnitAt(cut - 1);
  if (last >= 0xD800 && last <= 0xDBFF) cut--;
  return Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(text.substring(0, cut), style: _codeStyle),
      _budgetNote('${text.length - cut} more characters are not shown.'),
    ],
  );
}

/// A math block: typeset by flutter_math_fork when the TeX is within
/// [maxMathTexLength] and [maxMathBraceDepth], otherwise shown as its TeX
/// source, as it is when the TeX does not parse. 3000 nested \frac overflowed
/// the stack inside the typesetter, so the budget is checked before it runs
/// (eden-press#32).
Widget _math(Block block) {
  final tex = block.text;
  if (tex.length > maxMathTexLength ||
      braceDepth(tex) > maxMathBraceDepth ||
      definesMacro(tex)) {
    return _texSource(tex);
  }
  return Math.tex(
    tex,
    mathStyle: block.display ? MathStyle.display : MathStyle.text,
    onErrorFallback: (_) => _texSource(tex),
  );
}

Widget _texSource(String tex) => Text(tex, style: _codeStyle);

/// Whether [tex] defines a macro (`\newcommand`, `\renewcommand`, `\def`) or
/// uses a macro parameter (`#`). The length and depth budgets measure the
/// source, and a short macro can expand into millions of glyphs, so such TeX
/// is shown as source rather than typeset.
bool definesMacro(String tex) =>
    tex.contains('#') ||
    tex.contains(r'\newcommand') ||
    tex.contains(r'\renewcommand') ||
    tex.contains(r'\def');

/// The deepest `{` nesting in [tex], ignoring escaped braces (`\{`). It stops
/// counting once past [maxMathBraceDepth]; the walk is linear either way.
int braceDepth(String tex) {
  var depth = 0;
  var deepest = 0;
  for (var i = 0; i < tex.length; i++) {
    final c = tex.codeUnitAt(i);
    if (c == 0x5C) {
      i++; // skip the escaped character
    } else if (c == 0x7B) {
      depth++;
      if (depth > deepest) deepest = depth;
      if (deepest > maxMathBraceDepth) return deepest;
    } else if (c == 0x7D && depth > 0) {
      depth--;
    }
  }
  return deepest;
}

Widget _budgetNote(String text) => Padding(
  padding: const EdgeInsets.only(top: 6),
  child: Text(
    text,
    style: const TextStyle(fontSize: 13, color: Color(0xFF5F6368)),
  ),
);

/// A table block. Within [maxTableRowsPainted] rows,
/// [maxTableColumnsPainted] columns and [maxTableCellsPainted] cells it is the
/// plain Table it always was.
/// Past either budget (eden-press#32: a 20000x20 table never finished one
/// frame, and 5000 columns took 9.8 s) it paints at most that many columns,
/// in fixed-width cells that scroll horizontally, and at most that many
/// rows, with "Show all N rows" switching to a list that builds rows only as
/// they scroll into view.
class _BudgetedTable extends StatefulWidget {
  const _BudgetedTable(this.block, {super.key});

  final Block block;

  @override
  State<_BudgetedTable> createState() => _BudgetedTableState();
}

class _BudgetedTableState extends State<_BudgetedTable> {
  bool _showAll = false;

  static const double _cellWidth = 160;
  static const double _rowExtent = 32;
  static const double _allRowsHeight = 480;

  @override
  Widget build(BuildContext context) {
    final block = widget.block;
    final columns = block.headers.isNotEmpty
        ? block.headers.length
        : block.rows.fold<int>(0, (m, r) => r.length > m ? r.length : m);
    if (columns == 0) return const SizedBox.shrink();
    final shownColumns = min(columns, maxTableColumnsPainted);
    // The cost is rows x columns, so the cell budget can bind before either
    // of the others (100 x 50 is within both and is 5000 cells).
    final rowBudget = min(
      maxTableRowsPainted,
      max(1, maxTableCellsPainted ~/ shownColumns),
    );
    final overBudget =
        columns > maxTableColumnsPainted || block.rows.length > rowBudget;
    if (!overBudget) {
      return Container(
        margin: const EdgeInsets.symmetric(vertical: 8),
        child: Table(
          border: _tableBorder,
          children: [
            if (block.headers.isNotEmpty)
              _tableRow(block.headers, block.headerRuns, columns, header: true),
            for (var r = 0; r < block.rows.length; r++)
              _tableRow(block.rows[r], _rowRuns(block, r), columns),
          ],
        ),
      );
    }

    final width = _cellWidth * shownColumns;
    final rowCount = block.rows.length;
    final Widget body;
    if (_showAll) {
      body = SizedBox(
        width: width,
        height: _allRowsHeight,
        child: ListView.builder(
          key: const Key('eden-docs-table-rows'),
          itemCount: rowCount,
          // A fixed extent is what makes a jump to row 19999 cheap: without
          // it the list lays out every row before the target. Each cell is
          // therefore one line, cut with an ellipsis (_fixedRow), never
          // clipped into its neighbour.
          itemExtent: _rowExtent,
          itemBuilder: (context, r) => _fixedRow(block.rows[r], shownColumns),
        ),
      );
    } else {
      final shownRows = min(rowCount, rowBudget);
      body = Table(
        border: _tableBorder,
        defaultColumnWidth: const FixedColumnWidth(_cellWidth),
        children: [
          for (var r = 0; r < shownRows; r++)
            _tableRow(block.rows[r], _rowRuns(block, r), shownColumns),
        ],
      );
    }
    return Container(
      margin: const EdgeInsets.symmetric(vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                if (block.headers.isNotEmpty)
                  Table(
                    border: _tableBorder,
                    defaultColumnWidth: const FixedColumnWidth(_cellWidth),
                    children: [
                      _tableRow(
                        block.headers,
                        block.headerRuns,
                        shownColumns,
                        header: true,
                      ),
                    ],
                  ),
                body,
              ],
            ),
          ),
          if (columns > shownColumns)
            _budgetNote(
              '${columns - shownColumns} more columns are not shown '
              '($shownColumns of $columns).',
            ),
          if (!_showAll && rowCount > rowBudget)
            TextButton(
              onPressed: () => setState(() => _showAll = true),
              child: Text('Show all $rowCount rows'),
            ),
        ],
      ),
    );
  }
}

const _tableRule = BorderSide(color: Color(0xFFDADCE0), width: 0.5);

final _tableBorder = TableBorder.all(
  color: _tableRule.color,
  width: _tableRule.width,
);

/// Row [r]'s slice of the table's rowRuns, or null for a plain row.
List<List<TextRun>?>? _rowRuns(Block block, int r) =>
    r < block.rowRuns.length ? block.rowRuns[r] : null;

/// One table cell. [runs] is the row's slice of headerRuns / rowRuns, or null
/// for a plain row; it is indexed by the row's own cells (a ragged row has
/// fewer than the table's columns), never by the header width.
Widget _tableCell(
  List<String> cells,
  List<List<TextRun>?>? runs,
  int i, {
  bool header = false,
}) {
  final text = i < cells.length ? cells[i] : '';
  final style = header ? const TextStyle(fontWeight: FontWeight.w600) : null;
  final cellRuns = runs != null && i < cells.length && i < runs.length
      ? runs[i]
      : null;
  return Padding(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
    child: cellRuns == null || cellRuns.isEmpty
        ? Text(text, style: style)
        : Text.rich(richSpanForRuns(cellRuns, text, style: style)),
  );
}

TableRow _tableRow(
  List<String> cells,
  List<List<TextRun>?>? runs,
  int columns, {
  bool header = false,
}) => TableRow(
  children: List<Widget>.generate(
    columns,
    (i) => _tableCell(cells, runs, i, header: header),
  ),
);

/// A row of the lazy "Show all" list: the row's plain cell text, in
/// fixed-width boxes so the columns line up without a Table.
/// Every row is _rowExtent tall (the list's itemExtent), so each cell is one
/// line, ending in an ellipsis when it is longer; the full text stays in the
/// document and in the collapsed table. All cells stretch to the row height,
/// so the borders line up.
Widget _fixedRow(List<String> cells, int columns) => Row(
  crossAxisAlignment: CrossAxisAlignment.stretch,
  children: [
    for (var i = 0; i < columns; i++)
      Container(
        width: _BudgetedTableState._cellWidth,
        padding: const EdgeInsets.symmetric(horizontal: 8),
        alignment: Alignment.centerLeft,
        decoration: const BoxDecoration(
          border: Border.fromBorderSide(_tableRule),
        ),
        child: Text(
          i < cells.length ? cells[i] : '',
          maxLines: 1,
          softWrap: false,
          overflow: TextOverflow.ellipsis,
        ),
      ),
  ],
);

Widget _sheetEmbed(Block block) {
  final embed = block.sheetEmbed ?? const <String, dynamic>{};
  final sheetName = '${embed['sheet'] ?? ''}';
  final snapshot = embed['snapshot'];
  final cells = <(int, int, String)>[];
  if (snapshot is Map && snapshot['cells'] is List) {
    for (final raw in snapshot['cells'] as List) {
      if (raw is! Map) continue;
      final r = raw['r'] as int? ?? 0;
      final c = raw['c'] as int? ?? 0;
      final v = raw['f'] != null ? '=${raw['f']}' : '${raw['v'] ?? ''}';
      cells.add((r, c, v));
    }
  }

  return Container(
    width: double.infinity,
    margin: const EdgeInsets.symmetric(vertical: 8),
    decoration: BoxDecoration(
      border: Border.all(color: const Color(0xFFDADCE0)),
      borderRadius: BorderRadius.circular(8),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: const BoxDecoration(
            color: Color(0xFFF1F3F4),
            borderRadius: BorderRadius.vertical(top: Radius.circular(7)),
          ),
          child: Row(
            children: [
              const Icon(Icons.table_chart, size: 18),
              const SizedBox(width: 8),
              Text(
                sheetName.isEmpty ? 'Sheet embed' : sheetName,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ],
          ),
        ),
        if (cells.isEmpty)
          const Padding(
            padding: EdgeInsets.all(12),
            child: Text(
              'Live sheet reference (no offline snapshot)',
              style: TextStyle(color: Color(0xFF5F6368)),
            ),
          )
        else
          _snapshotTable(cells),
      ],
    ),
  );
}

Widget _snapshotTable(List<(int, int, String)> cells) {
  var maxR = 0;
  var maxC = 0;
  final grid = <(int, int), String>{};
  for (final (r, c, v) in cells) {
    if (r > maxR) maxR = r;
    if (c > maxC) maxC = c;
    grid[(r, c)] = v;
  }
  return Table(
    border: TableBorder.all(color: const Color(0xFFE0E0E0), width: 0.5),
    children: [
      for (var r = 0; r <= maxR; r++)
        TableRow(
          children: [
            for (var c = 0; c <= maxC; c++)
              Padding(
                padding: const EdgeInsets.all(6),
                child: Text(grid[(r, c)] ?? ''),
              ),
          ],
        ),
    ],
  );
}

Widget _figure(Block block) {
  final caption = block.text.isNotEmpty
      ? block.text
      : (block.title.isNotEmpty ? block.title : 'Figure');
  Widget visual;
  if (block.svg.isNotEmpty) {
    visual = SvgPicture.string(
      block.svg,
      height: 96,
      fit: BoxFit.contain,
      placeholderBuilder: (_) => _imageCard(caption: caption, subtitle: 'SVG'),
    );
  } else if (block.src.isNotEmpty) {
    visual = _imageCard(caption: caption, subtitle: block.src);
  } else {
    visual = _imageCard(caption: caption, subtitle: 'No asset');
  }
  return Padding(
    padding: const EdgeInsets.symmetric(vertical: 8),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        visual,
        if (caption.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              caption,
              style: const TextStyle(
                fontSize: 13,
                color: Color(0xFF5F6368),
                fontStyle: FontStyle.italic,
              ),
            ),
          ),
      ],
    ),
  );
}

Widget _imageCard({required String caption, required String subtitle}) {
  return Container(
    width: double.infinity,
    padding: const EdgeInsets.all(16),
    decoration: BoxDecoration(
      color: const Color(0xFFF8F9FA),
      borderRadius: BorderRadius.circular(8),
      border: Border.all(color: const Color(0xFFE0E0E0)),
    ),
    child: Row(
      children: [
        const Icon(Icons.image_outlined, size: 36, color: Color(0xFF5F6368)),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                caption.isEmpty ? 'Image' : caption,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              if (subtitle.isNotEmpty)
                Text(
                  subtitle,
                  style: const TextStyle(
                    fontSize: 12,
                    color: Color(0xFF5F6368),
                  ),
                ),
            ],
          ),
        ),
      ],
    ),
  );
}

TextStyle _headingStyle(int level) {
  final size = switch (level) {
    1 => 28.0,
    2 => 22.0,
    3 => 18.0,
    _ => 16.0,
  };
  return TextStyle(
    fontSize: size,
    fontWeight: FontWeight.w700,
    height: 1.25,
    color: const Color(0xFF202124),
  );
}

({Color background, Color border, Color foreground, IconData icon}) _tone(
  String tone,
) {
  switch (tone) {
    case 'warn':
    case 'warning':
      return (
        background: const Color(0xFFFFF8E1),
        border: const Color(0xFFFFECB3),
        foreground: const Color(0xFF8A6D00),
        icon: Icons.warning_amber_rounded,
      );
    case 'danger':
    case 'error':
      return (
        background: const Color(0xFFFCE8E6),
        border: const Color(0xFFFAD2CF),
        foreground: const Color(0xFFC5221F),
        icon: Icons.error_outline,
      );
    case 'success':
      return (
        background: const Color(0xFFE6F4EA),
        border: const Color(0xFFCEECD5),
        foreground: const Color(0xFF137333),
        icon: Icons.check_circle_outline,
      );
    default:
      return (
        background: const Color(0xFFE8F0FE),
        border: const Color(0xFFD2E3FC),
        foreground: const Color(0xFF1967D2),
        icon: Icons.info_outline,
      );
  }
}
