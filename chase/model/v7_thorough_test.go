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

func TestV7ValidateNewKinds(t *testing.T) {
	cases := []struct {
		name    string
		block   Block
		wantErr string
	}{
		{"file_missing_src", Block{Kind: BlockFile, Name: "x"}, "missing src"},
		{"figure_empty", Block{Kind: BlockFigure}, "src or svg"},
		{"sheet_nil_embed", Block{Kind: BlockSheet}, "sheetEmbed"},
		{"sheet_empty", Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{}}, "sheet name or snapshot"},
		{"divider_with_text", Block{Kind: BlockDivider, Text: "nope"}, "must not carry"},
		{"ok_file", Block{Kind: BlockFile, Src: "a.bin"}, ""},
		{"ok_figure_svg", Block{Kind: BlockFigure, SVG: "<svg/>"}, ""},
		{"ok_figure_src", Block{Kind: BlockFigure, Src: "a.png"}, ""},
		{"ok_sheet_name", Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "S"}}, ""},
		{"ok_sheet_snap", Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{{R: 0, C: 0, V: "1"}}}}}, ""},
		{"ok_todo", Block{Kind: BlockTodo, Text: "x", Checked: true}, ""},
		{"ok_callout", Block{Kind: BlockCallout, Text: "n", Tone: "warn"}, ""},
		{"ok_divider", Block{Kind: BlockDivider}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEditableBlock(tc.block)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestV7PatchBlockMergeAndClearFlags(t *testing.T) {
	doc := NewDocument("")
	mustApply(t, doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "tog", Kind: BlockToggle, Text: "T", Collapsed: true, Children: []Block{{ID: "inner", Kind: BlockParagraph, Text: "c"}}}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "todo", Kind: BlockTodo, Text: "do", Checked: true}},
	})
	mustApply(t, doc, []Op{
		{Op: OpPatchBlock, BlockID: "todo", Block: &Block{Text: "done"}, ClearChecked: true},
		{Op: OpPatchBlock, BlockID: "tog", ClearCollapsed: true, Block: &Block{Icon: "x"}},
	})
	todo := findBlockPtr(doc, "todo")
	if todo.Checked || todo.Text != "done" {
		t.Fatalf("todo patch: %+v", todo)
	}
	tog := findBlockPtr(doc, "tog")
	if tog.Collapsed || len(tog.Children) != 1 || tog.Children[0].Text != "c" {
		t.Fatalf("toggle patch lost children or collapsed: %+v", tog)
	}
	if err := ApplyOps(doc, []Op{{Op: OpPatchBlock, BlockID: "todo", Block: &Block{Kind: BlockParagraph}}}); err == nil {
		t.Fatal("expected kind-change reject")
	}
}

func TestV7ReplaceTextAllAndDocWide(t *testing.T) {
	doc := NewDocument("")
	mustApply(t, doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "a", Kind: BlockParagraph, Text: "foo foo"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "b", Kind: BlockParagraph, Text: "foo bar"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockList, Items: []ListItem{{Text: "foo item"}}}},
	})
	mustApply(t, doc, []Op{{Op: OpReplaceText, Old: "foo", New: "baz", All: true}})
	if findBlockPtr(doc, "a").Text != "baz baz" {
		t.Fatalf("a = %q", findBlockPtr(doc, "a").Text)
	}
	if findBlockPtr(doc, "b").Text != "baz bar" {
		t.Fatalf("b = %q", findBlockPtr(doc, "b").Text)
	}
	list := doc.Sections[0].Blocks[2]
	if list.Items[0].Text != "baz item" {
		t.Fatalf("list item = %q", list.Items[0].Text)
	}
	if err := ApplyOps(doc, []Op{{Op: OpReplaceText, Old: "zzz", New: "n"}}); err == nil {
		t.Fatal("expected not found")
	}
}

func TestV7InsertTextBounds(t *testing.T) {
	doc := NewDocument("")
	mustApply(t, doc, []Op{{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "p", Kind: BlockParagraph, Text: "ab"}}})
	if err := ApplyOps(doc, []Op{{Op: OpInsertText, BlockID: "p", Offset: 99, Text: "x"}}); err == nil {
		t.Fatal("expected offset error")
	}
	mustApply(t, doc, []Op{{Op: OpInsertText, BlockID: "p", Offset: 2, Text: "c"}})
	if findBlockPtr(doc, "p").Text != "abc" {
		t.Fatalf("got %q", findBlockPtr(doc, "p").Text)
	}
}

