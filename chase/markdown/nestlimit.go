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

package markdown

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MaxContainerNesting is the deepest goldmark may nest block quotes and list
// items inside one another. It is the document model's nesting limit
// (model.MaxNestingDepth is defined as this constant, so the two cannot
// differ); it lives here because chase/model imports this package.
//
// goldmark's own parse is quadratic in the containers one line opens: it
// re-reads the line's offset for every block it opens on it, so twenty
// thousand "- " on one line took 10 s, and AODex accepts 400 KB of Markdown
// (eden-press#29). Bounding the parse from inside the parser keeps it in step
// with goldmark's own reading of the line (fences, HTML blocks, thematic
// breaks, indented code), which a lexical pre-scan could not.
const MaxContainerNesting = 32

// NestingLimitKey is set in the parser.Context, to the byte offset of the
// first container marker past MaxContainerNesting, when the parse declined to
// open one. That marker and the rest of its line are read as text instead.
var NestingLimitKey = parser.NewContextKey()

// NestingLimitedBlockParsers returns goldmark's default block parsers with
// the block-quote and list parsers wrapped so they decline to open a
// container nested MaxContainerNesting deep already. It panics if the default
// set no longer holds those two parsers: a goldmark upgrade that renamed them
// must fail loudly, not silently drop the bound.
func NestingLimitedBlockParsers() []util.PrioritizedValue {
	var out []util.PrioritizedValue
	wrapped := 0
	for _, pv := range parser.DefaultBlockParsers() {
		if bp, ok := pv.Value.(parser.BlockParser); ok {
			switch fmt.Sprintf("%T", bp) {
			case "*parser.blockquoteParser", "*parser.listParser":
				pv.Value = &nestingLimited{bp}
				wrapped++
			}
		}
		out = append(out, pv)
	}
	if wrapped != 2 {
		panic(fmt.Sprintf("markdown: found %d of goldmark's block-quote and list parsers to bound, want 2", wrapped))
	}
	return out
}

// NewNestingLimitedParser is parser.NewParser with goldmark's defaults, the
// container parsers bounded (NestingLimitedBlockParsers). Extensions add to it
// as they do to goldmark's default parser.
func NewNestingLimitedParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(NestingLimitedBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

// nestingLimited is a container block parser that declines to open a block
// under MaxContainerNesting containers. Declining leaves the marker to the
// parsers that follow, so it is read as paragraph text.
type nestingLimited struct{ parser.BlockParser }

func (n *nestingLimited) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if containerDepth(parent) >= MaxContainerNesting {
		if pc.Get(NestingLimitKey) == nil {
			_, seg := reader.Position()
			pc.Set(NestingLimitKey, seg.Start)
		}
		return nil, parser.NoChildren
	}
	return n.BlockParser.Open(parent, reader, pc)
}

// containerDepth counts the block quotes and list items that hold n, n
// included. It stops past the limit, so it costs at most MaxContainerNesting
// steps per call.
func containerDepth(n ast.Node) int {
	d := 0
	for ; n != nil && d <= MaxContainerNesting; n = n.Parent() {
		switch n.(type) {
		case *ast.Blockquote, *ast.ListItem:
			d++
		}
	}
	return d
}
