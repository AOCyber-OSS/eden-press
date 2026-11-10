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

import 'package:eden_press_model/eden_press_model.dart';
import 'package:flutter/material.dart';

import 'blocks.dart';

/// Flowing document surface for an Eden [ModelDocument].
///
/// Drop this into any Flutter app. Prefer feeding a model decoded from the
/// Go JSON envelope (`--format json` / `serve ?format=json`). HTML clients
/// should request HTML from Go instead of using this widget.
class EdenDocsView extends StatelessWidget {
  const EdenDocsView({
    super.key,
    required this.document,
    this.padding = const EdgeInsets.all(24),
    this.maxContentWidth = 720,
  });

  /// Builds from a wire JSON object. Accepts either the full lowercase
  /// envelope (`{ "model": {...} }`), a capitalized `Output` (`Model`), or a
  /// bare `ModelDocument` map.
  factory EdenDocsView.fromJson(
    Map<String, dynamic> json, {
    Key? key,
    EdgeInsetsGeometry padding = const EdgeInsets.all(24),
    double maxContentWidth = 720,
  }) {
    final dynamic nested = json['model'] ?? json['Model'] ?? json;
    final modelJson = nested is Map<String, dynamic>
        ? nested
        : <String, dynamic>{};
    return EdenDocsView(
      key: key,
      document: ModelDocument.fromJson(modelJson),
      padding: padding,
      maxContentWidth: maxContentWidth,
    );
  }

  final ModelDocument document;
  final EdgeInsetsGeometry padding;
  final double maxContentWidth;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth.isFinite
            ? constraints.maxWidth.clamp(0, maxContentWidth).toDouble()
            : maxContentWidth;
        return Align(
          alignment: Alignment.topCenter,
          child: SizedBox(
            width: width,
            child: ListView(
              key: const Key('eden-docs'),
              padding: padding,
              children: [
                for (final section in document.sections) ...[
                  for (final block in section.blocks) _block(block),
                  if (section != document.sections.last)
                    const SizedBox(height: 24),
                ],
              ],
            ),
          ),
        );
      },
    );
  }

  /// Paints [block] at [depth]; its children are painted one level deeper.
  Widget _block(Block block, [int depth = 1]) => buildDocBlock(
    block,
    depth: depth,
    nest: (child) => _block(child, depth + 1),
  );
}
