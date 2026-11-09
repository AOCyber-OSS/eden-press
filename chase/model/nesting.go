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

import (
	"fmt"
	"strings"

	"go.aocyber.ai/eden-press/chase/markdown"
)

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
//
// It is defined as markdown.MaxContainerNesting, the bound on goldmark's own
// parse, so the parse and the model can never disagree on the number.
const MaxNestingDepth = markdown.MaxContainerNesting

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
// list item, is deeper than MaxNestingDepth, and nil otherwise. It stops at the
// first level past the limit, so a hostile document costs no more than one
// MaxNestingDepth-deep walk of it.
//
// Every press byte API runs it on a document it decodes, and ApplyOps runs it
// on its input, so a stored document from before the limit is refused with a
// typed error rather than processed.
func ValidateNesting(d *Document) error {
	if d == nil {
		return nil
	}
	for si := range d.Sections {
		if p, depth := firstTooDeep(d.Sections[si].Blocks, 1); p != nil {
			return &NestingDepthError{Path: fmt.Sprintf("sections[%d].", si) + p.String(), Depth: depth}
		}
	}
	return nil
}

// nestPath is the route from a section's block list to one block or list item:
// steps[0] indexes the section's blocks, each later step a block's children,
// and item (when >= 0) the last block's items.
type nestPath struct {
	steps []int
	item  int
}

// String renders p as a JSON path below a section ("blocks[0].children[2]").
func (p *nestPath) String() string {
	var b strings.Builder
	for i, s := range p.steps {
		if i == 0 {
			fmt.Fprintf(&b, "blocks[%d]", s)
		} else {
			fmt.Fprintf(&b, ".children[%d]", s)
		}
	}
	if p.item >= 0 {
		fmt.Fprintf(&b, ".items[%d]", p.item)
	}
	return b.String()
}

// firstTooDeep finds the first block (depth-first) or list item in bs, whose
// blocks are at depth, that is deeper than MaxNestingDepth, and returns its
// path and depth, or nil. It never descends below the limit.
func firstTooDeep(bs []Block, depth int) (*nestPath, int) {
	for i := range bs {
		if depth > MaxNestingDepth {
			return &nestPath{steps: []int{i}, item: -1}, depth
		}
		for j, it := range bs[i].Items {
			if depth+it.Level > MaxNestingDepth {
				return &nestPath{steps: []int{i}, item: j}, depth + it.Level
			}
		}
		if p, d := firstTooDeep(bs[i].Children, depth+1); p != nil {
			p.steps = append([]int{i}, p.steps...)
			return p, d
		}
	}
	return nil, 0
}

// height is how many levels b occupies counting itself: 1 for a leaf, one
// more per level of children, and 1+Level for a list item at Level. It stops
// counting once the result passes budget, so a hostile payload costs no more
// than budget levels of it.
func height(b *Block, budget int) int {
	h := 1
	for _, it := range b.Items {
		h = max(h, 1+it.Level)
	}
	for i := range b.Children {
		if h > budget {
			break
		}
		h = max(h, 1+height(&b.Children[i], budget-1))
	}
	return h
}

// checkFits returns a *NestingDepthError when b, placed so that b itself is
// at depth, would put a block or list item past MaxNestingDepth. what names the
// op and place for the error ("insert_block under b_7").
func checkFits(b *Block, depth int, what string) error {
	if d := depth - 1 + height(b, MaxNestingDepth-depth+1); d > MaxNestingDepth {
		return &NestingDepthError{Path: what, Depth: d}
	}
	return nil
}

// findBlockDepth is findBlockPtr that also reports the block's depth (1 for a
// section's own blocks), or (nil, 0).
func findBlockDepth(doc *Document, id string) (*Block, int) {
	var find func(bs []Block, depth int) (*Block, int)
	find = func(bs []Block, depth int) (*Block, int) {
		for i := range bs {
			if bs[i].ID == id {
				return &bs[i], depth
			}
			if p, d := find(bs[i].Children, depth+1); p != nil {
				return p, d
			}
		}
		return nil, 0
	}
	for si := range doc.Sections {
		if p, d := find(doc.Sections[si].Blocks, 1); p != nil {
			return p, d
		}
	}
	return nil, 0
}

// FlattenNesting rewrites d so ValidateNesting accepts it, with lenient
// FromMarkdown's mapping (flattenNesting): blocks below the deepest allowed
// container become its children in document order, and list item levels are
// lowered. No block is dropped. It is the repair for a stored document from
// before the limit, which the press byte APIs otherwise refuse.
func FlattenNesting(d *Document) {
	if d == nil {
		return
	}
	for si := range d.Sections {
		flattenNesting(d.Sections[si].Blocks, 1)
	}
}

// flattenNesting is lenient FromMarkdown's mapping for ConstructNestingDepth.
// Below each container at depth MaxNestingDepth-1 (the deepest whose children
// may stand), every block of the subtree becomes one of its children, in
// depth-first order and with no children of its own; and a list item's Level
// is lowered to MaxNestingDepth minus its list's depth. Content is moved, never
// dropped. One pass: linear in the size of the document.
func flattenNesting(bs []Block, depth int) {
	for i := range bs {
		b := &bs[i]
		for j := range b.Items {
			b.Items[j].Level = min(b.Items[j].Level, MaxNestingDepth-depth)
		}
		if depth < MaxNestingDepth-1 {
			flattenNesting(b.Children, depth+1)
			continue
		}
		if depth == MaxNestingDepth-1 && hasGrandchildren(b.Children) {
			var flat []Block
			collectFlat(b.Children, &flat)
			b.Children = flat
		}
		flattenNesting(b.Children, depth+1) // clamps the item levels of the flattened blocks
	}
}

func hasGrandchildren(bs []Block) bool {
	for i := range bs {
		if len(bs[i].Children) > 0 {
			return true
		}
	}
	return false
}

// collectFlat appends bs and all their descendants, depth-first, each without
// children, to out.
func collectFlat(bs []Block, out *[]Block) {
	for i := range bs {
		kids := bs[i].Children
		b := bs[i]
		b.Children = nil
		*out = append(*out, b)
		collectFlat(kids, out)
	}
}
