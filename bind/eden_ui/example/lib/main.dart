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
import 'package:eden_ui/eden_ui.dart';
import 'package:flutter/material.dart';

void main() {
  runApp(const EdenDesktopDemo());
}

class EdenDesktopDemo extends StatelessWidget {
  const EdenDesktopDemo({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Eden Desktop Demo',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFFB8860B)),
        useMaterial3: true,
      ),
      home: const _HostShell(),
    );
  }
}

enum _Surface { sheets, slides, docs }

class _HostShell extends StatefulWidget {
  const _HostShell();

  @override
  State<_HostShell> createState() => _HostShellState();
}

class _HostShellState extends State<_HostShell> {
  _Surface _surface = _Surface.sheets;
  final List<String> _editLog = <String>[];

  late final EdenWorkbook _workbook = _metricsWorkbook();
  late final ModelDocument _slides = _deck();
  late final ModelDocument _docs = _article();

  void _onEdit(EdenEdit edit) {
    setState(() {
      // Host owns persistence: apply each edit into the workbook the next
      // EdenView mount will copy, so switching surfaces does not lose work.
      if (edit is EdenCellEdit) {
        _workbook.setCell(edit.sheet, edit.row, edit.column, edit.value);
        _editLog.insert(
          0,
          'cell ${edit.sheet}!${_col(edit.column)}${edit.row + 1}=${edit.value}',
        );
      } else if (edit is EdenSheetAdded) {
        if (_workbook.sheet(edit.name) == null) {
          _workbook.addSheet(edit.name);
        }
        _editLog.insert(0, 'added sheet ${edit.name}');
      }
      if (_editLog.length > 12) {
        _editLog.removeLast();
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Eden Desktop Demo'),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 16),
            child: Center(
              child: Text(
                key: const Key('edit-log-count'),
                'edits: ${_editLog.length}',
              ),
            ),
          ),
        ],
      ),
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _surface.index,
            onDestinationSelected: (i) {
              setState(() => _surface = _Surface.values[i]);
            },
            labelType: NavigationRailLabelType.all,
            destinations: const [
              NavigationRailDestination(
                icon: Icon(Icons.table_chart_outlined),
                selectedIcon: Icon(Icons.table_chart),
                label: Text('Sheets'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.slideshow_outlined),
                selectedIcon: Icon(Icons.slideshow),
                label: Text('Slides'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.article_outlined),
                selectedIcon: Icon(Icons.article),
                label: Text('Docs'),
              ),
            ],
          ),
          const VerticalDivider(width: 1),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Expanded(
                  child: KeyedSubtree(
                    key: ValueKey(_surface),
                    child: switch (_surface) {
                      _Surface.sheets => EdenView(
                        key: const Key('surface-sheets'),
                        workbook: _workbook,
                        onEdit: _onEdit,
                      ),
                      _Surface.slides => EdenView(
                        key: const Key('surface-slides'),
                        document: _slides,
                      ),
                      _Surface.docs => EdenView(
                        key: const Key('surface-docs'),
                        document: _docs,
                      ),
                    },
                  ),
                ),
                if (_editLog.isNotEmpty)
                  Material(
                    color: const Color(0xFFF5F5F5),
                    child: SizedBox(
                      height: 72,
                      child: ListView(
                        key: const Key('edit-log'),
                        padding: const EdgeInsets.all(8),
                        children: [
                          for (final line in _editLog)
                            Text(line, style: const TextStyle(fontSize: 12)),
                        ],
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

EdenWorkbook _metricsWorkbook() {
  final book = EdenWorkbook(title: 'Metrics');
  final sheet = book.addSheet('Metrics');
  sheet.setCell(0, 0, 'Metric');
  sheet.setCell(0, 1, 'Q3');
  sheet.setCell(1, 0, 'p95');
  sheet.setCell(1, 1, '550');
  sheet.setCell(2, 0, 'sum');
  sheet.setCell(2, 1, '=B2*2');
  sheet.headerRows.add(0);
  // Far cell for scroll drive.
  sheet.setCell(80, 0, 'far-row');
  return book;
}

ModelDocument _deck() {
  return ModelDocument(
    schemaVersion: ModelDocument.expectedSchemaVersion,
    meta: const Meta(directives: {'marp': 'true'}),
    sections: [
      Section(
        id: 1,
        attrs: const {},
        notes: const [],
        blocks: [
          const Block(
            kind: BlockKind.heading,
            text: 'Alpha',
            level: 1,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
          const Block(
            kind: BlockKind.paragraph,
            text: 'First slide body',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
        ],
      ),
      Section(
        id: 2,
        attrs: const {},
        notes: const [],
        blocks: [
          const Block(
            kind: BlockKind.heading,
            text: 'Beta',
            level: 1,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
          const Block(
            kind: BlockKind.paragraph,
            text: 'Second slide body',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
        ],
      ),
      Section(
        id: 3,
        attrs: const {},
        notes: const [],
        blocks: [
          const Block(
            kind: BlockKind.heading,
            text: 'Gamma',
            level: 1,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
        ],
      ),
    ],
    outline: const [],
  );
}

ModelDocument _article() {
  return ModelDocument(
    schemaVersion: ModelDocument.expectedSchemaVersion,
    meta: const Meta(directives: {'profile': 'article'}),
    sections: [
      Section(
        id: 1,
        attrs: const {},
        notes: const [],
        blocks: [
          const Block(
            kind: BlockKind.heading,
            text: 'Roadmap',
            level: 1,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
          const Block(
            kind: BlockKind.paragraph,
            text: 'Alpha paragraph for the flowing document.',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
          const Block(
            kind: BlockKind.todo,
            text: 'Ship desktop demo',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            checked: true,
          ),
          const Block(
            kind: BlockKind.callout,
            text: 'Watch p95 latency',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            icon: '!',
            tone: 'warn',
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
                  {'r': 0, 'c': 0, 'v': 'Owner'},
                  {'r': 0, 'c': 1, 'v': 'Status'},
                  {'r': 1, 'c': 0, 'v': 'Ada'},
                  {'r': 1, 'c': 1, 'v': 'Done'},
                ],
              },
            },
          ),
          const Block(
            kind: BlockKind.figure,
            text: 'logo',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            svg:
                '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="#B8860B"/><text x="12" y="40" fill="white" font-size="18">EP</text></svg>',
          ),
          const Block(
            kind: BlockKind.toggle,
            text: 'Details',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
            children: [
              Block(
                kind: BlockKind.paragraph,
                text: 'Nested under toggle',
                level: 0,
                language: '',
                display: false,
                ordered: false,
                items: [],
              ),
            ],
          ),
          const Block(
            kind: BlockKind.code,
            text: 'fmt.Println("eden")',
            level: 0,
            language: 'go',
            display: false,
            ordered: false,
            items: [],
          ),
          const Block(
            kind: BlockKind.math,
            text: 'E=mc^2',
            level: 0,
            language: '',
            display: true,
            ordered: false,
            items: [],
          ),
        ],
      ),
      Section(
        id: 2,
        attrs: const {},
        notes: const [],
        blocks: [
          const Block(
            kind: BlockKind.paragraph,
            text: 'Beta paragraph stays visible with Alpha.',
            level: 0,
            language: '',
            display: false,
            ordered: false,
            items: [],
          ),
        ],
      ),
    ],
    outline: const [],
  );
}

String _col(int index) {
  var n = index;
  final chars = <int>[];
  while (n >= 0) {
    chars.add(65 + (n % 26));
    n = (n ~/ 26) - 1;
  }
  return String.fromCharCodes(chars.reversed);
}
