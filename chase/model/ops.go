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
	"fmt"
	"strconv"
	"strings"
)

// OpKind is the discriminator for a document mutation an agent (or the
// human editor) applies to a Document.
type OpKind string

const (
	OpInsertBlock       OpKind = "insert_block"
	OpUpdateBlock       OpKind = "update_block"
	OpDeleteBlock       OpKind = "delete_block"
	OpMoveBlock         OpKind = "move_block"
	OpPatchBlock        OpKind = "patch_block"
	OpReplaceText       OpKind = "replace_text"
	OpInsertText        OpKind = "insert_text"
	OpInsertSection     OpKind = "insert_section"
	OpDeleteSection     OpKind = "delete_section"
	OpMoveSection       OpKind = "move_section"
	OpSetSectionAttrs   OpKind = "set_section_attrs"
	OpSetSectionNotes   OpKind = "set_section_notes"
	OpSetMetaDirectives OpKind = "set_meta_directives"
	OpSetProperties     OpKind = "set_properties"
	OpSetPropertyValue  OpKind = "set_property_value"
	OpAddComment        OpKind = "add_comment"
	OpResolveComment    OpKind = "resolve_comment"
	OpApplySuggestion   OpKind = "apply_suggestion"
)

// Op is one mutation in an apply_ops batch.
type Op struct {
	Op OpKind `json:"op"`

	SectionID int  `json:"sectionId,omitempty"`
	Index     *int `json:"index,omitempty"`

	BlockID       string `json:"blockId,omitempty"`
	ParentBlockID string `json:"parentBlockId,omitempty"`

	Block *Block `json:"block,omitempty"`

	// Attrs replaces section attrs for set_section_attrs (nil clears).
	Attrs map[string]string `json:"attrs,omitempty"`
	// Notes replaces section notes for set_section_notes.
	Notes []string `json:"notes,omitempty"`
	// Directives merges into meta.directives for set_meta_directives.
	Directives map[string]string `json:"directives,omitempty"`
	// ReplaceDirectives when true replaces meta.directives entirely.
	ReplaceDirectives bool `json:"replaceDirectives,omitempty"`

	// Text ops (schema v7).
	Old    string `json:"old,omitempty"`
	New    string `json:"new,omitempty"`
	Text   string `json:"text,omitempty"`
	Offset int    `json:"offset,omitempty"`
	All    bool   `json:"all,omitempty"`

	// patch_block flags to clear bool fields that omitempty cannot express.
	ClearChecked   bool `json:"clearChecked,omitempty"`
	ClearCollapsed bool `json:"clearCollapsed,omitempty"`

	// Page properties (schema v7).
	Properties        []PropertyDef `json:"properties,omitempty"`
	ReplaceProperties bool          `json:"replaceProperties,omitempty"`
	PropertyID        string        `json:"propertyId,omitempty"`
	Value             string        `json:"value,omitempty"`

	// Comments (schema v7).
	Comment   *Comment `json:"comment,omitempty"`
	CommentID string   `json:"commentId,omitempty"`
}

// NewDocument returns an empty single-section page.
func NewDocument(title string) *Document {
	doc := &Document{
		SchemaVersion: SchemaVersion,
		Sections:      []Section{{ID: 1}},
		Outline:       []OutlineEntry{},
	}
	if title != "" {
		doc.Meta.Directives = map[string]string{"title": title}
	}
	return doc
}

// EnsureBlockIDs assigns missing block ids depth-first. Existing ids are
// preserved. Safe to call repeatedly.
func EnsureBlockIDs(doc *Document) {
	if doc == nil {
		return
	}
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = SchemaVersion
	}
	next := maxBlockID(doc)
	for si := range doc.Sections {
		ensureBlockIDsSlice(&doc.Sections[si].Blocks, &next)
	}
}

// ensureBlockIDsSlice gives every block without an id the next b_N, counting
// up from *next. The caller seeds *next with the highest b_N in the document
// once (maxBlockID): every id handed out here is one more than the last, so it
// stays the maximum and the document never has to be walked again. That keeps
// assigning n ids O(n); walking the document per id made it O(n^2).
func ensureBlockIDsSlice(blocks *[]Block, next *int) {
	for i := range *blocks {
		if (*blocks)[i].ID == "" {
			*next++
			(*blocks)[i].ID = fmt.Sprintf("b_%d", *next)
		}
		if len((*blocks)[i].Children) > 0 {
			ensureBlockIDsSlice(&(*blocks)[i].Children, next)
		}
	}
}

