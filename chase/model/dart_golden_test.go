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
	"bytes"
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// updateDartGolden rewrites the Dart fixture from dartGoldenDocument. Run it
// only when the Go model's JSON shape changes on purpose, then make the Dart
// toJson in bind/eden_press_model agree with the new bytes:
//
//	go test ./chase/model -run TestDartGolden -update-dart-golden
var updateDartGolden = flag.Bool(
	"update-dart-golden",
	false,
	"rewrite bind/eden_press_model/test/fixtures/v7_all_kinds.json from dartGoldenDocument",
)

// dartGoldenPath is the fixture bind/eden_press_model/test/model_json_test.dart
// decodes. It is the cross-language contract: the Dart package proves that
// ModelDocument.fromJson(golden).toJson() equals it, so Dart and Go agree on
// every key and every omitempty decision for the same document.
var dartGoldenPath = filepath.Join(
	"..", "..", "bind", "eden_press_model", "test", "fixtures", "v7_all_kinds.json",
)

// dartGoldenDocument is a hand-built Document that exercises every field the
// Dart model mirrors: all 16 block kinds, rich-text runs, a checked and an
// unchecked todo, callout icon/tone, a file with a size above 2^32, a sheet
// embed with a range, columns, row ids, a formula cell and a rels cell, a
// figure with inline SVG, section notes/attrs/spans, meta directives, typed
// properties, property values and comments.
//
// Several entries exist only to pin an omitempty edge the Dart side must
// mirror exactly:
//   - the first block's Span starts at 0 (Span.start is NOT omitempty);
//   - SheetRange.R0/C0 are 0 and therefore omitted (they ARE omitempty);
//   - an all-zero SheetRange still emits "range":{} (the pointer is non-nil);
//   - an empty CommentSuggestion emits "suggestion":{};
//   - Comment.Start is a pointer, so a 0 offset is kept;
//   - a ListItem with empty text still emits "text":"" (not omitempty);
//   - section 2 carries nothing but its id;
//   - characters Go escapes in JSON (<, >, &) appear in strings.
func dartGoldenDocument() Document {
	commentStart, commentStop := 0, 5
	return Document{
		SchemaVersion: SchemaVersion,
		Meta: Meta{Directives: map[string]string{
			"theme":   "default",
			"profile": "article",
			"title":   "Q3 <Review> & plan",
		}},
		Sections: []Section{
			{
				ID:    1,
				Attrs: map[string]string{"data-tone": "warm", "class": "lead"},
				Notes: []string{"Speaker note one", "Note two — ünïcödé"},
				Span:  &Span{Start: 0, Stop: 420},
				Blocks: []Block{
					{ID: "b_head", Kind: BlockHeading, Text: "Roadmap", Level: 1, Span: &Span{Start: 0, Stop: 9}},
					{
						ID:   "b_para",
						Kind: BlockParagraph,
						Text: "Hello bold link",
						Runs: []TextRun{
							{Text: "Hello "},
							{Text: "bold", Marks: []string{"bold", "italic"}},
							{Text: " link", Link: "https://aocyber.ai"},
						},
						Span: &Span{Start: 11, Stop: 40},
					},
					{
						ID:      "b_list",
						Kind:    BlockList,
						Ordered: true,
						Items: []ListItem{
							{ID: "li_1", Text: "First"},
							{
								ID:    "li_2",
								Text:  "Nested code",
								Level: 1,
								Runs:  []TextRun{{Text: "Nested "}, {Text: "code", Marks: []string{"code"}}},
							},
							{Text: ""},
						},
					},
					{ID: "b_code", Kind: BlockCode, Text: "fmt.Println(\"<hi>\")\n", Language: "go"},
					{ID: "b_math", Kind: BlockMath, Text: `\frac{a}{b}`, Display: true},
					{Kind: BlockMath, Text: "x^2"},
					{
						ID:      "b_table",
						Kind:    BlockTable,
						Headers: []string{"Metric", "Q3"},
						Rows:    [][]string{{"p95 latency", "550ms"}, {"ragged"}},
						Aligns:  []string{"left", ""},
					},
					{ID: "b_img", Kind: BlockImage, Text: "Q3 chart", Src: "chart.png", Title: "Quarterly"},
					{
						ID:       "b_quote",
						Kind:     BlockQuote,
						Text:     "Synthesized from the sync.",
						Children: []Block{{Kind: BlockParagraph, Text: "Quoted child"}},
					},
					{
						ID:        "b_toggle",
						Kind:      BlockToggle,
						Text:      "Details",
						Collapsed: true,
						Children:  []Block{{ID: "b_toggle_child", Kind: BlockParagraph, Text: "Nested"}},
					},
					{ID: "b_sync", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_toggle"}},
					{ID: "b_sync_ext", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_remote", PageID: "page_2"}},
					{
						ID:       "b_todo",
						Kind:     BlockTodo,
						Text:     "Ship it",
						Checked:  true,
						Runs:     []TextRun{{Text: "Ship it", Marks: []string{"strike"}}},
						Children: []Block{{ID: "b_todo_child", Kind: BlockParagraph, Text: "Release notes"}},
					},
					{ID: "b_todo_open", Kind: BlockTodo, Text: "Write docs"},
					{ID: "b_callout", Kind: BlockCallout, Text: "Watch latency", Icon: "⚠️", Tone: "warn"},
					{ID: "b_div", Kind: BlockDivider},
					{
						ID:   "b_file",
						Kind: BlockFile,
						Name: "report.pdf",
						Mime: "application/pdf",
						Size: 5368709120,
						Src:  "files/report.pdf",
					},
					{
						ID:   "b_sheet",
						Kind: BlockSheet,
						SheetEmbed: &SheetEmbed{
							PageID:       "page_wb",
							WorkbookHint: "Budget",
							Sheet:        "Tasks",
							Range: &SheetRange{
								R0:        0,
								R1:        3,
								C0:        0,
								C1:        2,
								ColumnIDs: []string{"col_a", "col_b"},
								RowIDs:    []string{"row_0", "row_1"},
								RangeID:   "rng_1",
							},
							Snapshot: &SheetSnapshot{
								Columns: []SheetColumnSnap{
									{ID: "col_a", Name: "Item", Type: "text"},
									{ID: "col_b", Name: "Status", Type: "select", Options: []string{"todo", "done"}},
								},
								RowIDs: []SheetRowIDSnap{{R: 0, ID: "row_0"}, {R: 1, ID: "row_1"}},
								Cells: []SheetCellSnap{
									{R: 0, C: 0, V: "A"},
									{R: 0, C: 1, V: "B"},
									{R: 1, C: 0, V: "3"},
									{R: 1, C: 1, V: "Spec", Rels: []map[string]string{
										{"title": "Spec", "pageId": "page_9"},
									}},
									{R: 2, C: 0, F: "SUM(A1:A2)", V: "7"},
									{R: 2, C: 1, F: "A1*2"},
								},
							},
						},
					},
					{ID: "b_sheet_live", Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "Live", Range: &SheetRange{}}},
					{
						ID:    "b_fig",
						Kind:  BlockFigure,
						Text:  "logo",
						Icon:  "brand",
						SVG:   `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`,
						Src:   "assets/logo.svg",
						Title: "Logo",
					},
				},
			},
			{ID: 2},
			{
				ID: 3,
				Blocks: []Block{
					{Kind: BlockHeading, Text: "Appendix", Level: 2},
					{Kind: BlockParagraph},
				},
			},
		},
		Outline: []OutlineEntry{
			{SectionID: 1, Level: 1, Text: "Roadmap", Slug: "roadmap", Span: &Span{Start: 0, Stop: 9}},
			{SectionID: 3, Level: 2, Text: "Appendix", Slug: "appendix"},
		},
		Properties: []PropertyDef{
			{ID: "p_status", Name: "Status", Type: PropSelect, Options: []string{"draft", "final"}},
			{ID: "p_due", Name: "Due", Type: PropDate},
			{ID: "p_done", Name: "Done", Type: PropCheckbox},
		},
		PropertyValues: map[string]string{"p_status": "draft", "p_due": "2026-11-03", "p_done": "false"},
		Comments: []Comment{
			{
				ID:      "c_1",
				BlockID: "b_para",
				Start:   &commentStart,
				Stop:    &commentStop,
				Body:    "Tighten this",
				Suggestion: &CommentSuggestion{
					Text: "Hi bold",
					Runs: []TextRun{{Text: "Hi "}, {Text: "bold", Marks: []string{"bold"}}},
				},
			},
			{ID: "c_2", BlockID: "b_todo", Body: "Done?", Resolved: true},
			{ID: "c_3", BlockID: "b_code", Body: "", Suggestion: &CommentSuggestion{}},
		},
	}
}

// TestDartGolden pins json.Marshal(dartGoldenDocument()) to the committed Dart
// fixture (Test list 1 of 1026-05). If this fails, Go's JSON shape moved:
// regenerate with -update-dart-golden, then make bind/eden_press_model's toJson
// agree (its `dart test` compares against the same file).
func TestDartGolden(t *testing.T) {
	doc := dartGoldenDocument()
	assertCoversEveryBlockKind(t, doc)

	got := indentedJSON(t, doc)

	if *updateDartGolden {
		if err := os.MkdirAll(filepath.Dir(dartGoldenPath), 0o755); err != nil {
			t.Fatalf("create fixture dir: %v", err)
		}
		if err := os.WriteFile(dartGoldenPath, got, 0o644); err != nil {
			t.Fatalf("write %s: %v", dartGoldenPath, err)
		}
		t.Logf("wrote %s (%d bytes)", dartGoldenPath, len(got))
	}

	want, err := os.ReadFile(dartGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v (generate it with: go test ./chase/model -run TestDartGolden -update-dart-golden)",
			dartGoldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("json.Marshal(dartGoldenDocument()) no longer equals %s.\n"+
			"If the model change is intended, regenerate with -update-dart-golden and update the Dart toJson.\n"+
			"got:\n%s", dartGoldenPath, got)
	}

	// The golden must also be a fixed point for Go itself: decoding it and
	// re-marshaling reproduces it byte for byte. That is the exact property the
	// Dart package mirrors (toJson(fromJson(golden)) == golden).
	var decoded Document
	if err := json.Unmarshal(want, &decoded); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if again := indentedJSON(t, decoded); !bytes.Equal(again, want) {
		t.Fatalf("golden is not a Go round-trip fixed point:\n%s", again)
	}
}

// indentedJSON is json.Marshal(v), indented two spaces with a trailing
// newline so the committed fixture is reviewable. Indentation only adds
// whitespace between tokens; the keys, values and escaping are Marshal's.
func indentedJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		t.Fatalf("indent: %v", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

// assertCoversEveryBlockKind fails if doc does not use every BlockKind
// constant declared in document.go. The constants are read from the source
// with go/parser, so adding a 17th kind without extending the golden (and
// with it the Dart enum) fails here instead of silently shipping.
func assertCoversEveryBlockKind(t *testing.T, doc Document) {
	t.Helper()
	declared := declaredBlockKinds(t)

	used := map[string]bool{}
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			used[string(b.Kind)] = true
			walk(b.Children)
		}
	}
	for _, s := range doc.Sections {
		walk(s.Blocks)
	}

	var missing []string
	for _, k := range declared {
		if !used[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("dartGoldenDocument does not use block kinds %v (declared: %v)", missing, declared)
	}
	if len(used) != len(declared) {
		t.Fatalf("dartGoldenDocument uses %d kinds, document.go declares %d", len(used), len(declared))
	}
}

// declaredBlockKinds returns the string value of every `X BlockKind = "..."`
// constant in document.go, sorted.
func declaredBlockKinds(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "document.go", nil, 0)
	if err != nil {
		t.Fatalf("parse document.go: %v", err)
	}
	var kinds []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "BlockKind" {
				continue
			}
			for _, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("BlockKind constant with a non-literal value at %s", fset.Position(value.Pos()))
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", lit.Value, err)
				}
				kinds = append(kinds, s)
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatal("found no BlockKind constants in document.go")
	}
	sort.Strings(kinds)
	return kinds
}
