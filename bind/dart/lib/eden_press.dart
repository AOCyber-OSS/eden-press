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

/// `eden_press`: the JS-free Dart/Flutter rendering surface for Eden Press
/// (DART-04).
///
/// [render] round-trips Markdown through the Eden Press engine (native via
/// dart:ffi on Android/iOS, wasm via dart:js_interop on Web -- see
/// `src/native_loader.dart` / `src/web_loader.dart`) and returns a decoded
/// [Output] whose [ModelDocument] (schema-v2 `Section.Blocks`) can be handed
/// to [EdenPressView] for a fully native render: math via `flutter_math_fork`,
/// code via `flutter_highlighting`. Nothing in this path parses `Output.html`
/// or executes JavaScript in the Dart application layer.
library;

import 'dart:convert';

import 'src/model.dart';
// Conditional import: Web (dart.library.js_interop available) gets
// src/web_loader.dart; every other target (Android/iOS/desktop) gets
// src/native_loader.dart. Both expose the same `String renderJson(String)`.
import 'src/native_loader.dart'
    if (dart.library.js_interop) 'src/web_loader.dart'
    as backend;

// The document model moved to the pure-Dart eden_press_model package (no
// FFI, no plugin) so eden_docs can depend on it alone. Re-exported here so
// every existing `package:eden_press/eden_press.dart` import still sees it.
export 'package:eden_press_model/eden_press_model.dart';

export 'src/fit_text.dart';
export 'src/model.dart';
export 'src/render_surface.dart';

/// Renders [md] through the Eden Press engine and returns the decoded
/// [Output], including its schema-v2 [ModelDocument].
///
/// Throws a [StateError] if the engine reports an error in the response
/// envelope (`response.error` populated instead of `response.output`).
Future<Output> render(
  String md, {
  EdenPressOptions opts = const EdenPressOptions(),
}) async {
  final request = RenderRequest(markdown: md, options: opts);
  final responseJson = backend.renderJson(jsonEncode(request.toJson()));
  final response = RenderResponse.fromJson(
    jsonDecode(responseJson) as Map<String, dynamic>,
  );
  final output = response.output;
  if (output == null) {
    throw StateError(
      response.error ?? 'eden_press: render failed with no error message',
    );
  }
  return output;
}
