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

import 'package:eden_press/eden_press.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// Schema v3 (EPD-R1) added table/image/quote block kinds to chase/model.
//
// Before it, a GFM table's content was absent from the docmodel entirely. Now
// it is present — but a table's payload lives in `headers`/`rows`, NOT in
// `text`, so a consumer that falls back to Text(block.text) for an unfamiliar
// kind renders a table as an EMPTY WIDGET. That is the same silent drop the
// Go-side exporters had, reproduced one layer down, and it is why parsing the
// new kinds is not optional for this binding.

Output _doc(List<Map<String, dynamic>> blocks) => Output.fromJson({
  'HTML': '',
  'CSS': '',
  'Model': {
    'schemaVersion': 'eden-press.model/v3',
    'sections': [
      {'id': 1, 'blocks': blocks},
    ],
    'outline': <dynamic>[],
  },
  'Meta': <String, dynamic>{},
  'Comments': <dynamic>[],
});

void main() {
  test('Case 1: a table block parses its headers, rows and aligns', () {
    final out = _doc([
      {
        'kind': 'table',
        'headers': ['Metric', 'Q3'],
        'rows': [
          ['p95 latency', '550ms'],
        ],
        'aligns': ['left', 'right'],
      },
    ]);
    final block = out.model.sections.single.blocks.single;
    expect(block.kind, BlockKind.table);
    expect(block.headers, ['Metric', 'Q3']);
    expect(block.rows, [
      ['p95 latency', '550ms'],
    ]);
    expect(block.aligns, ['left', 'right']);
  });

  test('Case 2: an image block parses src, alt text and title', () {
    final out = _doc([
      {
        'kind': 'image',
        'src': 'chart.png',
        'text': 'Q3 chart',
        'title': 'Quarterly',
      },
    ]);
    final block = out.model.sections.single.blocks.single;
    expect(block.kind, BlockKind.image);
    expect(block.src, 'chart.png');
    expect(block.text, 'Q3 chart');
    expect(block.title, 'Quarterly');
  });

  test('Case 3: a quote block is distinguishable from a paragraph', () {
    final out = _doc([
      {'kind': 'quote', 'text': 'Synthesized from the sync.'},
      {'kind': 'paragraph', 'text': 'Ordinary prose.'},
    ]);
    expect(out.model.sections.single.blocks.map((b) => b.kind), [
      BlockKind.quote,
      BlockKind.paragraph,
    ]);
  });

  testWidgets('Case 4: a table renders its cells, not an empty widget', (
    tester,
  ) async {
    final out = _doc([
      {
        'kind': 'table',
        'headers': ['Metric', 'Q3'],
        'rows': [
          ['p95 latency', '550ms'],
        ],
      },
    ]);
    await tester.pumpWidget(
      MaterialApp(home: Scaffold(body: EdenPressView(out))),
    );
    for (final cell in ['Metric', 'Q3', 'p95 latency', '550ms']) {
      expect(
        find.text(cell),
        findsOneWidget,
        reason: 'table cell "$cell" did not reach the widget tree',
      );
    }
  });

  testWidgets('Case 5: an image renders its alt text rather than nothing', (
    tester,
  ) async {
    // The binding does not fetch remote images (no I/O in the render path),
    // so the alt text is what must survive — silently rendering nothing would
    // lose the content entirely.
    final out = _doc([
      {'kind': 'image', 'src': 'chart.png', 'text': 'Q3 chart'},
    ]);
    await tester.pumpWidget(
      MaterialApp(home: Scaffold(body: EdenPressView(out))),
    );
    expect(find.textContaining('Q3 chart'), findsOneWidget);
  });

  test('Case 6: an unrecognized future kind still degrades to its text', () {
    final out = _doc([
      {'kind': 'some-future-kind', 'text': 'still visible'},
    ]);
    final block = out.model.sections.single.blocks.single;
    expect(block.kind, BlockKind.unknown);
    expect(block.text, 'still visible');
  });

  test('Case 7: the binding records the wire shape it now understands', () {
    expect(ModelDocument.expectedSchemaVersion, 'eden-press.model/v7');
  });

  test('Case 8: toggle and synced blocks parse children and syncRef', () {
    final out = _doc([
      {
        'id': 'b_tog',
        'kind': 'toggle',
        'text': 'Details',
        'collapsed': true,
        'children': [
          {'kind': 'paragraph', 'text': 'Nested'},
        ],
      },
      {
        'id': 'b_sync',
        'kind': 'synced',
        'syncRef': {'blockId': 'b_tog', 'pageId': ''},
      },
    ]);
    final blocks = out.model.sections.single.blocks;
    expect(blocks[0].kind, BlockKind.toggle);
    expect(blocks[0].collapsed, isTrue);
    expect(blocks[0].children.single.text, 'Nested');
    expect(blocks[1].kind, BlockKind.synced);
    expect(blocks[1].syncRef?['blockId'], 'b_tog');
  });

  test('Case 9: v7 kinds and rich runs parse', () {
    final out = _doc([
      {
        'kind': 'todo',
        'text': 'Ship',
        'checked': true,
        'runs': [
          {
            'text': 'Ship',
            'marks': ['bold'],
          },
        ],
      },
      {
        'kind': 'sheet',
        'sheetEmbed': {
          'sheet': 'Tasks',
          'snapshot': {
            'cells': [
              {'r': 0, 'c': 0, 'v': 'A'},
            ],
          },
        },
      },
      {'kind': 'figure', 'svg': '<svg/>', 'text': 'logo'},
      {'kind': 'divider'},
    ]);
    final blocks = out.model.sections.single.blocks;
    expect(blocks[0].kind, BlockKind.todo);
    expect(blocks[0].checked, isTrue);
    expect(blocks[0].runs.single.marks, ['bold']);
    expect(blocks[1].kind, BlockKind.sheet);
    expect(blocks[1].sheetEmbed?['sheet'], 'Tasks');
    expect(blocks[2].kind, BlockKind.figure);
    expect(blocks[3].kind, BlockKind.divider);
  });
}

// ModelDocument.expectedSchemaVersion is the binding's own record of which
// wire shape it understands. Leaving it stale after the Go side moves would
// make any version check this constant feeds silently reject — or silently
// accept — the wrong shape. The same reasoning moves it v4 -> v5 for the AI
// doc-ops spike's additive block id: v5 is a strict superset of v4, so every
// case in this file still describes the shape correctly — the constant is
// simply the one thing that must track chase/model.SchemaVersion.
