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

import 'dart:math' as math;

import 'workbook.dart';

/// A formula the grid could not evaluate. [code] is what the cell shows.
class FormulaError implements Exception {
  FormulaError(this.code);
  final String code;

  @override
  String toString() => 'FormulaError($code)';
}

/// Override for TODAY/NOW in tests. Null uses wall-clock UTC.
DateTime? formulaNowOverride;

DateTime _formulaNow() => formulaNowOverride ?? DateTime.now().toUtc();

/// Override for RAND/RANDBETWEEN in tests. Null uses [math.Random].
double? formulaRandOverride;

double _formulaRand() => formulaRandOverride ?? math.Random().nextDouble();

/// Excel serial day 0 is 1899-12-30 (absorbs the 1900 leap-year bug for
/// dates on/after 1900-03-01).
final DateTime _excelEpoch = DateTime.utc(1899, 12, 30);

/// A rectangular A1-style range kept intact so lookup / conditional
/// aggregates can address cells by position. Aggregates flatten it.
class _FormulaRange {
  _FormulaRange(
    this.ctx,
    this.sheet,
    this.row0,
    this.col0,
    this.row1,
    this.col1,
  );
  final _Parser ctx;
  final String sheet;
  final int row0;
  final int col0;
  final int row1;
  final int col1;

  int get rows => row1 - row0 + 1;
  int get cols => col1 - col0 + 1;

  Object valueAt(int row, int col) =>
      ctx._valueOf(_Ref(sheet, col, row), record: false);

  /// Scalarize a 1×1 range; larger ranges are `#VALUE!` in scalar context.
  Object get single {
    if (rows != 1 || cols != 1) throw FormulaError('#VALUE!');
    return valueAt(row0, col0);
  }
}

/// Evaluates [formula] (no leading "=") against [book], starting from [sheet].
///
/// Returns a [num], [String], or [bool]. Throws [FormulaError] on failure.
///
/// Coverage grows under Objective 9 / `conformance/corpus/xlsxformula`. This is
/// still a subset of Excel; unknown functions are `#NAME?`. The stored formula
/// is what an `.xlsx` export asks Excel to calculate.
Object evaluateFormula(
  EdenWorkbook book,
  String sheet,
  String formula, {
  List<CellSpan>? reads,
}) {
  final parser = _Parser(book, sheet, formula, reads: reads);
  final value = parser.parseComparison();
  parser.skip();
  if (!parser.done) {
    throw FormulaError('#ERR');
  }
  return value;
}

