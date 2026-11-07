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
	"sort"
	"testing"
	"time"
)

// Block and list-item id assignment (AODex#1149). The cutover migration
// converts every stored document with FromMarkdown, which assigns every id of
// a fresh document. nextBlockID and nextListItemID each walked the whole
// document per id, so assigning n ids was O(n^2): a 1.7 MB Markdown body took
// 73 s. These tests pin the ids the old rule produced (each new id is one more
// than the highest b_N / li_N already in the document, depth-first) and that the
// work now grows linearly.
//
// Test list:
//
//	L1 a fresh document gets b_1..b_n in depth-first order, children included
//	L2 existing ids are kept; new ids continue above the highest b_N, whatever
//	   order or gaps the existing ones have; non-numeric ids are ignored
//	L3 list items: the same rule with li_N, across lists and nested blocks
//	L4 the work is linear: 4x the blocks takes well under 16x the time
//	L5 insert_block numbers new children above the document, the inserted
//	   block and any ids its children already carry: no duplicate
//	L6 a negative b_N / li_N is not a number the rule counts

// genBlocks builds n top-level blocks; every third is a toggle with two
// children and every fifth a three-item list, so ids span nesting and items.
func genBlocks(n int) []Block {
	out := make([]Block, 0, n)
	for i := 0; i < n; i++ {
		switch {
		case i%5 == 0:
			out = append(out, Block{Kind: BlockList, Items: []ListItem{{Text: "a"}, {Text: "b"}, {Text: "c"}}})
		case i%3 == 0:
			out = append(out, Block{Kind: BlockToggle, Text: "t", Children: []Block{
				{Kind: BlockParagraph, Text: "c1"}, {Kind: BlockParagraph, Text: "c2"},
			}})
		default:
			out = append(out, Block{Kind: BlockParagraph, Text: fmt.Sprintf("p%d", i)})
		}
	}
	return out
}

// depthFirstIDs lists ids in the depth-first order the rule assigns them in.
func depthFirstIDs(blocks []Block) []string {
	var out []string
	for _, b := range blocks {
		out = append(out, b.ID)
		out = append(out, depthFirstIDs(b.Children)...)
	}
	return out
}

func TestEnsureBlockIDs_L1_FreshDocumentIsSequentialDepthFirst(t *testing.T) {
	doc := NewDocument("")
	doc.Sections = []Section{{ID: 1, Blocks: genBlocks(7)}, {ID: 2, Blocks: genBlocks(4)}}
	EnsureBlockIDs(doc)

	var got []string
	for _, s := range doc.Sections {
		got = append(got, depthFirstIDs(s.Blocks)...)
	}
	for i, id := range got {
		if want := fmt.Sprintf("b_%d", i+1); id != want {
			t.Fatalf("id %d = %q, want %q (all: %v)", i, id, want, got)
		}
	}
}

func TestEnsureBlockIDs_L2_ExistingIDsKeptAndNewOnesContinueAboveTheMax(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{
		{Kind: BlockParagraph, Text: "new1"},
		{ID: "b_9", Kind: BlockParagraph, Text: "kept"},
		{ID: "intro", Kind: BlockParagraph, Text: "named"},
		{Kind: BlockToggle, Text: "t", Children: []Block{{ID: "b_4", Kind: BlockParagraph}, {Kind: BlockParagraph}}},
		{Kind: BlockParagraph, Text: "new3"},
	}
	EnsureBlockIDs(doc)
	got := depthFirstIDs(doc.Sections[0].Blocks)
	want := []string{"b_10", "b_9", "intro", "b_11", "b_4", "b_12", "b_13"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

func TestEnsureListItemIDs_L3_SameRuleForListItems(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{
		{Kind: BlockList, Items: []ListItem{{Text: "a"}, {ID: "li_7", Text: "kept"}, {Text: "b"}}},
		{Kind: BlockToggle, Children: []Block{{Kind: BlockList, Items: []ListItem{{Text: "nested"}}}}},
		{Kind: BlockList, Items: []ListItem{{ID: "x", Text: "named"}, {Text: "c"}}},
	}
	EnsureListItemIDs(doc)
	var got []string
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			for _, it := range b.Items {
				got = append(got, it.ID)
			}
			walk(b.Children)
		}
	}
	walk(doc.Sections[0].Blocks)
	want := []string{"li_8", "li_7", "li_9", "li_10", "x", "li_11"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("item ids = %v, want %v", got, want)
	}
}

func timeEnsure(n int) time.Duration {
	doc := NewDocument("")
	doc.Sections[0].Blocks = genBlocks(n)
	start := time.Now()
	EnsureBlockIDs(doc)
	EnsureListItemIDs(doc)
	return time.Since(start)
}

func TestEnsureIDs_L4_WorkGrowsLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	// Median of five runs each, at sizes where one linear pass takes
	// milliseconds, so a GC pause or a noisy runner moves one sample, not the
	// verdict.
	median := func(n int) time.Duration {
		var d []time.Duration
		for i := 0; i < 5; i++ {
			d = append(d, timeEnsure(n))
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		return d[2]
	}
	const n = 20000
	timeEnsure(n) // warm up
	small, large := median(n), median(4*n)
	// Linear is ~4x; quadratic is ~16x. 8x leaves room for noise either way.
	if ratio := float64(large) / float64(small); ratio > 8 {
		t.Fatalf("4x the blocks took %.1fx the time (%v vs %v): id assignment is not linear", ratio, large, small)
	}
}

func BenchmarkEnsureIDs20000Blocks(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		doc := NewDocument("")
		doc.Sections[0].Blocks = genBlocks(20000)
		b.StartTimer()
		EnsureBlockIDs(doc)
		EnsureListItemIDs(doc)
	}
}

func TestInsertBlock_L5_ChildrenNumberedAboveEveryIDInPlay(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{{ID: "b_4", Kind: BlockParagraph, Text: "existing"}}
	err := ApplyOps(doc, []Op{{Op: OpInsertBlock, SectionID: 1, Block: &Block{
		ID: "b_5", Kind: BlockToggle, Text: "t",
		Children: []Block{{Kind: BlockParagraph, Text: "new"}, {ID: "b_6", Kind: BlockParagraph, Text: "carried"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range depthFirstIDs(doc.Sections[0].Blocks) {
		if seen[id] {
			t.Fatalf("duplicate id %q: %v", id, depthFirstIDs(doc.Sections[0].Blocks))
		}
		seen[id] = true
	}
	if got := doc.Sections[0].Blocks[1].Children[0].ID; got != "b_7" {
		t.Fatalf("new child id = %q, want b_7 (above b_4, b_5 and b_6)", got)
	}
}

func TestEnsureIDs_L6_NegativeNumbersAreNotCounted(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{
		{ID: "b_-9", Kind: BlockParagraph},
		{Kind: BlockList, Items: []ListItem{{ID: "li_-3"}, {Text: "x"}}},
	}
	EnsureBlockIDs(doc)
	EnsureListItemIDs(doc)
	if got := doc.Sections[0].Blocks[1].ID; got != "b_1" {
		t.Fatalf("block id = %q, want b_1", got)
	}
	if got := doc.Sections[0].Blocks[1].Items[1].ID; got != "li_1" {
		t.Fatalf("item id = %q, want li_1", got)
	}
}
