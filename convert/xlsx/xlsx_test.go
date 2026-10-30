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
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/conformance/corpus/xlsxtyping"
)

func readPart(t *testing.T, pkg []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(b)
	}
	t.Fatalf("part %q not present; have %v", name, partNames(t, pkg))
	return ""
}

func partNames(t *testing.T, pkg []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

func tableDoc(blocks ...model.Block) *model.Document {
	return &model.Document{
		SchemaVersion: model.SchemaVersion,
		Sections:      []model.Section{{ID: 1, Blocks: blocks}},
	}
}

var metricsTable = model.Block{
	Kind:    model.BlockTable,
	Headers: []string{"Metric", "Q2", "Q3"},
	Rows: [][]string{
		{"p95 latency", "840", "550"},
		{"Error rate", "1.2", "0.4"},
	},
}

// TestToXLSXNilDoc: a nil document is a caller error, not a panic.
func TestToXLSXNilDoc(t *testing.T) {
	if _, err := ToXLSX(nil, Options{}); err == nil {
		t.Fatal("ToXLSX(nil) = nil error, want an error")
	}
}

// TestPackageGraph: the minimal OPC graph Excel requires is present and every
// part is declared in [Content_Types].xml.
func TestPackageGraph(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	for _, want := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/worksheets/sheet1.xml",
		"xl/styles.xml",
		"docProps/core.xml",
		"docProps/app.xml",
	} {
		found := false
		for _, got := range partNames(t, pkg) {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required part %q (have %v)", want, partNames(t, pkg))
		}
	}
	ct := readPart(t, pkg, "[Content_Types].xml")
	for _, want := range []string{"/xl/workbook.xml", "/xl/worksheets/sheet1.xml", "/xl/styles.xml"} {
		if !strings.Contains(ct, want) {
			t.Errorf("[Content_Types].xml has no Override for %q: %s", want, ct)
		}
	}
}

// TestDeterminism: same document twice, byte-identical package.
func TestDeterminism(t *testing.T) {
	d := tableDoc(metricsTable)
	a, err := ToXLSX(d, Options{})
	if err != nil {
		t.Fatalf("ToXLSX #1: %v", err)
	}
	b, err := ToXLSX(d, Options{})
	if err != nil {
		t.Fatalf("ToXLSX #2: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("ToXLSX is not deterministic: %d vs %d bytes", len(a), len(b))
	}
}

// TestTableToCells: a table's header and body land in the right cells, with A1
// references that match their row/column position.
func TestTableToCells(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")

	for _, want := range []string{`r="A1"`, `r="B1"`, `r="C1"`, `r="A2"`, `r="C3"`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("missing cell reference %s: %s", want, sheet)
		}
	}
	for _, want := range []string{"Metric", "Q2", "Q3", "p95 latency", "Error rate"} {
		if !strings.Contains(sheet, want) {
			t.Errorf("missing cell text %q: %s", want, sheet)
		}
	}
}

// TestNumericCellsAreNumbers is the point of exporting a spreadsheet at all: a
// workbook whose numbers are text cannot be summed, charted or sorted. Numeric
// cell values must be emitted as typed numbers, and non-numeric text must not.
func TestNumericCellsAreNumbers(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")

	// 840 is numeric -> <v>840</v> with no t="inlineStr".
	if !strings.Contains(sheet, "<v>840</v>") {
		t.Errorf("numeric cell 840 not emitted as a number: %s", sheet)
	}
	if !strings.Contains(sheet, "<v>0.4</v>") {
		t.Errorf("decimal cell 0.4 not emitted as a number: %s", sheet)
	}
	// Header text must be an inline string, never a number.
	if !strings.Contains(sheet, `t="inlineStr"`) {
		t.Errorf("text cells are not inline strings: %s", sheet)
	}
	// "p95 latency" must NOT be coerced to a number.
	if strings.Contains(sheet, "<v>p95 latency</v>") {
		t.Errorf("text was emitted as a numeric value: %s", sheet)
	}
}