// ApplyOps applies ops in order, ensures ids, and rebuilds Outline.
func ApplyOps(doc *Document, ops []Op) error {
	return ApplyOpsWithOptions(doc, ops, ApplyOpsOptions{})
}

func applyOne(doc *Document, op Op) error {
	switch op.Op {
	case OpInsertBlock:
		return insertBlock(doc, op)
	case OpUpdateBlock:
		return updateBlock(doc, op)
	case OpDeleteBlock:
		return deleteBlock(doc, op)
	case OpMoveBlock:
		return moveBlock(doc, op)
	case OpPatchBlock:
		return patchBlock(doc, op)
	case OpReplaceText:
		return replaceText(doc, op)
	case OpInsertText:
		return insertText(doc, op)
	case OpInsertSection:
		return insertSection(doc, op)
	case OpDeleteSection:
		return deleteSection(doc, op)
	case OpMoveSection:
		return moveSection(doc, op)
	case OpSetSectionAttrs:
		return setSectionAttrs(doc, op)
	case OpSetSectionNotes:
		return setSectionNotes(doc, op)
	case OpSetMetaDirectives:
		return setMetaDirectives(doc, op)
	case OpSetProperties:
		return setProperties(doc, op)
	case OpSetPropertyValue:
		return setPropertyValue(doc, op)
	case OpAddComment:
		return addComment(doc, op)
	case OpResolveComment:
		return resolveComment(doc, op)
	case OpApplySuggestion:
		return applySuggestion(doc, op)
	default:
		return fmt.Errorf("unknown op %q", op.Op)
	}
}

func insertBlock(doc *Document, op Op) error {
	if op.Block == nil {
		return fmt.Errorf("insert_block: missing block")
	}
	if err := validateEditableBlock(*op.Block); err != nil {
		return err
	}
	b := *op.Block
	SyncTextFromRuns(&b)
	syncTableCellRuns(&b)
	if b.ID == "" {
		b.ID = nextBlockID(doc)
	} else if findBlockPtr(doc, b.ID) != nil {
		return fmt.Errorf("insert_block: duplicate block id %q", b.ID)
	}
	// The block is not in the document yet, so its own b_N counts too: its
	// children are numbered above it, never given its id.
	next := maxBlockID(doc)
	if n, ok := blockIDNumber(b.ID); ok && n > next {
		next = n
	}
	ensureBlockIDsSlice(&b.Children, &next)

	if op.ParentBlockID != "" {
		parent := findBlockPtr(doc, op.ParentBlockID)
		if parent == nil {
			return fmt.Errorf("insert_block: parent %q not found", op.ParentBlockID)
		}
		if parent.Kind == BlockSynced {
			return fmt.Errorf("insert_block: cannot nest under synced block")
		}
		return insertIntoSlice(&parent.Children, b, op.Index)
	}
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		return err
	}
	return insertIntoSlice(&doc.Sections[si].Blocks, b, op.Index)
}

func insertIntoSlice(blocks *[]Block, b Block, index *int) error {
	if index == nil {
		*blocks = append(*blocks, b)
		return nil
	}
	idx := *index
	if idx < 0 || idx > len(*blocks) {
		return fmt.Errorf("insert_block: index %d out of range [0,%d]", idx, len(*blocks))
	}
	out := make([]Block, 0, len(*blocks)+1)
	out = append(out, (*blocks)[:idx]...)
	out = append(out, b)
	out = append(out, (*blocks)[idx:]...)
	*blocks = out
	return nil
}

func updateBlock(doc *Document, op Op) error {
	if op.BlockID == "" {
		return fmt.Errorf("update_block: missing blockId")
	}
	if op.Block == nil {
		return fmt.Errorf("update_block: missing block")
	}
	if err := validateEditableBlock(*op.Block); err != nil {
		return err
	}
	ptr := findBlockPtr(doc, op.BlockID)
	if ptr == nil {
		return fmt.Errorf("update_block: block %q not found", op.BlockID)
	}
	if ptr.Kind == BlockSynced {
		return fmt.Errorf("update_block: mutate the source block, not synced instance %q", op.BlockID)
	}
	b := *op.Block
	SyncTextFromRuns(&b)
	syncTableCellRuns(&b)
	if b.ID == "" {
		b.ID = op.BlockID
	} else if b.ID != op.BlockID {
		return fmt.Errorf("update_block: block.id %q must equal blockId %q", b.ID, op.BlockID)
	}
	// Preserve children unless the update payload supplies them.
	if b.Children == nil {
		b.Children = ptr.Children
	}
	*ptr = b
	return nil
}

