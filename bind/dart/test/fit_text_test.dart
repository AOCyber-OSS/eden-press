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

// 08-07-TRD.md Test list, driven RED -> GREEN, outermost to innermost:
//   1. computeFitFontSize returns maxFontSize when text fits at max in the
//      given constraints (SHRINK-ONLY -- never upscales past max).
//   2. A long heading in a narrow BoxConstraints returns a size STRICTLY
//      LESS than maxFontSize that does not overflow (height <= maxHeight,
//      width <= maxWidth).
//   3. Monotonic: for the same text, a wider constraint yields a fitted size
//      >= a narrower constraint's fitted size.
//   4. Widget (testWidgets): a FitText with a long string inside a
//      constrained SizedBox renders WITHOUT a RenderFlex/overflow error; the
//      fitted Text uses a size <= its style's max.
//   5. JS-free assertion: fit_text.dart + render_surface.dart declare no
//      JS-interop / web-only-package / embedded-browser-surface import --
//      the DART-04 JS-free contract extended to auto-fit.
//
// No generated/property-based data -- every fixture below is a hand-built
// fixed string + explicit BoxConstraints (mirrors render_surface_test.dart's
// testWidgets/unit-test shape).

import 'dart:io';

import 'package:eden_press/src/fit_text.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

const _longHeading =
    'This Is An Extremely Long Heading Title That Would Normally Overflow '
    'A Narrow Slide Column';

void main() {
  group('computeFitFontSize', () {
    test('Case 1: text that already fits at maxFontSize returns maxFontSize '
        '(shrink-only, never upscales)', () {
      const style = TextStyle();
      final fitted = computeFitFontSize(
        'Hi',
        const BoxConstraints(maxWidth: 400, maxHeight: 200),
        style: style,
        maxFontSize: 24,
        minFontSize: 8,
      );
      expect(fitted, 24.0);
    });

    test('Case 2: a long heading in narrow constraints returns < maxFontSize '
        'and does not overflow (height/width both within bounds)', () {
      const style = TextStyle();
      const constraints = BoxConstraints(maxWidth: 120, maxHeight: 100);

      final fitted = computeFitFontSize(
        _longHeading,
        constraints,
        style: style,
        maxFontSize: 48,
        minFontSize: 8,
      );
      expect(fitted, lessThan(48.0));

      final tp = TextPainter(
        text: TextSpan(
          text: _longHeading,
          style: style.copyWith(fontSize: fitted),
        ),
        textDirection: TextDirection.ltr,
      )..layout(maxWidth: constraints.maxWidth);
      expect(tp.height, lessThanOrEqualTo(constraints.maxHeight));
      expect(tp.width, lessThanOrEqualTo(constraints.maxWidth));
      expect(tp.didExceedMaxLines, isFalse);
    });

    test('Case 3: monotonic -- a wider constraint fits >= a narrower '
        "constraint's fitted size, for the same text", () {
      const style = TextStyle();
      final narrow = computeFitFontSize(
        _longHeading,
        const BoxConstraints(maxWidth: 100, maxHeight: 200),
        style: style,
        maxFontSize: 48,
        minFontSize: 8,
      );
      final wide = computeFitFontSize(
        _longHeading,
        const BoxConstraints(maxWidth: 400, maxHeight: 200),
        style: style,
        maxFontSize: 48,
        minFontSize: 8,
      );
      expect(wide, greaterThanOrEqualTo(narrow));
    });
  });

  group('FitText widget', () {
    testWidgets(
      'Case 4: a long heading in a constrained SizedBox renders without a '
      'RenderFlex/overflow error; the fitted Text size stays <= the style max',
      (tester) async {
        await tester.pumpWidget(
          MaterialApp(
            home: Scaffold(
              body: SizedBox(
                width: 200,
                height: 60,
                child: FitText(
                  _longHeading,
                  style: const TextStyle(
                    fontSize: 32,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
            ),
          ),
        );
        await tester.pumpAndSettle();

        // No overflow/RenderFlex exception surfaced during the pump.
        expect(tester.takeException(), isNull);

        final rendered = tester.widget<Text>(find.byType(Text));
        expect(rendered.style!.fontSize, lessThanOrEqualTo(32.0));
      },
    );
  });

  group('JS-free assertion (DART-04 extended to auto-fit)', () {
    test('Case 5: fit_text.dart and render_surface.dart import no JS-interop / '
        'web-only-package / embedded-browser-surface dependency', () {
      const forbidden = ['dart:js', 'package:web', 'webview'];
      for (final relativePath in [
        'lib/src/fit_text.dart',
        'lib/src/render_surface.dart',
      ]) {
        final file = File(relativePath);
        expect(file.existsSync(), isTrue, reason: 'run tests from bind/dart/');
        final source = file.readAsStringSync();
        for (final needle in forbidden) {
          expect(
            source.contains(needle),
            isFalse,
            reason:
                '$relativePath must not reference "$needle" anywhere -- '
                'forbidden by the DART-04 JS-free contract extended to '
                'auto-fit',
          );
        }
      }
    });
  });
}