// TestNumericDetection asserts isNumeric against the PUBLISHED corpus rather
// than a table restated here.
//
// The cases used to be listed inline, and one of them -- {"+3", true} -- pinned
// the exact OPPOSITE of what AODex's coerceInput asserts for the same string.
// Two suites, both green, both right by their own lights, disagreeing about one
// string. That is the drift decision D6 closes: the two XLSX writers stay
// separate (unifying them would turn "007" into 7, dates into text and a
// formula into the literal "=SUM(A1:A5)"), but they share ONE rule, published
// as data in conformance/corpus/xlsxtyping.
//
// So do not re-add a local table here, and in particular do not "fix" +3 back
// to a number -- change the corpus, and both sides move together or fail.
func TestNumericDetection(t *testing.T) {
	if len(xlsxtyping.Cases) == 0 {
		t.Fatal("the typing corpus is empty; an empty corpus asserts nothing while looking green")
	}
	for _, c := range xlsxtyping.Cases {
		want := c.Class == xlsxtyping.ClassNumber
		if got := isNumeric(c.Input); got != want {
			reason := c.Reason
			if reason == "" {
				reason = "no reason recorded; see conformance/corpus/xlsxtyping"
			}
			t.Errorf("isNumeric(%q) = %v, want %v (corpus class %q)\n  reason: %s",
				c.Input, got, want, c.Class, reason)
		}
	}
}

// mustPrecede asserts that needle appears before other in part. Presence alone
// is not enough for a <sheetView>: a pane emitted AFTER <sheetData> is silently
// ignored by every viewer, so a substring assertion would pass on a workbook
// that does not freeze anything. Ordering is the property that matters.
func mustPrecede(t *testing.T, part, needle, other string) {
	t.Helper()
	i := strings.Index(part, needle)
	j := strings.Index(part, other)
	if i < 0 {
		t.Fatalf("%s is absent from the part: %s", needle, part)
	}
	if j < 0 {
		t.Fatalf("%s is absent from the part: %s", other, part)
	}
	if i > j {
		t.Errorf("%s appears AFTER %s (at %d vs %d); a pane after <sheetData> is silently ignored: %s",
			needle, other, i, j, part)
	}
}

// TestHeaderRowIsFrozen: sheetRows' docstring says header rows are "rendered
// bold and frozen". Bold was real; frozen was not -- buildSheetXML emitted no
// <sheetView> at all. This pins the freeze so the docstring stays true.
func TestHeaderRowIsFrozen(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")

	for _, want := range []string{
		`<sheetViews>`,
		`<sheetView`,
		`ySplit="1"`,
		`topLeftCell="A2"`,
		`activePane="bottomLeft"`,
		`state="frozen"`,
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("frozen header pane missing %s: %s", want, sheet)
		}
	}
	// The failure mode that looks like "the pane element does nothing".
	mustPrecede(t, sheet, "<sheetViews>", "<sheetData>")
	if err := xmlWellFormed(sheet); err != nil {
		t.Errorf("sheet is not well-formed with the pane: %v", err)
	}
}

// TestNoHeaderRowNoPane: freezing row 1 when row 1 is a data row would pin an
// arbitrary record to the top of the user's view, which is worse than not
// freezing at all. A table with no header row must emit no pane.
func TestNoHeaderRowNoPane(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(model.Block{
		Kind: model.BlockTable,
		Rows: [][]string{{"a", "1"}, {"b", "2"}},
	}), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	if strings.Contains(sheet, "<pane") || strings.Contains(sheet, "<sheetViews>") {
		t.Errorf("a header-less sheet froze a data row: %s", sheet)
	}
}

