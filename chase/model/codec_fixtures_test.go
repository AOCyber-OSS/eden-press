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

// ---- v7-only kinds (objective 1026, TRD 02) ----
//
// The seven kinds with no GFM spelling are written as container directives
// (`:::kind {json}` ... `:::`), and todo as a GFM task item. As above, every
// Document and every Markdown string is a hand-written literal; the JSON header
// lines are what encoding/json produces for the header struct (fixed field
// order, `<` `>` `&` escaped), written out by hand.

// fxIDs gives d the ids ApplyOps would: b_N depth-first, li_N in document
// order. It uses the package's own EnsureBlockIDs / EnsureListItemIDs, never the
// importer under test.
func fxIDs(d *Document) *Document {
	EnsureBlockIDs(d)
	EnsureListItemIDs(d)
	return d
}

func fxToggle(title string, collapsed bool, children ...Block) Block {
	return Block{Kind: BlockToggle, Text: title, Collapsed: collapsed, Children: children}
}

func codecV7Fixtures() []codecFixture {
	return []codecFixture{
		{
			name: "toggle_collapsed_with_nested_toggle",
			doc: func() *Document {
				return fxDoc([]Block{fxToggle("Details", true,
					fxPara("first"),
					fxToggle("Inner", false, fxPara("deep")),
				)})
			},
			md: "::::toggle {\"collapsed\":true}\nDetails\n\nfirst\n\n:::toggle\nInner\n\ndeep\n:::\n::::\n",
		},
		{
			name: "toggle_title_with_marks",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockToggle, Text: "Read me", Runs: []TextRun{fxRun("Read", "bold"), fxRun(" me")}, Children: []Block{fxPara("x")}}})
			},
			md: ":::toggle\n**Read** me\n\nx\n:::\n",
		},
		{
			// With no title, a first child paragraph would read as the title, so
			// the header says so.
			name: "toggle_without_title_whose_first_child_is_a_paragraph",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockToggle, Children: []Block{fxPara("child")}}})
			},
			md: ":::toggle {\"noText\":true}\nchild\n:::\n",
		},
		{
			name: "toggle_without_title_whose_first_child_is_a_list",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockToggle, Children: []Block{{Kind: BlockList, Items: fxItems("a", "b")}}}})
			},
			md: ":::toggle\n- a\n- b\n:::\n",
		},
		{
			name: "toggle_and_callout_with_nothing_inside",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockToggle}, {Kind: BlockCallout}})
			},
			md: ":::toggle\n:::\n\n:::callout\n:::\n",
		},
		{
			name: "synced_same_document_and_other_page",
			doc: func() *Document {
				return fxDoc([]Block{
					fxPara("source"),
					{Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_1"}},
					{Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_9", PageID: "pg_2"}},
				})
			},
			md: "source\n\n:::synced {\"syncRef\":{\"blockId\":\"b_1\"}}\n:::\n\n:::synced {\"syncRef\":{\"blockId\":\"b_9\",\"pageId\":\"pg_2\"}}\n:::\n",
		},
		{
			name: "todo_checked_unchecked_marks_and_children",
			doc: func() *Document {
				return fxDoc([]Block{
					{Kind: BlockTodo, Text: "buy milk"},
					{Kind: BlockTodo, Checked: true, Text: "ship it", Runs: []TextRun{fxRun("ship "), fxRun("it", "bold")}},
					{Kind: BlockTodo, Text: "plan", Children: []Block{fxPara("details"), {Kind: BlockTodo, Checked: true, Text: "sub"}}},
					fxPara("after"),
				})
			},
			md: "- [ ] buy milk\n- [x] ship **it**\n- [ ] plan\n\n  details\n\n  - [x] sub\n\nafter\n",
		},
		{
			name: "todo_with_empty_text_and_a_hard_break",
			doc: func() *Document {
				return fxDoc([]Block{
					{Kind: BlockTodo},
					{Kind: BlockTodo, Checked: true, Text: "a\nb"},
					{Kind: BlockTodo, Text: "", Children: []Block{fxPara("c")}},
				})
			},
			md: "- [ ]\n- [x] a\\\n  b\n- [ ]\n\n  c\n",
		},
		{
			// A todo list and a plain list in the same marker would be one
			// (mixed) list, so the marker alternates between them.
			name: "todo_next_to_plain_lists_alternates_the_marker",
			doc: func() *Document {
				return fxDoc([]Block{
					{Kind: BlockList, Items: fxItems("a", "b")},
					{Kind: BlockTodo, Text: "t1"},
					{Kind: BlockTodo, Checked: true, Text: "t2"},
					{Kind: BlockList, Items: fxItems("c")},
					{Kind: BlockTodo, Text: "t3"},
				})
			},
			md: "- a\n- b\n\n* [ ] t1\n* [x] t2\n\n- c\n\n* [ ] t3\n",
		},
		{
			name: "callout_with_icon_tone_runs_and_a_child_list",
			doc: func() *Document {
				return fxDoc([]Block{{
					Kind: BlockCallout, Icon: "💡", Tone: "info",
					Text: "Remember hydration", Runs: []TextRun{fxRun("Remember "), fxRun("hydration", "bold")},
					Children: []Block{{Kind: BlockList, Items: fxItems("one", "two")}},
				}})
			},
			md: ":::callout {\"icon\":\"💡\",\"tone\":\"info\"}\nRemember **hydration**\n\n- one\n- two\n:::\n",
		},
		{
			// The first paragraph of a directive body is its text, even when that
			// is only math (a bare paragraph of only math is a math block).
			name: "callout_whose_text_is_only_math",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockCallout, Text: "x^2", Runs: []TextRun{fxRun("x^2", "math")}}})
			},
			md: ":::callout\n$x^2$\n:::\n",
		},
		{
			name: "callout_inside_a_quote",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockQuote, Text: "Quoted", Children: []Block{{Kind: BlockCallout, Tone: "warn", Text: "inside"}}}})
			},
			md: "> Quoted\n>\n> :::callout {\"tone\":\"warn\"}\n> inside\n> :::\n",
		},
		{
			name: "file_with_every_field",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockFile, Name: "report.pdf", Mime: "application/pdf", Size: 2048, Src: "files/report.pdf"}})
			},
			md: ":::file {\"name\":\"report.pdf\",\"mime\":\"application/pdf\",\"size\":2048,\"src\":\"files/report.pdf\"}\n:::\n",
		},
		{
			name: "sheet_embed_with_snapshot_formula_and_rels",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockSheet, SheetEmbed: &SheetEmbed{
					PageID: "pg_1", WorkbookHint: "Q3 budget", Sheet: "Budget",
					Range: &SheetRange{R0: 0, R1: 2, C0: 0, C1: 1},
					Snapshot: &SheetSnapshot{
						Columns: []SheetColumnSnap{{ID: "c1", Name: "Item", Type: "text"}, {ID: "c2", Name: "Qty", Type: "number"}},
						RowIDs:  []SheetRowIDSnap{{R: 0, ID: "row0"}, {R: 1, ID: "row1"}},
						Cells: []SheetCellSnap{
							{R: 0, C: 0, V: "Item"}, {R: 0, C: 1, V: "Qty"},
							{R: 1, C: 0, V: "widgets"},
							{R: 1, C: 1, V: "4", F: "=A2*2", Rels: []map[string]string{{"row": "row1", "table": "t1"}}},
						},
					},
				}}})
			},
			md: ":::sheet {\"sheetEmbed\":{\"pageId\":\"pg_1\",\"workbookHint\":\"Q3 budget\",\"sheet\":\"Budget\",\"range\":{\"r1\":2,\"c1\":1}," +
				"\"snapshot\":{\"columns\":[{\"id\":\"c1\",\"name\":\"Item\",\"type\":\"text\"},{\"id\":\"c2\",\"name\":\"Qty\",\"type\":\"number\"}]," +
				"\"rowIds\":[{\"r\":0,\"id\":\"row0\"},{\"r\":1,\"id\":\"row1\"}]," +
				"\"cells\":[{\"r\":0,\"c\":0,\"v\":\"Item\"},{\"r\":0,\"c\":1,\"v\":\"Qty\"},{\"r\":1,\"c\":0,\"v\":\"widgets\"}," +
				"{\"r\":1,\"c\":1,\"v\":\"4\",\"f\":\"=A2*2\",\"rels\":[{\"row\":\"row1\",\"table\":\"t1\"}]}]}}}\n:::\n",
		},
		{
			name: "figure_with_src_title_alt_and_inline_svg",
			doc: func() *Document {
				return fxDoc([]Block{{
					Kind: BlockFigure, Src: "a.png", Title: "Chart", Text: "alt text",
					SVG: "<svg width=\"10\" height=\"10\">\n  <path d=\"M0 0L1 1\"/>\n  <text>a < b</text>\n</svg>",
				}})
			},
			md: ":::figure {\"src\":\"a.png\",\"title\":\"Chart\",\"alt\":\"alt text\"}\n<svg width=\"10\" height=\"10\">\n  <path d=\"M0 0L1 1\"/>\n  <text>a < b</text>\n</svg>\n:::\n",
		},
		{
			// The SVG body is verbatim: a blank line, a trailing newline and lines
			// that look like fences inside it all survive, and the fence grows.
			name: "figure_svg_with_blank_lines_and_fence_lookalikes",
			doc: func() *Document {
				return fxDoc([]Block{{Kind: BlockFigure, SVG: "<svg>\n:::\n\n::::: x\n</svg>\n"}})
			},
			md: "::::::figure\n<svg>\n:::\n\n::::: x\n</svg>\n\n::::::\n",
		},
		{
			name: "three_levels_of_nesting_around_a_code_block_with_a_fence_line",
			doc: func() *Document {
				code := Block{Kind: BlockCode, Text: "a\n:::\nb"}
				return fxDoc([]Block{fxToggle("A", false, fxToggle("B", false, fxToggle("C", false, code)))})
			},
			md: "::::::toggle\nA\n\n:::::toggle\nB\n\n::::toggle\nC\n\n```\na\n:::\nb\n```\n::::\n:::::\n::::::\n",
		},
		{
			name: "all_seven_kinds_after_a_heading",
			doc: func() *Document {
				d := fxDoc([]Block{
					{Kind: BlockHeading, Level: 1, Text: "Kinds"},
					fxToggle("t", true),
					{Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_1"}},
					{Kind: BlockTodo, Text: "do"},
					{Kind: BlockCallout, Text: "note"},
					{Kind: BlockFile, Src: "f.bin"},
					{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "S"}},
					{Kind: BlockFigure, SVG: "<svg/>"},
				})
				d.Outline = []OutlineEntry{{SectionID: 1, Level: 1, Text: "Kinds", Slug: "kinds"}}
				return d
			},
			md: "# Kinds\n\n:::toggle {\"collapsed\":true}\nt\n:::\n\n:::synced {\"syncRef\":{\"blockId\":\"b_1\"}}\n:::\n\n- [ ] do\n\n" +
				":::callout\nnote\n:::\n\n:::file {\"src\":\"f.bin\"}\n:::\n\n:::sheet {\"sheetEmbed\":{\"sheet\":\"S\"}}\n:::\n\n:::figure\n<svg/>\n:::\n",
		},
	}
}