/// Formats [n] for a grid cell. Whole numbers stay whole.
String formatNumber(num n) {
  final d = n.toDouble();
  if (d.isNaN || d.isInfinite) {
    throw FormulaError('#NUM!');
  }
  if (d == d.roundToDouble() && d.abs() < 1e15) {
    return d.round().toString();
  }
  var text = d.toStringAsPrecision(12);
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

String formatValue(Object value) {
  if (value is bool) {
    return value ? 'TRUE' : 'FALSE';
  }
  if (value is String) {
    return value;
  }
  if (value is num) {
    return formatNumber(value);
  }
  throw FormulaError('#VALUE!');
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
    final shown = formatValue(
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

typedef _Fn = Object Function(List<Object> args, _Parser ctx);

final Map<String, _Fn> _functions = {
  // Aggregates / math (shipped + Phase A)
  'SUM': (a, c) => _foldNums(_flatten(a, c), 0, (x, y) => x + y),
  'PRODUCT': (a, c) {
    final f = _flatten(a, c);
    return f.isEmpty ? 0 : _foldNums(f, 1, (x, y) => x * y);
  },
  'AVERAGE': (a, c) => _average(_flatten(a, c), c),
  'AVERAGEA': (a, c) => _average(_flatten(a, c), c),
  'MIN': (a, c) => _minFn(_flatten(a, c), c),
  'MAX': (a, c) => _maxFn(_flatten(a, c), c),
  'MINA': (a, c) => _minFn(_flatten(a, c), c),
  'MAXA': (a, c) => _maxFn(_flatten(a, c), c),
  'ABS': (a, _) => _unaryNum(a, (n) => n.abs()),
  'SIGN': (a, _) => _unaryNum(a, (n) => n == 0 ? 0 : (n > 0 ? 1 : -1)),
  'INT': (a, _) => _unaryNum(a, (n) => n.floorToDouble()),
  'TRUNC': (a, _) => _trunc(a),
  'ROUND': (a, _) => _round(a, (n) => n.roundToDouble()),
  'ROUNDUP': (a, _) => _round(a, (n) => n.ceilToDouble()),
  'ROUNDDOWN': (a, _) => _round(a, (n) => n.floorToDouble()),
  'CEILING': (a, _) => _ceilingFloor(a, up: true),
  'FLOOR': (a, _) => _ceilingFloor(a, up: false),
  'MOD': (a, _) => _mod(a),
  'POWER': (a, _) => _power(a),
  'SQRT': (a, _) => _unaryNum(a, (n) {
        if (n < 0) throw FormulaError('#NUM!');
        return math.sqrt(n);
      }),
  'EXP': (a, _) => _unaryNum(a, math.exp),
  'LN': (a, _) => _unaryNum(a, (n) {
        if (n <= 0) throw FormulaError('#NUM!');
        return math.log(n);
      }),
  'LOG': _logFn,
  'LOG10': (a, _) => _unaryNum(a, (n) {
        if (n <= 0) throw FormulaError('#NUM!');
        return math.log(n) / math.ln10;
      }),
  'PI': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    return math.pi;
  },
  'QUOTIENT': (a, _) {
    final nums = _nums(a);
    if (nums.length != 2) throw FormulaError('#ERR');
    if (nums[1] == 0) throw FormulaError('#DIV/0!');
    return (nums[0] / nums[1]).truncateToDouble();
  },
  'GCD': (a, _) => _gcdLcm(a, gcd: true),
  'LCM': (a, _) => _gcdLcm(a, gcd: false),
  'COUNT': (a, c) => _count(_flatten(a, c), numbersOnly: true),
  'COUNTA': (a, c) => _count(_flatten(a, c), numbersOnly: false),
  'COUNTBLANK': (a, c) => _countBlank(a, c),
  'MEDIAN': (a, c) => _median(_flatten(a, c), c),

  // Phase C — math / trig / combinatorics (first tranche)
  'SIN': (a, _) => _unaryNum(a, math.sin),
  'COS': (a, _) => _unaryNum(a, math.cos),
  'TAN': (a, _) => _unaryNum(a, math.tan),
  'ASIN': (a, _) => _unaryNum(a, (n) {
        if (n < -1 || n > 1) throw FormulaError('#NUM!');
        return math.asin(n);
      }),
  'ACOS': (a, _) => _unaryNum(a, (n) {
        if (n < -1 || n > 1) throw FormulaError('#NUM!');
        return math.acos(n);
      }),
  'ATAN': (a, _) => _unaryNum(a, math.atan),
  'ATAN2': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    // Excel ATAN2(x, y) — note argument order vs Dart atan2(y, x).
    return math.atan2(_asNum(a[1]), _asNum(a[0]));
  },
  'SINH': (a, _) => _unaryNum(a, (n) => (math.exp(n) - math.exp(-n)) / 2),
  'COSH': (a, _) => _unaryNum(a, (n) => (math.exp(n) + math.exp(-n)) / 2),
  'TANH': (a, _) => _unaryNum(a, (n) {
        final e2 = math.exp(2 * n);
        return (e2 - 1) / (e2 + 1);
      }),
  'ASINH': (a, _) => _unaryNum(a, (n) => math.log(n + math.sqrt(n * n + 1))),
  'ACOSH': (a, _) => _unaryNum(a, (n) {
        if (n < 1) throw FormulaError('#NUM!');
        return math.log(n + math.sqrt(n * n - 1));
      }),
  'ATANH': (a, _) => _unaryNum(a, (n) {
        if (n <= -1 || n >= 1) throw FormulaError('#NUM!');
        return 0.5 * math.log((1 + n) / (1 - n));
      }),
  'DEGREES': (a, _) => _unaryNum(a, (n) => n * 180 / math.pi),
  'RADIANS': (a, _) => _unaryNum(a, (n) => n * math.pi / 180),
  'EVEN': (a, _) => _unaryNum(a, (n) {
        if (n >= 0) {
          final c = n.ceilToDouble();
          return c % 2 == 0 ? c : c + 1;
        }
        final f = n.floorToDouble();
        return f % 2 == 0 ? f : f - 1;
      }),
  'ODD': (a, _) => _unaryNum(a, (n) {
        if (n >= 0) {
          final c = n.ceilToDouble();
          return c % 2 == 0 ? c + 1 : c;
        }
        final f = n.floorToDouble();
        return f % 2 == 0 ? f - 1 : f;
      }),
  'FACT': (a, _) => _unaryNum(a, (n) {
        if (n < 0) throw FormulaError('#NUM!');
        final k = n.floor();
        if (k > 170) throw FormulaError('#NUM!');
        var acc = 1.0;
        for (var i = 2; i <= k; i++) {
          acc *= i;
        }
        return acc;
      }),
  'COMBIN': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    final n = _asNum(a[0]).floor();
    final k = _asNum(a[1]).floor();
    if (n < 0 || k < 0 || k > n) throw FormulaError('#NUM!');
    return _combin(n, k).toDouble();
  },
  'PERMUT': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    final n = _asNum(a[0]).floor();
    final k = _asNum(a[1]).floor();
    if (n < 0 || k < 0 || k > n) throw FormulaError('#NUM!');
    var acc = 1.0;
    for (var i = 0; i < k; i++) {
      acc *= (n - i);
    }
    return acc;
  },
  'MROUND': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    final n = _asNum(a[0]);
    final mult = _asNum(a[1]);
    if (mult == 0) return 0.0;
    if (n * mult < 0) throw FormulaError('#NUM!');
    return (n / mult).roundToDouble() * mult;
  },
  'SUMSQ': (a, c) {
    var acc = 0.0;
    for (final v in _flatten(a, c)) {
      final n = _asNum(v);
      acc += n * n;
    }
    return acc;
  },
  'SQRTPI': (a, _) => _unaryNum(a, (n) {
        if (n < 0) throw FormulaError('#NUM!');
        return math.sqrt(n * math.pi);
      }),
  'RAND': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    return _formulaRand();
  },
  'RANDBETWEEN': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    final lo = _asNum(a[0]).ceil();
    final hi = _asNum(a[1]).floor();
    if (lo > hi) throw FormulaError('#NUM!');
    if (lo == hi) return lo.toDouble();
    final span = hi - lo + 1;
    return (lo + (_formulaRand() * span).floor()).toDouble();
  },
  'LARGE': (a, c) => _largeSmall(a, c, large: true),
  'SMALL': (a, c) => _largeSmall(a, c, large: false),

  // Phase C — statistical
  'STDEV.S': (a, c) => _stdev(_flatten(a, c), sample: true),
  'STDEV.P': (a, c) => _stdev(_flatten(a, c), sample: false),
  'STDEV': (a, c) => _stdev(_flatten(a, c), sample: true),
  'VAR.S': (a, c) => _variance(_flatten(a, c), sample: true),
  'VAR.P': (a, c) => _variance(_flatten(a, c), sample: false),
  'VAR': (a, c) => _variance(_flatten(a, c), sample: true),

  // Phase C — text
  'EXACT': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    return _asText(a[0]) == _asText(a[1]);
  },
  'CLEAN': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _asText(a[0]).replaceAll(RegExp(r'[\x00-\x1F]'), '');
  },

  // Phase C — financial lite
  'PMT': _pmtFn,
  'PV': _pvFn,
  'FV': _fvFn,
  'SLN': (a, _) {
    if (a.length != 3) throw FormulaError('#ERR');
    final cost = _asNum(a[0]);
    final salvage = _asNum(a[1]);
    final life = _asNum(a[2]);
    if (life == 0) throw FormulaError('#DIV/0!');
    return (cost - salvage) / life;
  },

  // Phase B — date & time
  'DATE': _dateFn,
  'DATEVALUE': _dateValueFn,
  'TIME': _timeFn,
  'TIMEVALUE': _timeValueFn,
  'TODAY': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    return _dateToSerial(_formulaNow());
  },
  'NOW': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    return _dateTimeToSerial(_formulaNow());
  },
  'YEAR': (a, _) => _unaryNum(a, (n) => _serialToDate(n).year.toDouble()),
  'MONTH': (a, _) => _unaryNum(a, (n) => _serialToDate(n).month.toDouble()),
  'DAY': (a, _) => _unaryNum(a, (n) => _serialToDate(n).day.toDouble()),
  'HOUR': (a, _) => _unaryNum(a, (n) => _serialToDateTime(n).hour.toDouble()),
  'MINUTE': (a, _) =>
      _unaryNum(a, (n) => _serialToDateTime(n).minute.toDouble()),
  'SECOND': (a, _) =>
      _unaryNum(a, (n) => _serialToDateTime(n).second.toDouble()),
  'WEEKDAY': _weekdayFn,
  'WEEKNUM': _weeknumFn,
  'DAYS': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    return (_asNum(a[0]) - _asNum(a[1])).truncateToDouble();
  },
  'EDATE': _edateFn,
  'EOMONTH': _eomonthFn,
  'YEARFRAC': _yearfracFn,
  'DAYS360': _days360Fn,
  'NETWORKDAYS': _networkdaysFn,
  'WORKDAY': _workdayFn,

  // Phase B — lookup & reference
  'INDEX': _indexFn,
  'HYPERLINK': (a, _) {
    // Display-only: return friendly name or the URL text; no navigation.
    if (a.isEmpty || a.length > 2) throw FormulaError('#ERR');
    if (a.length == 2) return _asText(a[1]);
    return _asText(a[0]);
  },
  'MATCH': _matchFn,
  'VLOOKUP': _vlookupFn,
  'HLOOKUP': _hlookupFn,
  'XLOOKUP': _xlookupFn,
  'CHOOSE': _chooseFn,
  'COLUMN': _columnFn,
  'COLUMNS': _columnsFn,
  'ROW': _rowFn,
  'ROWS': _rowsFn,
  'ADDRESS': _addressFn,

  // Phase B — conditional aggregates
  'SUMIF': _sumIfFn,
  'SUMIFS': _sumIfsFn,
  'COUNTIF': _countIfFn,
  'COUNTIFS': _countIfsFn,
  'AVERAGEIF': _averageIfFn,
  'AVERAGEIFS': _averageIfsFn,
  'MAXIFS': _maxIfsFn,
  'MINIFS': _minIfsFn,

  // Logical
  'IF': _ifFn,
  'AND': (a, _) => a.every(_truthy),
  'OR': (a, _) => a.any(_truthy),
  'NOT': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return !_truthy(a[0]);
  },
  'TRUE': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    return true;
  },
  'FALSE': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    return false;
  },
  'XOR': (a, _) => a.where(_truthy).length.isOdd,
  'IFERROR': _ifError,
  'IFNA': _ifNa,
  'IFS': _ifs,
  'SWITCH': _switchFn,

  // Text
  'LEFT': (a, _) => _leftRight(a, left: true),
  'RIGHT': (a, _) => _leftRight(a, left: false),
  'MID': _mid,
  'LEN': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _asText(a[0]).length.toDouble();
  },
  'TRIM': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _asText(a[0]).trim().replaceAll(RegExp(r'\s+'), ' ');
  },
  'UPPER': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _asText(a[0]).toUpperCase();
  },
  'LOWER': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _asText(a[0]).toLowerCase();
  },
  'PROPER': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final t = _asText(a[0]).toLowerCase();
    return t.replaceAllMapped(
      RegExp(r'(^|[^a-zA-Z])([a-z])'),
      (m) => '${m[1]}${m[2]!.toUpperCase()}',
    );
  },
  'CONCAT': (a, _) => a.map(_asText).join(),
  'CONCATENATE': (a, _) => a.map(_asText).join(),
  'TEXTJOIN': _textJoin,
  'SUBSTITUTE': _substitute,
  'REPLACE': _replace,
  'FIND': (a, _) => _findSearch(a, caseSensitive: true),
  'SEARCH': (a, _) => _findSearch(a, caseSensitive: false),
  'REPT': (a, _) {
    if (a.length != 2) throw FormulaError('#ERR');
    final n = _asNum(a[1]).floor();
    if (n < 0) throw FormulaError('#VALUE!');
    return _asText(a[0]) * n;
  },
  'VALUE': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final n = double.tryParse(_asText(a[0]).trim());
    if (n == null) throw FormulaError('#VALUE!');
    return n;
  },
  'T': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final v = _unwrap(a[0]);
    return v is String ? v : '';
  },
  'CHAR': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final code = _asNum(a[0]).round();
    if (code < 1 || code > 255) throw FormulaError('#VALUE!');
    return String.fromCharCode(code);
  },
  'CODE': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final t = _asText(a[0]);
    if (t.isEmpty) throw FormulaError('#VALUE!');
    return t.codeUnitAt(0).toDouble();
  },

  // Information
  'ISBLANK': (a, c) => _isBlank(a, c),
  'ISNUMBER': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _unwrap(a[0]) is num;
  },
  'ISTEXT': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _unwrap(a[0]) is String;
  },
  'ISLOGICAL': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _unwrap(a[0]) is bool;
  },
  'ISERR': (a, _) => false, // errors throw before reaching here as values
  'ISERROR': (a, _) => false,
  'ISNA': (a, _) => false,
  'ISNONTEXT': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    return _unwrap(a[0]) is! String;
  },
  'N': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final v = _unwrap(a[0]);
    if (v is num) return v.toDouble();
    if (v is bool) return v ? 1.0 : 0.0;
    return 0.0;
  },
  'TYPE': (a, _) {
    if (a.length != 1) throw FormulaError('#ERR');
    final v = _unwrap(a[0]);
    if (v is num) return 1.0;
    if (v is String) return 2.0;
    if (v is bool) return 4.0;
    return 16.0;
  },
  'NA': (a, _) {
    if (a.isNotEmpty) throw FormulaError('#ERR');
    throw FormulaError('#N/A');
  },
};

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

  // comparison → concat → add → mul → power → unary → primary
  Object parseComparison() {
    var left = parseConcat();
    while (true) {
      skip();
      final op = _matchCompare();
      if (op == null) break;
      final right = parseConcat();
      left = _compare(left, right, op);
    }
    return left;
  }

  String? _matchCompare() {
    if (done) return null;
    if (source.startsWith('<>', i)) {
      i += 2;
      return '<>';
    }
    if (source.startsWith('<=', i)) {
      i += 2;
      return '<=';
    }
    if (source.startsWith('>=', i)) {
      i += 2;
      return '>=';
    }
    final ch = source[i];
    if (ch == '<' || ch == '>' || ch == '=') {
      i++;
      return ch;
    }
    return null;
  }

  Object parseConcat() {
    var value = parseExpression();
    while (true) {
      skip();
      if (done || source[i] != '&') break;
      i++;
      value = _asText(value) + _asText(parseExpression());
    }
    return value;
  }

  Object parseExpression() {
    var value = parseTerm();
    while (true) {
      skip();
      if (done || (source[i] != '+' && source[i] != '-')) break;
      final op = source[i++];
      final right = parseTerm();
      value = op == '+'
          ? _asNum(value) + _asNum(right)
          : _asNum(value) - _asNum(right);
    }
    return value;
  }

  Object parseTerm() {
    var value = parsePower();
    while (true) {
      skip();
      if (done || (source[i] != '*' && source[i] != '/')) break;
      final op = source[i++];
      final right = parsePower();
      if (op == '/') {
        final r = _asNum(right);
        if (r == 0) throw FormulaError('#DIV/0!');
        value = _asNum(value) / r;
      } else {
        value = _asNum(value) * _asNum(right);
      }
    }
    return value;
  }

  Object parsePower() {
    final value = parseUnary();
    skip();
    if (!done && source[i] == '^') {
      i++;
      final exp = parseUnary();
      return math.pow(_asNum(value), _asNum(exp)).toDouble();
    }
    return value;
  }

  Object parseUnary() {
    skip();
    if (!done && source[i] == '-') {
      i++;
      return -_asNum(parseUnary());
    }
    if (!done && source[i] == '+') {
      i++;
      return parseUnary();
    }
    return parsePrimary();
  }

  Object parsePrimary() {
    skip();
    if (done) throw FormulaError('#ERR');
    final ch = source[i];
    if (ch == '(') {
      i++;
      final value = parseComparison();
      skip();
      if (done || source[i] != ')') throw FormulaError('#ERR');
      i++;
      return value;
    }
    if (ch == '"') {
      return _stringLiteral();
    }
    if (_isDigit(ch) || ch == '.') {
      return _number();
    }
    if (ch == "'") {
      return _quotedSheetRef();
    }
    if (_isLetter(ch) || ch == '\$') {
      return _identOrRef();
    }
    throw FormulaError('#ERR');
  }

  String _stringLiteral() {
    i++; // opening "
    final buf = StringBuffer();
    while (!done) {
      final ch = source[i++];
      if (ch == '"') {
        if (!done && source[i] == '"') {
          i++;
          buf.write('"');
          continue;
        }
        return buf.toString();
      }
      buf.write(ch);
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
    if (value == null || text == '.') throw FormulaError('#NUM!');
    return value;
  }

  Object _quotedSheetRef() {
    i++; // '
    final nameBuf = StringBuffer();
    while (!done) {
      final ch = source[i++];
      if (ch == "'") {
        if (!done && source[i] == "'") {
          i++;
          nameBuf.write("'");
          continue;
        }
        break;
      }
      nameBuf.write(ch);
    }
    skip();
    if (done || source[i] != '!') throw FormulaError('#REF!');
    i++;
    final target = book.canonicalSheetName(nameBuf.toString());
    if (target == null) throw FormulaError('#REF!');
    final ref = _expectCell(target);
    return _valueOf(ref);
  }

  Object _identOrRef() {
    // Optional $ then an identifier: function name (LOG10, STDEV.S, ATAN2),
    // sheet token (Sheet1!A1), cell (A1 / $A$1), or TRUE/FALSE.
    final start = i;
    while (!done && source[i] == '\$') {
      i++;
    }
    if (done || !_isLetter(source[i])) {
      i = start;
      throw FormulaError('#ERR');
    }
    final letterStart = i;
    while (!done && _isLetter(source[i])) {
      i++;
    }
    var ident = source.substring(letterStart, i).toUpperCase();

    // Dotted segments for modern Excel names (STDEV.S, NETWORKDAYS.INTL).
    while (!done && source[i] == '.') {
      final mark = i;
      i++;
      if (done || !_isLetter(source[i])) {
        i = mark;
        break;
      }
      final segStart = i;
      while (!done && (_isLetter(source[i]) || _isDigit(source[i]))) {
        i++;
      }
      ident = '$ident.${source.substring(segStart, i).toUpperCase()}';
    }

    // Optional $ before the row number ($A$1).
    while (!done && source[i] == '\$') {
      i++;
    }

    // Trailing digits: LOG10(…) / ATAN2(…) vs A1 vs Sheet1!A1.
    if (!done && _isDigit(source[i])) {
      final digitStart = i;
      while (!done && _isDigit(source[i])) {
        i++;
      }
      final digits = source.substring(digitStart, i);
      final afterDigits = i;
      skip();
      if (!done && source[i] == '(') {
        return _call('$ident$digits');
      }
      if (!done && source[i] == '!') {
        i++;
        final target = book.canonicalSheetName('$ident$digits');
        if (target == null) throw FormulaError('#REF!');
        return _valueOf(_expectCell(target));
      }
      // Cell reference: letters + row digits.
      i = afterDigits;
      return _valueOf(_Ref(sheet, _columnIndex(ident), int.parse(digits) - 1));
    }

    skip();
    if (!done && source[i] == '!') {
      i++;
      final target = book.canonicalSheetName(ident);
      if (target == null) throw FormulaError('#REF!');
      return _valueOf(_expectCell(target));
    }
    if (!done && source[i] == '(') {
      return _call(ident);
    }
    if (ident == 'TRUE') return true;
    if (ident == 'FALSE') return false;
    throw FormulaError('#NAME?');
  }

  Object _call(String name) {
    // Special: IFERROR / IFNA need lazy second arg.
    if (name == 'IFERROR' || name == 'IFNA') {
      return _callLazyCatch(name);
    }
    final args = _args();
    final fn = _functions[name];
    if (fn == null) throw FormulaError('#NAME?');
    return fn(args, this);
  }

  Object _callLazyCatch(String name) {
    if (done || source[i] != '(') throw FormulaError('#ERR');
    i++;
    Object primary;
    try {
      primary = parseComparison();
    } on FormulaError catch (err) {
      skip();
      if (done || source[i] != ',') throw err;
      i++;
      final fallback = parseComparison();
      skip();
      if (done || source[i] != ')') throw FormulaError('#ERR');
      i++;
      if (name == 'IFNA' && err.code != '#N/A') throw err;
      return fallback;
    }
    skip();
    if (!done && source[i] == ',') {
      i++;
      parseComparison(); // consume fallback unused
    }
    skip();
    if (done || source[i] != ')') throw FormulaError('#ERR');
    i++;
    return primary;
  }

  _Ref _expectCell(String sheetName) {
    skip();
    while (!done && source[i] == '\$') {
      i++;
    }
    final start = i;
    while (!done && _isLetter(source[i])) {
      i++;
    }
    final letters = source.substring(start, i).toUpperCase();
    while (!done && source[i] == '\$') {
      i++;
    }
    final digitStart = i;
    while (!done && _isDigit(source[i])) {
      i++;
    }
    final digits = source.substring(digitStart, i);
    if (letters.isEmpty || digits.isEmpty) throw FormulaError('#REF!');
    if (book.sheet(sheetName) == null) throw FormulaError('#REF!');
    return _Ref(sheetName, _columnIndex(letters), int.parse(digits) - 1);
  }

  List<Object> _args() {
    if (done || source[i] != '(') throw FormulaError('#ERR');
    i++;
    skip();
    if (!done && source[i] == ')') {
      i++;
      return const [];
    }
    final values = <Object>[];
    while (true) {
      values.add(_arg());
      skip();
      if (!done && source[i] == ',') {
        i++;
        continue;
      }
      break;
    }
    skip();
    if (done || source[i] != ')') throw FormulaError('#ERR');
    i++;
    return values;
  }

  /// One function argument. Multi-cell (and bare cell) refs stay as
  /// [_FormulaRange] so lookup / COLUMN / SUMIF can see positions; aggregates
  /// flatten via [_flatten]. Expressions like `A1+1` still evaluate.
  Object _arg() {
    if (_looksLikeRange()) {
      final start = _readRef(sheet);
      skip();
      i++; // colon
      final end = _readRef(start.sheet);
      return _makeRange(start, end);
    }
    final mark = i;
    final ref = _tryReadRef(sheet);
    if (ref != null) {
      skip();
      if (done || source[i] == ',' || source[i] == ')') {
        reads?.add(CellSpan.cell(ref.sheet, ref.row, ref.column));
        return _FormulaRange(
          this,
          ref.sheet,
          ref.row,
          ref.column,
          ref.row,
          ref.column,
        );
      }
      i = mark;
    }
    return parseComparison();
  }

  bool _looksLikeRange() {
    final mark = i;
    if (_tryReadRef(sheet) == null) {
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
    while (!done && source[i] == '\$') {
      i++;
    }
    if (done || !_isLetter(source[i])) {
      i = mark;
      return null;
    }
    final start = i;
    while (!done && _isLetter(source[i])) {
      i++;
    }
    final letters = source.substring(start, i).toUpperCase();
    while (!done && source[i] == '\$') {
      i++;
    }
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
    if (ref == null) throw FormulaError('#REF!');
    return ref;
  }

  _FormulaRange _makeRange(_Ref start, _Ref end) {
    if (start.sheet != end.sheet) throw FormulaError('#REF!');
    final r0 = start.row < end.row ? start.row : end.row;
    final r1 = start.row > end.row ? start.row : end.row;
    final c0 = start.column < end.column ? start.column : end.column;
    final c1 = start.column > end.column ? start.column : end.column;
    reads?.add(CellSpan(start.sheet, r0, c0, r1, c1));
    if (book.sheet(start.sheet) == null) throw FormulaError('#REF!');
    return _FormulaRange(this, start.sheet, r0, c0, r1, c1);
  }

  Object _valueOf(_Ref ref, {bool record = true}) {
    if (record) {
      reads?.add(CellSpan.cell(ref.sheet, ref.row, ref.column));
    }
    final key = '${ref.sheet}!${_columnLabel(ref.column)}${ref.row + 1}';
    if (_stack.contains(key)) throw FormulaError('#CYCLE!');
    final raw = book.cell(ref.sheet, ref.row, ref.column);
    // Blank cells are empty strings so ISBLANK works; arithmetic still fails
    // via _asNum('') → #VALUE! (and SUM ranges skip blanks via visitStored).
    if (raw.isEmpty) {
      return '';
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
        final value = parser.parseComparison();
        parser.skip();
        if (!parser.done) throw FormulaError('#ERR');
        book.noteReads(ref.sheet, ref.row, ref.column, nested);
        return value;
      } finally {
        _stack.remove(key);
      }
    }
    final number = double.tryParse(raw);
    if (number == null || _isIdentifier(raw)) {
      // Boolean literals stored as text
      final upper = raw.toUpperCase();
      if (upper == 'TRUE') return true;
      if (upper == 'FALSE') return false;
      return raw; // text cell
    }
    return number;
  }
}

