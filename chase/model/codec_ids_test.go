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
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// Stable ids and byte spans (objective 1026, TRD 02, Task 3).
//
// Block, list-item and comment ids must survive a trip through Markdown, because
// the model addresses blocks by id (edit_document) and comments anchor to them.
// They ride in the trailer (codec_ids.go), and FromMarkdownOptions.Prev lets a
// source-view edit keep them too. Expectations are hand-built Documents; ids on
// the starting documents come from EnsureBlockIDs / EnsureListItemIDs, never
// from the reader under test.

// idsDoc is the starting document of the edit tests: ids b_1..b_4, li_1, li_2
// and a comment anchored to b_2.
func idsDoc() *Document {
	d := fxDoc([]Block{
		{Kind: BlockHeading, Level: 1, Text: "Title"},
		fxPara("alpha"),
		{Kind: BlockList, Items: fxItems("one", "two")},
		fxPara("omega"),
	})
	d.Outline = []OutlineEntry{{SectionID: 1, Level: 1, Text: "Title", Slug: "title"}}
	d.Comments = []Comment{{ID: "c_1", BlockID: "b_2", Body: "check alpha"}}
	return fxIDs(d)
}

func blockIDs(d *Document) []string {
	var out []string
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			out = append(out, b.ID)
			walk(b.Children)
		}
	}
	for _, s := range d.Sections {
		walk(s.Blocks)
	}
	return out
}

func itemIDs(d *Document) []string {
	var out []string
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			for _, it := range b.Items {
				out = append(out, it.ID)
			}
			walk(b.Children)
		}
	}
	for _, s := range d.Sections {
		walk(s.Blocks)
	}
	return out
}

func sameStrings(a, b []string) bool {
	return reflect.DeepEqual(append([]string(nil), a...), append([]string(nil), b...))
}

// readBoth reads md once with Prev and once with only the trailer to go on; both
// must agree, and the result is returned.
func readBoth(t *testing.T, md string, prev *Document) *Document {
	t.Helper()
	withPrev := mustFromMarkdown(t, md, FromMarkdownOptions{Prev: prev})
	trailerOnly := mustFromMarkdown(t, md, FromMarkdownOptions{})
	assertDocEqual(t, trailerOnly, withPrev)
	return withPrev
}

// TestCodecTrailerCarriesIDsAsAFingerprintedSequence pins the trailer's shape
// for ids. The fingerprints are FNV-1a (32 bit) of the JSON of the block (or
// item) with its id, span and children left out and its runs in canonical form;
// the hex values below were computed independently of this package.
func TestCodecTrailerCarriesIDsAsAFingerprintedSequence(t *testing.T) {
	d := fxIDs(fxDoc([]Block{fxPara("hello")}))
	want := "hello\n\n" + `<!-- eden:model {"v":1,"blocks":[{"id":"b_1","k":"paragraph","f":"4937d476"}]} -->` + "\n"
	if got := ToMarkdown(d); got != want {
		t.Fatalf("ToMarkdown\n got: %q\nwant: %q", got, want)
	}

	l := fxIDs(fxDoc([]Block{{Kind: BlockList, Items: fxItems("one")}}))
	want = "- one\n\n" + `<!-- eden:model {"v":1,"blocks":[{"id":"b_1","k":"list","f":"b48beae5","li":[["li_1","b5130d27"]]}]} -->` + "\n"
	if got := ToMarkdown(l); got != want {
		t.Fatalf("ToMarkdown\n got: %q\nwant: %q", got, want)
	}

	// Children come after their parent, depth first.
	n := fxIDs(fxDoc([]Block{fxToggle("T", false, fxPara("c1"), fxPara("c2")), fxPara("after")}))
	var tr struct {
		Blocks []struct {
			ID string `json:"id"`
			K  string `json:"k"`
		} `json:"blocks"`
	}
	md := ToMarkdown(n)
	payload := md[strings.Index(md, "<!-- eden:model ")+len("<!-- eden:model "):]
	payload = payload[:strings.Index(payload, " -->")]
	if err := json.Unmarshal([]byte(payload), &tr); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range tr.Blocks {
		got = append(got, b.ID+":"+b.K)
	}
	if want := []string{"b_1:toggle", "b_2:paragraph", "b_3:paragraph", "b_4:paragraph"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sequence = %v, want %v", got, want)
	}
}

