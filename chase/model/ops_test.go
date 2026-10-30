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
		t.Fatalf("sections = %+v, want one section id=1", doc.Sections)
	}
	if doc.Outline == nil {
		t.Fatal("Outline must be non-nil empty slice for stable agent JSON")
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"blocks"`) {
		t.Fatalf("empty page must omit blocks: %s", b)
	}
}

func TestApplyOpsInsertUpdateDeleteHeadingAndParagraph(t *testing.T) {
	doc := NewDocument("spike")
	idx0 := 0
	if err := ApplyOps(doc, []Op{
		{
			Op:        OpInsertBlock,
			SectionID: 1,
			Index:     &idx0,
			Block: &Block{
				ID:    "b_title",
				Kind:  BlockHeading,
				Level: 1,
				Text:  "Alpha",
			},
		},
		{
			Op:        OpInsertBlock,
			SectionID: 1,
			Block: &Block{
				ID:   "b_body",
				Kind: BlockParagraph,
				Text: "Hello",
			},
		},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if len(doc.Sections[0].Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(doc.Sections[0].Blocks))
	}
	if len(doc.Outline) != 1 || doc.Outline[0].Text != "Alpha" {
		t.Fatalf("outline = %+v", doc.Outline)
	}

	if err := ApplyOps(doc, []Op{{
		Op:      OpUpdateBlock,
		BlockID: "b_title",
		Block: &Block{
			ID:    "b_title",
			Kind:  BlockHeading,
			Level: 1,
			Text:  "Q3 plan",
		},
	}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if doc.Sections[0].Blocks[0].Text != "Q3 plan" {
		t.Fatalf("title text = %q", doc.Sections[0].Blocks[0].Text)
	}
	if doc.Outline[0].Text != "Q3 plan" {
		t.Fatalf("outline not rebuilt: %+v", doc.Outline)
	}

	if err := ApplyOps(doc, []Op{{
		Op:      OpDeleteBlock,
		BlockID: "b_body",
	}}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(doc.Sections[0].Blocks) != 1 {
		t.Fatalf("after delete blocks = %d", len(doc.Sections[0].Blocks))
	}
}

func TestApplyOpsAssignsIDWhenMissing(t *testing.T) {
	doc := NewDocument("")
	if err := ApplyOps(doc, []Op{{
		Op:        OpInsertBlock,
		SectionID: 1,
		Block:     &Block{Kind: BlockParagraph, Text: "no id"},
	}}); err != nil {
		t.Fatal(err)
	}
	id := doc.Sections[0].Blocks[0].ID
	if id == "" {
		t.Fatal("expected auto-assigned block id")
	}
	if !strings.HasPrefix(id, "b_") {
		t.Fatalf("id = %q, want b_ prefix", id)
	}
}

func TestApplyOpsRejectsUnsupportedKind(t *testing.T) {
	doc := NewDocument("")
	err := ApplyOps(doc, []Op{{
		Op:        OpInsertBlock,
		SectionID: 1,
		Block:     &Block{ID: "x", Kind: BlockCode, Text: "fmt.Println(1)", Language: "go"},
	}})
	if err == nil {
		t.Fatal("expected error for code block in spike")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyOpsJSONRoundTrip(t *testing.T) {
	doc := NewDocument("round")
	if err := ApplyOps(doc, []Op{{
		Op:        OpInsertBlock,
		SectionID: 1,
		Block:     &Block{ID: "b1", Kind: BlockParagraph, Text: "one"},
	}}); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var got Document
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Sections[0].Blocks[0].ID != "b1" {
		t.Fatalf("id lost in JSON: %+v", got.Sections[0].Blocks[0])
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
	b, err = json.Marshal(Block{ID: "b1", Kind: BlockParagraph, Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"b1","kind":"paragraph","text":"x"}`
	if string(b) != want {
		t.Fatalf("got %s want %s", b, want)
	}
}
