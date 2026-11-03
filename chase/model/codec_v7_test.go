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
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The v7-only kinds (objective 1026, TRD 02, Task 1): toggle, synced, todo,
// callout, file, sheet and figure. Expected Documents and Markdown are
// hand-written literals (codecV7Fixtures); nothing is produced by the reader or
// writer under test.

// TestCodecV7ToMarkdownExactBytes pins the spelling of every v7-only kind byte
// for byte.
func TestCodecV7ToMarkdownExactBytes(t *testing.T) {
	for _, fx := range codecV7Fixtures() {
		t.Run(fx.name, func(t *testing.T) {
			if got := ToMarkdown(fx.doc()); got != fx.md {
				t.Fatalf("ToMarkdown mismatch\n got: %q\nwant: %q", got, fx.md)
			}
		})
	}
}

// TestCodecV7Identity is the core claim for the v7 kinds: a hand-built Document
// survives ToMarkdown then FromMarkdown, the Markdown is a fixed point, and the
// hand-written Markdown reads as the same Document. Ids and spans are stripped
// here; Task 3 repeats the test with Prev and the trailer so ids count.
func TestCodecV7Identity(t *testing.T) {
	for _, fx := range codecV7Fixtures() {
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

// TestCodecV7ProseThatLooksLikeADirectiveIsEscaped: text that starts a line with
// `:::` would open (or close) a directive, so it is written with the first colon
// escaped and reads back as the same text.
func TestCodecV7ProseThatLooksLikeADirectiveIsEscaped(t *testing.T) {
	cases := []struct{ name, text, md string }{
		{"opener_shape", ":::toggle {\"collapsed\":true}", "\\:::toggle {\"collapsed\":true}"},
		{"closer_shape", ":::", "\\:::"},
		{"longer_run", "::::::x", "\\::::::x"},
		{"after_a_hard_break", "line\n:::callout", "line\\\n\\:::callout"},
		{"two_colons_are_literal", "::two", "::two"},
		{"colons_inside_a_line_are_literal", "a ::: b", "a ::: b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := fxDoc([]Block{fxPara(tc.text)})
			if got := ToMarkdown(want); got != tc.md+"\n" {
				t.Fatalf("ToMarkdown\n got: %q\nwant: %q", got, tc.md+"\n")
			}
			assertDocEqual(t, stripIDsAndSpans(mustFromMarkdown(t, tc.md+"\n", FromMarkdownOptions{})), want)
		})
	}
}

// TestCodecV7DirectiveHeaderIsOneJSONLine: the header is encoded with
// json.Marshal on a single line, which escapes `<`, `>` and `&`, so a value
// holding `-->`, markup or a newline cannot end the line or look like markup.
func TestCodecV7DirectiveHeaderIsOneJSONLine(t *testing.T) {
	nasty := "a --> b <x> & y\nz \u2028 \"q\""
	want := fxDoc([]Block{
		{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: nasty, WorkbookHint: "<script>"}},
		{Kind: BlockFile, Src: "f?a=1&b=2", Name: nasty},
	})
	md := ToMarkdown(want)
	for _, ln := range strings.Split(md, "\n") {
		if !strings.HasPrefix(ln, ":::") || ln == ":::" {
			continue
		}
		for _, banned := range []string{"-->", "<", ">", "&", "\u2028"} {
			if strings.Contains(ln, banned) {
				t.Fatalf("header line holds %q: %q", banned, ln)
			}
		}
	}
	if !strings.Contains(md, `\u003e`) || !strings.Contains(md, `\u003c`) || !strings.Contains(md, `\u0026`) {
		t.Fatalf("expected JSON escapes for < > & in:\n%s", md)
	}
	assertDocEqual(t, stripIDsAndSpans(mustFromMarkdown(t, md, FromMarkdownOptions{})), want)
}