// ---- sections, notes, attrs, properties and comments (objective 1026, TRD 02) ----

func intp(n int) *int { return &n }

// codecSectionFixtures cover the layer around the blocks: a section's attrs and
// notes, document-level properties, and comments. The JSON is what json.Marshal
// produces (sorted map keys, `<` `>` `&` escaped), written out by hand.
func codecSectionFixtures() []codecFixture {
	return []codecFixture{
		{
			// Attrs and notes use the same `<!-- ... -->` syntax in Marp, so the
			// canonical spellings name themselves: no string can be mistaken for
			// the other, including a note that looks like `key: value` or holds `-->`.
			name: "section_attrs_and_notes_are_unambiguous",
			doc: func() *Document {
				d := fxDoc([]Block{fxPara("body")})
				d.Sections[0].Attrs = map[string]string{"paginate": "true", "data-theme": "gaia"}
				d.Sections[0].Notes = []string{"key: value", "a --> b", "plain"}
				return d
			},
			md: `<!-- eden:attrs {"data-theme":"gaia","paginate":"true"} -->

body

<!-- eden:note "key: value" -->
<!-- eden:note "a --\u003e b" -->
<!-- eden:note "plain" -->
`,
		},
		{
			name: "sections_with_attrs_only_notes_only_and_nothing",
			doc: func() *Document {
				d := fxDoc(
					[]Block{{Kind: BlockHeading, Level: 1, Text: "One"}},
					nil,
					nil,
					[]Block{fxPara("four")},
				)
				d.Sections[0].Attrs = map[string]string{"_class": "lead"}
				d.Sections[1].Notes = []string{"only a note"}
				d.Outline = []OutlineEntry{{SectionID: 1, Level: 1, Text: "One", Slug: "one"}}
				return d
			},
			md: `<!-- eden:attrs {"_class":"lead"} -->

# One

---

<!-- eden:note "only a note" -->

---

---

four
`,
		},
		{
			name: "notes_with_newlines_quotes_markup_and_nothing",
			doc: func() *Document {
				d := fxDoc()
				d.Sections[0].Notes = []string{"line1\nline2", "say \"hi\"", "é ☕ <b>", ""}
				return d
			},
			md: `<!-- eden:note "line1\nline2" -->
<!-- eden:note "say \"hi\"" -->
<!-- eden:note "é ☕ \u003cb\u003e" -->
<!-- eden:note "" -->
`,
		},
		{
			// Properties, property values and comments have no place in the body, so
			// they ride in one trailing comment. Comments keep their anchors, ranges,
			// resolved flag and suggestions (with runs).
			name: "document_level_data_rides_in_one_trailing_comment",
			doc: func() *Document {
				d := fxDoc([]Block{fxPara("hello")})
				d.Meta.Directives = map[string]string{"title": "A: B # c"}
				d.Properties = []PropertyDef{
					{ID: "status", Name: "Status", Type: PropSelect, Options: []string{"todo", "done"}},
					{ID: "due", Name: "Due", Type: PropDate},
				}
				d.PropertyValues = map[string]string{"status": "todo", "due": "2026-11-03"}
				d.Comments = []Comment{
					{ID: "c_1", BlockID: "b_1", Start: intp(0), Stop: intp(5), Body: "fix <this>"},
					{ID: "c_2", BlockID: "b_1", Body: "done already", Resolved: true},
					{ID: "c_3", BlockID: "b_2", Body: "suggest", Suggestion: &CommentSuggestion{
						Text: "fixed text", Runs: []TextRun{fxRun("fixed", "bold"), fxRun(" text")},
					}},
				}
				return d
			},
			md: `---
title: "A: B # c"
---

hello

<!-- eden:model {"v":1,"properties":[{"id":"status","name":"Status","type":"select","options":["todo","done"]},{"id":"due","name":"Due","type":"date"}],"propertyValues":{"due":"2026-11-03","status":"todo"},"comments":[{"id":"c_1","blockId":"b_1","start":0,"stop":5,"body":"fix \u003cthis\u003e"},{"id":"c_2","blockId":"b_1","body":"done already","resolved":true},{"id":"c_3","blockId":"b_2","body":"suggest","suggestion":{"text":"fixed text","runs":[{"text":"fixed","marks":["bold"]},{"text":" text"}]}}]} -->
`,
		},
		{
			// A document with only document-level data still writes the trailer.
			name: "trailer_alone_when_the_body_is_empty",
			doc: func() *Document {
				d := fxDoc()
				d.PropertyValues = map[string]string{"k": "v"}
				return d
			},
			md: `<!-- eden:model {"v":1,"propertyValues":{"k":"v"}} -->
`,
		},
	}
}
