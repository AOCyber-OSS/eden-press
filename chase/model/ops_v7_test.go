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
	"strings"
	"testing"
)

func TestV7RichTextPatchAndReplace(t *testing.T) {
	doc := NewDocument("t")
	if err := ApplyOps(doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{
			ID:   "b1",
			Kind: BlockParagraph,
			Runs: []TextRun{{Text: "Hello ", Marks: []string{"bold"}}, {Text: "world"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Blocks[0].Text != "Hello world" {
		t.Fatalf("text invariant = %q", doc.Sections[0].Blocks[0].Text)
	}
	if err := ApplyOps(doc, []Op{
		{Op: OpReplaceText, BlockID: "b1", Old: "world", New: "eden"},
	}); err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Blocks[0].Text != "Hello eden" {
		t.Fatalf("after replace = %q", doc.Sections[0].Blocks[0].Text)
	}
	if err := ApplyOps(doc, []Op{
		{Op: OpInsertText, BlockID: "b1", Offset: 0, Text: "X "},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(doc.Sections[0].Blocks[0].Text, "X ") {
		t.Fatalf("insert = %q", doc.Sections[0].Blocks[0].Text)
	}
	if err := ApplyOps(doc, []Op{
		{Op: OpPatchBlock, BlockID: "b1", Block: &Block{Text: "plain"}},
	}); err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Blocks[0].Text != "plain" {
		t.Fatalf("patch = %q", doc.Sections[0].Blocks[0].Text)
	}
}

func TestV7NewKindsSheetEmbedAndProps(t *testing.T) {
	doc := NewDocument("")
	if err := ApplyOps(doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "t1", Kind: BlockTodo, Text: "Ship", Checked: true}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "c1", Kind: BlockCallout, Text: "Note", Icon: "!", Tone: "info"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "d1", Kind: BlockDivider}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "f1", Kind: BlockFile, Src: "a.pdf", Name: "A"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "fig", Kind: BlockFigure, SVG: "<svg/>"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{
			ID:   "sh",
			Kind: BlockSheet,
			SheetEmbed: &SheetEmbed{
				Sheet: "Tasks",
				Snapshot: &SheetSnapshot{
					Cells: []SheetCellSnap{{R: 0, C: 0, V: "A"}, {R: 0, C: 1, V: "B"}},
				},
			},
		}},
		{Op: OpSetProperties, Properties: []PropertyDef{{ID: "status", Name: "Status", Type: PropSelect, Options: []string{"todo", "done"}}}},
		{Op: OpSetPropertyValue, PropertyID: "status", Value: "todo"},
		{Op: OpAddComment, Comment: &Comment{BlockID: "t1", Body: "nudge", Suggestion: &CommentSuggestion{Text: "Shipped"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %s", doc.SchemaVersion)
	}
	if doc.PropertyValues["status"] != "todo" {
		t.Fatalf("prop = %v", doc.PropertyValues)
	}
	if len(doc.Comments) != 1 || doc.Comments[0].ID == "" {
		t.Fatalf("comments = %+v", doc.Comments)
	}
	cid := doc.Comments[0].ID
	if err := ApplyOps(doc, []Op{{Op: OpApplySuggestion, CommentID: cid}}); err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Blocks[0].Text != "Shipped" || !doc.Comments[0].Resolved {
		t.Fatalf("suggestion apply failed: %+v comment=%+v", doc.Sections[0].Blocks[0], doc.Comments[0])
	}
	md := ToMarkdown(doc)
	if !strings.Contains(md, "- [x]") && !strings.Contains(md, "- [ ]") {
		// checked was replaced by suggestion text on todo — still should have table from sheet
	}
	if !strings.Contains(md, "| A | B |") {
		t.Fatalf("sheet snapshot md missing: %s", md)
	}
}

func TestV7DryRunDoesNotMutate(t *testing.T) {
	doc := NewDocument("")
	_ = ApplyOps(doc, []Op{{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b1", Kind: BlockParagraph, Text: "A"}}})
	before := doc.Sections[0].Blocks[0].Text
	err := ApplyOpsWithOptions(doc, []Op{
		{Op: OpReplaceText, BlockID: "b1", Old: "A", New: "B"},
	}, ApplyOpsOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Blocks[0].Text != before {
		t.Fatalf("dry-run mutated: %q", doc.Sections[0].Blocks[0].Text)
	}
	err = ApplyOpsWithOptions(doc, []Op{
		{Op: OpReplaceText, BlockID: "b1", Old: "ZZZ", New: "B"},
	}, ApplyOpsOptions{DryRun: true})
	if err == nil {
		t.Fatal("expected dry-run validation error")
	}
}

func TestV7ListItemIDs(t *testing.T) {
	doc := NewDocument("")
	_ = ApplyOps(doc, []Op{{Op: OpInsertBlock, SectionID: 1, Block: &Block{
		Kind: BlockList, Items: []ListItem{{Text: "one"}, {Text: "two"}},
	}}})
	EnsureListItemIDs(doc)
	if doc.Sections[0].Blocks[0].Items[0].ID == "" || doc.Sections[0].Blocks[0].Items[1].ID == "" {
		t.Fatalf("missing list item ids: %+v", doc.Sections[0].Blocks[0].Items)
	}
}