// TestCodecV7ReadsHandWrittenSpellings: the reader accepts what a person
// writes, not only what ToMarkdown writes.
func TestCodecV7ReadsHandWrittenSpellings(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want *Document
	}{
		{
			name: "space_after_the_fence_and_spaced_json",
			src:  "::: toggle { \"collapsed\" : true }\nTitle\n:::\n",
			want: fxDoc([]Block{fxToggle("Title", true)}),
		},
		{
			name: "closing_fence_longer_than_the_opening_one",
			src:  ":::toggle\nTitle\n::::::\n",
			want: fxDoc([]Block{fxToggle("Title", false)}),
		},
		{
			name: "closing_fence_with_trailing_spaces",
			src:  ":::toggle\nTitle\n:::   \n\nafter\n",
			want: fxDoc([]Block{fxToggle("Title", false), fxPara("after")}),
		},
		{
			name: "capital_x_is_checked_and_star_is_a_bullet",
			src:  "* [X] done\n* [ ] open\n",
			want: fxDoc([]Block{{Kind: BlockTodo, Checked: true, Text: "done"}, {Kind: BlockTodo, Text: "open"}}),
		},
		{
			name: "loose_task_list_is_still_one_todo_per_item",
			src:  "- [ ] a\n\n- [x] b\n",
			want: fxDoc([]Block{{Kind: BlockTodo, Text: "a"}, {Kind: BlockTodo, Checked: true, Text: "b"}}),
		},
		{
			name: "directive_body_blocks_after_the_text_are_children",
			src:  ":::callout {\"tone\":\"info\"}\nHeads up\n\n> quoted\n\n```go\nx := 1\n```\n:::\n",
			want: fxDoc([]Block{{Kind: BlockCallout, Tone: "info", Text: "Heads up", Children: []Block{
				{Kind: BlockQuote, Text: "quoted"},
				{Kind: BlockCode, Language: "go", Text: "x := 1"},
			}}}),
		},
		{
			name: "a_directive_interrupts_a_paragraph",
			src:  "before\n:::file {\"src\":\"f.bin\"}\n:::\n",
			want: fxDoc([]Block{fxPara("before"), {Kind: BlockFile, Src: "f.bin"}}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertDocEqual(t, stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{})), tc.want)
		})
	}
}

// v7RefusalCases are constructs the v7 reader cannot represent; Offset is the
// first byte of the construct (for a directive, its opening fence).
func v7RefusalCases() []refusalCase {
	return []refusalCase{
		{"unknown_directive", "intro\n\n:::foo\nbody\n:::\n", "unknown-directive", ":::foo"},
		{"unknown_directive_in_a_quote", "> :::foo\n> x\n> :::\n", "unknown-directive", ":::foo"},
		{"directive_json_truncated", "para\n\n:::toggle {\"collapsed\":\nbody\n:::\n", "directive-json", ":::toggle"},
		{"directive_header_not_json", "para\n\n:::callout nope\nx\n:::\n", "directive-json", ":::callout"},
		{"directive_header_unknown_key", ":::toggle {\"bogus\":1}\n:::\n", "directive-json", ":::toggle"},
		{"directive_header_wrong_type", ":::file {\"size\":\"big\"}\n:::\n", "directive-json", ":::file"},
		{"directive_header_trailing_junk", ":::toggle {} x\n:::\n", "directive-json", ":::toggle"},
		{"directive_never_closed", "a\n\n:::toggle\nbody\n", "directive-unclosed", ":::toggle"},
		{"mixed_list_plain_item_first", "- a\n- [x] done\n", "list-mixed-todo", "[x]"},
		{"mixed_list_task_item_first", "- [ ] a\n- b\n", "list-mixed-todo", "[ ]"},
		{"task_item_nested_in_a_plain_list", "- a\n  - [ ] b\n", "list-mixed-todo", "[ ]"},
		{"task_item_in_an_ordered_list", "1. [ ] a\n", "list-mixed-todo", "[ ]"},
	}
}

// TestCodecV7StrictRefusals: a v7 construct the model cannot carry exactly is an
// *UnrepresentableError with the construct name and the byte offset of the
// construct (Test list 7).
func TestCodecV7StrictRefusals(t *testing.T) {
	for _, tc := range v7RefusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := FromMarkdown(tc.src, FromMarkdownOptions{})
			if err == nil {
				t.Fatalf("expected a refusal, got document:\n%s", docJSON(t, doc))
			}
			var ue *UnrepresentableError
			if !errors.As(err, &ue) {
				t.Fatalf("error is %T (%v), want *UnrepresentableError", err, err)
			}
			if ue.Construct != tc.construct {
				t.Fatalf("Construct = %q, want %q (%v)", ue.Construct, tc.construct, err)
			}
			want := strings.Index(tc.src, tc.at)
			if ue.Offset != want {
				t.Fatalf("Offset = %d, want %d (%q)", ue.Offset, want, tc.src[want:])
			}
		})
	}
}

