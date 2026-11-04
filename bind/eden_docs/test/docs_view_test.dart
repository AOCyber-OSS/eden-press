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

import 'package:eden_docs/eden_docs.dart';
import 'package:eden_press_model/eden_press_model.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

Block _b(
  BlockKind kind, {
  String text = '',
  bool checked = false,
  String tone = '',
  String icon = '',
  String svg = '',
  Map<String, dynamic>? sheetEmbed,
  List<Block> children = const [],
  List<TextRun> runs = const [],
}) {
  return Block(
    kind: kind,
    text: text,
    level: kind == BlockKind.heading ? 1 : 0,
    language: kind == BlockKind.code ? 'go' : '',
    display: kind == BlockKind.math,
    ordered: false,
    items: const [],
    checked: checked,
    tone: tone,
    icon: icon,
    svg: svg,
    sheetEmbed: sheetEmbed,
    children: children,
    runs: runs,
  );
}

Block _table({
  List<String> headers = const [],
  List<List<String>> rows = const [],
  List<List<TextRun>?> headerRuns = const [],
  List<List<List<TextRun>?>?> rowRuns = const [],
}) {
  return Block(
    kind: BlockKind.table,
    text: '',
    level: 0,
    language: '',
    display: false,
    ordered: false,
    items: const [],
    headers: headers,
    rows: rows,
    headerRuns: headerRuns,
    rowRuns: rowRuns,
  );
}

/// The table the Go golden (chase/model/dart_golden_test.go) writes as
/// `b_table`: a bold header cell, code containing `|`, a plain row, a mixed
/// plain/link run and a math run, and a ragged one-cell row.
Block _formattedTable() => _table(
  headers: const ['Metric', 'Q3'],
  rows: const [
    ['a|b', '550ms'],
    ['p95 latency', '550ms'],
    ['see docs', 'x^2'],
    ['ragged'],
  ],
  headerRuns: const [
    [
      TextRun(text: 'Metric', marks: ['bold']),
    ],
    null,
  ],
  rowRuns: const [
    [
      [
        TextRun(text: 'a|b', marks: ['code']),
      ],
      null,
    ],
    null,
    [
      [
        TextRun(text: 'see '),
        TextRun(text: 'docs', link: 'https://aocyber.ai/docs'),
      ],
      [
        TextRun(text: 'x^2', marks: ['math']),
      ],
    ],
    [
      [
        TextRun(text: 'ragged', marks: ['italic']),
      ],
    ],
  ],
);

Future<void> _pump(WidgetTester tester, List<Block> blocks) {
  final doc = ModelDocument(
    schemaVersion: ModelDocument.expectedSchemaVersion,
    meta: const Meta(directives: {}),
    sections: [
      Section(id: 1, attrs: const {}, notes: const [], blocks: blocks),
    ],
    outline: const [],
  );
  return tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: EdenDocsView(document: doc)),
    ),
  );
}

/// The RichText whose plain text is exactly [text] (a styled table cell).
Finder _richCell(String text) => find.byWidgetPredicate(
  (w) => w is RichText && w.text.toPlainText() == text,
);

/// Every TextSpan with text under [finder]'s single RichText, in order.
List<TextSpan> _leaves(WidgetTester tester, Finder finder) {
  final leaves = <TextSpan>[];
  tester.widget<RichText>(finder).text.visitChildren((span) {
    if (span is TextSpan && span.text != null) leaves.add(span);
    return true;
  });
  return leaves;
}

TextSpan _leaf(WidgetTester tester, String cell, String text) =>
    _leaves(tester, _richCell(cell)).firstWhere((s) => s.text == text);

