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
	"reflect"
	"strings"
	"testing"
)

// Formatted table cells in the Markdown codec (objective 1026, TRD 26, AODex D14
// part 2). A cell is read through the same inline walker as a paragraph, so it
// holds bold, italic, strike, code, inline math, links and autolinks, and it
// refuses exactly what a paragraph refuses. The runs land in Block.HeaderRuns and
// Block.RowRuns in the canonical form TRD 25 defined.

// cellTable is a one-column table with a plain header and one body row whose only
// cell holds runs.
func cellTable(runs ...TextRun) *Document {
	return fxDoc([]Block{{
		Kind:    BlockTable,
		Headers: []string{"h"}, Aligns: []string{""},
		Rows:    [][]string{{FlattenRuns(runs)}},
		RowRuns: [][][]TextRun{{runs}},
	}})
}

// assertCanonicalCells fails when d is not in the canonical form EnsureTextRunsInvariant
// puts it in (TRD 25), which is what a strict read must produce.
func assertCanonicalCells(t *testing.T, d *Document) {
	t.Helper()
	inv := cloneDoc(d)
	EnsureTextRunsInvariant(inv)
	if diff := deepDiff(inv, cloneDoc(d)); diff != "" {
		t.Fatalf("the read is not in canonical cell form (EnsureTextRunsInvariant changes it at %s):\n%s", diff, docJSON(t, d))
	}
}

// assertCellIdentity is the identity claim for one document: FromMarkdown(ToMarkdown(d))
// equals d with no Prev (modulo ids) and with Prev (ids included), and the Markdown
// is a fixed point.
func assertCellIdentity(t *testing.T, d *Document) {
	t.Helper()
	if err := propValid(d); err != nil {
		t.Fatalf("bad test document: %v\n%s", err, docJSON(t, d))
	}
	md := ToMarkdown(d)
	got, err := FromMarkdown(md, FromMarkdownOptions{})
	if err != nil {
		t.Fatalf("strict read of canonical Markdown failed: %v\nmarkdown: %q", err, md)
	}
	if diff := deepDiff(stripIDsAndSpans(cloneDoc(got)), stripIDsAndSpans(cloneDoc(d))); diff != "" {
		t.Fatalf("FromMarkdown(ToMarkdown(d)) != d at %s\nmarkdown: %q\n--- got ---\n%s\n--- want ---\n%s", diff, md, docJSON(t, got), docJSON(t, d))
	}
	if again := ToMarkdown(got); again != md {
		t.Fatalf("Markdown is not a fixed point\n first: %q\nsecond: %q", md, again)
	}
	withIDs := fxIDs(cloneDoc(d))
	mdIDs := ToMarkdown(withIDs)
	prev, err := FromMarkdown(mdIDs, FromMarkdownOptions{Prev: withIDs})
	if err != nil {
		t.Fatalf("strict read with Prev failed: %v\nmarkdown: %q", err, mdIDs)
	}
	if diff := deepDiff(prev, withIDs); diff != "" {
		t.Fatalf("FromMarkdown(ToMarkdown(d), {Prev: d}) != d at %s\nmarkdown: %q", diff, mdIDs)
	}
}

// Test list 1: strict accepts every kind of inline formatting in a header and in
// a body cell, reads it into HeaderRuns/RowRuns, and keeps the flattened text in
// Headers/Rows.
func TestCodecTableRunsStrictAccepts(t *testing.T) {
	const row = "| **b** | *i* | ~~s~~ | `c` | $x$ | [l](u) | <https://a.b> |\n"
	src := row + "| --- | --- | --- | --- | --- | --- | --- |\n" + row

	runs := [][]TextRun{
		{fxRun("b", "bold")}, {fxRun("i", "italic")}, {fxRun("s", "strike")}, {fxRun("c", "code")},
		{fxRun("x", "math")}, {fxLink("l", "u")}, {fxLink("https://a.b", "https://a.b")},
	}
	texts := []string{"b", "i", "s", "c", "x", "l", "https://a.b"}
	want := fxDoc([]Block{{
		Kind:    BlockTable,
		Headers: texts, Aligns: make([]string, 7),
		Rows:       [][]string{texts},
		HeaderRuns: runs,
		RowRuns:    [][][]TextRun{runs},
	}})

	got := mustFromMarkdown(t, src, FromMarkdownOptions{})
	assertDocEqual(t, stripIDsAndSpans(got), stripIDsAndSpans(want))
	assertCanonicalCells(t, got)

	// The same cells under lenient mode: nothing to map, so the same document.
	lenient := mustFromMarkdown(t, src, FromMarkdownOptions{Lenient: true})
	assertDocEqual(t, stripIDsAndSpans(lenient), stripIDsAndSpans(want))

	// And a document that holds them writes Markdown that reads back as itself.
	assertCellIdentity(t, want)
}