// TestCodecV7LenientMappings pins what lenient mode does instead: a directive it
// cannot read stays as literal paragraph text, a never-closed one runs to the end
// of its container, and a mixed task list keeps each checkbox as literal text.
func TestCodecV7LenientMappings(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want *Document
	}{
		{"unknown_directive_stays_literal", "intro\n\n:::foo\nbody\n:::\n", fxDoc([]Block{fxPara("intro"), fxPara(":::foo\nbody\n:::")})},
		{"bad_json_stays_literal", ":::toggle {oops\nbody\n:::\n", fxDoc([]Block{fxPara(":::toggle {oops\nbody\n:::")})},
		{"unclosed_directive_runs_to_the_end", "a\n\n:::toggle\nbody\n", fxDoc([]Block{fxPara("a"), fxToggle("body", false)})},
		{"mixed_list_keeps_checkboxes_as_text", "- a\n- [x] done\n", fxDoc([]Block{{Kind: BlockList, Items: fxItems("a", "[x] done")}})},
		{"ordered_task_list_keeps_checkbox_as_text", "1. [ ] a\n", fxDoc([]Block{{Kind: BlockList, Ordered: true, Items: fxItems("[ ] a")}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{Lenient: true}))
			assertDocEqual(t, got, stripIDsAndSpans(tc.want))
		})
	}
}

// TestCodecV7IsStableOnGarbage reads seeded random soup that is heavy on
// directive fences, headers and task markers. Whatever it parses to, writing that
// and reading it again must give the same Document, and nothing may panic.
func TestCodecV7IsStableOnGarbage(t *testing.T) {
	alphabet := []string{
		"a", "b", " ", "  ", "\n", "\n\n", ":::", "::::", ":::toggle", ":::callout", ":::figure", ":::file", ":::sheet", ":::synced",
		":::foo", " {}", " {\"collapsed\":true}", " {\"icon\":\"x\",\"tone\":\"info\"}", " {\"noText\":true}", "{", "}", "\"", "- [ ] ", "- [x] ", "* [ ] ",
		"- ", "> ", "```", "$$", "# ", "---", "***", "<svg>", "</svg>", "\t",
	}
	rng := rand.New(rand.NewSource(11))
	bad := 0
	for i := 0; i < 20000 && bad < 5; i++ {
		src := sweepText(rng, alphabet, 1+rng.Intn(24))
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

// genV7Tree returns a random document body: trees of the v7 kinds (and the
// containers that hold them) built from text heavy on Markdown and fence
// punctuation, kept inside the canonical form.
func genV7Tree(rng *rand.Rand) []Block {
	alphabet := []string{"a", "B", "1", ".", " ", "*", "_", "`", "~", "#", ">", "-", "+", "|", "$", "<", "[", "]", "!", "&", "\\", ":", "::", ":::", "{", "}", "\"", "é", "x"}
	line := func() string { return sweepText(rng, alphabet, rng.Intn(7)) }
	text := func() string {
		s := line()
		for rng.Intn(5) == 0 {
			s += "\n" + line()
		}
		if strings.HasPrefix(s, "\n") || strings.HasSuffix(s, "\n") || strings.Contains(s, "\n\n") {
			return "t"
		}
		return s
	}
	var gen func(depth int) Block
	kids := func(depth int) []Block {
		if depth >= 3 {
			return nil
		}
		var out []Block
		for n := rng.Intn(4); n > 0; n-- {
			out = append(out, gen(depth+1))
		}
		return out
	}
	gen = func(depth int) Block {
		switch rng.Intn(12) {
		case 0:
			return Block{Kind: BlockToggle, Text: text(), Collapsed: rng.Intn(2) == 0, Children: kids(depth)}
		case 1:
			return Block{Kind: BlockToggle, Children: kids(depth)} // no title: the first child must not become one
		case 2:
			return Block{Kind: BlockCallout, Text: text(), Tone: []string{"", "info", "warn"}[rng.Intn(3)], Icon: []string{"", "!", "💡"}[rng.Intn(3)], Children: kids(depth)}
		case 3:
			return Block{Kind: BlockTodo, Text: text(), Checked: rng.Intn(2) == 0, Children: kids(depth)}
		case 4:
			return Block{Kind: BlockParagraph, Text: "p" + text()}
		case 5:
			return Block{Kind: BlockFile, Src: "s" + line(), Name: line(), Mime: []string{"", "text/plain"}[rng.Intn(2)], Size: int64(rng.Intn(3))}
		case 6:
			svg := sweepText(rng, []string{"<svg>", "</svg>", "\n", "\n\n", ":::", "::::", " ", "a", "<", "&", "\\"}, rng.Intn(8))
			return Block{Kind: BlockFigure, Src: line(), SVG: svg, Text: line()}
		case 7:
			// A quote's leading paragraphs are its Text, so a child paragraph straight
			// after the Text is not canonical.
			var rest []Block
			for _, k := range kids(depth) {
				if k.Kind != BlockParagraph {
					rest = append(rest, k)
				}
			}
			return Block{Kind: BlockQuote, Text: "q" + text(), Children: rest}
		case 8:
			return Block{Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_" + line(), PageID: line()}}
		case 9:
			return Block{Kind: BlockCode, Text: sweepText(rng, []string{"a", "\n", ":::", "::::", "`", "```", " "}, rng.Intn(6))}
		case 10:
			return Block{Kind: BlockList, Ordered: rng.Intn(2) == 0, Items: []ListItem{{Text: "i" + line()}, {Text: "j" + line(), Level: 1}}}
		default:
			return Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "S" + line(), WorkbookHint: line()}}
		}
	}
	var blocks []Block
	for n := 1 + rng.Intn(4); n > 0; n-- {
		blocks = append(blocks, gen(0))
	}
	cleanV7Tree(blocks)
	return blocks
}

// cleanV7Tree keeps a generated tree inside two limits of the canonical form:
//   - a whitespace-only line inside a list item is a blank line to the item, so
//     code nested in a todo cannot keep one;
//   - goldmark ends a todo (a list item) at the first block after a nested list
//     whose last item is empty and is followed by a blank line, so a child list
//     never ends in an empty item here (genV7Tree never makes one).
func cleanV7Tree(bs []Block) {
	fix := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, ln := range lines {
			if strings.TrimSpace(ln) == "" {
				lines[i] = ""
			}
		}
		return strings.Join(lines, "\n")
	}
	for i := range bs {
		bs[i].SVG = fix(bs[i].SVG)
		if bs[i].Kind == BlockCode {
			bs[i].Text = fix(bs[i].Text)
		}
		cleanV7Tree(bs[i].Children)
	}
}

