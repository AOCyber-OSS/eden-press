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

/// Native, JS-free shrink-to-fit auto-fit for Eden Press headings (RESOLVED
/// DECISION #4, the Flutter half): measures text with Flutter's
/// [TextPainter] and binary-searches the largest font size in
/// `[minFontSize, maxFontSize]` that lays out within a given
/// [BoxConstraints] without overflowing -- matching Marp's `<!--fit-->`
/// shrink-to-fit contract, not CSS fluid typography:
///
/// - **SHRINK-ONLY**: text that already fits at [computeFitFontSize]'s
///   `maxFontSize` keeps `maxFontSize` unchanged -- this never upscales past
///   the authored size.
/// - **Monotonic**: a wider [BoxConstraints] never yields a smaller fitted
///   size than a narrower one, for the same text.
///
/// This file imports nothing beyond `package:flutter/material.dart`: no JS
/// interop layer, no web-only package, no embedded browser surface --
/// [TextPainter] is pure Flutter painting (see render_surface.dart's library
/// doc for the sibling JS-free contract this file extends to auto-fit).
library;

import 'package:flutter/material.dart';

/// Number of binary-search iterations run when [text] overflows at
/// `maxFontSize`. 12 iterations over a realistic `[minFontSize,
/// maxFontSize]` span converges to well under a tenth of a logical pixel,
/// which is sub-pixel precision for on-screen text.
const int _fitSearchIterations = 12;

/// Measures [text] with [TextPainter.layout] and binary-searches the
/// largest font size in `[minFontSize, maxFontSize]` whose laid-out size
/// fits within [constraints] -- SHRINK-ONLY: if [text] already fits at
/// [maxFontSize], [maxFontSize] is returned unchanged (this never grows
/// past it).
///
/// [style] supplies every text attribute except font size (weight, family,
/// color, ...); the candidate size under test is applied via
/// [TextStyle.copyWith] on each measurement.
///
/// If [constraints].maxHeight is infinite (e.g. an unconstrained `Column`),
/// there is nothing to overflow vertically, so fitting falls back to
/// width-only -- the slide-width overflow is the real target in that case.
double computeFitFontSize(
  String text,
  BoxConstraints constraints, {
  required TextStyle style,
  double maxFontSize = 96,
  double minFontSize = 8,
}) {
  final double maxWidth = constraints.maxWidth;
  final double maxHeight = constraints.maxHeight;

  bool fits(double fontSize) {
    final painter = TextPainter(
      text: TextSpan(
        text: text,
        style: style.copyWith(fontSize: fontSize),
      ),
      textDirection: TextDirection.ltr,
      maxLines: null,
    )..layout(maxWidth: maxWidth);

    final heightFits = maxHeight.isInfinite || painter.height <= maxHeight;
    return heightFits &&
        painter.width <= maxWidth &&
        !painter.didExceedMaxLines;
  }

  // SHRINK-ONLY: already fits at the authored max -- never upscale past it.
  if (fits(maxFontSize)) return maxFontSize;

  double lo = minFontSize;
  double hi = maxFontSize;
  for (var i = 0; i < _fitSearchIterations; i++) {
    final mid = (lo + hi) / 2;
    if (fits(mid)) {
      lo = mid;
    } else {
      hi = mid;
    }
  }
  return lo;
}

/// A [StatelessWidget] that fits [text] to its ACTUAL allotted box at build
/// time: a [LayoutBuilder] supplies the real [BoxConstraints] Flutter has
/// given this widget, [computeFitFontSize] picks the largest non-overflowing
/// size (shrink-only against [style]'s font size), and a [Text] renders at
/// that fitted size -- the native, JS-free equivalent of Marp's
/// `<!--fit-->` heading shrink.
class FitText extends StatelessWidget {
  /// Creates a shrink-to-fit [Text] for [text].
  ///
  /// [maxFontSize] defaults to `style.fontSize` (falling back to `96` if
  /// [style] carries no font size) so the shrink-only ceiling is the
  /// AUTHORED size of the block being rendered -- e.g. a heading's level
  /// style -- not an unrelated arbitrary maximum.
  FitText(
    this.text, {
    super.key,
    required this.style,
    double? maxFontSize,
    this.minFontSize = 8,
  }) : maxFontSize = maxFontSize ?? style.fontSize ?? 96;

  /// The raw text to fit -- from `Block.text` (the schema-v2 model), never
  /// `Output.html`.
  final String text;

  /// The base style; every attribute other than font size (weight, family,
  /// color, ...) is honored as given. The fitted size replaces
  /// `style.fontSize` via [TextStyle.copyWith].
  final TextStyle style;

  /// The shrink-only ceiling passed to [computeFitFontSize].
  final double maxFontSize;

  /// The floor passed to [computeFitFontSize].
  final double minFontSize;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final fitted = computeFitFontSize(
          text,
          constraints,
          style: style,
          maxFontSize: maxFontSize,
          minFontSize: minFontSize,
        );
        return Text(text, style: style.copyWith(fontSize: fitted));
      },
    );
  }
}