// --- function helpers ---

Object _unwrap(Object v) => v is _FormulaRange ? v.single : v;

bool _truthy(Object v) {
  final x = _unwrap(v);
  if (x is bool) return x;
  if (x is num) return x != 0;
  if (x is String) return x.isNotEmpty;
  return false;
}

double _asNum(Object v) {
  final x = _unwrap(v);
  if (x is num) return x.toDouble();
  if (x is bool) return x ? 1 : 0;
  if (x is String) {
    final t = x.trim();
    // xlsxtyping D6: authored identifiers like "007" / "+3" must not become
    // numbers when a formula coerces text.
    if (_isIdentifier(t)) throw FormulaError('#VALUE!');
    final n = double.tryParse(t);
    if (n != null) return n;
  }
  throw FormulaError('#VALUE!');
}

String _asText(Object v) {
  final x = _unwrap(v);
  if (x is String) return x;
  if (x is bool) return x ? 'TRUE' : 'FALSE';
  if (x is num) return formatNumber(x);
  throw FormulaError('#VALUE!');
}

/// Expand [_FormulaRange] args via sparse [EdenSheet.visitStored] so blanks
/// stay omitted for SUM/COUNT/PRODUCT — same contract as pre-Phase-B ranges.
List<Object> _flatten(List<Object> args, _Parser ctx) {
  final out = <Object>[];
  for (final a in args) {
    if (a is _FormulaRange) {
      final target = ctx.book.sheet(a.sheet);
      if (target == null) throw FormulaError('#REF!');
      target.visitStored(a.row0, a.col0, a.row1, a.col1, (row, column) {
        out.add(ctx._valueOf(_Ref(a.sheet, column, row), record: false));
      });
    } else {
      out.add(a);
    }
  }
  return out;
}