// TestCodecV7BlockSweep checks that each random tree comes back from ToMarkdown
// then FromMarkdown unchanged.
func TestCodecV7BlockSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(1026))
	fails := 0
	for i := 0; i < 3000; i++ {
		d := fxDoc(genV7Tree(rng))
		md := ToMarkdown(d)
		got, err := FromMarkdown(md, FromMarkdownOptions{})
		if err == nil && reflect.DeepEqual(got, d) {
			continue
		}
		fails++
		if fails <= 2 {
			gotJSON := "nil"
			if got != nil {
				gotJSON = docJSON(t, got)
			}
			t.Errorf("tree does not round-trip\n markdown: %q\n error: %v\n want: %s\n got: %s", md, err, docJSON(t, d), gotJSON)
		}
	}
	if fails > 0 {
		t.Fatalf("%d round-trip failures", fails)
	}
}

// ---- sections, notes, attrs, front matter, properties and comments (TRD 02, Task 2) ----

func TestCodecSectionLayerExactBytes(t *testing.T) {
	for _, fx := range codecSectionFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			if got := ToMarkdown(fx.doc()); got != fx.md {
				t.Fatalf("ToMarkdown mismatch\n got: %q\nwant: %q", got, fx.md)
			}
		})
	}
}

// TestCodecSectionLayerIdentity (Test list 2 and 4): section attrs, notes (order
// kept), front matter, properties, property values and comments survive
// docmodel -> Markdown -> docmodel exactly, and the Markdown is a fixed point.
func TestCodecSectionLayerIdentity(t *testing.T) {
	for _, fx := range codecSectionFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			want := stripIDsAndSpans(fx.doc())
			md := ToMarkdown(fx.doc())
			got := stripIDsAndSpans(mustFromMarkdown(t, md, FromMarkdownOptions{}))
			assertDocEqual(t, got, want)
			if again := ToMarkdown(got); again != md {
				t.Fatalf("Markdown is not a fixed point\n first: %q\nsecond: %q", md, again)
			}
			assertDocEqual(t, stripIDsAndSpans(mustFromMarkdown(t, fx.md, FromMarkdownOptions{})), want)
		})
	}
}

