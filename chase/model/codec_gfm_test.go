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