func deleteBlock(doc *Document, op Op) error {
	if op.BlockID == "" {
		return fmt.Errorf("delete_block: missing blockId")
	}
	if isSyncSource(doc, op.BlockID) {
		return fmt.Errorf("delete_block: block %q is a sync source; delete synced instances first", op.BlockID)
	}
	if !removeBlock(doc, op.BlockID) {
		return fmt.Errorf("delete_block: block %q not found", op.BlockID)
	}
	return nil
}

func moveBlock(doc *Document, op Op) error {
	if op.BlockID == "" {
		return fmt.Errorf("move_block: missing blockId")
	}
	bptr := findBlockPtr(doc, op.BlockID)
	if bptr == nil {
		return fmt.Errorf("move_block: block %q not found", op.BlockID)
	}
	moved := *bptr
	if op.ParentBlockID != "" {
		if op.ParentBlockID == op.BlockID || isAncestor(doc, op.BlockID, op.ParentBlockID) {
			return fmt.Errorf("move_block: would create a cycle")
		}
	}
	if !removeBlock(doc, op.BlockID) {
		return fmt.Errorf("move_block: block %q not found", op.BlockID)
	}
	if op.ParentBlockID != "" {
		parent := findBlockPtr(doc, op.ParentBlockID)
		if parent == nil {
			// restore — should not happen often; re-insert at section 1 end
			_ = insertIntoSlice(&doc.Sections[0].Blocks, moved, nil)
			return fmt.Errorf("move_block: parent %q not found", op.ParentBlockID)
		}
		return insertIntoSlice(&parent.Children, moved, op.Index)
	}
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		_ = insertIntoSlice(&doc.Sections[0].Blocks, moved, nil)
		return err
	}
	return insertIntoSlice(&doc.Sections[si].Blocks, moved, op.Index)
}

func insertSection(doc *Document, op Op) error {
	sec := Section{ID: 0, Blocks: nil}
	if op.Attrs != nil {
		sec.Attrs = op.Attrs
	}
	if op.Notes != nil {
		sec.Notes = op.Notes
	}
	idx := len(doc.Sections)
	if op.Index != nil {
		idx = *op.Index
		if idx < 0 || idx > len(doc.Sections) {
			return fmt.Errorf("insert_section: index %d out of range [0,%d]", idx, len(doc.Sections))
		}
	}
	out := make([]Section, 0, len(doc.Sections)+1)
	out = append(out, doc.Sections[:idx]...)
	out = append(out, sec)
	out = append(out, doc.Sections[idx:]...)
	doc.Sections = out
	renumberSections(doc)
	return nil
}

func deleteSection(doc *Document, op Op) error {
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		return err
	}
	if len(doc.Sections) == 1 {
		return fmt.Errorf("delete_section: cannot delete the last section")
	}
	doc.Sections = append(doc.Sections[:si], doc.Sections[si+1:]...)
	renumberSections(doc)
	return nil
}

func moveSection(doc *Document, op Op) error {
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		return err
	}
	if op.Index == nil {
		return fmt.Errorf("move_section: missing index")
	}
	idx := *op.Index
	if idx < 0 || idx >= len(doc.Sections) {
		return fmt.Errorf("move_section: index %d out of range [0,%d)", idx, len(doc.Sections))
	}
	if si == idx {
		return nil
	}
	sec := doc.Sections[si]
	without := append(append([]Section{}, doc.Sections[:si]...), doc.Sections[si+1:]...)
	if idx > si {
		idx--
	}
	out := make([]Section, 0, len(doc.Sections))
	out = append(out, without[:idx]...)
	out = append(out, sec)
	out = append(out, without[idx:]...)
	doc.Sections = out
	renumberSections(doc)
	return nil
}

