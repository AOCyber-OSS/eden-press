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

/// Dart mirrors of the `eden-press.capi/v1` JSON wire envelope and the
/// `eden-press.model/v2` document model.
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

/// Mirrors `model.Meta` (chase/model/document.go): `{"directives": {...}}`.
class Meta {
  const Meta({required this.directives});

  final Map<String, String> directives;

  factory Meta.fromJson(Map<String, dynamic> json) {
    final directivesJson = json['directives'];
    return Meta(
      directives:
          directivesJson is Map
              ? directivesJson.map(
                (key, value) => MapEntry(key as String, value as String),
              )
              : const <String, String>{},
    );
  }
}

/// Mirrors `model.Document` (chase/model/document.go): the schema-v2 document
/// carried in `Output.Model`.
class ModelDocument {
  const ModelDocument({
    required this.schemaVersion,
    required this.meta,
    required this.sections,
    required this.outline,
  });

  /// The wire shape this binding understands.
  ///
  /// Moved v3 -> v4 with AODex Objective 16 (`span`); v4 -> v5 with the AI
  /// doc-ops spike (optional block `id`). v5 is a STRICT SUPERSET of v4 --
  /// no field was removed, renamed or re-classified -- so this binding parses
  /// a v5 document correctly whether or not it reads the new key.
  ///
  /// The `span` offsets are BYTES. Dart strings are UTF-16, so any code that
  /// indexes a source string with one MUST convert first; a raw byte offset
  /// will be wrong for every position after the first multi-byte character.
  /// This class does not yet surface spans -- reading them is deliberately
  /// left to the consumer that needs them, along with that conversion.
  static const expectedSchemaVersion = 'eden-press.model/v6';

  final String schemaVersion;
  final Meta meta;
  final List<Section> sections;
  final List<OutlineEntry> outline;

  factory ModelDocument.fromJson(Map<String, dynamic> json) {
    final metaJson = json['meta'];
    final sectionsJson = json['sections'];
    final outlineJson = json['outline'];
    return ModelDocument(
      schemaVersion: json['schemaVersion'] as String? ?? '',
      meta:
          metaJson is Map<String, dynamic>
              ? Meta.fromJson(metaJson)
              : const Meta(directives: {}),
      sections:
          sectionsJson is List
              ? sectionsJson
                  .map((e) => Section.fromJson(e as Map<String, dynamic>))
                  .toList()
              : const <Section>[],
      outline:
          outlineJson is List
              ? outlineJson
                  .map((e) => OutlineEntry.fromJson(e as Map<String, dynamic>))
                  .toList()
              : const <OutlineEntry>[],
    );
  }
}

/// Mirrors `model.Section` (chase/model/document.go).
class Section {
  const Section({
    required this.id,
    required this.attrs,
    required this.notes,
    required this.blocks,
  });

  final int id;
  final Map<String, String> attrs;
  final List<String> notes;
  final List<Block> blocks;

  factory Section.fromJson(Map<String, dynamic> json) {
    final attrsJson = json['attrs'];
    final notesJson = json['notes'];
    final blocksJson = json['blocks'];
    return Section(
      id: json['id'] as int? ?? 0,
      attrs:
          attrsJson is Map
              ? attrsJson.map(
                (key, value) => MapEntry(key as String, value as String),
              )
              : const <String, String>{},
      notes:
          notesJson is List
              ? notesJson.map((e) => e as String).toList()
              : const <String>[],
      blocks:
          blocksJson is List
              ? blocksJson
                  .map((e) => Block.fromJson(e as Map<String, dynamic>))
                  .toList()
              : const <Block>[],
    );
  }
}

/// Mirrors `model.BlockKind` (chase/model/document.go) string enum values.
enum BlockKind {
  paragraph,
  list,
  code,
  math,
  heading,
  table,
  image,
  quote,
  toggle,
  synced,
  unknown,
}

BlockKind _blockKindFromWire(String value) {
  switch (value) {
    case 'paragraph':
      return BlockKind.paragraph;
    case 'list':
      return BlockKind.list;
    case 'code':
      return BlockKind.code;
    case 'math':
      return BlockKind.math;
    case 'heading':
      return BlockKind.heading;
    case 'table':
      return BlockKind.table;
    case 'image':
      return BlockKind.image;
    case 'quote':
      return BlockKind.quote;
    case 'toggle':
      return BlockKind.toggle;
    case 'synced':
      return BlockKind.synced;
    default:
      return BlockKind.unknown;
  }
}

