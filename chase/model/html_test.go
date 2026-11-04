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

func TestToHTMLRendersV7Blocks(t *testing.T) {
	doc := &Document{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{Directives: map[string]string{"profile": "article"}},
		Sections: []Section{{
			ID: 1,
			Blocks: []Block{
				{Kind: BlockHeading, Level: 1, Text: "Roadmap"},
				{Kind: BlockTodo, Text: "Ship", Checked: true},
				{Kind: BlockCallout, Text: "Watch", Tone: "warn", Icon: "!"},
				{Kind: BlockSheet, SheetEmbed: &SheetEmbed{
					Sheet:    "Tasks",
					Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{{R: 0, C: 0, V: "A"}}},
				}},
				{Kind: BlockFigure, Text: "logo", SVG: "<svg/>"},
				{Kind: BlockDivider},
			},
		}},
	}
	html := ToHTML(doc)
	for _, want := range []string{
		`class="eden-docs"`,
		"<h1>Roadmap</h1>",
		`type="checkbox" disabled checked`,
		"Ship",
		`class="callout warn"`,
		"Tasks",
		"<td>A</td>",
		"<svg/>",
		"<figcaption>logo</figcaption>",
		"<hr/>",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ToHTML missing %q in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "[sheet:") || strings.Contains(html, `"kind"`) {
		t.Fatalf("ToHTML looks like a JSON dump: %s", html)
	}
}
