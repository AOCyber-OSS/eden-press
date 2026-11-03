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

/// Dart mirrors of the `eden-press.capi/v1` JSON wire envelope.
///
/// The document model the envelope carries (ModelDocument, Meta, Section,
/// Block, ...) lives in the pure-Dart `eden_press_model` package, which
/// `package:eden_press/eden_press.dart` re-exports.
///
/// **Wire-key case is intentional and load-bearing, not a typo.** The Go
/// `press.Output` struct (press/options.go) carries NO json tags, so it
/// marshals with Go-default CAPITALIZED field names: `HTML`, `CSS`, `Model`,
/// `Meta`, `Comments`. The nested `model.Document` type (chase/model/document.go)
/// DOES carry explicit lowercase json tags (`schemaVersion`, `meta`,
/// `sections`, `outline`, `id`, `attrs`, `notes`, `blocks`, `kind`, `text`,
/// `level`, `language`, `display`, `ordered`, `items`). Both facts were
/// confirmed by reading the Go source directly (not guessed) and match the
/// wire-key correction documented in 07-02-SUMMARY.md. Get this wrong and
/// every field silently parses as null.
library;

import 'package:eden_press_model/eden_press_model.dart';

/// Request-side render options, mirrored from `core.requestOptions`
/// (bind/capi/core/render.go).
class EdenPressOptions {
  const EdenPressOptions({
    this.theme = '',
    this.profile = '',
    this.inlineSvg = false,
    this.mathMode = '',
    this.noHighlight = false,
    this.highlightStyle = '',
  });

  final String theme;
  final String profile;
  final bool inlineSvg;
  final String mathMode;
  final bool noHighlight;
  final String highlightStyle;

  Map<String, dynamic> toJson() => <String, dynamic>{
    'theme': theme,
    'profile': profile,
    'inlineSvg': inlineSvg,
    'mathMode': mathMode,
    'noHighlight': noHighlight,
    'highlightStyle': highlightStyle,
  };
}

/// Top-level request envelope, mirrored from `core.request`.
class RenderRequest {
  const RenderRequest({
    required this.markdown,
    this.options = const EdenPressOptions(),
    this.envelopeVersion = 'eden-press.capi/v1',
  });

  final String envelopeVersion;
  final String markdown;
  final EdenPressOptions options;

  Map<String, dynamic> toJson() => <String, dynamic>{
    'envelopeVersion': envelopeVersion,
    'markdown': markdown,
    'options': options.toJson(),
  };
}

/// Top-level response envelope, mirrored from `core.response`. Exactly one
/// of [output] / [error] is populated.
class RenderResponse {
  const RenderResponse({
    required this.envelopeVersion,
    this.output,
    this.error,
  });

  final String envelopeVersion;
  final Output? output;
  final String? error;

  factory RenderResponse.fromJson(Map<String, dynamic> json) {
    final outputJson = json['output'];
    return RenderResponse(
      envelopeVersion: json['envelopeVersion'] as String? ?? '',
      output:
          outputJson is Map<String, dynamic>
              ? Output.fromJson(outputJson)
              : null,
      error: json['error'] as String?,
    );
  }
}

/// Mirrors `press.Output` (press/options.go). Field keys are Go-default
/// CAPITALIZED (no json tags on this struct): HTML, CSS, Model, Meta, Comments.
class Output {
  const Output({
    required this.html,
    required this.css,
    required this.model,
    required this.meta,
    required this.comments,
  });

  final String html;
  final String css;
  final ModelDocument model;
  final Meta meta;
  final List<String> comments;

  factory Output.fromJson(Map<String, dynamic> json) {
    final modelJson = json['Model'];
    final metaJson = json['Meta'];
    final commentsJson = json['Comments'];
    return Output(
      html: json['HTML'] as String? ?? '',
      css: json['CSS'] as String? ?? '',
      model:
          modelJson is Map<String, dynamic>
              ? ModelDocument.fromJson(modelJson)
              : const ModelDocument(
                schemaVersion: '',
                meta: Meta(directives: {}),
                sections: [],
                outline: [],
              ),
      meta:
          metaJson is Map<String, dynamic>
              ? Meta.fromJson(metaJson)
              : const Meta(directives: {}),
      comments:
          commentsJson is List
              ? commentsJson.map((e) => e as String).toList()
              : const <String>[],
    );
  }
}
