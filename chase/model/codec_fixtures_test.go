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

// Hand-built fixtures for the Markdown codec (objective 1026, TRD 01).
//
// Every expected Document below is a literal written by hand, and every
// expected Markdown string is a literal written by hand. Nothing here is built
// by Build, press.Render or ImportMarkdownModelJSON: those are the lossy paths
// the codec replaces, and an expectation produced by them would only prove the
// codec agrees with its own bugs.
//
// A fixture is in CANONICAL form, the one FromMarkdown produces for the
// Markdown ToMarkdown emits for it:
//   - a rich paragraph/heading/list item carries Runs (Text == FlattenRuns);
//     a plain one carries Text only and leaves Runs nil;
//   - adjacent runs with the same marks and link are merged, and a marked run
//     has no leading or trailing whitespace;
//   - marks appear in the order bold, italic, strike, code, math;
//   - a list's first item is level 0 and no item is more than one level
//     deeper than the one before it;
//   - Section IDs are 1..n and Outline is what rebuildOutline derives from the
//     blocks (the function ApplyOps uses), written out here as literals.

// codecFixture pairs a hand-built Document with the exact Markdown ToMarkdown
// must emit for it.
type codecFixture struct {
	name string
	doc  func() *Document
	md   string
}

// fxDoc builds a Document with one Section per argument. Outline is the
// non-nil empty slice NewDocument produces; fixtures with headings replace it.
func fxDoc(sections ...[]Block) *Document {
	d := &Document{SchemaVersion: SchemaVersion, Outline: []OutlineEntry{}}
	if len(sections) == 0 {
		sections = [][]Block{nil}
	}
	for i, blocks := range sections {
		d.Sections = append(d.Sections, Section{ID: i + 1, Blocks: blocks})
	}
	return d
}

func fxPara(text string) Block {
	return Block{Kind: BlockParagraph, Text: text}
}

// fxRich builds a rich paragraph whose Text is the flattened runs.
func fxRich(runs ...TextRun) Block {
	return Block{Kind: BlockParagraph, Text: FlattenRuns(runs), Runs: runs}
}

func fxRun(text string, marks ...string) TextRun {
	return TextRun{Text: text, Marks: marks}
}

func fxLink(text, href string, marks ...string) TextRun {
	return TextRun{Text: text, Marks: marks, Link: href}
}

func fxItems(texts ...string) []ListItem {
	items := make([]ListItem, len(texts))
	for i, t := range texts {
		items[i] = ListItem{Text: t}
	}
	return items
}

