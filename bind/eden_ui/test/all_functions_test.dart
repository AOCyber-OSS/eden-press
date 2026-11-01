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

import 'package:eden_ui/eden_ui.dart';
import 'package:flutter_test/flutter_test.dart';

import 'fn_catalog.dart';

EdenWorkbook _book() {
  final book = EdenWorkbook(title: 'All formulas');
  final data = book.addSheet('Data');
  data.setCell(0, 0, '1');
  data.setCell(1, 0, '2');
  data.setCell(2, 0, '3');
  data.setCell(0, 1, '10');
  data.setCell(1, 1, '20');
  data.setCell(2, 1, '30');
  data.setCell(0, 2, 'x');
  data.setCell(1, 2, 'y');
  data.setCell(2, 2, 'x');
  data.setCell(0, 3, 'a');
  data.setCell(1, 3, 'b');
  data.setCell(2, 3, 'c');
  data.setCell(0, 4, '10');
  data.setCell(1, 4, '20');
  data.setCell(2, 4, '30');
  data.setCell(0, 5, 'x');
  data.setCell(1, 5, 'y');
  data.setCell(2, 5, 'z');
  data.setCell(0, 6, '1');
  data.setCell(1, 6, '2');
  data.setCell(2, 6, '3');
  data.setCell(1, 7, '1');
  final sample = ['2', '4', '4', '4', '5', '5', '7', '9'];
  for (var i = 0; i < sample.length; i++) {
    data.setCell(i, 8, sample[i]);
  }
  data.setCell(0, 9, 'x');
  data.setCell(0, 10, 'y');
  data.setCell(0, 11, 'z');
  data.setCell(1, 9, '1');
  data.setCell(1, 10, '2');
  data.setCell(1, 11, '3');
  book.addSheet('Catalog');
  return book;
}

String _fmt(Object v) {
  if (v is bool) return v ? 'TRUE' : 'FALSE';
  if (v is num) {
    final d = v.toDouble();
    if (d == d.roundToDouble() && d.abs() < 1e15) {
      return d.round().toString();
    }
    return formatNumber(d);
  }
  return v.toString();
}

void main() {
  setUp(() {
    formulaNowOverride = DateTime.utc(2020, 1, 15, 12);
    formulaRandOverride = 0.25;
  });
  tearDown(() {
    formulaNowOverride = null;
    formulaRandOverride = null;
  });

  test('catalog covers every registered function exactly once', () {
    expect(kAllFunctionSamples.length, kRegisteredFunctionCount);
    final names = kAllFunctionSamples.map((s) => s.name).toSet();
    expect(names.length, kRegisteredFunctionCount);
  });

  test('each registered function evaluates to its catalog expect', () {
    final book = _book();
    final failures = <String>[];
    for (final s in kAllFunctionSamples) {
      try {
        if (s.expect.startsWith('#')) {
          expect(
            () => evaluateFormula(book, 'Catalog', s.expr),
            throwsA(
              isA<FormulaError>().having((e) => e.code, 'code', s.expect),
            ),
            reason: s.name,
          );
          // Also check display path surfaces the error code.
          book.sheet('Catalog')!.setCell(0, 0, '=${s.expr}');
          expect(displayCell(book, 'Catalog', 0, 0), s.expect, reason: s.name);
          continue;
        }
        final got = evaluateFormula(book, 'Catalog', s.expr);
        final text = _fmt(got);
        final want = s.expect;
        if (text.toUpperCase() != want.toUpperCase()) {
          // Numeric tolerance for formatted floats.
          final g = double.tryParse(text);
          final w = double.tryParse(want);
          if (g != null && w != null && (g - w).abs() < 1e-6) {
            continue;
          }
          failures.add('${s.name}: got "$text" want "$want" expr=${s.expr}');
        }
      } catch (e) {
        failures.add('${s.name}: $e expr=${s.expr}');
      }
    }
    expect(failures, isEmpty, reason: failures.join('\n'));
  });
}
