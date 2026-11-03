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

/// Dart mirror of the `eden-press.model/v7` document model
/// (chase/model/document.go).
///
/// Moved here from the eden_press FFI plugin (bind/dart) so a host app can
/// read and write documents without that plugin's native artifacts. eden_press
/// re-exports this library, so code importing `package:eden_press` still sees
/// every class below.
///
/// The json keys are the Go struct's explicit lowercase json tags
/// (`schemaVersion`, `meta`, `sections`, `outline`, `kind`, ...), confirmed by
/// reading chase/model/document.go. They are NOT the capitalized keys of the
/// surrounding `press.Output` envelope, which eden_press decodes itself.
///
/// **toJson is a contract, not a convenience.** Every `toJson` writes what Go's
/// `json.Marshal` writes for the same value: the same tag names, in Go
/// struct-field order, with each `omitempty` mirrored field by field (an
/// omitempty string is dropped when empty, a bool when false, an int when 0, a
/// list or map when empty, a pointer when null; a field without omitempty is
/// always written). Map keys are written sorted, as Go sorts them.
/// test/model_json_test.dart proves `ModelDocument.fromJson(golden).toJson()`
/// equals a golden that chase/model/dart_golden_test.go writes with
/// json.Marshal.
///
/// Two differences are inherent, and neither changes what Go decodes:
///   * For the non-omitempty `sections` and `outline`, Go writes `null` for a
///     nil slice and `[]` for an empty one. Dart has one empty list and writes
///     `[]`; Go decodes both to a zero-length slice.
///   * Go's encoder escapes `<`, `>` and `&` in strings (`<`, ...);
///     `jsonEncode` does not. The decoded strings are identical.
library;

/// Mirrors `model.Meta`: `{"directives": {...}}`.
class Meta {
  const Meta({required this.directives});

  final Map<String, String> directives;

  factory Meta.fromJson(Map<String, dynamic> json) {
    final directivesJson = json['directives'];
    return Meta(
      directives: directivesJson is Map
          ? directivesJson.map(
              (key, value) => MapEntry(key as String, value as String),
            )
          : const <String, String>{},
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    if (directives.isNotEmpty) 'directives': _goSorted(directives),
  };
}

/// Mirrors `model.Document`: the schema-v7 document carried in
/// `press.Output.Model` and served by `--format json`.
class ModelDocument {
  const ModelDocument({
    required this.schemaVersion,
    required this.meta,
    required this.sections,
    required this.outline,
    this.properties = const <PropertyDef>[],
    this.propertyValues = const <String, String>{},
    this.comments = const <Comment>[],
  });

  /// The wire shape this binding understands.
  ///
  /// Moved v3 -> v4 with AODex Objective 16 (`span`); v4 -> v5 with the AI
  /// doc-ops spike (optional block `id`); v5 -> v6 (children, toggle/synced);
  /// v6 -> v7 (runs, todo/callout/divider/file/sheet/figure, sheet embeds,
  /// page properties, comments). Each step is a STRICT SUPERSET of the one
  /// before -- no field was removed, renamed or re-classified -- so this
  /// binding parses an older document correctly.
  ///
  /// The `span` offsets are BYTES. Dart strings are UTF-16, so any code that
  /// indexes a source string with one MUST convert first; see [Span].
  static const expectedSchemaVersion = 'eden-press.model/v7';

  final String schemaVersion;
  final Meta meta;
  final List<Section> sections;
  final List<OutlineEntry> outline;

  /// The typed page-property schema (schema v7).
  final List<PropertyDef> properties;

  /// Property id -> string value (schema v7). multi_select and relation
  /// values may be JSON-encoded lists inside the string.
  final Map<String, String> propertyValues;

  /// Review annotations anchored to blocks (schema v7).
  final List<Comment> comments;

