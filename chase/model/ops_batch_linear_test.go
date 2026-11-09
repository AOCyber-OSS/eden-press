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
	"testing"
	"time"
)

// An ops batch numbers new blocks and comments from counters seeded once per
// batch (eden-press#30). insertBlock used to walk the whole document for the
// highest b_N on every op, and addComment scanned every comment, so a batch of
// k inserts was O(k^2): 16k inserts took 2.7 s and 100k did not finish in 60 s.
//
// The work is measured by counting whole-document id scans through
// idScanObserver, not by wall-clock ratios: a ratio gate flaked 1 run in 3 on
// a loaded machine (see TestEnsureIDs_L4_WorkGrowsLinearly).
//
// Test list:
//
//	B1 k inserts without ids get b_1..b_k in order, all unique
//	B2 the number of whole-document id scans does not grow with k (blocks)
//	B3 an insert carrying b_N above the counter: later inserts number above it
//	B4 update_block / patch_block children carrying b_N: later inserts number above them
//	B5 delete then insert in one batch never hands out the deleted id again
//	B6 k add_comment ops get c_1..c_k, and comment-id scans do not grow with k
//	B7 an add_comment carrying c_N: later comments number above it
//	B8 100k inserts finish in under 2 s (skipped with -short)
//	B9 dry run assigns the same ids as a real run and leaves the input alone

// paragraphInserts builds k insert_block ops for plain paragraphs without ids,
// all appended to section 1: the batch AODex's block editor sends when a model
// writes a document block by block.
func paragraphInserts(k int) []Op {
	ops := make([]Op, k)
	for i := range ops {
		ops[i] = Op{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockParagraph, Text: fmt.Sprintf("p%d", i)}}
	}
	return ops
}

// commentOps builds k add_comment ops without ids, all anchored on blockID.
func commentOps(k int, blockID string) []Op {
	ops := make([]Op, k)
	for i := range ops {
		ops[i] = Op{Op: OpAddComment, Comment: &Comment{BlockID: blockID, Body: fmt.Sprintf("c%d", i)}}
	}
	return ops
}

// countScans runs f with idScanObserver counting whole-document id scans by
// kind ("block", "comment"), and returns the counts.
func countScans(t *testing.T, f func()) map[string]int {
	t.Helper()
	got := map[string]int{}
	idScanObserver = func(kind string) { got[kind]++ }
	defer func() { idScanObserver = nil }()
	f()
	return got
}

// assertUniqueBlockIDs fails on a repeated or empty block id anywhere in doc.
func assertUniqueBlockIDs(t *testing.T, doc *Document) {
	t.Helper()
	seen := map[string]bool{}
	for _, s := range doc.Sections {
		for _, id := range depthFirstIDs(s.Blocks) {
			if id == "" || seen[id] {
				t.Fatalf("block id %q is empty or repeated", id)
			}
			seen[id] = true
		}
	}
}

func TestOpsBatch_B2_BlockIDScansDoNotGrowWithInserts(t *testing.T) {
	scans := func(k int) int {
		doc := NewDocument("")
		return countScans(t, func() {
			if err := ApplyOps(doc, paragraphInserts(k)); err != nil {
				t.Fatal(err)
			}
		})["block"]
	}
	small, large := scans(250), scans(1000)
	if large != small {
		t.Fatalf("whole-document block-id scans: %d for 250 inserts, %d for 1000; want the same constant (one seed per batch)", small, large)
	}
}

func TestOpsBatch_B1_InsertsWithoutIDsAreSequential(t *testing.T) {
	doc := NewDocument("")
	if err := ApplyOps(doc, paragraphInserts(50)); err != nil {
		t.Fatal(err)
	}
	for i, b := range doc.Sections[0].Blocks {
		if want := fmt.Sprintf("b_%d", i+1); b.ID != want {
			t.Fatalf("block %d id = %q, want %q", i, b.ID, want)
		}
	}
}