// TestCodecNoTrailerWithoutDocumentLevelData: a document with no properties,
// property values, comments or ids keeps its Markdown free of the trailer.
func TestCodecNoTrailerWithoutDocumentLevelData(t *testing.T) {
	all := append(append(codecFixtures(), codecV7Fixtures()...), codecSectionFixtures()[:3]...)
	for _, fx := range all {
		if got := ToMarkdown(fx.doc()); strings.Contains(got, "eden:model") {
			t.Fatalf("%s: trailer emitted for a document with no document-level data:\n%s", fx.name, got)
		}
	}
	if got := ToMarkdown(NewDocument("T")); strings.Contains(got, "eden:model") {
		t.Fatalf("empty document: %q", got)
	}
}

// TestCodecLegacyMarpInput (Test list 3): Marp-spelled input still reads. A
// comment whose YAML keys are all known directives is a section attr, any other
// comment is a note, and front matter becomes Meta.Directives. What is read is
// then written in the canonical spellings and reads back unchanged.
func TestCodecLegacyMarpInput(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want func() *Document
	}{
		{
			name: "spot_directive_note_and_front_matter",
			src:  "---\ntheme: gaia\nsize: 16:9\n---\n\n<!-- _class: lead -->\n\n# Title\n\n<!-- TODO: fix -->\n\nbody\n",
			want: func() *Document {
				d := fxDoc([]Block{{Kind: BlockHeading, Level: 1, Text: "Title"}, fxPara("body")})
				d.Meta.Directives = map[string]string{"theme": "gaia", "size": "16:9"}
				d.Sections[0].Attrs = map[string]string{"_class": "lead"}
				d.Sections[0].Notes = []string{"TODO: fix"}
				d.Outline = []OutlineEntry{{SectionID: 1, Level: 1, Text: "Title", Slug: "title"}}
				return d
			},
		},
		{
			name: "multi_line_yaml_comment_is_several_attrs",
			src:  "<!--\n_class: lead\npaginate: true\n-->\n\nx\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("x")})
				d.Sections[0].Attrs = map[string]string{"_class": "lead", "paginate": "true"}
				return d
			},
		},
		{
			name: "one_unknown_key_makes_the_whole_comment_a_note",
			src:  "<!-- _class: lead\nTODO: fix -->\n\nx\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("x")})
				d.Sections[0].Notes = []string{"_class: lead\nTODO: fix"}
				return d
			},
		},
		{
			name: "plain_comment_is_a_note_and_order_is_kept",
			src:  "<!-- one -->\n\ntext\n\n<!-- two -->\n\nmore\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("text"), fxPara("more")})
				d.Sections[0].Notes = []string{"one", "two"}
				return d
			},
		},
		{
			name: "attrs_belong_to_the_section_they_are_in",
			src:  "<!-- _class: lead -->\n\na\n\n---\n\n<!-- _class: end -->\n<!-- paginate: false -->\n\nb\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("a")}, []Block{fxPara("b")})
				d.Sections[0].Attrs = map[string]string{"_class": "lead"}
				d.Sections[1].Attrs = map[string]string{"_class": "end", "paginate": "false"}
				return d
			},
		},
		{
			name: "a_comment_straight_after_a_paragraph_line",
			src:  "line\n<!-- _footer: hi -->\nnext\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("line"), fxPara("next")})
				d.Sections[0].Attrs = map[string]string{"_footer": "hi"}
				return d
			},
		},
		{
			name: "an_unknown_eden_comment_is_an_ordinary_note",
			src:  "<!-- eden:future thing -->\n\nx\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("x")})
				d.Sections[0].Notes = []string{"eden:future thing"}
				return d
			},
		},
		{
			name: "an_empty_comment_is_an_empty_note",
			src:  "<!---->\n\nx\n",
			want: func() *Document {
				d := fxDoc([]Block{fxPara("x")})
				d.Sections[0].Notes = []string{""}
				return d
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want()
			got := stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{}))
			assertDocEqual(t, got, want)

			md := ToMarkdown(got)
			if strings.Contains(md, "<!-- _") || strings.Contains(md, "<!-- paginate") || strings.Contains(md, "<!-- TODO") {
				t.Fatalf("a legacy spelling was written back instead of the canonical one:\n%s", md)
			}
			assertDocEqual(t, stripIDsAndSpans(mustFromMarkdown(t, md, FromMarkdownOptions{})), want)
		})
	}
}