// codecFixtures is the shared matrix: TRD 01 Task 1 asserts ToMarkdown's exact
// bytes for each entry; Task 2 asserts FromMarkdown(ToMarkdown(d)) == d and
// that the Markdown is a fixed point.
func codecFixtures() []codecFixture {
	return []codecFixture{
		{
			name: "paragraph_all_marks_and_links",
			doc: func() *Document {
				return fxDoc([]Block{fxRich(
					fxRun("plain "),
					fxRun("bold", "bold"),
					fxRun(" "),
					fxRun("italic", "italic"),
					fxRun(" "),
					fxRun("both", "bold", "italic"),
					fxRun(" "),
					fxRun("code", "code"),
					fxRun(" "),
					fxRun("gone", "strike"),
					fxRun(" "),
					fxLink("link", "https://example.com/a?b=1"),
					fxRun(" "),
					fxLink("bold link", "https://example.com", "bold"),
					fxRun(" end"),
				)})
			},
			md: "plain **bold** *italic* ***both*** `code` ~~gone~~ [link](https://example.com/a?b=1) [**bold link**](https://example.com) end\n",
		},
		{
			name: "headings_levels_1_to_6",
			doc: func() *Document {
				d := fxDoc([]Block{
					{Kind: BlockHeading, Level: 1, Text: "Title"},
					{Kind: BlockHeading, Level: 2, Text: "Two words", Runs: []TextRun{fxRun("Two", "bold"), fxRun(" words")}},
					{Kind: BlockHeading, Level: 3, Text: "Three"},
					{Kind: BlockHeading, Level: 4, Text: "Four x", Runs: []TextRun{fxRun("Four "), fxRun("x", "italic")}},
					{Kind: BlockHeading, Level: 5, Text: "Five"},
					{Kind: BlockHeading, Level: 6, Text: "Six"},
				})
				d.Outline = []OutlineEntry{
					{SectionID: 1, Level: 1, Text: "Title", Slug: "title"},
					{SectionID: 1, Level: 2, Text: "Two words", Slug: "two-words"},
					{SectionID: 1, Level: 3, Text: "Three", Slug: "three"},
					{SectionID: 1, Level: 4, Text: "Four x", Slug: "four-x"},
					{SectionID: 1, Level: 5, Text: "Five", Slug: "five"},
					{SectionID: 1, Level: 6, Text: "Six", Slug: "six"},
				}
				return d
			},
			md: "# Title\n\n## **Two** words\n\n### Three\n\n#### Four *x*\n\n##### Five\n\n###### Six\n",
		},
		{
			// A trailing run of '#' after a space is an ATX closing sequence and
			// would be dropped, so a heading escapes it.
			name: "heading_trailing_hash_is_escaped",
			doc: func() *Document {
				d := fxDoc([]Block{
					{Kind: BlockHeading, Level: 2, Text: "Issue #"},
					{Kind: BlockHeading, Level: 3, Text: "C#"},
				})
				d.Outline = []OutlineEntry{
					{SectionID: 1, Level: 2, Text: "Issue #", Slug: "issue"},
					{SectionID: 1, Level: 3, Text: "C#", Slug: "c"},
				}
				return d
			},
			md: "## Issue \\#\n\n### C#\n",
		},
		{
			name: "bullet_list_levels_0_1_2_with_item_runs",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockList, Items: []ListItem{
					{Text: "one"},
					{Text: "two bold", Runs: []TextRun{fxRun("two "), fxRun("bold", "bold")}},
					{Text: "child a", Level: 1},
					{Text: "grandchild", Level: 2},
					{Text: "child b", Level: 1},
					{Text: "three"},
				}}})
			},
			md: "- one\n- two **bold**\n  - child a\n    - grandchild\n  - child b\n- three\n",
		},
		{
			// An empty item cannot interrupt the text line above it (it would
			// read as a continuation of it), so a blank line separates them.
			name: "list_empty_nested_item_after_text_is_preceded_by_a_blank_line",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockList, Items: []ListItem{{Text: "a"}, {Level: 1}, {Text: "b"}}}})
			},
			md: "- a\n\n  -\n- b\n",
		},
		{
			name: "list_empty_item_owning_a_nested_item",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockList, Items: []ListItem{{}, {Text: "x", Level: 1}}}})
			},
			md: "-\n  - x\n",
		},
		{
			// "![" would start an image.
			name: "link_after_an_exclamation_mark_is_not_an_image",
			doc: func() *Document {
				return fxDoc([]Block{fxRich(fxRun("wow!"), fxLink("x", "https://x.io"))})
			},
			md: "wow\\![x](https://x.io)\n",
		},
		{
			// CommonMark's flanking rules do not open `~~` after a letter when
			// a delimiter follows, so the block is written with tags that read
			// back as the same marks.
			name: "marks_delimiters_cannot_express_are_written_as_tags",
			doc: func() *Document {
				return fxDoc([]Block{fxRich(fxRun("aa"), fxRun("bb", "bold", "strike"), fxRun(" q"))})
			},
			md: "aa<del><strong>bb</strong></del> q\n",
		},
		{
			name: "adjacent_strike_runs_use_tags_instead_of_a_tilde_wall",
			doc: func() *Document {
				return fxDoc([]Block{fxRich(fxRun("aa", "strike"), fxRun("bb", "bold", "strike"))})
			},
			md: "<del>aa</del><del><strong>bb</strong></del>\n",
		},
		{
			name: "ordered_list_with_nested_ordered",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockList, Ordered: true, Items: []ListItem{
					{Text: "first"},
					{Text: "second"},
					{Text: "nested one", Level: 1},
					{Text: "nested two", Level: 1},
					{Text: "third"},
				}}})
			},
			md: "1. first\n2. second\n   1. nested one\n   2. nested two\n3. third\n",
		},
		{
			name: "ordered_list_two_digit_marker_indents_children_by_marker_width",
			doc: func() *Document {
				items := fxItems("n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9", "n10")
				items = append(items, ListItem{Text: "child", Level: 1})
				return fxDoc([]Block{{Kind: BlockList, Ordered: true, Items: items}})
			},
			md: "1. n1\n2. n2\n3. n3\n4. n4\n5. n5\n6. n6\n7. n7\n8. n8\n9. n9\n10. n10\n    1. child\n",
		},
		{
			name: "adjacent_lists_alternate_markers_so_they_stay_separate",
			doc: func() *Document {
				return fxDoc([]Block{
					{Kind: BlockList, Items: fxItems("a")},
					{Kind: BlockList, Items: fxItems("b")},
					{Kind: BlockList, Items: fxItems("c")},
				})
			},
			md: "- a\n\n* b\n\n- c\n",
		},
		{
			name: "code_block_with_language_and_inner_backtick_fence",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockCode, Language: "go", Text: "a := `x`\n```\nb"}})
			},
			md: "````go\na := `x`\n```\nb\n````\n",
		},
		{
			name: "code_block_trailing_newline_is_a_blank_line_before_the_fence",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockCode, Text: "x\n"}})
			},
			md: "```\nx\n\n```\n",
		},
		{
			name: "code_block_empty",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockCode}})
			},
			md: "```\n```\n",
		},
		{
			name: "math_inline_run_inside_paragraph",
			doc: func() *Document {
				return fxDoc([]Block{fxRich(fxRun("Energy "), fxRun("a^2", "math"), fxRun(" here"))})
			},
			md: "Energy $a^2$ here\n",
		},
		{
			name: "math_display_block",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockMath, Display: true, Text: "E=mc^2"}})
			},
			md: "$$\nE=mc^2\n$$\n",
		},
		{
			name: "math_standalone_inline_block",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockMath, Text: "x_1"}})
			},
			md: "$x_1$\n",
		},
		{
			name: "inline_math_then_display_math_is_not_duplicated",
			doc: func() *Document {
				return fxDoc([]Block{
					fxRich(fxRun("Inline "), fxRun("a^2", "math"), fxRun(" then")),
					{Kind: BlockMath, Display: true, Text: "E=mc^2"},
				})
			},
			md: "Inline $a^2$ then\n\n$$\nE=mc^2\n$$\n",
		},
		{
			name: "table_alignments_and_escaped_cells",
			doc: func() *Document {
				return fxDoc([]Block{{
					Kind:    BlockTable,
					Headers: []string{"Name", "Qty", "Price", "Note"},
					Aligns:  []string{"left", "center", "right", ""},
					Rows: [][]string{
						{"a|b", "x*y", `c\d`, "plain"},
						{"two", "2", "3.50", ""},
					},
				}})
			},
			md: "| Name | Qty | Price | Note |\n" +
				"| :--- | :---: | ---: | --- |\n" +
				"| a\\|b | x\\*y | c\\\\d | plain |\n" +
				"| two | 2 | 3.50 |  |\n",
		},
		{
			name: "table_header_only",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockTable, Headers: []string{"A", "B"}, Aligns: []string{"", ""}}})
			},
			md: "| A | B |\n| --- | --- |\n",
		},
		{
			name: "image_with_alt_and_title",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockImage, Src: "https://example.com/p.png", Text: "A logo", Title: "Brand mark"}})
			},
			md: "![A logo](https://example.com/p.png \"Brand mark\")\n",
		},
		{
			name: "image_with_escapes_and_spaced_destination",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockImage, Src: "dir/my pic.png", Text: "a [b] *c*", Title: `say "hi"`}})
			},
			md: "![a \\[b\\] \\*c\\*](<dir/my pic.png> \"say \\\"hi\\\"\")\n",
		},
		{
			name: "image_without_title_or_alt",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockImage, Src: "a.png"}})
			},
			md: "![](a.png)\n",
		},
		{
			name: "quote_multi_paragraph",
			doc: func() *Document {
				return fxDoc([]Block{{
					Kind: BlockQuote,
					Text: "first\n\nsecond",
					Runs: []TextRun{fxRun("first\n\n"), fxRun("second", "bold")},
				}})
			},
			md: "> first\n>\n> **second**\n",
		},
		{
			name: "quote_with_text_and_children",
			doc: func() *Document {
				return fxDoc([]Block{{
					Kind: BlockQuote,
					Text: "intro",
					Children: []Block{
						{Kind: BlockList, Items: fxItems("a", "b")},
						{Kind: BlockCode, Text: "code"},
					},
				}})
			},
			md: "> intro\n>\n> - a\n> - b\n>\n> ```\n> code\n> ```\n",
		},
		{
			name: "divider_block_between_paragraphs_in_one_section",
			doc: func() *Document {
				return fxDoc([]Block{fxPara("before"), {Kind: BlockDivider}, fxPara("after")})
			},
			md: "before\n\n***\n\nafter\n",
		},
		{
			name: "two_sections_are_separated_by_a_dash_rule",
			doc: func() *Document {
				return fxDoc([]Block{fxPara("one")}, []Block{fxPara("two")})
			},
			md: "one\n\n---\n\ntwo\n",
		},
		{
			name: "leading_and_trailing_empty_sections",
			doc: func() *Document {
				return fxDoc(nil, []Block{fxPara("mid")}, nil)
			},
			md: "---\n\nmid\n\n---\n",
		},
		{
			name: "front_matter_directives_sorted_and_quoted",
			doc: func() *Document {
				d := fxDoc([]Block{fxPara("body")})
				d.Meta.Directives = map[string]string{
					"theme": "default", "title": "A: B", "marp": "true", "empty": "", "note": "line1\nline2",
				}
				return d
			},
			md: "---\nempty: \"\"\nmarp: true\nnote: \"line1\\nline2\"\ntheme: default\ntitle: \"A: B\"\n---\n\nbody\n",
		},
		{
			name: "empty_document",
			doc:  func() *Document { return fxDoc() },
			md:   "\n",
		},
		{
			name: "front_matter_only",
			doc: func() *Document {
				d := fxDoc()
				d.Meta.Directives = map[string]string{"title": "T"}
				return d
			},
			md: "---\ntitle: T\n---\n",
		},
		{
			name: "kitchen_sink_blocks_join_with_one_blank_line",
			doc: func() *Document {
				d := fxDoc([]Block{
					{Kind: BlockHeading, Level: 1, Text: "Doc"},
					fxPara("Intro."),
					{Kind: BlockList, Items: fxItems("x", "y")},
					{Kind: BlockCode, Language: "sh", Text: "echo hi"},
					{Kind: BlockTable, Headers: []string{"k", "v"}, Aligns: []string{"", ""}, Rows: [][]string{{"a", "1"}}},
					{Kind: BlockQuote, Text: "quoted"},
				})
				d.Outline = []OutlineEntry{{SectionID: 1, Level: 1, Text: "Doc", Slug: "doc"}}
				return d
			},
			md: "# Doc\n\nIntro.\n\n- x\n- y\n\n```sh\necho hi\n```\n\n| k | v |\n| --- | --- |\n| a | 1 |\n\n> quoted\n",
		},
	}
}

