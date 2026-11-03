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

// 1026-05 Test list item 5: eden_docs depends on the pure-Dart
// eden_press_model, NEVER on the eden_press FFI plugin.
//
// eden_press is a `ffiPlugin: true` package: its iOS pod vendors a gitignored
// EdenPress.xcframework and its Android side packages prebuilt jniLibs. A host
// app (AODex) that pulls eden_docs as a git dependency would get that plugin
// too and fail to build on iOS, Android and macOS. eden_docs only needs the
// document model, so this test fails the moment the plugin comes back --
// directly in pubspec.yaml, or transitively in the resolved package graph.
//
// Same technique as bind/dart/test/render_surface_test.dart (Case 4): parse
// the package's own pubspec structurally rather than grepping it.

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:yaml/yaml.dart';

Set<String> _keys(YamlMap doc, String section) =>
    (doc[section] as YamlMap? ?? YamlMap()).keys.cast<String>().toSet();

void main() {
  test('pubspec dependencies contain eden_press_model and not eden_press', () {
    final pubspec = File('pubspec.yaml');
    expect(
      pubspec.existsSync(),
      isTrue,
      reason: 'run tests from bind/eden_docs/',
    );
    final doc = loadYaml(pubspec.readAsStringSync()) as YamlMap;

    expect(_keys(doc, 'dependencies'), contains('eden_press_model'));
    for (final section in [
      'dependencies',
      'dev_dependencies',
      'dependency_overrides',
    ]) {
      expect(
        _keys(doc, section),
        isNot(contains('eden_press')),
        reason:
            '$section names eden_press, the FFI plugin; eden_docs must use '
            'eden_press_model so host apps need no native artifacts',
      );
    }
  });

  test('the resolved package graph contains no eden_press plugin', () {
    // `flutter test` runs `pub get` first, so the resolution exists. This
    // catches the plugin arriving transitively, through some other package.
    final config = File('.dart_tool/package_config.json');
    expect(
      config.existsSync(),
      isTrue,
      reason: 'run `flutter pub get` in bind/eden_docs/ first',
    );
    final json = jsonDecode(config.readAsStringSync()) as Map<String, dynamic>;
    final names = <String>{
      for (final pkg in json['packages'] as List)
        (pkg as Map<String, dynamic>)['name'] as String,
    };
    expect(names, contains('eden_press_model'));
    expect(names, isNot(contains('eden_press')));
  });
}