_FormulaRange _requireRange(Object a) {
  if (a is _FormulaRange) return a;
  throw FormulaError('#VALUE!');
}

double _dateToSerial(DateTime d) {
  final day = DateTime.utc(d.year, d.month, d.day);
  return day.difference(_excelEpoch).inDays.toDouble();
}

double _dateTimeToSerial(DateTime d) {
  final us = d.toUtc().difference(_excelEpoch).inMicroseconds;
  return us / Duration.microsecondsPerDay;
}

DateTime _serialToDate(num n) {
  final days = n.floor();
  return _excelEpoch.add(Duration(days: days));
}

DateTime _serialToDateTime(num n) {
  final whole = n.floor();
  final frac = n.toDouble() - whole;
  final base = _excelEpoch.add(Duration(days: whole));
  final us = (frac * Duration.microsecondsPerDay).round();
  return base.add(Duration(microseconds: us));
}

Object _dateFn(List<Object> args, _Parser _) {
  if (args.length != 3) throw FormulaError('#ERR');
  var y = _asNum(args[0]).round();
  var m = _asNum(args[1]).round();
  var d = _asNum(args[2]).round();
  // Excel: years 0–1899 map to 1900–3799.
  if (y >= 0 && y <= 1899) y += 1900;
  // Month/day overflow — DateTime constructor rejects; normalize manually.
  while (m > 12) {
    m -= 12;
    y++;
  }
  while (m < 1) {
    m += 12;
    y--;
  }
  final first = DateTime.utc(y, m, 1);
  final dt = first.add(Duration(days: d - 1));
  return _dateToSerial(dt);
}

Object _dateValueFn(List<Object> args, _Parser _) {
  if (args.length != 1) throw FormulaError('#ERR');
  final t = _asText(args[0]).trim();
  final parsed = DateTime.tryParse(t) ?? _parseLooseDate(t);
  if (parsed == null) throw FormulaError('#VALUE!');
  return _dateToSerial(parsed.toUtc());
}

Object _timeValueFn(List<Object> args, _Parser _) {
  if (args.length != 1) throw FormulaError('#ERR');
  final t = _asText(args[0]).trim();
  final m = RegExp(
    r'^(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([AaPp][Mm])?$',
  ).firstMatch(t);
  if (m == null) throw FormulaError('#VALUE!');
  var h = int.parse(m.group(1)!);
  final min = int.parse(m.group(2)!);
  final sec = int.parse(m.group(3) ?? '0');
  final ampm = m.group(4)?.toUpperCase();
  if (ampm != null) {
    if (h < 1 || h > 12) throw FormulaError('#VALUE!');
    if (ampm == 'PM' && h < 12) h += 12;
    if (ampm == 'AM' && h == 12) h = 0;
  }
  final frac = (h * 3600 + min * 60 + sec) / 86400.0;
  return frac - frac.floorToDouble();
}

int _combin(int n, int k) {
  if (k > n - k) k = n - k;
  var result = 1;
  for (var i = 1; i <= k; i++) {
    result = result * (n - k + i) ~/ i;
  }
  return result;
}