// TestFreezeIsPerSheet: the flag lives on the grid, not on the workbook, so a
// workbook mixing header-bearing and header-less tables must freeze only the
// sheets that have a header.
func TestFreezeIsPerSheet(t *testing.T) {
	d := &model.Document{
		SchemaVersion: model.SchemaVersion,
		Sections: []model.Section{
			{ID: 1, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: "Headed"},
				metricsTable,
			}},
			{ID: 2, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: "Headless"},
				{Kind: model.BlockTable, Rows: [][]string{{"a", "1"}}},
			}},
		},
	}
	pkg, err := ToXLSX(d, Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	if s := readPart(t, pkg, "xl/worksheets/sheet1.xml"); !strings.Contains(s, `state="frozen"`) {
		t.Errorf("sheet1 has a header row but no frozen pane: %s", s)
	}
	if s := readPart(t, pkg, "xl/worksheets/sheet2.xml"); strings.Contains(s, "<pane") {
		t.Errorf("sheet2 has no header row but froze anyway: %s", s)
	}
}

// alignedTable is the fixture from openable_test.go's acceptance workbook: a
// label column followed by three right-aligned numeric columns, which is what
// model.Block.Aligns' docstring means by "a right-aligned numeric column must
// survive into the export".
var alignedTable = model.Block{
	Kind:    model.BlockTable,
	Headers: []string{"Metric", "Q2", "Q3", "Delta"},
	Rows: [][]string{
		{"p95 latency", "840", "550", "-34"},
		{"Error rate", "1.2", "0.4", "-0.8"},
	},
	Aligns: []string{"left", "right", "right", "right"},
}

// cellStyle returns the s="N" attribute value of the cell at ref, or -1 when
// the cell carries no style attribute at all (the default format).
func cellStyle(t *testing.T, part, ref string) int {
	t.Helper()
	i := strings.Index(part, `<c r="`+ref+`"`)
	if i < 0 {
		t.Fatalf("no cell at %s in: %s", ref, part)
	}
	rest := part[i+len(`<c r="`+ref+`"`):]
	end := strings.Index(rest, ">")
	if end < 0 {
		t.Fatalf("unterminated cell at %s", ref)
	}
	attrs := rest[:end]
	j := strings.Index(attrs, ` s="`)
	if j < 0 {
		return -1
	}
	v := attrs[j+len(` s="`):]
	k := strings.Index(v, `"`)
	if k < 0 {
		t.Fatalf("unterminated s attribute at %s", ref)
	}
	n, err := strconv.Atoi(v[:k])
	if err != nil {
		t.Fatalf("cell %s has non-numeric style %q", ref, v[:k])
	}
	return n
}

