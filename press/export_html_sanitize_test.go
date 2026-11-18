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
	"encoding/json"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
)

func exportHTML(t *testing.T, blocks ...model.Block) string {
	t.Helper()
	raw, err := json.Marshal(docOf(nil, blocks))
	if err != nil {
		t.Fatal(err)
	}
	out, err := ExportHTMLJSON(raw)
	if err != nil {
		t.Fatalf("ExportHTMLJSON: %v", err)
	}
	return string(out)
}

// eden-press#24: ExportHTMLJSON wrote a figure's SVG verbatim, so a document
// served as its HTML export could run script. Everything that can execute,
// navigate or fetch is gone from the export.
func TestExportHTMLJSONSanitizesFigureSVG(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" onload="alert(1)">` +
		`<script>alert(2)</script>` +
		`<foreignObject width="10" height="10"><iframe src="https://evil.test/"></iframe></foreignObject>` +
		`<a href="javascript:alert(3)"><circle r="2"/></a>` +
		`<image href="http://169.254.169.254/latest/meta-data/"/>` +
		`<use href="https://evil.test/sprite.svg#x"/>` +
		`<animate attributeName="href" to="javascript:alert(4)"/>` +
		`<rect width="5" height="5" fill="url(https://evil.test/p)" onclick="alert(5)"/>` +
		`</svg>`
	h := exportHTML(t, model.Block{Kind: model.BlockFigure, Text: "fig", SVG: svg})

	for _, banned := range []string{"<script", "alert(", "onload", "onclick", "foreignObject", "foreignobject",
		"<iframe", "javascript:", "<image", "<use", "<animate", "169.254", "evil.test"} {
		if strings.Contains(h, banned) {
			t.Errorf("export contains %q:\n%s", banned, h)
		}
	}
}

// The sanitizer must not cost a figure its drawing: shapes, paths, text and
// gradients (the ModelPolicy SVG subset) survive the export, camelCase intact.
func TestExportHTMLJSONKeepsBenignFigureSVG(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50">` +
		`<defs><linearGradient id="g" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#f00"/></linearGradient></defs>` +
		`<rect x="1" y="1" width="40" height="20" fill="url(#g)"/>` +
		`<path d="M0 0 L10 10 Z" stroke="#333" stroke-width="2"/>` +
		`<text x="5" y="40" font-size="12">Q3</text></svg>`
	h := exportHTML(t, model.Block{Kind: model.BlockFigure, Text: "chart", SVG: svg})

	for _, want := range []string{`viewBox="0 0 100 50"`, `<linearGradient id="g" gradientUnits="userSpaceOnUse">`,
		`<rect x="1" y="1" width="40" height="20" fill="url(#g)"`, `<path d="M0 0 L10 10 Z"`, `>Q3</text>`,
		`<figcaption>chart</figcaption>`} {
		if !strings.Contains(h, want) {
			t.Errorf("export lost %q:\n%s", want, h)
		}
	}
}

// A document with nothing hostile in it exports as it did before the body was
// sanitized: structure, classes, the todo checkbox, and its own relative file
// and image paths.
func TestExportHTMLJSONKeepsOrdinaryDocument(t *testing.T) {
	h := exportHTML(t,
		head(2, "Plan"),
		model.Block{Kind: model.BlockParagraph, Runs: []model.TextRun{{Text: "see "}, {Text: "docs", Link: "https://example.com/d?a=1&b=2"}, {Text: " or "}, {Text: "top", Link: "#plan"}}},
		model.Block{Kind: model.BlockTable, Headers: []string{"k"}, Rows: [][]string{{"v"}}},
		model.Block{Kind: model.BlockTodo, Text: "ship", Checked: true},
		model.Block{Kind: model.BlockCallout, Tone: "warn", Text: "careful"},
		model.Block{Kind: model.BlockCode, Language: "go", Text: "x := 1"},
		model.Block{Kind: model.BlockFile, Name: "report.pdf", Src: "files/report.pdf"},
		model.Block{Kind: model.BlockImage, Text: "logo", Src: "assets/logo.png", Title: "Brand"},
		model.Block{Kind: model.BlockToggle, Text: "more", Children: []model.Block{para("inside")}},
	)

	for _, want := range []string{
		`<article class="eden-docs">`, `<h2>Plan</h2>`,
		`<a href="https://example.com/d?a=1&amp;b=2">docs</a>`, `<a href="#plan">top</a>`,
		`<th>k</th>`, `<td>v</td>`,
		`<input type="checkbox" disabled="" checked=""/>`,
		`<aside class="callout warn">`, `<code class="language-go">x := 1</code>`,
		`<a href="files/report.pdf">report.pdf</a>`,
		`<img src="assets/logo.png" alt="logo" title="Brand"/>`,
		`<details open=""><summary>more</summary>`, `<p>inside</p>`,
		"<style>",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("export lost %q:\n%s", want, h)
		}
	}
}
