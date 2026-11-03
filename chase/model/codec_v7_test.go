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

// TestCodecV7BlockSweep builds random trees of the v7 kinds (and the containers
// that hold them) from text heavy on Markdown and fence punctuation, and checks
// that each comes back from ToMarkdown then FromMarkdown unchanged.
func TestCodecV7BlockSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(1026))
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
	// Two limits of the canonical form, which the generator stays inside:
	//   - a whitespace-only line inside a list item is a blank line to the item, so
	//     code nested in a todo cannot keep one;
	//   - goldmark ends a todo (a list item) at the first block after a nested list
	//     whose last item is empty and is followed by a blank line, so a child list
	//     never ends in an empty item here.
	var clean func(bs []Block)
	clean = func(bs []Block) {
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
			clean(bs[i].Children)
		}
	}
	fails := 0
	for i := 0; i < 3000; i++ {
		var blocks []Block
		for n := 1 + rng.Intn(4); n > 0; n-- {
			blocks = append(blocks, gen(0))
		}
		clean(blocks)
		d := fxDoc(blocks)
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