// A plain table keeps the shape it had before D14: neither key, plain cell text.
func TestCodecTableRunsPlainTableHasNoRuns(t *testing.T) {
	got := mustFromMarkdown(t, "| a | b |\n| --- | --- |\n| 1 | 2 |\n", FromMarkdownOptions{})
	b := got.Sections[0].Blocks[0]
	if b.HeaderRuns != nil || b.RowRuns != nil {
		t.Fatalf("a plain table must have no cell runs: %+v / %+v", b.HeaderRuns, b.RowRuns)
	}
}

// A cell that only looks styled (escaped delimiters) is plain, and a table with
// one styled cell has its other cells null.
func TestCodecTableRunsOnlyStyledCellsCarryRuns(t *testing.T) {
	got := mustFromMarkdown(t, "| a | \\*b\\* |\n| --- | --- |\n| **c** | d |\n| e | f |\n", FromMarkdownOptions{})
	b := got.Sections[0].Blocks[0]
	if b.HeaderRuns != nil {
		t.Fatalf("an escaped delimiter is plain text, but HeaderRuns = %+v", b.HeaderRuns)
	}
	if b.Headers[1] != "*b*" {
		t.Fatalf("Headers[1] = %q, want *b*", b.Headers[1])
	}
	want := [][][]TextRun{{{fxRun("c", "bold")}, nil}, nil}
	if !reflect.DeepEqual(b.RowRuns, want) {
		t.Fatalf("RowRuns = %+v, want %+v", b.RowRuns, want)
	}
	assertCanonicalCells(t, got)
}

// Test list 2: pipes. Every | is escaped, inside code spans too; GFM un-escapes a
// code span's before the inline parse.
func TestCodecTableRunsPipes(t *testing.T) {
	cases := []struct {
		name string
		runs []TextRun
		cell string // the exact spelling, or "" to check the round trip only
	}{
		{"code_with_pipe", []TextRun{fxRun("a|b", "code")}, "`a\\|b`"},
		{"code_with_backslash_before_pipe", []TextRun{fxRun(`a\|b`, "code")}, "`a\\\\|b`"},
		{"code_with_backtick_and_pipe", []TextRun{fxRun("`|", "code")}, "`` `\\| ``"},
		{"code_that_is_one_pipe", []TextRun{fxRun("|", "code")}, "`\\|`"},
		{"link_text_with_pipe", []TextRun{fxLink("a|b", "u")}, "[a\\|b](u)"},
		{"link_destination_with_pipe", []TextRun{fxLink("l", "u|v")}, "[l](u\\|v)"},
		{"link_destination_with_space_and_pipe", []TextRun{fxLink("l", "u v|w")}, "[l](<u v\\|w>)"},
		{"bold_text_with_pipe", []TextRun{fxRun("a|b", "bold")}, "**a\\|b**"},
		{"math_with_pipe", []TextRun{fxRun("a|b", "math")}, "$a\\|b$"},
		{"math_with_backslash_pipe", []TextRun{fxRun(`a\|b`, "math")}, "$a\\\\|b$"},
		{"math_with_two_pipes", []TextRun{fxRun(`|x|+|y|`, "math")}, "$\\|x\\|+\\|y\\|$"},
		{"plain_pipe_beside_bold", []TextRun{fxRun("x|y"), fxRun("z", "bold")}, ""},
		{"code_pipe_beside_plain_pipe", []TextRun{fxRun("p|q "), fxRun("a|b", "code"), fxRun(" r|s")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := cellTable(tc.runs...)
			md := ToMarkdown(d)
			if tc.cell != "" {
				want := "| h |\n| --- |\n| " + tc.cell + " |\n"
				if md != want {
					t.Fatalf("ToMarkdown\n got: %q\nwant: %q", md, want)
				}
				// The literal spelling reads as the same document.
				lit := mustFromMarkdown(t, want, FromMarkdownOptions{})
				assertDocEqual(t, stripIDsAndSpans(lit), stripIDsAndSpans(d))
			}
			assertCellIdentity(t, d)
		})
	}
}