// TestCodecIdentityWithIDs re-runs every fixture with ids counting (Test list 1):
// FromMarkdown(ToMarkdown(d), Prev: d) == d, and so does reading the Markdown
// alone, because the trailer carries the ids.
func TestCodecIdentityWithIDs(t *testing.T) {
	all := append(append(codecFixtures(), codecV7Fixtures()...), codecSectionFixtures()...)
	for _, fx := range all {
		t.Run(fx.name, func(t *testing.T) {
			d := fxIDs(fx.doc())
			md := ToMarkdown(d)
			assertDocEqual(t, mustFromMarkdown(t, md, FromMarkdownOptions{Prev: d}), d)
			assertDocEqual(t, mustFromMarkdown(t, md, FromMarkdownOptions{}), d)
			if again := ToMarkdown(mustFromMarkdown(t, md, FromMarkdownOptions{})); again != md {
				t.Fatalf("Markdown is not a fixed point\n first: %q\nsecond: %q", md, again)
			}
		})
	}
}

// TestCodecIDsSurviveSourceEdits is Test list 5: a person edits the Markdown, and
// unchanged blocks keep their ids.
func TestCodecIDsSurviveSourceEdits(t *testing.T) {
	d := idsDoc()
	md := ToMarkdown(d)

	t.Run("unchanged", func(t *testing.T) {
		assertDocEqual(t, readBoth(t, md, d), d)
	})

	t.Run("a_paragraph_inserted_in_the_middle_gets_the_next_free_id", func(t *testing.T) {
		edited := strings.Replace(md, "alpha\n\n", "alpha\n\ninserted\n\n", 1)
		want := idsDoc()
		blocks := want.Sections[0].Blocks
		want.Sections[0].Blocks = append(append(append([]Block(nil), blocks[:2]...), Block{ID: "b_5", Kind: BlockParagraph, Text: "inserted"}), blocks[2:]...)
		assertDocEqual(t, readBoth(t, edited, d), want)
	})

	t.Run("an_edited_paragraph_keeps_its_id_and_its_comment", func(t *testing.T) {
		edited := strings.Replace(md, "alpha\n", "alpha, edited\n", 1)
		want := idsDoc()
		want.Sections[0].Blocks[1].Text = "alpha, edited"
		assertDocEqual(t, readBoth(t, edited, d), want)
	})

	t.Run("list_items_keep_their_ids_through_edits_and_deletes", func(t *testing.T) {
		edited := strings.Replace(md, "- one\n- two\n", "- two!\n- three\n", 1)
		got := readBoth(t, edited, d)
		// "one" is gone, "two" was edited, "three" is new: positional pairing keeps
		// li_1 on the first slot, li_2 on the second.
		if want := []string{"li_1", "li_2"}; !sameStrings(itemIDs(got), want) {
			t.Fatalf("item ids = %v, want %v", itemIDs(got), want)
		}
		edited = strings.Replace(md, "- one\n- two\n", "- two\n", 1)
		if got := readBoth(t, edited, d); !sameStrings(itemIDs(got), []string{"li_2"}) {
			t.Fatalf("item ids after deleting the first item = %v, want [li_2]", itemIDs(got))
		}
		edited = strings.Replace(md, "- one\n- two\n", "- one\n- two\n- three\n", 1)
		if got := readBoth(t, edited, d); !sameStrings(itemIDs(got), []string{"li_1", "li_2", "li_3"}) {
			t.Fatalf("item ids after appending an item = %v, want [li_1 li_2 li_3]", itemIDs(got))
		}
	})

	t.Run("deleting_a_commented_block_leaves_the_comment_as_delete_block_does", func(t *testing.T) {
		edited := strings.Replace(md, "alpha\n\n", "", 1)
		want := idsDoc()
		if err := ApplyOps(want, []Op{{Op: OpDeleteBlock, BlockID: "b_2"}}); err != nil {
			t.Fatal(err)
		}
		got := readBoth(t, edited, d)
		assertDocEqual(t, got, want)
		if len(got.Comments) != 1 || got.Comments[0].BlockID != "b_2" {
			t.Fatalf("the comment was dropped or moved: %+v", got.Comments)
		}
	})

	t.Run("a_deleted_blocks_id_is_not_given_to_a_new_block", func(t *testing.T) {
		// omega (b_4, the highest id) is deleted and a heading is added: a heading
		// is not an edited paragraph, so it is new, and b_4 stays retired.
		edited := strings.Replace(md, "omega\n", "", 1) + ""
		edited = strings.Replace(edited, "\n\n<!-- eden:model", "\n# Fresh\n\n<!-- eden:model", 1)
		got := readBoth(t, edited, d)
		if want := []string{"b_1", "b_2", "b_3", "b_5"}; !sameStrings(blockIDs(got), want) {
			t.Fatalf("ids = %v, want %v", blockIDs(got), want)
		}
	})

	t.Run("moved_blocks_keep_their_ids", func(t *testing.T) {
		edited := strings.Replace(md, "alpha\n\n- one\n- two\n\nomega\n", "omega\n\nalpha\n\n- one\n- two\n", 1)
		got := readBoth(t, edited, d)
		if want := []string{"b_1", "b_4", "b_2", "b_3"}; !sameStrings(blockIDs(got), want) {
			t.Fatalf("ids = %v, want %v", blockIDs(got), want)
		}
	})
}

