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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// WorkbookSchema is the versioned JSON shape a host embeds and eden-ui edits.
// xlsx remains the interchange file; this schema is the editing snapshot.
// Writers emit v3 (columns, row ids, relation refs). Readers still accept
// v2 and v1.
const (
	WorkbookSchema   = "eden-press.workbook/v3"
	workbookSchemaV2 = "eden-press.workbook/v2"
	workbookSchemaV1 = "eden-press.workbook/v1"
)

// ColumnType is a typed property on a sheet column (workbook v3).
type ColumnType string

const (
	ColumnText        ColumnType = "text"
	ColumnNumber      ColumnType = "number"
	ColumnFormula     ColumnType = "formula"
	ColumnRelation    ColumnType = "relation"
	ColumnSelect      ColumnType = "select"
	ColumnMultiSelect ColumnType = "multi_select"
	ColumnDate        ColumnType = "date"
	ColumnCheckbox    ColumnType = "checkbox"
	ColumnURL         ColumnType = "url"
	ColumnEmail       ColumnType = "email"
)

// Column is a named typed property (workbook v3+).
type Column struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Type    ColumnType `json:"type"`
	Options []string   `json:"options,omitempty"`
}

// NamedRange is a stable named rectangle on a sheet (additive v3 field).
type NamedRange struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Sheet string `json:"sheet"`
	R0    int    `json:"r0"`
	R1    int    `json:"r1"`
	C0    int    `json:"c0"`
	C1    int    `json:"c1"`
}

// RelationRef points at a row (optionally on another page/sheet).
type RelationRef struct {
	RowID  string `json:"rowId"`
	PageID string `json:"pageId,omitempty"`
	Sheet  string `json:"sheet,omitempty"`
}

// Excel's worksheet limits. A cell outside them cannot be written, and
// emitting a row past the limit produces a workbook Excel refuses to open.
const (
	maxSheetRows = 1048576
	maxSheetCols = 16384
	maxSheetName = 31
)

// Workbook is an editable spreadsheet: sheets, cells, formulas, and the
// .xlsx bytes those edits become. It is the management surface on top of the
// same OOXML packager ToXLSX uses. Formulas are stored, not evaluated.
type Workbook struct {
	title       string
	sheets      []*Sheet
	namedRanges []NamedRange
}

// Sheet is one worksheet. Cells are sparse. A value that starts with "=" is
// stored as a formula (the "=" is not part of the Excel expression). Get
// returns that "=" again, so Set(Get()) round-trips.
type Sheet struct {
	name     string
	cells    map[int]map[int]storedCell
	header   map[int]bool
	aligns   []string
	columns  []Column
	rowIDs   map[int]string                 // row index -> stable id
	relCells map[int]map[int][]RelationRef // row -> col -> refs
	maxRow   int
	maxCol   int
}

type storedCell struct {
	text    string // raw value, or a formula body without the leading "="
	formula bool
	set     bool
}

// NewWorkbook returns an empty workbook. title is the dc:title core property;
// an empty title is written as "Workbook" at pack time.
func NewWorkbook(title string) *Workbook {
	return &Workbook{title: title}
}

// Title returns the workbook title.
func (w *Workbook) Title() string { return w.title }

// SetTitle replaces the workbook title.
func (w *Workbook) SetTitle(title string) { w.title = title }

// SheetNames returns worksheet names in workbook order.
func (w *Workbook) SheetNames() []string {
	out := make([]string, len(w.sheets))
	for i, s := range w.sheets {
		out[i] = s.name
	}
	return out
}

// Sheet returns the worksheet with this exact name.
func (w *Workbook) Sheet(name string) (*Sheet, bool) {
	for _, s := range w.sheets {
		if s.name == name {
			return s, true
		}
	}
	return nil, false
}

// AddSheet appends a worksheet. The name must be unique ignoring case and
// must satisfy Excel's sheet-name rules.
func (w *Workbook) AddSheet(name string) (*Sheet, error) {
	if err := validateSheetName(name); err != nil {
		return nil, err
	}
	if w.hasName(name) {
		return nil, fmt.Errorf("xlsx: sheet %q already exists", name)
	}
	s := newSheet(name)
	w.sheets = append(w.sheets, s)
	return s, nil
}

// RenameSheet changes a worksheet's name. Renaming a sheet to its current
// name succeeds. The new name follows the same rules as AddSheet.
func (w *Workbook) RenameSheet(oldName, newName string) error {
	s, ok := w.Sheet(oldName)
	if !ok {
		return fmt.Errorf("xlsx: sheet %q not found", oldName)
	}
	if oldName == newName {
		return nil
	}
	if err := validateSheetName(newName); err != nil {
		return err
	}
	if w.hasName(newName) {
		return fmt.Errorf("xlsx: sheet %q already exists", newName)
	}
	s.name = newName
	return nil
}