// Test list 3: identity over the shapes a real table has.
func TestCodecTableRunsIdentity(t *testing.T) {
	t.Run("mixed_cell", func(t *testing.T) {
		assertCellIdentity(t, cellTable(fxRun("plain "), fxRun("bold", "bold"), fxRun(" "), fxLink("l", "u")))
	})
	t.Run("edge_spaces", func(t *testing.T) {
		assertCellIdentity(t, cellTable(fxRun("  "), fxRun("b", "bold"), fxRun("   ")))
		assertCellIdentity(t, cellTable(fxRun(" x"), fxRun("c", "code"), fxRun("\t")))
		assertCellIdentity(t, cellTable(fxRun(" ", "code")))
	})
	t.Run("empty_cell_beside_a_styled_one", func(t *testing.T) {
		d := fxDoc([]Block{{
			Kind:    BlockTable,
			Headers: []string{"", "h"}, Aligns: []string{"", "center"},
			Rows: [][]string{{"", "b"}, {"c", ""}},
			// Cells 0 of row 0 and 1 of row 1 are empty and plain (null).
			HeaderRuns: [][]TextRun{nil, {fxRun("h", "italic")}},
			RowRuns:    [][][]TextRun{{nil, {fxRun("b", "bold")}}, {{fxRun("c", "strike")}, nil}},
		}})
		assertCellIdentity(t, d)
	})
	t.Run("rows_with_and_without_runs", func(t *testing.T) {
		// A table whose rows are partly styled: row 0 has no runs at all (null), row 1
		// styles its last cell, row 2 its first.
		d := fxDoc([]Block{{
			Kind:    BlockTable,
			Headers: []string{"a", "b", "c"}, Aligns: []string{"left", "", "right"},
			Rows:    [][]string{{"1", "2", "3"}, {"4", "5", "six"}, {"seven", "8", "9"}},
			RowRuns: [][][]TextRun{nil, {nil, nil, {fxRun("six", "code")}}, {{fxLink("seven", "https://x.io")}, nil, nil}},
		}})
		assertCellIdentity(t, d)
	})
	t.Run("empty_header_row_with_styled_body", func(t *testing.T) {
		d := fxDoc([]Block{{
			Kind:    BlockTable,
			Headers: []string{"", ""}, Aligns: []string{"", ""},
			Rows:    [][]string{{"a", "b"}},
			RowRuns: [][][]TextRun{{{fxRun("a", "bold")}, {fxRun("b", "italic")}}},
		}})
		assertCellIdentity(t, d)
	})
	t.Run("header_only_styled", func(t *testing.T) {
		d := fxDoc([]Block{{
			Kind:       BlockTable,
			Headers:    []string{"Name", "Count"},
			Aligns:     []string{"", "right"},
			HeaderRuns: [][]TextRun{{fxRun("Name", "bold")}, {fxRun("Count", "bold")}},
			Rows:       [][]string{{"x", "1"}},
		}})
		assertCellIdentity(t, d)
	})
	t.Run("nested_marks_and_adjacent_runs", func(t *testing.T) {
		assertCellIdentity(t, cellTable(
			fxRun("a", "bold", "italic"), fxRun(" b ", "code"), fxRun("c", "bold", "strike"), fxLink("d", "u", "bold"), fxRun("e", "math"),
		))
	})
	t.Run("a_table_inside_a_container", func(t *testing.T) {
		tbl := cellTable(fxRun("x", "bold")).Sections[0].Blocks[0]
		assertCellIdentity(t, fxDoc([]Block{{Kind: BlockToggle, Text: "t", Children: []Block{tbl}}}))
		assertCellIdentity(t, fxDoc([]Block{{Kind: BlockQuote, Text: "q", Children: []Block{tbl}}}))
	})
}

