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

import 'package:eden_press/eden_press.dart';

/// Which editor an embeddable [EdenView] opens. These are the three document
/// kinds the surface is built to host: a flowing document, a slide deck, and
/// a spreadsheet.
enum EdenDocumentKind { document, slides, spreadsheet }

/// Picks a kind from document directives, then from shape.
///
/// `profile` wins (`sheet`/`spreadsheet`, `paged`/`article`, `slides`).
/// A Marp deck is slides. A document that is only headings and tables is a
/// spreadsheet. Several sections with no other signal are slides. A single
/// prose section is a document.
EdenDocumentKind inferEdenDocumentKind(ModelDocument document) {
  final profile = (document.meta.directives['profile'] ?? '').toLowerCase();
  switch (profile) {
    case 'sheet':
    case 'spreadsheet':
      return EdenDocumentKind.spreadsheet;
    case 'paged':
    case 'article':
      return EdenDocumentKind.document;
    case 'slides':
      return EdenDocumentKind.slides;
  }
  if ((document.meta.directives['marp'] ?? '').toLowerCase() == 'true') {
    return EdenDocumentKind.slides;
  }
  if (_tableOnly(document)) {
    return EdenDocumentKind.spreadsheet;
  }
  if (document.sections.length > 1) {
    return EdenDocumentKind.slides;
  }
  return EdenDocumentKind.document;
}

bool _tableOnly(ModelDocument document) {
  var tables = 0;
  for (final section in document.sections) {
    for (final block in section.blocks) {
      switch (block.kind) {
        case BlockKind.table:
          tables++;
        case BlockKind.heading:
        case BlockKind.divider:
          // Structural chrome — allowed beside tables.
          break;
        case BlockKind.sheet:
        case BlockKind.figure:
        case BlockKind.file:
        case BlockKind.synced:
          // Sheet embeds / figures / files are never a pure table workbook.
          return false;
        case BlockKind.paragraph:
        case BlockKind.list:
        case BlockKind.code:
        case BlockKind.math:
        case BlockKind.image:
        case BlockKind.quote:
        case BlockKind.toggle:
        case BlockKind.todo:
        case BlockKind.callout:
        case BlockKind.unknown:
          if (block.text.trim().isEmpty &&
              block.items.isEmpty &&
              block.rows.isEmpty &&
              block.children.isEmpty &&
              block.runs.isEmpty) {
            break;
          }
          return false;
      }
    }
  }
  return tables > 0;
}