// TestRightAlignedColumnSurvives: Block.Aligns was documented as "load-bearing
// for ... a future convert/xlsx: a right-aligned numeric column must survive
// into the export", and for three schema versions the writer never read it --
// its only appearance in this package was a test fixture. This is the
// docstring becoming true.
func TestRightAlignedColumnSurvives(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(alignedTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")

	// Header row: bold, and bold+right in the three aligned columns.
	if got := cellStyle(t, sheet, "A1"); got != xfBold {
		t.Errorf("header A1 style = %d, want %d (bold, no alignment)", got, xfBold)
	}
	for _, ref := range []string{"B1", "C1", "D1"} {
		if got := cellStyle(t, sheet, ref); got != xfBoldRight {
			t.Errorf("header %s style = %d, want %d (bold+right)", ref, got, xfBoldRight)
		}
	}
	// Body rows: the label column keeps the default, the numeric columns
	// carry right alignment.
	for _, ref := range []string{"A2", "A3"} {
		if got := cellStyle(t, sheet, ref); got != -1 {
			t.Errorf(`body %s style = %d, want no s attribute -- a "left" column must not be restyled`, ref, got)
		}
	}
	for _, ref := range []string{"B2", "C2", "D2", "B3", "C3", "D3"} {
		if got := cellStyle(t, sheet, ref); got != xfRight {
			t.Errorf("body %s style = %d, want %d (right)", ref, got, xfRight)
		}
	}
	if err := xmlWellFormed(sheet); err != nil {
		t.Errorf("sheet is not well-formed with alignment styles: %v", err)
	}
}

// TestCenterAlignedColumn: the other alignment a GFM table can express.
func TestCenterAlignedColumn(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(model.Block{
		Kind:    model.BlockTable,
		Headers: []string{"A", "B"},
		Rows:    [][]string{{"x", "y"}},
		Aligns:  []string{"", "center"},
	}), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	if got := cellStyle(t, sheet, "B2"); got != xfCenter {
		t.Errorf("centered cell B2 style = %d, want %d", got, xfCenter)
	}
	if got := cellStyle(t, sheet, "B1"); got != xfBoldCenter {
		t.Errorf("centered header B1 style = %d, want %d", got, xfBoldCenter)
	}
	// The unspecified column proves the feature is opt-in.
	if got := cellStyle(t, sheet, "A2"); got != -1 {
		t.Errorf("unaligned cell A2 style = %d, want no s attribute", got)
	}
	if got := cellStyle(t, sheet, "A1"); got != xfBold {
		t.Errorf("unaligned header A1 style = %d, want %d (bold only)", got, xfBold)
	}
}

// The worksheet convert/xlsx produces for a table with NO Aligns. Alignment
// is opt-in, so a table that asked for none must stay free of alignment
// styles. The pin includes <dimension> and omits empty default-style cells;
// refresh it only when that worksheet shape changes on purpose.
//
// A hash rather than the literal bytes keeps the assertion readable; the
// failure prints the actual part, which is what a reader needs.
const (
	noAlignsSheetSHA256 = "4d43e47e5191e8c8d69b776705ad492cee3937f89d180cce859168ab15cf51a7"
	noAlignsSheetLen    = 854
)

// TestNoAlignsWorksheetIsByteIdentical is the no-regression case: the blast
// radius of the alignment change must be exactly the tables that asked for it.
func TestNoAlignsWorksheetIsByteIdentical(t *testing.T) {
	if metricsTable.Aligns != nil {
		t.Fatal("the fixture grew an Aligns slice; this test only means something without one")
	}
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	sum := sha256.Sum256([]byte(sheet))
	got := hex.EncodeToString(sum[:])
	if len(sheet) != noAlignsSheetLen || got != noAlignsSheetSHA256 {
		t.Errorf("a table with no Aligns changed: %d bytes / %s, want %d bytes / %s\n%s",
			len(sheet), got, noAlignsSheetLen, noAlignsSheetSHA256, sheet)
	}
}

// TestAlignsRaggedRowDoesNotPanic: the docmodel preserves rows exactly as
// authored, so a row can be shorter OR longer than its table's Aligns slice.
// An unguarded per-column lookup panics on real input, not a hypothetical.
func TestAlignsRaggedRowDoesNotPanic(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(model.Block{
		Kind:    model.BlockTable,
		Headers: []string{"A", "B", "C"},
		Rows:    [][]string{{"1"}, {"1", "2", "3", "4"}},
		// Fewer alignments than columns, on purpose.
		Aligns: []string{"right"},
	}), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	if got := cellStyle(t, sheet, "A2"); got != xfRight {
		t.Errorf("A2 style = %d, want %d", got, xfRight)
	}
	// The short row is padded in the grid, then empty default-style cells
	// are omitted. C2 must not come back with a style from column A.
	if strings.Contains(sheet, `<c r="C2"`) {
		t.Errorf("padded empty C2 was written: %s", sheet)
	}
	if err := xmlWellFormed(sheet); err != nil {
		t.Errorf("ragged sheet is not well-formed: %v", err)
	}
}

// TestStylesTableIsConsistent guards the two ways this styles part breaks
// Excel opaquely: a count attribute that disagrees with the children, and a
// reordered <styleSheet>.
func TestStylesTableIsConsistent(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(alignedTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	styles := readPart(t, pkg, "xl/styles.xml")

	i := strings.Index(styles, `<cellXfs count="`)
	if i < 0 {
		t.Fatalf("no cellXfs in styles.xml: %s", styles)
	}
	block := styles[i:]
	if end := strings.Index(block, `</cellXfs>`); end >= 0 {
		block = block[:end]
	}
	declared := block[len(`<cellXfs count="`):]
	declared = declared[:strings.Index(declared, `"`)]
	want, err := strconv.Atoi(declared)
	if err != nil {
		t.Fatalf("cellXfs count is not a number: %q", declared)
	}
	if got := strings.Count(block, "<xf "); got != want {
		t.Errorf(`<cellXfs count="%d"> but %d <xf> children; Excel rejects this with no useful message`, want, got)
	}
	if want != xfCount {
		t.Errorf("cellXfs count = %d, want xfCount = %d", want, xfCount)
	}

	// Child order is fixed by the schema; the part's own comment says Excel
	// rejects the workbook if it changes.
	order := []string{"<fonts ", "<fills ", "<borders ", "<cellStyleXfs ", "<cellXfs "}
	last := -1
	for _, el := range order {
		at := strings.Index(styles, el)
		if at < 0 {
			t.Fatalf("styles.xml is missing %s: %s", el, styles)
		}
		if at < last {
			t.Errorf("styleSheet children are out of schema order at %s: %s", el, styles)
		}
		last = at
	}
	// The two mandatory placeholder fills must survive.
	if !strings.Contains(styles, `patternType="none"`) || !strings.Contains(styles, `patternType="gray125"`) {
		t.Errorf("the mandatory placeholder fills were dropped: %s", styles)
	}
	// Indices 0 and 1 keep their meaning: every previously-emitted s="1"
	// still means bold.
	if !strings.Contains(styles, `<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>`) {
		t.Errorf("xf 0/1 are no longer the original default/bold pair; existing s=\"1\" cells would be restyled: %s", styles)
	}
	if err := xmlWellFormed(styles); err != nil {
		t.Errorf("styles.xml is not well-formed: %v", err)
	}
}

// TestColumnRef covers the A..Z -> AA rollover, the classic off-by-one in
// spreadsheet writers.
func TestColumnRef(t *testing.T) {
	for _, tc := range []struct {
		idx  int
		want string
	}{
		{0, "A"}, {1, "B"}, {25, "Z"}, {26, "AA"}, {27, "AB"}, {51, "AZ"}, {52, "BA"}, {701, "ZZ"}, {702, "AAA"},
	} {
		if got := columnRef(tc.idx); got != tc.want {
			t.Errorf("columnRef(%d) = %q, want %q", tc.idx, got, tc.want)
		}
	}
}

// TestOneSheetPerSection: each section carrying a table becomes its own sheet;
// a section with no table is skipped rather than producing an empty sheet.
func TestOneSheetPerSection(t *testing.T) {
	d := &model.Document{
		SchemaVersion: model.SchemaVersion,
		Sections: []model.Section{
			{ID: 1, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: "Metrics"},
				metricsTable,
			}},
			{ID: 2, Blocks: []model.Block{
				{Kind: model.BlockParagraph, Text: "prose only, no table"},
			}},
			{ID: 3, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: "Costs"},
				{Kind: model.BlockTable, Headers: []string{"Item"}, Rows: [][]string{{"x"}}},
			}},
		},
	}
	pkg, err := ToXLSX(d, Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	names := partNames(t, pkg)
	sheets := 0
	for _, n := range names {
		if strings.HasPrefix(n, "xl/worksheets/sheet") {
			sheets++
		}
	}
	if sheets != 2 {
		t.Errorf("worksheets = %d, want 2 (the table-free section must be skipped): %v", sheets, names)
	}

	wb := readPart(t, pkg, "xl/workbook.xml")
	if !strings.Contains(wb, `name="Metrics"`) || !strings.Contains(wb, `name="Costs"`) {
		t.Errorf("sheets not named from their section headings: %s", wb)
	}
}

