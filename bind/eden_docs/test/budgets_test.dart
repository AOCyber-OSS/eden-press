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

// eden-press#32: one stored block must not freeze EdenDocsView. Each case is
// generated (never pasted), pumped after a warm-up pump (the first pump of a
// process loads fonts and caches), and bounded at one second.
//
// Test list:
//   B1 a 20000x20 table pumps in under 1 s; its first rows are painted and
//      "Show all 20000 rows" is offered
//   B2 "Show all" pages the rows lazily: the last row is not built until
//      scrolled to
//   B3 a 1x5000 table pumps in under 1 s and says how many columns it leaves
//      out
//   B4 3000 nested \frac shows the TeX source, with no exception
//   B5 invalid TeX shows the TeX source (onErrorFallback), with no exception
//   B6 a 400 KB code block pumps in under 1 s as plain text, not highlighted
//   B7 a small table, short code and simple math render as before (no
//      "Show all", highlighted, typeset)

import 'package:eden_docs/eden_docs.dart';
import 'package:eden_press_model/eden_press_model.dart';
import 'package:flutter/material.dart';
import 'package:flutter_highlighting/flutter_highlighting.dart';
import 'package:flutter_math_fork/flutter_math.dart';
import 'package:flutter_test/flutter_test.dart';

/// A one-section document holding [blocks] (JSON block maps).
ModelDocument docOf(List<Map<String, dynamic>> blocks) =>
    ModelDocument.fromJson({
      'schemaVersion': 'eden-press.model/v7',
      'meta': <String, dynamic>{},
      'sections': [
        {'id': 1, 'blocks': blocks},
      ],
    });

/// A [rows] x [cols] table whose cells are "r{row}c{col}".
Map<String, dynamic> tableBlock(int rows, int cols) => {
  'kind': 'table',
  'headers': [for (var c = 0; c < cols; c++) 'h$c'],
  'rows': [
    for (var r = 0; r < rows; r++) [for (var c = 0; c < cols; c++) 'r${r}c$c'],
  ],
};

/// [n] nested \frac, innermost x: \frac{\frac{...}{1}}{1}.
String nestedFrac(int n) {
  final b = StringBuffer();
  for (var i = 0; i < n; i++) {
    b.write(r'\frac{');
  }
  b.write('x');
  for (var i = 0; i < n; i++) {
    b.write('}{1}');
  }
  return b.toString();
}

Map<String, dynamic> mathBlock(String tex) => {
  'kind': 'math',
  'text': tex,
  'display': true,
};

Map<String, dynamic> codeBlock(String text) => {
  'kind': 'code',
  'text': text,
  'language': 'javascript',
};

/// Pumps [doc] after a warm-up pump and returns the milliseconds the
/// document itself took. Printed so CI logs show the margin.
Future<int> timedPump(WidgetTester tester, ModelDocument doc) async {
  Future<void> pump(ModelDocument d) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: EdenDocsView(document: d)),
      ),
    );
    await tester.pump();
  }

  await pump(
    docOf([
      {'kind': 'paragraph', 'text': 'warm'},
    ]),
  );
  final watch = Stopwatch()..start();
  await pump(doc);
  watch.stop();
  // ignore: avoid_print
  print('pump: ${watch.elapsedMilliseconds} ms');
  return watch.elapsedMilliseconds;
}

void main() {
  testWidgets('B1 a 20000x20 table pumps in under a second', (tester) async {
    final ms = await timedPump(tester, docOf([tableBlock(20000, 20)]));
    expect(tester.takeException(), isNull);
    expect(ms, lessThan(1000));
    expect(find.text('r0c0'), findsOneWidget);
    expect(find.text('Show all 20000 rows'), findsOneWidget);
    expect(find.text('r${maxTableRowsPainted}c0'), findsNothing);
  });

  testWidgets('B2 Show all pages the rows lazily', (tester) async {
    await timedPump(tester, docOf([tableBlock(20000, 20)]));
    final showAll = find.text('Show all 20000 rows');
    await tester.ensureVisible(showAll);
    await tester.pump();
    await tester.tap(showAll);
    await tester.pump();
    expect(tester.takeException(), isNull);
    expect(find.text('Show all 20000 rows'), findsNothing);
    expect(find.text('r19999c0'), findsNothing);
    // Jump the row list to its end: the last row is built only now.
    final rows = tester.state<ScrollableState>(
      find.descendant(
        of: find.byKey(const Key('eden-docs-table-rows')),
        matching: find.byType(Scrollable),
      ),
    );
    rows.position.jumpTo(rows.position.maxScrollExtent);
    await tester.pump();
    expect(find.text('r19999c0'), findsOneWidget);
  });

  testWidgets('B3 a 1x5000 table pumps in under a second', (tester) async {
    final ms = await timedPump(tester, docOf([tableBlock(1, 5000)]));
    expect(tester.takeException(), isNull);
    expect(ms, lessThan(1000));
    expect(find.text('r0c0'), findsOneWidget);
    expect(
      find.textContaining('${5000 - maxTableColumnsPainted} more columns'),
      findsOneWidget,
    );
  });

  testWidgets('B4 3000 nested \\frac shows its TeX source', (tester) async {
    final tex = nestedFrac(3000);
    await timedPump(tester, docOf([mathBlock(tex)]));
    expect(tester.takeException(), isNull);
    expect(find.text(tex), findsOneWidget);
    expect(find.byType(Math), findsNothing);
  });

  testWidgets('B5 invalid TeX shows its source', (tester) async {
    const tex = r'\begin{x}\end{y}}}}{{{\left(';
    await timedPump(tester, docOf([mathBlock(tex)]));
    expect(tester.takeException(), isNull);
    expect(find.text(tex), findsOneWidget);
  });

  testWidgets('B6 a 400 KB code block pumps in under a second', (tester) async {
    final code = '/*' * 200000;
    final ms = await timedPump(tester, docOf([codeBlock(code)]));
    expect(tester.takeException(), isNull);
    expect(ms, lessThan(1000));
    expect(find.byType(HighlightView), findsNothing);
    expect(
      find.textContaining(
        '${code.length - maxPlainCodeLength} more characters',
      ),
      findsOneWidget,
    );
  });

  testWidgets('B7 small blocks render as before', (tester) async {
    await timedPump(
      tester,
      docOf([
        tableBlock(3, 2),
        codeBlock('const x = 1;'),
        mathBlock(r'\frac{a}{b}'),
      ]),
    );
    expect(tester.takeException(), isNull);
    expect(find.text('r2c1'), findsOneWidget);
    expect(find.textContaining('Show all'), findsNothing);
    expect(find.byType(HighlightView), findsOneWidget);
    expect(find.byType(Math), findsOneWidget);
  });
}