func TestV7CommentsLifecycle(t *testing.T) {
	doc := NewDocument("")
	mustApply(t, doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{ID: "p", Kind: BlockParagraph, Text: "draft"}},
		{Op: OpAddComment, Comment: &Comment{BlockID: "p", Body: "fix me"}},
		{Op: OpAddComment, Comment: &Comment{ID: "c_custom", BlockID: "p", Body: "sug", Suggestion: &CommentSuggestion{Text: "final", Runs: []TextRun{{Text: "final", Marks: []string{"bold"}}}}}},
	})
	if len(doc.Comments) != 2 {
		t.Fatalf("comments = %d", len(doc.Comments))
	}
	mustApply(t, doc, []Op{{Op: OpResolveComment, CommentID: doc.Comments[0].ID}})
	if !doc.Comments[0].Resolved {
		t.Fatal("not resolved")
	}
	mustApply(t, doc, []Op{{Op: OpApplySuggestion, CommentID: "c_custom"}})
	p := findBlockPtr(doc, "p")
	if p.Text != "final" || len(p.Runs) != 1 || p.Runs[0].Marks[0] != "bold" {
		t.Fatalf("suggestion: %+v", p)
	}
	if !doc.Comments[1].Resolved {
		t.Fatal("suggestion comment not resolved")
	}
	if err := ApplyOps(doc, []Op{{Op: OpAddComment, Comment: &Comment{BlockID: "missing", Body: "x"}}}); err == nil {
		t.Fatal("expected missing block")
	}
	if err := ApplyOps(doc, []Op{{Op: OpApplySuggestion, CommentID: doc.Comments[0].ID}}); err == nil {
		t.Fatal("expected no suggestion")
	}
}

func TestV7PropertiesValidation(t *testing.T) {
	doc := NewDocument("")
	if err := ApplyOps(doc, []Op{{Op: OpSetProperties, Properties: []PropertyDef{{ID: "", Name: "X", Type: PropText}}}}); err == nil {
		t.Fatal("expected missing id")
	}
	if err := ApplyOps(doc, []Op{{Op: OpSetProperties, Properties: []PropertyDef{{ID: "p", Name: "X", Type: "nope"}}}}); err == nil {
		t.Fatal("expected bad type")
	}
	mustApply(t, doc, []Op{
		{Op: OpSetProperties, Properties: []PropertyDef{
			{ID: "status", Name: "Status", Type: PropSelect, Options: []string{"a", "b"}},
			{ID: "due", Name: "Due", Type: PropDate},
			{ID: "ok", Name: "OK", Type: PropCheckbox},
			{ID: "url", Name: "URL", Type: PropURL},
			{ID: "mail", Name: "Mail", Type: PropEmail},
			{ID: "tags", Name: "Tags", Type: PropMultiSelect},
			{ID: "n", Name: "N", Type: PropNumber},
			{ID: "rel", Name: "Rel", Type: PropRelation},
		}},
		{Op: OpSetPropertyValue, PropertyID: "status", Value: "a"},
		{Op: OpSetPropertyValue, PropertyID: "due", Value: "2026-10-31"},
	})
	if err := ApplyOps(doc, []Op{{Op: OpSetPropertyValue, PropertyID: "missing", Value: "x"}}); err == nil {
		t.Fatal("expected unknown property")
	}
	mustApply(t, doc, []Op{{Op: OpSetProperties, ReplaceProperties: true, Properties: []PropertyDef{{ID: "only", Name: "Only", Type: PropText}}}})
	if len(doc.Properties) != 1 || doc.Properties[0].ID != "only" {
		t.Fatalf("replace: %+v", doc.Properties)
	}
}

