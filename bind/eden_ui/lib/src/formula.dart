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

import 'workbook.dart';

/// A formula the grid could not evaluate. [code] is what the cell shows.
class FormulaError implements Exception {
  FormulaError(this.code);
  final String code;
}

/// Evaluates [formula] (no leading "=") against [book], starting from [sheet].
///
/// The supported language is the subset a spreadsheet grid needs in order to
/// show results while editing: numbers, `+ - * /`, parentheses, cell
/// references (`A1`, `Sheet!A1`), and `SUM` / `AVERAGE` / `MIN` / `MAX`,
/// including ranges (`A1:A3`). `SUM`, `AVERAGE`, `MIN`, and `MAX` ignore
/// blank cells and only visit stored values. This is not Excel's formula
/// language. The stored formula is still what an `.xlsx` export asks Excel
/// to calculate.
double evaluateFormula(
  EdenWorkbook book,
  String sheet,
  String formula, {
  List<CellSpan>? reads,
}) {
  final parser = _Parser(book, sheet, formula, reads: reads);
  final value = parser.parseExpression();
  parser.skip();
  if (!parser.done) {
    throw FormulaError('#ERR');
  }
  return value;
}

/// Formats [n] for a grid cell. Whole numbers stay whole.
String formatNumber(double n) {
  if (n.isNaN || n.isInfinite) {
    throw FormulaError('#NUM!');
  }
  if (n == n.roundToDouble() && n.abs() < 1e15) {
    return n.round().toString();
  }
  var text = n.toStringAsPrecision(12);
  if (text.contains('e') || text.contains('E')) {
    return text;
  }
  if (text.contains('.')) {
    text = text
        .replaceFirst(RegExp(r'0+$'), '')
        .replaceFirst(RegExp(r'\.$'), '');
  }
  return text;
}

/// What a grid cell shows. Formulas show their result; everything else shows
/// the stored text, so an identifier like `007` is not rewritten as `7`.
String displayCell(EdenWorkbook book, String sheet, int row, int column) {
  final cached = book.cachedDisplay(sheet, row, column);
  if (cached != null) {
    return cached;
  }
  final raw = book.cell(sheet, row, column);
  if (!raw.startsWith('=')) {
    book.cacheDisplay(sheet, row, column, raw, const []);
    return raw;
  }
  final reads = <CellSpan>[];
  try {
    final shown = formatNumber(
      evaluateFormula(book, sheet, raw.substring(1), reads: reads),
    );
    book.cacheDisplay(sheet, row, column, shown, reads);
    return shown;
  } on FormulaError catch (err) {
    book.cacheDisplay(sheet, row, column, err.code, reads);
    return err.code;
  }
}

class _Ref {
  _Ref(this.sheet, this.column, this.row);
  final String sheet;
  final int column;
  final int row;
}

class _Parser {
  _Parser(this.book, this.sheet, this.source, {this.reads});

  final EdenWorkbook book;
  final String sheet;
  final String source;
  final List<CellSpan>? reads;
  int i = 0;
  final List<String> _stack = [];

  bool get done => i >= source.length;

  void skip() {
    while (!done && (source[i] == ' ' || source[i] == '\t')) {
      i++;
    }
  }

  double parseExpression() {
    var value = parseTerm();
    while (true) {
      skip();
      if (done || (source[i] != '+' && source[i] != '-')) {
        break;
      }
      final op = source[i++];
      final right = parseTerm();
      value = op == '+' ? value + right : value - right;
    }
    return value;
  }

  double parseTerm() {
    var value = parseUnary();
    while (true) {
      skip();
      if (done || (source[i] != '*' && source[i] != '/')) {
        break;
      }
      final op = source[i++];
      final right = parseUnary();
      if (op == '/') {
        if (right == 0) {
          throw FormulaError('#DIV/0!');
        }
        value = value / right;
      } else {
        value = value * right;
      }
    }
    return value;
  }

  double parseUnary() {
    skip();
    if (!done && source[i] == '-') {
      i++;
      return -parseUnary();
    }
    return parsePrimary();
  }

