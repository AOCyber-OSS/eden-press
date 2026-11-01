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
	"strings"
	"testing"
)

func TestWorkbookV7ColumnTypesAndNamedRangesRoundTrip(t *testing.T) {
	wb := NewWorkbook("Props")
	if _, err := wb.AddSheet("Data"); err != nil {
		t.Fatal(err)
	}
	types := []struct {
		id, name string
		typ      ColumnType
		opts     []string
	}{
		{"t", "Text", ColumnText, nil},
		{"n", "Num", ColumnNumber, nil},
		{"f", "Form", ColumnFormula, nil},
		{"r", "Rel", ColumnRelation, nil},
		{"s", "Sel", ColumnSelect, []string{"a", "b"}},
		{"ms", "Multi", ColumnMultiSelect, []string{"x", "y"}},
		{"d", "Date", ColumnDate, nil},
		{"c", "Check", ColumnCheckbox, nil},
		{"u", "URL", ColumnURL, nil},
		{"e", "Email", ColumnEmail, nil},
	}
	ops := []WbOp{
		{Op: WbOpSetCell, Sheet: "Data", Row: 0, Column: 0, Value: "row"},
		{Op: WbOpEnsureRowID, Sheet: "Data", Row: 0, RowID: "r_1"},
	}
	for _, col := range types {
		ops = append(ops, WbOp{Op: WbOpSetColumn, Sheet: "Data", ColumnID: col.id, Name: col.name, Type: string(col.typ), Options: col.opts})
	}
	ops = append(ops,
		WbOp{Op: WbOpSetNamedRange, RangeID: "nr_top", Name: "Top", Sheet: "Data", R0: 0, R1: 5, C0: 0, C1: 2},
		WbOp{Op: WbOpSetNamedRange, RangeID: "nr_top", Name: "TopUpdated", Sheet: "Data", R0: 0, R1: 3, C0: 0, C1: 1}, // upsert
	)
	if err := ApplyWorkbookOps(wb, ops); err != nil {
		t.Fatal(err)
	}
	if err := ApplyWorkbookOps(wb, []WbOp{{Op: WbOpSetColumn, Sheet: "Data", ColumnID: "bad", Name: "Bad", Type: "nope"}}); err == nil {
		t.Fatal("expected unknown column type")
	}
	if err := ApplyWorkbookOps(wb, []WbOp{{Op: WbOpSetNamedRange, RangeID: "x", Name: "X", Sheet: "Nope"}}); err == nil {
		t.Fatal("expected missing sheet")
	}

	raw, err := wb.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	if probe["schema"] != WorkbookSchema {
		t.Fatalf("schema = %v", probe["schema"])
	}
	nr, ok := probe["namedRanges"].([]any)
	if !ok || len(nr) != 1 {
		t.Fatalf("namedRanges = %v", probe["namedRanges"])
	}
	entry := nr[0].(map[string]any)
	if entry["name"] != "TopUpdated" || entry["id"] != "nr_top" {
		t.Fatalf("named range upsert: %+v", entry)
	}

	got, err := ParseWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := got.Sheet("Data")
	cols := s.Columns()
	if len(cols) != len(types) {
		t.Fatalf("cols = %d want %d (%+v)", len(cols), len(types), cols)
	}
	byID := map[string]Column{}
	for _, c := range cols {
		byID[c.ID] = c
	}
	if byID["s"].Type != ColumnSelect || len(byID["s"].Options) != 2 {
		t.Fatalf("select col: %+v", byID["s"])
	}
	if byID["ms"].Type != ColumnMultiSelect || byID["d"].Type != ColumnDate || byID["c"].Type != ColumnCheckbox {
		t.Fatalf("typed cols: %+v", byID)
	}
	if len(got.namedRanges) != 1 || got.namedRanges[0].Name != "TopUpdated" || got.namedRanges[0].R1 != 3 {
		t.Fatalf("parsed namedRanges: %+v", got.namedRanges)
	}
	if !strings.Contains(string(raw), `"options"`) {
		t.Fatalf("options missing from json: %s", raw)
	}
}