// TestSheetNameSanitization: Excel rejects a workbook whose sheet names break
// its rules, so names are sanitized, length-capped and de-duplicated.
func TestSheetNameSanitization(t *testing.T) {
	long := strings.Repeat("x", 40)
	d := &model.Document{
		SchemaVersion: model.SchemaVersion,
		Sections: []model.Section{
			{ID: 1, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: `A/B:C\D?E*F[G]H`},
				metricsTable,
			}},
			{ID: 2, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: long},
				metricsTable,
			}},
			{ID: 3, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: long},
				metricsTable,
			}},
		},
	}
	pkg, err := ToXLSX(d, Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	wb := readPart(t, pkg, "xl/workbook.xml")

	for _, bad := range []string{"/", ":", "\\", "?", "*", "[", "]"} {
		if strings.Contains(wb, `name="`) && strings.Contains(extractNames(wb), bad) {
			t.Errorf("sheet name retains the illegal character %q: %s", bad, wb)
		}
	}
	for _, n := range strings.Split(extractNames(wb), "\x00") {
		if len([]rune(n)) > 31 {
			t.Errorf("sheet name %q exceeds Excel's 31-character limit", n)
		}
	}
	// The two identical over-long names must not collide.
	names := strings.Split(extractNames(wb), "\x00")
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate sheet name %q (Excel rejects the workbook): %v", n, names)
		}
		seen[n] = true
	}
}

