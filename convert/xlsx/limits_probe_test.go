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
	"strings"
	"testing"
)

func TestCellAddressBounds(t *testing.T) {
	wb := NewWorkbook("bounds")
	sheet, err := wb.AddSheet("Grid")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		row, col int
		ok       bool
	}{
		{0, 0, true},
		{maxSheetRows - 1, 0, true},
		{0, maxSheetCols - 1, true},
		{maxSheetRows - 1, maxSheetCols - 1, true},
		{-1, 0, false},
		{0, -1, false},
		{maxSheetRows, 0, false},
		{0, maxSheetCols, false},
		{maxSheetRows, maxSheetCols, false},
	}
	for _, tc := range cases {
		err := sheet.Set(tc.row, tc.col, "1")
		if tc.ok && err != nil {
			t.Errorf("Set(%d,%d) rejected a cell inside 0..%d x 0..%d: %v", tc.row, tc.col, maxSheetRows-1, maxSheetCols-1, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("Set(%d,%d) accepted a cell outside the grid", tc.row, tc.col)
		}
	}
	if got := columnRef(maxSheetCols - 1); got != "XFD" {
		t.Errorf("columnRef(%d) = %q, want XFD", maxSheetCols-1, got)
	}
	if err := sheet.SetHeader(maxSheetRows, true); err == nil {
		t.Error("SetHeader accepted a row past the last Excel row")
	}
	if err := sheet.SetAlign(maxSheetCols, "right"); err == nil {
		t.Error("SetAlign accepted a column past XFD")
	}
	// Occupied cells are stored sparsely. Bytes writes those cells, not the
	// rectangle between them. The export is covered by TestSparseCornerExport.
}

func TestSparseCornerExport(t *testing.T) {
	wb := NewWorkbook("corners")
	sheet, err := wb.AddSheet("Grid")
	if err != nil {
		t.Fatal(err)
	}
	if err := sheet.Set(maxSheetRows-1, 0, "9"); err != nil {
		t.Fatal(err)
	}
	if err := sheet.Set(0, maxSheetCols-1, "8"); err != nil {
		t.Fatal(err)
	}
	if err := sheet.SetAlign(2, "right"); err != nil {
		t.Fatal(err)
	}

	pkg, err := wb.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg) >= 64*1024 {
		t.Fatalf("xlsx is %d bytes, want under 64KB for two occupied cells", len(pkg))
	}
	xml := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	if got := strings.Count(xml, "<row "); got != 2 {
		t.Fatalf("sheet has %d row tags, want 2:\n%s", got, xml)
	}
	if !strings.Contains(xml, `<c r="A1048576"`) || !strings.Contains(xml, ">9<") {
		t.Fatalf("last row cell missing:\n%s", xml)
	}
	if !strings.Contains(xml, `<c r="XFD1"`) || !strings.Contains(xml, ">8<") {
		t.Fatalf("XFD cell missing:\n%s", xml)
	}
	if !strings.Contains(xml, `<dimension ref="A1:XFD1048576"/>`) {
		t.Fatalf("dimension = occupied bounds missing:\n%s", xml)
	}
	if !strings.Contains(xml, `<col min="3" max="3" style="2"/>`) {
		t.Fatalf("column alignment missing:\n%s", xml)
	}

	opened, err := Open(pkg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := opened.Sheet("Grid")
	if !ok {
		t.Fatalf("sheets = %v", opened.SheetNames())
	}
	if v, ok := got.Get(maxSheetRows-1, 0); !ok || v != "9" {
		t.Fatalf("last row = %q ok=%v", v, ok)
	}
	if v, ok := got.Get(0, maxSheetCols-1); !ok || v != "8" {
		t.Fatalf("XFD = %q ok=%v", v, ok)
	}
	if got.Align(2) != "right" {
		t.Fatalf("align = %q", got.Align(2))
	}
}
