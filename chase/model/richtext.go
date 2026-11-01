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

// EnsureTextRunsInvariant walks the document and flattens Runs into Text.
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
			walk(blocks[i].Children)
		}
	}
	for si := range doc.Sections {
		walk(doc.Sections[si].Blocks)
	}
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
