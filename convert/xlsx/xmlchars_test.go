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

// Issue #39 (Refs aodex#1239): no XML-illegal character may reach any part.
//
// TEST LIST
//
//	X1 an inline-string cell with C0 controls, U+FFFE/U+FFFF and invalid
//	   UTF-8 (a lone surrogate) yields strict-valid parts; the legal text
//	   around them survives
//	X2 a formula cell with illegal characters
//	X3 a sheet name (from a section heading) with illegal characters
//	X4 a sheet name made ONLY of illegal characters falls back to SheetN
//	X5 the dc:title core property (Options.Title)
//	X6 Workbook.AddSheet / RenameSheet reject a name with illegal characters
//	X7 tab, LF and CR in a cell are preserved
//	X8 a cell made only of illegal characters
//
// Not applicable: this writer emits no shared-string table, comments,
// headers/footers, or user text in docProps/app.xml.

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/internal/xmltext/xmltexttest"
)

const illegalXML = xmltexttest.Illegal

var assertStrictParts = xmltexttest.AssertStrictParts

func illegalTableDoc(heading string, headers []string, rows [][]string) *model.Document {
	blocks := []model.Block{}
	if heading != "" {
		blocks = append(blocks, model.Block{Kind: model.BlockHeading, Level: 2, Text: heading})
	}
	blocks = append(blocks, model.Block{Kind: model.BlockTable, Headers: headers, Rows: rows})
	return &model.Document{Sections: []model.Section{{Blocks: blocks}}}
}

func TestXMLIllegal_X1_InlineStringCell(t *testing.T) {
	doc := illegalTableDoc("", []string{"Vendor"}, [][]string{{"red" + illegalXML + " & ok"}})
	pkg, err := ToXLSX(doc, Options{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	parts := assertStrictParts(t, pkg)
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], "red &amp; ok") {
		t.Errorf("legal text around the illegal characters was lost:\n%s", parts["xl/worksheets/sheet1.xml"])
	}
}

// sheetNames pulls the name="..." attributes out of xl/workbook.xml.
func sheetNames(t *testing.T, workbookXML string) []string {
	t.Helper()
	var names []string
	rest := workbookXML
	for {
		i := strings.Index(rest, `<sheet name="`)
		if i < 0 {
			return names
		}
		rest = rest[i+len(`<sheet name="`):]
		j := strings.Index(rest, `"`)
		names = append(names, rest[:j])
		rest = rest[j:]
	}
}