func TestV7MarkdownExportAllNewKinds(t *testing.T) {
	doc := NewDocument("Deck")
	mustApply(t, doc, []Op{
		{Op: OpSetMetaDirectives, Directives: map[string]string{"theme": "default"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockHeading, Level: 1, Text: "Title", Runs: []TextRun{{Text: "Title", Marks: []string{"bold"}}}}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockTodo, Text: "Open", Checked: false}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockTodo, Text: "Done", Checked: true}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockCallout, Text: "Heads up", Icon: "⚠️", Tone: "warn"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockDivider}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockFile, Src: "doc.pdf", Name: "Spec"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockFigure, Src: "logo.png", Text: "Logo", Title: "Brand"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockFigure, SVG: "<svg id=\"x\"/>"}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockParagraph, Runs: []TextRun{
			{Text: "bold", Marks: []string{"bold"}},
			{Text: " ", Marks: nil},
			{Text: "code", Marks: []string{"code"}},
			{Text: " ", Marks: nil},
			{Text: "link", Link: "https://example.com"},
		}}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{
			Kind: BlockSheet,
			SheetEmbed: &SheetEmbed{
				Sheet: "Tasks",
				Range: &SheetRange{R0: 0, R1: 1, C0: 0, C1: 1, ColumnIDs: []string{"a"}},
				Snapshot: &SheetSnapshot{
					Columns: []SheetColumnSnap{{ID: "a", Name: "A", Type: "text"}},
					Cells:   []SheetCellSnap{{R: 0, C: 0, V: "H1"}, {R: 0, C: 1, V: "H2"}, {R: 1, C: 0, V: "v"}, {R: 1, C: 1, F: "A1"}},
				},
			},
		}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "EmptyOnly"}}},
	})
	md := ToMarkdown(doc)
	// Each v7-only kind has an explicit, parseable spelling (TRD 02): a todo is
	// a GFM task item and the others are container directives whose JSON header
	// carries the scalar fields. Nothing is flattened to a table, a link or a
	// quote any more.
	checks := []string{
		"theme: default",
		"**Title**",
		"- [ ] Open",
		"- [x] Done",
		":::callout {\"icon\":\"⚠️\",\"tone\":\"warn\"}\nHeads up\n:::",
		"---",
		":::file {\"name\":\"Spec\",\"src\":\"doc.pdf\"}",
		":::figure {\"src\":\"logo.png\",\"title\":\"Brand\",\"alt\":\"Logo\"}",
		":::figure\n<svg id=\"x\"/>\n:::",
		"**bold**",
		"`code`",
		"[link](https://example.com)",
		"\"cells\":[{\"r\":0,\"c\":0,\"v\":\"H1\"},{\"r\":0,\"c\":1,\"v\":\"H2\"},{\"r\":1,\"c\":0,\"v\":\"v\"},{\"r\":1,\"c\":1,\"f\":\"A1\"}]",
		":::sheet {\"sheetEmbed\":{\"sheet\":\"EmptyOnly\"}}",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q in:\n%s", want, md)
		}
	}
}

func TestV7JSONRoundTripPreservesFields(t *testing.T) {
	doc := NewDocument("R")
	mustApply(t, doc, []Op{
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{
			ID: "b1", Kind: BlockParagraph,
			Runs: []TextRun{{Text: "Hi", Marks: []string{"italic"}, Link: "https://a"}},
		}},
		{Op: OpInsertBlock, SectionID: 1, Block: &Block{
			ID: "sh", Kind: BlockSheet,
			SheetEmbed: &SheetEmbed{
				PageID: "page-1", WorkbookHint: "wb", Sheet: "S",
				Range:    &SheetRange{RangeID: "nr1", RowIDs: []string{"r_a"}},
				Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{{R: 0, C: 0, V: "1", Rels: []map[string]string{{"rowId": "r_b"}}}}},
			},
		}},
		{Op: OpSetProperties, Properties: []PropertyDef{{ID: "s", Name: "S", Type: PropSelect, Options: []string{"x"}}}},
		{Op: OpSetPropertyValue, PropertyID: "s", Value: "x"},
		{Op: OpAddComment, Comment: &Comment{BlockID: "b1", Body: "c", Start: intPtr(0), Stop: intPtr(2)}},
	})
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var got Document
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("schema %s", got.SchemaVersion)
	}
	if got.Sections[0].Blocks[0].Runs[0].Link != "https://a" {
		t.Fatalf("runs: %+v", got.Sections[0].Blocks[0].Runs)
	}
	se := got.Sections[0].Blocks[1].SheetEmbed
	if se == nil || se.PageID != "page-1" || se.Range.RangeID != "nr1" || se.Snapshot.Cells[0].Rels[0]["rowId"] != "r_b" {
		t.Fatalf("sheetEmbed: %+v", se)
	}
	if got.PropertyValues["s"] != "x" || got.Comments[0].Body != "c" {
		t.Fatalf("meta fields lost: props=%v comments=%+v", got.PropertyValues, got.Comments)
	}
}

func TestV7RichTextInvariantHelpers(t *testing.T) {
	if FlattenRuns(nil) != "" {
		t.Fatal("empty")
	}
	runs := []TextRun{{Text: "a"}, {Text: "b", Marks: []string{"bold"}}}
	if FlattenRuns(runs) != "ab" {
		t.Fatal(FlattenRuns(runs))
	}
	next, ok := ReplaceInRuns(runs, "ab", "z", false)
	if !ok || FlattenRuns(next) != "z" {
		t.Fatalf("%v %v", next, ok)
	}
	b := Block{Runs: []TextRun{{Text: "x"}}}
	SyncTextFromRuns(&b)
	if b.Text != "x" {
		t.Fatal(b.Text)
	}
}

func mustApply(t *testing.T, doc *Document, ops []Op) {
	t.Helper()
	if err := ApplyOps(doc, ops); err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
}
