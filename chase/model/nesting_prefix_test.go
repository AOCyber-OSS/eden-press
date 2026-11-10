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
	"strings"
	"testing"
	"time"
)

// goldmark's own parse is quadratic in the container markers on one line: it
// re-reads the line's offset for every block it opens, so '- 'x19600 took 10 s
// and '>'x19600 1.7 s before any of this package's code ran. AODex accepts
// 400 KB of Markdown. The limit must therefore hold before the parse too
// (eden-press#29).
//
// Test list:
//
//	N12 lenient one-line "- - - ... x" (4900 markers) is fast and keeps "x"
//	N13 strict 40000 '>' markers are refused fast, at the 33rd marker
//	N14 400 KB of '>' on one line, lenient, under one second
//	N15 a top-level fenced code line of 40 '>' is code, kept byte for byte
//	N16 a line of 40 spaced dashes is still a divider (a thematic break)
//	N17 with Spans, a block after a trimmed line still slices the original input

// bounded runs f and fails when it takes longer than limit (skipped in -short).
func bounded(t *testing.T, limit time.Duration, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if d := time.Since(start); !testing.Short() && d > limit {
		t.Fatalf("took %v, want under %v", d, limit)
	}
}

func TestNesting_N12_OneLineDeepListIsCheap(t *testing.T) {
	md := strings.Repeat("- ", 4900) + "x"
	var doc *Document
	bounded(t, 200*time.Millisecond, func() {
		var err error
		if doc, err = FromMarkdown(md, FromMarkdownOptions{Lenient: true}); err != nil {
			t.Fatal(err)
		}
	})
	if got := maxDepth(doc); got > MaxNestingDepth {
		t.Fatalf("depth = %d", got)
	}
	if !strings.Contains(ToMarkdown(doc), "x") {
		t.Fatal(`"x" was lost`)
	}
}

func TestNesting_N13_StrictRefusesAtTheFirstMarkerPastTheLimit(t *testing.T) {
	md := strings.Repeat(">", 40000) + " x"
	var err error
	bounded(t, 200*time.Millisecond, func() { _, err = FromMarkdown(md, FromMarkdownOptions{}) })
	var ue *UnrepresentableError
	if !errors.As(err, &ue) || ue.Construct != ConstructNestingDepth {
		t.Fatalf("err = %v, want %q", err, ConstructNestingDepth)
	}
	if ue.Offset != MaxNestingDepth {
		t.Fatalf("offset = %d, want %d (marker %d)", ue.Offset, MaxNestingDepth, MaxNestingDepth+1)
	}
}

func TestNesting_N14_FourHundredKBOfQuoteMarkers(t *testing.T) {
	md := strings.Repeat(">", 400<<10) + " x"
	bounded(t, time.Second, func() {
		if _, err := FromMarkdown(md, FromMarkdownOptions{Lenient: true}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestNesting_N15_FencedCodeIsNotTrimmed(t *testing.T) {
	line := strings.Repeat(">", 40)
	md := "```\n" + line + "\n```\n"
	for _, lenient := range []bool{false, true} {
		doc, err := FromMarkdown(md, FromMarkdownOptions{Lenient: lenient})
		if err != nil {
			t.Fatalf("lenient=%v: %v", lenient, err)
		}
		if b := doc.Sections[0].Blocks; len(b) != 1 || b[0].Kind != BlockCode || b[0].Text != line {
			t.Fatalf("lenient=%v: blocks = %+v, want one code block %q", lenient, b, line)
		}
	}
}

func TestNesting_N16_SpacedDashLineIsADivider(t *testing.T) {
	doc, err := FromMarkdown("a\n\n"+strings.Repeat("* ", 40)+"\n\nb", FromMarkdownOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if b := doc.Sections[0].Blocks; len(b) != 3 || b[1].Kind != BlockDivider {
		t.Fatalf("blocks = %+v, want paragraph, divider, paragraph", b)
	}
}

func TestNesting_N17_SpansAfterATrimmedLineSliceTheInput(t *testing.T) {
	md := "\r\n" + quoteMarkdown(40) + "\r\n\r\nafter\r\n"
	doc, err := FromMarkdown(md, FromMarkdownOptions{Lenient: true, Spans: true})
	if err != nil {
		t.Fatal(err)
	}
	bs := doc.Sections[0].Blocks
	last := bs[len(bs)-1]
	if last.Span == nil || md[last.Span.Start:last.Span.Stop] != "after" {
		t.Fatalf("last block %+v: span does not slice %q out of the input", last, "after")
	}
}

// Review findings on #37 (/code-review): the lexical pre-parse scan disagreed
// with goldmark, so it could be bypassed or could corrupt content.
//
//	N18 a fence the scan thought was open (inside a list item, inside HTML)
//	    must not let a deep line through unbounded
//	N19 fenced code inside a quote keeps a 40-'>' code line byte for byte
//	N20 a thematic break inside a quote is a divider, not 40 list markers

func TestNesting_N18_NoFenceDesyncBypass(t *testing.T) {
	deep := strings.Repeat("- ", 12000) + "x\n"
	for _, prefix := range []string{"- a\n  ```\nb\n\n", "<div>\n```\n</div>\n\n"} {
		bounded(t, 200*time.Millisecond, func() {
			if _, err := FromMarkdown(prefix+deep, FromMarkdownOptions{Lenient: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNesting_N19_CodeInsideAQuoteIsKept(t *testing.T) {
	line := strings.Repeat(">", 40) + " code"
	md := "> ```\n> " + line + "\n> ```\n"
	for _, lenient := range []bool{false, true} {
		doc, err := FromMarkdown(md, FromMarkdownOptions{Lenient: lenient})
		if err != nil {
			t.Fatalf("lenient=%v: %v", lenient, err)
		}
		q := doc.Sections[0].Blocks[0]
		if len(q.Children) != 1 || q.Children[0].Kind != BlockCode || q.Children[0].Text != line {
			t.Fatalf("lenient=%v: quote = %+v, want one code child %q", lenient, q, line)
		}
	}
}

func TestNesting_N20_ThematicBreakInsideAQuote(t *testing.T) {
	doc, err := FromMarkdown("> a\n>\n> "+strings.Repeat("- ", 40)+"\n", FromMarkdownOptions{})
	if err != nil {
		t.Fatal(err)
	}
	q := doc.Sections[0].Blocks[0]
	if len(q.Children) != 1 || q.Children[0].Kind != BlockDivider {
		t.Fatalf("quote = %+v, want text a and one divider child", q)
	}
}