/// Mirrors `model.Block` (chase/model/document.go). [text] carries raw TeX
/// for `math` blocks and raw (pre-highlight) source for `code` blocks --
/// never rendered HTML. [language] is the fenced info-string for `code`
/// blocks. [display] distinguishes display-mode (`$$`) vs inline (`$`) math.
class Block {
  const Block({
    this.id = '',
    required this.kind,
    required this.text,
    required this.level,
    required this.language,
    required this.display,
    required this.ordered,
    required this.items,
    this.headers = const <String>[],
    this.rows = const <List<String>>[],
    this.aligns = const <String>[],
    this.src = '',
    this.title = '',
    this.children = const <Block>[],
    this.collapsed = false,
    this.syncRef,
  });

  /// Stable block id for apply_ops (schema v5+). Empty on Build()-produced
  /// Markdown imports until an editor/agent assigns one.
  final String id;

  final BlockKind kind;
  final String text;
  final int level;
  final String language;
  final bool display;
  final bool ordered;
  final List<ListItem> items;

  /// Header-row cell texts of a `table` block, left to right. Empty for a
  /// header-less table and for every other kind.
  final List<String> headers;

  /// Body rows of a `table` block, each a left-to-right list of cell texts.
  /// A table's payload lives HERE, never in [text].
  final List<List<String>> rows;

  /// Per-column alignment of a `table` block: `left` / `right` / `center`, or
  /// an empty entry where the column declared none.
  final List<String> aligns;

  /// Destination URL of an `image` block. The alt text is in [text].
  final String src;

  /// Optional title of an `image` block -- the `![alt](src "title")` third
  /// argument.
  final String title;

  /// Nested children (schema v6).
  final List<Block> children;

  /// Toggle collapsed default (schema v6).
  final bool collapsed;

  /// Synced-block source ref (schema v6).
  final Map<String, String>? syncRef;

  factory Block.fromJson(Map<String, dynamic> json) {
    final itemsJson = json['items'];
    final childrenJson = json['children'];
    final sync = json['syncRef'];
    return Block(
      id: json['id'] as String? ?? '',
      kind: _blockKindFromWire(json['kind'] as String? ?? ''),
      text: json['text'] as String? ?? '',
      level: json['level'] as int? ?? 0,
      language: json['language'] as String? ?? '',
      display: json['display'] as bool? ?? false,
      ordered: json['ordered'] as bool? ?? false,
      items:
          itemsJson is List
              ? itemsJson
                  .map((e) => ListItem.fromJson(e as Map<String, dynamic>))
                  .toList()
              : const <ListItem>[],
      headers: _stringList(json['headers']),
      rows:
          json['rows'] is List
              ? (json['rows'] as List)
                  .map<List<String>>((r) => _stringList(r))
                  .toList()
              : const <List<String>>[],
      aligns: _stringList(json['aligns']),
      src: json['src'] as String? ?? '',
      title: json['title'] as String? ?? '',
      children:
          childrenJson is List
              ? childrenJson
                  .map((e) => Block.fromJson(e as Map<String, dynamic>))
                  .toList()
              : const <Block>[],
      collapsed: json['collapsed'] as bool? ?? false,
      syncRef:
          sync is Map
              ? sync.map((k, v) => MapEntry(k.toString(), v?.toString() ?? ''))
              : null,
    );
  }
}

/// Mirrors `model.ListItem` (chase/model/document.go).
class ListItem {
  const ListItem({required this.text, required this.level});

  final String text;
  final int level;

  factory ListItem.fromJson(Map<String, dynamic> json) => ListItem(
    text: json['text'] as String? ?? '',
    level: json['level'] as int? ?? 0,
  );
}

/// Mirrors `model.OutlineEntry` (chase/model/document.go).
class OutlineEntry {
  const OutlineEntry({
    required this.sectionId,
    required this.level,
    required this.text,
    required this.slug,
  });

  final int sectionId;
  final int level;
  final String text;
  final String slug;

  factory OutlineEntry.fromJson(Map<String, dynamic> json) => OutlineEntry(
    sectionId: json['sectionId'] as int? ?? 0,
    level: json['level'] as int? ?? 0,
    text: json['text'] as String? ?? '',
    slug: json['slug'] as String? ?? '',
  );
}

/// Decodes a wire array of strings, tolerating a null/absent field (every
/// schema-v3 addition is `omitempty` on the Go side, so an unused one is
/// simply not present) and a non-string entry.
List<String> _stringList(Object? value) {
  if (value is! List) {
    return const <String>[];
  }
  return value.map((e) => e?.toString() ?? '').toList();
}
