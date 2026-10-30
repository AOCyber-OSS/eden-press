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

EdenWorkbook _book() {
  final book = EdenWorkbook(title: 'Budget');
  final sheet = book.addSheet('FY');
  sheet.setCell(0, 0, '1');
  sheet.setCell(1, 0, '2');
  sheet.setCell(2, 0, '3');
  sheet.setCell(0, 1, '=A1+A2*3');
  sheet.setCell(1, 1, '=SUM(A1:A3)');
  sheet.setCell(2, 1, '007');
  final other = book.addSheet('Notes');
  other.setCell(0, 0, '4');
  return book;
}

void main() {
  test('arithmetic respects precedence and parentheses', () {
    final book = _book();
    expect(evaluateFormula(book, 'FY', '1+2*3'), 7);
    expect(evaluateFormula(book, 'FY', '(1+2)*3'), 9);
    expect(displayCell(book, 'FY', 0, 1), '7');
  });

  test('SUM and AVERAGE read a range, and 007 stays text', () {
    final book = _book();
    expect(evaluateFormula(book, 'FY', 'SUM(A1:A3)'), 6);
    expect(evaluateFormula(book, 'FY', 'AVERAGE(A1:A3)'), 2);
    expect(displayCell(book, 'FY', 2, 1), '007');
    expect(
      () => evaluateFormula(book, 'FY', 'A1+C3'),
      throwsA(isA<FormulaError>().having((e) => e.code, 'code', '#VALUE!')),
    );
  });

  test('cross-sheet refs and cycles', () {
    final book = _book();
    expect(evaluateFormula(book, 'FY', 'Notes!A1*2'), 8);
    final sheet = book.sheet('FY')!;
    sheet.setCell(3, 0, '=A5');
    sheet.setCell(4, 0, '=A4');
    expect(displayCell(book, 'FY', 3, 0), '#CYCLE!');
  });

  test('aggregates ignore blanks and a long range stays on stored cells', () {
    final book = EdenWorkbook();
    final sheet = book.addSheet('FY');
    sheet.setCell(0, 0, '1');
    sheet.setCell(2, 0, '3');
    expect(evaluateFormula(book, 'FY', 'SUM(A1:A3)'), 4);
    expect(evaluateFormula(book, 'FY', 'AVERAGE(A1:A3)'), 2);
    expect(evaluateFormula(book, 'FY', 'MIN(A1:A3)'), 1);
    expect(evaluateFormula(book, 'FY', 'MAX(A1:A3)'), 3);

    sheet.setCell(99999, 0, '2');
    final watch = Stopwatch()..start();
    final value = evaluateFormula(book, 'FY', 'SUM(A1:A100000)');
    watch.stop();
    expect(value, 6);
    expect(watch.elapsedMilliseconds, lessThan(25));
  });

  test('a cached formula refreshes when a cell inside its range changes', () {
    final book = EdenWorkbook();
    final sheet = book.addSheet('FY');
    sheet.setCell(0, 0, '1');
    sheet.setCell(2, 0, '3');
    sheet.setCell(3, 0, '=SUM(A1:A3)');
    expect(displayCell(book, 'FY', 3, 0), '4');
    sheet.setCell(1, 0, '10');
    expect(displayCell(book, 'FY', 3, 0), '14');
  });

  test('division by zero and unknown functions', () {
    final book = _book();
    expect(
      () => evaluateFormula(book, 'FY', '1/0'),
      throwsA(isA<FormulaError>().having((e) => e.code, 'code', '#DIV/0!')),
    );
    expect(
      () => evaluateFormula(book, 'FY', 'POW(2,3)'),
      throwsA(isA<FormulaError>().having((e) => e.code, 'code', '#NAME?')),
    );
  });
}
