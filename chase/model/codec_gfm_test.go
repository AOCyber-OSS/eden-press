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
	"errors"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestToMarkdownFixturesExactBytes pins ToMarkdown's output for every GFM kind
// byte for byte (objective 1026 TRD 01, Test list 3).
func TestToMarkdownFixturesExactBytes(t *testing.T) {
	for _, fx := range codecFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			got := ToMarkdown(fx.doc())
			if got != fx.md {
				t.Fatalf("ToMarkdown mismatch\n got: %q\nwant: %q", got, fx.md)
			}
		})
	}
}

// TestToMarkdownEscapesMetacharacters pins how plain text containing Markdown
// metacharacters is spelled so that it re-parses as the same text (Test list 4).
func TestToMarkdownEscapesMetacharacters(t *testing.T) {
	for _, tc := range codecEscapeCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := ToMarkdown(fxDoc([]Block{fxPara(tc.text)}))
			want := tc.md + "\n"
			if got != want {
				t.Fatalf("ToMarkdown(%q)\n got: %q\nwant: %q", tc.text, got, want)
			}
		})
	}
}

// TestToMarkdownIsDeterministic: the same Document yields the same bytes on
// every call, and Meta.Directives / Section.Attrs are emitted in sorted key
// order (Test list 1). Go map order is randomized per range, so 50 calls over
// 5 + 4 keys would differ with probability ~1 under the old serializer.
func TestToMarkdownIsDeterministic(t *testing.T) {
	build := func() *Document {
		d := fxDoc([]Block{fxPara("body")})
		d.Meta.Directives = map[string]string{
			"theme": "gaia", "size": "16:9", "paginate": "true", "title": "T", "author": "A",
		}
		d.Sections[0].Attrs = map[string]string{
			"class": "lead", "style": "x", "data-theme": "y", "lang": "en",
		}
		return d
	}
	const want = "---\n" +
		"author: A\n" +
		"paginate: true\n" +
		"size: \"16:9\"\n" +
		"theme: gaia\n" +
		"title: T\n" +
		"---\n\n" +
		"<!-- class: lead -->\n" +
		"<!-- data-theme: y -->\n" +
		"<!-- lang: en -->\n" +
		"<!-- style: x -->\n" +
		"body\n"
	doc := build()
	for i := 0; i < 50; i++ {
		if got := ToMarkdown(doc); got != want {
			t.Fatalf("call %d differs\n got: %q\nwant: %q", i, got, want)
		}
	}
	// A fresh Document with freshly allocated maps (new hash seeds) must agree too.
	for i := 0; i < 20; i++ {
		if got := ToMarkdown(build()); got != want {
			t.Fatalf("fresh doc %d differs\n got: %q\nwant: %q", i, got, want)
		}
	}
}

// TestToMarkdownEmitsNoDerivedAttrs: ToMarkdown emits only what the Document
// holds. It never invents a data-theme / style comment from the theme
// directive, and it emits those keys verbatim when they ARE held (Test list 2).
func TestToMarkdownEmitsNoDerivedAttrs(t *testing.T) {
	d := fxDoc([]Block{fxPara("body")})
	d.Meta.Directives = map[string]string{"theme": "gaia"}
	got := ToMarkdown(d)
	for _, banned := range []string{"data-theme", "style", "<!--"} {
		if strings.Contains(got, banned) {
			t.Fatalf("ToMarkdown injected %q:\n%s", banned, got)
		}
	}

	held := fxDoc([]Block{fxPara("body")})
	held.Sections[0].Attrs = map[string]string{"data-theme": "gaia"}
	got = ToMarkdown(held)
	if n := strings.Count(got, "<!-- data-theme: gaia -->"); n != 1 {
		t.Fatalf("held data-theme emitted %d times, want 1:\n%s", n, got)
	}
	if strings.Contains(got, "style") {
		t.Fatalf("a style comment appeared that the Document does not hold:\n%s", got)
	}
}

// TestToMarkdownMathIsNotDuplicated is the regression for the lossy probe in
// RESEARCH 1.4: inline `$a^2$` followed by display `$$E=mc^2$$` used to emit the
// inline math a second time as a standalone block and to pad the display block
// with blank lines (Test list 7).
func TestToMarkdownMathIsNotDuplicated(t *testing.T) {
	d := fxDoc([]Block{
		fxRich(fxRun("Inline "), fxRun("a^2", "math"), fxRun(" then")),
		{Kind: BlockMath, Display: true, Text: "E=mc^2"},
	})
	got := ToMarkdown(d)
	if n := strings.Count(got, "a^2"); n != 1 {
		t.Fatalf("inline math appears %d times, want 1:\n%s", n, got)
	}
	if n := strings.Count(got, "E=mc^2"); n != 1 {
		t.Fatalf("display math appears %d times, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "$$\nE=mc^2\n$$") {
		t.Fatalf("display math must have no blank line inside $$:\n%s", got)
	}
}

// TestToMarkdownNilDocument keeps the long-standing nil contract.
func TestToMarkdownNilDocument(t *testing.T) {
	if got := ToMarkdown(nil); got != "" {
		t.Fatalf("ToMarkdown(nil) = %q, want empty", got)
	}
}

// ---- FromMarkdown (objective 1026 TRD 01, Task 2) ----

// stripIDsAndSpans zeroes every block/list-item id and every span in d and
// returns d. TRD 01 neither assigns ids nor records spans (TRD 02 does), so
// this is a no-op today; the identity tests call it so that they keep meaning
// "identity modulo ids and spans" once TRD 02 lands.
func stripIDsAndSpans(d *Document) *Document {
	var walk func([]Block)
	walk = func(blocks []Block) {
		for i := range blocks {
			blocks[i].ID = ""
			blocks[i].Span = nil
			for j := range blocks[i].Items {
				blocks[i].Items[j].ID = ""
			}
			walk(blocks[i].Children)
		}
	}
	for i := range d.Sections {
		d.Sections[i].Span = nil
		walk(d.Sections[i].Blocks)
	}
	for i := range d.Outline {
		d.Outline[i].Span = nil
	}
	return d
}

func docJSON(t *testing.T, d *Document) string {
	t.Helper()
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertDocEqual(t *testing.T, got, want *Document) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("documents differ\n--- got ---\n%s\n--- want ---\n%s", docJSON(t, got), docJSON(t, want))
	}
}

