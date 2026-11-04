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
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// Wide table rows (objective 1026, TRD 31, AODex D18). GFM ignores the cells of a
// body row that lie beyond the header's width, and goldmark's AST never holds
// them, so a codec that only walked the AST dropped them without a word, in strict
// mode too (TRD 26 found it). FromMarkdown now reads a table whose body row is
// wider than its header as a table as wide as its widest row: the header gains
// empty cells, Aligns gains "" entries, a narrower row gains empty cells, and no
// cell is lost. No new refusal: the operator chose padding (D18).

// wideTableDoc is a one-block document holding a plain table.
func wideTableDoc(headers, aligns []string, rows [][]string) *Document {
	return fxDoc([]Block{{Kind: BlockTable, Headers: headers, Aligns: aligns, Rows: rows}})
}

// tablesOf is every table block of d, in document order, at any depth.
func tablesOf(d *Document) []*Block {
	var out []*Block
	walkAll(d, func(b *Block, level int) {
		if b.Kind == BlockTable {
			out = append(out, b)
		}
	})
	return out
}

// readWide reads src in strict and in lenient mode and fails if either is refused or
// if the two reads differ (a source with nothing to map reads the same either way).
func readWide(t *testing.T, src string) *Document {
	t.Helper()
	strict := mustFromMarkdown(t, src, FromMarkdownOptions{})
	lenient := mustFromMarkdown(t, src, FromMarkdownOptions{Lenient: true})
	if diff := deepDiff(stripIDsAndSpans(lenient), stripIDsAndSpans(strict)); diff != "" {
		t.Fatalf("strict and lenient reads differ at %s\nsource: %q", diff, src)
	}
	return strict
}

// Test list 1: the header is padded to the widest row; every cell is kept.
func TestCodecCellsWideRowPadsHeader(t *testing.T) {
	cases := []struct {
		name, src string
		want      *Document
	}{
		{
			"one_extra_cell",
			"| a |\n| - |\n| x | zqsentinel |\n",
			wideTableDoc([]string{"a", ""}, []string{"", ""}, [][]string{{"x", "zqsentinel"}}),
		},
		{
			"several_extra_cells",
			"| a |\n| - |\n| w | x | y | z |\n",
			wideTableDoc([]string{"a", "", "", ""}, []string{"", "", "", ""}, [][]string{{"w", "x", "y", "z"}}),
		},
		{
			"alignment_is_kept_and_the_new_columns_are_unaligned",
			"| a | b |\n| :-- | --: |\n| 1 | 2 | 3 |\n",
			wideTableDoc([]string{"a", "b", ""}, []string{"left", "right", ""}, [][]string{{"1", "2", "3"}}),
		},
		{
			"the_widest_row_sets_the_width_and_narrower_rows_pad",
			"| a |\n| - |\n| 1 | 2 |\n| 3 | 4 | 5 |\n| 6 |\n",
			wideTableDoc([]string{"a", "", ""}, []string{"", "", ""}, [][]string{{"1", "2", ""}, {"3", "4", "5"}, {"6", "", ""}}),
		},
		{
			"the_widest_row_comes_first",
			"| a | b |\n| - | - |\n| 1 | 2 | 3 | 4 |\n| 5 | 6 |\n",
			wideTableDoc([]string{"a", "b", "", ""}, []string{"", "", "", ""}, [][]string{{"1", "2", "3", "4"}, {"5", "6", "", ""}}),
		},
		{
			"an_empty_excess_cell_is_a_cell",
			"| a |\n| - |\n| x | |\n",
			wideTableDoc([]string{"a", ""}, []string{"", ""}, [][]string{{"x", ""}}),
		},
		{
			"a_table_without_outer_pipes",
			"a | b\n--- | ---\n1 | 2 | 3\n",
			wideTableDoc([]string{"a", "b", ""}, []string{"", "", ""}, [][]string{{"1", "2", "3"}}),
		},
		{
			"text_before_the_table_stays_a_paragraph",
			"intro\n| a |\n| - |\n| x | y |\n",
			fxDoc([]Block{fxPara("intro"), {Kind: BlockTable, Headers: []string{"a", ""}, Aligns: []string{"", ""}, Rows: [][]string{{"x", "y"}}}}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := readWide(t, tc.src)
			assertDocEqual(t, stripIDsAndSpans(got), tc.want)
			assertCanonicalCells(t, got)
			if err := propValid(got); err != nil {
				t.Fatalf("the padded read is not a valid document (Validate wants every row as wide as the header): %v", err)
			}
		})
	}
}

