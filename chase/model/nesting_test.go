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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Nesting depth limit (eden-press#29). Deep nesting made every byte API do
// quadratic work and the Flutter renderer hang; nothing limited it.
//
// Test list:
//
//	N1  strict FromMarkdown refuses a quote nested MaxNestingDepth+1 deep with
//	    ConstructNestingDepth, at an offset inside the input
//	N2  strict FromMarkdown accepts exactly MaxNestingDepth levels (boundary)
//	N3  lenient FromMarkdown of '>'x4900 returns quickly, no deeper than the
//	    limit, and keeps the text
//	N4  lenient flattening keeps every nested block's text, in document order
//	    (directive containers reach the tree flattening; quote lines past the
//	    limit are trimmed before the parse and keep their text in order)
//	N5  nested lists: strict refuses an item past the limit; lenient reads its
//	    marker as text of the item above, so the depth is the limit
//	N6  ValidateNesting: depth MaxNestingDepth is fine, one more is a
//	    *NestingDepthError naming the path; list levels count
//	N7  ValidateNesting on a 4900-deep document stops at the limit (fast)
//	N8  ApplyOps refuses insert_block / update_block / patch_block / move_block
//	    whose result would nest past the limit, and accepts the same op one
//	    level shallower
//	N9  ApplyOps refuses a document that is already too deep
//	N10 FromMarkdown + JSON grows linearly in depth up to the limit (bytes)
//	N11 MaxNestingDepth equals the conformance fixture the Dart mirror reads

// quoteChain builds depth quotes, each the only child of the one above, with
// text "lN" at level N, the innermost holding a paragraph "leaf". It is built
// from the bottom up, so the generator never recurses.
func quoteChain(depth int) Block {
	b := Block{Kind: BlockParagraph, Text: "leaf"}
	for n := depth; n >= 1; n-- {
		b = Block{Kind: BlockQuote, Text: fmt.Sprintf("l%d", n), Children: []Block{b}}
	}
	return b
}

// docWith wraps blocks in a one-section document.
func docWith(blocks ...Block) *Document {
	d := NewDocument("")
	d.Sections[0].Blocks = blocks
	return d
}

// quoteMarkdown is n '>' markers and a word: the input the issue measured.
func quoteMarkdown(n int) string { return strings.Repeat(">", n) + " x" }

// listMarkdown is n nested bullet items, one per line, item k at indent k.
func listMarkdown(n int) string {
	var b strings.Builder
	for k := 0; k < n; k++ {
		b.WriteString(strings.Repeat("  ", k))
		fmt.Fprintf(&b, "- i%d\n", k)
	}
	return b.String()
}

// maxDepth is the deepest block or list item in d (items count Level).
func maxDepth(d *Document) int {
	m := 0
	var walk func(bs []Block, depth int)
	walk = func(bs []Block, depth int) {
		for _, b := range bs {
			m = max(m, depth)
			for _, it := range b.Items {
				m = max(m, depth+it.Level)
			}
			walk(b.Children, depth+1)
		}
	}
	for _, s := range d.Sections {
		walk(s.Blocks, 1)
	}
	return m
}

// texts lists every block's Text depth-first.
func texts(bs []Block) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Text)
		out = append(out, texts(b.Children)...)
	}
	return out
}

func TestNesting_N1_StrictRefusesOneLevelPastTheLimit(t *testing.T) {
	md := quoteMarkdown(MaxNestingDepth + 1)
	_, err := FromMarkdown(md, FromMarkdownOptions{})
	var ue *UnrepresentableError
	if !errors.As(err, &ue) || ue.Construct != ConstructNestingDepth {
		t.Fatalf("FromMarkdown(%d quotes) err = %v, want UnrepresentableError %q", MaxNestingDepth+1, err, ConstructNestingDepth)
	}
	if ue.Offset <= 0 || ue.Offset >= len(md) {
		t.Fatalf("offset %d not inside the %d-byte input", ue.Offset, len(md))
	}
}

