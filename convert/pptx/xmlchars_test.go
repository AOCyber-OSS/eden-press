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

package pptx

// Issue #39: pptx escaped through xml.EscapeText, which turns XML-illegal
// characters into U+FFFD -- a valid part, but a visible replacement glyph
// where xlsx and docx now drop the character. One convention for all three.

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/internal/xmltext/xmltexttest"
)

func TestXMLIllegal_P1_DroppedNotReplaced(t *testing.T) {
	doc := &model.Document{Sections: []model.Section{{Blocks: []model.Block{
		{Kind: model.BlockHeading, Level: 1, Text: "Q3" + xmltexttest.Illegal + " review"},
		{Kind: model.BlockParagraph, Text: "red\x1b[0m & ok"},
	}}}}
	pkg, err := ToPPTX(doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	parts := xmltexttest.AssertStrictParts(t, pkg)
	for name, body := range parts {
		if strings.ContainsRune(body, '\uFFFD') {
			t.Errorf("%s: illegal character became U+FFFD instead of being dropped", name)
		}
	}
	slide := parts["ppt/slides/slide1.xml"]
	for _, want := range []string{"Q3 review", "red[0m &amp; ok"} {
		if !strings.Contains(slide, want) {
			t.Errorf("slide1.xml lost %q:\n%s", want, slide)
		}
	}
}
