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

/// Excel's worksheet limits. A cell outside them cannot be stored. These match
/// `maxSheetRows` and `maxSheetCols` in `convert/xlsx`.
const sheetRowLimit = 1048576;
const sheetColumnLimit = 16384;

/// JSON schema shared with `convert/xlsx.Workbook` in the Go engine.
/// Writers emit v2. [EdenWorkbook.fromJson] still reads v1.
const workbookSchema = 'eden-press.workbook/v2';

/// Rectangular snapshot written before cells carried their own coordinates.
const workbookSchemaV1 = 'eden-press.workbook/v1';

/// One worksheet. Cells are addressed 0-based and stored sparse. A value that
/// starts with `=` is a formula; [cell] returns it with that prefix so the
/// formula bar can show what the author typed.
class EdenSheet {
  EdenSheet(this.name);

  String name;
  final Map<int, Map<int, String>> _cells = {};
  final Set<int> headerRows = {};
  final List<String> aligns = [];
  int maxRow = -1;
  int maxCol = -1;

  /// Called after a cell write so the workbook can drop cached formula results.
  void Function(int row, int column)? onChanged;

  String cell(int row, int column) => _cells[row]?[column] ?? '';

  void setCell(int row, int column, String value) {
    if (row < 0 ||
        row >= sheetRowLimit ||
        column < 0 ||
        column >= sheetColumnLimit) {
      throw ArgumentError(
        'cell $row,$column is outside 0..${sheetRowLimit - 1} x 0..${sheetColumnLimit - 1}',
      );
    }
    final rowCells = _cells.putIfAbsent(row, () => {});
    if (value.isEmpty) {
      rowCells.remove(column);
      if (rowCells.isEmpty) {
        _cells.remove(row);
      }
    } else {
      rowCells[column] = value;
    }
    _recomputeExtent();
    onChanged?.call(row, column);
  }

  /// Visits stored non-empty cells inside the inclusive rectangle. Blank
  /// addresses are not visited.
  void visitStored(
    int row0,
    int column0,
    int row1,
    int column1,
    void Function(int row, int column) visit,
  ) {
    for (final entry in _cells.entries) {
      final row = entry.key;
      if (row < row0 || row > row1) {
        continue;
      }
      for (final column in entry.value.keys) {
        if (column < column0 || column > column1) {
          continue;
        }
        final value = entry.value[column] ?? '';
        if (value.isEmpty) {
          continue;
        }
        visit(row, column);
      }
    }
  }

  void _recomputeExtent() {
    maxRow = -1;
    maxCol = -1;
    _cells.forEach((row, cols) {
      if (cols.isEmpty) {
        return;
      }
      if (row > maxRow) {
        maxRow = row;
      }
      for (final column in cols.keys) {
        if (column > maxCol) {
          maxCol = column;
        }
      }
    });
    for (final row in headerRows) {
      if (row > maxRow) {
        maxRow = row;
      }
    }
  }

  EdenSheet copy() {
    final next = EdenSheet(name);
    _cells.forEach((row, cols) {
      next._cells[row] = Map<int, String>.from(cols);
    });
    next.headerRows.addAll(headerRows);
    next.aligns.addAll(aligns);
    next.maxRow = maxRow;
    next.maxCol = maxCol;
    return next;
  }
}

/// The spreadsheet document eden-ui edits. An `.xlsx` file is the interchange
/// format; this object is the editing snapshot (`workbookSchema`).
class EdenWorkbook {
  EdenWorkbook({this.title = '', List<EdenSheet>? sheets})
    : sheets = sheets ?? <EdenSheet>[] {
    _listen();
  }

  String title;
  final List<EdenSheet> sheets;
  final Map<String, String> _display = {};
  final Map<String, List<CellSpan>> _reads = {};

  List<String> get sheetNames => [for (final sheet in sheets) sheet.name];

  EdenSheet? sheet(String name) {
    for (final candidate in sheets) {
      if (candidate.name == name) {
        return candidate;
      }
    }
    return null;
  }

  /// Sheet names in formulas are case-insensitive, matching Excel. Returns
  /// the stored name so later cell lookups hit the sheet map.
  String? canonicalSheetName(String name) {
    final folded = name.toLowerCase();
    for (final candidate in sheets) {
      if (candidate.name.toLowerCase() == folded) {
        return candidate.name;
      }
    }
    return null;
  }

  String cell(String sheetName, int row, int column) {
    return sheet(sheetName)?.cell(row, column) ?? '';
  }

  void setCell(String sheetName, int row, int column, String value) {
    final target = sheet(sheetName);
    if (target == null) {
      throw ArgumentError('sheet $sheetName not found');
    }
    target.setCell(row, column, value);
  }

