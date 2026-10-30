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

import 'package:eden_ui/eden_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// Spreadsheet performance plan for the editor.
//
// Formula cost follows stored cells inside the range, not the span.
// A second display of the same formula is a cache hit.
// The grid builds the viewport at every height: gesture detectors stay
// flat while the row count grows. Go file budgets live in
// convert/xlsx/perf_test.go.

void main() {
  test('formula cost follows stored cells', () {
    final sparse = _timedSum(filled: 2, span: 100000);
    expect(sparse.value, 2);
    expect(sparse.milliseconds, lessThan(25));
    print(
      'PERF\tformula\tkind=sparse-span\tfilled=2\tspan=100000\tms=${sparse.milliseconds}',
    );

    final packed = _timedSum(filled: 20000, span: 20000);
    expect(packed.value, 20000);
    expect(packed.milliseconds, lessThan(250));
    print(
      'PERF\tformula\tkind=packed-column\tfilled=20000\tspan=20000\tms=${packed.milliseconds}',
    );
  });

  test('a displayed formula is cached until an input changes', () {
    final book = EdenWorkbook();
    final sheet = book.addSheet('FY');
    for (var i = 0; i < 5000; i++) {
      sheet.setCell(i, 0, '1');
    }
    sheet.setCell(5000, 0, '=SUM(A1:A5000)');

    final first = Stopwatch()..start();
    expect(displayCell(book, 'FY', 5000, 0), '5000');
    first.stop();
    final second = Stopwatch()..start();
    expect(displayCell(book, 'FY', 5000, 0), '5000');
    second.stop();
    print(
      'PERF\tformula-cache\tfirst_ms=${first.elapsedMilliseconds}\tsecond_us=${second.elapsedMicroseconds}',
    );
    expect(first.elapsedMilliseconds, lessThan(200));
    expect(second.elapsedMilliseconds, lessThan(5));

    sheet.setCell(10, 0, '8');
    final after = Stopwatch()..start();
    expect(displayCell(book, 'FY', 5000, 0), '5007');
    after.stop();
    print('PERF\tformula-invalidate\tms=${after.elapsedMilliseconds}');
    expect(after.elapsedMilliseconds, lessThan(200));
  });

  test('five hundred dependents refresh after one edit', () {
    final book = EdenWorkbook();
    final sheet = book.addSheet('FY');
    sheet.setCell(0, 0, '2');
    for (var i = 0; i < 500; i++) {
      sheet.setCell(i, 1, '=A1');
    }
    for (var i = 0; i < 500; i++) {
      expect(displayCell(book, 'FY', i, 1), '2');
    }
    final watch = Stopwatch()..start();
    sheet.setCell(0, 0, '5');
    for (var i = 0; i < 500; i++) {
      expect(displayCell(book, 'FY', i, 1), '5');
    }
    watch.stop();
    print(
      'PERF\tformula-fanout\tdependents=500\tms=${watch.elapsedMilliseconds}',
    );
    expect(watch.elapsedMilliseconds, lessThan(500));
  });

  for (final rows in [8, 500, 2000, 5000]) {
    testWidgets('grid viewport at $rows rows', (tester) async {
      tester.view.physicalSize = const Size(1280, 720);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final book = EdenWorkbook();
      final sheet = book.addSheet('Grid');
      sheet.setCell(0, 0, 'top');
      if (rows > 8) {
        sheet.setCell(rows - 1, 0, 'far');
      }
      final watch = Stopwatch()..start();
      await tester.pumpWidget(_app(EdenView(workbook: book)));
      watch.stop();
      final gestures = find.byType(GestureDetector).evaluate().length;
      print(
        'PERF\tgrid\trows=$rows\tgestures=$gestures\tpump_ms=${watch.elapsedMilliseconds}',
      );
      expect(gestures, lessThan(400));
      expect(watch.elapsedMilliseconds, lessThan(1500));
      expect(find.text('top'), findsWidgets);
      if (rows > 8) {
        expect(find.text('far'), findsNothing);
      }
    });
  }

  testWidgets('scrolling a 5000-row sheet reveals the far cell', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1280, 720);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final book = EdenWorkbook();
    final sheet = book.addSheet('Grid');
    sheet.setCell(0, 0, 'top');
    sheet.setCell(4999, 0, 'far');
    await tester.pumpWidget(_app(EdenView(workbook: book)));
    final vertical = find.byWidgetPredicate(
      (widget) =>
          widget is Scrollable && widget.axisDirection == AxisDirection.down,
    );
    final watch = Stopwatch()..start();
    tester.state<ScrollableState>(vertical).position.jumpTo(4999 * 28);
    await tester.pump();
    watch.stop();
    print('PERF\tgrid-scroll\trows=5000\tms=${watch.elapsedMilliseconds}');
    expect(find.text('far'), findsOneWidget);
    expect(watch.elapsedMilliseconds, lessThan(500));
    expect(find.byType(GestureDetector).evaluate().length, lessThan(400));
  });
}

({int value, int milliseconds}) _timedSum({
  required int filled,
  required int span,
}) {
  final book = EdenWorkbook();
  final sheet = book.addSheet('FY');
  sheet.setCell(0, 0, '1');
  if (filled == 2) {
    sheet.setCell(span - 1, 0, '1');
  } else {
    for (var i = 1; i < filled; i++) {
      sheet.setCell(i, 0, '1');
    }
  }
  final watch = Stopwatch()..start();
  final value = evaluateFormula(book, 'FY', 'SUM(A1:A$span)');
  watch.stop();
  return (value: value.round(), milliseconds: watch.elapsedMilliseconds);
}

Widget _app(Widget child) {
  return MaterialApp(home: Scaffold(body: child));
}