Object _largeSmall(List<Object> args, _Parser ctx, {required bool large}) {
  if (args.length < 2) throw FormulaError('#ERR');
  final k = _asNum(args.last).floor();
  final values = _flatten(args.sublist(0, args.length - 1), ctx);
  final nums = <double>[];
  for (final v in values) {
    if (v is num) nums.add(v.toDouble());
  }
  if (k < 1 || k > nums.length) throw FormulaError('#NUM!');
  nums.sort();
  return large ? nums[nums.length - k] : nums[k - 1];
}

Object _stdev(List<Object> args, {required bool sample}) {
  final nums = <double>[];
  for (final a in args) {
    if (a is num) nums.add(a.toDouble());
  }
  if (sample && nums.length < 2) throw FormulaError('#DIV/0!');
  if (!sample && nums.isEmpty) throw FormulaError('#DIV/0!');
  final mean = nums.reduce((a, b) => a + b) / nums.length;
  var acc = 0.0;
  for (final n in nums) {
    final d = n - mean;
    acc += d * d;
  }
  final denom = sample ? nums.length - 1 : nums.length;
  return math.sqrt(acc / denom);
}

Object _variance(List<Object> args, {required bool sample}) {
  final nums = <double>[];
  for (final a in args) {
    if (a is num) nums.add(a.toDouble());
  }
  if (sample && nums.length < 2) throw FormulaError('#DIV/0!');
  if (!sample && nums.isEmpty) throw FormulaError('#DIV/0!');
  final mean = nums.reduce((a, b) => a + b) / nums.length;
  var acc = 0.0;
  for (final n in nums) {
    final d = n - mean;
    acc += d * d;
  }
  final denom = sample ? nums.length - 1 : nums.length;
  return acc / denom;
}

Object _pmtFn(List<Object> args, _Parser _) {
  // PMT(rate, nper, pv, [fv], [type])
  if (args.length < 3 || args.length > 5) throw FormulaError('#ERR');
  final rate = _asNum(args[0]);
  final nper = _asNum(args[1]);
  final pv = _asNum(args[2]);
  final fv = args.length >= 4 ? _asNum(args[3]) : 0.0;
  final type = args.length == 5 ? _asNum(args[4]).round() : 0;
  if (rate == 0) return -(pv + fv) / nper;
  final pow = math.pow(1 + rate, nper).toDouble();
  var pmt = (-pv * pow - fv) * rate / ((1 + rate * type) * (pow - 1));
  return pmt;
}

Object _pvFn(List<Object> args, _Parser _) {
  // PV(rate, nper, pmt, [fv], [type])
  if (args.length < 3 || args.length > 5) throw FormulaError('#ERR');
  final rate = _asNum(args[0]);
  final nper = _asNum(args[1]);
  final pmt = _asNum(args[2]);
  final fv = args.length >= 4 ? _asNum(args[3]) : 0.0;
  final type = args.length == 5 ? _asNum(args[4]).round() : 0;
  if (rate == 0) return -pmt * nper - fv;
  final pow = math.pow(1 + rate, nper).toDouble();
  return -(pmt * (1 + rate * type) * (pow - 1) / rate + fv) / pow;
}

Object _fvFn(List<Object> args, _Parser _) {
  // FV(rate, nper, pmt, [pv], [type])
  if (args.length < 3 || args.length > 5) throw FormulaError('#ERR');
  final rate = _asNum(args[0]);
  final nper = _asNum(args[1]);
  final pmt = _asNum(args[2]);
  final pv = args.length >= 4 ? _asNum(args[3]) : 0.0;
  final type = args.length == 5 ? _asNum(args[4]).round() : 0;
  if (rate == 0) return -pv - pmt * nper;
  final pow = math.pow(1 + rate, nper).toDouble();
  return -pv * pow - pmt * (1 + rate * type) * (pow - 1) / rate;
}

Object _days360Fn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  final s = _serialToDate(_asNum(args[0]));
  final e = _serialToDate(_asNum(args[1]));
  final european = args.length == 3 && _truthy(args[2]);
  var d1 = s.day;
  var d2 = e.day;
  final m1 = s.month;
  final m2 = e.month;
  final y1 = s.year;
  final y2 = e.year;
  if (european) {
    if (d1 == 31) d1 = 30;
    if (d2 == 31) d2 = 30;
  } else {
    // US NASD method (simplified)
    if (d1 == 31) d1 = 30;
    if (d2 == 31 && d1 >= 30) d2 = 30;
  }
  return ((y2 - y1) * 360 + (m2 - m1) * 30 + (d2 - d1)).toDouble();
}

bool _isWeekend(DateTime d) =>
    d.weekday == DateTime.saturday || d.weekday == DateTime.sunday;

Object _networkdaysFn(List<Object> args, _Parser ctx) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  var start = _asNum(args[0]).floor();
  var end = _asNum(args[1]).floor();
  var sign = 1;
  if (start > end) {
    final t = start;
    start = end;
    end = t;
    sign = -1;
  }
  final holidays = <int>{};
  if (args.length == 3 && args[2] is _FormulaRange) {
    for (final v in _flatten([args[2]], ctx)) {
      if (v is num) holidays.add(v.floor());
    }
  }
  var n = 0;
  for (var s = start; s <= end; s++) {
    final d = _serialToDate(s);
    if (_isWeekend(d) || holidays.contains(s)) continue;
    n++;
  }
  return (sign * n).toDouble();
}

Object _workdayFn(List<Object> args, _Parser ctx) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  var serial = _asNum(args[0]).floor();
  var days = _asNum(args[1]).round();
  final holidays = <int>{};
  if (args.length == 3 && args[2] is _FormulaRange) {
    for (final v in _flatten([args[2]], ctx)) {
      if (v is num) holidays.add(v.floor());
    }
  }
  final step = days >= 0 ? 1 : -1;
  days = days.abs();
  while (days > 0) {
    serial += step;
    final d = _serialToDate(serial);
    if (_isWeekend(d) || holidays.contains(serial)) continue;
    days--;
  }
  return serial.toDouble();
}

DateTime? _parseLooseDate(String t) {
  final m = RegExp(r'^(\d{1,2})[/-](\d{1,2})[/-](\d{2,4})$').firstMatch(t);
  if (m == null) return null;
  var a = int.parse(m.group(1)!);
  var b = int.parse(m.group(2)!);
  var y = int.parse(m.group(3)!);
  if (y < 100) y += 2000;
  // Prefer M/D/Y when a>12 would be invalid as month elsewhere; if a<=12 use MDY.
  if (a > 12) {
    // D/M/Y
    return DateTime.utc(y, b, a);
  }
  return DateTime.utc(y, a, b);
}

Object _timeFn(List<Object> args, _Parser _) {
  if (args.length != 3) throw FormulaError('#ERR');
  final h = _asNum(args[0]);
  final m = _asNum(args[1]);
  final s = _asNum(args[2]);
  final frac = (h * 3600 + m * 60 + s) / 86400.0;
  return frac - frac.floorToDouble();
}

Object _weekdayFn(List<Object> args, _Parser _) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final dt = _serialToDate(_asNum(args[0]));
  final returnType = args.length == 2 ? _asNum(args[1]).round() : 1;
  // Dart: Mon=1..Sun=7. Excel type 1: Sun=1..Sat=7; type 2: Mon=1..Sun=7.
  final iso = dt.weekday; // Mon=1
  switch (returnType) {
    case 1:
      return (iso % 7 + 1).toDouble(); // Sun=1
    case 2:
    case 11:
      return iso.toDouble();
    case 3:
      return ((iso + 6) % 7).toDouble(); // Mon=0
    default:
      throw FormulaError('#NUM!');
  }
}

Object _weeknumFn(List<Object> args, _Parser _) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final dt = _serialToDate(_asNum(args[0]));
  final returnType = args.length == 2 ? _asNum(args[1]).round() : 1;
  // Simplified: week 1 contains Jan 1; weeks start Sun (1) or Mon (2/11).
  final start = DateTime.utc(dt.year, 1, 1);
  final dayOfYear = dt.difference(start).inDays;
  final weekStartOffset = returnType == 2 || returnType == 11
      ? (start.weekday - 1)
      : (start.weekday % 7); // Sun-based: Sun=0
  return ((dayOfYear + weekStartOffset) ~/ 7 + 1).toDouble();
}

(int, int) _addMonths(int year, int month, int months) {
  final total = month - 1 + months;
  final y = year + (total / 12).floor();
  final m = total - (total / 12).floor() * 12 + 1;
  return (y, m);
}

