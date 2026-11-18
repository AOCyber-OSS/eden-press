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

package press

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"go.aocyber.ai/eden-press/convert/xlsx"
)

// ImportWorkbookXLSXJSON is the public door for user uploads, so the zip-bomb
// refusal must reach its callers as a typed error they can match (eden-press#43).
func TestImportWorkbookXLSXJSONRefusesZipBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("xl/workbook.xml", `<workbook><sheets><sheet name="S" sheetId="1" r:id="rId1" xmlns:r="r"/></sheets></workbook>`)
	write("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	w, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte{' '}, 1<<20)
	for n := 0; n < 8; n++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = ImportWorkbookXLSXJSON(buf.Bytes())
	if !errors.Is(err, xlsx.ErrLimitExceeded) {
		t.Fatalf("ImportWorkbookXLSXJSON(%d-byte bomb) err = %v, want errors.Is xlsx.ErrLimitExceeded", buf.Len(), err)
	}
}

// A caller with its own upload policy passes stricter limits through press.
func TestImportWorkbookXLSXJSONWithLimits(t *testing.T) {
	wb := xlsx.NewWorkbook("t")
	sheet, err := wb.AddSheet("S")
	if err != nil {
		t.Fatal(err)
	}
	for c := 0; c < 6; c++ {
		if err := sheet.Set(0, c, "1"); err != nil {
			t.Fatal(err)
		}
	}
	pkg, err := wb.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportWorkbookXLSXJSONWithLimits(pkg, xlsx.Limits{}); err != nil {
		t.Fatalf("default limits refused a 6-cell workbook: %v", err)
	}
	_, err = ImportWorkbookXLSXJSONWithLimits(pkg, xlsx.Limits{Cells: 5})
	var le *xlsx.LimitError
	if !errors.As(err, &le) || le.Limit != "cells" || le.Value != 5 {
		t.Fatalf("ImportWorkbookXLSXJSONWithLimits(Cells: 5) err = %v, want a cells LimitError of 5", err)
	}
}