// A header that no cell of is styled keeps HeaderRuns null even when it is padded
// (TRD 25's byte-identical encoding rule), and a row's runs are only as long as
// the cells that carry them.
func TestCodecCellsWideRowKeepsRunsCanonical(t *testing.T) {
	got := readWide(t, "| a |\n| - |\n| x | **b** |\n")
	b := tablesOf(got)[0]
	if b.HeaderRuns != nil {
		t.Fatalf("an unstyled padded header must have no HeaderRuns, got %+v", b.HeaderRuns)
	}
	want := [][][]TextRun{{nil, {fxRun("b", "bold")}}}
	if !reflect.DeepEqual(b.RowRuns, want) {
		t.Fatalf("RowRuns = %+v, want %+v", b.RowRuns, want)
	}
	if !reflect.DeepEqual(b.Rows, [][]string{{"x", "b"}}) || !reflect.DeepEqual(b.Headers, []string{"a", ""}) {
		t.Fatalf("Headers = %q, Rows = %q", b.Headers, b.Rows)
	}
	assertCanonicalCells(t, got)

	// A styled header stays styled and the padded header cell is a plain empty one.
	got = readWide(t, "| **a** |\n| - |\n| x | y |\n")
	b = tablesOf(got)[0]
	if wantH := [][]TextRun{{fxRun("a", "bold")}, nil}; !reflect.DeepEqual(b.HeaderRuns, wantH) {
		t.Fatalf("HeaderRuns = %+v, want %+v", b.HeaderRuns, wantH)
	}
	assertCanonicalCells(t, got)
}

// cellSpellings are cells whose reading depends on the table splitting the row
// before the inline parse (pipes, code spans, math), with what each one reads as.
var cellSpellings = []struct {
	cell string
	text string
	runs []TextRun // nil for a plain cell
}{
	{"plain", "plain", nil},
	{"**b**", "b", []TextRun{fxRun("b", "bold")}},
	{"*i*", "i", []TextRun{fxRun("i", "italic")}},
	{"~~s~~", "s", []TextRun{fxRun("s", "strike")}},
	{"`c`", "c", []TextRun{fxRun("c", "code")}},
	{"$x$", "x", []TextRun{fxRun("x", "math")}},
	{"[l](u)", "l", []TextRun{fxLink("l", "u")}},
	{"<https://a.b>", "https://a.b", []TextRun{fxLink("https://a.b", "https://a.b")}},
	{"<strong>s</strong>", "s", []TextRun{fxRun("s", "bold")}},
	{`p\|q`, "p|q", nil},
	{"`a\\|b`", "a|b", []TextRun{fxRun("a|b", "code")}},
	{"`a\\\\|b`", `a\|b`, []TextRun{fxRun(`a\|b`, "code")}},
	{"`` `\\| ``", "`|", []TextRun{fxRun("`|", "code")}},
	{"`\\|`", "|", []TextRun{fxRun("|", "code")}},
	{"[a\\|b](u)", "a|b", []TextRun{fxLink("a|b", "u")}},
	{"[l](u\\|v)", "l", []TextRun{fxLink("l", "u|v")}},
	{"[l](<u v\\|w>)", "l", []TextRun{fxLink("l", "u v|w")}},
	{"**a\\|b**", "a|b", []TextRun{fxRun("a|b", "bold")}},
	{"$a\\|b$", "a|b", []TextRun{fxRun("a|b", "math")}},
	{"$a\\\\|b$", `a\|b`, []TextRun{fxRun(`a\|b`, "math")}},
	{"$\\|x\\|+\\|y\\|$", "|x|+|y|", []TextRun{fxRun("|x|+|y|", "math")}},
	{"x `p\\|q` y", "x p|q y", []TextRun{fxRun("x "), fxRun("p|q", "code"), fxRun(" y")}},
}

