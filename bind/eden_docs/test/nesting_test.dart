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

// eden-press#29: a stored document nested past maxNestingDepth (one written
// before the Go side enforced the limit) must not hang EdenDocsView. Each
// nested quote added a Container, Column and Padding: depth 2000 took 91 s
// per pump and 4900 never finished.
//
// Test list:
//   D1 a 4900-deep quote chain pumps in under a second, with no exception,
//      and says how many blocks below the limit it does not show
//   D2 no block is painted inside more than maxNestingDepth quote frames,
//      and every level's text is still painted (flattened, not dropped)
//   D3 a chain exactly maxNestingDepth deep paints every level nested
//   D4 a list item's indent stops growing at the limit
//   D5 a flattened run paints maxFlattenedBlocks blocks, then the count of
//      the rest

import 'package:eden_docs/eden_docs.dart';
import 'package:eden_press_model/eden_press_model.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// A document whose blocks nest [depth] deep: depth-1 quotes "l1".."l{n}",
/// each the only child of the one above, and a paragraph "leaf" at the
/// bottom. Built bottom-up as JSON, the form a stored document arrives in.
Map<String, dynamic> quoteChainDoc(int depth) {
  Map<String, dynamic> block = {'kind': 'paragraph', 'text': 'leaf'};
  for (var n = depth - 1; n >= 1; n--) {
    block = {
      'kind': 'quote',
      'text': 'l$n',
      'children': [block],
    };
  }
  return {
    'schemaVersion': 'eden-press.model/v7',
    'meta': <String, dynamic>{},
    'sections': [
      {
        'id': 1,
        'blocks': [block],
      },
    ],
  };
}

/// Matches the frame EdenDocsView paints around a quote (its left rule).
final Finder quoteFrames = find.byWidgetPredicate((w) {
  if (w is! Container) return false;
  final d = w.decoration;
  return d is BoxDecoration &&
      d.border is Border &&
      (d.border! as Border).left.color == const Color(0xFFBDBDBD);
});

Future<void> pumpDoc(WidgetTester tester, Map<String, dynamic> json) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: EdenDocsView(document: ModelDocument.fromJson(json)),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  testWidgets('D1 a 4900-deep quote chain pumps in under a second', (
    tester,
  ) async {
    final json = quoteChainDoc(4900);
    final watch = Stopwatch()..start();
    await pumpDoc(tester, json);
    watch.stop();
    expect(tester.takeException(), isNull);
    expect(watch.elapsedMilliseconds, lessThan(1000));
    // 4900 levels: 30 nested quotes, then a flat run under the 31st.
    final hidden = 4900 - (maxNestingDepth - 1) - maxFlattenedBlocks;
    expect(find.textContaining('$hidden more blocks'), findsOneWidget);
  });

  testWidgets('D2 nothing is painted deeper than maxNestingDepth', (
    tester,
  ) async {
    await pumpDoc(tester, quoteChainDoc(maxNestingDepth + 20));
    final leaf = find.text('leaf', findRichText: true);
    expect(leaf, findsOneWidget);
    final frames = find.ancestor(of: leaf, matching: quoteFrames);
    expect(frames.evaluate().length, lessThanOrEqualTo(maxNestingDepth));
    // Every level's text is still there, flattened into the deepest frame.
    for (var n = 1; n < maxNestingDepth + 20; n++) {
      expect(find.text('l$n', findRichText: true), findsOneWidget);
    }
  });

  testWidgets('D3 a chain exactly maxNestingDepth deep stays nested', (
    tester,
  ) async {
    await pumpDoc(tester, quoteChainDoc(maxNestingDepth));
    final leaf = find.text('leaf', findRichText: true);
    // depth-1 quotes enclose the leaf paragraph at depth maxNestingDepth.
    expect(
      find.ancestor(of: leaf, matching: quoteFrames).evaluate().length,
      maxNestingDepth - 1,
    );
  });

  testWidgets('D4 a list item indent stops growing at the limit', (
    tester,
  ) async {
    Map<String, dynamic> list(int level) => {
      'schemaVersion': 'eden-press.model/v7',
      'meta': <String, dynamic>{},
      'sections': [
        {
          'id': 1,
          'blocks': [
            {
              'kind': 'list',
              'items': [
                {'text': 'deep', 'level': level},
              ],
            },
          ],
        },
      ],
    };
    double indent(WidgetTester t) {
      final pad = t.widget<Padding>(
        find
            .ancestor(of: find.text('deep'), matching: find.byType(Padding))
            .first,
      );
      return (pad.padding as EdgeInsets).left;
    }

    await pumpDoc(tester, list(maxNestingDepth - 1));
    final atLimit = indent(tester);
    await pumpDoc(tester, list(4900));
    expect(tester.takeException(), isNull);
    expect(indent(tester), atLimit);
  });

  testWidgets('D5 a flattened run stops at maxFlattenedBlocks', (tester) async {
    final depth = maxNestingDepth + maxFlattenedBlocks + 5;
    await pumpDoc(tester, quoteChainDoc(depth));
    // Under the quote at maxNestingDepth - 1 the run holds every deeper
    // quote and the leaf: depth - (maxNestingDepth - 1) blocks.
    final run = depth - (maxNestingDepth - 1);
    final first = maxNestingDepth;
    final lastShown = first + maxFlattenedBlocks - 1;
    expect(find.text('l$lastShown', findRichText: true), findsOneWidget);
    expect(find.text('l${lastShown + 1}', findRichText: true), findsNothing);
    expect(
      find.textContaining('${run - maxFlattenedBlocks} more blocks'),
      findsOneWidget,
    );
  });
}