func setSectionAttrs(doc *Document, op Op) error {
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		return err
	}
	doc.Sections[si].Attrs = op.Attrs
	return nil
}

func setSectionNotes(doc *Document, op Op) error {
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		return err
	}
	doc.Sections[si].Notes = op.Notes
	return nil
}

func setMetaDirectives(doc *Document, op Op) error {
	if op.ReplaceDirectives || doc.Meta.Directives == nil {
		doc.Meta.Directives = map[string]string{}
	}
	for k, v := range op.Directives {
		doc.Meta.Directives[k] = v
	}
	return nil
}

func renumberSections(doc *Document) {
	for i := range doc.Sections {
		doc.Sections[i].ID = i + 1
	}
}

func validateEditableBlock(b Block) error {
	if err := validateCellRuns(b); err != nil {
		return err
	}
	switch b.Kind {
	case BlockParagraph:
		return validateChildren(b)
	case BlockHeading:
		if b.Level < 1 || b.Level > 6 {
			return fmt.Errorf("heading level %d out of range 1..6", b.Level)
		}
		return validateChildren(b)
	case BlockList:
		return validateChildren(b)
	case BlockCode, BlockMath:
		return validateChildren(b)
	case BlockTable:
		return validateChildren(b)
	case BlockImage:
		if b.Src == "" {
			return fmt.Errorf("image block missing src")
		}
		return validateChildren(b)
	case BlockQuote:
		return validateChildren(b)
	case BlockToggle:
		return validateChildren(b)
	case BlockSynced:
		if b.SyncRef == nil || b.SyncRef.BlockID == "" {
			return fmt.Errorf("synced block missing syncRef.blockId")
		}
		if len(b.Children) > 0 {
			return fmt.Errorf("synced block cannot have children")
		}
		return nil
	case BlockTodo:
		return validateChildren(b)
	case BlockCallout:
		return validateChildren(b)
	case BlockDivider:
		if b.Text != "" || len(b.Children) > 0 {
			return fmt.Errorf("divider must not carry text or children")
		}
		return nil
	case BlockFile:
		if b.Src == "" {
			return fmt.Errorf("file block missing src")
		}
		return nil
	case BlockSheet:
		if b.SheetEmbed == nil {
			return fmt.Errorf("sheet block missing sheetEmbed")
		}
		if b.SheetEmbed.Sheet == "" && b.SheetEmbed.Snapshot == nil {
			return fmt.Errorf("sheet embed needs sheet name or snapshot")
		}
		return nil
	case BlockFigure:
		if b.Src == "" && b.SVG == "" {
			return fmt.Errorf("figure block needs src or svg")
		}
		return nil
	default:
		if b.Kind == "" {
			return fmt.Errorf("missing block kind")
		}
		return fmt.Errorf("unsupported block kind %q", b.Kind)
	}
}

// validateCellRuns checks a block's cell runs (schema v7; AODex 1026 D14). Only
// a table carries HeaderRuns or RowRuns. A non-nil HeaderRuns has one entry per
// header cell, a non-nil RowRuns one entry per row, and a non-nil RowRuns[r]
// one entry per cell of row r, so the runs stay in lockstep with the cells; a
// nil entry is a plain cell (or a row of plain cells). No cell run holds a
// newline: a GFM table cell is one line, so a newline is refused rather than
// flattened. Marks are not checked, as they are not for Block.Runs.
// A block with no kind is left to validateEditableBlock to report.
func validateCellRuns(b Block) error {
	if (b.HeaderRuns == nil && b.RowRuns == nil) || b.Kind == "" {
		return nil
	}
	if b.Kind != BlockTable {
		return fmt.Errorf("%s block cannot carry headerRuns or rowRuns (table cells only)", b.Kind)
	}
	if b.HeaderRuns != nil && len(b.HeaderRuns) != len(b.Headers) {
		return fmt.Errorf("table headerRuns has %d entries for %d header cells", len(b.HeaderRuns), len(b.Headers))
	}
	for c, runs := range b.HeaderRuns {
		if err := singleLineCell(fmt.Sprintf("headerRuns[%d]", c), runs); err != nil {
			return err
		}
	}
	if b.RowRuns != nil && len(b.RowRuns) != len(b.Rows) {
		return fmt.Errorf("table rowRuns has %d entries for %d rows", len(b.RowRuns), len(b.Rows))
	}
	for r, row := range b.RowRuns {
		if row == nil {
			continue
		}
		if len(row) != len(b.Rows[r]) {
			return fmt.Errorf("table rowRuns[%d] has %d entries for %d cells", r, len(row), len(b.Rows[r]))
		}
		for c, runs := range row {
			if err := singleLineCell(fmt.Sprintf("rowRuns[%d][%d]", r, c), runs); err != nil {
				return err
			}
		}
	}
	return nil
}