// DeleteSheet removes a worksheet. The last remaining sheet cannot be
// deleted: an xlsx package with zero worksheets does not open.
func (w *Workbook) DeleteSheet(name string) error {
	idx := -1
	for i, s := range w.sheets {
		if s.name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("xlsx: sheet %q not found", name)
	}
	if len(w.sheets) == 1 {
		return fmt.Errorf("xlsx: cannot delete the last sheet %q", name)
	}
	w.sheets = append(w.sheets[:idx], w.sheets[idx+1:]...)
	return nil
}

// Bytes renders the workbook to an .xlsx package. The same workbook twice
// yields identical bytes.
func (w *Workbook) Bytes() ([]byte, error) {
	if w == nil {
		return nil, fmt.Errorf("xlsx: workbook is nil")
	}
	packed := make([]packedSheet, len(w.sheets))
	for i, s := range w.sheets {
		packed[i] = packedSheet{name: s.name, grid: s.grid()}
	}
	return packWorkbook(w.title, packed)
}

// Name returns the worksheet name.
func (s *Sheet) Name() string { return s.name }

// Set writes value at a 0-based cell. A value whose first character is "="
// is stored as a formula. row and col must fall inside Excel's grid.
func (s *Sheet) Set(row, col int, value string) error {
	if err := checkCell(row, col); err != nil {
		return err
	}
	cell := storedCell{set: true}
	if strings.HasPrefix(value, "=") {
		cell.formula = true
		cell.text = value[1:]
	} else {
		cell.text = value
	}
	s.put(row, col, cell)
	return nil
}

// Get returns the cell text. Formula cells come back with a leading "=".
// ok is false when the cell has never been set.
func (s *Sheet) Get(row, col int) (string, bool) {
	cell, ok := s.at(row, col)
	if !ok || !cell.set {
		return "", false
	}
	if cell.formula {
		return "=" + cell.text, true
	}
	return cell.text, true
}

// SetHeader marks a whole row as a header (bold). Row 0, when a header, is
// frozen in the written worksheet — the same rule the document exporter uses.
func (s *Sheet) SetHeader(row int, header bool) error {
	if row < 0 || row >= maxSheetRows {
		return fmt.Errorf("xlsx: row %d is outside 0..%d", row, maxSheetRows-1)
	}
	if header {
		s.header[row] = true
		if row > s.maxRow {
			s.maxRow = row
		}
		return nil
	}
	delete(s.header, row)
	return nil
}

// Header reports whether row is a header row.
func (s *Sheet) Header(row int) bool { return s.header[row] }

// SetAlign sets a column's alignment to "" (general), "left", "right", or
// "center". "left" is stored but written as Excel's General alignment, which
// already left-aligns text and still right-aligns numbers.
func (s *Sheet) SetAlign(col int, align string) error {
	switch align {
	case "", "left", "right", "center":
	default:
		return fmt.Errorf("xlsx: alignment %q is not left, right, center, or empty", align)
	}
	if col < 0 || col >= maxSheetCols {
		return fmt.Errorf("xlsx: column %d is outside 0..%d", col, maxSheetCols-1)
	}
	if col >= len(s.aligns) {
		next := make([]string, col+1)
		copy(next, s.aligns)
		s.aligns = next
	}
	s.aligns[col] = align
	return nil
}

// Align returns the column alignment, or "" when unset.
func (s *Sheet) Align(col int) string {
	if col < 0 || col >= len(s.aligns) {
		return ""
	}
	return s.aligns[col]
}

func newSheet(name string) *Sheet {
	return &Sheet{
		name:     name,
		cells:    map[int]map[int]storedCell{},
		header:   map[int]bool{},
		rowIDs:   map[int]string{},
		relCells: map[int]map[int][]RelationRef{},
		maxRow:   -1,
		maxCol:   -1,
	}
}

func (s *Sheet) put(row, col int, cell storedCell) {
	rowCells := s.cells[row]
	if rowCells == nil {
		rowCells = map[int]storedCell{}
		s.cells[row] = rowCells
	}
	rowCells[col] = cell
	if row > s.maxRow {
		s.maxRow = row
	}
	if col > s.maxCol {
		s.maxCol = col
	}
}

func (s *Sheet) at(row, col int) (storedCell, bool) {
	rowCells, ok := s.cells[row]
	if !ok {
		return storedCell{}, false
	}
	cell, ok := rowCells[col]
	return cell, ok
}

