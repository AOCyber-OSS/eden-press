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

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
)

func TestRenderBlockV7Kinds(t *testing.T) {
	cases := []struct {
		name  string
		block model.Block
		want  []string
	}{
		{"todo_open", model.Block{Kind: model.BlockTodo, Text: "Open"}, []string{"☐ ", "Open"}},
		{"todo_done", model.Block{Kind: model.BlockTodo, Text: "Done", Checked: true}, []string{"☑ ", "Done"}},
		{"callout", model.Block{Kind: model.BlockCallout, Text: "Note"}, []string{"Quote", "Note"}},
		{"divider", model.Block{Kind: model.BlockDivider}, []string{"―"}},
		{"file", model.Block{Kind: model.BlockFile, Src: "a.pdf", Name: "Spec"}, []string{"[Spec](a.pdf)"}},
		{"figure_src", model.Block{Kind: model.BlockFigure, Src: "a.png", Text: "Alt"}, []string{"[Alt: a.png]"}},
		{"figure_svg", model.Block{Kind: model.BlockFigure, SVG: "<svg/>", Text: "mark"}, []string{"[mark]"}},
		{"sheet_snap", model.Block{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{
			Snapshot: &model.SheetSnapshot{Cells: []model.SheetCellSnap{{R: 0, C: 0, V: "H"}, {R: 1, C: 0, V: "V"}}},
		}}, []string{"<w:tbl>", "H", "V"}},
		{"sheet_name", model.Block{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "Tasks"}}, []string{"[sheet:Tasks]"}},
		{"synced", model.Block{Kind: model.BlockSynced, SyncRef: &model.SyncRef{BlockID: "x"}}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderBlock(tc.block)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Fatalf("missing %q in %s", w, got)
				}
			}
			if tc.name == "synced" && got != "" {
				t.Fatalf("synced should be empty, got %q", got)
			}
		})
	}
}