// extractNames pulls the name="..." attribute values out of workbook.xml,
// NUL-joined so the caller can split them unambiguously.
func extractNames(wb string) string {
	var out []string
	for _, chunk := range strings.Split(wb, `<sheet name="`)[1:] {
		if i := strings.Index(chunk, `"`); i >= 0 {
			out = append(out, chunk[:i])
		}
	}
	return strings.Join(out, "\x00")
}

// TestNoTablesFallback: a workbook must contain at least one sheet, so a
// document with no tables still produces a valid (if sparse) package rather
// than an empty workbook Excel would reject.
func TestNoTablesFallback(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(
		model.Block{Kind: model.BlockHeading, Level: 1, Text: "Notes"},
		model.Block{Kind: model.BlockParagraph, Text: "no tables here"},
	), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	if !strings.Contains(sheet, "no tables here") {
		t.Errorf("fallback sheet did not carry the document's text: %s", sheet)
	}
}

// TestRaggedRows: rows shorter than the header are padded; the sheet stays
// rectangular.
func TestRaggedRows(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(model.Block{
		Kind:    model.BlockTable,
		Headers: []string{"A", "B", "C"},
		Rows:    [][]string{{"1"}, {"1", "2", "3", "4"}},
	}), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	// The overflowing 4th value must not appear in a D column.
	if strings.Contains(sheet, `r="D`) {
		t.Errorf("overflow cell created a D column beyond the header width: %s", sheet)
	}
}

// TestXMLEscaping: cell text is untrusted and must not be able to corrupt the
// sheet.
func TestXMLEscaping(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(model.Block{
		Kind:    model.BlockTable,
		Headers: []string{`a & b <c>`},
		Rows:    [][]string{{`"quoted" <row/>`}},
	}), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	sheet := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	if !strings.Contains(sheet, "&amp;") || !strings.Contains(sheet, "&lt;") {
		t.Errorf("cell text not XML-escaped: %s", sheet)
	}
	if err := xmlWellFormed(sheet); err != nil {
		t.Errorf("sheet is not well-formed after escaping: %v", err)
	}
}

// TestAllPartsWellFormed runs every emitted part through a real XML decoder.
func TestAllPartsWellFormed(t *testing.T) {
	pkg, err := ToXLSX(tableDoc(metricsTable), Options{})
	if err != nil {
		t.Fatalf("ToXLSX: %v", err)
	}
	for _, name := range partNames(t, pkg) {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		if err := xmlWellFormed(readPart(t, pkg, name)); err != nil {
			t.Errorf("%s is not well-formed: %v", name, err)
		}
	}
}