// An excess cell is read exactly as the same cell in a column of the header: the
// same inline reader, the same pipe rules (TRD 26), in strict and in lenient mode.
func TestCodecCellsWideRowStyledAndPipeCells(t *testing.T) {
	cellOf := func(d *Document, row, col int) (string, []TextRun) {
		b := tablesOf(d)[0]
		if row >= len(b.Rows) || col >= len(b.Rows[row]) {
			return "<missing cell>", nil // a dropped cell is a failure, not a panic
		}
		var runs []TextRun
		if row < len(b.RowRuns) {
			runs = cellRunsAt(b.RowRuns[row], col)
		}
		return b.Rows[row][col], runs
	}
	for _, tc := range cellSpellings {
		t.Run(tc.cell, func(t *testing.T) {
			column := readWide(t, "| h |\n| - |\n| "+tc.cell+" |\n")
			colText, colRuns := cellOf(column, 0, 0)
			if colText != tc.text || !reflect.DeepEqual(colRuns, tc.runs) {
				t.Fatalf("the column cell reads as (%q, %+v), want (%q, %+v): fix the table", colText, colRuns, tc.text, tc.runs)
			}

			// The cell is the first excess cell, the second one, and a middle one.
			for _, c := range []struct {
				src string
				idx int
			}{
				{"| h |\n| - |\n| x | " + tc.cell + " |\n", 1},
				{"| h |\n| - |\n| x | y | " + tc.cell + " |\n", 2},
				{"| h |\n| - |\n| x | " + tc.cell + " | z |\n", 1},
			} {
				d := readWide(t, c.src)
				b := tablesOf(d)[0]
				txt, runs := cellOf(d, 0, c.idx)
				if txt != tc.text || !reflect.DeepEqual(runs, tc.runs) {
					t.Fatalf("source %q: excess cell %d reads as (%q, %+v), want (%q, %+v)\n%s", c.src, c.idx, txt, runs, tc.text, tc.runs, docJSON(t, d))
				}
				if len(b.Headers) != len(b.Rows[0]) || len(b.Aligns) != len(b.Headers) {
					t.Fatalf("source %q: header %d, aligns %d, row %d: not one width", c.src, len(b.Headers), len(b.Aligns), len(b.Rows[0]))
				}
				assertCanonicalCells(t, d)
				assertCellIdentity(t, d)
			}
		})
	}
}

// Identity: the padded document is a fixed point of write-then-read, and the
// Markdown it writes is stable. The writer needs no change: an empty header cell
// is `|  |` and an unaligned column is `---`.
func TestCodecCellsWideRowIdentity(t *testing.T) {
	srcs := []string{
		"| a |\n| - |\n| x | zqsentinel |\n",
		"| a | b |\n| :-- | --: |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 | 7 |\n",
		"| a |\n| - |\n| 1 | 2 |\n| 3 | 4 | 5 |\n| 6 |\n",
		"| **a** |\n| :-: |\n| x | `c\\|d` | $e\\|f$ | [g\\|h](u\\|v) |\n",
		"a | b\n--- | ---\n1 | 2 | 3\n",
		"| a |\n| - |\n| x | |\n",
	}
	for _, src := range srcs {
		d := readWide(t, src)
		assertCellIdentity(t, d)
	}

	// The canonical form of the first, pinned.
	d := readWide(t, srcs[0])
	if got, want := ToMarkdown(d), "| a |  |\n| --- | --- |\n| x | zqsentinel |\n"; got != want {
		t.Fatalf("ToMarkdown\n got: %q\nwant: %q", got, want)
	}
}

// Rows no wider than the header read as before: no padding of the header, and a
// narrower row keeps GFM's empty-cell padding.
func TestCodecCellsNarrowAndEqualRowsAreUnchanged(t *testing.T) {
	cases := []struct {
		src  string
		want *Document
	}{
		{"| a | b |\n| - | - |\n| 1 | 2 |\n", wideTableDoc([]string{"a", "b"}, []string{"", ""}, [][]string{{"1", "2"}})},
		{"| a | b |\n| - | - |\n| 1 |\n| 2 | 3 |\n", wideTableDoc([]string{"a", "b"}, []string{"", ""}, [][]string{{"1", ""}, {"2", "3"}})},
		{"| a | b |\n| :- | -: |\n", wideTableDoc([]string{"a", "b"}, []string{"left", "right"}, nil)},
		{"| a |  |\n| --- | --- |\n| x | y |\n", wideTableDoc([]string{"a", ""}, []string{"", ""}, [][]string{{"x", "y"}})},
	}
	for _, tc := range cases {
		assertDocEqual(t, stripIDsAndSpans(readWide(t, tc.src)), tc.want)
	}
}