// singleLineCell refuses a cell whose runs hold a newline.
func singleLineCell(at string, runs []TextRun) error {
	for _, run := range runs {
		if strings.Contains(run.Text, "\n") {
			return fmt.Errorf("table %s: a cell run cannot hold a newline (a table cell is one line)", at)
		}
	}
	return nil
}

func validateChildren(b Block) error {
	for i, c := range b.Children {
		if err := validateEditableBlock(c); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
	}
	return nil
}

func sectionIndex(doc *Document, sectionID int) (int, error) {
	if sectionID == 0 {
		return -1, fmt.Errorf("missing sectionId")
	}
	for i := range doc.Sections {
		if doc.Sections[i].ID == sectionID {
			return i, nil
		}
	}
	return -1, fmt.Errorf("section %d not found", sectionID)
}

func findBlockPtr(doc *Document, id string) *Block {
	for si := range doc.Sections {
		if p := findInSlice(doc.Sections[si].Blocks, id); p != nil {
			return p
		}
	}
	return nil
}

func findInSlice(blocks []Block, id string) *Block {
	for i := range blocks {
		if blocks[i].ID == id {
			return &blocks[i]
		}
		if p := findInSlice(blocks[i].Children, id); p != nil {
			return p
		}
	}
	return nil
}

func removeBlock(doc *Document, id string) bool {
	for si := range doc.Sections {
		if removeFromSlice(&doc.Sections[si].Blocks, id) {
			return true
		}
	}
	return false
}

func removeFromSlice(blocks *[]Block, id string) bool {
	for i := range *blocks {
		if (*blocks)[i].ID == id {
			*blocks = append((*blocks)[:i], (*blocks)[i+1:]...)
			return true
		}
		if removeFromSlice(&(*blocks)[i].Children, id) {
			return true
		}
	}
	return false
}

func isAncestor(doc *Document, ancestorID, descendantID string) bool {
	anc := findBlockPtr(doc, ancestorID)
	if anc == nil {
		return false
	}
	return findInSlice(anc.Children, descendantID) != nil
}

func isSyncSource(doc *Document, id string) bool {
	var walk func([]Block) bool
	walk = func(blocks []Block) bool {
		for _, b := range blocks {
			if b.Kind == BlockSynced && b.SyncRef != nil && b.SyncRef.BlockID == id && b.SyncRef.PageID == "" {
				return true
			}
			if walk(b.Children) {
				return true
			}
		}
		return false
	}
	for _, sec := range doc.Sections {
		if walk(sec.Blocks) {
			return true
		}
	}
	return false
}

func nextBlockID(doc *Document) string {
	return fmt.Sprintf("b_%d", maxBlockID(doc)+1)
}

// maxBlockID is the highest N among the document's b_N block ids (0 for none).
func maxBlockID(doc *Document) int {
	max := 0
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			if n, ok := blockIDNumber(b.ID); ok && n > max {
				max = n
			}
			walk(b.Children)
		}
	}
	for _, sec := range doc.Sections {
		walk(sec.Blocks)
	}
	return max
}

func blockIDNumber(id string) (int, bool) {
	if !strings.HasPrefix(id, "b_") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "b_"))
	return n, err == nil
}

func rebuildOutline(doc *Document) {
	out := make([]OutlineEntry, 0)
	var walk func(sectionID int, blocks []Block)
	walk = func(sectionID int, blocks []Block) {
		for _, b := range blocks {
			if b.Kind == BlockHeading {
				out = append(out, OutlineEntry{
					SectionID: sectionID,
					Level:     b.Level,
					Text:      b.Text,
					Slug:      slugify(b.Text),
				})
			}
			walk(sectionID, b.Children)
		}
	}
	for _, sec := range doc.Sections {
		walk(sec.ID, sec.Blocks)
	}
	doc.Outline = out
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