func TestXMLIllegal_X3_SheetNameFromHeading(t *testing.T) {
	table := model.Block{Kind: model.BlockTable, Headers: []string{"h"}, Rows: [][]string{{"1"}}}
	doc := &model.Document{Sections: []model.Section{
		{Blocks: []model.Block{{Kind: model.BlockHeading, Level: 2, Text: "Q3" + illegalXML + " Plan"}, table}},
		// Differs from the first only by the illegal characters: once they
		// are dropped the two names collide, and Excel rejects duplicates.
		{Blocks: []model.Block{{Kind: model.BlockHeading, Level: 2, Text: "Q3 Plan"}, table}},
	}}
	pkg, err := ToXLSX(doc, Options{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	parts := assertStrictParts(t, pkg)
	got := sheetNames(t, parts["xl/workbook.xml"])
	want := []string{"Q3 Plan", "Q3 Plan (2)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("sheet names = %q, want %q", got, want)
	}
}

func TestXMLIllegal_X4_SheetNameOnlyIllegalFallsBack(t *testing.T) {
	doc := illegalTableDoc(illegalXML, []string{"h"}, [][]string{{"1"}})
	pkg, err := ToXLSX(doc, Options{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	parts := assertStrictParts(t, pkg)
	if got := sheetNames(t, parts["xl/workbook.xml"]); len(got) != 1 || got[0] != "Sheet1" {
		t.Errorf("sheet names = %q, want [Sheet1]", got)
	}
}

func TestXMLIllegal_X5_CoreTitle(t *testing.T) {
	doc := illegalTableDoc("", []string{"h"}, [][]string{{"1"}})
	pkg, err := ToXLSX(doc, Options{Title: "Q3" + illegalXML + " & review"})
	if err != nil {
		t.Fatal(err)
	}
	parts := assertStrictParts(t, pkg)
	if !strings.Contains(parts["docProps/core.xml"], "<dc:title>Q3 &amp; review</dc:title>") {
		t.Errorf("core.xml title:\n%s", parts["docProps/core.xml"])
	}
}

// X6: the Workbook API stores names verbatim, so dropping illegal
// characters at write time could empty a name or collide two. It rejects
// them up front instead, like the other characters Excel refuses.
func TestXMLIllegal_X6_WorkbookRejectsIllegalSheetName(t *testing.T) {
	w := NewWorkbook("t")
	if _, err := w.AddSheet("Plan" + "\x0b"); err == nil {
		t.Error("AddSheet accepted a name with U+000B")
	}
	if _, err := w.AddSheet("\xed\xa0\x80"); err == nil {
		t.Error("AddSheet accepted a name of invalid UTF-8")
	}
	if _, err := w.AddSheet("Plan"); err != nil {
		t.Fatal(err)
	}
	if err := w.RenameSheet("Plan", "Plan\x00"); err == nil {
		t.Error("RenameSheet accepted a name with U+0000")
	}
}

func TestXMLIllegal_X2_FormulaCell(t *testing.T) {
	w := NewWorkbook("t")
	s, err := w.AddSheet("Plan")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(0, 0, "=SUM(A2:A3)"+illegalXML+"&\"x\""); err != nil {
		t.Fatal(err)
	}
	pkg, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parts := assertStrictParts(t, pkg)
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], "<f>SUM(A2:A3)&amp;&quot;x&quot;</f>") {
		t.Errorf("formula:\n%s", parts["xl/worksheets/sheet1.xml"])
	}
}

func TestXMLIllegal_X7_TabLFCRPreserved(t *testing.T) {
	doc := illegalTableDoc("", []string{"h"}, [][]string{{"a\tb\nc\rd" + illegalXML}})
	pkg, err := ToXLSX(doc, Options{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	parts := assertStrictParts(t, pkg)
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], ">a\tb\nc\rd</t>") {
		t.Errorf("tab/LF/CR not preserved:\n%q", parts["xl/worksheets/sheet1.xml"])
	}
}

func TestXMLIllegal_X8_CellOnlyIllegal(t *testing.T) {
	doc := illegalTableDoc("", []string{"h", illegalXML}, [][]string{{illegalXML, "x"}})
	pkg, err := ToXLSX(doc, Options{Title: illegalXML})
	if err != nil {
		t.Fatal(err)
	}
	assertStrictParts(t, pkg)
}

// X9: a snapshot stored before #39 may carry a sheet name with an
// XML-illegal character. Loading it must not fail -- that would make the
// user's workbook unopenable. The name is cleaned like sheetName() cleans a
// heading: illegal runes dropped, empty falls back to SheetN, a collision
// gets a " (k)" suffix, and named ranges follow the rename.
func TestXMLIllegal_X9_ParseWorkbookCleansLegacySheetNames(t *testing.T) {
	sheets := func(schema string) string {
		cells := `"cells":[{"r":0,"c":0,"v":"x"}]`
		if schema == "eden-press.workbook/v1" {
			cells = `"rows":[{"cells":[{"r":0,"c":0,"v":"x"}]}]`
		}
		return `[` +
			`{"name":"Plan\u000b",` + cells + `},` +
			`{"name":"\u0000\u000b￿",` + cells + `},` +
			`{"name":"Plan",` + cells + `}]`
	}
	for _, schema := range []string{"eden-press.workbook/v1", "eden-press.workbook/v2", "eden-press.workbook/v3"} {
		t.Run(schema, func(t *testing.T) {
			js := `{"schema":"` + schema + `","title":"t\u000c","sheets":` + sheets(schema) +
				`,"namedRanges":[{"id":"n1","name":"r","sheet":"Plan\u000b","r0":0,"r1":0,"c0":0,"c1":0}]}`
			wb, err := ParseWorkbook([]byte(js))
			if err != nil {
				t.Fatalf("legacy snapshot no longer loads: %v", err)
			}
			got := strings.Join(wb.SheetNames(), "|")
			if want := "Plan (2)|Sheet2|Plan"; got != want {
				t.Errorf("sheet names = %q, want %q", got, want)
			}
			if wb.namedRanges[0].Sheet != "Plan (2)" {
				t.Errorf("named range sheet = %q, want it to follow the rename", wb.namedRanges[0].Sheet)
			}
			if v, _ := wb.sheets[0].Get(0, 0); v != "x" {
				t.Errorf("cell lost: %q", v)
			}
			pkg, err := wb.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			assertStrictParts(t, pkg)
			// And the cleaned workbook round-trips through JSON unchanged.
			again, err := wb.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			wb2, err := ParseWorkbook(again)
			if err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			if got2 := strings.Join(wb2.SheetNames(), "|"); got2 != got {
				t.Errorf("round-trip names = %q, want %q", got2, got)
			}
		})
	}
}
