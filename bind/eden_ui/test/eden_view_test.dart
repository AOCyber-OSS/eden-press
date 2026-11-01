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

import 'dart:convert';

import 'package:eden_press/eden_press.dart';
import 'package:eden_ui/eden_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

ModelDocument _doc({
  required List<Section> sections,
  Map<String, String> directives = const {},
}) {
  return ModelDocument(
    schemaVersion: 'eden-press.model/v7',
    meta: Meta(directives: directives),
    sections: sections,
    outline: const [],
  );
}

Section _section(int id, List<Block> blocks) {
  return Section(id: id, attrs: const {}, notes: const [], blocks: blocks);
}

Block _heading(String text) {
  return Block(
    kind: BlockKind.heading,
    text: text,
    level: 1,
    language: '',
    display: false,
    ordered: false,
    items: const [],
  );
}

Block _paragraph(String text) {
  return Block(
    kind: BlockKind.paragraph,
    text: text,
    level: 0,
    language: '',
    display: false,
    ordered: false,
    items: const [],
  );
}

Block _table() {
  return Block(
    kind: BlockKind.table,
    text: '',
    level: 0,
    language: '',
    display: false,
    ordered: false,
    items: const [],
    headers: const ['Metric', 'Q3'],
    rows: const [
      ['p95', '550'],
    ],
  );
}

Widget _app(Widget child) {
  return MaterialApp(home: Scaffold(body: child));
}

