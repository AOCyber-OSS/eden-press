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
/// responsive without a server round trip.
sealed class EdenEdit {
  const EdenEdit();
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
}

/// A worksheet named [name] was added at the end of the workbook.
class EdenSheetAdded extends EdenEdit {
  const EdenSheetAdded(this.name);
  final String name;
}
