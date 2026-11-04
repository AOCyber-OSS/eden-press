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

// 1026-05 Test list, items 2-4, plus the omitempty edges they depend on.
//
// test/fixtures/v7_all_kinds.json is written by Go, never by hand:
//   go test ./chase/model -run TestDartGolden -update-dart-golden
// chase/model/dart_golden_test.go builds the document and pins
// json.Marshal's output to that file, so every assertion below that compares
// against the golden is a comparison against Go.

import 'dart:convert';
import 'dart:io';

import 'package:eden_press_model/eden_press_model.dart';
import 'package:test/test.dart';

Map<String, dynamic> _golden() {
  final file = File('test/fixtures/v7_all_kinds.json');
  expect(
    file.existsSync(),
    isTrue,
    reason: 'run `dart test` from bind/eden_press_model/',
  );
  return jsonDecode(file.readAsStringSync()) as Map<String, dynamic>;
}

/// Every block in [blocks], depth first, children included.
Iterable<Map<String, dynamic>> _blocksIn(Object? blocks) sync* {
  if (blocks is! List) return;
  for (final raw in blocks) {
    final block = raw as Map<String, dynamic>;
    yield block;
    yield* _blocksIn(block['children']);
  }
}

Map<String, dynamic> _goldenBlock(Map<String, dynamic> golden, String id) {
  for (final section in golden['sections'] as List) {
    for (final block in _blocksIn((section as Map)['blocks'])) {
      if (block['id'] == id) return block;
    }
  }
  throw StateError('golden has no block $id');
}

Block _bare(
  BlockKind kind, {
  String text = '',
  Span? span,
  bool checked = false,
}) => Block(
  kind: kind,
  text: text,
  level: 0,
  language: '',
  display: false,
  ordered: false,
  items: const [],
  checked: checked,
  span: span,
);