void main() {
  group('table cells with runs (1026-29)', () {
    testWidgets('a bold header cell renders w700', (tester) async {
      await _pump(tester, [_formattedTable()]);
      expect(
        _leaf(tester, 'Metric', 'Metric').style?.fontWeight,
        FontWeight.w700,
      );
    });

    testWidgets('a code cell renders monospace and keeps the pipe', (
      tester,
    ) async {
      await _pump(tester, [_formattedTable()]);
      final code = _leaf(tester, 'a|b', 'a|b');
      expect(code.style?.fontFamily, 'monospace');
      expect(code.style?.backgroundColor, isNotNull);
    });

    testWidgets('a link run is underlined and its plain neighbour is not', (
      tester,
    ) async {
      await _pump(tester, [_formattedTable()]);
      expect(
        _leaf(tester, 'see docs', 'docs').style?.decoration,
        TextDecoration.underline,
      );
      expect(
        _leaf(tester, 'see docs', 'see ').style?.decoration,
        isNot(TextDecoration.underline),
      );
    });

    testWidgets('a math run renders as its TeX text, like a paragraph', (
      tester,
    ) async {
      await _pump(tester, [_formattedTable()]);
      final math = _leaf(tester, 'x^2', 'x^2');
      expect(math.style?.fontFamily, isNull);
      expect(math.style?.fontWeight, isNot(FontWeight.w700));
    });

    testWidgets('the ragged row indexes by its own cells and renders', (
      tester,
    ) async {
      await _pump(tester, [_formattedTable()]);
      expect(tester.takeException(), isNull);
      expect(
        _leaf(tester, 'ragged', 'ragged').style?.fontStyle,
        FontStyle.italic,
      );
    });

    testWidgets('plain cells stay plain Text, header ones keep w600', (
      tester,
    ) async {
      await _pump(tester, [_formattedTable()]);
      final plainBody = tester.widget<Text>(find.text('p95 latency'));
      expect(plainBody.data, 'p95 latency');
      expect(plainBody.textSpan, isNull);
      final plainHeader = tester.widget<Text>(find.text('Q3'));
      expect(plainHeader.data, 'Q3');
      expect(plainHeader.style?.fontWeight, FontWeight.w600);
    });

    testWidgets('a table with no cell runs renders exactly as before', (
      tester,
    ) async {
      await _pump(tester, [
        _table(
          headers: const ['Name', 'Value'],
          rows: const [
            ['p95', '550ms'],
            ['ragged'],
          ],
        ),
      ]);
      for (final cell in ['Name', 'Value', 'p95', '550ms', 'ragged']) {
        final text = tester.widget<Text>(find.text(cell));
        expect(text.data, cell);
        expect(text.textSpan, isNull);
      }
      expect(
        tester.widget<Text>(find.text('Name')).style?.fontWeight,
        FontWeight.w600,
      );
      expect(tester.widget<Text>(find.text('p95')).style, isNull);
    });
  });

  testWidgets('renders v7 blocks as widgets, not JSON dumps', (tester) async {
    final doc = ModelDocument(
      schemaVersion: ModelDocument.expectedSchemaVersion,
      meta: const Meta(directives: {'profile': 'article'}),
      sections: [
        Section(
          id: 1,
          attrs: const {},
          notes: const [],
          blocks: [
            _b(BlockKind.heading, text: 'Roadmap'),
            _b(
              BlockKind.paragraph,
              text: 'Hello',
              runs: const [
                TextRun(text: 'Hello', marks: ['bold']),
              ],
            ),
            _b(BlockKind.todo, text: 'Ship it', checked: true),
            _b(
              BlockKind.callout,
              text: 'Watch latency',
              tone: 'warn',
              icon: '!',
            ),
            _b(BlockKind.divider),
            _b(
              BlockKind.sheet,
              sheetEmbed: {
                'sheet': 'Tasks',
                'snapshot': {
                  'cells': [
                    {'r': 0, 'c': 0, 'v': 'A'},
                    {'r': 0, 'c': 1, 'v': 'B'},
                  ],
                },
              },
            ),
            _b(
              BlockKind.figure,
              text: 'logo',
              svg: '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>',
            ),
            _b(
              BlockKind.toggle,
              text: 'Details',
              children: [_b(BlockKind.paragraph, text: 'Nested')],
            ),
            _b(BlockKind.code, text: 'fmt.Println(1)'),
            _b(BlockKind.math, text: 'E=mc^2'),
          ],
        ),
      ],
      outline: const [],
    );

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: EdenDocsView(document: doc)),
      ),
    );

    expect(find.byKey(const Key('eden-docs')), findsOneWidget);
    expect(find.text('Roadmap'), findsOneWidget);
    expect(find.text('Ship it'), findsOneWidget);
    expect(find.text('Watch latency'), findsOneWidget);
    expect(find.text('Tasks'), findsOneWidget);
    expect(find.text('A'), findsOneWidget);
    expect(find.text('B'), findsOneWidget);
    expect(find.text('logo'), findsOneWidget);
    expect(find.text('Details'), findsOneWidget);
    expect(find.text('Nested'), findsOneWidget);
    // Placeholders from the old EdenPressView path must not appear.
    expect(find.text('[sheet:Tasks]'), findsNothing);
    expect(find.text('[logo]'), findsNothing);
    expect(find.textContaining('"kind"'), findsNothing);
  });
}
