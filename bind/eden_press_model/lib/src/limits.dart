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

/// The deepest a block may nest: a section's own blocks are at depth 1, their
/// children at depth 2, and so on; a list item at `level` L in a list at depth
/// d counts as depth d+L.
///
/// The same number as Go's `model.MaxNestingDepth` (eden-press#29). The Go
/// byte APIs refuse documents deeper than this, and eden_docs paints anything
/// deeper flat, so a stored document from before the limit cannot hang the
/// view. test/limits_test.dart and Go's TestNesting_N11 both check it against
/// test/fixtures/limits.json.
const int maxNestingDepth = 32;

/// How many blocks eden_docs paints flattened below [maxNestingDepth] before
/// it shows a count of the rest instead.
///
/// A stored document from before the limit can hold thousands of blocks
/// below it, and a flattened run is laid out in one frame (it sits inside one
/// container), so painting all of them would freeze the view the way the
/// nesting did. The blocks are still in the document; only the view stops.
/// Dart only: the Go byte APIs refuse such a document outright.
const int maxFlattenedBlocks = 200;