// Where the table is: a quote, a toggle, a todo, a section other than the first,
// a document with several tables of which only some are wide.
func TestCodecCellsWideRowInContainersAndSections(t *testing.T) {
	wide := func(t *testing.T, d *Document, i int) {
		t.Helper()
		tables := tablesOf(d)
		if i >= len(tables) {
			t.Fatalf("want a table at index %d, the document has %d\n%s", i, len(tables), docJSON(t, d))
		}
		b := tables[i]
		if !reflect.DeepEqual(b.Headers, []string{"a", ""}) || !reflect.DeepEqual(b.Aligns, []string{"", ""}) || !reflect.DeepEqual(b.Rows, [][]string{{"x", "zqsentinel"}}) {
			t.Fatalf("table %d: Headers %q, Aligns %q, Rows %q", i, b.Headers, b.Aligns, b.Rows)
		}
	}
	cases := []struct{ name, src string }{
		{"quote", "> | a |\n> | - |\n> | x | zqsentinel |\n"},
		{"toggle", ":::toggle\n| a |\n| - |\n| x | zqsentinel |\n:::\n"},
		{"todo", "- [ ] t\n\n  | a |\n  | - |\n  | x | zqsentinel |\n"},
		{"callout", ":::callout\nlead\n\n| a |\n| - |\n| x | zqsentinel |\n:::\n"},
		{"nested_quote", "> > | a |\n> > | - |\n> > | x | zqsentinel |\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := readWide(t, tc.src)
			wide(t, d, 0)
			assertCellIdentity(t, d)
		})
	}

	t.Run("sections", func(t *testing.T) {
		src := "# One\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n---\n\n# Two\n\n| a |\n| - |\n| x | zqsentinel |\n\n| c |\n| - |\n| 3 |\n"
		d := readWide(t, src)
		if len(d.Sections) != 2 {
			t.Fatalf("want 2 sections, got %d\n%s", len(d.Sections), docJSON(t, d))
		}
		tables := tablesOf(d)
		if len(tables) != 3 {
			t.Fatalf("want 3 tables, got %d", len(tables))
		}
		if !reflect.DeepEqual(tables[0].Headers, []string{"a", "b"}) || !reflect.DeepEqual(tables[0].Rows, [][]string{{"1", "2"}}) {
			t.Fatalf("the narrow table in section one changed: %q %q", tables[0].Headers, tables[0].Rows)
		}
		wide(t, d, 1)
		if !reflect.DeepEqual(tables[2].Headers, []string{"c"}) || !reflect.DeepEqual(tables[2].Rows, [][]string{{"3"}}) {
			t.Fatalf("the narrow table after the wide one changed: %q %q", tables[2].Headers, tables[2].Rows)
		}
		assertCellIdentity(t, d)
	})
}

// No new refusal class: a wide row is accepted in strict mode, and the refusals a
// cell already has apply to an excess cell under the same names and at the same
// place (the cell's own offset for an unclosed tag, the construct's for the rest).
func TestCodecCellsWideRowRefusalsMatchColumnCells(t *testing.T) {
	cases := []struct {
		name, cell, construct, at string
	}{
		{"image", "![alt](b)", "inline-image", "![alt]"},
		{"raw_html", "y<br>z", "raw-html", "<br>"},
		{"link_title", `[l](u "t")`, "link-title", "[l]"},
		{"unclosed_mark_tag", "<strong>y", "raw-html", " <strong>y"},
		{"inline_display_math", "$$x$$", "inline-display-math", "$$x$$"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "| a |\n| --- |\n| x | " + tc.cell + " |\n"
			_, err := FromMarkdown(src, FromMarkdownOptions{})
			var ue *UnrepresentableError
			if !errors.As(err, &ue) {
				t.Fatalf("want *UnrepresentableError, got %v", err)
			}
			if ue.Construct != tc.construct {
				t.Fatalf("Construct = %q, want %q (%v)", ue.Construct, tc.construct, err)
			}
			if want := strings.Index(src, tc.at); ue.Offset != want {
				t.Fatalf("Offset = %d, want %d", ue.Offset, want)
			}
			lenient := mustFromMarkdown(t, src, FromMarkdownOptions{Lenient: true})
			b := tablesOf(lenient)[0]
			if len(b.Rows[0]) != 2 || b.Rows[0][0] != "x" || b.Rows[0][1] == "" {
				t.Fatalf("lenient must keep the excess cell, got %q", b.Rows)
			}
			assertCellIdentity(t, lenient)
		})
	}
	t.Run("a_wide_row_alone_is_accepted_by_strict_mode", func(t *testing.T) {
		if _, err := FromMarkdown("| a |\n| - |\n| x | y | z |\n", FromMarkdownOptions{}); err != nil {
			t.Fatalf("strict refused a wide row: %v", err)
		}
	})
}

