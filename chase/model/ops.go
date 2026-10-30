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
// human editor) applies to a Document. The vocabulary is intentionally
// small: this is the AI-first edit spike surface, not a full CRDT.
type OpKind string

const (
	// OpInsertBlock inserts Block into section SectionID at Index
	// (append when Index is nil).
	OpInsertBlock OpKind = "insert_block"
	// OpUpdateBlock replaces the block addressed by BlockID with Block.
	OpUpdateBlock OpKind = "update_block"
	// OpDeleteBlock removes the block addressed by BlockID.
	OpDeleteBlock OpKind = "delete_block"
)

// Op is one mutation in an apply_ops batch. JSON field names are the
// agent-facing wire contract (see AGENTS.md).
type Op struct {
	Op OpKind `json:"op"`

	// SectionID is the 1-based section for insert_block.
	SectionID int `json:"sectionId,omitempty"`

	// Index is the 0-based insert position within the section's Blocks.
	// Nil means append.
	Index *int `json:"index,omitempty"`

	// BlockID addresses an existing block for update_block / delete_block.
	BlockID string `json:"blockId,omitempty"`

	// Block is the payload for insert_block / update_block.
	Block *Block `json:"block,omitempty"`
}

// NewDocument returns an empty single-section page suitable as an agent
// create_page result. Title is stored in Meta.Directives["title"] when
// non-empty. Outline is a non-nil empty slice so JSON always carries
// "outline":[].
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

// ApplyOps applies ops to doc in order. On success, Outline is rebuilt from
// heading blocks. The spike allows paragraph and heading blocks only;
// other kinds return an error so agents fail closed rather than silently
// dropping rich content.
//
// doc must be non-nil. ApplyOps mutates doc in place.
func ApplyOps(doc *Document, ops []Op) error {
	if doc == nil {
		return fmt.Errorf("model.ApplyOps: nil document")
	}
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = SchemaVersion
	}
	for i, op := range ops {
		if err := applyOne(doc, op); err != nil {
			return fmt.Errorf("model.ApplyOps: op[%d]: %w", i, err)
		}
	}
	rebuildOutline(doc)
	return nil
}

func applyOne(doc *Document, op Op) error {
	switch op.Op {
	case OpInsertBlock:
		return insertBlock(doc, op)
	case OpUpdateBlock:
		return updateBlock(doc, op)
	case OpDeleteBlock:
		return deleteBlock(doc, op)
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
	si, err := sectionIndex(doc, op.SectionID)
	if err != nil {
		return err
	}
	b := *op.Block
	if b.ID == "" {
		b.ID = nextBlockID(doc)
	} else if blockIndex(doc, b.ID) >= 0 {
		return fmt.Errorf("insert_block: duplicate block id %q", b.ID)
	}
	blocks := doc.Sections[si].Blocks
	if op.Index == nil {
		doc.Sections[si].Blocks = append(blocks, b)
		return nil
	}
	idx := *op.Index
	if idx < 0 || idx > len(blocks) {
		return fmt.Errorf("insert_block: index %d out of range [0,%d]", idx, len(blocks))
	}
	out := make([]Block, 0, len(blocks)+1)
	out = append(out, blocks[:idx]...)
	out = append(out, b)
	out = append(out, blocks[idx:]...)
	doc.Sections[si].Blocks = out
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
	si, bi := findBlock(doc, op.BlockID)
	if si < 0 {
		return fmt.Errorf("update_block: block %q not found", op.BlockID)
	}
	b := *op.Block
	if b.ID == "" {
		b.ID = op.BlockID
	} else if b.ID != op.BlockID {
		return fmt.Errorf("update_block: block.id %q must equal blockId %q", b.ID, op.BlockID)
	}
	doc.Sections[si].Blocks[bi] = b
	return nil
}

func deleteBlock(doc *Document, op Op) error {
	if op.BlockID == "" {
		return fmt.Errorf("delete_block: missing blockId")
	}
	si, bi := findBlock(doc, op.BlockID)
	if si < 0 {
		return fmt.Errorf("delete_block: block %q not found", op.BlockID)
	}
	blocks := doc.Sections[si].Blocks
	doc.Sections[si].Blocks = append(blocks[:bi], blocks[bi+1:]...)
	return nil
}

func validateEditableBlock(b Block) error {
	switch b.Kind {
	case BlockParagraph:
		return nil
	case BlockHeading:
		if b.Level < 1 || b.Level > 6 {
			return fmt.Errorf("heading level %d out of range 1..6", b.Level)
		}
		return nil
	case BlockList, BlockCode, BlockMath, BlockTable, BlockImage, BlockQuote:
		return fmt.Errorf("unsupported block kind %q in apply_ops spike (paragraph|heading only)", b.Kind)
	default:
		if b.Kind == "" {
			return fmt.Errorf("missing block kind")
		}
		return fmt.Errorf("unsupported block kind %q in apply_ops spike (paragraph|heading only)", b.Kind)
	}
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

func findBlock(doc *Document, id string) (sectionIdx, blockIdx int) {
	for si := range doc.Sections {
		for bi, b := range doc.Sections[si].Blocks {
			if b.ID == id {
				return si, bi
			}
		}
	}
	return -1, -1
}

func blockIndex(doc *Document, id string) int {
	si, bi := findBlock(doc, id)
	if si < 0 {
		return -1
	}
	return bi
}

func nextBlockID(doc *Document) string {
	max := 0
	for _, sec := range doc.Sections {
		for _, b := range sec.Blocks {
			if !strings.HasPrefix(b.ID, "b_") {
				continue
			}
			n, err := strconv.Atoi(strings.TrimPrefix(b.ID, "b_"))
			if err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("b_%d", max+1)
}

func rebuildOutline(doc *Document) {
	out := make([]OutlineEntry, 0)
	for _, sec := range doc.Sections {
		for _, b := range sec.Blocks {
			if b.Kind != BlockHeading {
				continue
			}
			out = append(out, OutlineEntry{
				SectionID: sec.ID,
				Level:     b.Level,
				Text:      b.Text,
				Slug:      slugify(b.Text),
			})
		}
	}
	doc.Outline = out
}

// slugify is a minimal agent-facing slug for outline entries produced by
// ApplyOps (not the goldmark AutoHeadingID path). Good enough for jump
// targets in the spike; import_markdown still uses Build's real slugs.
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
