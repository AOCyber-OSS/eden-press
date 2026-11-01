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

package xlsx

import (
	"fmt"
	"strconv"
	"strings"
)

// WbOpKind discriminates workbook mutations for agents/editors.
type WbOpKind string

const (
	WbOpSetCell       WbOpKind = "set_cell"
	WbOpAddSheet      WbOpKind = "add_sheet"
	WbOpRenameSheet   WbOpKind = "rename_sheet"
	WbOpDeleteSheet   WbOpKind = "delete_sheet"
	WbOpSetHeaderRow  WbOpKind = "set_header_row"
	WbOpSetAlign      WbOpKind = "set_align"
	WbOpSetColumn     WbOpKind = "set_column"
	WbOpSetRelation   WbOpKind = "set_relation"
	WbOpClearRelation WbOpKind = "clear_relation"
	WbOpEnsureRowID   WbOpKind = "ensure_row_id"
)

// WbOp is one workbook mutation.
type WbOp struct {
	Op WbOpKind `json:"op"`

	Sheet    string `json:"sheet,omitempty"`
	NewName  string `json:"newName,omitempty"`
	Row      int    `json:"row,omitempty"`
	Column   int    `json:"column,omitempty"`
	Value    string `json:"value,omitempty"`
	Header   bool   `json:"header,omitempty"`
	Align    string `json:"align,omitempty"`
	ColumnID string `json:"columnId,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	RowID    string `json:"rowId,omitempty"`
	Rels     []RelationRef `json:"rels,omitempty"`
}

// ApplyWorkbookOps applies ops in order and ensures every occupied row has a row id.
func ApplyWorkbookOps(wb *Workbook, ops []WbOp) error {
	if wb == nil {
		return fmt.Errorf("xlsx: ApplyWorkbookOps: nil workbook")
	}
	for i, op := range ops {
		if err := applyWbOne(wb, op); err != nil {
			return fmt.Errorf("xlsx: ApplyWorkbookOps: op[%d]: %w", i, err)
		}
	}
	EnsureRowIDs(wb)
	return nil
}

// EnsureRowIDs assigns missing stable row ids for rows that have content or relations.
func EnsureRowIDs(wb *Workbook) {
	if wb == nil {
		return
	}
	for _, s := range wb.sheets {
		s.ensureRowIDs()
	}
}

func (s *Sheet) ensureRowIDs() {
	rows := map[int]bool{}
	for r, cols := range s.cells {
		for _, cell := range cols {
			if cellOccupied(cell) {
				rows[r] = true
				break
			}
		}
	}
	for r, cols := range s.relCells {
		for _, refs := range cols {
			if len(refs) > 0 {
				rows[r] = true
				break
			}
		}
	}
	for r := range s.header {
		rows[r] = true
	}
	for r := range rows {
		if s.rowIDs[r] == "" {
			s.rowIDs[r] = s.nextRowID()
		}
	}
}

func (s *Sheet) nextRowID() string {
	max := 0
	for _, id := range s.rowIDs {
		if strings.HasPrefix(id, "r_") {
			n, err := strconv.Atoi(strings.TrimPrefix(id, "r_"))
			if err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("r_%d", max+1)
}

// SetRelations stores relation refs at a cell (workbook v3).
func (s *Sheet) SetRelations(row, col int, refs []RelationRef) error {
	if err := checkCell(row, col); err != nil {
		return err
	}
	if s.relCells[row] == nil {
		s.relCells[row] = map[int][]RelationRef{}
	}
	if len(refs) == 0 {
		delete(s.relCells[row], col)
		if len(s.relCells[row]) == 0 {
			delete(s.relCells, row)
		}
		return nil
	}
	s.relCells[row][col] = append([]RelationRef{}, refs...)
	if row > s.maxRow {
		s.maxRow = row
	}
	if col > s.maxCol {
		s.maxCol = col
	}
	return nil
}

// Relations returns relation refs at a cell.
func (s *Sheet) Relations(row, col int) []RelationRef {
	return s.relCells[row][col]
}

// SetColumn upserts a typed column definition by id.
func (s *Sheet) SetColumn(col Column) error {
	if col.ID == "" {
		return fmt.Errorf("xlsx: column id is empty")
	}
	switch col.Type {
	case ColumnText, ColumnNumber, ColumnFormula, ColumnRelation, "":
		if col.Type == "" {
			col.Type = ColumnText
		}
	default:
		return fmt.Errorf("xlsx: unknown column type %q", col.Type)
	}
	for i := range s.columns {
		if s.columns[i].ID == col.ID {
			s.columns[i] = col
			return nil
		}
	}
	s.columns = append(s.columns, col)
	return nil
}

// Columns returns a copy of column definitions.
func (s *Sheet) Columns() []Column {
	return append([]Column{}, s.columns...)
}

// RowID returns the stable id for a row index.
func (s *Sheet) RowID(row int) string { return s.rowIDs[row] }

// RowIndexByID finds a row index by stable id.
func (s *Sheet) RowIndexByID(id string) (int, bool) {
	for r, rid := range s.rowIDs {
		if rid == id {
			return r, true
		}
	}
	return 0, false
}

func applyWbOne(wb *Workbook, op WbOp) error {
	switch op.Op {
	case WbOpSetCell:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		return s.Set(op.Row, op.Column, op.Value)
	case WbOpAddSheet:
		_, err := wb.AddSheet(op.Sheet)
		return err
	case WbOpRenameSheet:
		return wb.RenameSheet(op.Sheet, op.NewName)
	case WbOpDeleteSheet:
		return wb.DeleteSheet(op.Sheet)
	case WbOpSetHeaderRow:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		return s.SetHeader(op.Row, op.Header)
	case WbOpSetAlign:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		return s.SetAlign(op.Column, op.Align)
	case WbOpSetColumn:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		return s.SetColumn(Column{ID: op.ColumnID, Name: op.Name, Type: ColumnType(op.Type)})
	case WbOpSetRelation:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		row := op.Row
		if op.RowID != "" {
			r, ok := s.RowIndexByID(op.RowID)
			if !ok {
				return fmt.Errorf("row id %q not found", op.RowID)
			}
			row = r
		}
		col, err := resolveColumnIndex(s, op)
		if err != nil {
			return err
		}
		for _, ref := range op.Rels {
			if ref.PageID == "" && ref.Sheet == "" {
				if _, ok := s.RowIndexByID(ref.RowID); !ok && ref.RowID != "" {
					// same-sheet target must exist when addressable
					if !rowIDExists(wb, ref.RowID) {
						return fmt.Errorf("relation target row %q not found", ref.RowID)
					}
				}
			}
		}
		return s.SetRelations(row, col, op.Rels)
	case WbOpClearRelation:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		row := op.Row
		if op.RowID != "" {
			r, ok := s.RowIndexByID(op.RowID)
			if !ok {
				return fmt.Errorf("row id %q not found", op.RowID)
			}
			row = r
		}
		col, err := resolveColumnIndex(s, op)
		if err != nil {
			return err
		}
		return s.SetRelations(row, col, nil)
	case WbOpEnsureRowID:
		s, err := requireSheet(wb, op.Sheet)
		if err != nil {
			return err
		}
		if s.rowIDs[op.Row] == "" {
			if op.RowID != "" {
				s.rowIDs[op.Row] = op.RowID
			} else {
				s.rowIDs[op.Row] = s.nextRowID()
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown op %q", op.Op)
	}
}

func requireSheet(wb *Workbook, name string) (*Sheet, error) {
	if name == "" {
		return nil, fmt.Errorf("missing sheet")
	}
	s, ok := wb.Sheet(name)
	if !ok {
		return nil, fmt.Errorf("sheet %q not found", name)
	}
	return s, nil
}

func resolveColumnIndex(s *Sheet, op WbOp) (int, error) {
	if op.ColumnID != "" {
		for i, c := range s.columns {
			if c.ID == op.ColumnID {
				return i, nil
			}
		}
		return 0, fmt.Errorf("column id %q not found", op.ColumnID)
	}
	return op.Column, nil
}

func rowIDExists(wb *Workbook, id string) bool {
	for _, s := range wb.sheets {
		if _, ok := s.RowIndexByID(id); ok {
			return true
		}
	}
	return false
}
