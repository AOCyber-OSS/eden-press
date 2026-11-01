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
	"bytes"
	"strings"
	"testing"
)

func TestWorkbookBuildEditAndRoundTrip(t *testing.T) {
	wb := NewWorkbook("Q3")
	metrics, err := wb.AddSheet("Metrics")
	if err != nil {
		t.Fatalf("AddSheet: %v", err)
	}
	notes, err := wb.AddSheet("Notes")
	if err != nil {
		t.Fatalf("AddSheet notes: %v", err)
	}
	if err := metrics.SetHeader(0, true); err != nil {
		t.Fatalf("SetHeader: %v", err)
	}
	for col, v := range []string{"Metric", "Q3"} {
		if err := metrics.Set(0, col, v); err != nil {
			t.Fatalf("Set header: %v", err)
		}
	}
	if err := metrics.SetAlign(1, "right"); err != nil {
		t.Fatalf("SetAlign: %v", err)
	}
	if err := metrics.Set(1, 0, "p95"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := metrics.Set(1, 1, "550"); err != nil {
		t.Fatalf("Set number: %v", err)
	}
	if err := metrics.Set(2, 0, "id"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := metrics.Set(2, 1, "007"); err != nil {
		t.Fatalf("Set identifier: %v", err)
	}
	if err := metrics.Set(3, 1, "=SUM(B2:B2)"); err != nil {
		t.Fatalf("Set formula: %v", err)
	}
	if err := notes.Set(0, 0, "keep"); err != nil {
		t.Fatalf("Set notes: %v", err)
	}

	if _, err := wb.AddSheet("metrics"); err == nil {
		t.Fatal("AddSheet duplicate case-insensitive name succeeded")
	}
	if err := wb.RenameSheet("Notes", "Notes/2"); err == nil {
		t.Fatal("RenameSheet accepted a slash")
	}
	if err := wb.RenameSheet("Notes", "Scratch"); err != nil {
		t.Fatalf("RenameSheet: %v", err)
	}
	lone := NewWorkbook("lone")
	if _, err := lone.AddSheet("Only"); err != nil {
		t.Fatalf("AddSheet Only: %v", err)
	}
	if err := lone.DeleteSheet("Only"); err == nil {
		t.Fatal("DeleteSheet removed the last sheet")
	}
	if _, err := lone.AddSheet("Extra"); err != nil {
		t.Fatalf("AddSheet Extra: %v", err)
	}
	if err := lone.DeleteSheet("Only"); err != nil {
		t.Fatalf("DeleteSheet: %v", err)
	}

	a, err := wb.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	b, err := wb.Bytes()
	if err != nil {
		t.Fatalf("Bytes again: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("Bytes is not deterministic")
	}
	sheetXML := ""
	for _, name := range partNames(t, a) {
		if !strings.HasPrefix(name, "xl/worksheets/sheet") {
			continue
		}
		xml := readPart(t, a, name)
		if strings.Contains(xml, "<f>SUM(B2:B2)</f>") {
			sheetXML = xml
			break
		}
	}
	if sheetXML == "" {
		t.Fatal("formula not written as a formula cell")
	}
	wbXML := readPart(t, a, "xl/workbook.xml")
	if !strings.Contains(wbXML, `fullCalcOnLoad="1"`) {
		t.Fatalf("workbook with formulas missing calcPr fullCalcOnLoad: %s", wbXML)
	}
	if strings.Contains(sheetXML, ">007<") && strings.Contains(sheetXML, "<v>007</v>") {
		t.Fatal("identifier 007 was stored as a number")
	}
	if !strings.Contains(sheetXML, ">007<") {
		t.Fatalf("identifier 007 missing from sheet:\n%s", sheetXML)
	}

	opened, err := Open(a)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened.Title() != "Q3" {
		t.Fatalf("title = %q", opened.Title())
	}
	if got := opened.SheetNames(); len(got) != 2 || got[0] != "Metrics" || got[1] != "Scratch" {
		t.Fatalf("sheets = %v", got)
	}
	scratch, _ := opened.Sheet("Scratch")
	if v, ok := scratch.Get(0, 0); !ok || v != "keep" {
		t.Fatalf("Scratch!A1 = %q ok=%v", v, ok)
	}
	again, _ := opened.Sheet("Metrics")
	if !again.Header(0) {
		t.Fatal("header row was not restored")
	}
	if again.Align(1) != "right" {
		t.Fatalf("align = %q", again.Align(1))
	}
	if v, _ := again.Get(1, 1); v != "550" {
		t.Fatalf("Q3 value = %q", v)
	}
	if v, _ := again.Get(2, 1); v != "007" {
		t.Fatalf("identifier = %q", v)
	}
	if v, _ := again.Get(3, 1); v != "=SUM(B2:B2)" {
		t.Fatalf("formula = %q", v)
	}
}

func TestWorkbookJSONSnapshot(t *testing.T) {
	wb := NewWorkbook("Budget")
	sheet, err := wb.AddSheet("FY")
	if err != nil {
		t.Fatalf("AddSheet: %v", err)
	}
	if err := sheet.Set(0, 0, "100"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := sheet.Set(0, 1, "=A1*2"); err != nil {
		t.Fatalf("Set formula: %v", err)
	}
	raw, err := wb.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	parsed, err := ParseWorkbook(raw)
	if err != nil {
		t.Fatalf("ParseWorkbook: %v", err)
	}
	if parsed.Title() != "Budget" || parsed.SheetNames()[0] != "FY" {
		t.Fatalf("parsed header = %q %v", parsed.Title(), parsed.SheetNames())
	}
	fy, _ := parsed.Sheet("FY")
	if v, _ := fy.Get(0, 0); v != "100" {
		t.Fatalf("value = %q", v)
	}
	if v, _ := fy.Get(0, 1); v != "=A1*2" {
		t.Fatalf("formula = %q", v)
	}
	if _, err := ParseWorkbook([]byte(`{"schema":"nope","sheets":[]}`)); err == nil {
		t.Fatal("ParseWorkbook accepted an unknown schema")
	}
	if !bytes.Contains(raw, []byte(WorkbookSchema)) {
		t.Fatalf("snapshot schema = %s", raw)
	}
}

func TestSparseSnapshotAndV1Read(t *testing.T) {
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
	if err := sheet.SetHeader(0, true); err != nil {
		t.Fatal(err)
	}
	raw, err := wb.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) >= 64*1024 {
		t.Fatalf("snapshot is %d bytes, want under 64KB for two occupied cells", len(raw))
	}
	if bytes.Contains(raw, []byte(`"rows"`)) {
		t.Fatalf("v2 snapshot still uses rectangular rows: %s", raw)
	}
	parsed, err := ParseWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parsed.Sheet("Grid")
	if !ok {
		t.Fatal("Grid missing")
	}
	if v, ok := got.Get(maxSheetRows-1, 0); !ok || v != "9" {
		t.Fatalf("last row = %q ok=%v", v, ok)
	}
	if v, ok := got.Get(0, maxSheetCols-1); !ok || v != "8" {
		t.Fatalf("XFD = %q ok=%v", v, ok)
	}
	if !got.Header(0) {
		t.Fatal("header row was not restored")
	}

	v1 := []byte(`{"schema":"eden-press.workbook/v1","title":"Old","sheets":[{"name":"FY","aligns":["","right"],"rows":[{"header":true,"cells":[{"v":"A"},{"v":"B"}]},{"cells":[{},{"f":"A1"}]}]}]}`)
	old, err := ParseWorkbook(v1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Title() != "Old" {
		t.Fatalf("title = %q", old.Title())
	}
	fy, ok := old.Sheet("FY")
	if !ok {
		t.Fatal("FY missing")
	}
	if !fy.Header(0) {
		t.Fatal("v1 header was not restored")
	}
	if fy.Align(1) != "right" {
		t.Fatalf("v1 align = %q", fy.Align(1))
	}
	if v, _ := fy.Get(0, 0); v != "A" {
		t.Fatalf("v1 A1 = %q", v)
	}
	if v, _ := fy.Get(1, 1); v != "=A1" {
		t.Fatalf("v1 B2 = %q", v)
	}
}

func TestOpenDocumentExport(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{Title: "Metrics"})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	wb, err := Open(pkg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if wb.Title() != "Metrics" {
		t.Fatalf("title = %q", wb.Title())
	}
	sheet, ok := wb.Sheet("Sheet1")
	if !ok {
		t.Fatalf("sheets = %v", wb.SheetNames())
	}
	if v, _ := sheet.Get(0, 2); v != "Q3" || !sheet.Header(0) {
		t.Fatalf("header Q3 = %q header=%v", v, sheet.Header(0))
	}
	if v, _ := sheet.Get(1, 2); v != "550" {
		t.Fatalf("550 cell = %q", v)
	}
	if v, _ := sheet.Get(2, 1); v != "1.2" {
		t.Fatalf("1.2 cell = %q", v)
	}
}

func TestParseCellRefMatchesColumnRef(t *testing.T) {
	for i := 0; i <= 60; i++ {
		col, row, ok := parseCellRef(columnRef(i) + "1")
		if !ok || col != i || row != 0 {
			t.Fatalf("parseCellRef(%s1) = %d,%d ok=%v", columnRef(i), col, row, ok)
		}
	}
}
