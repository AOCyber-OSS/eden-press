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
	"os"
	"sort"
	"strings"
	"testing"
)

// Formatted table cells (schema v7, additive; AODex 1026 D14): Block.HeaderRuns
// and Block.RowRuns sit beside the plain Headers and Rows, as Runs sits beside
// Text.

const v7WithoutCellRunsGolden = "testdata/v7_without_cell_runs.golden.json"

// v7AllKindsDoc is a hand-built document with every one of the 16 block kinds,
// page properties, a comment and a section note. Its tables are plain: one with
// headers, aligns and ragged rows, and one with no headers. It is built by hand,
// not through Build or FromMarkdown, so the golden it was captured into cannot
// move when a reader changes.
func v7AllKindsDoc() *Document {
	start, stop := 0, 5
	return &Document{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{Directives: map[string]string{"title": "All kinds"}},
		Sections: []Section{{
			ID:    1,
			Attrs: map[string]string{"class": "lead"},
			Notes: []string{"remember"},
			Blocks: []Block{
				{ID: "b_1", Kind: BlockHeading, Level: 1, Text: "All kinds", Span: &Span{Start: 0, Stop: 11}},
				{ID: "b_2", Kind: BlockParagraph, Text: "plain and bold", Runs: []TextRun{{Text: "plain and "}, {Text: "bold", Marks: []string{"bold"}}}},
				{ID: "b_3", Kind: BlockList, Ordered: true, Items: []ListItem{
					{ID: "li_1", Text: "one"},
					{ID: "li_2", Text: "two", Level: 1, Runs: []TextRun{{Text: "two", Marks: []string{"italic"}}}},
				}},
				{ID: "b_4", Kind: BlockCode, Language: "go", Text: "fmt.Println(1)"},
				{ID: "b_5", Kind: BlockMath, Display: true, Text: "x^2"},
				{ID: "b_6", Kind: BlockTable,
					Headers: []string{"Name", "Qty", "Note"},
					Rows:    [][]string{{"a", "1", "x"}, {"b", "2"}, {"c", "3", "y", "extra"}},
					Aligns:  []string{"left", "right", ""}},
				{ID: "b_7", Kind: BlockImage, Src: "cat.png", Text: "a cat", Title: "Cat"},
				{ID: "b_8", Kind: BlockQuote, Text: "quoted", Children: []Block{{ID: "b_9", Kind: BlockParagraph, Text: "inside"}}},
				{ID: "b_10", Kind: BlockToggle, Text: "More", Collapsed: true, Children: []Block{
					{ID: "b_11", Kind: BlockTodo, Text: "do it", Checked: true},
				}},
				{ID: "b_12", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_2"}},
				{ID: "b_13", Kind: BlockCallout, Tone: "info", Icon: "!", Text: "note"},
				{ID: "b_14", Kind: BlockDivider},
				{ID: "b_15", Kind: BlockFile, Src: "files/r.pdf", Name: "r.pdf", Mime: "application/pdf", Size: 1024},
				{ID: "b_16", Kind: BlockSheet, SheetEmbed: &SheetEmbed{
					Sheet:    "S",
					Range:    &SheetRange{R1: 2, C1: 1},
					Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{{R: 0, C: 0, V: "A"}}},
				}},
				{ID: "b_17", Kind: BlockFigure, Src: "fig.png", SVG: "<svg/>"},
				{ID: "b_18", Kind: BlockTable, Rows: [][]string{{"only", "row"}}},
			},
		}},
		Outline:        []OutlineEntry{{SectionID: 1, Level: 1, Text: "All kinds", Slug: "all-kinds"}},
		Properties:     []PropertyDef{{ID: "status", Name: "Status", Type: PropSelect, Options: []string{"todo", "done"}}},
		PropertyValues: map[string]string{"status": "todo"},
		Comments: []Comment{{ID: "c_1", BlockID: "b_2", Start: &start, Stop: &stop, Body: "check",
			Suggestion: &CommentSuggestion{Text: "Plain", Runs: []TextRun{{Text: "Plain"}}}}},
	}
}

// marshalGolden is the byte form the golden was written in.
func marshalGolden(t *testing.T, d *Document) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return append(raw, '\n')
}