// grid returns only rows that hold a formula or a non-empty value. The
// bounding box of those cells is not materialized: a value on the last row
// and a value in column XFD are two cells, not the full Excel grid.
func (s *Sheet) grid() sheetRows {
	g := sheetRows{freezeHeader: s.header[0]}
	if len(s.aligns) > 0 {
		g.cols = append([]string(nil), s.aligns...)
	}
	rowNums := make([]int, 0, len(s.cells))
	for r, cols := range s.cells {
		if rowHasContent(cols) {
			rowNums = append(rowNums, r)
		}
	}
	if len(rowNums) == 0 {
		return g
	}
	sort.Ints(rowNums)
	g.rowIndex = rowNums
	for _, r := range rowNums {
		rowCells := s.cells[r]
		width := 0
		for c, cell := range rowCells {
			if !cellOccupied(cell) {
				continue
			}
			if c+1 > width {
				width = c + 1
			}
		}
		cells := make([]string, width)
		formula := make([]bool, width)
		for c, cell := range rowCells {
			if c >= width || !cellOccupied(cell) {
				continue
			}
			cells[c] = cell.text
			formula[c] = cell.formula
		}
		g.rows = append(g.rows, cells)
		g.headerRow = append(g.headerRow, s.header[r])
		g.aligns = append(g.aligns, s.aligns)
		g.formula = append(g.formula, formula)
	}
	return g
}

func cellOccupied(cell storedCell) bool {
	return cell.set && (cell.formula || cell.text != "")
}

func rowHasContent(cols map[int]storedCell) bool {
	for _, cell := range cols {
		if cellOccupied(cell) {
			return true
		}
	}
	return false
}

func (w *Workbook) hasName(name string) bool {
	folded := strings.ToLower(name)
	for _, s := range w.sheets {
		if strings.ToLower(s.name) == folded {
			return true
		}
	}
	return false
}

func validateSheetName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("xlsx: sheet name is empty")
	}
	if utf8.RuneCountInString(name) > maxSheetName {
		return fmt.Errorf("xlsx: sheet name %q exceeds %d characters", name, maxSheetName)
	}
	if strings.ContainsAny(name, `:\/?*[]`) {
		return fmt.Errorf("xlsx: sheet name %q contains a character Excel rejects", name)
	}
	return nil
}

func checkCell(row, col int) error {
	if row < 0 || row >= maxSheetRows {
		return fmt.Errorf("xlsx: row %d is outside 0..%d", row, maxSheetRows-1)
	}
	if col < 0 || col >= maxSheetCols {
		return fmt.Errorf("xlsx: column %d is outside 0..%d", col, maxSheetCols-1)
	}
	return nil
}

type workbookSnapshot struct {
	Schema      string          `json:"schema"`
	Title       string          `json:"title,omitempty"`
	Sheets      []sheetSnapshot `json:"sheets"`
	NamedRanges []NamedRange    `json:"namedRanges,omitempty"`
}

type sheetSnapshot struct {
	Name       string             `json:"name"`
	Aligns     []string           `json:"aligns,omitempty"`
	Columns    []Column           `json:"columns,omitempty"`
	RowIDs     []rowIDSnapshot    `json:"rowIds,omitempty"`
	Rows       []rowSnapshot      `json:"rows,omitempty"`
	HeaderRows []int              `json:"headerRows,omitempty"`
	Cells      []cellSnapshot     `json:"cells,omitempty"`
}

type rowIDSnapshot struct {
	R  int    `json:"r"`
	ID string `json:"id"`
}

type rowSnapshot struct {
	Header bool           `json:"header,omitempty"`
	Cells  []cellSnapshot `json:"cells"`
}

// cellSnapshot is one cell. In v2, r and c are 0-based coordinates and are
// always written, including zero. f is an Excel formula without "=". v is
// the literal text. A formula cell omits v. v1 snapshots omit r and c; the
// array position is the coordinate.
type cellSnapshot struct {
	R    int           `json:"r"`
	C    int           `json:"c"`
	V    string        `json:"v,omitempty"`
	F    string        `json:"f,omitempty"`
	Rels []RelationRef `json:"rels,omitempty"`
}

// MarshalJSON writes the editing snapshot (WorkbookSchema). Only occupied
// cells are written, each with its own row and column.
func (w *Workbook) MarshalJSON() ([]byte, error) {
	if w == nil {
		return nil, fmt.Errorf("xlsx: workbook is nil")
	}
	snap := workbookSnapshot{Schema: WorkbookSchema, Title: w.title, Sheets: []sheetSnapshot{}, NamedRanges: append([]NamedRange{}, w.namedRanges...)}
	for _, s := range w.sheets {
		snap.Sheets = append(snap.Sheets, s.snapshot())
	}
	return json.Marshal(snap)
}