Object _edateFn(List<Object> args, _Parser _) {
  if (args.length != 2) throw FormulaError('#ERR');
  final dt = _serialToDate(_asNum(args[0]));
  final months = _asNum(args[1]).round();
  final (y, m) = _addMonths(dt.year, dt.month, months);
  final lastDay = DateTime.utc(y, m + 1, 0).day;
  final d = dt.day > lastDay ? lastDay : dt.day;
  return _dateToSerial(DateTime.utc(y, m, d));
}

Object _eomonthFn(List<Object> args, _Parser _) {
  if (args.length != 2) throw FormulaError('#ERR');
  final dt = _serialToDate(_asNum(args[0]));
  final months = _asNum(args[1]).round();
  final (y, m) = _addMonths(dt.year, dt.month, months);
  final last = DateTime.utc(y, m + 1, 0);
  return _dateToSerial(last);
}

Object _yearfracFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  final start = _asNum(args[0]);
  final end = _asNum(args[1]);
  final basis = args.length == 3 ? _asNum(args[2]).round() : 0;
  final days = (end - start).abs();
  switch (basis) {
    case 0: // 30/360 US — simplified
    case 4: // 30/360 European — simplified same
      return days / 360.0;
    case 1: // actual/actual — use 365.25 average
      return days / 365.25;
    case 2:
      return days / 360.0;
    case 3:
      return days / 365.0;
    default:
      throw FormulaError('#NUM!');
  }
}

Object _indexFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  final range = _requireRange(args[0]);
  final rowNum = _asNum(args[1]).round();
  final colNum = args.length == 3 ? _asNum(args[2]).round() : 1;
  if (rowNum < 1 || colNum < 1) throw FormulaError('#VALUE!');
  if (rowNum > range.rows || colNum > range.cols) throw FormulaError('#REF!');
  return range.valueAt(range.row0 + rowNum - 1, range.col0 + colNum - 1);
}

Object _matchFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  final lookup = _unwrap(args[0]);
  final range = _requireRange(args[1]);
  final matchType = args.length == 3 ? _asNum(args[2]).round() : 1;
  if (range.rows > 1 && range.cols > 1) throw FormulaError('#N/A');
  final n = range.rows * range.cols;
  Object at(int i) {
    if (range.cols == 1) {
      return range.valueAt(range.row0 + i, range.col0);
    }
    return range.valueAt(range.row0, range.col0 + i);
  }

  if (matchType == 0) {
    for (var i = 0; i < n; i++) {
      if (_compare(at(i), lookup, '=') == true) return (i + 1).toDouble();
    }
    throw FormulaError('#N/A');
  }
  if (matchType == 1) {
    if (lookup is! num) throw FormulaError('#N/A');
    final needle = lookup;
    var best = -1;
    for (var i = 0; i < n; i++) {
      final v = at(i);
      if (v is! num) continue;
      if (v <= needle) {
        best = i;
      } else {
        break;
      }
    }
    if (best < 0) throw FormulaError('#N/A');
    return (best + 1).toDouble();
  }
  if (matchType == -1) {
    if (lookup is! num) throw FormulaError('#N/A');
    final needle = lookup;
    var best = -1;
    for (var i = 0; i < n; i++) {
      final v = at(i);
      if (v is! num) continue;
      if (v >= needle) {
        best = i;
      } else {
        break;
      }
    }
    if (best < 0) throw FormulaError('#N/A');
    return (best + 1).toDouble();
  }
  throw FormulaError('#NUM!');
}

Object _vlookupFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length > 4) throw FormulaError('#ERR');
  final lookup = _unwrap(args[0]);
  final table = _requireRange(args[1]);
  final col = _asNum(args[2]).round();
  final approx = args.length < 4 || _truthy(args[3]);
  if (col < 1 || col > table.cols) throw FormulaError('#REF!');
  if (!approx) {
    for (var r = 0; r < table.rows; r++) {
      final key = table.valueAt(table.row0 + r, table.col0);
      if (_compare(key, lookup, '=') == true) {
        return table.valueAt(table.row0 + r, table.col0 + col - 1);
      }
    }
    throw FormulaError('#N/A');
  }
  // Approximate: last key <= lookup in first column (numeric).
  if (lookup is! num) throw FormulaError('#N/A');
  final needle = lookup;
  var best = -1;
  for (var r = 0; r < table.rows; r++) {
    final key = table.valueAt(table.row0 + r, table.col0);
    if (key is num && key <= needle) {
      best = r;
    } else if (best >= 0 && key is num) {
      break;
    }
  }
  if (best < 0) throw FormulaError('#N/A');
  return table.valueAt(table.row0 + best, table.col0 + col - 1);
}

Object _hlookupFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length > 4) throw FormulaError('#ERR');
  final lookup = _unwrap(args[0]);
  final table = _requireRange(args[1]);
  final row = _asNum(args[2]).round();
  final exact = args.length == 4 && !_truthy(args[3]);
  if (row < 1 || row > table.rows) throw FormulaError('#REF!');
  for (var c = 0; c < table.cols; c++) {
    final key = table.valueAt(table.row0, table.col0 + c);
    if (_compare(key, lookup, '=') == true) {
      return table.valueAt(table.row0 + row - 1, table.col0 + c);
    }
  }
  if (exact) throw FormulaError('#N/A');
  throw FormulaError('#N/A');
}

Object _xlookupFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length > 4) throw FormulaError('#ERR');
  final lookup = _unwrap(args[0]);
  final lookupRange = _requireRange(args[1]);
  final returnRange = _requireRange(args[2]);
  final n = lookupRange.rows * lookupRange.cols;
  final m = returnRange.rows * returnRange.cols;
  if (n != m) throw FormulaError('#VALUE!');
  Object lup(int i) => lookupRange.cols == 1
      ? lookupRange.valueAt(lookupRange.row0 + i, lookupRange.col0)
      : lookupRange.valueAt(lookupRange.row0, lookupRange.col0 + i);
  Object ret(int i) => returnRange.cols == 1
      ? returnRange.valueAt(returnRange.row0 + i, returnRange.col0)
      : returnRange.valueAt(returnRange.row0, returnRange.col0 + i);
  for (var i = 0; i < n; i++) {
    if (_compare(lup(i), lookup, '=') == true) return ret(i);
  }
  if (args.length == 4) return _unwrap(args[3]);
  throw FormulaError('#N/A');
}

Object _chooseFn(List<Object> args, _Parser _) {
  if (args.length < 2) throw FormulaError('#ERR');
  final idx = _asNum(args[0]).round();
  if (idx < 1 || idx >= args.length) throw FormulaError('#VALUE!');
  return _unwrap(args[idx]);
}

Object _columnFn(List<Object> args, _Parser _) {
  if (args.isEmpty) throw FormulaError('#ERR');
  return (_requireRange(args[0]).col0 + 1).toDouble();
}

Object _columnsFn(List<Object> args, _Parser _) {
  if (args.length != 1) throw FormulaError('#ERR');
  return _requireRange(args[0]).cols.toDouble();
}

Object _rowFn(List<Object> args, _Parser _) {
  if (args.isEmpty) throw FormulaError('#ERR');
  return (_requireRange(args[0]).row0 + 1).toDouble();
}

Object _rowsFn(List<Object> args, _Parser _) {
  if (args.length != 1) throw FormulaError('#ERR');
  return _requireRange(args[0]).rows.toDouble();
}

Object _addressFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 5) throw FormulaError('#ERR');
  final row = _asNum(args[0]).round();
  final col = _asNum(args[1]).round();
  if (row < 1 || col < 1) throw FormulaError('#VALUE!');
  final absNum = args.length >= 3 ? _asNum(args[2]).round() : 1;
  final a1 = args.length < 4 || _truthy(args[3]);
  if (!a1) throw FormulaError('#VALUE!'); // R1C1 not supported
  final colLabel = _columnLabel(col - 1);
  late final String cell;
  switch (absNum) {
    case 1:
      cell = '\$$colLabel\$$row';
      break;
    case 2:
      cell = '$colLabel\$$row';
      break;
    case 3:
      cell = '\$$colLabel$row';
      break;
    case 4:
      cell = '$colLabel$row';
      break;
    default:
      throw FormulaError('#VALUE!');
  }
  if (args.length < 5) return cell;
  final sheetName = _asText(args[4]);
  if (sheetName.contains(' ') || sheetName.contains("'")) {
    return "'$sheetName'!$cell";
  }
  return '$sheetName!$cell';
}