// TestV7CellRunsAdditiveGolden proves the two cell-run fields are additive: the
// golden was captured from the code BEFORE they existed (commit e90867a on
// feat/1026-lossless-codec), so a document with no styled cell must still
// marshal to exactly those bytes, and decoding it must leave both fields nil.
// If this fails after a change, a field lost omitempty or was declared before an
// existing one: fix the declaration and never regenerate the golden.
func TestV7CellRunsAdditiveGolden(t *testing.T) {
	golden, err := os.ReadFile(v7WithoutCellRunsGolden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	// The golden really is a capture without cell runs, of every kind.
	for _, bad := range []string{`"headerRuns"`, `"rowRuns"`} {
		if strings.Contains(string(golden), bad) {
			t.Fatalf("%s holds %s: it was captured after the change and proves nothing", v7WithoutCellRunsGolden, bad)
		}
	}
	if !strings.Contains(string(golden), `"eden-press.model/v7"`) {
		t.Fatalf("%s is not a v7 capture", v7WithoutCellRunsGolden)
	}

	doc := v7AllKindsDoc()
	kinds := map[BlockKind]bool{}
	walkAll(doc, func(b *Block, _ int) { kinds[b.Kind] = true })
	if len(kinds) != len(propKinds) {
		got := make([]string, 0, len(kinds))
		for k := range kinds {
			got = append(got, string(k))
		}
		sort.Strings(got)
		t.Fatalf("the fixture covers %d kinds %v, want all %d", len(kinds), got, len(propKinds))
	}

	if got := marshalGolden(t, doc); string(got) != string(golden) {
		t.Fatalf("a document with no styled cell no longer marshals to the golden\n--- golden ---\n%s\n--- now ---\n%s", golden, got)
	}

	var back Document
	if err := json.Unmarshal(golden, &back); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if got := marshalGolden(t, &back); string(got) != string(golden) {
		t.Fatalf("decoding the golden and writing it again changes it\n--- golden ---\n%s\n--- now ---\n%s", golden, got)
	}
}

// cellBold, cellCode and cellLinked are the runs the tests below put in cells.
func cellBold(s string) []TextRun { return []TextRun{{Text: s, Marks: []string{"bold"}}} }
func cellCode(s string) []TextRun { return []TextRun{{Text: s, Marks: []string{"code"}}} }
func cellLinked(s, href string) []TextRun {
	return []TextRun{{Text: "see "}, {Text: s, Link: href}}
}

// tableDoc wraps blocks in a one-section document with ids.
func tableDoc(blocks ...Block) *Document {
	return &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: blocks}}, Outline: []OutlineEntry{}}
}

