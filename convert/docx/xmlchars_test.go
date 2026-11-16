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

package docx

// Issue #39 (Refs aodex#1239): no XML-illegal character may reach any part.
//
// TEST LIST
//
//	D1 paragraph and heading runs with C0 controls, U+FFFE/U+FFFF and
//	   invalid UTF-8 yield strict-valid parts; the legal text survives
//	D2 table header and body cells
//	D3 a code block keeps tab, and LF still becomes <w:br/>; CR is kept
//	D4 the dc:title core property (Options.Title)
//	D5 link/caption text: file, image and figure placeholders (this writer
//	   renders link targets as caption text, not w:hyperlink)
//	D6 a paragraph made only of illegal characters
//	D7 list items, quotes and math
//
// Not applicable: this writer emits no headers/footers, real hyperlinks, or
// user text in docProps/app.xml.

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/internal/xmltext/xmltexttest"
)

const illegalXML = xmltexttest.Illegal

func docOf(blocks ...model.Block) *model.Document {
	return &model.Document{Sections: []model.Section{{Blocks: blocks}}}
}

func TestXMLIllegal_D1_ParagraphAndHeadingRuns(t *testing.T) {
	doc := docOf(
		model.Block{Kind: model.BlockHeading, Level: 1, Text: "Q3" + illegalXML + " review"},
		model.Block{Kind: model.BlockParagraph, Text: "colour \x1b[31mred\x1b[0m & NUL\x00 ok"},
	)
	pkg, err := ToDOCX(doc, Options{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	parts := xmltexttest.AssertStrictParts(t, pkg)
	body := parts["word/document.xml"]
	for _, want := range []string{"Q3 review", "colour [31mred[0m &amp; NUL ok"} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml lost %q:\n%s", want, body)
		}
	}
}

func mustDOCX(t *testing.T, doc *model.Document, title string) map[string]string {
	t.Helper()
	pkg, err := ToDOCX(doc, Options{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return xmltexttest.AssertStrictParts(t, pkg)
}

func TestXMLIllegal_D2_TableCells(t *testing.T) {
	parts := mustDOCX(t, docOf(model.Block{
		Kind:    model.BlockTable,
		Headers: []string{"Ven" + illegalXML + "dor", "Price"},
		Rows:    [][]string{{"North" + illegalXML + "gate", "1\x0c200"}},
	}), "t")
	body := parts["word/document.xml"]
	for _, want := range []string{">Vendor<", ">Northgate<", ">1200<"} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml lost %q:\n%s", want, body)
		}
	}
}

func TestXMLIllegal_D3_CodeKeepsTabLFCR(t *testing.T) {
	parts := mustDOCX(t, docOf(model.Block{
		Kind: model.BlockCode,
		Text: "func f() {\n\treturn\x1b[0m 1\r\n}" + illegalXML,
	}), "t")
	body := parts["word/document.xml"]
	for _, want := range []string{"\treturn[0m 1\r</w:t>", "<w:br/>", ">}</w:t>"} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml lost %q:\n%q", want, body)
		}
	}
}

func TestXMLIllegal_D4_CoreTitle(t *testing.T) {
	parts := mustDOCX(t, docOf(model.Block{Kind: model.BlockParagraph, Text: "x"}), "Q3"+illegalXML+" & review")
	if !strings.Contains(parts["docProps/core.xml"], "<dc:title>Q3 &amp; review</dc:title>") {
		t.Errorf("core.xml title:\n%s", parts["docProps/core.xml"])
	}
}

func TestXMLIllegal_D5_LinkAndCaptionText(t *testing.T) {
	parts := mustDOCX(t, docOf(
		model.Block{Kind: model.BlockFile, Name: "spec" + illegalXML + ".pdf", Src: "https://x.test/a?b=1\x00&c=2"},
		model.Block{Kind: model.BlockImage, Text: "chart" + illegalXML, Src: "img\x0b.png"},
		model.Block{Kind: model.BlockFigure, Text: "fig" + illegalXML, Src: "f.svg"},
	), "t")
	body := parts["word/document.xml"]
	for _, want := range []string{"[spec.pdf](https://x.test/a?b=1&amp;c=2)", "[chart: img.png]", "[fig: f.svg]"} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml lost %q:\n%s", want, body)
		}
	}
}

func TestXMLIllegal_D6_ParagraphOnlyIllegal(t *testing.T) {
	mustDOCX(t, docOf(
		model.Block{Kind: model.BlockHeading, Level: 1, Text: illegalXML},
		model.Block{Kind: model.BlockParagraph, Text: illegalXML},
	), illegalXML)
}

func TestXMLIllegal_D7_ListQuoteMath(t *testing.T) {
	parts := mustDOCX(t, docOf(
		model.Block{Kind: model.BlockList, Items: []model.ListItem{{Text: "one" + illegalXML}, {Text: "two\x0b", Level: 1}}},
		model.Block{Kind: model.BlockQuote, Text: "said" + illegalXML + "\nthen"},
		model.Block{Kind: model.BlockMath, Text: `\frac{a}{b}` + illegalXML},
	), "t")
	body := parts["word/document.xml"]
	for _, want := range []string{">one<", ">two<", ">said<", ">then<", `>\frac{a}{b}<`} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml lost %q:\n%s", want, body)
		}
	}
}