bool _criteriaMatch(Object value, Object criteria) {
  final crit = _unwrap(criteria);
  if (crit is num || crit is bool) {
    return _compare(_unwrap(value), crit, '=') == true;
  }
  final c = _asText(crit);
  for (final op in ['>=', '<=', '<>', '>', '<', '=']) {
    if (c.startsWith(op)) {
      final rest = c.substring(op.length);
      final n = double.tryParse(rest);
      if (n != null) {
        final v = _unwrap(value);
        if (v is! num) return false;
        return _compare(v, n, op) == true;
      }
      return _compare(_asText(_unwrap(value)), rest, op) == true;
    }
  }
  final v = _unwrap(value);
  if (v is num) {
    final n = double.tryParse(c);
    if (n != null) return v == n;
  }
  if (c.contains('*') || c.contains('?')) {
    final pattern = RegExp(
      '^' +
          RegExp.escape(c)
              .replaceAll(r'\*', '.*')
              .replaceAll(r'\?', '.') +
          r'$',
      caseSensitive: false,
    );
    return pattern.hasMatch(_asText(v));
  }
  return _asText(v).toLowerCase() == c.toLowerCase();
}

void _forEachAligned(
  _FormulaRange criteriaRange,
  _FormulaRange? valuesRange,
  void Function(Object criteriaValue, Object? sumValue) visit,
) {
  final rows = criteriaRange.rows;
  final cols = criteriaRange.cols;
  if (valuesRange != null &&
      (valuesRange.rows < rows || valuesRange.cols < cols)) {
    // Excel requires same shape; tolerate equal dims only.
  }
  for (var r = 0; r < rows; r++) {
    for (var c = 0; c < cols; c++) {
      final cv = criteriaRange.valueAt(
        criteriaRange.row0 + r,
        criteriaRange.col0 + c,
      );
      Object? sv;
      if (valuesRange != null) {
        sv = valuesRange.valueAt(valuesRange.row0 + r, valuesRange.col0 + c);
      }
      visit(cv, sv);
    }
  }
}

Object _sumIfFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  final range = _requireRange(args[0]);
  final criteria = args[1];
  final sumRange =
      args.length == 3 ? _requireRange(args[2]) : range;
  var acc = 0.0;
  _forEachAligned(range, sumRange, (cv, sv) {
    if (_criteriaMatch(cv, criteria)) {
      final x = sv ?? cv;
      if (x is num) acc += x.toDouble();
      // blanks / text ignored like Excel SUMIF
    }
  });
  return acc;
}

Object _countIfFn(List<Object> args, _Parser _) {
  if (args.length != 2) throw FormulaError('#ERR');
  final range = _requireRange(args[0]);
  final criteria = args[1];
  var n = 0;
  _forEachAligned(range, null, (cv, _) {
    if (_criteriaMatch(cv, criteria)) n++;
  });
  return n.toDouble();
}

Object _averageIfFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  final range = _requireRange(args[0]);
  final criteria = args[1];
  final avgRange =
      args.length == 3 ? _requireRange(args[2]) : range;
  var acc = 0.0;
  var n = 0;
  _forEachAligned(range, avgRange, (cv, sv) {
    if (_criteriaMatch(cv, criteria)) {
      final x = sv ?? cv;
      if (x is num) {
        acc += x.toDouble();
        n++;
      }
    }
  });
  if (n == 0) throw FormulaError('#DIV/0!');
  return acc / n;
}

bool _ifsMatch(
  int r,
  int c,
  List<_FormulaRange> critRanges,
  List<Object> criteria,
) {
  for (var i = 0; i < critRanges.length; i++) {
    final cr = critRanges[i];
    final cv = cr.valueAt(cr.row0 + r, cr.col0 + c);
    if (!_criteriaMatch(cv, criteria[i])) return false;
  }
  return true;
}

Object _sumIfsFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length.isEven) throw FormulaError('#ERR');
  final sumRange = _requireRange(args[0]);
  final critRanges = <_FormulaRange>[];
  final criteria = <Object>[];
  for (var i = 1; i < args.length; i += 2) {
    critRanges.add(_requireRange(args[i]));
    criteria.add(args[i + 1]);
  }
  final rows = critRanges.first.rows;
  final cols = critRanges.first.cols;
  var acc = 0.0;
  for (var r = 0; r < rows; r++) {
    for (var c = 0; c < cols; c++) {
      if (!_ifsMatch(r, c, critRanges, criteria)) continue;
      final v = sumRange.valueAt(sumRange.row0 + r, sumRange.col0 + c);
      if (v is num) acc += v.toDouble();
    }
  }
  return acc;
}

Object _countIfsFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length.isOdd) throw FormulaError('#ERR');
  final critRanges = <_FormulaRange>[];
  final criteria = <Object>[];
  for (var i = 0; i < args.length; i += 2) {
    critRanges.add(_requireRange(args[i]));
    criteria.add(args[i + 1]);
  }
  final rows = critRanges.first.rows;
  final cols = critRanges.first.cols;
  var n = 0;
  for (var r = 0; r < rows; r++) {
    for (var c = 0; c < cols; c++) {
      if (_ifsMatch(r, c, critRanges, criteria)) n++;
    }
  }
  return n.toDouble();
}

Object _averageIfsFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length.isEven) throw FormulaError('#ERR');
  final avgRange = _requireRange(args[0]);
  final critRanges = <_FormulaRange>[];
  final criteria = <Object>[];
  for (var i = 1; i < args.length; i += 2) {
    critRanges.add(_requireRange(args[i]));
    criteria.add(args[i + 1]);
  }
  final rows = critRanges.first.rows;
  final cols = critRanges.first.cols;
  var acc = 0.0;
  var n = 0;
  for (var r = 0; r < rows; r++) {
    for (var c = 0; c < cols; c++) {
      if (!_ifsMatch(r, c, critRanges, criteria)) continue;
      final v = avgRange.valueAt(avgRange.row0 + r, avgRange.col0 + c);
      if (v is num) {
        acc += v.toDouble();
        n++;
      }
    }
  }
  if (n == 0) throw FormulaError('#DIV/0!');
  return acc / n;
}

Object _maxIfsFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length.isEven) throw FormulaError('#ERR');
  final maxRange = _requireRange(args[0]);
  final critRanges = <_FormulaRange>[];
  final criteria = <Object>[];
  for (var i = 1; i < args.length; i += 2) {
    critRanges.add(_requireRange(args[i]));
    criteria.add(args[i + 1]);
  }
  final rows = critRanges.first.rows;
  final cols = critRanges.first.cols;
  double? best;
  for (var r = 0; r < rows; r++) {
    for (var c = 0; c < cols; c++) {
      if (!_ifsMatch(r, c, critRanges, criteria)) continue;
      final v = maxRange.valueAt(maxRange.row0 + r, maxRange.col0 + c);
      if (v is num) {
        best = best == null ? v.toDouble() : math.max(best, v.toDouble());
      }
    }
  }
  if (best == null) throw FormulaError('#DIV/0!');
  return best;
}

Object _minIfsFn(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length.isEven) throw FormulaError('#ERR');
  final minRange = _requireRange(args[0]);
  final critRanges = <_FormulaRange>[];
  final criteria = <Object>[];
  for (var i = 1; i < args.length; i += 2) {
    critRanges.add(_requireRange(args[i]));
    criteria.add(args[i + 1]);
  }
  final rows = critRanges.first.rows;
  final cols = critRanges.first.cols;
  double? best;
  for (var r = 0; r < rows; r++) {
    for (var c = 0; c < cols; c++) {
      if (!_ifsMatch(r, c, critRanges, criteria)) continue;
      final v = minRange.valueAt(minRange.row0 + r, minRange.col0 + c);
      if (v is num) {
        best = best == null ? v.toDouble() : math.min(best, v.toDouble());
      }
    }
  }
  if (best == null) throw FormulaError('#DIV/0!');
  return best;
}

List<double> _nums(List<Object> args) => args.map(_asNum).toList();

double _foldNums(
  List<Object> args,
  double seed,
  double Function(double, double) op,
) {
  var acc = seed;
  for (final a in args) {
    acc = op(acc, _asNum(a));
  }
  return acc;
}

Object _average(List<Object> args, _Parser _) {
  if (args.isEmpty) throw FormulaError('#DIV/0!');
  return _foldNums(args, 0, (a, b) => a + b) / args.length;
}

Object _minFn(List<Object> args, _Parser _) {
  if (args.isEmpty) return 0;
  return args.map(_asNum).reduce(math.min);
}

Object _maxFn(List<Object> args, _Parser _) {
  if (args.isEmpty) return 0;
  return args.map(_asNum).reduce(math.max);
}

Object _unaryNum(List<Object> args, double Function(double) fn) {
  if (args.length != 1) throw FormulaError('#ERR');
  return fn(_asNum(args[0]));
}

Object _trunc(List<Object> args) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final n = _asNum(args[0]);
  final digits = args.length == 2 ? _asNum(args[1]).round() : 0;
  final f = math.pow(10.0, digits).toDouble();
  return (n * f).truncateToDouble() / f;
}

