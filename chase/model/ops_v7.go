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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ApplyOpsOptions controls ApplyOps behavior (schema v7).
type ApplyOpsOptions struct {
	// DryRun validates and simulates without mutating doc. When true, ApplyOps
	// clones the document internally for validation and leaves the caller's
	// doc unchanged.
	DryRun bool
}

// ApplyOpsWithOptions applies ops with optional dry-run.
func ApplyOpsWithOptions(doc *Document, ops []Op, opts ApplyOpsOptions) error {
	if doc == nil {
		return fmt.Errorf("model.ApplyOps: nil document")
	}
	target := doc
	if opts.DryRun {
		raw, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("model.ApplyOps dry-run clone: %w", err)
		}
		var clone Document
		if err := json.Unmarshal(raw, &clone); err != nil {
			return fmt.Errorf("model.ApplyOps dry-run clone: %w", err)
		}
		target = &clone
	}
	return applyOpsMutating(target, ops)
}

func applyOpsMutating(doc *Document, ops []Op) error {
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = SchemaVersion
	}
	doc.SchemaVersion = SchemaVersion
	EnsureBlockIDs(doc)
	EnsureListItemIDs(doc)
	for i, op := range ops {
		if err := applyOne(doc, op); err != nil {
			return fmt.Errorf("model.ApplyOps: op[%d]: %w", i, err)
		}
	}
	EnsureBlockIDs(doc)
	EnsureListItemIDs(doc)
	EnsureTextRunsInvariant(doc)
	rebuildOutline(doc)
	return nil
}

// EnsureListItemIDs assigns missing list item ids (li_N).
func EnsureListItemIDs(doc *Document) {
	if doc == nil {
		return
	}
	// Seeded once and counted up, as EnsureBlockIDs does: n items stay O(n).
	next := maxListItemID(doc)
	var walk func([]Block)
	walk = func(blocks []Block) {
		for i := range blocks {
			for j := range blocks[i].Items {
				if blocks[i].Items[j].ID == "" {
					next++
					blocks[i].Items[j].ID = fmt.Sprintf("li_%d", next)
				}
				SyncListItemTextFromRuns(&blocks[i].Items[j])
			}
			walk(blocks[i].Children)
		}
	}
	for si := range doc.Sections {
		walk(doc.Sections[si].Blocks)
	}
}

// maxListItemID is the highest N among the document's li_N item ids (0 for none).
func maxListItemID(doc *Document) int {
	max := 0
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			for _, it := range b.Items {
				if strings.HasPrefix(it.ID, "li_") {
					n, err := strconv.Atoi(strings.TrimPrefix(it.ID, "li_"))
					if err == nil && n > max {
						max = n
					}
				}
			}
			walk(b.Children)
		}
	}
	for _, sec := range doc.Sections {
		walk(sec.Blocks)
	}
	return max
}