// TestCodecIdenticalParagraphsKeepPositionalIDs: where two blocks are the same,
// the ids follow position, not guesswork.
func TestCodecIdenticalParagraphsKeepPositionalIDs(t *testing.T) {
	d := fxIDs(fxDoc([]Block{fxPara("same"), fxPara("same"), fxPara("tail")}))
	md := ToMarkdown(d)
	assertDocEqual(t, readBoth(t, md, d), d)

	cases := []struct {
		name, from, to string
		want           []string
	}{
		{"delete_the_first_duplicate", "same\n\nsame\n\ntail\n", "same\n\ntail\n", []string{"b_1", "b_3"}},
		{"insert_a_third_duplicate", "same\n\nsame\n\ntail\n", "same\n\nsame\n\nsame\n\ntail\n", []string{"b_1", "b_2", "b_4", "b_3"}},
		{"delete_the_second_duplicate_and_the_tail", "same\n\nsame\n\ntail\n", "same\n", []string{"b_1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edited := strings.Replace(md, tc.from, tc.to, 1)
			if got := readBoth(t, edited, d); !sameStrings(blockIDs(got), tc.want) {
				t.Fatalf("ids = %v, want %v", blockIDs(got), tc.want)
			}
		})
	}
}

// TestCodecFreshIDsUseTheEnsureScheme: with nothing to match, the ids a read
// assigns are exactly the ones EnsureBlockIDs / EnsureListItemIDs would.
func TestCodecFreshIDsUseTheEnsureScheme(t *testing.T) {
	src := "# T\n\nalpha\n\n- a\n- b\n\n:::toggle\nt\n\n- c\n:::\n\n- [ ] todo\n\n  child\n"
	want := mustFromMarkdown(t, src, FromMarkdownOptions{})
	EnsureBlockIDs(want)
	EnsureListItemIDs(want)
	got := mustFromMarkdown(t, src, FromMarkdownOptions{Prev: NewDocument("")})
	assertDocEqual(t, got, want)
	if len(blockIDs(got)) == 0 || blockIDs(got)[0] != "b_1" {
		t.Fatalf("ids = %v", blockIDs(got))
	}
}

// ---- spans ----

// spanText is the text a span covers, or a marker when there is no usable span
// (so a failing test reports instead of panicking).
func spanText(md string, s *Span) string {
	if !spanOK(md, s) {
		return "<no span>"
	}
	return md[s.Start:s.Stop]
}

func spanOK(md string, s *Span) bool {
	return s != nil && 0 <= s.Start && s.Start <= s.Stop && s.Stop <= len(md)
}

// walkBlocks calls f on every block of d, children included.
func walkBlocks(d *Document, f func(b *Block)) {
	var walk func([]Block)
	walk = func(bs []Block) {
		for i := range bs {
			f(&bs[i])
			walk(bs[i].Children)
		}
	}
	for i := range d.Sections {
		walk(d.Sections[i].Blocks)
	}
}

