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

// eden-press#29: the nesting limit is one number shared by Go and Dart.
// test/fixtures/limits.json is the conformance fixture: Go's
// TestNesting_N11 checks model.MaxNestingDepth against it, and this test
// checks maxNestingDepth, so the two cannot drift apart unnoticed.

import 'dart:convert';
import 'dart:io';

import 'package:eden_press_model/eden_press_model.dart';
import 'package:test/test.dart';

void main() {
  test('maxNestingDepth matches the Go conformance fixture', () {
    final file = File('test/fixtures/limits.json');
    expect(
      file.existsSync(),
      isTrue,
      reason: 'run `dart test` from bind/eden_press_model/',
    );
    final limits = jsonDecode(file.readAsStringSync()) as Map<String, dynamic>;
    expect(maxNestingDepth, limits['maxNestingDepth']);
  });
}
