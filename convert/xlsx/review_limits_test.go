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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// manySheetsOnePart is a package whose workbook lists n sheets that all
// resolve to the same worksheet part.
func manySheetsOnePart(t *testing.T, n int, sheet zipPart) []byte {
	t.Helper()
	var wb, rels strings.Builder
	wb.WriteString(`<workbook xmlns:r="r"><sheets>`)
	rels.WriteString(`<Relationships>`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&wb, `<sheet name="S%d" sheetId="%d" r:id="rId%d"/>`, i, i, i)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>`, i)
	}
	wb.WriteString(`</sheets></workbook>`)
	rels.WriteString(`</Relationships>`)
	sheet.name = "xl/worksheets/sheet1.xml"
	return buildPackage(t, []zipPart{
		{name: "xl/workbook.xml", body: wb.String()},
		{name: "xl/_rels/workbook.xml.rels", body: rels.String()},
		sheet,
	})
}

// The ratio floor exempts a small part from MaxRatio, but the exemption is
// not a budget per read. A package whose sheets all point at one tiny part
// that inflates to just under the floor would otherwise inflate and parse
// that part once per sheet, up to TotalBytes, from a few kilobytes of upload.
// The package as a whole is held to the ratio, so this refuses long before.
func TestOpenRefusesOneSmallPartReadByManySheets(t *testing.T) {
	pkg := manySheetsOnePart(t, 400, zipPart{fill: ' ', n: RatioFloorBytes - 1})

	_, err := Open(pkg)
	var le *LimitError
	if !errors.As(err, &le) || le.Limit != LimitCompressionRatio {
		t.Fatalf("Open(%d-byte package reading one ~1 MiB part 400 times) err = %v, want a compression-ratio LimitError", len(pkg), err)
	}
}

// Many distinct small parts, each under the floor, are the same amplification
// as one part read many times; the package-wide ratio refuses them too.
func TestOpenRefusesManyDistinctSmallParts(t *testing.T) {
	const n = 300
	var wb, rels strings.Builder
	wb.WriteString(`<workbook xmlns:r="r"><sheets>`)
	rels.WriteString(`<Relationships>`)
	parts := []zipPart{{}, {}}
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&wb, `<sheet name="S%d" sheetId="%d" r:id="rId%d"/>`, i, i, i)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
		parts = append(parts, zipPart{name: fmt.Sprintf("xl/worksheets/sheet%d.xml", i), fill: ' ', n: RatioFloorBytes - 1})
	}
	wb.WriteString(`</sheets></workbook>`)
	rels.WriteString(`</Relationships>`)
	parts[0] = zipPart{name: "xl/workbook.xml", body: wb.String()}
	parts[1] = zipPart{name: "xl/_rels/workbook.xml.rels", body: rels.String()}
	pkg := buildPackage(t, parts)

	_, err := Open(pkg)
	var le *LimitError
	if !errors.As(err, &le) || le.Limit != LimitCompressionRatio {
		t.Fatalf("Open(%d-byte package of %d ~1 MiB parts) err = %v, want a compression-ratio LimitError", len(pkg), n, err)
	}
}

// The package-wide ratio must not refuse a large, legitimate, very regular
// workbook: every cell the same shared string, which is the most compressible
// shape a real sheet takes (the r="A1" references keep it near 15-20x).
func TestOpenAcceptsLargeRepetitiveWorkbook(t *testing.T) {
	if testing.Short() {
		t.Skip("SKIPPED, NOT PASSED: builds a 40 MB sheet part")
	}
	const rows, cols = 60_000, 20
	var b strings.Builder
	b.WriteString(`<worksheet><sheetData>`)
	for r := 1; r <= rows; r++ {
		fmt.Fprintf(&b, `<row r="%d">`, r)
		for c := 0; c < cols; c++ {
			b.WriteString(`<c r="` + columnRef(c) + strconv.Itoa(r) + `" t="s" s="2"><v>0</v></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	parts := append(minimalParts(zipPart{body: b.String()}),
		zipPart{name: "xl/sharedStrings.xml", body: `<sst><si><t>same</t></si></sst>`})
	pkg := buildPackage(t, parts)

	if _, err := Open(pkg); err != nil {
		t.Fatalf("Open(%d-byte package, %d cells of one shared string) = %v, want it accepted", len(pkg), rows*cols, err)
	}
}

// A <col> that aligns a span of columns does span-many writes, and was not
// charged to any budget: a sheet of full-width <col> elements, under every
// byte and ratio limit, cost 16,384 writes per element. Its span is charged to
// the cell budget.
func TestOpenChargesColumnSpansToTheCellBudget(t *testing.T) {
	sheet := `<worksheet><cols>` + strings.Repeat(`<col min="1" max="16384" style="2"/>`, 10) + `</cols><sheetData/></worksheet>`
	pkg := buildPackage(t, minimalParts(zipPart{body: sheet}))

	_, err := OpenWithLimits(pkg, Limits{Cells: 100_000})
	var le *LimitError
	if !errors.As(err, &le) || le.Limit != LimitCells {
		t.Fatalf("OpenWithLimits(10 full-width aligned cols, cells cap 100k) err = %v, want a cells LimitError", err)
	}
	// One full-width column style, the shape Excel writes, fits easily.
	one := `<worksheet><cols><col min="3" max="16384" style="2"/></cols><sheetData/></worksheet>`
	if _, err := Open(buildPackage(t, minimalParts(zipPart{body: one}))); err != nil {
		t.Fatalf("Open(one full-width aligned col) = %v, want it accepted", err)
	}
}

// docProps/core.xml only supplies the title. A core part this reader cannot
// decode (an unsupported compression method here) used to fall back to the
// default title; it must not now reject the whole workbook. A limit on it is
// still a refusal (TestOpenRejectsOversizedOptionalPart).
func TestOpenToleratesUnreadableCoreProps(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range minimalParts(zipPart{body: `<worksheet/>`}) {
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.CreateRaw(&zip.FileHeader{Name: "docProps/core.xml", Method: 99, CompressedSize64: 4, UncompressedSize64: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("junk")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	wb, err := Open(buf.Bytes())
	if err != nil {
		t.Fatalf("Open(workbook with an undecodable docProps/core.xml) = %v, want it opened with the default title", err)
	}
	if wb.Title() != "Workbook" {
		t.Errorf("title = %q, want the default %q", wb.Title(), "Workbook")
	}
}

// The limit names a caller switches on are exported constants, so a caller
// can map each refusal to its own message without matching strings.
func TestLimitNamesAreExported(t *testing.T) {
	for _, name := range []string{LimitPartBytes, LimitTotalBytes, LimitParts, LimitCells, LimitCompressionRatio} {
		if name == "" {
			t.Fatal("empty limit name")
		}
	}
}

// Aligning columns left to right, as a full-width <col> does, must not
// reallocate the alignment slice once per column: that was 16,384 copies of a
// growing slice, seconds of CPU for one <col> element in one sheet.
func TestSetAlignAscendingIsNotQuadratic(t *testing.T) {
	allocs := testing.AllocsPerRun(3, func() {
		s := &Sheet{}
		for c := 0; c < maxSheetCols; c++ {
			if err := s.SetAlign(c, "right"); err != nil {
				t.Fatal(err)
			}
		}
	})
	if allocs > 64 {
		t.Fatalf("aligning %d columns in order allocated %.0f times, want amortised growth (<= 64)", maxSheetCols, allocs)
	}
}
