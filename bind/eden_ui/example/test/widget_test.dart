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

import 'package:eden_ui_demo/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('demo shell opens Sheets with Metrics', (tester) async {
    await tester.pumpWidget(const EdenDesktopDemo());
    expect(find.text('Eden Desktop Demo'), findsOneWidget);
    expect(find.text('Metrics'), findsWidgets);
    expect(find.text('550'), findsOneWidget);
    expect(find.text('1100'), findsOneWidget);

    await tester.tap(find.text('Slides'));
    await tester.pumpAndSettle();
    expect(find.text('Slide 1 of 3'), findsOneWidget);
    expect(find.text('Alpha'), findsOneWidget);
    await tester.tap(find.byKey(const Key('eden-slides-next')));
    await tester.pumpAndSettle();
    expect(find.text('Beta'), findsOneWidget);

    await tester.tap(find.text('Docs'));
    await tester.pumpAndSettle();
    expect(find.text('Roadmap'), findsOneWidget);
    expect(find.textContaining('Ship desktop demo'), findsOneWidget);
  });
}
