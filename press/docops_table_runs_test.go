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

package press

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
)

// Formatted table cells (schema v7, AODex 1026 D14) through the byte helpers.

// styledTableDocJSON is a canonical stored document with a styled, ragged,
// aligned table, written as GetDocJSON writes one: ids on every block, the
// outline ApplyOps derives, and cell runs in canonical form.
func styledTableDocJSON(t *testing.T) []byte {
	t.Helper()
	d := model.Document{
		SchemaVersion: model.SchemaVersion,
		Meta:          model.Meta{Directives: map[string]string{"title": "T"}},
		Sections: []model.Section{{ID: 1, Blocks: []model.Block{
			{ID: "b_1", Kind: model.BlockParagraph, Text: "intro"},
			{ID: "b_2", Kind: model.BlockTable,
				Headers:    []string{"Name", "Qty"},
				Rows:       [][]string{{"a|b", "1"}, {"see docs"}},
				Aligns:     []string{"", "right"},
				HeaderRuns: [][]model.TextRun{{{Text: "Name", Marks: []string{"bold"}}}, nil},
				RowRuns: [][][]model.TextRun{
					{{{Text: "a|b", Marks: []string{"code"}}}, nil},
					{{{Text: "see "}, {Text: "docs", Link: "https://x.io"}}},
				}},
		}}},
		Outline: []model.OutlineEntry{},
	}
	raw, err := json.MarshalIndent(&d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"headerRuns"`, `"rowRuns"`, "null"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("fixture lacks %s:\n%s", want, raw)
		}
	}
	return raw
}

func TestTableRunsGetDocJSONPreservesCellRuns(t *testing.T) {
	in := styledTableDocJSON(t)
	out, err := GetDocJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, in) {
		t.Fatalf("GetDocJSON changed a canonical styled table\n--- in ---\n%s\n--- out ---\n%s", in, out)
	}
}

func TestTableRunsApplyOpsJSONPreservesCellRuns(t *testing.T) {
	in := styledTableDocJSON(t)

	// A batch that changes nothing returns the document byte for byte.
	out, err := ApplyOpsJSON(in, []byte(`[{"op":"set_meta_directives","directives":{"title":"T"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, in) {
		t.Fatalf("a no-op batch changed the document\n--- in ---\n%s\n--- out ---\n%s", in, out)
	}

	// A batch that edits another block leaves the table's bytes alone.
	out, err = ApplyOpsJSON(in, []byte(`[{"op":"patch_block","blockId":"b_1","block":{"text":"changed"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := blockJSON(t, out, "b_2"), blockJSON(t, in, "b_2"); got != want {
		t.Fatalf("editing a paragraph changed the table\nwant %s\n got %s", want, got)
	}
}

func TestTableRunsApplyOpsJSONOptsDryRunLeavesTheInput(t *testing.T) {
	in := styledTableDocJSON(t)
	keep := append([]byte(nil), in...)
	ops := []byte(`[{"op":"patch_block","blockId":"b_2","block":{"rowRuns":[[null,[{"text":"1","marks":["bold"]}]],null]}}]`)

	out, err := ApplyOpsJSONOpts(in, ops, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, keep) || !bytes.Equal(in, keep) {
		t.Fatalf("a dry run changed the document\n--- in ---\n%s\n--- out ---\n%s", keep, out)
	}

	// The same batch, for real, sets the runs it names (and clears none it does not).
	out, err = ApplyOpsJSONOpts(in, ops, false)
	if err != nil {
		t.Fatal(err)
	}
	d := decodeDoc(t, out)
	tb := d.Sections[0].Blocks[1]
	if tb.RowRuns == nil || tb.RowRuns[0][0] != nil || model.FlattenRuns(tb.RowRuns[0][1]) != "1" || tb.RowRuns[1] != nil {
		t.Fatalf("rowRuns after the patch: %+v", tb.RowRuns)
	}
	if tb.HeaderRuns == nil || tb.Rows[1][0] != "see docs" {
		t.Fatalf("the patch touched cells it did not name: %+v %q", tb.HeaderRuns, tb.Rows)
	}
}

func TestTableRunsApplyOpsJSONRefusesABadShape(t *testing.T) {
	in := styledTableDocJSON(t)
	keep := append([]byte(nil), in...)
	for _, ops := range []string{
		`[{"op":"patch_block","blockId":"b_2","block":{"rowRuns":[null]}}]`,
		`[{"op":"patch_block","blockId":"b_2","block":{"headerRuns":[[{"text":"a\nb"}],null]}}]`,
		`[{"op":"patch_block","blockId":"b_1","block":{"rowRuns":[[[{"text":"x","marks":["bold"]}]]]}}]`,
	} {
		if out, err := ApplyOpsJSON(in, []byte(ops)); err == nil {
			t.Errorf("%s was accepted:\n%s", ops, out)
		}
		if !bytes.Equal(in, keep) {
			t.Fatalf("a refused batch changed its input")
		}
	}
}

// blockJSON returns the compact JSON of the block with id in doc.
func blockJSON(t *testing.T, doc []byte, id string) string {
	t.Helper()
	d := decodeDoc(t, doc)
	for _, b := range d.Sections[0].Blocks {
		if b.ID == id {
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			return string(raw)
		}
	}
	t.Fatalf("no block %q", id)
	return ""
}

// plainTableDoc is an ops-built document holding one plain table b_1 (two
// header cells, two rows of two).
func plainTableDoc(t *testing.T) model.Document {
	t.Helper()
	doc, err := NewDocJSON("")
	if err != nil {
		t.Fatal(err)
	}
	ops := []byte(`[{"op":"insert_block","sectionId":1,"block":{"kind":"table","headers":["a","b"],"rows":[["1","2"],["3","4"]]}}]`)
	if doc, err = ApplyOpsJSON(doc, ops); err != nil {
		t.Fatal(err)
	}
	return decodeDoc(t, doc)
}

// A stored plain table may hold cell runs that are all null: that styles no
// cell, so the guard must treat it as the same table as one without the keys,
// as it does an all-empty "aligns".
func TestTableRunsGuardTreatsAllNullCellRunsAsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(b *model.Block)
		wire string
	}{
		{"rowRuns all null", func(b *model.Block) { b.RowRuns = [][][]model.TextRun{nil, nil} }, `"rowRuns":[null,null]`},
		{"headerRuns all null", func(b *model.Block) { b.HeaderRuns = [][]model.TextRun{nil, nil} }, `"headerRuns":[null,null]`},
		{"rowRuns null at every depth", func(b *model.Block) { b.RowRuns = [][][]model.TextRun{{nil, nil}, nil} }, `"rowRuns":[[null,null],null]`},
		{"rowRuns with an empty row", func(b *model.Block) { b.RowRuns = [][][]model.TextRun{{}, nil} }, `"rowRuns":[[],null]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := plainTableDoc(t)
			tc.set(&d.Sections[0].Blocks[0])
			raw, err := json.Marshal(&d)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.wire) {
				t.Fatalf("the stored document does not hold %s:\n%s", tc.wire, raw)
			}
			if err := VerifyMarkdownRoundTripJSON(raw); err != nil {
				t.Fatalf("a plain table with all-null cell runs should pass: %v", err)
			}
		})
	}
}

// patch_block keeps a paragraph's single plain run (SingleRun), and the guard
// reports that paragraph at ".runs", because the reader writes no runs for plain
// text. Cells do not share the problem: the invariant drops a plain cell run, so
// a table edited through ops stores no cell runs and passes the guard.
func TestTableRunsOpsBuiltPlainCellsPassTheGuard(t *testing.T) {
	doc, err := NewDocJSON("")
	if err != nil {
		t.Fatal(err)
	}
	ops := []byte(`[
		{"op":"insert_block","sectionId":1,"block":{"id":"b_1","kind":"table","headers":["a","b"],"rows":[["1","2"]]}},
		{"op":"patch_block","blockId":"b_1","block":{"headerRuns":[[{"text":"A"}],null],"rowRuns":[[[{"text":"one"}],null]]}}
	]`)
	if doc, err = ApplyOpsJSON(doc, ops); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"headerRuns"`, `"rowRuns"`} {
		if bytes.Contains(doc, []byte(key)) {
			t.Fatalf("plain cell runs were stored as %s:\n%s", key, doc)
		}
	}
	tb := decodeDoc(t, doc).Sections[0].Blocks[0]
	if tb.Headers[0] != "A" || tb.Rows[0][0] != "one" {
		t.Fatalf("plain cell runs did not set the text: %q %q", tb.Headers, tb.Rows)
	}
	if err := VerifyMarkdownRoundTripJSON(doc); err != nil {
		t.Fatalf("an ops-built plain table should pass the guard: %v", err)
	}
}
