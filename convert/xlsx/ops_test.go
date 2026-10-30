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
	"testing"
)

func TestApplyWorkbookOpsAndRelations(t *testing.T) {
	wb := NewWorkbook("T")
	if _, err := wb.AddSheet("Tasks"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyWorkbookOps(wb, []WbOp{
		{Op: WbOpSetCell, Sheet: "Tasks", Row: 0, Column: 0, Value: "A"},
		{Op: WbOpSetCell, Sheet: "Tasks", Row: 1, Column: 0, Value: "B"},
		{Op: WbOpEnsureRowID, Sheet: "Tasks", Row: 0, RowID: "r_a"},
		{Op: WbOpEnsureRowID, Sheet: "Tasks", Row: 1, RowID: "r_b"},
		{Op: WbOpSetColumn, Sheet: "Tasks", ColumnID: "rel", Name: "Related", Type: string(ColumnRelation)},
		{Op: WbOpSetRelation, Sheet: "Tasks", RowID: "r_a", ColumnID: "rel", Rels: []RelationRef{{RowID: "r_b"}}},
		{Op: WbOpAddSheet, Sheet: "Extra"},
		{Op: WbOpRenameSheet, Sheet: "Extra", NewName: "More"},
		{Op: WbOpSetHeaderRow, Sheet: "Tasks", Row: 0, Header: true},
		{Op: WbOpSetAlign, Sheet: "Tasks", Column: 0, Align: "left"},
	}); err != nil {
		t.Fatal(err)
	}
	s, _ := wb.Sheet("Tasks")
	if s.RowID(0) != "r_a" {
		t.Fatalf("row id = %q", s.RowID(0))
	}
	refs := s.Relations(0, 0)
	if len(refs) != 1 || refs[0].RowID != "r_b" {
		t.Fatalf("rels = %+v", refs)
	}
	if err := ApplyWorkbookOps(wb, []WbOp{
		{Op: WbOpClearRelation, Sheet: "Tasks", RowID: "r_a", ColumnID: "rel"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(s.Relations(0, 0)) != 0 {
		t.Fatalf("clear failed: %+v", s.Relations(0, 0))
	}
	raw, err := wb.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("invalid json")
	}
	got, err := ParseWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	gs, _ := got.Sheet("Tasks")
	if len(gs.Columns()) != 1 || gs.Columns()[0].Type != ColumnRelation {
		t.Fatalf("columns = %+v", gs.Columns())
	}
	if _, ok := got.Sheet("More"); !ok {
		t.Fatal("renamed sheet missing")
	}
}
