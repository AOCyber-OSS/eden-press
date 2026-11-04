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
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"
)

// FuzzFromMarkdown (objective 1026, TRD 03) feeds FromMarkdown arbitrary bytes.
// It must never panic, in either mode and with every option; strict mode may only
// fail with *UnrepresentableError; and whatever a read produces must be a fixed
// point of the codec: writing it and reading that again gives the same Document
// and the same Markdown. Spans must lie inside the input.
//
// Seeds are the Markdown of every hand-built fixture (the exact bytes ToMarkdown
// emits for each kind) and every file of the conformance corpus, so the fuzzer
// starts from real shapes of all 16 kinds. A crasher is kept by Go under
// testdata/fuzz/FuzzFromMarkdown and replays on every plain `go test`.
//
//	go test -fuzz=FuzzFromMarkdown -fuzztime=60s ./chase/model/
func FuzzFromMarkdown(f *testing.F) {
	for _, set := range [][]codecFixture{codecFixtures(), codecV7Fixtures(), codecSectionFixtures()} {
		for _, fx := range set {
			f.Add(fx.md)
			f.Add(ToMarkdown(fxIDs(fx.doc())))
		}
	}
	for _, pattern := range []string{
		filepath.Join("..", "..", "conformance", "corpus", "cases", "*", "input.md"),
		filepath.Join("testdata", "*.md"),
	} {
		paths, _ := filepath.Glob(pattern)
		for _, p := range paths {
			if raw, err := os.ReadFile(p); err == nil {
				f.Add(string(raw))
			}
		}
	}
	// Shapes that have bitten before: a tab-indented fence line, a closing $$ with
	// text after it, unclosed directives, an eden:model comment with a bad payload.
	for _, s := range []string{
		"> \t`", "$$\nx\n$$ tail\n", ":::toggle\nunclosed", ":::callout {\"icon\":}\n:::\n",
		"<!-- eden:model {\"v\":1,\"blocks\":[{\"id\":\"b_1\",\"k\":\"paragraph\",\"f\":\"00000000\"}]} -->\n",
		"- [ ] a\n- b\n", "1. a\n   3. b\n", "| a |\n| - |\n| **b** |\n", "x\r\ny\r\n",
		// Formatted table cells (TRD 26): every kind of styled cell, pipes in code,
		// math and link destinations, tags, and the refusals a cell shares with a paragraph.
		"| **a** | `b\\|c` |\n| :-: | --: |\n| [l](u) | $x\\|y$ |\n",
		"| *i* | ~~s~~ | <https://a.b> |\n| --- | --- | --- |\n| <strong>d</strong> | `` ` `` | [l](<u v\\|w>) |\n",
		"| a |\n| - |\n| x<br>y |\n", "| a |\n| - |\n| ![i](u) |\n", "| a |\n| - |\n| [l](u \"t\") |\n",
		"| `a`**b**~~c~~ |\n| - |\n| $x\\\\|y$ |\n", "| a\\ |\n| - |\n| `\\|` |\n",
		// Wide table rows (TRD 31): excess cells, styled and with pipes, a row wider than
		// its neighbours, a table in a quote, no outer pipes, and the row-end shapes
		// (an escaped outer pipe, a trailing empty cell) where the splitter and goldmark
		// have to agree.
		"| a |\n| - |\n| x | zqsentinel |\n", "| a | b |\n| :-- | --: |\n| 1 | 2 | 3 |\n| 4 |\n| 5 | 6 | 7 | 8 |\n",
		"| a |\n| - |\n| x | **b** | `c\\|d` | $e\\|f$ | [g\\|h](u\\|v) |\n", "> | a |\n> | - |\n> | x | y |\n", "a | b\n--- | ---\n1 | 2 | 3\n",
		"| a |\n| - |\n| x | |\n", "| a |\n| - |\n| x | y \\|\n", "| a |\n| - |\n| x | `y |\n", "| a |\n| - |\n| x | <strong>y |\n",
		"| a |\n| - |\n| x | ![i](u) |\n", "intro\n| a |\n| - |\n| x | y |\n\nafter\n", ":::toggle\n| a |\n| - |\n| x | y |\n:::\n",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, src string) {
		for _, lenient := range []bool{false, true} {
			d, err := FromMarkdown(src, FromMarkdownOptions{Lenient: lenient})
			if err != nil {
				var ue *UnrepresentableError
				if lenient || !errors.As(err, &ue) {
					t.Fatalf("lenient=%v: unexpected error %T: %v", lenient, err, err)
				}
				continue
			}

			// Options must not panic and spans must lie inside the input.
			withSpans, err := FromMarkdown(src, FromMarkdownOptions{Lenient: lenient, Spans: true, Prev: d})
			if err != nil {
				t.Fatalf("lenient=%v: read with Spans and Prev failed: %v", lenient, err)
			}
			checkSpansInside(t, withSpans, len(src))

			if !utf8.ValidString(src) {
				continue // a Document is JSON, which cannot carry invalid UTF-8
			}

			if todoEmptyTailHazard(d) {
				continue // a known limit of goldmark, see todoEmptyTailHazard
			}

			md := ToMarkdown(d)
			d2, err := FromMarkdown(md, FromMarkdownOptions{})
			if err != nil {
				t.Fatalf("lenient=%v: canonical Markdown is refused: %v\n source: %q\n canonical: %q", lenient, err, src, md)
			}
			if !reflect.DeepEqual(d, d2) {
				t.Fatalf("lenient=%v: read is not a fixed point at %s\n source: %q\n canonical: %q", lenient, deepDiff(d2, d), src, md)
			}
			if again := ToMarkdown(d2); again != md {
				t.Fatalf("lenient=%v: Markdown is not a fixed point\n source: %q\n first: %q\nsecond: %q", lenient, src, md, again)
			}
			if hasIDs(d) {
				if d3, err := FromMarkdown(md, FromMarkdownOptions{Prev: d}); err != nil || !reflect.DeepEqual(d, d3) {
					t.Fatalf("lenient=%v: Prev does not give the ids back (err %v)\n source: %q\n canonical: %q", lenient, err, src, md)
				}
			}
		}
	})
}

