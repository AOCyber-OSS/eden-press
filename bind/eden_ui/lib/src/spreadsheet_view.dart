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

import 'package:flutter/material.dart';
import 'package:two_dimensional_scrollables/two_dimensional_scrollables.dart';

import 'edit.dart';
import 'formula.dart';
import 'workbook.dart';

/// Grid editor for one [EdenWorkbook]: sheet tabs, a formula bar, and cells
/// that show formula results. Edits are reported through [onEdit].
class SpreadsheetView extends StatefulWidget {
  const SpreadsheetView({super.key, required this.workbook, this.onEdit});

  final EdenWorkbook workbook;
  final ValueChanged<EdenEdit>? onEdit;

  @override
  State<SpreadsheetView> createState() => _SpreadsheetViewState();
}

class _SpreadsheetViewState extends State<SpreadsheetView> {
  late EdenWorkbook _book;
  late String _sheetName;
  int _row = 0;
  int _column = 0;
  final TextEditingController _bar = TextEditingController();

  @override
  void initState() {
    super.initState();
    _book = widget.workbook.copy();
    _sheetName = _book.sheetNames.isEmpty ? '' : _book.sheetNames.first;
    _syncBar();
  }

  @override
  void didUpdateWidget(SpreadsheetView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.workbook, widget.workbook)) {
      _book = widget.workbook.copy();
      if (_book.sheet(_sheetName) == null) {
        _sheetName = _book.sheetNames.isEmpty ? '' : _book.sheetNames.first;
        _row = 0;
        _column = 0;
      }
      _syncBar();
    }
  }

  @override
  void dispose() {
    _bar.dispose();
    super.dispose();
  }

  void _syncBar() {
    if (_sheetName.isEmpty) {
      _bar.text = '';
      return;
    }
    _bar.text = _book.cell(_sheetName, _row, _column);
  }

  void _select(int row, int column) {
    setState(() {
      _row = row;
      _column = column;
      _syncBar();
    });
  }

  void _commitBar(String value) {
    if (_sheetName.isEmpty) {
      return;
    }
    setState(() {
      _book.setCell(_sheetName, _row, _column, value);
    });
    widget.onEdit?.call(
      EdenCellEdit(sheet: _sheetName, row: _row, column: _column, value: value),
    );
  }

  void _addSheet() {
    final name = _book.freshSheetName();
    setState(() {
      _book.addSheet(name);
      _sheetName = name;
      _row = 0;
      _column = 0;
      _syncBar();
    });
    widget.onEdit?.call(EdenSheetAdded(name));
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _formulaBar(),
        _tabs(),
        Expanded(child: _sheetName.isEmpty ? const SizedBox.shrink() : _grid()),
      ],
    );
  }

  Widget _formulaBar() {
    final address = _sheetName.isEmpty
        ? ''
        : '${_columnLabel(_column)}${_row + 1}';
    return Material(
      color: const Color(0xFFF7F7F7),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        child: Row(
          children: [
            SizedBox(
              width: 56,
              child: Text(
                address,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ),
            Expanded(
              child: TextField(
                key: const Key('eden-formula-bar'),
                controller: _bar,
                decoration: const InputDecoration(
                  isDense: true,
                  border: OutlineInputBorder(),
                  hintText: 'Value or =formula',
                ),
                onSubmitted: _commitBar,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _tabs() {
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        children: [
          for (final name in _book.sheetNames)
            TextButton(
              onPressed: () {
                setState(() {
                  _sheetName = name;
                  _row = 0;
                  _column = 0;
                  _syncBar();
                });
              },
              style: TextButton.styleFrom(
                foregroundColor: name == _sheetName
                    ? const Color(0xFF0B57D0)
                    : const Color(0xFF444444),
              ),
              child: Text(name),
            ),
          IconButton(
            key: const Key('eden-add-sheet'),
            tooltip: 'Add sheet',
            onPressed: _addSheet,
            icon: const Icon(Icons.add),
          ),
        ],
      ),
    );
  }

  Widget _grid() {
    final sheet = _book.sheet(_sheetName)!;
    final rows = _displayedRows(sheet);
    final cols = _displayedCols(sheet);
    return TableView.builder(
      pinnedRowCount: 1,
      pinnedColumnCount: 1,
      diagonalDragBehavior: DiagonalDragBehavior.free,
      rowCount: rows + 1,
      columnCount: cols + 1,
      rowBuilder: (index) => TableSpan(extent: const FixedTableSpanExtent(28)),
      columnBuilder: (index) {
        return TableSpan(extent: FixedTableSpanExtent(index == 0 ? 40 : 96));
      },
      cellBuilder: (context, vicinity) {
        if (vicinity.row == 0 && vicinity.column == 0) {
          return const TableViewCell(child: _Corner());
        }
        if (vicinity.row == 0) {
          return TableViewCell(
            child: _label(_columnLabel(vicinity.column - 1), width: 96),
          );
        }
        if (vicinity.column == 0) {
          return TableViewCell(child: _label('${vicinity.row}', width: 40));
        }
        return TableViewCell(
          child: _cell(sheet, vicinity.row - 1, vicinity.column - 1),
        );
      },
    );
  }

  Widget _label(String text, {required double width}) {
    return Container(
      width: width,
      height: 28,
      alignment: Alignment.center,
      color: const Color(0xFFF0F0F0),
      child: Text(text, style: const TextStyle(fontSize: 12)),
    );
  }

  Widget _cell(EdenSheet sheet, int row, int column) {
    final selected = row == _row && column == _column;
    final header = sheet.headerRows.contains(row);
    final shown = displayCell(_book, sheet.name, row, column);
    return GestureDetector(
      onTap: () => _select(row, column),
      child: Container(
        width: 96,
        height: 28,
        alignment: Alignment.centerLeft,
        padding: const EdgeInsets.symmetric(horizontal: 4),
        decoration: BoxDecoration(
          color: header ? const Color(0xFFF7F7F7) : Colors.white,
          border: Border.all(
            color: selected ? const Color(0xFF0B57D0) : const Color(0xFFE0E0E0),
            width: selected ? 1.5 : 0.5,
          ),
        ),
        child: Text(
          shown,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(
            fontWeight: header ? FontWeight.w600 : FontWeight.w400,
            fontSize: 13,
          ),
        ),
      ),
    );
  }
}

/// At least eight rows and four columns, two past the occupied cell, and
/// never past Excel's grid. The table builds only the visible window.
int _displayedRows(EdenSheet sheet) {
  final rows = sheet.maxRow < 7 ? 8 : sheet.maxRow + 2;
  return rows > sheetRowLimit ? sheetRowLimit : rows;
}

int _displayedCols(EdenSheet sheet) {
  final cols = sheet.maxCol < 3 ? 4 : sheet.maxCol + 2;
  return cols > sheetColumnLimit ? sheetColumnLimit : cols;
}

class _Corner extends StatelessWidget {
  const _Corner();

  @override
  Widget build(BuildContext context) {
    return const ColoredBox(color: Color(0xFFF0F0F0));
  }
}

String _columnLabel(int index) {
  var n = index;
  final chars = <int>[];
  while (n >= 0) {
    chars.add(65 + (n % 26));
    n = (n ~/ 26) - 1;
  }
  return String.fromCharCodes(chars.reversed);
}