// ParseWorkbook reads a v3 snapshot, and still reads v2 and v1.
func ParseWorkbook(b []byte) (*Workbook, error) {
	var snap workbookSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, fmt.Errorf("xlsx: workbook json: %w", err)
	}
	switch snap.Schema {
	case WorkbookSchema, workbookSchemaV2, workbookSchemaV1:
	default:
		return nil, fmt.Errorf("xlsx: workbook schema %q, want %s, %s, or %s", snap.Schema, WorkbookSchema, workbookSchemaV2, workbookSchemaV1)
	}
	wb := NewWorkbook(snap.Title)
	wb.namedRanges = append([]NamedRange{}, snap.NamedRanges...)
	for _, sh := range snap.Sheets {
		sheet, err := wb.AddSheet(sh.Name)
		if err != nil {
			return nil, err
		}
		sheet.columns = append([]Column{}, sh.Columns...)
		for _, rid := range sh.RowIDs {
			sheet.rowIDs[rid.R] = rid.ID
		}
		for col, align := range sh.Aligns {
			if align == "" {
				continue
			}
			if err := sheet.SetAlign(col, align); err != nil {
				return nil, err
			}
		}
		if snap.Schema == workbookSchemaV1 {
			if err := applyV1Rows(sheet, sh.Rows); err != nil {
				return nil, err
			}
			continue
		}
		for _, row := range sh.HeaderRows {
			if err := sheet.SetHeader(row, true); err != nil {
				return nil, err
			}
		}
		for _, cell := range sh.Cells {
			if err := applyCell(sheet, cell.R, cell.C, cell); err != nil {
				return nil, err
			}
		}
	}
	return wb, nil
}

func applyV1Rows(sheet *Sheet, rows []rowSnapshot) error {
	for r, row := range rows {
		if row.Header {
			if err := sheet.SetHeader(r, true); err != nil {
				return err
			}
		}
		for c, cell := range row.Cells {
			if err := applyCell(sheet, r, c, cell); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyCell(sheet *Sheet, row, col int, cell cellSnapshot) error {
	if cell.F != "" && cell.V != "" {
		return fmt.Errorf("xlsx: cell %d,%d has both v and f", row, col)
	}
	if cell.F != "" {
		if err := sheet.Set(row, col, "="+cell.F); err != nil {
			return err
		}
	} else if cell.V != "" {
		if err := sheet.Set(row, col, cell.V); err != nil {
			return err
		}
	}
	if len(cell.Rels) > 0 {
		return sheet.SetRelations(row, col, cell.Rels)
	}
	return nil
}

func (s *Sheet) snapshot() sheetSnapshot {
	out := sheetSnapshot{Name: s.name, Cells: []cellSnapshot{}, Columns: append([]Column{}, s.columns...)}
	trimmed := trimAligns(s.aligns)
	if len(trimmed) > 0 {
		out.Aligns = trimmed
	}
	for row, header := range s.header {
		if header {
			out.HeaderRows = append(out.HeaderRows, row)
		}
	}
	sort.Ints(out.HeaderRows)
	for row, id := range s.rowIDs {
		if id != "" {
			out.RowIDs = append(out.RowIDs, rowIDSnapshot{R: row, ID: id})
		}
	}
	sort.Slice(out.RowIDs, func(i, j int) bool { return out.RowIDs[i].R < out.RowIDs[j].R })
	type addr struct{ row, col int }
	var addrs []addr
	seen := map[addr]bool{}
	for row, cols := range s.cells {
		for col, cell := range cols {
			if cellOccupied(cell) {
				a := addr{row, col}
				addrs = append(addrs, a)
				seen[a] = true
			}
		}
	}
	for row, cols := range s.relCells {
		for col, refs := range cols {
			if len(refs) == 0 {
				continue
			}
			a := addr{row, col}
			if !seen[a] {
				addrs = append(addrs, a)
				seen[a] = true
			}
		}
	}
	sort.Slice(addrs, func(i, j int) bool {
		if addrs[i].row != addrs[j].row {
			return addrs[i].row < addrs[j].row
		}
		return addrs[i].col < addrs[j].col
	})
	for _, a := range addrs {
		cell, _ := s.at(a.row, a.col)
		snap := cellSnapshot{R: a.row, C: a.col}
		if cell.set {
			if cell.formula {
				snap.F = cell.text
			} else {
				snap.V = cell.text
			}
		}
		if refs := s.relCells[a.row][a.col]; len(refs) > 0 {
			snap.Rels = append([]RelationRef{}, refs...)
		}
		out.Cells = append(out.Cells, snap)
	}
	return out
}

func trimAligns(aligns []string) []string {
	end := len(aligns)
	for end > 0 && aligns[end-1] == "" {
		end--
	}
	if end == 0 {
		return nil
	}
	return aligns[:end]
}
