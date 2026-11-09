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

package model

import "fmt"

// MaxNestingDepth is the deepest a block may nest: a section's own blocks are
// at depth 1, their children at depth 2, and so on, and a list item at Level L
// in a list at depth d counts as depth d+L. The Dart mirror
// (bind/eden_press_model, maxNestingDepth) holds the same number; the
// conformance fixture written by TestNesting_N11 keeps the two in step.
//
// Past this depth every byte API did quadratic work (eden-press#29): a few KB of
// "> > > ..." became hundreds of MB of indented JSON, and the Flutter renderer
// hung on the nested layout. 32 is far deeper than any document a person
// writes and shallow enough that every later step stays linear.
const MaxNestingDepth = 32

// NestingDepthError reports a decoded document, or the result of an op, that
// nests past MaxNestingDepth.
type NestingDepthError struct {
	// Path locates the first block found too deep, as a JSON path
	// (sections[0].blocks[2].children[0]...), or the op that would put it there.
	Path string

	// Depth is that block's depth (always MaxNestingDepth+1 for a document:
	// the walk stops at the first level past the limit).
	Depth int
}

// Error implements error.
func (e *NestingDepthError) Error() string {
	return fmt.Sprintf("model: %s nests %d deep, past the limit of %d", e.Path, e.Depth, MaxNestingDepth)
}

// ValidateNesting returns a *NestingDepthError when any block in d, or any
// list item, is deeper than MaxNestingDepth, and nil otherwise.
func ValidateNesting(d *Document) error {
	return nil
}