func nextCommentID(doc *Document) string {
	max := 0
	for _, c := range doc.Comments {
		if strings.HasPrefix(c.ID, "c_") {
			n, err := strconv.Atoi(strings.TrimPrefix(c.ID, "c_"))
			if err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("c_%d", max+1)
}

func patchBlock(doc *Document, op Op) error {
	if op.BlockID == "" {
		return fmt.Errorf("patch_block: missing blockId")
	}
	if op.Block == nil {
		return fmt.Errorf("patch_block: missing block patch")
	}
	ptr := findBlockPtr(doc, op.BlockID)
	if ptr == nil {
		return fmt.Errorf("patch_block: block %q not found", op.BlockID)
	}
	if ptr.Kind == BlockSynced {
		return fmt.Errorf("patch_block: mutate the source block, not synced instance %q", op.BlockID)
	}
	p := op.Block
	if p.Kind != "" && p.Kind != ptr.Kind {
		return fmt.Errorf("patch_block: cannot change kind from %q to %q", ptr.Kind, p.Kind)
	}
	// The patch is applied to a copy and the block is replaced only once the
	// copy validates, so a refused patch leaves the document as it was.
	next := *ptr
	if p.Text != "" || len(p.Runs) > 0 {
		if len(p.Runs) > 0 {
			next.Runs = append([]TextRun{}, p.Runs...)
			next.Text = FlattenRuns(next.Runs)
		} else {
			next.Text = p.Text
			next.Runs = SingleRun(p.Text)
		}
	}
	if p.Level != 0 {
		next.Level = p.Level
	}
	if p.Language != "" {
		next.Language = p.Language
	}
	if p.Display {
		next.Display = true
	}
	if p.Checked {
		next.Checked = true
	}
	if op.ClearChecked {
		next.Checked = false
	}
	if p.Icon != "" {
		next.Icon = p.Icon
	}
	if p.Tone != "" {
		next.Tone = p.Tone
	}
	if p.Src != "" {
		next.Src = p.Src
	}
	if p.Title != "" {
		next.Title = p.Title
	}
	if p.Name != "" {
		next.Name = p.Name
	}
	if p.Mime != "" {
		next.Mime = p.Mime
	}
	if p.Size != 0 {
		next.Size = p.Size
	}
	if p.SVG != "" {
		next.SVG = p.SVG
	}
	if p.SheetEmbed != nil {
		next.SheetEmbed = p.SheetEmbed
	}
	if p.SyncRef != nil {
		next.SyncRef = p.SyncRef
	}
	if p.Collapsed {
		next.Collapsed = true
	}
	if op.ClearCollapsed {
		next.Collapsed = false
	}
	if p.Items != nil {
		next.Items = p.Items
	}
	// Cell runs (schema v7; AODex 1026 D14). New Headers (Rows) without
	// HeaderRuns (RowRuns) clear the old runs, which describe the old cells.
	// Runs given win: they are set, and the cell text is re-derived from them.
	// A patch that names neither the cells nor their runs leaves the runs alone.
	if p.Headers != nil {
		next.Headers = p.Headers
		next.HeaderRuns = nil
	}
	if p.Rows != nil {
		next.Rows = p.Rows
		next.RowRuns = nil
	}
	if p.HeaderRuns != nil {
		next.HeaderRuns = p.HeaderRuns
	}
	if p.RowRuns != nil {
		next.RowRuns = p.RowRuns
	}
	if p.Aligns != nil {
		next.Aligns = p.Aligns
	}
	if p.Children != nil {
		next.Children = p.Children
	}
	if err := validateEditableBlock(next); err != nil {
		return err
	}
	syncTableCellRuns(&next)
	*ptr = next
	return nil
}

func replaceText(doc *Document, op Op) error {
	if op.Old == "" {
		return fmt.Errorf("replace_text: missing old")
	}
	replaced := false
	apply := func(b *Block) {
		if len(b.Runs) > 0 {
			runs, ok := ReplaceInRuns(b.Runs, op.Old, op.New, op.All)
			if ok {
				b.Runs = runs
				b.Text = FlattenRuns(runs)
				replaced = true
			}
			return
		}
		if !strings.Contains(b.Text, op.Old) {
			return
		}
		if op.All {
			b.Text = strings.ReplaceAll(b.Text, op.Old, op.New)
		} else {
			b.Text = strings.Replace(b.Text, op.Old, op.New, 1)
		}
		replaced = true
	}
	if op.BlockID != "" {
		ptr := findBlockPtr(doc, op.BlockID)
		if ptr == nil {
			return fmt.Errorf("replace_text: block %q not found", op.BlockID)
		}
		apply(ptr)
		if !replaced {
			return fmt.Errorf("replace_text: %q not found in block %q", op.Old, op.BlockID)
		}
		return nil
	}
	var walk func([]Block)
	walk = func(blocks []Block) {
		for i := range blocks {
			before := replaced
			apply(&blocks[i])
			if replaced && !op.All && !before {
				return
			}
			for j := range blocks[i].Items {
				it := &blocks[i].Items[j]
				if len(it.Runs) > 0 {
					runs, ok := ReplaceInRuns(it.Runs, op.Old, op.New, op.All)
					if ok {
						it.Runs = runs
						it.Text = FlattenRuns(runs)
						replaced = true
						if !op.All {
							return
						}
					}
				} else if strings.Contains(it.Text, op.Old) {
					if op.All {
						it.Text = strings.ReplaceAll(it.Text, op.Old, op.New)
					} else {
						it.Text = strings.Replace(it.Text, op.Old, op.New, 1)
						replaced = true
						return
					}
					replaced = true
				}
			}
			walk(blocks[i].Children)
			if replaced && !op.All {
				return
			}
		}
	}
	for si := range doc.Sections {
		walk(doc.Sections[si].Blocks)
		if replaced && !op.All {
			break
		}
	}
	if !replaced {
		return fmt.Errorf("replace_text: %q not found", op.Old)
	}
	return nil
}

func insertText(doc *Document, op Op) error {
	if op.BlockID == "" {
		return fmt.Errorf("insert_text: missing blockId")
	}
	ptr := findBlockPtr(doc, op.BlockID)
	if ptr == nil {
		return fmt.Errorf("insert_text: block %q not found", op.BlockID)
	}
	plain := ptr.Text
	if len(ptr.Runs) > 0 {
		plain = FlattenRuns(ptr.Runs)
	}
	next, err := InsertInPlain(plain, op.Offset, op.Text)
	if err != nil {
		return err
	}
	ptr.Text = next
	ptr.Runs = SingleRun(next)
	return nil
}

func setProperties(doc *Document, op Op) error {
	if op.ReplaceProperties {
		doc.Properties = nil
	}
	if op.Properties != nil {
		doc.Properties = append([]PropertyDef{}, op.Properties...)
	}
	for _, p := range doc.Properties {
		if p.ID == "" {
			return fmt.Errorf("set_properties: property missing id")
		}
		if err := validatePropertyType(p.Type); err != nil {
			return err
		}
	}
	return nil
}

func setPropertyValue(doc *Document, op Op) error {
	if op.PropertyID == "" {
		return fmt.Errorf("set_property_value: missing propertyId")
	}
	found := false
	for _, p := range doc.Properties {
		if p.ID == op.PropertyID {
			found = true
			break
		}
	}
	if !found && len(doc.Properties) > 0 {
		return fmt.Errorf("set_property_value: property %q not found", op.PropertyID)
	}
	if doc.PropertyValues == nil {
		doc.PropertyValues = map[string]string{}
	}
	doc.PropertyValues[op.PropertyID] = op.Value
	return nil
}

func validatePropertyType(t PropertyType) error {
	switch t {
	case PropText, PropNumber, PropSelect, PropMultiSelect, PropDate, PropCheckbox, PropURL, PropEmail, PropRelation, "":
		return nil
	default:
		return fmt.Errorf("unknown property type %q", t)
	}
}

func addComment(doc *Document, op Op) error {
	if op.Comment == nil {
		return fmt.Errorf("add_comment: missing comment")
	}
	c := *op.Comment
	if c.BlockID == "" {
		return fmt.Errorf("add_comment: missing blockId")
	}
	if findBlockPtr(doc, c.BlockID) == nil {
		return fmt.Errorf("add_comment: block %q not found", c.BlockID)
	}
	if c.ID == "" {
		c.ID = nextCommentID(doc)
	}
	doc.Comments = append(doc.Comments, c)
	return nil
}

func resolveComment(doc *Document, op Op) error {
	if op.CommentID == "" {
		return fmt.Errorf("resolve_comment: missing commentId")
	}
	for i := range doc.Comments {
		if doc.Comments[i].ID == op.CommentID {
			doc.Comments[i].Resolved = true
			return nil
		}
	}
	return fmt.Errorf("resolve_comment: comment %q not found", op.CommentID)
}

func applySuggestion(doc *Document, op Op) error {
	if op.CommentID == "" {
		return fmt.Errorf("apply_suggestion: missing commentId")
	}
	var c *Comment
	for i := range doc.Comments {
		if doc.Comments[i].ID == op.CommentID {
			c = &doc.Comments[i]
			break
		}
	}
	if c == nil {
		return fmt.Errorf("apply_suggestion: comment %q not found", op.CommentID)
	}
	if c.Suggestion == nil {
		return fmt.Errorf("apply_suggestion: comment %q has no suggestion", op.CommentID)
	}
	ptr := findBlockPtr(doc, c.BlockID)
	if ptr == nil {
		return fmt.Errorf("apply_suggestion: block %q not found", c.BlockID)
	}
	if len(c.Suggestion.Runs) > 0 {
		ptr.Runs = append([]TextRun{}, c.Suggestion.Runs...)
		ptr.Text = FlattenRuns(ptr.Runs)
	} else {
		ptr.Text = c.Suggestion.Text
		ptr.Runs = SingleRun(ptr.Text)
	}
	c.Resolved = true
	return nil
}