  EdenSheet addSheet(String name) {
    if (name.trim().isEmpty || sheet(name) != null) {
      throw ArgumentError('sheet name $name is empty or already used');
    }
    final created = EdenSheet(name);
    sheets.add(created);
    _listen();
    return created;
  }

  /// Next `SheetN` name that is not already taken.
  String freshSheetName() {
    var n = sheets.length + 1;
    while (sheet('Sheet$n') != null) {
      n++;
    }
    return 'Sheet$n';
  }

  EdenWorkbook copy() {
    return EdenWorkbook(
      title: title,
      sheets: [for (final sheet in sheets) sheet.copy()],
    );
  }

  String? cachedDisplay(String sheetName, int row, int column) {
    return _display[_cacheKey(sheetName, row, column)];
  }

  void cacheDisplay(
    String sheetName,
    int row,
    int column,
    String shown,
    List<CellSpan> reads,
  ) {
    final key = _cacheKey(sheetName, row, column);
    _display[key] = shown;
    _reads[key] = reads;
  }

  /// Records the cells a formula read, without caching its display text.
  /// Nested formulas use this so a later edit inside their range still
  /// invalidates formulas that depend on them.
  void noteReads(String sheetName, int row, int column, List<CellSpan> reads) {
    _reads[_cacheKey(sheetName, row, column)] = reads;
  }

  void invalidateCell(String sheetName, int row, int column) {
    final dirty = <String>{_cacheKey(sheetName, row, column)};
    var grew = true;
    while (grew) {
      grew = false;
      for (final entry in _reads.entries) {
        if (dirty.contains(entry.key)) {
          continue;
        }
        for (final span in entry.value) {
          if (_spanHits(span, dirty)) {
            dirty.add(entry.key);
            grew = true;
            break;
          }
        }
      }
    }
    for (final key in dirty) {
      _display.remove(key);
      _reads.remove(key);
    }
  }

  void _listen() {
    for (final sheet in sheets) {
      sheet.onChanged = (row, column) =>
          invalidateCell(sheet.name, row, column);
    }
  }

  bool _spanHits(CellSpan span, Set<String> dirty) {
    for (final key in dirty) {
      final at = key.split('\u0000');
      if (at.length != 3) {
        continue;
      }
      if (span.covers(at[0], int.parse(at[1]), int.parse(at[2]))) {
        return true;
      }
    }
    return false;
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    'schema': workbookSchema,
    if (title.isNotEmpty) 'title': title,
    'sheets': [for (final sheet in sheets) _sheetJson(sheet)],
  };

  factory EdenWorkbook.fromJson(Map<String, dynamic> json) {
    final schema = json['schema'];
    if (schema != workbookSchema && schema != workbookSchemaV1) {
      throw FormatException(
        'workbook schema $schema, want $workbookSchema or $workbookSchemaV1',
      );
    }
    final book = EdenWorkbook(title: json['title'] as String? ?? '');
    final rawSheets = json['sheets'];
    if (rawSheets is! List) {
      return book;
    }
    for (final raw in rawSheets) {
      if (raw is! Map) {
        continue;
      }
      final name = raw['name'] as String? ?? '';
      final sheet = book.addSheet(name);
      final aligns = raw['aligns'];
      if (aligns is List) {
        sheet.aligns.addAll(aligns.map((e) => e?.toString() ?? ''));
      }
      if (schema == workbookSchemaV1) {
        _readV1Rows(sheet, raw['rows']);
      } else {
        _readV2(sheet, raw);
      }
    }
    return book;
  }

  /// Lifts markdown tables into worksheets: one sheet per section that has a
  /// table, named from that section's first heading. This is how a rendered
  /// Eden document becomes an editable spreadsheet.
  factory EdenWorkbook.fromDocument(ModelDocument document) {
    final book = EdenWorkbook(title: document.meta.directives['title'] ?? '');
    var index = 0;
    for (final section in document.sections) {
      final tables = [
        for (final block in section.blocks)
          if (block.kind == BlockKind.table) block,
      ];
      if (tables.isEmpty) {
        continue;
      }
      index++;
      var name = '';
      for (final block in section.blocks) {
        if (block.kind == BlockKind.heading && block.text.trim().isNotEmpty) {
          name = block.text.trim();
          break;
        }
      }
      if (name.isEmpty) {
        name = 'Sheet$index';
      }
      name = _cleanSheetName(name);
      if (book.sheet(name) != null) {
        name = _cleanSheetName('$name ($index)');
      }
      final sheet = book.addSheet(name);
      var row = 0;
      var stacked = false;
      for (final table in tables) {
        if (stacked) {
          row++;
        }
        stacked = true;
        final columns = _columnCount(table);
        if (table.headers.isNotEmpty) {
          sheet.headerRows.add(row);
          for (var c = 0; c < columns; c++) {
            final text = c < table.headers.length ? table.headers[c] : '';
            if (text.isNotEmpty) {
              sheet.setCell(row, c, text);
            }
          }
          row++;
        }
        for (final cells in table.rows) {
          for (var c = 0; c < columns && c < cells.length; c++) {
            if (cells[c].isNotEmpty) {
              sheet.setCell(row, c, cells[c]);
            }
          }
          row++;
        }
      }
    }
    return book;
  }
}

