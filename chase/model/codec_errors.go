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

// UnrepresentableError reports a Markdown construct the v7 document model cannot
// carry exactly. Strict FromMarkdown returns it instead of dropping or altering
// the construct; lenient FromMarkdown (FromMarkdownOptions.Lenient) applies the
// mapping documented on the matching Construct constant and returns no error.
type UnrepresentableError struct {
	// Offset is the byte offset into the input at which the construct starts
	// (the same byte-offset convention as Span). The input is read after its
	// line endings are normalized to "\n", so for input that contains no "\r" it
	// is an offset into the original string.
	Offset int

	// Construct is a stable, machine-readable name for the construct; one of
	// the Construct* constants.
	Construct string

	// Detail is optional human-readable context (a count, a kind name).
	Detail string
}

// Error implements error.
func (e *UnrepresentableError) Error() string {
	msg := fmt.Sprintf("markdown %s at byte %d cannot be represented exactly in the document model", e.Construct, e.Offset)
	if e.Detail != "" {
		msg += " (" + e.Detail + ")"
	}
	return msg
}

// The constructs strict FromMarkdown refuses, each with the deterministic
// mapping lenient mode applies instead. Names are part of the codec's contract:
// callers (the AODex migration guard, the editor's Markdown view) switch on
// them.
const (
	// ConstructRawHTML is an HTML block, an inline HTML tag or an HTML comment
	// that is not a whole top-level comment (one with text after its closing
	// `-->`, or inside a quote, list or directive). A top-level comment is a
	// section note or attrs and is not refused. Lenient: a block becomes a
	// paragraph whose Text is the literal source lines; inline HTML becomes
	// literal text.
	ConstructRawHTML = "raw-html"

	// ConstructTableCellFormatting is a table cell holding anything but plain
	// text (emphasis, code, a link, an image, math). Table rows are plain text
	// in v7. Lenient: the cell is flattened to its plain text.
	ConstructTableCellFormatting = "table-cell-formatting"

	// ConstructListStart is an ordered list that does not start at 1; the
	// model has no start number. Lenient: the start is reset to 1.
	ConstructListStart = "list-start"

	// ConstructListMixedType is a nested list whose type (bullet or ordered)
	// differs from the outermost list's; the model has one type per list
	// block. Lenient: the outer list's type wins.
	ConstructListMixedType = "list-mixed-type"

	// ConstructListMixedTodo is a GFM task item (- [ ] / - [x]) anywhere a todo
	// block cannot stand for it: in a list that also holds plain items, nested
	// in a plain list, or in an ordered list. A bullet list made only of task
	// items is read as todo blocks (one per item). Lenient: the item stays a
	// list item and keeps its checkbox as literal text ("[ ] t", "[x] t").
	ConstructListMixedTodo = "list-mixed-todo"

	// ConstructLinkTitle is a link with a title attribute; TextRun.Link is an
	// href only. Lenient: the title is dropped.
	ConstructLinkTitle = "link-title"

	// ConstructEmptyLink is a link with no text or no destination. Lenient: an
	// empty-text link is dropped; a link with no destination is plain text.
	ConstructEmptyLink = "empty-link"

	// ConstructInlineImage is an image that shares its paragraph with other
	// content; only a paragraph holding nothing but one image is an image
	// block. Lenient: the image becomes its alt text.
	ConstructInlineImage = "inline-image"

	// ConstructCodeInfoString is a code fence whose info string has more than
	// one word; the model keeps the language only. Lenient: the first word is
	// the language and the rest is dropped.
	ConstructCodeInfoString = "code-info-string"

	// ConstructListItemBlocks is a list item holding anything but one
	// paragraph and nested lists (a second paragraph, a code block, a quote).
	// Lenient: the extra blocks' text is appended to the item's text after a
	// line break.
	ConstructListItemBlocks = "list-item-blocks"

	// ConstructInlineDisplayMath is $$...$$ sharing a paragraph with other
	// content; display math is a block. Lenient: it becomes inline math.
	ConstructInlineDisplayMath = "inline-display-math"

	// ConstructMathUnclosed is a $$ block with no closing $$. Lenient: the
	// math runs to the end of the enclosing block.
	ConstructMathUnclosed = "math-unclosed"

	// ConstructMathTrailingText is text after the closing $$ of a display math
	// block on the same line ("$$ tail"). Display math is a block, so the text
	// cannot share it. Lenient: the text becomes the paragraph that follows the
	// math block.
	ConstructMathTrailingText = "math-trailing-text"

	// ConstructFrontMatter is a leading --- block that does not parse as
	// key: value lines (including a duplicate key or a bad quoted value).
	// Lenient: unparseable lines are skipped and a later duplicate key wins.
	ConstructFrontMatter = "front-matter"

	// ConstructUnknownDirective is a container directive (":::name") whose name
	// is not one of toggle, synced, callout, file, sheet or figure. Lenient: the
	// whole directive, fences included, stays as literal paragraph text.
	ConstructUnknownDirective = "unknown-directive"

	// ConstructDirectiveJSON is a directive whose header is not one JSON object
	// of the known fields (bad JSON, trailing text, an unknown key, a wrong type),
	// or an eden:attrs / eden:note / eden:model comment whose payload is not valid
	// (including an eden:model version this package does not read and a second
	// eden:model comment). Lenient: a directive stays as literal paragraph text; a
	// comment stays as a section note, text unchanged.
	ConstructDirectiveJSON = "directive-json"

	// ConstructDirectiveUnclosed is a directive with no closing fence before the
	// end of its container. Lenient: it runs to the end of the container.
	ConstructDirectiveUnclosed = "directive-unclosed"

	// ConstructUnsupported is a Markdown node this codec version does not
	// read at all. Lenient: it is skipped.
	ConstructUnsupported = "unsupported"
)
