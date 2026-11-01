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
import 'package:flutter/services.dart';
import 'package:two_dimensional_scrollables/two_dimensional_scrollables.dart';

import 'edit.dart';
import 'formula.dart';
import 'workbook.dart';

/// Grid editor for one [EdenWorkbook]: sheet tabs, a formula bar, and cells
/// that show formula results. Edits are reported through [onEdit].
///
/// Cells edit in place: click an already-selected cell, press F2, or start
/// typing. Enter commits; Escape cancels. The formula bar stays in sync with
/// the in-cell editor.
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
  bool _editing = false;
  /// After focus, keep the caret at the end (do not re-select the seed char).
  bool _caretAtEndAfterFocus = false;
  final TextEditingController _bar = TextEditingController();
  final FocusNode _gridFocus = FocusNode();
  final FocusNode _cellFocus = FocusNode();
  final FocusNode _barFocus = FocusNode();

  @override
  void initState() {
    super.initState();
    _book = widget.workbook.copy();
    _sheetName = _book.sheetNames.isEmpty ? '' : _book.sheetNames.first;
    _cellFocus.addListener(_onCellFocusChange);
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
      _editing = false;
      _caretAtEndAfterFocus = false;
      _syncBar();
    }
  }

  @override
  void dispose() {
    _cellFocus.removeListener(_onCellFocusChange);
    _bar.dispose();
    _gridFocus.dispose();
    _cellFocus.dispose();
    _barFocus.dispose();
    super.dispose();
  }

  void _syncBar() {
    if (_sheetName.isEmpty) {
      _bar.text = '';
      return;
    }
    _bar.text = _book.cell(_sheetName, _row, _column);
  }

  void _collapseCaretToEnd() {
    final offset = _bar.text.length;
    _bar.selection = TextSelection.collapsed(offset: offset);
  }

  void _onCellFocusChange() {
    // Web/desktop TextField often select-all on focus, which highlights the
    // first typed character and makes the next keystroke overwrite it.
    if (_cellFocus.hasFocus && _editing && _caretAtEndAfterFocus) {
      _collapseCaretToEnd();
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted || !_editing || !_caretAtEndAfterFocus) {
          return;
        }
        _collapseCaretToEnd();
        _caretAtEndAfterFocus = false;
      });
    }
  }

  void _select(int row, int column) {
    setState(() {
      _editing = false;
      _caretAtEndAfterFocus = false;
      _row = row;
      _column = column;
      _syncBar();
    });
    _gridFocus.requestFocus();
  }

  void _beginEdit({String? overwriteWith}) {
    if (_sheetName.isEmpty) {
      return;
    }
    // Typing while selected seeds the first character; keep the caret after
    // it. Click/F2 edit places the caret at the end of the existing value
    // (not select-all) so the first keystroke inserts instead of replacing
    // and then getting re-selected by focus.
    final seed = overwriteWith;
    setState(() {
      _editing = true;
      _caretAtEndAfterFocus = true;
      if (seed != null) {
        _bar.text = seed;
      } else {
        _syncBar();
      }
      _collapseCaretToEnd();
    });
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_editing) {
        return;
      }
      _cellFocus.requestFocus();
      _collapseCaretToEnd();
    });
  }

  void _cancelEdit() {
    setState(() {
      _editing = false;
      _caretAtEndAfterFocus = false;
      _syncBar();
    });
    _gridFocus.requestFocus();
  }

  void _commitBar(String value) {
    if (_sheetName.isEmpty) {
      return;
    }
    setState(() {
      _book.setCell(_sheetName, _row, _column, value);
      _editing = false;
      _caretAtEndAfterFocus = false;
    });
    widget.onEdit?.call(
      EdenCellEdit(sheet: _sheetName, row: _row, column: _column, value: value),
    );
    _gridFocus.requestFocus();
  }

  /// Excel-style navigation: clamp to the sheet limits and grow the visible
  /// grid when the selection walks into empty cells.
  void _moveSelection(int rowDelta, int columnDelta) {
    if (_sheetName.isEmpty || (rowDelta == 0 && columnDelta == 0)) {
      return;
    }
    final row = (_row + rowDelta).clamp(0, sheetRowLimit - 1);
    final column = (_column + columnDelta).clamp(0, sheetColumnLimit - 1);
    if (row == _row && column == _column) {
      _gridFocus.requestFocus();
      return;
    }
    _select(row, column);
  }

  /// Commit the in-progress edit (if any), then move like Excel.
  void _commitAndMove({required int rowDelta, required int columnDelta}) {
    if (_sheetName.isEmpty) {
      return;
    }
    if (_editing) {
      _commitBar(_bar.text);
    }
    _moveSelection(rowDelta, columnDelta);
  }

  void _addSheet() {
    if (_editing) {
      _commitBar(_bar.text);
    }
    final name = _book.freshSheetName();
    setState(() {
      _book.addSheet(name);
      _sheetName = name;
      _row = 0;
      _column = 0;
      _editing = false;
      _caretAtEndAfterFocus = false;
      _syncBar();
    });
    widget.onEdit?.call(EdenSheetAdded(name));
  }

  bool get _shiftPressed => HardwareKeyboard.instance.isShiftPressed;

  KeyEventResult _onGridKey(FocusNode node, KeyEvent event) {
    if (_editing || event is! KeyDownEvent || _sheetName.isEmpty) {
      return KeyEventResult.ignored;
    }
    if (event.logicalKey == LogicalKeyboardKey.f2) {
      _beginEdit();
      return KeyEventResult.handled;
    }
    // Excel: Enter/Tab move the selection when not editing.
    if (event.logicalKey == LogicalKeyboardKey.enter ||
        event.logicalKey == LogicalKeyboardKey.numpadEnter) {
      _moveSelection(_shiftPressed ? -1 : 1, 0);
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.tab) {
      _moveSelection(0, _shiftPressed ? -1 : 1);
      return KeyEventResult.handled;
    }
    final ch = event.character;
    if (ch == null || ch.isEmpty) {
      return KeyEventResult.ignored;
    }
    // Ignore pure control/meta chords; allow shift for symbols.
    final keys = HardwareKeyboard.instance.logicalKeysPressed;
    if (keys.contains(LogicalKeyboardKey.controlLeft) ||
        keys.contains(LogicalKeyboardKey.controlRight) ||
        keys.contains(LogicalKeyboardKey.metaLeft) ||
        keys.contains(LogicalKeyboardKey.metaRight) ||
        keys.contains(LogicalKeyboardKey.altLeft) ||
        keys.contains(LogicalKeyboardKey.altRight)) {
      return KeyEventResult.ignored;
    }
    if (ch == '\n' || ch == '\r' || ch == '\t') {
      return KeyEventResult.ignored;
    }
    _beginEdit(overwriteWith: ch);
    return KeyEventResult.handled;
  }

  KeyEventResult _onEditorKey(FocusNode node, KeyEvent event) {
    if (!_editing || event is! KeyDownEvent) {
      return KeyEventResult.ignored;
    }
    if (event.logicalKey == LogicalKeyboardKey.escape) {
      _cancelEdit();
      return KeyEventResult.handled;
    }
    // Excel: Enter commits and moves down; Tab commits and moves right.
    // Shift reverses the direction.
    if (event.logicalKey == LogicalKeyboardKey.enter ||
        event.logicalKey == LogicalKeyboardKey.numpadEnter) {
      _commitAndMove(rowDelta: _shiftPressed ? -1 : 1, columnDelta: 0);
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.tab) {
      _commitAndMove(rowDelta: 0, columnDelta: _shiftPressed ? -1 : 1);
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
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
              child: Focus(
                onKeyEvent: _onEditorKey,
                child: TextField(
                  key: const Key('eden-formula-bar'),
                  focusNode: _barFocus,
                  controller: _bar,
                  decoration: const InputDecoration(
                    isDense: true,
                    border: OutlineInputBorder(),
                    hintText: 'Value or =formula',
                  ),
                  onTap: () {
                    if (!_editing) {
                      setState(() => _editing = true);
                    }
                  },
                  onChanged: (_) {
                    if (!_editing) {
                      setState(() => _editing = true);
                    }
                  },
                  // Enter is handled in [_onEditorKey] (commit + move). This
                  // IME fallback only runs if the key path did not already
                  // commit.
                  onSubmitted: (value) {
                    if (_editing) {
                      _commitAndMove(rowDelta: 1, columnDelta: 0);
                    }
                  },
                ),
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
                if (_editing) {
                  _commitBar(_bar.text);
                }
                setState(() {
                  _sheetName = name;
                  _row = 0;
                  _column = 0;
                  _editing = false;
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
    final rows = _displayedRows(sheet, selectedRow: _row);
    final cols = _displayedCols(sheet, selectedColumn: _column);
    return Focus(
      focusNode: _gridFocus,
      onKeyEvent: _onGridKey,
      child: TableView.builder(
        pinnedRowCount: 1,
        pinnedColumnCount: 1,
        diagonalDragBehavior: DiagonalDragBehavior.free,
        rowCount: rows + 1,
        columnCount: cols + 1,
        rowBuilder: (index) =>
            const TableSpan(extent: FixedTableSpanExtent(28)),
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
            child: SizedBox.expand(
              child: _cell(sheet, vicinity.row - 1, vicinity.column - 1),
            ),
          );
        },
      ),
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
    final editingHere = selected && _editing;

    if (editingHere) {
      return Container(
        key: Key('eden-cell-$row-$column'),
        alignment: Alignment.centerLeft,
        padding: const EdgeInsets.symmetric(horizontal: 2),
        decoration: BoxDecoration(
          color: Colors.white,
          border: Border.all(color: const Color(0xFF0B57D0), width: 2),
        ),
        child: Focus(
          onKeyEvent: _onEditorKey,
          child: TextField(
            key: const Key('eden-cell-editor'),
            focusNode: _cellFocus,
            controller: _bar,
            style: const TextStyle(fontSize: 13),
            cursorHeight: 16,
            decoration: const InputDecoration(
              isDense: true,
              border: InputBorder.none,
              contentPadding: EdgeInsets.symmetric(horizontal: 2, vertical: 4),
            ),
            onSubmitted: (value) {
              if (_editing) {
                _commitAndMove(rowDelta: 1, columnDelta: 0);
              }
            },
          ),
        ),
      );
    }

    final shown = displayCell(_book, sheet.name, row, column);
    // No onDoubleTap: pairing it with onTap delays selection until the
    // double-tap timeout fires, which feels broken for a spreadsheet.
    return GestureDetector(
      key: Key('eden-cell-$row-$column'),
      behavior: HitTestBehavior.opaque,
      onTap: () {
        if (_editing && (row != _row || column != _column)) {
          _commitBar(_bar.text);
          _select(row, column);
          return;
        }
        if (selected) {
          _beginEdit();
          return;
        }
        _select(row, column);
      },
      child: Container(
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

/// At least eight rows and four columns, two past the occupied/selected cell,
/// and never past Excel's grid. The table builds only the visible window.
int _displayedRows(EdenSheet sheet, {int selectedRow = 0}) {
  final extent = sheet.maxRow > selectedRow ? sheet.maxRow : selectedRow;
  final rows = extent < 7 ? 8 : extent + 2;
  return rows > sheetRowLimit ? sheetRowLimit : rows;
}

int _displayedCols(EdenSheet sheet, {int selectedColumn = 0}) {
  final extent = sheet.maxCol > selectedColumn ? sheet.maxCol : selectedColumn;
  final cols = extent < 3 ? 4 : extent + 2;
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