  double parsePrimary() {
    skip();
    if (done) {
      throw FormulaError('#ERR');
    }
    final ch = source[i];
    if (ch == '(') {
      i++;
      final value = parseExpression();
      skip();
      if (done || source[i] != ')') {
        throw FormulaError('#ERR');
      }
      i++;
      return value;
    }
    if (_isDigit(ch) || ch == '.') {
      return _number();
    }
    if (_isLetter(ch)) {
      return _ident();
    }
    throw FormulaError('#ERR');
  }

  double _number() {
    final start = i;
    while (!done && (_isDigit(source[i]) || source[i] == '.')) {
      i++;
    }
    final text = source.substring(start, i);
    final value = double.tryParse(text);
    if (value == null || text == '.') {
      throw FormulaError('#NUM!');
    }
    return value;
  }

  double _ident() {
    final start = i;
    while (!done && _isLetter(source[i])) {
      i++;
    }
    final letters = source.substring(start, i).toUpperCase();
    final digitStart = i;
    while (!done && _isDigit(source[i])) {
      i++;
    }
    final digits = source.substring(digitStart, i);
    skip();
    if (!done && source[i] == '!') {
      i++;
      final target = book.canonicalSheetName('$letters$digits');
      if (target == null) {
        throw FormulaError('#REF!');
      }
      final ref = _expectCell(target);
      return _valueOf(ref);
    }
    if (!done && source[i] == '(') {
      if (digits.isNotEmpty) {
        throw FormulaError('#NAME?');
      }
      final args = _args();
      return _call(letters, args);
    }
    if (digits.isEmpty) {
      throw FormulaError('#NAME?');
    }
    return _valueOf(_Ref(sheet, _columnIndex(letters), int.parse(digits) - 1));
  }

  _Ref _expectCell(String sheetName) {
    skip();
    final start = i;
    while (!done && _isLetter(source[i])) {
      i++;
    }
    final letters = source.substring(start, i).toUpperCase();
    final digitStart = i;
    while (!done && _isDigit(source[i])) {
      i++;
    }
    final digits = source.substring(digitStart, i);
    if (letters.isEmpty || digits.isEmpty) {
      throw FormulaError('#REF!');
    }
    if (book.sheet(sheetName) == null) {
      throw FormulaError('#REF!');
    }
    return _Ref(sheetName, _columnIndex(letters), int.parse(digits) - 1);
  }

  List<double> _args() {
    if (done || source[i] != '(') {
      throw FormulaError('#ERR');
    }
    i++;
    skip();
    if (!done && source[i] == ')') {
      i++;
      return const [];
    }
    final values = <double>[];
    while (true) {
      values.addAll(_arg());
      skip();
      if (!done && source[i] == ',') {
        i++;
        continue;
      }
      break;
    }
    skip();
    if (done || source[i] != ')') {
      throw FormulaError('#ERR');
    }
    i++;
    return values;
  }

  List<double> _arg() {
    if (_looksLikeRange()) {
      final start = _readRef(sheet);
      skip();
      i++; // colon
      final end = _readRef(start.sheet);
      return _range(start, end);
    }
    return [parseExpression()];
  }

  bool _looksLikeRange() {
    final mark = i;
    final sheetName = sheet;
    if (_tryReadRef(sheetName) == null) {
      i = mark;
      return false;
    }
    skip();
    final colon = !done && source[i] == ':';
    i = mark;
    return colon;
  }

  _Ref? _tryReadRef(String currentSheet) {
    final mark = i;
    if (done || !_isLetter(source[i])) {
      return null;
    }
    final start = i;
    while (!done && _isLetter(source[i])) {
      i++;
    }
    final letters = source.substring(start, i).toUpperCase();
    final digitStart = i;
    while (!done && _isDigit(source[i])) {
      i++;
    }
    final digits = source.substring(digitStart, i);
    skip();
    if (!done && source[i] == '!') {
      final name = book.canonicalSheetName(
        digits.isEmpty ? letters : '$letters$digits',
      );
      if (name == null) {
        i = mark;
        return null;
      }
      i++;
      return _tryReadRef(name);
    }
    if (letters.isEmpty || digits.isEmpty) {
      i = mark;
      return null;
    }
    return _Ref(currentSheet, _columnIndex(letters), int.parse(digits) - 1);
  }