// codecEscapeCase is one plain-text paragraph and the Markdown that must
// re-parse as exactly the same Text.
type codecEscapeCase struct {
	name string
	text string
	md   string
}

// codecEscapeCases covers text containing Markdown metacharacters. The md
// column is the exact serializer output; the text must survive a round trip
// unchanged.
func codecEscapeCases() []codecEscapeCase {
	return []codecEscapeCase{
		{"star_not_a_list", "* not a list", `\* not a list`},
		{"hash_not_a_heading", "# not a heading", `\# not a heading`},
		{"hash_without_space_is_literal", "#hashtag stays", `#hashtag stays`},
		{"ordered_marker_dot", "1. not a list", `1\. not a list`},
		{"ordered_marker_paren", "1) not a list", `1\) not a list`},
		{"pipe", "a | b", `a \| b`},
		{"dollars_are_not_math", "$5 and $6", `\$5 and \$6`},
		{"html_tag_text", "<div>", `\<div>`},
		{"brackets", "[x]", `\[x\]`},
		{"quote_marker", "> not a quote", `\> not a quote`},
		{"dash_bullet", "- not a bullet", `\- not a bullet`},
		{"plus_bullet", "+ not a bullet", `\+ not a bullet`},
		{"dash_number_is_literal", "-5 stays", `-5 stays`},
		{"setext_equals", "===", `\===`},
		{"intraword_underscore_is_literal", "snake_case_name", `snake_case_name`},
		{"edge_underscores_are_escaped", "_lead and trail_", `\_lead and trail\_`},
		{"backtick_tilde_backslash", "`tick` ~tilde~ \\back", "\\`tick\\` \\~tilde\\~ \\\\back"},
		{"entity_lookalikes", "AT&T &amp; &copy; &#65;", `AT&T \&amp; \&copy; \&#65;`},
		{"hash_inside_text", "C# and #1", `C# and #1`},
		{"trailing_hash_in_a_paragraph_is_literal", "issue #", `issue #`},
		{"leading_and_trailing_space", " lead and trail ", `&#32;lead and trail&#32;`},
		{"hard_break", "two\nlines", "two\\\nlines"},
		{"marker_after_hard_break", "line\n- inner", "line\\\n\\- inner"},
		{"backslash_before_hard_break", "a\\\nb", "a&#92;\\\nb"},
		{"table_delimiter_lookalike_colons", "x\n:--:", "x\\\n\\:--:"},
		{"table_delimiter_lookalike_dash_colon", "x\n-:", "x\\\n\\-:"},
		{"unicode_untouched", "café ☕ 日本語", "café ☕ 日本語"},
	}
}
