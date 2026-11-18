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

package press

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/press/sanitize"
)

// eden-press#23: Policy() allows data: on every URL attribute (for the math
// PNG fallback), so a data: link written into a document kept its href in
// every link RenderModel emits. Links now carry only http, https, mailto or a
// #fragment; data: stays an image-only scheme.
func TestRenderModelDropsDataLinks(t *testing.T) {
	const evil = "data:text/html,<script>alert(1)</script>"
	link := []model.TextRun{{Text: "open", Link: evil}, {Text: " mail", Link: "mailto:a@example.com"}}
	doc := docOf(nil, []model.Block{
		{Kind: model.BlockParagraph, Runs: link},
		{Kind: model.BlockCallout, Runs: link},
		{Kind: model.BlockTable, Headers: []string{"h"}, Rows: [][]string{{"c"}},
			RowRuns: [][][]model.TextRun{{link}}},
		{Kind: model.BlockFile, Name: "f", Src: evil},
	})
	h := mustRenderModel(t, doc, Options{Profile: "paged"}).HTML

	if strings.Contains(h, `href="data:`) {
		t.Errorf("RenderModel kept a data: href:\n%s", h)
	}
	if n := strings.Count(h, "open"); n < 3 {
		t.Errorf("link text dropped with its href: %d of 3 \"open\" left:\n%s", n, h)
	}
}

// ModelPolicy gates a[href] to the link schemes while img[src] keeps data:,
// which the base64 PNG math fallback needs.
func TestModelPolicyLinkSchemes(t *testing.T) {
	cases := []struct {
		in, want string
		keep     bool
	}{
		{`<a href="data:text/html,x">t</a>`, `href=`, false},
		{`<a href="javascript:alert(1)">t</a>`, `href=`, false},
		{`<a href="https://example.com/">t</a>`, `href="https://example.com/"`, true},
		{`<a href="mailto:a@example.com">t</a>`, `href="mailto:a@example.com"`, true},
		{`<a href="#s2">t</a>`, `href="#s2"`, true},
		{`<img src="data:image/png;base64,AA==" alt="m">`, `src="data:image/png;base64,AA=="`, true},
	}
	for _, tc := range cases {
		got := sanitize.SanitizeModel(tc.in)
		if strings.Contains(got, tc.want) != tc.keep {
			t.Errorf("SanitizeModel(%q) = %q; want %q present=%v", tc.in, got, tc.want, tc.keep)
		}
	}
}