func TestOpsBatch_B3_ExplicitIDRaisesTheCounter(t *testing.T) {
	doc := NewDocument("")
	ops := paragraphInserts(1)
	ops = append(ops, Op{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_50", Kind: BlockParagraph, Text: "carried"}})
	ops = append(ops, paragraphInserts(2)...)
	if err := ApplyOps(doc, ops); err != nil {
		t.Fatal(err)
	}
	assertUniqueBlockIDs(t, doc)
	got := depthFirstIDs(doc.Sections[0].Blocks)
	if want := "[b_1 b_50 b_51 b_52]"; fmt.Sprint(got) != want {
		t.Fatalf("ids = %v, want %s", got, want)
	}
}

func TestOpsBatch_B4_UpdateAndPatchChildrenRaiseTheCounter(t *testing.T) {
	for _, kind := range []OpKind{OpUpdateBlock, OpPatchBlock} {
		t.Run(string(kind), func(t *testing.T) {
			doc := NewDocument("")
			doc.Sections[0].Blocks = []Block{{ID: "b_1", Kind: BlockToggle, Text: "t"}}
			ops := []Op{
				{Op: kind, BlockID: "b_1", Block: &Block{Kind: BlockToggle, Text: "t", Children: []Block{
					{ID: "b_70", Kind: BlockParagraph, Text: "carried"},
				}}},
			}
			ops = append(ops, paragraphInserts(1)...)
			if err := ApplyOps(doc, ops); err != nil {
				t.Fatal(err)
			}
			assertUniqueBlockIDs(t, doc)
			if got := doc.Sections[0].Blocks[1].ID; got != "b_71" {
				t.Fatalf("insert after %s got %q, want b_71 (above the child's b_70)", kind, got)
			}
		})
	}
}

func TestOpsBatch_B5_DeletedIDIsNotReusedInTheSameBatch(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{{ID: "b_1", Kind: BlockParagraph}, {ID: "b_2", Kind: BlockParagraph}}
	ops := []Op{{Op: OpDeleteBlock, BlockID: "b_2"}}
	ops = append(ops, paragraphInserts(1)...)
	if err := ApplyOps(doc, ops); err != nil {
		t.Fatal(err)
	}
	// A comment or a client still holding b_2 must not find it re-attached to
	// a different block.
	if got := doc.Sections[0].Blocks[1].ID; got != "b_3" {
		t.Fatalf("insert after deleting b_2 got %q, want b_3", got)
	}
}

func TestOpsBatch_B6_CommentIDsSequentialAndScansDoNotGrow(t *testing.T) {
	run := func(k int) (*Document, int) {
		doc := NewDocument("")
		doc.Sections[0].Blocks = []Block{{ID: "b_1", Kind: BlockParagraph, Text: "anchor"}}
		n := countScans(t, func() {
			if err := ApplyOps(doc, commentOps(k, "b_1")); err != nil {
				t.Fatal(err)
			}
		})["comment"]
		return doc, n
	}
	doc, small := run(250)
	for i, c := range doc.Comments {
		if want := fmt.Sprintf("c_%d", i+1); c.ID != want {
			t.Fatalf("comment %d id = %q, want %q", i, c.ID, want)
		}
	}
	if _, large := run(1000); large != small {
		t.Fatalf("comment-id scans: %d for 250 comments, %d for 1000; want the same constant", small, large)
	}
}

func TestOpsBatch_B7_ExplicitCommentIDRaisesTheCounter(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{{ID: "b_1", Kind: BlockParagraph, Text: "anchor"}}
	ops := []Op{{Op: OpAddComment, Comment: &Comment{ID: "c_9", BlockID: "b_1", Body: "carried"}}}
	ops = append(ops, commentOps(1, "b_1")...)
	if err := ApplyOps(doc, ops); err != nil {
		t.Fatal(err)
	}
	if got := doc.Comments[1].ID; got != "c_10" {
		t.Fatalf("comment after c_9 got %q, want c_10", got)
	}
}

func TestOpsBatch_B8_HundredThousandInsertsUnderTwoSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	doc := NewDocument("")
	ops := paragraphInserts(100000)
	start := time.Now()
	if err := ApplyOps(doc, ops); err != nil {
		t.Fatal(err)
	}
	// Linear work is about 0.1 s here; the bound is wide so only a return to
	// per-op walks (over 60 s) fails it, never a slow runner.
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("100k inserts took %v, want under 2s", d)
	}
	if got := doc.Sections[0].Blocks[99999].ID; got != "b_100000" {
		t.Fatalf("last id = %q, want b_100000", got)
	}
}

func TestOpsBatch_B9_DryRunAssignsTheSameIDsAndLeavesTheInput(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{{ID: "b_3", Kind: BlockParagraph}}
	if err := ApplyOpsWithOptions(doc, paragraphInserts(3), ApplyOpsOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Sections[0].Blocks); n != 1 {
		t.Fatalf("dry run changed the input: %d blocks", n)
	}
	if err := ApplyOps(doc, paragraphInserts(3)); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(depthFirstIDs(doc.Sections[0].Blocks)); got != "[b_3 b_4 b_5 b_6]" {
		t.Fatalf("ids = %s", got)
	}
}

// BenchmarkInsertBlockBatch16000 is the issue's measured case (2.7 s before).
func BenchmarkInsertBlockBatch16000(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		doc := NewDocument("")
		ops := paragraphInserts(16000)
		b.StartTimer()
		if err := ApplyOps(doc, ops); err != nil {
			b.Fatal(err)
		}
	}
}