func mustFromMarkdown(t *testing.T, md string, opts FromMarkdownOptions) *Document {
	t.Helper()
	d, err := FromMarkdown(md, opts)
	if err != nil {
		t.Fatalf("FromMarkdown(%q): %v", md, err)
	}
	return d
}

// TestCodecIdentityFixtures is the core claim: for every GFM kind a hand-built
// Document survives ToMarkdown then FromMarkdown unchanged, the Markdown is a
// fixed point, and the literal expected Markdown reads as the same Document
// (Test list 3 and 7).
func TestCodecIdentityFixtures(t *testing.T) {
	for _, fx := range codecFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			want := stripIDsAndSpans(fx.doc())
			md := ToMarkdown(fx.doc())

			got := stripIDsAndSpans(mustFromMarkdown(t, md, FromMarkdownOptions{}))
			assertDocEqual(t, got, want)

			if again := ToMarkdown(got); again != md {
				t.Fatalf("Markdown is not a fixed point\n first: %q\nsecond: %q", md, again)
			}

			fromLiteral := stripIDsAndSpans(mustFromMarkdown(t, fx.md, FromMarkdownOptions{}))
			assertDocEqual(t, fromLiteral, want)
		})
	}
}

// TestCodecIdentityEscaping: text containing Markdown metacharacters re-parses
// as exactly the same plain Text (Test list 4).
func TestCodecIdentityEscaping(t *testing.T) {
	for _, tc := range codecEscapeCases() {
		t.Run(tc.name, func(t *testing.T) {
			want := stripIDsAndSpans(fxDoc([]Block{fxPara(tc.text)}))
			got := stripIDsAndSpans(mustFromMarkdown(t, ToMarkdown(want), FromMarkdownOptions{}))
			assertDocEqual(t, got, want)

			fromLiteral := stripIDsAndSpans(mustFromMarkdown(t, tc.md+"\n", FromMarkdownOptions{}))
			assertDocEqual(t, fromLiteral, want)
		})
	}
}

// refusalCase is one source construct the v7 model cannot carry exactly.
type refusalCase struct {
	name      string
	src       string
	construct string
	at        string // the substring whose first byte offset UnrepresentableError.Offset must equal
}

func refusalCases() []refusalCase {
	return []refusalCase{
		{"raw_html_block", "para\n\n<div class=\"x\">hi</div>\n", "raw-html", `<div`},
		{"raw_html_comment", "intro\n\n<!-- note -->\n", "raw-html", `<!--`},
		{"raw_html_inline", "a <b>bold</b> c\n", "raw-html", `<b>`},
		{"table_cell_emphasis", "| a |\n| --- |\n| **b** |\n", "table-cell-formatting", `**b**`},
		{"table_cell_code", "| a |\n| --- |\n| `c` |\n", "table-cell-formatting", "`c`"},
		{"table_cell_link", "| a |\n| --- |\n| [l](u) |\n", "table-cell-formatting", `[l](u)`},
		{"ordered_list_start", "intro\n\n3. item\n", "list-start", `3.`},
		{"nested_ordered_list_start", "1. a\n\n   3. b\n", "list-start", `3.`},
		{"nested_bullet_under_ordered", "1. a\n   - b\n", "list-mixed-type", `- b`},
		{"nested_ordered_under_bullet", "- a\n  1. b\n", "list-mixed-type", `1. b`},
		{"task_item", "- [ ] t\n", "todo", `[ ]`},
		{"checked_task_item", "- a\n- [x] done\n", "todo", `[x]`},
		{"link_title", "see [a](https://x.io \"t\") now\n", "link-title", `[a]`},
		{"empty_link", "go [](https://x.io) now\n", "empty-link", `[](`},
		{"inline_image", "see ![i](u) here\n", "inline-image", `![i]`},
		{"two_images_in_a_paragraph", "![a](u)\n![b](v)\n", "inline-image", `![a]`},
		{"code_info_string", "```go title=x\nc\n```\n", "code-info-string", "```go"},
		{"list_item_second_paragraph", "- a\n\n  second para\n", "list-item-blocks", `second`},
		{"list_item_code_block", "- a\n\n  ```\n  c\n  ```\n", "list-item-blocks", "```"},
		{"inline_display_math", "a $$x$$ b\n", "inline-display-math", `$$x$$`},
		{"unclosed_display_math", "para\n\n$$\nx\n", "math-unclosed", `$$`},
		{"front_matter_not_a_pair", "---\ntitle: T\nnot a pair\n---\n\nbody\n", "front-matter", `not a pair`},
		{"front_matter_bad_quote", "---\ntitle: \"unterminated\n---\n\nbody\n", "front-matter", `title:`},
		{"front_matter_duplicate_key", "---\na: 1\na: 2\n---\n\nbody\n", "front-matter", `a: 2`},
	}
}