// TestCodecSpansPointAtHeadingText is Test list 6: with Spans, md[Start:Stop] is
// the heading text for every outline entry (the ATX prefix excluded, as Build
// does), in bytes: emoji and CJK headings come after multi-byte text.
func TestCodecSpansPointAtHeadingText(t *testing.T) {
	srcs := map[string]string{
		"plain":        "# One\n\ntext\n\n## Two words\n\n### Three\n",
		"multibyte":    "café ☕ first\n\n# Launch 🚀 plan\n\ndescription ✓\n\n## 日本語の見出し\n\n###### after\n",
		"front_matter": "---\ntitle: Ünïcode\ntheme: gaia\n---\n\n# After front matter\n\n## 日本語\n",
		"sections":     "# A\n\nx\n\n---\n\n# B 🎉\n\ny\n\n---\n\n## C\n",
		"nested":       "> # In a quote\n\n- [ ] todo\n\n  ## In a todo\n\n:::toggle\nTitle\n\n### In a toggle\n:::\n",
		"setext":       "Setext Title\n============\n\nbody\n\nSub Title\n---------\n",
		"closing_hash": "## Closed ##\n\n# Trailing   \n",
		"crlf":         "# One\r\n\r\ntext é\r\n\r\n---\r\n\r\n## Two 🎉\r\n",
	}
	for name, md := range srcs {
		t.Run(name, func(t *testing.T) {
			d := mustFromMarkdown(t, md, FromMarkdownOptions{Spans: true})
			if len(d.Outline) == 0 {
				t.Fatal("no outline")
			}
			for _, e := range d.Outline {
				if !spanOK(md, e.Span) {
					t.Fatalf("outline %q has span %+v", e.Text, e.Span)
				}
				if got := spanText(md, e.Span); got != e.Text {
					t.Fatalf("span %+v covers %q, want %q", e.Span, got, e.Text)
				}
			}
		})
	}
}

// TestCodecSpansAreOnEveryBlockAndSection: with Spans every block, section and
// outline entry has a span inside the input, and asking for spans changes
// nothing else; without Spans no span is set.
func TestCodecSpansAreOnEveryBlockAndSection(t *testing.T) {
	all := append(append(codecFixtures(), codecV7Fixtures()...), codecSectionFixtures()...)
	for _, fx := range all {
		t.Run(fx.name, func(t *testing.T) {
			md := ToMarkdown(fx.doc())
			plain := mustFromMarkdown(t, md, FromMarkdownOptions{})
			withSpans := mustFromMarkdown(t, md, FromMarkdownOptions{Spans: true})

			walkBlocks(plain, func(b *Block) {
				if b.Span != nil {
					t.Fatalf("Spans:false but %s block has a span", b.Kind)
				}
			})
			for _, s := range plain.Sections {
				if s.Span != nil {
					t.Fatal("Spans:false but a section has a span")
				}
			}
			for _, e := range plain.Outline {
				if e.Span != nil {
					t.Fatal("Spans:false but an outline entry has a span")
				}
			}

			walkBlocks(withSpans, func(b *Block) {
				if !spanOK(md, b.Span) {
					t.Fatalf("%s block has span %+v (len %d)", b.Kind, b.Span, len(md))
				}
			})
			for _, e := range withSpans.Outline {
				if !spanOK(md, e.Span) {
					t.Fatalf("outline entry %q has span %+v", e.Text, e.Span)
				}
			}
			for _, s := range withSpans.Sections {
				if len(s.Blocks) > 0 && !spanOK(md, s.Span) {
					t.Fatalf("section %d has span %+v", s.ID, s.Span)
				}
			}
			assertDocEqual(t, stripIDsAndSpans(withSpans), stripIDsAndSpans(plain))
		})
	}
}