Object _round(List<Object> args, double Function(double) toward) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final n = _asNum(args[0]);
  final digits = args.length == 2 ? _asNum(args[1]).round() : 0;
  final f = math.pow(10.0, digits).toDouble();
  // Excel ROUND uses half-away-from-zero; Dart roundToDouble is half-away.
  return toward(n * f) / f;
}

Object _ceilingFloor(List<Object> args, {required bool up}) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final n = _asNum(args[0]);
  final sig = args.length == 2 ? _asNum(args[1]) : 1.0;
  if (sig == 0) return 0.0;
  final q = n / sig;
  final r = up ? q.ceilToDouble() : q.floorToDouble();
  return r * sig;
}

Object _mod(List<Object> args) {
  if (args.length != 2) throw FormulaError('#ERR');
  final n = _asNum(args[0]);
  final d = _asNum(args[1]);
  if (d == 0) throw FormulaError('#DIV/0!');
  return n - d * (n / d).floorToDouble();
}

Object _power(List<Object> args) {
  if (args.length != 2) throw FormulaError('#ERR');
  return math.pow(_asNum(args[0]), _asNum(args[1])).toDouble();
}

Object _logFn(List<Object> args, _Parser _) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final n = _asNum(args[0]);
  if (n <= 0) throw FormulaError('#NUM!');
  final base = args.length == 2 ? _asNum(args[1]) : 10.0;
  if (base <= 0 || base == 1) throw FormulaError('#NUM!');
  return math.log(n) / math.log(base);
}

Object _gcdLcm(List<Object> args, {required bool gcd}) {
  if (args.isEmpty) throw FormulaError('#ERR');
  var acc = _asNum(args[0]).abs().round();
  for (var i = 1; i < args.length; i++) {
    final b = _asNum(args[i]).abs().round();
    acc = gcd ? _gcdInt(acc, b) : _lcmInt(acc, b);
  }
  return acc.toDouble();
}

int _gcdInt(int a, int b) {
  var x = a.abs();
  var y = b.abs();
  while (y != 0) {
    final t = y;
    y = x % y;
    x = t;
  }
  return x;
}

int _lcmInt(int a, int b) {
  if (a == 0 || b == 0) return 0;
  return (a ~/ _gcdInt(a, b) * b).abs();
}

Object _count(List<Object> args, {required bool numbersOnly}) {
  var n = 0;
  for (final a in args) {
    if (numbersOnly) {
      if (a is num) n++;
    } else {
      n++;
    }
  }
  return n.toDouble();
}

Object _countBlank(List<Object> args, _Parser ctx) {
  var n = 0;
  for (final a in args) {
    if (a is _FormulaRange) {
      for (var r = a.row0; r <= a.row1; r++) {
        for (var c = a.col0; c <= a.col1; c++) {
          if (ctx.book.cell(a.sheet, r, c).isEmpty) n++;
        }
      }
    } else if (a is String && a.isEmpty) {
      n++;
    }
  }
  return n.toDouble();
}

Object _median(List<Object> args, _Parser _) {
  if (args.isEmpty) throw FormulaError('#NUM!');
  final nums = _nums(args)..sort();
  final mid = nums.length ~/ 2;
  if (nums.length.isOdd) return nums[mid];
  return (nums[mid - 1] + nums[mid]) / 2;
}

Object _ifFn(List<Object> args, _Parser _) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  if (_truthy(args[0])) return args[1];
  return args.length == 3 ? args[2] : false;
}

Object _ifError(List<Object> args, _Parser _) {
  // Handled lazily in parser; registry entry unused.
  throw FormulaError('#ERR');
}

Object _ifNa(List<Object> args, _Parser _) {
  throw FormulaError('#ERR');
}

Object _ifs(List<Object> args, _Parser _) {
  if (args.isEmpty || args.length.isOdd) throw FormulaError('#ERR');
  for (var i = 0; i < args.length; i += 2) {
    if (_truthy(args[i])) return args[i + 1];
  }
  throw FormulaError('#N/A');
}

Object _switchFn(List<Object> args, _Parser _) {
  if (args.length < 3) throw FormulaError('#ERR');
  final expr = args[0];
  var i = 1;
  Object? defaultValue;
  final hasDefault = args.length.isEven;
  final limit = hasDefault ? args.length - 1 : args.length;
  while (i < limit) {
    if (_compare(expr, args[i], '=') == true) return args[i + 1];
    i += 2;
  }
  if (hasDefault) return args.last;
  throw FormulaError('#N/A');
}

Object _leftRight(List<Object> args, {required bool left}) {
  if (args.isEmpty || args.length > 2) throw FormulaError('#ERR');
  final text = _asText(args[0]);
  final n = args.length == 2 ? _asNum(args[1]).floor() : 1;
  if (n < 0) throw FormulaError('#VALUE!');
  if (n == 0) return '';
  if (left) {
    return text.substring(0, math.min(n, text.length));
  }
  if (n >= text.length) return text;
  return text.substring(text.length - n);
}

Object _mid(List<Object> args, _Parser _) {
  if (args.length != 3) throw FormulaError('#ERR');
  final text = _asText(args[0]);
  final start = _asNum(args[1]).floor();
  final len = _asNum(args[2]).floor();
  if (start < 1 || len < 0) throw FormulaError('#VALUE!');
  final from = start - 1;
  if (from >= text.length) return '';
  return text.substring(from, math.min(from + len, text.length));
}

Object _textJoin(List<Object> args, _Parser _) {
  if (args.length < 3) throw FormulaError('#ERR');
  final delim = _asText(args[0]);
  final ignoreEmpty = _truthy(args[1]);
  final parts = <String>[];
  for (var i = 2; i < args.length; i++) {
    final t = _asText(args[i]);
    if (ignoreEmpty && t.isEmpty) continue;
    parts.add(t);
  }
  return parts.join(delim);
}

Object _substitute(List<Object> args, _Parser _) {
  if (args.length < 3 || args.length > 4) throw FormulaError('#ERR');
  final text = _asText(args[0]);
  final old = _asText(args[1]);
  final neu = _asText(args[2]);
  if (args.length == 3) return text.replaceAll(old, neu);
  final instance = _asNum(args[3]).floor();
  if (instance < 1) throw FormulaError('#VALUE!');
  var seen = 0;
  return text.replaceAllMapped(RegExp(RegExp.escape(old)), (m) {
    seen++;
    return seen == instance ? neu : m[0]!;
  });
}

Object _replace(List<Object> args, _Parser _) {
  if (args.length != 4) throw FormulaError('#ERR');
  final text = _asText(args[0]);
  final start = _asNum(args[1]).floor();
  final len = _asNum(args[2]).floor();
  final neu = _asText(args[3]);
  if (start < 1 || len < 0) throw FormulaError('#VALUE!');
  final from = start - 1;
  final before = text.substring(0, math.min(from, text.length));
  final after = from + len >= text.length ? '' : text.substring(from + len);
  return before + neu + after;
}

Object _findSearch(List<Object> args, {required bool caseSensitive}) {
  if (args.length < 2 || args.length > 3) throw FormulaError('#ERR');
  var find = _asText(args[0]);
  var within = _asText(args[1]);
  final start = args.length == 3 ? _asNum(args[2]).floor() : 1;
  if (start < 1) throw FormulaError('#VALUE!');
  if (!caseSensitive) {
    find = find.toLowerCase();
    within = within.toLowerCase();
  }
  final idx = within.indexOf(find, start - 1);
  if (idx < 0) throw FormulaError('#VALUE!');
  return (idx + 1).toDouble();
}

Object _isBlank(List<Object> args, _Parser ctx) {
  if (args.length != 1) throw FormulaError('#ERR');
  final a = args[0];
  if (a is _FormulaRange) {
    if (a.rows != 1 || a.cols != 1) throw FormulaError('#VALUE!');
    return ctx.book.cell(a.sheet, a.row0, a.col0).isEmpty;
  }
  return a is String && a.isEmpty;
}

Object _compare(Object left, Object right, String op) {
  final l = _unwrap(left);
  final r = _unwrap(right);
  int cmp;
  if (l is num && r is num) {
    cmp = l.compareTo(r);
  } else if (l is bool && r is bool) {
    cmp = (l ? 1 : 0).compareTo(r ? 1 : 0);
  } else {
    cmp = _asText(l).toLowerCase().compareTo(_asText(r).toLowerCase());
  }
  switch (op) {
    case '=':
      return cmp == 0;
    case '<>':
      return cmp != 0;
    case '<':
      return cmp < 0;
    case '>':
      return cmp > 0;
    case '<=':
      return cmp <= 0;
    case '>=':
      return cmp >= 0;
  }
  throw FormulaError('#ERR');
}

bool _isIdentifier(String raw) {
  if (raw.length <= 1 || raw.contains('.')) return false;
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