// TestFromMarkdownStrictRefusals: a construct the model cannot carry exactly is
// reported as *UnrepresentableError with the Construct name and the byte offset
// of the construct, never dropped or altered (Test list 5).
func TestFromMarkdownStrictRefusals(t *testing.T) {
	for _, tc := range refusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := FromMarkdown(tc.src, FromMarkdownOptions{})
			if err == nil {
				t.Fatalf("expected a refusal, got document:\n%s", docJSON(t, doc))
			}
			if doc != nil {
				t.Fatalf("a refusal must not also return a document")
			}
			var ue *UnrepresentableError
			if !errors.As(err, &ue) {
				t.Fatalf("error is %T (%v), want *UnrepresentableError", err, err)
			}
			if ue.Construct != tc.construct {
				t.Fatalf("Construct = %q, want %q (%v)", ue.Construct, tc.construct, err)
			}
			want := strings.Index(tc.src, tc.at)
			if want < 0 {
				t.Fatalf("bad test: %q not in source", tc.at)
			}
			if ue.Offset != want {
				t.Fatalf("Offset = %d, want %d (%q)", ue.Offset, want, tc.src[want:])
			}
			if !strings.Contains(err.Error(), tc.construct) {
				t.Fatalf("Error() = %q should name the construct", err.Error())
			}
		})
	}
}

// TestFromMarkdownLenientNeverRefuses: lenient mode maps every refusal case
// instead of failing.
func TestFromMarkdownLenientNeverRefuses(t *testing.T) {
	for _, tc := range refusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FromMarkdown(tc.src, FromMarkdownOptions{Lenient: true}); err != nil {
				t.Fatalf("lenient mode must not refuse: %v", err)
			}
		})
	}
}

// TestFromMarkdownLenientMappings pins the documented, deterministic lenient
// mapping for the five constructs of Test list 6.
func TestFromMarkdownLenientMappings(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want *Document
	}{
		{
			name: "raw_html_block_becomes_literal_paragraph_text",
			src:  "para\n\n<div class=\"x\">hi</div>\n",
			want: fxDoc([]Block{fxPara("para"), fxPara(`<div class="x">hi</div>`)}),
		},
		{
			name: "raw_html_inline_becomes_literal_text",
			src:  "a <b>bold</b> c\n",
			want: fxDoc([]Block{fxPara("a <b>bold</b> c")}),
		},
		{
			name: "table_cell_marks_are_flattened",
			src:  "| a |\n| --- |\n| **b** |\n",
			want: fxDoc([]Block{{Kind: BlockTable, Headers: []string{"a"}, Aligns: []string{""}, Rows: [][]string{{"b"}}}}),
		},
		{
			name: "ordered_list_start_resets_to_one",
			src:  "intro\n\n3. item\n",
			want: fxDoc([]Block{fxPara("intro"), {Kind: BlockList, Ordered: true, Items: fxItems("item")}}),
		},
		{
			name: "outer_list_type_wins",
			src:  "1. a\n   - b\n",
			want: fxDoc([]Block{{Kind: BlockList, Ordered: true, Items: []ListItem{{Text: "a"}, {Text: "b", Level: 1}}}}),
		},
		{
			name: "task_item_keeps_its_checkbox_as_text",
			src:  "- [ ] t\n- [x] u\n",
			want: fxDoc([]Block{{Kind: BlockList, Items: fxItems("[ ] t", "[x] u")}}),
		},
		{
			name: "link_title_is_dropped",
			src:  "see [a](https://x.io \"t\") now\n",
			want: fxDoc([]Block{fxRich(fxRun("see "), fxLink("a", "https://x.io"), fxRun(" now"))}),
		},
		{
			name: "inline_image_becomes_its_alt_text",
			src:  "see ![i](u) here\n",
			want: fxDoc([]Block{fxPara("see i here")}),
		},
		{
			name: "code_info_string_keeps_the_language_word",
			src:  "```go title=x\nc\n```\n",
			want: fxDoc([]Block{{Kind: BlockCode, Language: "go", Text: "c"}}),
		},
		{
			name: "list_item_extra_blocks_join_the_item_text",
			src:  "- a\n\n  second para\n",
			want: fxDoc([]Block{{Kind: BlockList, Items: fxItems("a\nsecond para")}}),
		},
		{
			name: "inline_display_math_becomes_inline_math",
			src:  "a $$x$$ b\n",
			want: fxDoc([]Block{fxRich(fxRun("a "), fxRun("x", "math"), fxRun(" b"))}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{Lenient: true}))
			assertDocEqual(t, got, stripIDsAndSpans(tc.want))
			again := stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{Lenient: true}))
			assertDocEqual(t, again, got)
		})
	}
}