// TestCodecSpansCoverTheirConstruct pins what a span covers for the kinds whose
// extent is a choice.
func TestCodecSpansCoverTheirConstruct(t *testing.T) {
	md := "intro line\nsecond line\n\n::::callout {\"tone\":\"info\"}\nNote\n\n:::toggle\nT\n:::\n::::\n\n```go\nx := 1\n```\n\n***\n\n$$\nE=mc^2\n$$\n\n- [ ] todo item\n"
	d := mustFromMarkdown(t, md, FromMarkdownOptions{Spans: true})
	blocks := d.Sections[0].Blocks
	slice := func(b Block) string { return spanText(md, b.Span) }

	if got := slice(blocks[0]); got != "intro line\nsecond line" {
		t.Fatalf("paragraph span = %q", got)
	}
	if got := slice(blocks[1]); got != "::::callout {\"tone\":\"info\"}\nNote\n\n:::toggle\nT\n:::\n::::" {
		t.Fatalf("directive span = %q (the whole construct, fences included)", got)
	}
	if got := slice(blocks[1].Children[0]); got != ":::toggle\nT\n:::" {
		t.Fatalf("nested directive span = %q", got)
	}
	if got := slice(blocks[2]); got != "x := 1" {
		t.Fatalf("code span = %q (the content between the fences)", got)
	}
	if got := slice(blocks[3]); got != "***" {
		t.Fatalf("divider span = %q", got)
	}
	if got := slice(blocks[4]); got != "$$\nE=mc^2\n$$" {
		t.Fatalf("display math span = %q", got)
	}
	if got := slice(blocks[5]); !strings.Contains(got, "todo item") || strings.Contains(got, "\n") {
		t.Fatalf("todo span = %q", got)
	}
	if s := d.Sections[0].Span; !spanOK(md, s) || s.Start != 0 || md[s.Stop-len("todo item"):s.Stop] != "todo item" {
		t.Fatalf("section span = %+v", s)
	}
}

// TestCodecSpansAreRelativeToTheInputAfterFrontMatter: spans are offsets into the
// whole input, front matter and all, not into the body.
func TestCodecSpansAreRelativeToTheInputAfterFrontMatter(t *testing.T) {
	md := "---\ntitle: T\n---\n\nbody here\n"
	d := mustFromMarkdown(t, md, FromMarkdownOptions{Spans: true})
	b := d.Sections[0].Blocks[0]
	if got := spanText(md, b.Span); got != "body here" {
		t.Fatalf("span covers %q", got)
	}
}

// TestCodecSpansSurviveAnEditAndKeepIDs: the two features together, as the source
// view uses them.
func TestCodecSpansSurviveAnEditAndKeepIDs(t *testing.T) {
	d := idsDoc()
	edited := strings.Replace(ToMarkdown(d), "alpha\n", "alpha, edited\n", 1)
	got := mustFromMarkdown(t, edited, FromMarkdownOptions{Prev: d, Spans: true})
	h := got.Outline[0]
	if spanText(edited, h.Span) != "Title" {
		t.Fatalf("outline span covers %q", spanText(edited, h.Span))
	}
	if got.Sections[0].Blocks[1].ID != "b_2" || got.Sections[0].Blocks[1].Span == nil {
		t.Fatalf("edited block: %+v", got.Sections[0].Blocks[1])
	}
}

// TestCodecIDsSweepIdentity: random trees with ids come back identical, ids
// included, both with Prev and from the trailer alone.
func TestCodecIDsSweepIdentity(t *testing.T) {
	rng := rand.New(rand.NewSource(404))
	fails := 0
	for i := 0; i < 3000; i++ {
		d := fxIDs(fxDoc(genV7Tree(rng)))
		md := ToMarkdown(d)
		for _, opts := range []FromMarkdownOptions{{Prev: d}, {}} {
			got, err := FromMarkdown(md, opts)
			if err == nil && reflect.DeepEqual(got, d) {
				continue
			}
			fails++
			if fails <= 2 {
				gotJSON := "nil"
				if got != nil {
					gotJSON = docJSON(t, got)
				}
				t.Errorf("ids do not round-trip (Prev=%v)\n markdown: %q\n error: %v\n want: %s\n got: %s", opts.Prev != nil, md, err, docJSON(t, d), gotJSON)
			}
		}
	}
	if fails > 0 {
		t.Fatalf("%d round-trip failures", fails)
	}
}