// checkSpansInside fails when a span of d does not lie inside an input of n bytes
// or runs backwards.
func checkSpansInside(t *testing.T, d *Document, n int) {
	t.Helper()
	check := func(what string, s *Span) {
		if s != nil && (s.Start < 0 || s.Start > s.Stop || s.Stop > n) {
			t.Fatalf("%s span %+v is outside the input (%d bytes)", what, *s, n)
		}
	}
	walkAll(d, func(b *Block, level int) { check(string(b.Kind)+" block", b.Span) })
	for _, s := range d.Sections {
		check("section", s.Span)
	}
	for _, o := range d.Outline {
		check("outline entry", o.Span)
	}
}

// todoEmptyTailHazard reports whether d holds a todo whose child list ends in an
// empty item and is followed by another child. goldmark closes a todo at the first
// block after such a list (no spelling of the Markdown avoids it: a trailing
// space, a different indent and a tight layout were all tried), so the following
// children read back as siblings of the todo. This is the one canonical-form
// limit TRD 02 recorded for todos; it cannot come from a strict read, only from
// the lenient flattening of a construct the model cannot carry (an empty heading
// inside a list item, for one) or from a Document built by hand.
func todoEmptyTailHazard(d *Document) bool {
	found := false
	walkAll(d, func(b *Block, level int) {
		if b.Kind != BlockTodo {
			return
		}
		for i, c := range b.Children {
			if c.Kind == BlockList && len(c.Items) > 0 && i+1 < len(b.Children) {
				if last := c.Items[len(c.Items)-1]; last.Text == "" {
					found = true
				}
			}
		}
	})
	return found
}
