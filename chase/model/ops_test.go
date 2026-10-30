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
	"strings"
	"testing"
)

func TestNewDocumentEmptyPage(t *testing.T) {
	doc := NewDocument("Q3 plan")
	if doc.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", doc.SchemaVersion, SchemaVersion)
	}
	if doc.Meta.Directives["title"] != "Q3 plan" {
		t.Fatalf("title = %q", doc.Meta.Directives["title"])
	}
	if len(doc.Sections) != 1 || doc.Sections[0].ID != 1 {
		t.Fatalf("sections = %+v", doc.Sections)
	}
}

func TestEnsureBlockIDsStable(t *testing.T) {
	doc := NewDocument("")
	doc.Sections[0].Blocks = []Block{
		{Kind: BlockParagraph, Text: "a"},
		{Kind: BlockToggle, Text: "t", Children: []Block{{Kind: BlockParagraph, Text: "c"}}},
	}
	EnsureBlockIDs(doc)
	id0 := doc.Sections[0].Blocks[0].ID
	idChild := doc.Sections[0].Blocks[1].Children[0].ID
	if id0 == "" || idChild == "" {
		t.Fatal("expected ids")
	}
	EnsureBlockIDs(doc)
	if doc.Sections[0].Blocks[0].ID != id0 || doc.Sections[0].Blocks[1].Children[0].ID != idChild {
		t.Fatal("EnsureBlockIDs renumbered existing ids")
	}
}

func TestApplyOpsAllKindsAndMove(t *testing.T) {
	doc := NewDocument("spike")
	if err := ApplyOps(doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_h", Kind: BlockHeading, Level: 1, Text: "Title"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_p", Kind: BlockParagraph, Text: "Body"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_code", Kind: BlockCode, Language: "go", Text: "x"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_list", Kind: BlockList, Items: []ListItem{{Text: "one"}}}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_img", Kind: BlockImage, Src: "a.png", Text: "alt"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b_tog", Kind: BlockToggle, Text: "More", Children: []Block{{ID: "b_inner", Kind: BlockParagraph, Text: "nested"}}}},
		{Op: OpMoveBlock, BlockID: "b_p", SectionID: 1, Index: intPtr(0)},
	}); err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Blocks[0].ID != "b_p" {
		t.Fatalf("move failed: %+v", doc.Sections[0].Blocks)
	}
	if len(doc.Outline) != 1 || doc.Outline[0].Text != "Title" {
		t.Fatalf("outline = %+v", doc.Outline)
	}
}

func TestApplyOpsNestedInsertAndSyncedRules(t *testing.T) {
	doc := NewDocument("")
	if err := ApplyOps(doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "src", Kind: BlockParagraph, Text: "canonical"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "tog", Kind: BlockToggle, Text: "T"}},
		{Op: OpInsertBlock, ParentBlockID: "tog", Block: &Block{ID: "child", Kind: BlockParagraph, Text: "in toggle"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "sync", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "src"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyOps(doc, []Op{{Op: OpUpdateBlock, BlockID: "sync", Block: &Block{ID: "sync", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "src"}, Text: "x"}}}); err == nil {
		t.Fatal("expected reject update on synced")
	}
	if err := ApplyOps(doc, []Op{{Op: OpDeleteBlock, BlockID: "src"}}); err == nil {
		t.Fatal("expected reject delete of sync source")
	}
	if err := ApplyOps(doc, []Op{
		{Op: OpDeleteBlock, BlockID: "sync"},
		{Op: OpDeleteBlock, BlockID: "src"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSectionOps(t *testing.T) {
	doc := NewDocument("")
	if err := ApplyOps(doc, []Op{
		{Op: OpInsertSection, Index: intPtr(1), Attrs: map[string]string{"class": "lead"}},
		{Op: OpSetMetaDirectives, Directives: map[string]string{"theme": "gaia"}},
		{Op: OpSetSectionNotes, SectionID: 2, Notes: []string{"note"}},
		{Op: OpMoveSection, SectionID: 2, Index: intPtr(0)},
	}); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 2 {
		t.Fatalf("sections = %d", len(doc.Sections))
	}
	if doc.Sections[0].Attrs["class"] != "lead" {
		t.Fatalf("attrs = %+v", doc.Sections[0].Attrs)
	}
	if doc.Meta.Directives["theme"] != "gaia" {
		t.Fatalf("meta = %+v", doc.Meta.Directives)
	}
	if doc.Sections[0].ID != 1 || doc.Sections[1].ID != 2 {
		t.Fatalf("renumber failed: %+v", doc.Sections)
	}
}

func TestToMarkdownRoundTripSmoke(t *testing.T) {
	doc := NewDocument("")
	_ = ApplyOps(doc, []Op{
		{Op: OpSetMetaDirectives, Directives: map[string]string{"theme": "default"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockHeading, Level: 1, Text: "Hi"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockParagraph, Text: "Body"}},
	})
	md := ToMarkdown(doc)
	if !strings.Contains(md, "theme: default") || !strings.Contains(md, "# Hi") {
		t.Fatalf("md = %s", md)
	}
}

func TestBlockIDOmitempty(t *testing.T) {
	b, err := json.Marshal(Block{Kind: BlockParagraph, Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"id"`) {
		t.Fatalf("empty id must omit: %s", b)
	}
}

func intPtr(i int) *int { return &i }