// TestFromMarkdownMathIsNotDuplicated is the Markdown-to-model half of the
// RESEARCH 1.4 regression: inline `$a^2$` is one run of the paragraph and the
// display block is one math block; neither is emitted twice (Test list 7).
func TestFromMarkdownMathIsNotDuplicated(t *testing.T) {
	d := mustFromMarkdown(t, "Inline $a^2$ then\n\n$$\nE=mc^2\n$$\n", FromMarkdownOptions{})
	blocks := d.Sections[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks (paragraph, display math), got %d:\n%s", len(blocks), docJSON(t, d))
	}
	if blocks[0].Kind != BlockParagraph || blocks[0].Text != "Inline a^2 then" {
		t.Fatalf("paragraph = %+v", blocks[0])
	}
	if len(blocks[0].Runs) != 3 || !hasMark(blocks[0].Runs[1], "math") || blocks[0].Runs[1].Text != "a^2" {
		t.Fatalf("inline math must be a math-marked run: %+v", blocks[0].Runs)
	}
	if blocks[1].Kind != BlockMath || !blocks[1].Display || blocks[1].Text != "E=mc^2" {
		t.Fatalf("display math block = %+v", blocks[1])
	}
}

// TestFromMarkdownDisplayMathForms: the spellings of display math all read as
// the same block, and a blank line inside $$ is part of the TeX.
func TestFromMarkdownDisplayMathForms(t *testing.T) {
	cases := []struct{ name, src, tex string }{
		{"fenced", "$$\nx+y\n$$\n", "x+y"},
		{"one_line", "$$x+y$$\n", "x+y"},
		{"opener_with_text", "$$x+y\n$$\n", "x+y"},
		{"closer_with_text", "$$\nx+y$$\n", "x+y"},
		{"multi_line", "$$\na\nb\n$$\n", "a\nb"},
		{"blank_line_inside", "$$\na\n\nb\n$$\n", "a\n\nb"},
		{"markdown_inside_is_tex", "$$\n# not a heading\n- not a list\n$$\n", "# not a heading\n- not a list"},
		{"empty", "$$\n$$\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{}))
			want := stripIDsAndSpans(fxDoc([]Block{{Kind: BlockMath, Display: true, Text: tc.tex}}))
			assertDocEqual(t, got, want)
		})
	}
}