  factory ModelDocument.fromJson(Map<String, dynamic> json) {
    final metaJson = json['meta'];
    final sectionsJson = json['sections'];
    final outlineJson = json['outline'];
    final propertiesJson = json['properties'];
    final valuesJson = json['propertyValues'];
    final commentsJson = json['comments'];
    return ModelDocument(
      schemaVersion: json['schemaVersion'] as String? ?? '',
      meta: metaJson is Map<String, dynamic>
          ? Meta.fromJson(metaJson)
          : const Meta(directives: {}),
      sections: sectionsJson is List
          ? sectionsJson
                .map((e) => Section.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <Section>[],
      outline: outlineJson is List
          ? outlineJson
                .map((e) => OutlineEntry.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <OutlineEntry>[],
      properties: propertiesJson is List
          ? propertiesJson
                .map((e) => PropertyDef.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <PropertyDef>[],
      propertyValues: _stringMap(valuesJson),
      comments: commentsJson is List
          ? commentsJson
                .map((e) => Comment.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <Comment>[],
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    'schemaVersion': schemaVersion,
    'meta': meta.toJson(),
    'sections': [for (final s in sections) s.toJson()],
    'outline': [for (final o in outline) o.toJson()],
    if (properties.isNotEmpty)
      'properties': [for (final p in properties) p.toJson()],
    if (propertyValues.isNotEmpty) 'propertyValues': _goSorted(propertyValues),
    if (comments.isNotEmpty) 'comments': [for (final c in comments) c.toJson()],
  };
}

/// Mirrors `model.Section`.
class Section {
  const Section({
    required this.id,
    required this.attrs,
    required this.notes,
    required this.blocks,
    this.span,
  });

  final int id;
  final Map<String, String> attrs;
  final List<String> notes;
  final List<Block> blocks;

  /// Byte range of the section's content in the Markdown source (schema v4),
  /// or null when it has no positioned content. See [Span].
  final Span? span;

  factory Section.fromJson(Map<String, dynamic> json) {
    final attrsJson = json['attrs'];
    final notesJson = json['notes'];
    final blocksJson = json['blocks'];
    return Section(
      id: json['id'] as int? ?? 0,
      attrs: attrsJson is Map
          ? attrsJson.map(
              (key, value) => MapEntry(key as String, value as String),
            )
          : const <String, String>{},
      notes: notesJson is List
          ? notesJson.map((e) => e as String).toList()
          : const <String>[],
      blocks: blocksJson is List
          ? blocksJson
                .map((e) => Block.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <Block>[],
      span: _spanFrom(json['span']),
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    'id': id,
    if (attrs.isNotEmpty) 'attrs': _goSorted(attrs),
    if (notes.isNotEmpty) 'notes': [...notes],
    if (blocks.isNotEmpty) 'blocks': [for (final b in blocks) b.toJson()],
    if (span != null) 'span': span!.toJson(),
  };
}

/// Mirrors `model.Span`: a half-open BYTE range `[start, stop)` into the
/// Markdown source that produced the node carrying it.
///
/// OFFSETS ARE BYTES, not UTF-16 code units. Dart strings are UTF-16, so
/// convert before indexing a source string, or every position after the first
/// multi-byte character is wrong.
///
/// Both fields are always written, because Go's are not omitempty: a span that
/// starts at 0 (the first block of every document) must survive. "No
/// position" is a null span on the owner, never a fabricated `{0, 0}`.
class Span {
  const Span({required this.start, required this.stop});

  final int start;
  final int stop;

  factory Span.fromJson(Map<String, dynamic> json) => Span(
    start: (json['start'] as num?)?.toInt() ?? 0,
    stop: (json['stop'] as num?)?.toInt() ?? 0,
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    'start': start,
    'stop': stop,
  };
}

/// Mirrors the `model.BlockKind` string enum. [wire] is the Go string.
enum BlockKind {
  paragraph('paragraph'),
  list('list'),
  code('code'),
  math('math'),
  heading('heading'),
  table('table'),
  image('image'),
  quote('quote'),
  toggle('toggle'),
  synced('synced'),
  todo('todo'),
  callout('callout'),
  divider('divider'),
  file('file'),
  sheet('sheet'),
  figure('figure'),

  /// A kind this binding does not know (a newer schema). The block keeps the
  /// original string in [Block.unknownKind], so it survives a round trip.
  unknown('');

  const BlockKind(this.wire);

  /// The `model.BlockKind` value written on the wire.
  final String wire;

  static final Map<String, BlockKind> _byWire = <String, BlockKind>{
    for (final kind in values)
      if (kind != unknown) kind.wire: kind,
  };

  /// The kind whose [wire] is [value], or [unknown].
  static BlockKind fromWire(String value) => _byWire[value] ?? unknown;
}

/// Mirrors `model.TextRun` (schema v7): one rich-text span. [marks] are wire
/// strings such as `bold`, `italic`, `code`, `strike`; [link] is an href.
class TextRun {
  const TextRun({
    required this.text,
    this.marks = const <String>[],
    this.link = '',
  });

  final String text;
  final List<String> marks;
  final String link;

  factory TextRun.fromJson(Map<String, dynamic> json) => TextRun(
    text: json['text'] as String? ?? '',
    marks: _stringList(json['marks']),
    link: json['link'] as String? ?? '',
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    'text': text,
    if (marks.isNotEmpty) 'marks': [...marks],
    if (link.isNotEmpty) 'link': link,
  };
}

/// Mirrors `model.SyncRef` (schema v6): the canonical block a `synced`
/// instance reads through. An empty [pageId] means the same document.
class SyncRef {
  const SyncRef({required this.blockId, this.pageId = ''});

  final String blockId;
  final String pageId;

  factory SyncRef.fromJson(Map<String, dynamic> json) => SyncRef(
    blockId: json['blockId']?.toString() ?? '',
    pageId: json['pageId']?.toString() ?? '',
  );

  /// Typed `Map<String, String>` so it can be passed straight to
  /// `Block(syncRef: ...)`, whose field keeps its original map type.
  Map<String, String> toJson() => <String, String>{
    'blockId': blockId,
    if (pageId.isNotEmpty) 'pageId': pageId,
  };
}

/// Mirrors `model.SheetRange`: a rectangular or id-stable subset of a sheet.
/// Every field is omitempty in Go, so a 0 row or column is not written.
class SheetRange {
  const SheetRange({
    this.r0 = 0,
    this.r1 = 0,
    this.c0 = 0,
    this.c1 = 0,
    this.columnIds = const <String>[],
    this.rowIds = const <String>[],
    this.rangeId = '',
  });

  final int r0;
  final int r1;
  final int c0;
  final int c1;
  final List<String> columnIds;
  final List<String> rowIds;
  final String rangeId;

  factory SheetRange.fromJson(Map<String, dynamic> json) => SheetRange(
    r0: (json['r0'] as num?)?.toInt() ?? 0,
    r1: (json['r1'] as num?)?.toInt() ?? 0,
    c0: (json['c0'] as num?)?.toInt() ?? 0,
    c1: (json['c1'] as num?)?.toInt() ?? 0,
    columnIds: _stringList(json['columnIds']),
    rowIds: _stringList(json['rowIds']),
    rangeId: json['rangeId'] as String? ?? '',
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    if (r0 != 0) 'r0': r0,
    if (r1 != 0) 'r1': r1,
    if (c0 != 0) 'c0': c0,
    if (c1 != 0) 'c1': c1,
    if (columnIds.isNotEmpty) 'columnIds': [...columnIds],
    if (rowIds.isNotEmpty) 'rowIds': [...rowIds],
    if (rangeId.isNotEmpty) 'rangeId': rangeId,
  };
}

/// Mirrors `model.SheetColumnSnap`: one workbook column in a snapshot.
class SheetColumn {
  const SheetColumn({
    required this.id,
    required this.name,
    required this.type,
    this.options = const <String>[],
  });

  final String id;
  final String name;
  final String type;
  final List<String> options;

  factory SheetColumn.fromJson(Map<String, dynamic> json) => SheetColumn(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    type: json['type'] as String? ?? '',
    options: _stringList(json['options']),
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    'id': id,
    'name': name,
    'type': type,
    if (options.isNotEmpty) 'options': [...options],
  };
}

/// Mirrors `model.SheetRowIDSnap`: the stable id of snapshot row [r].
class SheetRowId {
  const SheetRowId({required this.r, required this.id});

  final int r;
  final String id;

  factory SheetRowId.fromJson(Map<String, dynamic> json) => SheetRowId(
    r: (json['r'] as num?)?.toInt() ?? 0,
    id: json['id'] as String? ?? '',
  );

  Map<String, dynamic> toJson() => <String, dynamic>{'r': r, 'id': id};
}

/// Mirrors `model.SheetCellSnap`: one sparse snapshot cell. [v] is the
/// displayed value and [f] the formula (without `=`); either may be empty.
/// [rels] are relation links, each a string map such as `{pageId, title}`.
class SheetCell {
  const SheetCell({
    required this.r,
    required this.c,
    this.v = '',
    this.f = '',
    this.rels = const <Map<String, String>>[],
  });

  final int r;
  final int c;
  final String v;
  final String f;
  final List<Map<String, String>> rels;

  factory SheetCell.fromJson(Map<String, dynamic> json) {
    final relsJson = json['rels'];
    return SheetCell(
      r: (json['r'] as num?)?.toInt() ?? 0,
      c: (json['c'] as num?)?.toInt() ?? 0,
      v: json['v']?.toString() ?? '',
      f: json['f']?.toString() ?? '',
      rels: relsJson is List
          ? relsJson.map((rel) => _stringMap(rel)).toList()
          : const <Map<String, String>>[],
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    'r': r,
    'c': c,
    if (v.isNotEmpty) 'v': v,
    if (f.isNotEmpty) 'f': f,
    if (rels.isNotEmpty) 'rels': [for (final rel in rels) _goSorted(rel)],
  };
}

/// Mirrors `model.SheetSnapshot`: the sparse cell subset an embed carries for
/// offline render.
class SheetSnapshot {
  const SheetSnapshot({
    this.columns = const <SheetColumn>[],
    this.rowIds = const <SheetRowId>[],
    this.cells = const <SheetCell>[],
  });

  final List<SheetColumn> columns;
  final List<SheetRowId> rowIds;
  final List<SheetCell> cells;

  factory SheetSnapshot.fromJson(Map<String, dynamic> json) {
    final columnsJson = json['columns'];
    final rowIdsJson = json['rowIds'];
    final cellsJson = json['cells'];
    return SheetSnapshot(
      columns: columnsJson is List
          ? columnsJson.map((e) => SheetColumn.fromJson(_jsonMap(e))).toList()
          : const <SheetColumn>[],
      rowIds: rowIdsJson is List
          ? rowIdsJson.map((e) => SheetRowId.fromJson(_jsonMap(e))).toList()
          : const <SheetRowId>[],
      cells: cellsJson is List
          ? cellsJson.map((e) => SheetCell.fromJson(_jsonMap(e))).toList()
          : const <SheetCell>[],
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    if (columns.isNotEmpty) 'columns': [for (final c in columns) c.toJson()],
    if (rowIds.isNotEmpty) 'rowIds': [for (final r in rowIds) r.toJson()],
    if (cells.isNotEmpty) 'cells': [for (final c in cells) c.toJson()],
  };
}

/// Mirrors `model.SheetEmbed` (schema v7, kind `sheet`): a live reference to
/// a workbook sheet or range, plus an optional snapshot for offline render.
class SheetEmbed {
  const SheetEmbed({
    this.pageId = '',
    this.workbookHint = '',
    this.sheet = '',
    this.range,
    this.snapshot,
  });

  final String pageId;
  final String workbookHint;
  final String sheet;
  final SheetRange? range;
  final SheetSnapshot? snapshot;

  factory SheetEmbed.fromJson(Map<String, dynamic> json) {
    final rangeJson = json['range'];
    final snapshotJson = json['snapshot'];
    return SheetEmbed(
      pageId: json['pageId']?.toString() ?? '',
      workbookHint: json['workbookHint']?.toString() ?? '',
      sheet: json['sheet']?.toString() ?? '',
      range: rangeJson is Map ? SheetRange.fromJson(_jsonMap(rangeJson)) : null,
      snapshot: snapshotJson is Map
          ? SheetSnapshot.fromJson(_jsonMap(snapshotJson))
          : null,
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    if (pageId.isNotEmpty) 'pageId': pageId,
    if (workbookHint.isNotEmpty) 'workbookHint': workbookHint,
    if (sheet.isNotEmpty) 'sheet': sheet,
    if (range != null) 'range': range!.toJson(),
    if (snapshot != null) 'snapshot': snapshot!.toJson(),
  };
}

/// Mirrors `model.PropertyDef` (schema v7): one typed page-property schema
/// entry. [type] is a `model.PropertyType` string: `text`, `number`,
/// `select`, `multi_select`, `date`, `checkbox`, `url`, `email`, `relation`.
class PropertyDef {
  const PropertyDef({
    required this.id,
    required this.name,
    required this.type,
    this.options = const <String>[],
  });

  final String id;
  final String name;
  final String type;
  final List<String> options;

  factory PropertyDef.fromJson(Map<String, dynamic> json) => PropertyDef(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    type: json['type'] as String? ?? '',
    options: _stringList(json['options']),
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    'id': id,
    'name': name,
    'type': type,
    if (options.isNotEmpty) 'options': [...options],
  };
}

/// Mirrors `model.Comment` (schema v7): a review annotation anchored to a
/// block. [start] and [stop] are pointers in Go, so 0 is a real offset and
/// null means "the whole block".
class Comment {
  const Comment({
    required this.id,
    required this.blockId,
    this.start,
    this.stop,
    required this.body,
    this.resolved = false,
    this.suggestion,
  });

  final String id;
  final String blockId;
  final int? start;
  final int? stop;
  final String body;
  final bool resolved;
  final CommentSuggestion? suggestion;

  factory Comment.fromJson(Map<String, dynamic> json) {
    final suggestionJson = json['suggestion'];
    return Comment(
      id: json['id'] as String? ?? '',
      blockId: json['blockId'] as String? ?? '',
      start: (json['start'] as num?)?.toInt(),
      stop: (json['stop'] as num?)?.toInt(),
      body: json['body'] as String? ?? '',
      resolved: json['resolved'] as bool? ?? false,
      suggestion: suggestionJson is Map
          ? CommentSuggestion.fromJson(_jsonMap(suggestionJson))
          : null,
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    'id': id,
    'blockId': blockId,
    if (start != null) 'start': start,
    if (stop != null) 'stop': stop,
    'body': body,
    if (resolved) 'resolved': true,
    if (suggestion != null) 'suggestion': suggestion!.toJson(),
  };
}

/// Mirrors `model.CommentSuggestion`: an optional proposed replacement.
class CommentSuggestion {
  const CommentSuggestion({this.text = '', this.runs = const <TextRun>[]});

  final String text;
  final List<TextRun> runs;

  factory CommentSuggestion.fromJson(Map<String, dynamic> json) {
    final runsJson = json['runs'];
    return CommentSuggestion(
      text: json['text'] as String? ?? '',
      runs: runsJson is List
          ? runsJson.map((e) => TextRun.fromJson(_jsonMap(e))).toList()
          : const <TextRun>[],
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
    if (text.isNotEmpty) 'text': text,
    if (runs.isNotEmpty) 'runs': [for (final r in runs) r.toJson()],
  };
}

/// Mirrors `model.Block`. [text] carries raw TeX for `math` blocks and raw
/// (pre-highlight) source for `code` blocks -- never rendered HTML.
/// [language] is the fenced info-string for `code` blocks. [display]
/// distinguishes display-mode (`$$`) vs inline (`$`) math.
class Block {
  const Block({
    this.id = '',
    required this.kind,
    this.unknownKind = '',
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
    this.runs = const <TextRun>[],
    this.checked = false,
    this.icon = '',
    this.tone = '',
    this.name = '',
    this.mime = '',
    this.size = 0,
    this.svg = '',
    this.sheetEmbed,
    this.span,
  });

  /// Stable block id for apply_ops (schema v5+). Empty on Build()-produced
  /// Markdown imports until an editor/agent assigns one.
  final String id;

  final BlockKind kind;

  /// The wire kind string when [kind] is [BlockKind.unknown] (a kind from a
  /// newer schema). [toJson] writes it back, so the block is not corrupted by
  /// a round trip through this binding. Ignored for every known kind.
  final String unknownKind;

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

  /// Synced-block source ref (schema v6), as its wire map. [typedSyncRef] is
  /// the typed view; the map type is kept so existing callers compile.
  final Map<String, String>? syncRef;

  /// Rich text runs (schema v7); [text] remains the plain fallback.
  final List<TextRun> runs;

  final bool checked;
  final String icon;
  final String tone;
  final String name;
  final String mime;
  final int size;
  final String svg;

  /// Sheet embed (schema v7, kind `sheet`), as its wire map. [typedSheetEmbed]
  /// is the typed view; build one with `SheetEmbed(...).toJson()`. The map type
  /// is kept so existing callers compile.
  final Map<String, dynamic>? sheetEmbed;

  /// Byte range of the Markdown source this block came from (schema v4), or
  /// null when unknown. See [Span].
  final Span? span;

  /// Typed view of [syncRef], or null.
  SyncRef? get typedSyncRef {
    final ref = syncRef;
    return ref == null ? null : SyncRef.fromJson(ref);
  }

  /// Typed view of [sheetEmbed] (range, snapshot columns, row ids and cells
  /// with their formulas), or null.
  SheetEmbed? get typedSheetEmbed {
    final embed = sheetEmbed;
    return embed == null ? null : SheetEmbed.fromJson(embed);
  }

  factory Block.fromJson(Map<String, dynamic> json) {
    final itemsJson = json['items'];
    final childrenJson = json['children'];
    final sync = json['syncRef'];
    final runsJson = json['runs'];
    final embed = json['sheetEmbed'];
    final wireKind = json['kind'] as String? ?? '';
    final kind = BlockKind.fromWire(wireKind);
    return Block(
      id: json['id'] as String? ?? '',
      kind: kind,
      unknownKind: kind == BlockKind.unknown ? wireKind : '',
      text: json['text'] as String? ?? '',
      level: json['level'] as int? ?? 0,
      language: json['language'] as String? ?? '',
      display: json['display'] as bool? ?? false,
      ordered: json['ordered'] as bool? ?? false,
      items: itemsJson is List
          ? itemsJson
                .map((e) => ListItem.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <ListItem>[],
      headers: _stringList(json['headers']),
      rows: json['rows'] is List
          ? (json['rows'] as List)
                .map<List<String>>((r) => _stringList(r))
                .toList()
          : const <List<String>>[],
      aligns: _stringList(json['aligns']),
      src: json['src'] as String? ?? '',
      title: json['title'] as String? ?? '',
      children: childrenJson is List
          ? childrenJson
                .map((e) => Block.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <Block>[],
      collapsed: json['collapsed'] as bool? ?? false,
      syncRef: sync is Map
          ? sync.map((k, v) => MapEntry(k.toString(), v?.toString() ?? ''))
          : null,
      runs: runsJson is List
          ? runsJson
                .map((e) => TextRun.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <TextRun>[],
      checked: json['checked'] as bool? ?? false,
      icon: json['icon'] as String? ?? '',
      tone: json['tone'] as String? ?? '',
      name: json['name'] as String? ?? '',
      mime: json['mime'] as String? ?? '',
      size: (json['size'] as num?)?.toInt() ?? 0,
      svg: json['svg'] as String? ?? '',
      sheetEmbed: embed is Map ? Map<String, dynamic>.from(embed) : null,
      span: _spanFrom(json['span']),
    );
  }

  /// Go `model.Block` field order and omitempty, field by field. [syncRef]
  /// and [sheetEmbed] are written through their typed classes, so a map built
  /// by hand (say with an empty `pageId`) is normalized the way Go would
  /// marshal the struct.
  Map<String, dynamic> toJson() => <String, dynamic>{
    if (id.isNotEmpty) 'id': id,
    'kind': kind == BlockKind.unknown ? unknownKind : kind.wire,
    if (text.isNotEmpty) 'text': text,
    if (runs.isNotEmpty) 'runs': [for (final r in runs) r.toJson()],
    if (checked) 'checked': true,
    if (icon.isNotEmpty) 'icon': icon,
    if (tone.isNotEmpty) 'tone': tone,
    if (name.isNotEmpty) 'name': name,
    if (mime.isNotEmpty) 'mime': mime,
    if (size != 0) 'size': size,
    if (svg.isNotEmpty) 'svg': svg,
    if (typedSheetEmbed case final embed?) 'sheetEmbed': embed.toJson(),
    if (level != 0) 'level': level,
    if (language.isNotEmpty) 'language': language,
    if (display) 'display': true,
    if (ordered) 'ordered': true,
    if (items.isNotEmpty) 'items': [for (final i in items) i.toJson()],
    if (headers.isNotEmpty) 'headers': [...headers],
    if (rows.isNotEmpty)
      'rows': [
        for (final r in rows) [...r],
      ],
    if (aligns.isNotEmpty) 'aligns': [...aligns],
    if (src.isNotEmpty) 'src': src,
    if (title.isNotEmpty) 'title': title,
    if (children.isNotEmpty) 'children': [for (final c in children) c.toJson()],
    if (collapsed) 'collapsed': true,
    if (typedSyncRef case final ref?) 'syncRef': ref.toJson(),
    if (span != null) 'span': span!.toJson(),
  };
}

/// Mirrors `model.ListItem`: one entry of a list block, with its 0-based
/// nesting [level].
class ListItem {
  const ListItem({
    this.id = '',
    required this.text,
    required this.level,
    this.runs = const <TextRun>[],
  });

  final String id;
  final String text;
  final int level;
  final List<TextRun> runs;

  factory ListItem.fromJson(Map<String, dynamic> json) {
    final runsJson = json['runs'];
    return ListItem(
      id: json['id'] as String? ?? '',
      text: json['text'] as String? ?? '',
      level: json['level'] as int? ?? 0,
      runs: runsJson is List
          ? runsJson
                .map((e) => TextRun.fromJson(e as Map<String, dynamic>))
                .toList()
          : const <TextRun>[],
    );
  }

  /// `text` has no omitempty in Go, so an empty item still writes it.
  Map<String, dynamic> toJson() => <String, dynamic>{
    if (id.isNotEmpty) 'id': id,
    'text': text,
    if (runs.isNotEmpty) 'runs': [for (final r in runs) r.toJson()],
    if (level != 0) 'level': level,
  };
}

/// Mirrors `model.OutlineEntry`: one heading of the document outline.
class OutlineEntry {
  const OutlineEntry({
    required this.sectionId,
    required this.level,
    required this.text,
    required this.slug,
    this.span,
  });

  final int sectionId;
  final int level;
  final String text;
  final String slug;

  /// Byte range of the heading text in the Markdown source, or null.
  final Span? span;

  factory OutlineEntry.fromJson(Map<String, dynamic> json) => OutlineEntry(
    sectionId: json['sectionId'] as int? ?? 0,
    level: json['level'] as int? ?? 0,
    text: json['text'] as String? ?? '',
    slug: json['slug'] as String? ?? '',
    span: _spanFrom(json['span']),
  );

  /// None of these Go fields is omitempty except `span`.
  Map<String, dynamic> toJson() => <String, dynamic>{
    'sectionId': sectionId,
    'level': level,
    'text': text,
    'slug': slug,
    if (span != null) 'span': span!.toJson(),
  };
}

/// Decodes a wire array of strings, tolerating a null/absent field (every
/// schema-v3+ addition is `omitempty` on the Go side, so an unused one is
/// simply not present) and a non-string entry.
List<String> _stringList(Object? value) {
  if (value is! List) {
    return const <String>[];
  }
  return value.map((e) => e?.toString() ?? '').toList();
}

/// Decodes a wire object of strings (Go `map[string]string`), or an empty map.
Map<String, String> _stringMap(Object? value) {
  if (value is! Map) {
    return const <String, String>{};
  }
  return value.map((k, v) => MapEntry(k.toString(), v?.toString() ?? ''));
}

/// A nested wire object as `Map<String, dynamic>`, whether it came from
/// `jsonDecode` or from a hand-built literal with narrower type arguments.
Map<String, dynamic> _jsonMap(Object? value) {
  if (value is Map<String, dynamic>) return value;
  if (value is Map) return Map<String, dynamic>.from(value);
  return const <String, dynamic>{};
}

Span? _spanFrom(Object? value) =>
    value is Map ? Span.fromJson(_jsonMap(value)) : null;

/// [m] with its keys in the order Go's encoder writes a map: sorted by byte
/// value of their UTF-8 encoding, which is code-point order. (Plain
/// [String.compareTo] compares UTF-16 code units, which disagrees for
/// characters beyond the Basic Multilingual Plane.)
Map<String, String> _goSorted(Map<String, String> m) {
  final keys = m.keys.toList()..sort(_compareCodePoints);
  return <String, String>{for (final k in keys) k: m[k]!};
}

int _compareCodePoints(String a, String b) {
  final ia = a.runes.iterator;
  final ib = b.runes.iterator;
  while (true) {
    final hasA = ia.moveNext();
    final hasB = ib.moveNext();
    if (!hasA || !hasB) {
      if (hasA == hasB) return 0;
      return hasA ? 1 : -1;
    }
    final diff = ia.current - ib.current;
    if (diff != 0) return diff;
  }
}
