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
import 'package:flutter_highlighting/flutter_highlighting.dart';
import 'package:flutter_highlighting/themes/github.dart';
import 'package:flutter_math_fork/flutter_math.dart';
import 'package:flutter_svg/flutter_svg.dart';

import 'rich_text.dart';

/// Paints one schema-v7 [Block] as a document widget (never a JSON dump).
Widget buildDocBlock(Block block, {required Widget Function(Block) nest}) {
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
            for (final child in block.children) nest(child),
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
                  left: 8.0 + 16.0 * block.items[i].level,
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
                children: [for (final child in block.children) nest(child)],
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
                  for (final child in block.children) nest(child),
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
          children: [for (final child in block.children) nest(child)],
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
        child: HighlightView(
          block.text,
          languageId: block.language.isEmpty ? 'plaintext' : block.language,
          theme: githubTheme,
          textStyle: const TextStyle(fontFamily: 'monospace', fontSize: 13),
        ),
      );
    case BlockKind.math:
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Math.tex(
          block.text,
          mathStyle: block.display ? MathStyle.display : MathStyle.text,
        ),
      );
    case BlockKind.table:
      return _table(block);
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

Widget _table(Block block) {
  final columns = block.headers.isNotEmpty
      ? block.headers.length
      : block.rows.fold<int>(0, (m, r) => r.length > m ? r.length : m);
  if (columns == 0) return const SizedBox.shrink();

  // [runs] is the row's slice of headerRuns / rowRuns, or null for a plain row.
  // It is indexed by the row's own cells (a ragged row has fewer than
  // [columns]), never by the header width.
  TableRow row(
    List<String> cells,
    List<List<TextRun>?>? runs, {
    required bool header,
  }) => TableRow(
    children: List<Widget>.generate(columns, (i) {
      final text = i < cells.length ? cells[i] : '';
      final style = header
          ? const TextStyle(fontWeight: FontWeight.w600)
          : null;
      final cellRuns = runs != null && i < cells.length && i < runs.length
          ? runs[i]
          : null;
      return Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
        child: cellRuns == null || cellRuns.isEmpty
            ? Text(text, style: style)
            : Text.rich(richSpanForRuns(cellRuns, text, style: style)),
      );
    }),
  );

  return Container(
    margin: const EdgeInsets.symmetric(vertical: 8),
    child: Table(
      border: TableBorder.all(color: const Color(0xFFDADCE0), width: 0.5),
      children: [
        if (block.headers.isNotEmpty)
          row(block.headers, block.headerRuns, header: true),
        for (var r = 0; r < block.rows.length; r++)
          row(
            block.rows[r],
            r < block.rowRuns.length ? block.rowRuns[r] : null,
            header: false,
          ),
      ],
    ),
  );
}

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