// TestFromMarkdownDollarsAreMathOnlyWhenTheyLookLikeMath: the pandoc rule keeps
// prices and the like as text.
func TestFromMarkdownDollarsAreMathOnlyWhenTheyLookLikeMath(t *testing.T) {
	text := []string{"costs $5 and $6 total", "a $ b $ c", "$ x$", "only one $ here", "5$ and 6$"}
	for _, s := range text {
		d := mustFromMarkdown(t, s+"\n", FromMarkdownOptions{})
		if len(d.Sections[0].Blocks) != 1 || d.Sections[0].Blocks[0].Kind != BlockParagraph || d.Sections[0].Blocks[0].Text != s || d.Sections[0].Blocks[0].Runs != nil {
			t.Errorf("%q should be a plain paragraph, got %s", s, docJSON(t, d))
		}
	}
	d := mustFromMarkdown(t, "so $x$ and $y_1$ and \\$z\\$ done\n", FromMarkdownOptions{})
	got := d.Sections[0].Blocks[0]
	want := fxRich(fxRun("so "), fxRun("x", "math"), fxRun(" and "), fxRun("y_1", "math"), fxRun(" and $z$ done"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// TestFromMarkdownEmptyDocuments: an empty source is the document NewDocument
// produces (one empty section), the same shape ToMarkdown writes as "\n".
func TestFromMarkdownEmptyDocuments(t *testing.T) {
	for _, src := range []string{"", "\n", "   \n\n  \n"} {
		got := mustFromMarkdown(t, src, FromMarkdownOptions{})
		assertDocEqual(t, got, NewDocument(""))
	}
	assertDocEqual(t, mustFromMarkdown(t, ToMarkdown(NewDocument("")), FromMarkdownOptions{}), NewDocument(""))
	assertDocEqual(t, mustFromMarkdown(t, ToMarkdown(NewDocument("T")), FromMarkdownOptions{}), NewDocument("T"))
}

// TestFromMarkdownFrontMatter covers the directive spellings ToMarkdown writes
// and the ones people type.
func TestFromMarkdownFrontMatter(t *testing.T) {
	src := "---\ntheme: gaia\ntitle: \"A: B\"\nauthor: 'O''Neil'\nmarp: true\nempty:\n---\n\nbody\n"
	got := mustFromMarkdown(t, src, FromMarkdownOptions{})
	want := fxDoc([]Block{fxPara("body")})
	want.Meta.Directives = map[string]string{
		"theme": "gaia", "title": "A: B", "author": "O'Neil", "marp": "true", "empty": "",
	}
	assertDocEqual(t, got, want)

	// A leading rule that is not front matter is a section break.
	got = mustFromMarkdown(t, "---\n\nbody\n", FromMarkdownOptions{})
	assertDocEqual(t, got, fxDoc(nil, []Block{fxPara("body")}))
	got = mustFromMarkdown(t, "---\n\nA\n\n---\n\nB\n", FromMarkdownOptions{})
	assertDocEqual(t, got, fxDoc(nil, []Block{fxPara("A")}, []Block{fxPara("B")}))
}

// TestFromMarkdownReadsCommonMarkSpellings: other spellings of the same content
// read as the same Document, so the canonical Markdown they re-serialize to is
// fixed. These are normalizations, not losses: bullet character, fence
// character, emphasis delimiter, list tightness, numbering, setext vs ATX,
// reference vs inline links, and entity spelling carry no content.
func TestFromMarkdownReadsCommonMarkSpellings(t *testing.T) {
	cases := []struct{ name, src, canonical string }{
		{"star_bullets", "* a\n* b\n", "- a\n- b\n"},
		{"plus_bullets", "+ a\n+ b\n", "- a\n- b\n"},
		{"loose_list", "- a\n\n- b\n", "- a\n- b\n"},
		{"ordered_all_ones", "1. a\n1. b\n1. c\n", "1. a\n2. b\n3. c\n"},
		{"ordered_paren", "1) a\n2) b\n", "1. a\n2. b\n"},
		{"underscore_emphasis", "_em_ __strong__ ___both___\n", "*em* **strong** ***both***\n"},
		{"setext_h1", "Title\n=====\n", "# Title\n"},
		{"setext_h2", "Sub\n---\n", "## Sub\n"},
		{"atx_closing_sequence", "## Title ##\n", "## Title\n"},
		{"heading_with_ordered_looking_text", "# 1. Intro\n", "# 1. Intro\n"},
		{"indented_code", "    indented code\n    more\n", "```\nindented code\nmore\n```\n"},
		{"tilde_fence", "~~~py\nx = 1\n~~~\n", "```py\nx = 1\n```\n"},
		{"long_fence", "`````\n```\ninner\n```\n`````\n", "````\n```\ninner\n```\n````\n"},
		{"fence_without_closing", "```\nunclosed\n", "```\nunclosed\n```\n"},
		{"underscore_rule", "a\n\n___\n\nb\n", "a\n\n***\n\nb\n"},
		{"spaced_star_rule", "a\n\n* * *\n\nb\n", "a\n\n***\n\nb\n"},
		{"spaced_dash_rule_is_a_section_break", "a\n\n- - -\n\nb\n", "a\n\n---\n\nb\n"},
		{"long_dash_rule", "a\n\n-----\n\nb\n", "a\n\n---\n\nb\n"},
		{"soft_line_break_is_a_space", "one\ntwo\n", "one two\n"},
		{"soft_wrap_in_a_list_item_is_a_space", "- one\n  two\n", "- one two\n"},
		{"two_space_hard_break", "one  \ntwo\n", "one\\\ntwo\n"},
		{"backslash_hard_break", "one\\\ntwo\n", "one\\\ntwo\n"},
		{"entities", "&copy; &amp; &#65; &#x42; &nbsp;|\n", "© & A B \u00a0\\|\n"},
		{"backslash_escapes", "\\*not em\\* \\# \\[x\\]\n", "\\*not em\\* # \\[x\\]\n"},
		{"autolink", "<https://x.io/a>\n", "[https://x.io/a](https://x.io/a)\n"},
		{"email_autolink", "<me@x.io>\n", "[me@x.io](mailto:me@x.io)\n"},
		{"reference_link", "[a][r] and [b]\n\n[r]: https://x.io\n[b]: https://y.io\n", "[a](https://x.io) and [b](https://y.io)\n"},
		{"image_single_quoted_title", "![a](b.png 'T')\n", "![a](b.png \"T\")\n"},
		{"image_alt_with_marks", "![a **b**](b.png)\n", "![a b](b.png)\n"},
		{"link_with_parens_in_destination", "[a](https://x.io/f_(1))\n", "[a](https://x.io/f_\\(1\\))\n"},
		{"angle_destination", "[a](<my file.md>)\n", "[a](<my file.md>)\n"},
		{"nested_quote", "> a\n>\n> > b\n", "> a\n>\n> > b\n"},
		{"lazy_quote_continuation", "> a\nb\n", "> a b\n"},
		{"quote_with_heading", "> # T\n> text\n", "> # T\n>\n> text\n"},
		{"table_without_outer_pipes", "a | b\n--|--\n1 | 2\n", "| a | b |\n| --- | --- |\n| 1 | 2 |\n"},
		{"table_alignment_spellings", "| a | b | c | d |\n|:-|:-:|-:|-|\n| 1 | 2 | 3 | 4 |\n", "| a | b | c | d |\n| :--- | :---: | ---: | --- |\n| 1 | 2 | 3 | 4 |\n"},
		{"strike_single_tilde", "~a~ ~~b~~\n", "~~a~~ ~~b~~\n"},
		{"nested_marks_collapse", "**a *b* c**\n", "**a** ***b*** **c**\n"},
		{"code_span_edge_spaces", "`` `a` ``\n", "`` `a` ``\n"},
		{"crlf_line_endings", "# T\r\n\r\nbody\r\nmore\r\n", "# T\n\nbody more\n"},
		{"trailing_blank_lines", "a\n\n\n\n", "a\n"},
		{"multi_line_setext_heading_is_one_line", "Line one\nline two\n=====\n", "# Line one line two\n"},
		{"multi_line_image_alt_is_one_line", "![one\ntwo](a.png)\n", "![one two](a.png)\n"},
		{"padded_inline_display_math_is_text", "a $$ x $$ b\n", "a \\$\\$ x \\$\\$ b\n"},
		{"display_math_followed_by_a_digit_is_not_display_math", "a $$x$$1 b\n", "a \\$$x$\\$1 b\n"},
		{"adjacent_inline_math_is_one_math_block", "$a$$b$\n", "$ab$\n"},
		{"inline_math_with_escaped_backslashes", "x $a\\\\$ y\n", "x $a\\\\$ y\n"},
		{"fence_language_with_a_backslash", "```a\\\\b\nx\n```\n", "```a\\\\b\nx\n```\n"},
		{"three_adjacent_lists_in_source", "- a\n\n* b\n\n- c\n", "- a\n\n* b\n\n- c\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := mustFromMarkdown(t, tc.src, FromMarkdownOptions{})
			if got := ToMarkdown(d); got != tc.canonical {
				t.Fatalf("ToMarkdown(FromMarkdown(%q))\n got: %q\nwant: %q", tc.src, got, tc.canonical)
			}
			// and the canonical spelling is itself stable
			again := mustFromMarkdown(t, tc.canonical, FromMarkdownOptions{})
			assertDocEqual(t, again, d)
		})
	}
}

// mergeAdjacentRuns joins neighbouring runs that have the same marks and link:
// canonical form never has two of them side by side.
func mergeAdjacentRuns(runs []TextRun) []TextRun {
	var out []TextRun
	for _, r := range runs {
		if n := len(out); n > 0 && out[n-1].Link == r.Link && equalMarks(out[n-1].Marks, r.Marks) {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

// TestCodecMarkAdjacency: runs with different marks that touch each other, with
// and without links, keep their marks and boundaries. Adjacent delimiters are
// where Markdown emphasis is most ambiguous.
func TestCodecMarkAdjacency(t *testing.T) {
	sets := [][]string{nil, {"bold"}, {"italic"}, {"bold", "italic"}, {"strike"}, {"code"}, {"bold", "strike"}, {"italic", "code"}}
	for _, a := range sets {
		for _, b := range sets {
			for _, link := range []string{"", "https://x.io"} {
				if equalMarks(a, b) {
					continue
				}
				name := strings.Join(a, "+") + "|" + strings.Join(b, "+") + "|" + link
				t.Run(name, func(t *testing.T) {
					want := fxDoc([]Block{fxRich(mergeAdjacentRuns([]TextRun{
						fxRun("p "),
						{Text: "aa", Marks: a, Link: link},
						{Text: "bb", Marks: b, Link: link},
						fxRun(" q"),
					})...)})
					md := ToMarkdown(want)
					got := mustFromMarkdown(t, md, FromMarkdownOptions{})
					assertDocEqual(t, got, want)
				})
			}
		}
	}
}

// fragmentPool is the vocabulary of TestFromMarkdownIsStableOnFragments.
var fragmentPool = []string{
	"# Heading one", "## Heading *two*", "plain paragraph text", "text with *em* and **strong** and `code`",
	"- a\n- b\n  - c\n    - d", "1. x\n2. y\n   1. z", "> quoted\n>\n> second", "> - in quote\n> - list",
	"```go\nfmt.Println(1)\n```", "````\n```\ninner\n```\n````", "| h1 | h2 |\n| :-- | --: |\n| 1 | 2 |\n| 3 | 4 |",
	"$$\nx^2 + y^2\n$$", "inline $a_1$ math", "---", "***", "![alt](pic.png \"title\")", "line one\nline two",
	"[link](https://x.io) and [**bold link**](https://y.io)", "~~gone~~ text", "snake_case and 2*3*4", "\\* escaped \\[x\\]",
	"- a\n\n* b", "Setext\n======", "    indented", "<b>raw</b> html", "<div>block</div>", "- [ ] task", "3. late start",
	"| a |\n| - |\n| **b** |", "&copy; entity", "a | b", "$5 and $6",
}

// TestFromMarkdownIsStableOnFragments assembles seeded random documents from
// the pool and checks the codec's stability invariant: reading the Markdown
// ToMarkdown writes for a parsed document gives the same document back, and
// reading never panics. Strict failures must be *UnrepresentableError.
// (TRD 03 adds the all-kinds property test over generated Documents.)
func TestFromMarkdownIsStableOnFragments(t *testing.T) {
	rng := rand.New(rand.NewSource(20261103))
	for i := 0; i < 400; i++ {
		n := 1 + rng.Intn(6)
		parts := make([]string, n)
		for j := range parts {
			parts[j] = fragmentPool[rng.Intn(len(fragmentPool))]
		}
		src := strings.Join(parts, "\n\n") + "\n"
		for _, lenient := range []bool{false, true} {
			d, err := FromMarkdown(src, FromMarkdownOptions{Lenient: lenient})
			if err != nil {
				var ue *UnrepresentableError
				if lenient || !errors.As(err, &ue) {
					t.Fatalf("lenient=%v: unexpected error %T %v for %q", lenient, err, err, src)
				}
				continue
			}
			md := ToMarkdown(d)
			d2, err := FromMarkdown(md, FromMarkdownOptions{})
			if err != nil {
				t.Fatalf("lenient=%v: canonical Markdown is refused: %v\nsource: %q\ncanonical: %q", lenient, err, src, md)
			}
			if !reflect.DeepEqual(d, d2) {
				t.Fatalf("lenient=%v: not stable\nsource: %q\ncanonical: %q\n--- first ---\n%s\n--- second ---\n%s",
					lenient, src, md, docJSON(t, d), docJSON(t, d2))
			}
			if md2 := ToMarkdown(d2); md2 != md {
				t.Fatalf("lenient=%v: Markdown not a fixed point\n%q\n%q", lenient, md, md2)
			}
		}
	}
}

// ---- seeded sweeps (stdlib only; TRD 03 adds the all-kinds property test) ----

func sweepText(rng *rand.Rand, alphabet []string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(alphabet[rng.Intn(len(alphabet))])
	}
	return b.String()
}

// sweepRoundTrip checks FromMarkdown(ToMarkdown(d)) == d for a one-block
// document and reports at most a few failures.
func sweepRoundTrip(t *testing.T, name string, blk Block, fails *int) {
	t.Helper()
	d := fxDoc([]Block{blk})
	rebuildOutline(d)
	md := ToMarkdown(d)
	got, err := FromMarkdown(md, FromMarkdownOptions{})
	if err == nil && reflect.DeepEqual(got, d) {
		return
	}
	*fails++
	if *fails > 5 {
		return
	}
	gotJSON := "nil"
	if got != nil {
		gotJSON = docJSON(t, got)
	}
	t.Errorf("%s does not round-trip\n markdown: %q\n error: %v\n want: %s\n got: %s", name, md, err, docJSON(t, d), gotJSON)
}

// TestCodecEscapingSweep: random text built from Markdown metacharacters comes
// back unchanged from a paragraph, a list item, a heading and a quote.
func TestCodecEscapingSweep(t *testing.T) {
	alphabet := []string{
		"a", "B", "1", "2", ".", ")", "(", " ", "*", "_", "`", "~", "#", ">", "-", "+", "=", "|", "$", "<", ">",
		"[", "]", "!", "&", "amp;", "#65;", "\\", "\n", "é", "x", ":", "/",
	}
	rng := rand.New(rand.NewSource(7))
	fails := 0
	for i := 0; i < 6000; i++ {
		s := sweepText(rng, alphabet, 1+rng.Intn(8))
		if strings.HasPrefix(s, "\n") || strings.HasSuffix(s, "\n") || strings.Contains(s, "\n\n") {
			continue // an empty line is a paragraph break, not text
		}
		sweepRoundTrip(t, "paragraph "+strconv.Quote(s), Block{Kind: BlockParagraph, Text: s}, &fails)
		sweepRoundTrip(t, "list item "+strconv.Quote(s), Block{Kind: BlockList, Items: []ListItem{{Text: s}}}, &fails)
		sweepRoundTrip(t, "quote "+strconv.Quote(s), Block{Kind: BlockQuote, Text: s}, &fails)
		if !strings.Contains(s, "\n") {
			sweepRoundTrip(t, "heading "+strconv.Quote(s), Block{Kind: BlockHeading, Level: 2, Text: s}, &fails)
		}
	}
	if fails > 0 {
		t.Fatalf("%d round-trip failures", fails)
	}
}

// TestCodecBlockSweep: random tables, images, code, display math and lists
// come back unchanged.
func TestCodecBlockSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	fails := 0
	textAlpha := []string{"a", "b", "1", " ", "*", "_", "`", "~", "|", "$", "[", "]", "(", ")", "<", ">", "&", "\\", ".", "-", "#", "!", ":", "é", "amp;"}
	for i := 0; i < 2500; i++ {
		cell := func() string { return strings.TrimSpace(sweepText(rng, textAlpha, rng.Intn(6))) }
		n := 1 + rng.Intn(3)
		headers, aligns := make([]string, n), make([]string, n)
		for k := range headers {
			headers[k] = cell()
			aligns[k] = []string{"", "left", "right", "center"}[rng.Intn(4)]
		}
		var rows [][]string
		for r := rng.Intn(3); r > 0; r-- {
			row := make([]string, n)
			for k := range row {
				row[k] = cell()
			}
			rows = append(rows, row)
		}
		sweepRoundTrip(t, "table", Block{Kind: BlockTable, Headers: headers, Aligns: aligns, Rows: rows}, &fails)

		src := sweepText(rng, []string{"a", "b", "/", ".", " ", "(", ")", "<", ">", "&", "amp;", "\\", "%", "#"}, 1+rng.Intn(6))
		src = "x" + src + "y"
		title := sweepText(rng, []string{"a", " ", "\"", "\\", "&", "amp;", "'"}, rng.Intn(4))
		alt := strings.TrimSpace(sweepText(rng, textAlpha, rng.Intn(5)))
		sweepRoundTrip(t, "image", Block{Kind: BlockImage, Src: src, Text: alt, Title: title}, &fails)

		code := sweepText(rng, []string{"a", "`", "``", "\n", " ", "~", "$", "#", "-"}, rng.Intn(8))
		sweepRoundTrip(t, "code", Block{Kind: BlockCode, Language: []string{"", "go", "c++", "x-y"}[rng.Intn(4)], Text: code}, &fails)

		tex := sweepText(rng, []string{"a", "x", "^", "_", "{", "}", "\\", " ", "\n", "#", "-", "|", "<", ">", "&", "1"}, rng.Intn(8))
		sweepRoundTrip(t, "display math", Block{Kind: BlockMath, Display: true, Text: tex}, &fails)

		var items []ListItem
		level := 0
		for k := 1 + rng.Intn(5); k > 0; k-- {
			if len(items) == 0 {
				level = 0
			} else {
				level = rng.Intn(level + 2)
			}
			text := strings.TrimSpace(sweepText(rng, []string{"a", "b", "1", ".", "-", "*", " ", "#", ">"}, rng.Intn(4)))
			items = append(items, ListItem{Text: text, Level: level})
		}
		sweepRoundTrip(t, "list", Block{Kind: BlockList, Ordered: rng.Intn(2) == 0, Items: items}, &fails)
	}
	if fails > 0 {
		t.Fatalf("%d round-trip failures", fails)
	}
}

// TestCodecRunsSweep: random canonical rich paragraphs (marks, links, math,
// punctuation next to delimiters) come back unchanged. Bold, italic and strike
// next to punctuation or letters are the case where `**` and `~~` would not
// read back, so the serializer switches such a block to <strong>/<em>/<del>.
func TestCodecRunsSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	fails := 0
	markSets := [][]string{nil, nil, {"bold"}, {"italic"}, {"bold", "italic"}, {"strike"}, {"code"}, {"bold", "strike"}, {"italic", "strike"}, {"bold", "italic", "strike"}, {"math"}}
	words := []string{
		"a", "bb", "x y", "(p)", "q.", "*", "_", "1", "é", ",", "x_1", "$", "a|b", "-", "[", "!", "**", "~~", "`",
		"a\\", "\\", ")", "](", "<em>", "&amp;", "# h", "1. ", "> q", "x*y", "2*3*4", "a_b_c", "((", "))",
	}
	links := []string{"", "", "", "https://x.io", "dir/a b.md"}
	for i := 0; i < 6000; i++ {
		var runs []TextRun
		for k := 1 + rng.Intn(5); k > 0; k-- {
			marks := markSets[rng.Intn(len(markSets))]
			text := words[rng.Intn(len(words))]
			if len(marks) == 1 && marks[0] == "math" {
				text = []string{"a^2", "x_1", "y"}[rng.Intn(3)]
			}
			runs = append(runs, TextRun{Text: text, Marks: marks, Link: links[rng.Intn(len(links))]})
		}
		runs = canonRuns(runs)
		if len(runs) == 0 || (len(runs) == 1 && hasMark(runs[0], "math")) {
			continue // a lone math run is a math block, not a paragraph
		}
		skip := false
		for k, r := range runs {
			// A closing $ followed by a digit does not close math.
			if hasMark(r, "math") && k+1 < len(runs) && runs[k+1].Text[0] >= '0' && runs[k+1].Text[0] <= '9' {
				skip = true
			}
		}
		if skip {
			continue
		}
		blk := Block{Kind: BlockParagraph, Text: FlattenRuns(runs), Runs: runs}
		if len(runs) == 1 && len(runs[0].Marks) == 0 && runs[0].Link == "" {
			blk.Runs = nil
		}
		sweepRoundTrip(t, "rich paragraph", blk, &fails)
	}
	if fails > 0 {
		t.Fatalf("%d round-trip failures", fails)
	}
}

// TestFromMarkdownIsStableOnGarbage reads seeded random soup of Markdown
// punctuation. Whatever it parses to, writing that and reading it again must
// give the same Document (strict refusals and the lenient mapping alike), and
// nothing may panic.
func TestFromMarkdownIsStableOnGarbage(t *testing.T) {
	alphabet := []string{
		"a", "b", " ", "  ", "\n", "\n\n", "\n    ", "#", "# ", "> ", "- ", "* ", "+ ", "1. ", "2) ", "`", "```", "~~~", "~~",
		"*", "**", "_", "__", "[", "]", "(", ")", "![", "](", "|", "|-", "-|", "---", "***", "$", "$$", "\\", "&", "<", ">",
		"<b>", "</b>", "<!--", "-->", "=", "===", ":", "-:", "[ ] ", "[x] ", "http://x.io", "é", "\t",
	}
	rng := rand.New(rand.NewSource(3))
	bad := 0
	for i := 0; i < 12000 && bad < 5; i++ {
		src := sweepText(rng, alphabet, 1+rng.Intn(20))
		for _, lenient := range []bool{false, true} {
			d, err := FromMarkdown(src, FromMarkdownOptions{Lenient: lenient})
			if err != nil {
				var ue *UnrepresentableError
				if lenient || !errors.As(err, &ue) {
					t.Fatalf("lenient=%v: unexpected error %T %v for %q", lenient, err, err, src)
				}
				continue
			}
			md := ToMarkdown(d)
			d2, err := FromMarkdown(md, FromMarkdownOptions{})
			if err == nil && reflect.DeepEqual(d, d2) {
				continue
			}
			bad++
			second := "nil"
			if d2 != nil {
				second = docJSON(t, d2)
			}
			t.Errorf("not stable (lenient=%v)\n source: %q\n canonical: %q\n error: %v\n first: %s\n second: %s", lenient, src, md, err, docJSON(t, d), second)
			break
		}
	}
}

// TestFromMarkdownIsSafeForConcurrentUse: the package-level parser is shared by
// every caller, so concurrent reads must not interfere (run with -race).
func TestFromMarkdownIsSafeForConcurrentUse(t *testing.T) {
	src := "# T\n\ntext with *em*, `code` and $x$\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n$$\ny\n$$\n"
	want := mustFromMarkdown(t, src, FromMarkdownOptions{})
	done := make(chan *Document, 8)
	for i := 0; i < 8; i++ {
		go func() {
			var d *Document
			for j := 0; j < 50; j++ {
				d, _ = FromMarkdown(src, FromMarkdownOptions{})
			}
			done <- d
		}()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; !reflect.DeepEqual(got, want) {
			t.Fatalf("concurrent read differs:\n%s", docJSON(t, got))
		}
	}
}