// TestTableRunsJSONShape pins the wire shape TRDs 26, 28 and 29 are coded
// against: the keys come in the order headers, rows, aligns, headerRuns,
// rowRuns, a plain cell is null, and a table with no styled cell has neither key.
func TestTableRunsJSONShape(t *testing.T) {
	b := Block{Kind: BlockTable,
		Headers:    []string{"Name", "Qty"},
		Rows:       [][]string{{"a|b", "1"}},
		Aligns:     []string{"", "right"},
		HeaderRuns: [][]TextRun{cellBold("Name"), nil},
		RowRuns:    [][][]TextRun{{cellCode("a|b"), nil}},
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	want := `{"kind":"table","headers":["Name","Qty"],"rows":[["a|b","1"]],"aligns":["","right"],` +
		`"headerRuns":[[{"text":"Name","marks":["bold"]}],null],"rowRuns":[[[{"text":"a|b","marks":["code"]}],null]]}`
	if got != want {
		t.Fatalf("table JSON\n got: %s\nwant: %s", got, want)
	}
	order := []string{"headers", "rows", "aligns", "headerRuns", "rowRuns"}
	for i := 1; i < len(order); i++ {
		if a, c := strings.Index(got, `"`+order[i-1]+`":`), strings.Index(got, `"`+order[i]+`":`); a < 0 || c < 0 || a > c {
			t.Errorf("key %q is not before %q in %s", order[i-1], order[i], got)
		}
	}

	plain := Block{Kind: BlockTable, Headers: []string{"A"}, Rows: [][]string{{"1"}}, Aligns: []string{"left"}}
	raw, err = json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"kind":"table","headers":["A"],"rows":[["1"]],"aligns":["left"]}`; string(raw) != want {
		t.Fatalf("a plain table\n got: %s\nwant: %s", raw, want)
	}
}

// TestTableRunsJSONFixedPoint shows null entries survive JSON: Marshal, then
// Unmarshal, then Marshal is byte-identical, and the decoded value keeps every
// nil entry nil (deepDiff tells nil from empty).
func TestTableRunsJSONFixedPoint(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    Block
	}{
		{"null entries", Block{Kind: BlockTable,
			Headers: []string{"a", "b"}, HeaderRuns: [][]TextRun{nil, cellBold("b")},
			Rows: [][]string{{"c", "d"}}, RowRuns: [][][]TextRun{{cellCode("c"), nil}}}},
		{"null row", Block{Kind: BlockTable,
			Rows:    [][]string{{"x", "y"}, {"z", "a|b"}},
			RowRuns: [][][]TextRun{nil, {nil, cellCode("a|b")}}}},
		{"ragged rows", Block{Kind: BlockTable,
			Headers: []string{"h1", "h2"},
			Rows:    [][]string{{"a"}, {"b", "c", "see x"}},
			RowRuns: [][][]TextRun{{cellBold("a")}, {nil, nil, cellLinked("x", "https://x.io")}}}},
		{"header-less with rowRuns", Block{Kind: BlockTable,
			Rows: [][]string{{"x", "y"}}, RowRuns: [][][]TextRun{{nil, cellBold("y")}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b1, err := json.Marshal(tc.b)
			if err != nil {
				t.Fatal(err)
			}
			var back Block
			if err := json.Unmarshal(b1, &back); err != nil {
				t.Fatal(err)
			}
			b2, err := json.Marshal(back)
			if err != nil {
				t.Fatal(err)
			}
			if string(b1) != string(b2) {
				t.Fatalf("not a JSON fixed point\nfirst: %s\nthen:  %s", b1, b2)
			}
			if d := deepDiff(tc.b, back); d != "" {
				t.Fatalf("decoded block differs: %s", d)
			}
		})
	}

	// The wire example from the TRD decodes to the documented value.
	wire := `{"kind":"table","rows":[["x","y"],["z","a|b"]],"rowRuns":[null,[null,[{"text":"a|b","marks":["code"]}]]]}`
	var b Block
	if err := json.Unmarshal([]byte(wire), &b); err != nil {
		t.Fatal(err)
	}
	if b.HeaderRuns != nil || len(b.RowRuns) != 2 || b.RowRuns[0] != nil || b.RowRuns[1][0] != nil || FlattenRuns(b.RowRuns[1][1]) != "a|b" {
		t.Fatalf("wire example decoded to %+v", b.RowRuns)
	}
	again, _ := json.Marshal(b)
	if string(again) != wire {
		t.Fatalf("wire example re-encodes as %s", again)
	}
}

// invariantTable runs EnsureTextRunsInvariant on a document holding b and
// returns the block it left.
func invariantTable(b Block) Block {
	d := tableDoc(b)
	EnsureTextRunsInvariant(d)
	return d.Sections[0].Blocks[0]
}

// TestTableRunsInvariant pins the canonical form EnsureTextRunsInvariant (and so
// GetDocJSON and ApplyOps) gives cell runs.
func TestTableRunsInvariant(t *testing.T) {
	t.Run("runs set the cell text", func(t *testing.T) {
		got := invariantTable(Block{Kind: BlockTable,
			Headers: []string{"stale", "B"}, HeaderRuns: [][]TextRun{cellBold("Name"), nil},
			Rows: [][]string{{"old", "1"}}, RowRuns: [][][]TextRun{{cellLinked("docs", "https://x.io"), nil}}})
		if got.Headers[0] != "Name" || got.Rows[0][0] != "see docs" {
			t.Fatalf("cell text not mirrored from runs: headers %q rows %q", got.Headers, got.Rows)
		}
		if d := deepDiff(got.HeaderRuns, [][]TextRun{cellBold("Name"), nil}); d != "" {
			t.Fatalf("styled header runs changed: %s", d)
		}
		if d := deepDiff(got.RowRuns, [][][]TextRun{{cellLinked("docs", "https://x.io"), nil}}); d != "" {
			t.Fatalf("styled row runs changed: %s", d)
		}
	})
	t.Run("a single plain run is a nil entry", func(t *testing.T) {
		got := invariantTable(Block{Kind: BlockTable,
			Headers: []string{"h", "x"}, HeaderRuns: [][]TextRun{{{Text: "H"}}, cellBold("x")},
			Rows: [][]string{{"old"}}, RowRuns: [][][]TextRun{{{{Text: "new"}}}}})
		if got.Headers[0] != "H" || got.Rows[0][0] != "new" {
			t.Fatalf("plain runs did not set the text: %q %q", got.Headers, got.Rows)
		}
		if d := deepDiff(got.HeaderRuns, [][]TextRun{nil, cellBold("x")}); d != "" {
			t.Fatalf("header runs: %s", d)
		}
		if got.RowRuns != nil {
			t.Fatalf("a table whose only styled-looking cell is plain keeps rowRuns %+v", got.RowRuns)
		}
	})
	t.Run("all-nil arrays are nil", func(t *testing.T) {
		got := invariantTable(Block{Kind: BlockTable,
			Headers: []string{"a", "b"}, HeaderRuns: [][]TextRun{nil, {}},
			Rows: [][]string{{"c"}, {"d", "e"}}, RowRuns: [][][]TextRun{nil, {nil, nil}}})
		if got.HeaderRuns != nil || got.RowRuns != nil {
			t.Fatalf("all-nil runs kept: headerRuns %+v rowRuns %+v", got.HeaderRuns, got.RowRuns)
		}
		if d := deepDiff(got.Headers, []string{"a", "b"}); d != "" {
			t.Fatalf("headers changed: %s", d)
		}
		if d := deepDiff(got.Rows, [][]string{{"c"}, {"d", "e"}}); d != "" {
			t.Fatalf("rows changed: %s", d)
		}
	})
	t.Run("a row with no styled cell is nil", func(t *testing.T) {
		got := invariantTable(Block{Kind: BlockTable,
			Rows: [][]string{{"a", "b"}, {"c", "d"}}, RowRuns: [][][]TextRun{{nil, {{Text: "b"}}}, {cellCode("c"), nil}}})
		if d := deepDiff(got.RowRuns, [][][]TextRun{nil, {cellCode("c"), nil}}); d != "" {
			t.Fatalf("row runs: %s", d)
		}
	})
	t.Run("nil-entry cells are untouched", func(t *testing.T) {
		got := invariantTable(Block{Kind: BlockTable,
			Headers: []string{"keep", "x"}, HeaderRuns: [][]TextRun{nil, cellBold("X")},
			Rows: [][]string{{"keep", "y"}, {"keep too"}}, RowRuns: [][][]TextRun{{nil, cellBold("Y")}, nil}})
		if got.Headers[0] != "keep" || got.Rows[0][0] != "keep" || got.Rows[1][0] != "keep too" {
			t.Fatalf("plain cells changed: %q %q", got.Headers, got.Rows)
		}
	})
	t.Run("lengths follow the cells", func(t *testing.T) {
		// Headers and Rows are the authoritative cells: a missing entry is a plain
		// cell, and an entry past the last cell describes nothing and is dropped.
		got := invariantTable(Block{Kind: BlockTable,
			Headers: []string{"a", "b", "c"}, HeaderRuns: [][]TextRun{cellBold("a")},
			Rows:    [][]string{{"d", "e"}, {"f"}},
			RowRuns: [][][]TextRun{{nil, cellBold("e"), cellBold("orphan"), nil, nil}, nil, {cellBold("no row")}}})
		if d := deepDiff(got.HeaderRuns, [][]TextRun{cellBold("a"), nil, nil}); d != "" {
			t.Fatalf("header runs: %s", d)
		}
		if d := deepDiff(got.RowRuns, [][][]TextRun{{nil, cellBold("e")}, nil}); d != "" {
			t.Fatalf("row runs: %s", d)
		}
		if d := deepDiff(got.Rows, [][]string{{"d", "e"}, {"f"}}); d != "" {
			t.Fatalf("rows changed: %s", d)
		}
	})
	t.Run("a document with no styled cell is unchanged", func(t *testing.T) {
		golden, err := os.ReadFile(v7WithoutCellRunsGolden)
		if err != nil {
			t.Fatal(err)
		}
		d := v7AllKindsDoc()
		EnsureTextRunsInvariant(d)
		if got := marshalGolden(t, d); string(got) != string(golden) {
			t.Fatalf("the invariant changed a document with plain tables\n--- golden ---\n%s\n--- now ---\n%s", golden, got)
		}
	})
}
