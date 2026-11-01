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
import 'package:eden_press/eden_press.dart';
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

void main() {
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
            _b(BlockKind.figure, text: 'logo', svg: '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>'),
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
      MaterialApp(home: Scaffold(body: EdenDocsView(document: doc))),
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
