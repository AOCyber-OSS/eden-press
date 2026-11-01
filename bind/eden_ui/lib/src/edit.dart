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

/// One change an embedded editor reports to its host.
///
/// The host owns persistence. These are the operations a later collaboration
/// layer can ship; the view applies each one locally so the session stays
/// responsive without a server round trip. Wire names match Go
/// `model.Op` / `xlsx.WbOp` (see AGENTS.md). Call [toWire] (or [toWireOps]
/// for doc batches) to emit the JSON maps agents/`eden-press` apply.
sealed class EdenEdit {
  const EdenEdit();

  /// JSON-shaped op map matching the Go wire contract, or null when this
  /// edit is a multi-op batch ([EdenDocOpsEdit] — use [toWireOps]).
  Map<String, Object?>? toWire();
}

/// A cell was set to [value]. Formulas keep the leading `=`.
class EdenCellEdit extends EdenEdit {
  const EdenCellEdit({
    required this.sheet,
    required this.row,
    required this.column,
    required this.value,
  });

  final String sheet;
  final int row;
  final int column;
  final String value;

  @override
  Map<String, Object?> toWire() => {
    'op': 'set_cell',
    'sheet': sheet,
    'row': row,
    'column': column,
    'value': value,
  };
}

/// A worksheet named [name] was added at the end of the workbook.
class EdenSheetAdded extends EdenEdit {
  const EdenSheetAdded(this.name);
  final String name;

  @override
  Map<String, Object?> toWire() => {'op': 'add_sheet', 'sheet': name};
}

/// A worksheet was renamed.
class EdenSheetRenamed extends EdenEdit {
  const EdenSheetRenamed({required this.sheet, required this.newName});
  final String sheet;
  final String newName;

  @override
  Map<String, Object?> toWire() => {
    'op': 'rename_sheet',
    'sheet': sheet,
    'newName': newName,
  };
}

/// A worksheet was deleted.
class EdenSheetDeleted extends EdenEdit {
  const EdenSheetDeleted(this.sheet);
  final String sheet;

  @override
  Map<String, Object?> toWire() => {'op': 'delete_sheet', 'sheet': sheet};
}

/// A header-row flag changed.
class EdenHeaderRowEdit extends EdenEdit {
  const EdenHeaderRowEdit({
    required this.sheet,
    required this.row,
    required this.header,
  });
  final String sheet;
  final int row;
  final bool header;

  @override
  Map<String, Object?> toWire() => {
    'op': 'set_header_row',
    'sheet': sheet,
    'row': row,
    'header': header,
  };
}

/// A column alignment changed.
class EdenAlignEdit extends EdenEdit {
  const EdenAlignEdit({
    required this.sheet,
    required this.column,
    required this.align,
  });
  final String sheet;
  final int column;
  final String align;

  @override
  Map<String, Object?> toWire() => {
    'op': 'set_align',
    'sheet': sheet,
    'column': column,
    'align': align,
  };
}

/// A typed column property was set (workbook v3).
class EdenColumnEdit extends EdenEdit {
  const EdenColumnEdit({
    required this.sheet,
    required this.columnId,
    required this.name,
    required this.type,
  });
  final String sheet;
  final String columnId;
  final String name;
  final String type;

  @override
  Map<String, Object?> toWire() => {
    'op': 'set_column',
    'sheet': sheet,
    'columnId': columnId,
    'name': name,
    'type': type,
  };
}

/// Relation refs were set or cleared on a cell (workbook v3).
class EdenRelationEdit extends EdenEdit {
  const EdenRelationEdit({
    required this.sheet,
    this.rowId = '',
    this.row = 0,
    this.columnId = '',
    this.column = 0,
    this.rels = const <Map<String, String>>[],
  });
  final String sheet;
  final String rowId;
  final int row;
  final String columnId;
  final int column;
  final List<Map<String, String>> rels;

  @override
  Map<String, Object?> toWire() {
    final op = <String, Object?>{
      'op': rels.isEmpty ? 'clear_relation' : 'set_relation',
      'sheet': sheet,
    };
    if (rowId.isNotEmpty) {
      op['rowId'] = rowId;
    } else {
      op['row'] = row;
    }
    if (columnId.isNotEmpty) {
      op['columnId'] = columnId;
    } else {
      op['column'] = column;
    }
    if (rels.isNotEmpty) {
      op['rels'] = rels;
    }
    return op;
  }
}

/// A batch of docmodel ops (JSON-shaped maps matching Go `model.Op`).
class EdenDocOpsEdit extends EdenEdit {
  const EdenDocOpsEdit(this.ops);
  final List<Map<String, Object?>> ops;

  @override
  Map<String, Object?>? toWire() => null;

  /// Wire ops array for `eden-press doc apply --ops`.
  List<Map<String, Object?>> toWireOps() =>
      List<Map<String, Object?>>.from(ops);
}

/// Helpers that mirror Go `model.Op` wire shapes for Docs hosts.
class EdenDocOp {
  static Map<String, Object?> patchBlock(String blockId, Map<String, Object?> patch) =>
      {'op': 'patch_block', 'blockId': blockId, 'block': patch};

  static Map<String, Object?> replaceText({
    required String old,
    required String neu,
    String blockId = '',
    bool all = false,
  }) => {
    'op': 'replace_text',
    if (blockId.isNotEmpty) 'blockId': blockId,
    'old': old,
    'new': neu,
    if (all) 'all': true,
  };

  static Map<String, Object?> insertText({
    required String blockId,
    required int offset,
    required String text,
  }) => {
    'op': 'insert_text',
    'blockId': blockId,
    'offset': offset,
    'text': text,
  };

  static Map<String, Object?> setProperties(List<Map<String, Object?>> properties, {bool replace = false}) =>
      {
        'op': 'set_properties',
        'properties': properties,
        if (replace) 'replaceProperties': true,
      };

  static Map<String, Object?> setPropertyValue(String propertyId, String value) =>
      {'op': 'set_property_value', 'propertyId': propertyId, 'value': value};

  static Map<String, Object?> addComment(Map<String, Object?> comment) =>
      {'op': 'add_comment', 'comment': comment};
}