void _readV1Rows(EdenSheet sheet, Object? rawRows) {
  if (rawRows is! List) {
    return;
  }
  for (var r = 0; r < rawRows.length; r++) {
    final row = rawRows[r];
    if (row is! Map) {
      continue;
    }
    if (row['header'] == true) {
      _markHeader(sheet, r);
    }
    final cells = row['cells'];
    if (cells is! List) {
      continue;
    }
    for (var c = 0; c < cells.length; c++) {
      final cell = cells[c];
      if (cell is! Map) {
        continue;
      }
      _applyCell(sheet, r, c, cell);
    }
  }
}

void _readV2(EdenSheet sheet, Map raw) {
  final headers = raw['headerRows'];
  if (headers is List) {
    for (final row in headers) {
      _markHeader(sheet, _asInt(row));
    }
  }
  final cells = raw['cells'];
  if (cells is! List) {
    return;
  }
  for (final cell in cells) {
    if (cell is! Map) {
      continue;
    }
    _applyCell(sheet, _asInt(cell['r']), _asInt(cell['c']), cell);
  }
}

void _markHeader(EdenSheet sheet, int row) {
  sheet.headerRows.add(row);
  if (row > sheet.maxRow) {
    sheet.maxRow = row;
  }
}

void _applyCell(EdenSheet sheet, int row, int column, Map cell) {
  final formula = cell['f'] as String? ?? '';
  final value = cell['v'] as String? ?? '';
  if (formula.isNotEmpty && value.isNotEmpty) {
    throw FormatException('cell $row,$column has both v and f');
  }
  if (formula.isNotEmpty) {
    sheet.setCell(row, column, '=$formula');
  } else if (value.isNotEmpty) {
    sheet.setCell(row, column, value);
  }
}

int _asInt(Object? value) {
  if (value is int) {
    return value;
  }
  if (value is num) {
    return value.toInt();
  }
  return 0;
}

Map<String, dynamic> _sheetJson(EdenSheet sheet) {
  final cells = <Map<String, dynamic>>[];
  final rowNumbers = sheet._cells.keys.toList()..sort();
  for (final row in rowNumbers) {
    final columns = sheet._cells[row]!.keys.toList()..sort();
    for (final column in columns) {
      final raw = sheet._cells[row]![column] ?? '';
      if (raw.startsWith('=')) {
        cells.add({'r': row, 'c': column, 'f': raw.substring(1)});
      } else if (raw.isNotEmpty) {
        cells.add({'r': row, 'c': column, 'v': raw});
      }
    }
  }
  final headers = sheet.headerRows.toList()..sort();
  final aligns = _trimAligns(sheet.aligns);
  return {
    'name': sheet.name,
    if (aligns.isNotEmpty) 'aligns': aligns,
    if (headers.isNotEmpty) 'headerRows': headers,
    'cells': cells,
  };
}

List<String> _trimAligns(List<String> aligns) {
  var end = aligns.length;
  while (end > 0 && aligns[end - 1].isEmpty) {
    end--;
  }
  return aligns.take(end).toList();
}

int _columnCount(Block table) {
  var columns = table.headers.length;
  for (final row in table.rows) {
    if (table.headers.isEmpty && row.length > columns) {
      columns = row.length;
    }
  }
  return columns;
}

String _cacheKey(String sheetName, int row, int column) {
  return '$sheetName\u0000$row\u0000$column';
}

/// A cell or inclusive rectangle that a formula read.
class CellSpan {
  CellSpan.cell(String sheet, int row, int column)
    : this(sheet, row, column, row, column);

  CellSpan(this.sheet, this.row0, this.column0, this.row1, this.column1);

  final String sheet;
  final int row0;
  final int column0;
  final int row1;
  final int column1;

  bool covers(String sheetName, int row, int column) {
    return sheet == sheetName &&
        row >= row0 &&
        row <= row1 &&
        column >= column0 &&
        column <= column1;
  }
}

String _cleanSheetName(String name) {
  final cleaned = name.replaceAll(RegExp(r'[:\\/?*\[\]]'), '').trim();
  if (cleaned.isEmpty) {
    return 'Sheet';
  }
  return cleaned.runes.length <= 31
      ? cleaned
      : String.fromCharCodes(cleaned.runes.take(31));
}