// TestCodecTrailerCanSitAnywhere: the trailer is found wherever it is at the top
// level, never becomes a note, and is not part of any section.
func TestCodecTrailerCanSitAnywhere(t *testing.T) {
	src := "body\n\n<!-- eden:model {\"v\":1,\"propertyValues\":{\"a\":\"b\"}} -->\n\nmore\n\n---\n\nnext\n"
	d := stripIDsAndSpans(mustFromMarkdown(t, src, FromMarkdownOptions{}))
	want := fxDoc([]Block{fxPara("body"), fxPara("more")}, []Block{fxPara("next")})
	want.PropertyValues = map[string]string{"a": "b"}
	assertDocEqual(t, d, want)
}

// commentRefusalCases are malformed canonical comments (Test list 7's
// "directive-json" for the section layer) and a comment that is not just a
// comment. Offset is the `<!--`.
func commentRefusalCases() []refusalCase {
	return []refusalCase{
		{"attrs_not_json", "<!-- eden:attrs {nope} -->\n\nx\n", "directive-json", "<!--"},
		{"attrs_not_an_object", "<!-- eden:attrs [\"a\"] -->\n\nx\n", "directive-json", "<!--"},
		{"attrs_value_not_a_string", "<!-- eden:attrs {\"a\":1} -->\n\nx\n", "directive-json", "<!--"},
		{"note_not_json", "intro\n\n<!-- eden:note not-json -->\n", "directive-json", "<!--"},
		{"note_not_a_string", "<!-- eden:note 5 -->\n\nx\n", "directive-json", "<!--"},
		{"trailer_not_json", "x\n\n<!-- eden:model {nope} -->\n", "directive-json", "<!--"},
		{"trailer_unknown_version", "x\n\n<!-- eden:model {\"v\":2} -->\n", "directive-json", "<!--"},
		{"trailer_unknown_key", "x\n\n<!-- eden:model {\"v\":1,\"bogus\":1} -->\n", "directive-json", "<!--"},
		{"trailer_twice", "x\n\n<!-- eden:model {\"v\":1} -->\n\n<!-- eden:model {\"v\":1} -->\n", "directive-json", "<!-- eden:model {\"v\":1} -->\n"},
		{"comment_with_trailing_text", "intro\n\n<!-- note --> trailing\n", "raw-html", "<!--"},
		{"comment_inside_a_quote", "> <!-- note -->\n", "raw-html", "<!--"},
	}
}

func TestCodecCommentStrictRefusals(t *testing.T) {
	for _, tc := range commentRefusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromMarkdown(tc.src, FromMarkdownOptions{})
			var ue *UnrepresentableError
			if !errors.As(err, &ue) {
				t.Fatalf("error is %v, want *UnrepresentableError", err)
			}
			if ue.Construct != tc.construct {
				t.Fatalf("Construct = %q, want %q (%v)", ue.Construct, tc.construct, err)
			}
			want := strings.Index(tc.src, tc.at)
			if tc.name == "trailer_twice" {
				want = strings.LastIndex(tc.src, tc.at)
			}
			if ue.Offset != want {
				t.Fatalf("Offset = %d, want %d", ue.Offset, want)
			}
		})
	}
}

// TestCodecMalformedCommentsAreNotesWhenLenient: nothing is dropped; a comment
// that cannot be read as what it claims to be stays as a note, verbatim.
func TestCodecMalformedCommentsAreNotesWhenLenient(t *testing.T) {
	cases := []struct{ name, src, note string }{
		{"attrs", "<!-- eden:attrs {nope} -->\n\nx\n", "eden:attrs {nope}"},
		{"note", "<!-- eden:note not-json -->\n\nx\n", "eden:note not-json"},
		{"trailer", "x\n\n<!-- eden:model {nope} -->\n", "eden:model {nope}"},
		{"trailer_version", "x\n\n<!-- eden:model {\"v\":2} -->\n", "eden:model {\"v\":2}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := stripIDsAndSpans(mustFromMarkdown(t, tc.src, FromMarkdownOptions{Lenient: true}))
			if len(d.Sections) != 1 || !reflect.DeepEqual(d.Sections[0].Notes, []string{tc.note}) {
				t.Fatalf("notes = %q, want [%q]\n%s", d.Sections[0].Notes, tc.note, docJSON(t, d))
			}
			md := ToMarkdown(d)
			if again := stripIDsAndSpans(mustFromMarkdown(t, md, FromMarkdownOptions{})); !reflect.DeepEqual(again, d) {
				t.Fatalf("a kept note is not stable:\n%s", md)
			}
		})
	}
}