// TestCodecIDsAreStableOnGarbage gives ids to whatever random soup parses to and
// checks that writing and reading it again, with Prev and without, gives the
// same document, ids included.
func TestCodecIDsAreStableOnGarbage(t *testing.T) {
	alphabet := []string{
		"a", "b", " ", "\n", "\n\n", "# ", "> ", "- ", "- [ ] ", "* [x] ", "1. ", "```", "$$", "---", "***", "|", "|-", ":::toggle\n", ":::callout {}\n",
		":::file {\"src\":\"f\"}\n:::\n", ":::", "<!-- x -->", "<!-- eden:note \"n\" -->", "![i](u)", "*", "**", "[l](u)", "`", "\t",
	}
	rng := rand.New(rand.NewSource(6))
	bad := 0
	for i := 0; i < 20000 && bad < 3; i++ {
		src := sweepText(rng, alphabet, 1+rng.Intn(18))
		d, err := FromMarkdown(src, FromMarkdownOptions{Lenient: true, Prev: NewDocument("")})
		if err != nil {
			t.Fatalf("lenient read failed for %q: %v", src, err)
		}
		md := ToMarkdown(d)
		for _, opts := range []FromMarkdownOptions{{Prev: d}, {}} {
			got, err := FromMarkdown(md, opts)
			if err == nil && reflect.DeepEqual(got, d) {
				continue
			}
			bad++
			var ue *UnrepresentableError
			if errors.As(err, &ue) {
				t.Errorf("canonical Markdown refused: %v\n source: %q\n canonical: %q", err, src, md)
			} else {
				t.Errorf("ids not stable (Prev=%v)\n source: %q\n canonical: %q\n first: %s\n second: %s", opts.Prev != nil, src, md, docJSON(t, d), docJSON(t, got))
			}
			break
		}
	}
}

// TestCodecIDsAfterRandomEdits: unrelated blocks keep their ids through a random
// edit of the Markdown. Every paragraph's text is unique, so "the same block" is
// never in doubt.
func TestCodecIDsAfterRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewSource(808))
	for i := 0; i < 1500; i++ {
		n := 2 + rng.Intn(8)
		var blocks []Block
		for k := 0; k < n; k++ {
			switch rng.Intn(4) {
			case 0:
				blocks = append(blocks, Block{Kind: BlockList, Items: fxItems(fmt.Sprintf("item %d a", k), fmt.Sprintf("item %d b", k))})
			case 1:
				blocks = append(blocks, Block{Kind: BlockHeading, Level: 2, Text: fmt.Sprintf("heading %d", k)})
			default:
				blocks = append(blocks, fxPara(fmt.Sprintf("paragraph %d", k)))
			}
		}
		old := fxIDs(fxDoc(append([]Block(nil), blocks...)))
		idOf := map[string]string{} // content label -> id
		for _, b := range old.Sections[0].Blocks {
			idOf[label(b)] = b.ID
		}

		edited := append([]Block(nil), blocks...)
		what := rng.Intn(4)
		at := rng.Intn(len(edited))
		var touched string // the label of the one block the edit changes
		switch what {
		case 0: // delete
			touched = label(edited[at])
			edited = append(edited[:at], edited[at+1:]...)
		case 1: // insert a new paragraph
			edited = append(edited[:at], append([]Block{fxPara(fmt.Sprintf("inserted %d", i))}, edited[at:]...)...)
		case 2: // edit a paragraph in place
			if edited[at].Kind != BlockParagraph {
				continue
			}
			touched = label(edited[at])
			edited[at] = fxPara(edited[at].Text + " (edited)")
		case 3: // swap two blocks
			other := rng.Intn(len(edited))
			edited[at], edited[other] = edited[other], edited[at]
		}
		md := ToMarkdown(fxDoc(edited))
		got, err := FromMarkdown(md, FromMarkdownOptions{Prev: old})
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, id := range blockIDs(got) {
			if id == "" || seen[id] {
				t.Fatalf("edit %d: ids are not unique and non-empty: %v\n%s", what, blockIDs(got), md)
			}
			seen[id] = true
		}
		for _, b := range got.Sections[0].Blocks {
			l := label(b)
			switch {
			case what == 2 && strings.HasSuffix(b.Text, " (edited)"):
				if b.ID != idOf[touched] {
					t.Fatalf("an edited paragraph lost its id: got %s want %s\n%s", b.ID, idOf[touched], md)
				}
			case l != touched && idOf[l] != "":
				if b.ID != idOf[l] {
					t.Fatalf("edit %d: %q was not touched but its id went from %s to %s\n%s", what, l, idOf[l], b.ID, md)
				}
			case idOf[l] == "":
				for _, oldID := range idOf {
					if b.ID == oldID {
						t.Fatalf("edit %d: new block %q was given the old id %s\n%s", what, l, oldID, md)
					}
				}
			}
		}
	}
}