void main() {
  group('Test list 2: round trip against the Go golden', () {
    test('ModelDocument.fromJson(golden).toJson() deep-equals the golden', () {
      final golden = _golden();
      expect(ModelDocument.fromJson(golden).toJson(), equals(golden));
    });

    test(
      'toJson writes keys in Go struct order, so re-encoding is identical',
      () {
        // jsonDecode keeps the golden's key order, which is Go's struct-field
        // order (and sorted order for Go maps). Equal encodings therefore prove
        // the order matches too, not only the key set.
        final golden = _golden();
        expect(
          jsonEncode(ModelDocument.fromJson(golden).toJson()),
          jsonEncode(golden),
        );
      },
    );

    test('toJson sorts map keys the way Go does', () {
      final doc = ModelDocument(
        schemaVersion: ModelDocument.expectedSchemaVersion,
        meta: const Meta(directives: {'theme': 'x', 'profile': 'y'}),
        sections: const [
          Section(
            id: 1,
            attrs: {'data-z': '1', 'class': 'a'},
            notes: [],
            blocks: [],
          ),
        ],
        outline: const [],
        propertyValues: const {'p_b': '2', 'p_a': '1'},
      );
      final json = doc.toJson();
      expect((json['meta'] as Map)['directives'].keys, ['profile', 'theme']);
      expect(((json['sections'] as List).single as Map)['attrs'].keys, [
        'class',
        'data-z',
      ]);
      expect((json['propertyValues'] as Map).keys, ['p_a', 'p_b']);
    });
  });

  group('Test list 3: block kinds', () {
    test('every BlockKind wire name equals the 16 Go kinds in the golden', () {
      final golden = _golden();
      final goKinds = <String>{
        for (final section in golden['sections'] as List)
          for (final block in _blocksIn((section as Map)['blocks']))
            block['kind'] as String,
      };
      final dartKinds = <String>{
        for (final kind in BlockKind.values)
          if (kind != BlockKind.unknown) kind.wire,
      };
      expect(goKinds, hasLength(16));
      expect(dartKinds, equals(goKinds));
    });

    test('BlockKind.fromWire inverts wire for every known kind', () {
      for (final kind in BlockKind.values) {
        if (kind == BlockKind.unknown) continue;
        expect(BlockKind.fromWire(kind.wire), kind);
      }
      expect(BlockKind.fromWire('some-future-kind'), BlockKind.unknown);
    });

    test('an unknown kind keeps its wire string through toJson', () {
      final block = Block.fromJson({
        'kind': 'some-future-kind',
        'text': 'still visible',
      });
      expect(block.kind, BlockKind.unknown);
      expect(block.toJson(), {
        'kind': 'some-future-kind',
        'text': 'still visible',
      });
    });
  });

  group('Test list 4: span', () {
    test('a Span that starts at 0 survives toJson', () {
      final json = _bare(
        BlockKind.paragraph,
        text: 'Top',
        span: const Span(start: 0, stop: 3),
      ).toJson();
      expect(json['span'], {'start': 0, 'stop': 3});
    });

    test('the golden first block keeps "start": 0 through the round trip', () {
      final golden = _golden();
      final first = ModelDocument.fromJson(golden).sections.first.blocks.first;
      expect(first.span?.start, 0);
      expect(first.toJson()['span'], {'start': 0, 'stop': 9});
    });

    test('a null Span stays absent on block, section and outline entry', () {
      expect(_bare(BlockKind.paragraph).toJson().containsKey('span'), isFalse);
      expect(
        const Section(
          id: 1,
          attrs: {},
          notes: [],
          blocks: [],
        ).toJson().containsKey('span'),
        isFalse,
      );
      expect(
        const OutlineEntry(
          sectionId: 1,
          level: 1,
          text: 'a',
          slug: 'a',
        ).toJson().containsKey('span'),
        isFalse,
      );
    });
  });

  group('omitempty mirrors the Go json tags', () {
    test('a bare block marshals to its kind alone', () {
      expect(_bare(BlockKind.paragraph).toJson(), {'kind': 'paragraph'});
    });

    test('Block.checked is omitted when false and written when true', () {
      expect(_bare(BlockKind.todo).toJson().containsKey('checked'), isFalse);
      expect(_bare(BlockKind.todo, checked: true).toJson()['checked'], isTrue);
    });

    test('a section with only an id marshals to its id alone', () {
      expect(const Section(id: 2, attrs: {}, notes: [], blocks: []).toJson(), {
        'id': 2,
      });
    });

    test('ListItem.text is written even when empty; level 0 is omitted', () {
      expect(const ListItem(text: '', level: 0).toJson(), {'text': ''});
    });

    test('OutlineEntry fields are never omitted', () {
      expect(
        const OutlineEntry(sectionId: 0, level: 0, text: '', slug: '').toJson(),
        {'sectionId': 0, 'level': 0, 'text': '', 'slug': ''},
      );
    });

    test('syncRef drops an empty pageId, as Go does', () {
      final block = Block.fromJson({
        'kind': 'synced',
        'syncRef': {'blockId': 'b_tog', 'pageId': ''},
      });
      expect(block.syncRef?['blockId'], 'b_tog');
      expect(block.typedSyncRef?.blockId, 'b_tog');
      expect(block.toJson()['syncRef'], {'blockId': 'b_tog'});
    });

    test('a comment start offset of 0 is kept (Go uses *int)', () {
      final golden = _golden();
      final comment = ModelDocument.fromJson(golden).comments.first;
      expect(comment.start, 0);
      expect(comment.toJson()['start'], 0);
      expect(const Comment(id: 'c', blockId: 'b', body: '').toJson(), {
        'id': 'c',
        'blockId': 'b',
        'body': '',
      });
    });

    test(
      'empty sections and outline encode as [] (Go decodes [] and null alike)',
      () {
        final json = const ModelDocument(
          schemaVersion: 'eden-press.model/v7',
          meta: Meta(directives: {}),
          sections: [],
          outline: [],
        ).toJson();
        expect(json, {
          'schemaVersion': 'eden-press.model/v7',
          'meta': <String, dynamic>{},
          'sections': <dynamic>[],
          'outline': <dynamic>[],
        });
      },
    );
  });

  // 1026-29 Test list 3: formatted table cells (docmodel v7 headerRuns and
  // rowRuns, added by 1026-25). The golden table b_table is written by Go.
  group('formatted table cells', () {
    Block golden(String id) =>
        ModelDocument.fromJson(_golden()).sections.first.blocks
            .firstWhere((b) => b.id == id);

    test('the golden table parses into typed headerRuns and rowRuns', () {
      final table = golden('b_table');
      expect(table.headerRuns, hasLength(2));
      expect(table.headerRuns[0]!.single.text, 'Metric');
      expect(table.headerRuns[0]!.single.marks, ['bold']);
      expect(table.rowRuns, hasLength(4));
      expect(table.rowRuns[0]![0]!.single.marks, ['code']);
      expect(table.rowRuns[0]![0]!.single.text, 'a|b');
      expect(table.rowRuns[2]![0]!, hasLength(2));
      expect(table.rowRuns[2]![0]![1].link, 'https://aocyber.ai/docs');
      expect(table.rowRuns[2]![1]!.single.marks, ['math']);
      // The ragged row has one cell and so one entry, not one per header.
      expect(table.rows[3], hasLength(1));
      expect(table.rowRuns[3], hasLength(1));
    });

    test('null entries survive fromJson as plain cells', () {
      final table = golden('b_table');
      expect(table.headerRuns[1], isNull);
      expect(table.rowRuns[0]![1], isNull);
      expect(table.rowRuns[1], isNull);
    });

    test('toJson writes the nulls back where Go wrote them', () {
      final json = golden('b_table').toJson();
      final headerRuns = json['headerRuns'] as List;
      expect(headerRuns, hasLength(2));
      expect(headerRuns[1], isNull);
      final rowRuns = json['rowRuns'] as List;
      expect(rowRuns, hasLength(4));
      expect((rowRuns[0] as List)[1], isNull);
      expect(rowRuns[1], isNull);
      expect(
        jsonEncode(json),
        jsonEncode(_goldenBlock(_golden(), 'b_table')),
        reason: 'the table must encode to Go\'s exact bytes, nulls included',
      );
    });

    test('a plain table writes neither key (golden and hand-built)', () {
      final plain = golden('b_table_plain');
      expect(plain.headerRuns, isEmpty);
      expect(plain.rowRuns, isEmpty);
      final json = plain.toJson();
      expect(json.containsKey('headerRuns'), isFalse);
      expect(json.containsKey('rowRuns'), isFalse);
      expect(
        jsonEncode(json),
        jsonEncode(_goldenBlock(_golden(), 'b_table_plain')),
      );
      final bare = Block(
        kind: BlockKind.table,
        text: '',
        level: 0,
        language: '',
        display: false,
        ordered: false,
        items: const [],
        headers: const ['H'],
        rows: const [
          ['c'],
        ],
      ).toJson();
      expect(bare.keys, ['kind', 'headers', 'rows']);
    });

    test('a hand-built block with cell runs encodes in Go key order', () {
      final json = Block(
        kind: BlockKind.table,
        text: '',
        level: 0,
        language: '',
        display: false,
        ordered: false,
        items: const [],
        headers: const ['H'],
        rows: const [
          ['c', 'd'],
        ],
        aligns: const ['left'],
        headerRuns: const [
          [
            TextRun(text: 'H', marks: ['bold']),
          ],
        ],
        rowRuns: const [
          [
            null,
            [TextRun(text: 'd', link: 'https://x.test')],
          ],
        ],
        src: 'after-runs',
      ).toJson();
      expect(json.keys, [
        'kind',
        'headers',
        'rows',
        'aligns',
        'headerRuns',
        'rowRuns',
        'src',
      ]);
      expect(
        jsonEncode(json['rowRuns']),
        '[[null,[{"text":"d","link":"https://x.test"}]]]',
      );
    });

    test('fromJson tolerates absent and null cell-run keys', () {
      for (final raw in [
        <String, dynamic>{'kind': 'table'},
        <String, dynamic>{'kind': 'table', 'headerRuns': null, 'rowRuns': null},
      ]) {
        final table = Block.fromJson(raw);
        expect(table.headerRuns, isEmpty);
        expect(table.rowRuns, isEmpty);
        expect(table.toJson(), {'kind': 'table'});
      }
    });
  });

  group('typed sheet embed', () {
    test('a formula cell parses into SheetCell and round-trips', () {
      final golden = _golden();
      final block = ModelDocument.fromJson(golden).sections.first.blocks
          .firstWhere((b) => b.id == 'b_sheet');
      final embed = block.typedSheetEmbed!;
      expect(embed.pageId, 'page_wb');
      expect(embed.sheet, 'Tasks');
      expect(embed.range!.r0, 0);
      expect(embed.range!.r1, 3);
      expect(embed.range!.columnIds, ['col_a', 'col_b']);
      expect(embed.snapshot!.columns.last.options, ['todo', 'done']);
      expect(embed.snapshot!.rowIds.last.id, 'row_1');

      final formula = embed.snapshot!.cells.firstWhere(
        (c) => c.r == 2 && c.c == 0,
      );
      expect(formula.f, 'SUM(A1:A2)');
      expect(formula.v, '7');
      final bare = embed.snapshot!.cells.firstWhere(
        (c) => c.r == 2 && c.c == 1,
      );
      expect(bare.f, 'A1*2');
      expect(bare.toJson(), {'r': 2, 'c': 1, 'f': 'A1*2'});
      final linked = embed.snapshot!.cells.firstWhere((c) => c.rels.isNotEmpty);
      expect(linked.rels.single['pageId'], 'page_9');

      expect(embed.toJson(), _goldenBlock(golden, 'b_sheet')['sheetEmbed']);
      expect(SheetEmbed.fromJson(embed.toJson()).toJson(), embed.toJson());
    });

    test('an all-zero range still writes "range": {}', () {
      final golden = _golden();
      final block = ModelDocument.fromJson(golden).sections.first.blocks
          .firstWhere((b) => b.id == 'b_sheet_live');
      expect(block.toJson()['sheetEmbed'], {
        'sheet': 'Live',
        'range': <String, dynamic>{},
      });
    });

    test('a typed SheetEmbed builds a sheet block whose toJson matches Go', () {
      const embed = SheetEmbed(
        sheet: 'Tasks',
        range: SheetRange(r0: 0, r1: 2),
        snapshot: SheetSnapshot(cells: [SheetCell(r: 0, c: 0, f: 'A1*2')]),
      );
      final block = Block(
        kind: BlockKind.sheet,
        text: '',
        level: 0,
        language: '',
        display: false,
        ordered: false,
        items: const [],
        sheetEmbed: embed.toJson(),
      );
      expect(block.toJson(), {
        'kind': 'sheet',
        'sheetEmbed': {
          'sheet': 'Tasks',
          'range': {'r1': 2},
          'snapshot': {
            'cells': [
              {'r': 0, 'c': 0, 'f': 'A1*2'},
            ],
          },
        },
      });
    });
  });
}
