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

import 'package:eden_docs/eden_docs.dart';
import 'package:eden_press/eden_press.dart';
import 'package:flutter/material.dart';

import 'edit.dart';
import 'kind.dart';
import 'spreadsheet_view.dart';
import 'workbook.dart';

/// Embeddable editor for an Eden document, slide deck, or spreadsheet.
///
/// A host puts this in its own widget tree and keeps the bytes. Pass
/// [document] for Docs and Slides (the engine's `ModelDocument`) or
/// [workbook] for Sheets. [onEdit] receives spreadsheet changes; document
/// and slide editing beyond navigation is still the host's, because those
/// kinds today render the engine model rather than a second rich-text CRDT.
///
/// [kind] overrides inference. When it is null, a workbook opens as a
/// spreadsheet and a document is classified by [inferEdenDocumentKind].
class EdenView extends StatelessWidget {
  const EdenView({
    super.key,
    this.document,
    this.workbook,
    this.kind,
    this.onEdit,
  }) : assert(document != null || workbook != null);

  final ModelDocument? document;
  final EdenWorkbook? workbook;
  final EdenDocumentKind? kind;
  final ValueChanged<EdenEdit>? onEdit;

  EdenDocumentKind get _kind {
    if (kind != null) {
      return kind!;
    }
    if (workbook != null) {
      return EdenDocumentKind.spreadsheet;
    }
    return inferEdenDocumentKind(document!);
  }

  @override
  Widget build(BuildContext context) {
    switch (_kind) {
      case EdenDocumentKind.spreadsheet:
        final book = workbook ??
            (document == null
                ? EdenWorkbook()
                : EdenWorkbook.fromDocument(document!));
        return SpreadsheetView(workbook: book, onEdit: onEdit);
      case EdenDocumentKind.slides:
        return _SlidesView(document: document!);
      case EdenDocumentKind.document:
        return _DocumentView(document: document!);
    }
  }
}

class _DocumentView extends StatelessWidget {
  const _DocumentView({required this.document});

  final ModelDocument document;

  @override
  Widget build(BuildContext context) {
    // Docs paint through the standalone eden_docs package (real block
    // widgets). Slides keep EdenPressView + FitText below.
    return EdenDocsView(document: document);
  }
}

class _SlidesView extends StatefulWidget {
  const _SlidesView({required this.document});

  final ModelDocument document;

  @override
  State<_SlidesView> createState() => _SlidesViewState();
}

class _SlidesViewState extends State<_SlidesView> {
  int _index = 0;

  void _goTo(int index) {
    final sections = widget.document.sections;
    final count = sections.isEmpty ? 1 : sections.length;
    if (index < 0 || index >= count || index == _index) {
      return;
    }
    setState(() => _index = index);
  }

  @override
  Widget build(BuildContext context) {
    final sections = widget.document.sections;
    final count = sections.isEmpty ? 1 : sections.length;
    final safeIndex = _index.clamp(0, count - 1);
    final atFirst = safeIndex <= 0;
    final atLast = safeIndex >= count - 1;
    // One slide at a time (no PageView). Desktop mouse-drag on PageView was
    // unreliable and mid-animation paint left blank frames; prev/next own
    // the index and rebuild the visible slide immediately.
    return Column(
      children: [
        Expanded(
          child: KeyedSubtree(
            key: const Key('eden-slides'),
            child: _slide(
              sections.isEmpty
                  ? null
                  : _outputFor(widget.document, sections[safeIndex]),
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.all(8),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              IconButton(
                key: const Key('eden-slides-prev'),
                tooltip: atFirst ? 'First slide' : 'Previous slide',
                onPressed: atFirst ? null : () => _goTo(safeIndex - 1),
                icon: const Icon(Icons.chevron_left),
              ),
              Text('Slide ${safeIndex + 1} of $count'),
              IconButton(
                key: const Key('eden-slides-next'),
                tooltip: atLast ? 'Last slide' : 'Next slide',
                onPressed: atLast ? null : () => _goTo(safeIndex + 1),
                icon: const Icon(Icons.chevron_right),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _slide(Output? output) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: Colors.white,
          border: Border.all(color: const Color(0xFFE0E0E0)),
        ),
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: output == null
              ? const SizedBox.shrink()
              : EdenPressView(output),
        ),
      ),
    );
  }
}

Output _outputFor(ModelDocument document, Section section) {
  return Output(
    html: '',
    css: '',
    model: ModelDocument(
      schemaVersion: document.schemaVersion,
      meta: document.meta,
      sections: [section],
      outline: const [],
    ),
    meta: document.meta,
    comments: const [],
  );
}
