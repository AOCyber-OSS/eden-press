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

// 07-05-TRD.md Test list, driven RED -> GREEN, outermost to innermost:
//   1. EdenPressView builds a tree with exactly one Math widget and one
//      HighlightView from a hand-built Output/Model fixture with one math +
//      one code block.
//   2. The math block's `display` flag maps to MathStyle.display (true) /
//      MathStyle.text (false).
//   3. The code block's `language` is passed to HighlightView.languageId and
//      `source` is the raw (un-highlighted) text.
//   4. pubspec.yaml's dependencies declare no html/dom-parsing/js/webview
//      package -- asserted structurally so the JS-free contract can't
//      silently regress.
//
// No generated/property-based data -- every fixture below is hand-built
// inline JSON mirroring the real eden-press.capi/v1 + eden-press.model/v2
// wire shapes (see model.dart's library doc for the confirmed field casing).

import 'dart:io';

import 'package:eden_press/eden_press.dart';
import 'package:flutter/material.dart';
import 'package:flutter_highlighting/flutter_highlighting.dart';
import 'package:flutter_math_fork/flutter_math.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yaml/yaml.dart';

/// Hand-built Output fixture: one Section with one `math` block
/// (`display: true`) followed by one `code` block (`language: dart`).
Output _fixtureOutput({bool mathDisplay = true, String language = 'dart'}) {
  final json = <String, dynamic>{
    'HTML': '<p>ignored by the render surface</p>',
    'CSS': '',
    'Model': {
      'schemaVersion': 'eden-press.model/v2',
      'meta': {'directives': <String, String>{}},
      'sections': [
        {
          'id': 1,
          'attrs': <String, String>{},
          'notes': <String>[],
          'blocks': [
            {'kind': 'math', 'text': r'\frac{a}{b}', 'display': mathDisplay},
            {'kind': 'code', 'text': 'void main() {}', 'language': language},
          ],
        },
      ],
      'outline': <Map<String, dynamic>>[],
    },
    'Meta': {'directives': <String, String>{}},
    'Comments': <String>[],
  };
  return Output.fromJson(json);
}

Widget _wrap(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  testWidgets(
    'Case 1: EdenPressView renders exactly one Math and one HighlightView from the JSON model',
    (tester) async {
      await tester.pumpWidget(_wrap(EdenPressView(_fixtureOutput())));

      expect(find.byType(Math), findsOneWidget);
      expect(find.byType(HighlightView), findsOneWidget);
    },
  );

  testWidgets(
    'Case 2: block.display=true maps to MathStyle.display, false maps to MathStyle.text',
    (tester) async {
      await tester.pumpWidget(
        _wrap(EdenPressView(_fixtureOutput(mathDisplay: true))),
      );
      final displayMath = tester.widget<Math>(find.byType(Math));
      expect(displayMath.mathStyle, MathStyle.display);

      await tester.pumpWidget(
        _wrap(EdenPressView(_fixtureOutput(mathDisplay: false))),
      );
      final inlineMath = tester.widget<Math>(find.byType(Math));
      expect(inlineMath.mathStyle, MathStyle.text);
    },
  );

  testWidgets(
    'Case 3: block.language -> HighlightView.languageId, block.text -> raw HighlightView.source',
    (tester) async {
      await tester.pumpWidget(
        _wrap(EdenPressView(_fixtureOutput(language: 'dart'))),
      );
      final view = tester.widget<HighlightView>(find.byType(HighlightView));

      expect(view.languageId, 'dart');
      // HighlightView.source is the raw, un-highlighted input (tabs expanded
      // to spaces internally; this fixture has none) -- never HTML/DOM markup.
      expect(view.source, 'void main() {}');
    },
  );

  test('Case 4: pubspec.yaml declares no html/dom-parsing/js/webview dependency', () {
    final pubspecFile = File('pubspec.yaml');
    expect(
      pubspecFile.existsSync(),
      isTrue,
      reason: 'run tests from bind/dart/',
    );

    final doc = loadYaml(pubspecFile.readAsStringSync()) as YamlMap;
    final deps = <String>{
      ...(doc['dependencies'] as YamlMap? ?? YamlMap()).keys.cast<String>(),
      ...(doc['dev_dependencies'] as YamlMap? ?? YamlMap()).keys.cast<String>(),
    };

    const forbiddenSubstrings = ['html', 'webview', 'dom', 'js'];
    for (final dep in deps) {
      final lower = dep.toLowerCase();
      for (final forbidden in forbiddenSubstrings) {
        expect(
          lower.contains(forbidden),
          isFalse,
          reason:
              'dependency "$dep" looks like an HTML/DOM/JS/webview package -- forbidden by DART-04',
        );
      }
    }
  });
}