func TestNesting_N2_StrictAcceptsExactlyTheLimit(t *testing.T) {
	doc, err := FromMarkdown(quoteMarkdown(MaxNestingDepth), FromMarkdownOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := maxDepth(doc); got != MaxNestingDepth {
		t.Fatalf("depth = %d, want %d", got, MaxNestingDepth)
	}
}

func TestNesting_N3_LenientDeepQuotesAreCheapAndKeepTheText(t *testing.T) {
	start := time.Now()
	doc, err := FromMarkdown(quoteMarkdown(4900), FromMarkdownOptions{Lenient: true})
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if got := maxDepth(doc); got > MaxNestingDepth {
		t.Fatalf("depth = %d, want at most %d", got, MaxNestingDepth)
	}
	if !strings.Contains(fmt.Sprint(texts(doc.Sections[0].Blocks)), "x") {
		t.Fatalf("the text was lost: %v", texts(doc.Sections[0].Blocks))
	}
	if !testing.Short() && took > time.Second {
		t.Fatalf("lenient read of 4900 quotes took %v", took)
	}
}

func TestNesting_N4_LenientFlatteningKeepsEveryBlockInOrder(t *testing.T) {
	// Directive containers: their fences are not line-prefix markers, so the
	// depth reaches the tree-level flattening (limitNesting) intact.
	const n = MaxNestingDepth + 8
	chain := Block{Kind: BlockToggle, Text: fmt.Sprintf("l%d", n)}
	for k := n - 1; k >= 1; k-- {
		chain = Block{Kind: BlockToggle, Text: fmt.Sprintf("l%d", k), Children: []Block{chain}}
	}
	doc, err := FromMarkdown(ToMarkdown(docWith(chain)), FromMarkdownOptions{Lenient: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := maxDepth(doc); got != MaxNestingDepth {
		t.Fatalf("depth = %d, want %d", got, MaxNestingDepth)
	}
	got := texts(doc.Sections[0].Blocks)
	if len(got) != n {
		t.Fatalf("%d blocks after flattening, want %d: %v", len(got), n, got)
	}
	for k, txt := range got {
		if want := fmt.Sprintf("l%d", k+1); txt != want {
			t.Fatalf("block %d text = %q, want %q (all: %v)", k, txt, want, got)
		}
	}

	// Quote markers past the limit on one line are trimmed before the parse
	// (limitLinePrefixes): the deeper lines' text lands in the deepest quote,
	// still all there and in order.
	var md strings.Builder
	for k := 1; k <= n; k++ {
		fmt.Fprintf(&md, "%s l%d\n%s\n", strings.Repeat(">", k), k, strings.Repeat(">", k))
	}
	doc, err = FromMarkdown(md.String(), FromMarkdownOptions{Lenient: true})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(texts(doc.Sections[0].Blocks), " ")
	at := 0
	for k := 1; k <= n; k++ {
		i := strings.Index(all[at:], fmt.Sprintf("l%d", k))
		if i < 0 {
			t.Fatalf("l%d missing or out of order in %q", k, all)
		}
		at += i
	}
}

func TestNesting_N5_NestedListLevelsCount(t *testing.T) {
	md := listMarkdown(MaxNestingDepth + 1)
	_, err := FromMarkdown(md, FromMarkdownOptions{})
	var ue *UnrepresentableError
	if !errors.As(err, &ue) || ue.Construct != ConstructNestingDepth {
		t.Fatalf("strict err = %v, want %q", err, ConstructNestingDepth)
	}
	doc, err := FromMarkdown(md, FromMarkdownOptions{Lenient: true})
	if err != nil {
		t.Fatal(err)
	}
	// The parse opens no list item past the limit, so the deepest line reads
	// as text of the item above it: one item fewer, no text lost.
	items := doc.Sections[0].Blocks[0].Items
	if len(items) != MaxNestingDepth || !strings.Contains(items[len(items)-1].Text, fmt.Sprintf("i%d", MaxNestingDepth)) {
		t.Fatalf("items = %+v, want %d, the last holding i%d", items, MaxNestingDepth, MaxNestingDepth)
	}
	if got := maxDepth(doc); got != MaxNestingDepth {
		t.Fatalf("depth = %d, want %d", got, MaxNestingDepth)
	}
	if _, err := FromMarkdown(listMarkdown(MaxNestingDepth), FromMarkdownOptions{}); err != nil {
		t.Fatalf("a list exactly %d deep is refused: %v", MaxNestingDepth, err)
	}
}

func TestNesting_N6_ValidateNesting(t *testing.T) {
	if err := ValidateNesting(docWith(quoteChain(MaxNestingDepth - 1))); err != nil {
		t.Fatalf("depth %d (chain + leaf) refused: %v", MaxNestingDepth, err)
	}
	err := ValidateNesting(docWith(quoteChain(MaxNestingDepth)))
	var ne *NestingDepthError
	if !errors.As(err, &ne) || ne.Depth != MaxNestingDepth+1 {
		t.Fatalf("depth %d: err = %v, want *NestingDepthError at depth %d", MaxNestingDepth+1, err, MaxNestingDepth+1)
	}
	if want := "sections[0].blocks[0]" + strings.Repeat(".children[0]", MaxNestingDepth); ne.Path != want {
		t.Fatalf("path = %q, want %q", ne.Path, want)
	}
	list := Block{Kind: BlockList, Items: []ListItem{{Text: "a"}, {Text: "deep", Level: MaxNestingDepth}}}
	if err := ValidateNesting(docWith(list)); !errors.As(err, &ne) || ne.Path != "sections[0].blocks[0].items[1]" {
		t.Fatalf("list item at level %d: err = %v, want a *NestingDepthError at items[1]", MaxNestingDepth, err)
	}
}

func TestNesting_N7_ValidateStopsAtTheLimit(t *testing.T) {
	doc := docWith(quoteChain(4900))
	start := time.Now()
	err := ValidateNesting(doc)
	if took := time.Since(start); !testing.Short() && took > 50*time.Millisecond {
		t.Fatalf("validating a 4900-deep document took %v", took)
	}
	var ne *NestingDepthError
	if !errors.As(err, &ne) || ne.Depth != MaxNestingDepth+1 {
		t.Fatalf("err = %v, want *NestingDepthError at depth %d", err, MaxNestingDepth+1)
	}
}

// deepDoc is a document whose deepest block, "deep", sits at depth d.
func deepDoc(d int) *Document {
	c := quoteChain(d - 1) // d-1 quotes and the leaf paragraph at depth d
	doc := docWith(c)
	EnsureBlockIDs(doc)
	return doc
}

// deepestID is the id of the innermost block of a quoteChain document.
func deepestID(doc *Document) string {
	b := &doc.Sections[0].Blocks[0]
	for len(b.Children) > 0 {
		b = &b.Children[0]
	}
	return b.ID
}

func TestNesting_N8_OpsRefuseResultsPastTheLimit(t *testing.T) {
	leaf := func() *Block { return &Block{Kind: BlockParagraph, Text: "new"} }
	two := func() *Block {
		return &Block{Kind: BlockQuote, Text: "q", Children: []Block{{Kind: BlockParagraph, Text: "c"}}}
	}
	cases := []struct {
		name string
		op   func(doc *Document) Op
		ok   bool
	}{
		{"insert a leaf at the limit", func(d *Document) Op {
			return Op{Op: OpInsertBlock, ParentBlockID: deepestID(d), Block: leaf()}
		}, false},
		{"insert a leaf one level above the limit", func(d *Document) Op {
			d.Sections[0].Blocks[0].Children[0].Children = nil // the parent is now at depth 2
			return Op{Op: OpInsertBlock, ParentBlockID: d.Sections[0].Blocks[0].Children[0].ID, Block: leaf()}
		}, true},
		{"insert a two-level block where one fits", func(d *Document) Op {
			parent := &d.Sections[0].Blocks[0]
			for i := 0; i < MaxNestingDepth-2; i++ {
				parent = &parent.Children[0]
			}
			parent.Children = nil // parent at depth MaxNestingDepth-1: one level fits below it
			return Op{Op: OpInsertBlock, ParentBlockID: parent.ID, Block: two()}
		}, false},
		{"update the deepest block with children", func(d *Document) Op {
			return Op{Op: OpUpdateBlock, BlockID: deepestID(d), Block: two()}
		}, false},
		{"patch children onto the deepest block", func(d *Document) Op {
			return Op{Op: OpPatchBlock, BlockID: deepestID(d), Block: &Block{Children: []Block{{Kind: BlockParagraph, Text: "c"}}}}
		}, false},
		{"move a top-level block under the deepest", func(d *Document) Op {
			d.Sections[0].Blocks = append(d.Sections[0].Blocks, Block{ID: "b_mover", Kind: BlockParagraph, Text: "m"})
			return Op{Op: OpMoveBlock, BlockID: "b_mover", ParentBlockID: deepestID(d)}
		}, false},
		{"insert a list whose item level goes past the limit", func(d *Document) Op {
			d.Sections[0].Blocks = nil
			return Op{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockList, Items: []ListItem{{Text: "x", Level: MaxNestingDepth}}}}
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := deepDoc(MaxNestingDepth)
			err := ApplyOps(doc, []Op{c.op(doc)})
			var ne *NestingDepthError
			if c.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.ok && !errors.As(err, &ne) {
				t.Fatalf("err = %v, want *NestingDepthError", err)
			}
		})
	}
}

func TestNesting_N9_OpsRefuseADocumentAlreadyTooDeep(t *testing.T) {
	doc := deepDoc(MaxNestingDepth + 1)
	err := ApplyOps(doc, []Op{{Op: OpSetMetaDirectives, Directives: map[string]string{"title": "t"}}})
	var ne *NestingDepthError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want *NestingDepthError", err)
	}
}

// jsonBytes is the size of the document read from md, as compact JSON.
func jsonBytes(t *testing.T, md string) int {
	t.Helper()
	doc, err := FromMarkdown(md, FromMarkdownOptions{Lenient: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return len(raw)
}

func TestNesting_N10_OutputGrowsLinearlyToTheLimit(t *testing.T) {
	base := jsonBytes(t, quoteMarkdown(1))
	per := func(d int) float64 { return float64(jsonBytes(t, quoteMarkdown(d))-base) / float64(d-1) }
	// Bytes per added level, at a quarter, half and all of the limit: linear
	// output is a constant; indented or quadratic output grows with depth.
	q, h, f := per(MaxNestingDepth/4), per(MaxNestingDepth/2), per(MaxNestingDepth)
	if f > 1.25*q || h > 1.25*q {
		t.Fatalf("bytes per level grow with depth (%.0f, %.0f, %.0f at %d, %d, %d)", q, h, f, MaxNestingDepth/4, MaxNestingDepth/2, MaxNestingDepth)
	}
	// Past the limit nothing nests further, so 4900 levels cost no more than
	// the limit plus the flattened tail.
	if got := jsonBytes(t, quoteMarkdown(4900)); got > 1<<20 {
		t.Fatalf("4900 levels is %d bytes of JSON, want under 1 MB", got)
	}
}

func TestNesting_N11_LimitMatchesTheDartConformanceFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "bind", "eden_press_model", "test", "fixtures", "limits.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		MaxNestingDepth int `json:"maxNestingDepth"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.MaxNestingDepth != MaxNestingDepth {
		t.Fatalf("limits.json maxNestingDepth = %d, model.MaxNestingDepth = %d: change both (and the Dart constant) together", f.MaxNestingDepth, MaxNestingDepth)
	}
}
