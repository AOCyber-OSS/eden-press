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

// SafeHref is the one link-scheme allowlist every HTML writer of a document
// uses (eden-press#23, #24): http, https, mailto, a same-document #fragment, or
// a schemeless relative reference. Anything else -- javascript:, data:,
// vbscript:, file:, blob: -- is dropped, however a browser would be persuaded
// to read the scheme.
func TestSafeHref(t *testing.T) {
	keep := []string{
		"https://example.com/a?b=c#d",
		"http://example.com",
		"HTTPS://EXAMPLE.COM",
		"mailto:someone@example.com",
		"#section-2",
		"files/report.pdf",
		"../up/one.png",
		"/abs/path",
		"//example.com/protocol-relative",
		"a.png?x=1:2",
	}
	for _, href := range keep {
		if got, ok := SafeHref(href); !ok || got != strings.TrimSpace(href) {
			t.Errorf("SafeHref(%q) = %q, %v; want it kept", href, got, ok)
		}
	}

	drop := []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		" javascript:alert(1)",
		"\x01javascript:alert(1)",
		"java\tscript:alert(1)",
		"java\nscript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"DATA:text/html;base64,PHNjcmlwdD4=",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"blob:https://example.com/uuid",
		"ftp://example.com/x",
		"",
	}
	for _, href := range drop {
		if got, ok := SafeHref(href); ok {
			t.Errorf("SafeHref(%q) = %q, true; want it dropped", href, got)
		}
	}
}

// ToHTML writes no href SafeHref refuses: a run link, a table-cell link and a
// file block's Src. The visible text stays; only the link goes.
func TestToHTMLDropsUnsafeHrefs(t *testing.T) {
	doc := fxDoc([]Block{
		fxRich(fxRun("see "), fxLink("here", "javascript:alert(1)")),
		{Kind: BlockTable, Headers: []string{"h"}, Rows: [][]string{{"c"}},
			RowRuns: [][][]TextRun{{{fxLink("cell", "data:text/html,<b>x</b>")}}}},
		{Kind: BlockFile, Name: "evil.html", Src: "data:text/html;base64,PHNjcmlwdD4="},
		fxRich(fxLink("ok", "https://example.com/")),
		{Kind: BlockFile, Name: "report.pdf", Src: "files/report.pdf"},
	})

	got := ToHTML(doc)
	for _, bad := range []string{"javascript:", "data:"} {
		if strings.Contains(got, bad) {
			t.Errorf("ToHTML kept a %s href:\n%s", bad, got)
		}
	}
	for _, want := range []string{"see here", "<td>cell</td>", "evil.html", `href="https://example.com/"`, `href="files/report.pdf"`} {
		if !strings.Contains(got, want) {
			t.Errorf("ToHTML lost %q:\n%s", want, got)
		}
	}
}
