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

import "strings"

// FlattenRuns concatenates run texts into the plain-text invariant.
func FlattenRuns(runs []TextRun) string {
	if len(runs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// SyncTextFromRuns sets b.Text from b.Runs when Runs is non-empty.
func SyncTextFromRuns(b *Block) {
	if b == nil || len(b.Runs) == 0 {
		return
	}
	b.Text = FlattenRuns(b.Runs)
}

// SyncListItemTextFromRuns sets item.Text from item.Runs when present.
func SyncListItemTextFromRuns(it *ListItem) {
	if it == nil || len(it.Runs) == 0 {
		return
	}
	it.Text = FlattenRuns(it.Runs)
}

// EnsureTextRunsInvariant walks the document and flattens Runs into Text. It
// also mirrors a table's cell runs into its Headers and Rows and puts
// HeaderRuns/RowRuns in canonical form (see syncTableCellRuns).
func EnsureTextRunsInvariant(doc *Document) {
	if doc == nil {
		return
	}
	var walk func([]Block)
	walk = func(blocks []Block) {
		for i := range blocks {
			SyncTextFromRuns(&blocks[i])
			for j := range blocks[i].Items {
				SyncListItemTextFromRuns(&blocks[i].Items[j])
			}
			syncTableCellRuns(&blocks[i])
			walk(blocks[i].Children)
		}
	}
	for si := range doc.Sections {
		walk(doc.Sections[si].Blocks)
	}
}

// syncTableCellRuns mirrors b's cell runs into its cell text and puts
// HeaderRuns and RowRuns in canonical form (schema v7; AODex 1026 D14):
//   - a cell with runs takes FlattenRuns of them as its text;
//   - a plain cell's entry is nil: no runs, or one run with no marks and no
//     link. A nil entry leaves its cell text as it is;
//   - a non-nil HeaderRuns has len(Headers) entries, a non-nil RowRuns has
//     len(Rows), and a non-nil RowRuns[r] has len(Rows[r]). Headers and Rows
//     are the authoritative cells: a missing entry is a plain cell, and an
//     entry past the last cell describes nothing and is dropped;
//   - HeaderRuns, RowRuns and each RowRuns[r] are nil unless they hold a
//     non-nil entry.
//
// It applies to every kind, so a non-table block (which has no cells) is left
// with neither field; ApplyOps refuses the fields there in the first place.
func syncTableCellRuns(b *Block) {
	if b == nil {
		return
	}
	b.HeaderRuns = canonCellRuns(b.Headers, b.HeaderRuns)
	if b.RowRuns == nil {
		return
	}
	rows := make([][][]TextRun, len(b.Rows))
	styled := false
	for r := range b.Rows {
		var runs [][]TextRun
		if r < len(b.RowRuns) {
			runs = b.RowRuns[r]
		}
		if rows[r] = canonCellRuns(b.Rows[r], runs); rows[r] != nil {
			styled = true
		}
	}
	if !styled {
		rows = nil
	}
	b.RowRuns = rows
}

// canonCellRuns mirrors runs into cells (in place) and returns runs in the
// canonical form syncTableCellRuns describes: len(cells) entries with nil for
// every plain cell, or nil when no cell is styled.
func canonCellRuns(cells []string, runs [][]TextRun) [][]TextRun {
	if runs == nil {
		return nil
	}
	out := make([][]TextRun, len(cells))
	styled := false
	for c := range cells {
		if c >= len(runs) || len(runs[c]) == 0 {
			continue
		}
		cells[c] = FlattenRuns(runs[c])
		if isPlainRuns(runs[c]) {
			continue
		}
		out[c] = runs[c]
		styled = true
	}
	if !styled {
		return nil
	}
	return out
}

// isPlainRuns reports whether runs is one run with no marks and no link: text
// that needs no runs at all (the codec's rule for Block.Runs, applied to cells).
func isPlainRuns(runs []TextRun) bool {
	return len(runs) == 1 && len(runs[0].Marks) == 0 && runs[0].Link == ""
}

// SingleRun returns a one-element runs slice for plain text, or nil if empty.
func SingleRun(text string) []TextRun {
	if text == "" {
		return nil
	}
	return []TextRun{{Text: text}}
}

// ReplaceInRuns replaces old with new across run texts (first or all).
// Returns the new runs and whether any replacement happened.
func ReplaceInRuns(runs []TextRun, old, new string, all bool) ([]TextRun, bool) {
	if old == "" || len(runs) == 0 {
		return runs, false
	}
	plain := FlattenRuns(runs)
	if !strings.Contains(plain, old) {
		return runs, false
	}
	// Simple path: operate on flattened plain, then wrap as a single run
	// (marks are lost across the replace — documented lossiness for agents
	// that need mark-preserving replace should use patch_block on runs).
	var next string
	if all {
		next = strings.ReplaceAll(plain, old, new)
	} else {
		next = strings.Replace(plain, old, new, 1)
	}
	if next == plain {
		return runs, false
	}
	return []TextRun{{Text: next}}, true
}

// InsertInPlain inserts s at byte offset off in plain text.
func InsertInPlain(plain string, off int, s string) (string, error) {
	if off < 0 || off > len(plain) {
		return "", errOffset
	}
	return plain[:off] + s + plain[off:], nil
}

var errOffset = errString("insert_text: offset out of range")

type errString string

func (e errString) Error() string { return string(e) }