// A hand-written cell that reads into runs gets the same Markdown back, whatever
// the source spelled (underscore emphasis, a reference link, an autolink, a
// character reference).
func TestCodecTableRunsCanonicalizesSpellings(t *testing.T) {
	src := "| _i_ | __b__ | [r][1] | <https://a.b/c> | a&amp;b **x** |\n| --- | --- | --- | --- | --- |\n| ***bi*** | <strong>s</strong> | `` a`b `` | <em>e</em><del>d</del> | x |\n\n[1]: https://r.io\n"
	got := mustFromMarkdown(t, src, FromMarkdownOptions{})
	assertCanonicalCells(t, got)
	b := got.Sections[0].Blocks[0]
	wantHead := [][]TextRun{
		{fxRun("i", "italic")}, {fxRun("b", "bold")}, {fxLink("r", "https://r.io")}, {fxLink("https://a.b/c", "https://a.b/c")},
		{fxRun("a&b "), fxRun("x", "bold")},
	}
	if !reflect.DeepEqual(b.HeaderRuns, wantHead) {
		t.Fatalf("HeaderRuns = %+v\nwant         %+v", b.HeaderRuns, wantHead)
	}
	wantRow := [][]TextRun{
		{fxRun("bi", "bold", "italic")}, {fxRun("s", "bold")}, {fxRun("a`b", "code")}, {fxRun("e", "italic"), fxRun("d", "strike")}, nil,
	}
	if !reflect.DeepEqual(b.RowRuns[0], wantRow) {
		t.Fatalf("RowRuns[0] = %+v\nwant         %+v", b.RowRuns[0], wantRow)
	}
	// Whatever the spelling was, the document writes and reads back as itself.
	assertCellIdentity(t, got)
}

// Test list 4: a cell refuses what a paragraph refuses, under the same names.
func TestCodecTableRunsRefusalsAreUnchanged(t *testing.T) {
	cases := []struct {
		name, src, construct, at string
	}{
		{"raw_html", "| a |\n| --- |\n| x<br>y |\n", "raw-html", "<br>"},
		{"raw_html_in_header", "| a<br>b |\n| --- |\n| x |\n", "raw-html", "<br>"},
		{"image", "| a |\n| --- |\n| ![a](b) |\n", "inline-image", "![a]"},
		{"link_title", "| a |\n| --- |\n| [l](u \"t\") |\n", "link-title", "[l]"},
		{"empty_link", "| a |\n| --- |\n| [](u) |\n", "empty-link", "[]("},
		{"unclosed_mark_tag", "| a |\n| --- |\n| <strong>x |\n", "raw-html", "<strong>"},
		{"inline_display_math", "| a |\n| --- |\n| $$x$$ |\n", "inline-display-math", "$$x$$"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := FromMarkdown(tc.src, FromMarkdownOptions{})
			var ue *UnrepresentableError
			if !errors.As(err, &ue) {
				t.Fatalf("want *UnrepresentableError, got %v (doc %v)", err, doc != nil)
			}
			if ue.Construct != tc.construct {
				t.Fatalf("Construct = %q, want %q (%v)", ue.Construct, tc.construct, err)
			}
			if want := strings.Index(tc.src, tc.at); ue.Offset != want {
				t.Fatalf("Offset = %d, want %d", ue.Offset, want)
			}
			if _, err := FromMarkdown(tc.src, FromMarkdownOptions{Lenient: true}); err != nil {
				t.Fatalf("lenient must not refuse: %v", err)
			}
		})
	}
}