// Spans: a table whose last row is wide ends at its last excess cell, not at the
// last cell the header had room for.
func TestCodecCellsWideRowSpanCoversTheExcessCell(t *testing.T) {
	src := "intro\n\n| a |\n| - |\n| x | zqsentinel |\n"
	d := mustFromMarkdown(t, src, FromMarkdownOptions{Spans: true})
	b := tablesOf(d)[0]
	if b.Span == nil {
		t.Fatalf("the table has no span")
	}
	end := strings.Index(src, "zqsentinel") + len("zqsentinel")
	if b.Span.Stop < end || b.Span.Stop > len(src) {
		t.Fatalf("span %+v does not cover the excess cell (ends at %d, the input has %d bytes)", *b.Span, end, len(src))
	}
	// The span starts in the header row (at its first cell's text), as any table's does.
	if row := strings.Index(src, "| a |"); b.Span.Start < row || b.Span.Start > row+len("| a |") {
		t.Fatalf("span %+v starts in the wrong place", *b.Span)
	}
}

// The row splitter is a copy of goldmark's (its AST keeps no excess cell to read
// them from), so it is pinned against goldmark: for a header wide enough to hold
// the row, the cells the splitter finds are the cells goldmark made, for any mix
// of pipes, backticks, backslashes and spaces. A goldmark upgrade that changes how
// a row splits fails here instead of silently disagreeing about where an excess
// cell starts.
func TestCodecCellsRowSplitterMatchesGoldmark(t *testing.T) {
	const width = 6
	rng := rand.New(rand.NewPCG(1026, 31))
	alphabet := []string{"a", "b", "|", "|", "\\", "`", " ", " ", "$", "*"}
	header := "|" + strings.Repeat(" h |", width) + "\n|" + strings.Repeat(" - |", width) + "\n"
	compared := 0
	for i := 0; i < 4000; i++ {
		var rb strings.Builder
		for n := 1 + rng.IntN(14); n > 0; n-- {
			rb.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		row := rb.String()
		src := []byte(header + row + "\n")
		root := mdEngine.Parser().Parse(text.NewReader(src))
		tbl, ok := root.FirstChild().(*east.Table)
		if !ok || tbl.ChildCount() != 2 {
			continue // the row is not part of the table (a blank line, a list, a fence)
		}
		body := tbl.LastChild()
		var want []text.Segment
		for c := body.FirstChild(); c != nil; c = c.NextSibling() {
			if lines := c.Lines(); lines.Len() > 0 {
				want = append(want, lines.At(0))
			}
		}
		if len(want) > width {
			want = want[:width] // the cells after the header's width were added by the codec
		}
		got := splitTableRow(text.NewSegment(len(header), len(header)+len(row)), src)
		if len(got) > width {
			got = got[:width]
		}
		if len(got) != len(want) {
			t.Fatalf("row %q: the splitter found %d cells, goldmark %d", row, len(got), len(want))
		}
		for k := range got {
			g := text.NewSegment(got[k].start, got[k].stop)
			g = g.TrimLeftSpace(src)
			g = g.TrimRightSpace(src)
			if g.Start != want[k].Start || g.Stop != want[k].Stop {
				t.Fatalf("row %q cell %d: the splitter has [%d,%d), goldmark [%d,%d)", row, k, g.Start, g.Stop, want[k].Start, want[k].Stop)
			}
		}
		compared++
	}
	if compared < 1000 {
		t.Fatalf("only %d of 4000 rows reached a table, so the test says little", compared)
	}
}
