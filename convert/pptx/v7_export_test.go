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

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
)

func TestFlattenBlocksV7Kinds(t *testing.T) {
	in := []model.Block{
		{Kind: model.BlockTodo, Text: "Ship", Checked: true, Children: []model.Block{{Kind: model.BlockParagraph, Text: "note"}}},
		{Kind: model.BlockCallout, Text: "Warn", Children: []model.Block{{Kind: model.BlockParagraph, Text: "body"}}},
		{Kind: model.BlockDivider},
		{Kind: model.BlockFile, Src: "a.pdf", Name: "A"},
		{Kind: model.BlockFigure, Text: "Logo", Src: "l.png"},
		{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "Tasks", Snapshot: &model.SheetSnapshot{Cells: []model.SheetCellSnap{{R: 0, C: 0, V: "A"}, {R: 0, C: 1, V: "B"}}}}},
		{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "OnlyName"}},
		{Kind: model.BlockSynced, SyncRef: &model.SyncRef{BlockID: "x"}},
	}
	out := flattenBlocks(in)
	joined := ""
	for _, b := range out {
		joined += b.Text + "|"
	}
	wants := []string{"Ship", "note", "Warn", "body", "—", "A", "Logo", "A | B", "[sheet:OnlyName]"}
	for _, w := range wants {
		if !strings.Contains(joined, w) {
			t.Fatalf("flatten missing %q in %q", w, joined)
		}
	}
	for _, b := range out {
		if b.Kind == model.BlockSynced {
			t.Fatal("synced should be skipped")
		}
	}
}