void main() {
  test('workbook JSON matches the Go editing snapshot', () {
    final book = EdenWorkbook(title: 'Budget');
    final sheet = book.addSheet('FY');
    sheet.setCell(0, 0, '100');
    sheet.setCell(0, 1, '=A1*2');
    sheet.ensureRowId(0, 'r_a');
    sheet.ensureRowId(1, 'r_b');
    sheet.setColumn(const EdenColumn(id: 'rel', name: 'Related', type: 'relation'));
    sheet.setRelations(0, 0, [const EdenRelationRef(rowId: 'r_b')]);
    final json = book.toJson();
    expect(json['schema'], workbookSchema);
    final parsed = EdenWorkbook.fromJson(json);
    expect(parsed.title, 'Budget');
    expect(parsed.cell('FY', 0, 0), '100');
    expect(parsed.cell('FY', 0, 1), '=A1*2');
    expect(parsed.sheet('FY')!.rowId(0), 'r_a');
    expect(parsed.sheet('FY')!.columns.single.id, 'rel');
    expect(parsed.sheet('FY')!.relations(0, 0).single.rowId, 'r_b');
    expect(
      () => EdenWorkbook.fromJson({'schema': 'nope', 'sheets': []}),
      throwsFormatException,
    );

    final v2 = EdenWorkbook.fromJson({
      'schema': workbookSchemaV2,
      'sheets': [
        {
          'name': 'FY',
          'cells': [
            {'r': 0, 'c': 0, 'v': 'x'},
          ],
        },
      ],
    });
    expect(v2.cell('FY', 0, 0), 'x');

    final legacy = EdenWorkbook.fromJson({
      'schema': workbookSchemaV1,
      'title': 'Old',
      'sheets': [
        {
          'name': 'FY',
          'rows': [
            {
              'header': true,
              'cells': [
                {'v': 'A'},
                {'v': 'B'},
              ],
            },
            {
              'cells': [
                {},
                {'f': 'A1'},
              ],
            },
          ],
        },
      ],
    });
    expect(legacy.cell('FY', 0, 0), 'A');
    expect(legacy.cell('FY', 1, 1), '=A1');
    expect(legacy.sheet('FY')!.headerRows, contains(0));

    final wide = EdenWorkbook();
    final grid = wide.addSheet('Grid');
    grid.setCell(1048575, 0, '9');
    grid.setCell(0, 16383, '8');
    final encoded = jsonEncode(wide.toJson());
    expect(encoded.length, lessThan(64 * 1024));
    expect(encoded.contains('"rows"'), isFalse);
    final again = EdenWorkbook.fromJson(
      jsonDecode(encoded) as Map<String, dynamic>,
    );
    expect(again.cell('Grid', 1048575, 0), '9');
    expect(again.cell('Grid', 0, 16383), '8');
  });

  test('setCell rejects addresses outside the Excel grid', () {
    final sheet = EdenWorkbook().addSheet('Grid');
    expect(() => sheet.setCell(sheetRowLimit, 0, 'x'), throwsArgumentError);
    expect(() => sheet.setCell(0, sheetColumnLimit, 'x'), throwsArgumentError);
    expect(() => sheet.setCell(-1, 0, 'x'), throwsArgumentError);
    expect(sheet.cell(0, 0), isEmpty);
  });

  test('a table-only document is a spreadsheet', () {
    final doc = _doc(
      sections: [
        _section(1, [_heading('Metrics'), _table()]),
      ],
    );
    expect(inferEdenDocumentKind(doc), EdenDocumentKind.spreadsheet);
    final book = EdenWorkbook.fromDocument(doc);
    expect(book.sheetNames, ['Metrics']);
    expect(book.cell('Metrics', 1, 1), '550');
  });

  test('marp is slides and a single paragraph is a document', () {
    final slides = _doc(
      directives: const {'marp': 'true'},
      sections: [
        _section(1, [_heading('Alpha')]),
        _section(2, [_heading('Beta')]),
      ],
    );
    expect(inferEdenDocumentKind(slides), EdenDocumentKind.slides);
    final prose = _doc(
      sections: [
        _section(1, [_paragraph('Hello')]),
      ],
    );
    expect(inferEdenDocumentKind(prose), EdenDocumentKind.document);
  });

  testWidgets('spreadsheet shows a formula result and commits an edit', (
    tester,
  ) async {
    final book = EdenWorkbook();
    final sheet = book.addSheet('FY');
    sheet.setCell(0, 0, '10');
    sheet.setCell(1, 0, '20');
    sheet.setCell(2, 0, '=SUM(A1:A2)');
    final edits = <EdenEdit>[];
    await tester.pumpWidget(_app(EdenView(workbook: book, onEdit: edits.add)));
    expect(find.text('30'), findsOneWidget);

    await tester.tap(find.text('30'));
    await tester.pump();
    final bar = tester.widget<TextField>(
      find.byKey(const Key('eden-formula-bar')),
    );
    expect(bar.controller!.text, '=SUM(A1:A2)');

    await tester.enterText(find.byKey(const Key('eden-formula-bar')), '9');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pump();

    expect(find.text('9'), findsWidgets);
    expect(edits, hasLength(1));
    final edit = edits.single as EdenCellEdit;
    expect(edit.sheet, 'FY');
    expect(edit.row, 2);
    expect(edit.column, 0);
    expect(edit.value, '9');
  });

  testWidgets('a tall sheet builds the viewport and scrolls to a far row', (
    tester,
  ) async {
    final book = EdenWorkbook();
    final sheet = book.addSheet('Grid');
    sheet.setCell(0, 0, 'top');
    sheet.setCell(4999, 0, 'far');
    await tester.pumpWidget(_app(EdenView(workbook: book)));
    expect(find.text('top'), findsWidgets);
    expect(find.text('far'), findsNothing);
    expect(find.byType(GestureDetector).evaluate().length, lessThan(500));

    final vertical = find.byWidgetPredicate(
      (widget) =>
          widget is Scrollable && widget.axisDirection == AxisDirection.down,
    );
    tester.state<ScrollableState>(vertical).position.jumpTo(4999 * 28);
    await tester.pump();
    expect(find.text('far'), findsOneWidget);
    expect(find.byType(GestureDetector).evaluate().length, lessThan(500));
  });

  testWidgets('add sheet reports a new worksheet', (tester) async {
    final book = EdenWorkbook()..addSheet('FY');
    final edits = <EdenEdit>[];
    await tester.pumpWidget(_app(EdenView(workbook: book, onEdit: edits.add)));
    await tester.tap(find.byKey(const Key('eden-add-sheet')));
    await tester.pump();
    expect(find.text('Sheet2'), findsOneWidget);
    expect(edits.single, isA<EdenSheetAdded>());
    expect((edits.single as EdenSheetAdded).name, 'Sheet2');
  });

  testWidgets('slides page one at a time and a document shows both', (
    tester,
  ) async {
    final doc = _doc(
      directives: const {'marp': 'true'},
      sections: [
        _section(1, [_heading('Alpha')]),
        _section(2, [_heading('Beta')]),
      ],
    );
    await tester.pumpWidget(_app(EdenView(document: doc)));
    expect(find.byKey(const Key('eden-slides')), findsOneWidget);
    expect(find.text('Alpha'), findsOneWidget);
    expect(find.text('Beta'), findsNothing);
    expect(find.text('Slide 1 of 2'), findsOneWidget);

    await tester.fling(
      find.byKey(const Key('eden-slides')),
      const Offset(-500, 0),
      1200,
    );
    await tester.pumpAndSettle();
    expect(find.text('Beta'), findsOneWidget);
    expect(find.text('Slide 2 of 2'), findsOneWidget);

    final prose = _doc(
      directives: const {'profile': 'article'},
      sections: [
        _section(1, [_paragraph('Alpha')]),
        _section(2, [_paragraph('Beta')]),
      ],
    );
    await tester.pumpWidget(_app(EdenView(document: prose)));
    expect(find.byKey(const Key('eden-slides')), findsNothing);
    expect(find.text('Alpha'), findsOneWidget);
    expect(find.text('Beta'), findsOneWidget);
  });

  test('a sheet embed keeps the surface as a document, not a spreadsheet', () {
    final doc = _doc(
      directives: const {'profile': 'article'},
      sections: [
        _section(1, [
          _heading('Plan'),
          const Block(
            kind: BlockKind.sheet,
            text: '',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            sheetEmbed: {
              'sheet': 'Tasks',
              'snapshot': {
                'cells': [
                  {'r': 0, 'c': 0, 'v': 'A'},
                ],
              },
            },
          ),
        ]),
      ],
    );
    expect(inferEdenDocumentKind(doc), EdenDocumentKind.document);
  });

  testWidgets('document mode renders v7 todo, sheet, figure, and divider', (
    tester,
  ) async {
    final doc = _doc(
      directives: const {'profile': 'article'},
      sections: [
        _section(1, [
          const Block(
            kind: BlockKind.todo,
            text: 'Ship v7',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            checked: true,
          ),
          const Block(
            kind: BlockKind.sheet,
            text: '',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            sheetEmbed: {'sheet': 'Tasks'},
          ),
          const Block(
            kind: BlockKind.figure,
            text: 'logo',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            svg: '<svg/>',
          ),
          const Block(
            kind: BlockKind.divider,
            text: '',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
          const Block(
            kind: BlockKind.callout,
            text: 'Watch latency',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            tone: 'warn',
            icon: '!',
          ),
        ]),
      ],
    );
    await tester.pumpWidget(_app(EdenView(document: doc)));
    expect(find.byKey(const Key('eden-slides')), findsNothing);
    expect(find.textContaining('Ship v7'), findsOneWidget);
    expect(find.text('[sheet:Tasks]'), findsOneWidget);
    expect(find.text('[logo]'), findsOneWidget);
    expect(find.byType(Divider), findsWidgets);
    expect(find.text('! Watch latency'), findsOneWidget);
  });
}