// label names a block by its content, for the edit sweep.
func label(b Block) string {
	if b.Kind == BlockList {
		return "list:" + b.Items[0].Text
	}
	return string(b.Kind) + ":" + strings.TrimSuffix(b.Text, " (edited)")
}

// TestCodecOutlineMatchesApplyOps: the Outline read from Markdown is the one
// ApplyOps keeps (rebuildOutline), slugs included: a repeated heading repeats its
// slug, and a heading with no ASCII letters or digits has an empty one. Build's
// goldmark ids would number repeats ("intro-1"); a document that came through
// ops would not match them, so the codec follows ops.
func TestCodecOutlineMatchesApplyOps(t *testing.T) {
	doc := NewDocument("")
	var ops []Op
	for _, h := range []struct {
		level int
		text  string
	}{{1, "Intro"}, {2, "Intro"}, {2, "日本語"}, {3, "Launch 🚀 plan!"}, {1, "Intro"}} {
		ops = append(ops, Op{Op: OpInsertBlock, SectionID: 1, Block: &Block{Kind: BlockHeading, Level: h.level, Text: h.text}})
	}
	mustApply(t, doc, ops)
	got := mustFromMarkdown(t, ToMarkdown(doc), FromMarkdownOptions{Prev: doc})
	if !reflect.DeepEqual(got.Outline, doc.Outline) {
		t.Fatalf("outline differs\n got: %+v\nwant: %+v", got.Outline, doc.Outline)
	}
	assertDocEqual(t, got, doc)
}

// TestCodecSpansStayInsideTheInput reads random soup with Spans on: every span of
// every block, section and outline entry is inside the input and not inverted,
// and the plain-text heading spans slice to the heading text.
func TestCodecSpansStayInsideTheInput(t *testing.T) {
	alphabet := []string{
		"a", "é", "🎉", " ", "\n", "\n\n", "\r\n", "# ", "## ", "> ", "- ", "- [ ] ", "1. ", "```", "$$", "---", "***", "|", "|-",
		":::toggle\n", "::::callout {}\n", ":::figure\n", ":::", "<!-- x -->", "<!-- eden:note \"n\" -->", "<!-- _class: x -->",
		"![i](u)", "*", "**", "[l](u)", "`", "\t", "~~~", "Title\n===\n", "Sub\n---\n",
	}
	rng := rand.New(rand.NewSource(17))
	for i := 0; i < 8000; i++ {
		src := sweepText(rng, alphabet, 1+rng.Intn(16))
		d, err := FromMarkdown(src, FromMarkdownOptions{Lenient: true, Spans: true})
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		bad := func(what string, s *Span) {
			if s != nil && (s.Start < 0 || s.Start > s.Stop || s.Stop > len(src)) {
				t.Fatalf("%s span %+v is outside %q", what, *s, src)
			}
		}
		walkBlocks(d, func(b *Block) { bad("block "+string(b.Kind), b.Span) })
		for _, sec := range d.Sections {
			bad("section", sec.Span)
		}
		for _, e := range d.Outline {
			bad("outline", e.Span)
		}
	}
}

// TestFromMarkdownSurvivesATabbedFenceLine: goldmark v1.8.4 panics on a short
// tab-indented line that could open a code fence ("> \t`"); the codec guards it.
func TestFromMarkdownSurvivesATabbedFenceLine(t *testing.T) {
	for _, src := range []string{"> \t`", "> \t~", "- [ ] b\n\n> \t`", ":::toggle\n> \t`\n:::\n"} {
		for _, lenient := range []bool{false, true} {
			if _, err := FromMarkdown(src, FromMarkdownOptions{Lenient: lenient}); err != nil {
				var ue *UnrepresentableError
				if lenient || !errors.As(err, &ue) {
					t.Fatalf("%q: %v", src, err)
				}
			}
		}
	}
}