  _Ref _readRef(String currentSheet) {
    final ref = _tryReadRef(currentSheet);
    if (ref == null) {
      throw FormulaError('#REF!');
    }
    return ref;
  }

  List<double> _range(_Ref start, _Ref end) {
    if (start.sheet != end.sheet) {
      throw FormulaError('#REF!');
    }
    final r0 = start.row < end.row ? start.row : end.row;
    final r1 = start.row > end.row ? start.row : end.row;
    final c0 = start.column < end.column ? start.column : end.column;
    final c1 = start.column > end.column ? start.column : end.column;
    reads?.add(CellSpan(start.sheet, r0, c0, r1, c1));
    final target = book.sheet(start.sheet);
    if (target == null) {
      throw FormulaError('#REF!');
    }
    final values = <double>[];
    target.visitStored(r0, c0, r1, c1, (row, column) {
      values.add(_valueOf(_Ref(start.sheet, column, row), record: false));
    });
    return values;
  }

  double _call(String name, List<double> args) {
    switch (name) {
      case 'SUM':
        return args.fold<double>(0, (a, b) => a + b);
      case 'AVERAGE':
        if (args.isEmpty) {
          throw FormulaError('#DIV/0!');
        }
        return args.fold<double>(0, (a, b) => a + b) / args.length;
      case 'MIN':
        if (args.isEmpty) {
          return 0;
        }
        return args.reduce((a, b) => a < b ? a : b);
      case 'MAX':
        if (args.isEmpty) {
          return 0;
        }
        return args.reduce((a, b) => a > b ? a : b);
      default:
        throw FormulaError('#NAME?');
    }
  }

  double _valueOf(_Ref ref, {bool blankAsZero = false, bool record = true}) {
    if (record) {
      reads?.add(CellSpan.cell(ref.sheet, ref.row, ref.column));
    }
    final key = '${ref.sheet}!${_columnLabel(ref.column)}${ref.row + 1}';
    if (_stack.contains(key)) {
      throw FormulaError('#CYCLE!');
    }
    final raw = book.cell(ref.sheet, ref.row, ref.column);
    if (raw.isEmpty) {
      if (blankAsZero) {
        return 0;
      }
      throw FormulaError('#VALUE!');
    }
    if (raw.startsWith('=')) {
      _stack.add(key);
      try {
        final nested = <CellSpan>[];
        final parser = _Parser(
          book,
          ref.sheet,
          raw.substring(1),
          reads: nested,
        );
        parser._stack.addAll(_stack);
        final value = parser.parseExpression();
        parser.skip();
        if (!parser.done) {
          throw FormulaError('#ERR');
        }
        book.noteReads(ref.sheet, ref.row, ref.column, nested);
        return value;
      } finally {
        _stack.remove(key);
      }
    }
    final number = double.tryParse(raw);
    if (number == null || _isIdentifier(raw)) {
      if (blankAsZero && raw.isEmpty) {
        return 0;
      }
      throw FormulaError('#VALUE!');
    }
    return number;
  }
}

bool _isIdentifier(String raw) {
  if (raw.length <= 1 || raw.contains('.')) {
    return false;
  }
  return raw.startsWith('0') || raw.startsWith('+');
}

int _columnIndex(String letters) {
  var n = 0;
  for (final code in letters.codeUnits) {
    n = n * 26 + (code - 64);
  }
  return n - 1;
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

bool _isLetter(String ch) {
  final c = ch.codeUnitAt(0);
  return (c >= 65 && c <= 90) || (c >= 97 && c <= 122);
}

bool _isDigit(String ch) {
  final c = ch.codeUnitAt(0);
  return c >= 48 && c <= 57;
}