// Lenient mode keeps every bit of formatting in a cell and maps the refused
// construct the way it does in a paragraph.
func TestCodecTableRunsLenientKeepsFormatting(t *testing.T) {
	cases := []struct {
		name, src string
		header    [][]TextRun
		row       [][]TextRun
		text      string // Rows[0][0]
	}{
		{
			name: "raw_html_is_literal_text_beside_bold",
			src:  "| a |\n| --- |\n| x<br>y **z** |\n",
			row:  [][]TextRun{{fxRun("x<br>y "), fxRun("z", "bold")}},
			text: "x<br>y z",
		},
		{
			name: "image_is_its_alt_text_beside_a_link",
			src:  "| a |\n| --- |\n| ![alt](b) [l](u) |\n",
			row:  [][]TextRun{{fxRun("alt "), fxLink("l", "u")}},
			text: "alt l",
		},
		{
			name: "link_title_is_dropped_but_the_link_stays",
			src:  "| a |\n| --- |\n| [l](u \"t\") |\n",
			row:  [][]TextRun{{fxLink("l", "u")}},
			text: "l",
		},
		{
			name: "display_math_is_inline_math",
			src:  "| a |\n| --- |\n| $$x$$ |\n",
			row:  [][]TextRun{{fxRun("x", "math")}},
			text: "x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFromMarkdown(t, tc.src, FromMarkdownOptions{Lenient: true})
			assertCanonicalCells(t, got)
			b := got.Sections[0].Blocks[0]
			if !reflect.DeepEqual(b.RowRuns, [][][]TextRun{tc.row}) {
				t.Fatalf("RowRuns = %+v, want %+v", b.RowRuns, [][][]TextRun{tc.row})
			}
			if b.Rows[0][0] != tc.text {
				t.Fatalf("Rows[0][0] = %q, want %q", b.Rows[0][0], tc.text)
			}
			// The lenient read round-trips with identity.
			assertCellIdentity(t, got)
		})
	}
}

// Test list 5: where delimiters would not read back inside a cell, the writer uses
// the tags FromMarkdown reads, and the cell comes back identical.
func TestCodecTableRunsWriterFallsBackToTags(t *testing.T) {
	cases := []struct {
		name string
		runs []TextRun
		tag  string // a substring the cell must contain
	}{
		{"strike_after_a_letter", []TextRun{fxRun("x"), fxRun("y", "bold", "strike")}, "<del>"},
		{"bold_between_punctuation_and_letter", []TextRun{fxRun("("), fxRun("b", "bold"), fxRun("c")}, ""},
		{"italic_inside_a_word", []TextRun{fxRun("a"), fxRun("b", "italic"), fxRun("c")}, ""},
		{"strike_before_a_pipe", []TextRun{fxRun("|"), fxRun("s", "strike"), fxRun("|")}, ""},
		{"bold_code_beside_a_letter", []TextRun{fxRun("k"), fxRun("c", "bold", "code"), fxRun("m")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := cellTable(tc.runs...)
			md := ToMarkdown(d)
			if tc.tag != "" && !strings.Contains(md, tc.tag) {
				t.Fatalf("want the tag form %q in the cell:\n%s", tc.tag, md)
			}
			assertCellIdentity(t, d)
		})
	}
}

// A cell mode check, not an assumption: the cell is read back as a cell of a
// real table. A string that reads back as a different cell count (an unescaped
// pipe) is not accepted.
func TestCodecTableRunsCellReadBack(t *testing.T) {
	want := []TextRun{fxRun("a", "bold"), fxRun("b")}
	if !readsBackAs("**a**b", want, inlineCell) {
		t.Fatalf("a plain bold cell must read back")
	}
	if readsBackAs("**a**b | c", want, inlineCell) {
		t.Fatalf("an unescaped pipe makes a second cell: not the same runs")
	}
	if readsBackAs("**a**", want, inlineCell) {
		t.Fatalf("the bold run alone is not the same runs")
	}
	// What only a cell reads: GFM un-escapes \| in a code span before the inline
	// parse, so the cell has code "a|b" where a paragraph has code "a\|b"; and a
	// leading "# " is text in a cell, a heading in a paragraph.
	if !readsBackAs("`a\\|b`", []TextRun{fxRun("a|b", "code")}, inlineCell) {
		t.Fatalf("a code span with an escaped pipe must read back as code a|b in a cell")
	}
	if !readsBackAs("# x**b**", []TextRun{fxRun("# x"), fxRun("b", "bold")}, inlineCell) {
		t.Fatalf("a cell that starts with # is text, not a heading")
	}
}