// TestCodecSectionLayerSweep builds random documents whose sections, attrs,
// notes, properties, property values and comments hold awkward strings (quotes,
// `-->`, `<`, newlines, unicode, empty), and checks each survives the trip.
func TestCodecSectionLayerSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(2611))
	alphabet := []string{"a", "B", " ", "-", "--", "-->", "<!--", "<", ">", "&", "\"", "\\", "\n", ":", "eden:", "é", "☕", "{", "}", "#", "_", "x"}
	str := func() string { return sweepText(rng, alphabet, rng.Intn(6)) }
	fails := 0
	for i := 0; i < 4000; i++ {
		var secs [][]Block
		for n := 1 + rng.Intn(4); n > 0; n-- {
			if rng.Intn(3) == 0 {
				secs = append(secs, nil)
			} else {
				secs = append(secs, []Block{fxPara("p" + strconv.Itoa(rng.Intn(100)))})
			}
		}
		d := fxDoc(secs...)
		for k := range d.Sections {
			if rng.Intn(2) == 0 {
				d.Sections[k].Attrs = map[string]string{}
				for n := 1 + rng.Intn(3); n > 0; n-- {
					d.Sections[k].Attrs[str()] = str()
				}
			}
			for n := rng.Intn(4); n > 0; n-- {
				d.Sections[k].Notes = append(d.Sections[k].Notes, str())
			}
		}
		if rng.Intn(2) == 0 {
			d.PropertyValues = map[string]string{str(): str(), str(): str()}
			d.Properties = []PropertyDef{{ID: str(), Name: str(), Type: PropText}}
			if rng.Intn(2) == 0 {
				d.Properties[0].Options = []string{str(), str()}
			}
		}
		for n := rng.Intn(3); n > 0; n-- {
			c := Comment{ID: str(), BlockID: str(), Body: str(), Resolved: rng.Intn(2) == 0}
			if rng.Intn(2) == 0 {
				c.Start, c.Stop = intp(rng.Intn(5)), intp(5+rng.Intn(5))
			}
			if rng.Intn(2) == 0 {
				c.Suggestion = &CommentSuggestion{Text: str(), Runs: []TextRun{{Text: str(), Marks: []string{"bold"}}}}
			}
			d.Comments = append(d.Comments, c)
		}
		md := ToMarkdown(d)
		got, err := FromMarkdown(md, FromMarkdownOptions{})
		if err == nil && reflect.DeepEqual(got, d) {
			continue
		}
		fails++
		if fails <= 2 {
			gotJSON := "nil"
			if got != nil {
				gotJSON = docJSON(t, got)
			}
			t.Errorf("does not round-trip\n markdown: %q\n error: %v\n want: %s\n got: %s", md, err, docJSON(t, d), gotJSON)
		}
	}
	if fails > 0 {
		t.Fatalf("%d round-trip failures", fails)
	}
}

// TestCodecSectionLayerIsStableOnGarbage reads random soup heavy on comments and
// section rules: whatever it parses to, writing and reading again must give the
// same document.
func TestCodecSectionLayerIsStableOnGarbage(t *testing.T) {
	alphabet := []string{
		"a", "\n", "\n\n", " ", "---", "<!--", "-->", "<!-- x -->", "<!-- _class: lead -->", "<!-- TODO: fix -->",
		"<!-- eden:attrs {\"a\":\"b\"} -->", "<!-- eden:attrs {x} -->", "<!-- eden:note \"n\" -->", "<!-- eden:note x -->",
		"<!-- eden:model {\"v\":1} -->", "<!-- eden:model {\"v\":1,\"propertyValues\":{\"k\":\"v\"}} -->",
		"<!-- eden:model {\"v\":9} -->", "<!--", "eden:note", "> ", "- ", ":::toggle\n", ":::\n", "# h",
	}
	rng := rand.New(rand.NewSource(31))
	bad := 0
	for i := 0; i < 20000 && bad < 5; i++ {
		src := sweepText(rng, alphabet, 1+rng.Intn(14))
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
