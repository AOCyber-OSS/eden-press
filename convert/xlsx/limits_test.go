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
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// zipPart is one entry of a hand-built package. Either body (small, literal)
// or fill (a byte repeated n times, streamed so the test never holds the
// inflated part) supplies its content.
type zipPart struct {
	name string
	body string
	fill byte
	n    int64
	// declare, when non-zero, overrides the uncompressed size written to the
	// entry's header: a lying header that under-states what the part inflates
	// to.
	declare uint64
}

// buildPackage writes parts into a zip. Content of fill parts is deflated as
// it is generated, so a part that inflates to hundreds of megabytes costs a
// few hundred kilobytes here.
func buildPackage(t *testing.T, parts []zipPart) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		if p.declare != 0 {
			writeLyingPart(t, zw, p)
			continue
		}
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatalf("create %s: %v", p.name, err)
		}
		if p.n > 0 {
			if err := streamFill(w, p.fill, p.n); err != nil {
				t.Fatalf("fill %s: %v", p.name, err)
			}
			continue
		}
		if _, err := io.WriteString(w, p.body); err != nil {
			t.Fatalf("write %s: %v", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// writeLyingPart deflates the real content, then stores it raw under a header
// that declares p.declare uncompressed bytes.
func writeLyingPart(t *testing.T, zw *zip.Writer, p zipPart) {
	t.Helper()
	var raw bytes.Buffer
	fw, err := newDeflater(&raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := streamFill(fw, p.fill, p.n); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name:               p.name,
		Method:             zip.Deflate,
		CompressedSize64:   uint64(raw.Len()),
		UncompressedSize64: p.declare,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
}

func streamFill(w io.Writer, c byte, n int64) error {
	chunk := bytes.Repeat([]byte{c}, 1<<20)
	for n > 0 {
		k := int64(len(chunk))
		if n < k {
			k = n
		}
		if _, err := w.Write(chunk[:k]); err != nil {
			return err
		}
		n -= k
	}
	return nil
}

// minimalParts is the smallest package Open accepts: one sheet whose
// worksheet part is sheetPart.
func minimalParts(sheetPart zipPart) []zipPart {
	sheetPart.name = "xl/worksheets/sheet1.xml"
	return []zipPart{
		{name: "xl/workbook.xml", body: `<workbook><sheets><sheet name="S" sheetId="1" r:id="rId1" xmlns:r="r"/></sheets></workbook>`},
		{name: "xl/_rels/workbook.xml.rels", body: `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`},
		sheetPart,
	}
}

// A part that inflates past the part-byte limit is refused with a LimitError
// naming that limit. The part is whitespace, valid XML filler, so the size is
// the only possible reason; the ratio guard is lifted so the byte cap is what
// trips.
func TestOpenRejectsOversizedPart(t *testing.T) {
	pkg := buildPackage(t, minimalParts(zipPart{fill: ' ', n: 1<<20 + 1}))

	_, err := OpenWithLimits(pkg, Limits{PartBytes: 1 << 20, Ratio: 1 << 30})
	var le *LimitError
	if !errors.As(err, &le) || le.Limit != "part bytes" || le.Value != 1<<20 {
		t.Fatalf("OpenWithLimits(oversized part) err = %v, want a part-bytes LimitError of %d", err, 1<<20)
	}
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("LimitError %v does not match ErrLimitExceeded", err)
	}
}

// A header that under-states the part's size does not get it past the cap:
// the bound is on the bytes inflated, not on what the header claims.
func TestOpenRejectsPartThatInflatesPastItsHeader(t *testing.T) {
	pkg := buildPackage(t, minimalParts(zipPart{fill: ' ', n: 2 << 20, declare: 1024}))

	_, err := Open(pkg)
	if err == nil {
		t.Fatal("Open accepted a part whose header declares 1 KiB but inflates to 2 MiB")
	}
	t.Logf("refused with: %v", err)
}

// Parts that each fit under the part limit but together inflate past the
// total limit are refused: a package of many just-under-the-cap parts is the
// same bomb split up. Two sheets point at two 1 MiB parts; the total cap is
// 1.5 MiB.
func TestOpenRejectsPackageOverTotalLimit(t *testing.T) {
	parts := minimalParts(zipPart{fill: ' ', n: 1 << 20})
	parts[0].body = `<workbook><sheets><sheet name="A" sheetId="1" r:id="rId1" xmlns:r="r"/><sheet name="B" sheetId="2" r:id="rId2" xmlns:r="r"/></sheets></workbook>`
	parts[1].body = `<Relationships>` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>` +
		`</Relationships>`
	parts = append(parts, zipPart{name: "xl/worksheets/sheet2.xml", fill: ' ', n: 1 << 20})
	pkg := buildPackage(t, parts)

	lim := testLimits()
	lim.PartBytes = 1 << 20
	lim.TotalBytes = 3 << 19
	_, err := openWithLimits(pkg, lim)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("openWithLimits(2 x 1 MiB, total cap 1.5 MiB) err = %v, want ErrLimitExceeded", err)
	}
}

// testLimits is defaultLimits, for a test to lower one bound of.
func testLimits() Limits { return DefaultLimits() }

// A package with more zip entries than the part-count limit is refused before
// any part is read: the central directory alone can be a memory cost.
func TestOpenRejectsTooManyParts(t *testing.T) {
	parts := minimalParts(zipPart{body: `<worksheet/>`})
	for i := 0; i < 8; i++ {
		parts = append(parts, zipPart{name: "xl/media/pad" + string(rune('a'+i)), body: "x"})
	}
	pkg := buildPackage(t, parts)

	lim := testLimits()
	lim.Parts = 10
	_, err := openWithLimits(pkg, lim)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("openWithLimits(11 parts, cap 10) err = %v, want ErrLimitExceeded", err)
	}

	lim.Parts = len(parts)
	if _, err := openWithLimits(pkg, lim); err != nil {
		t.Fatalf("openWithLimits(%d parts, cap %d) = %v, want the package accepted at the cap", len(parts), lim.Parts, err)
	}
}

// An optional part over the limit is an error, not a part treated as absent:
// silently dropping xl/sharedStrings.xml would import every shared-string cell
// as a wrong value instead of refusing the package.
func TestOpenRejectsOversizedOptionalPart(t *testing.T) {
	for _, name := range []string{"xl/sharedStrings.xml", "docProps/core.xml"} {
		t.Run(name, func(t *testing.T) {
			parts := append(minimalParts(zipPart{body: `<worksheet/>`}),
				zipPart{name: name, fill: ' ', n: 2 << 10})
			pkg := buildPackage(t, parts)

			lim := testLimits()
			lim.PartBytes = 1 << 10
			_, err := openWithLimits(pkg, lim)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("openWithLimits(oversized %s) err = %v, want ErrLimitExceeded", name, err)
			}
		})
	}
}

// sheetOfCells is a worksheet with n one-character cells, the densest shape a
// sheet part can take: the case where a part well under the byte limit still
// costs the most memory once parsed.
func sheetOfCells(n int) string {
	var b strings.Builder
	b.WriteString(`<worksheet><sheetData><row r="1">`)
	for i := 0; i < n; i++ {
		b.WriteString(`<c r="`)
		b.WriteString(columnRef(i % maxSheetCols))
		b.WriteString(strconv.Itoa(i/maxSheetCols + 1))
		b.WriteString(`"><v>1</v></c>`)
	}
	b.WriteString(`</row></sheetData></worksheet>`)
	return b.String()
}

// Bytes are not the cost that matters once a part is parsed: every cell is a
// map entry. A review measured a 26 MB sheet at 645 MB allocated and 140 MB
// live, so Open also caps the cells it will build.
func TestOpenRejectsTooManyCells(t *testing.T) {
	pkg := buildPackage(t, minimalParts(zipPart{body: sheetOfCells(11)}))

	lim := testLimits()
	lim.Cells = 10
	if _, err := openWithLimits(pkg, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("openWithLimits(11 cells, cap 10) err = %v, want ErrLimitExceeded", err)
	}
	lim.Cells = 11
	if _, err := openWithLimits(pkg, lim); err != nil {
		t.Fatalf("openWithLimits(11 cells, cap 11) = %v, want the sheet accepted at the cap", err)
	}
}

// A shared-strings table is parsed before any sheet; its entry count is held
// to the same cap, since every entry is a string the import keeps.
func TestOpenRejectsTooManySharedStrings(t *testing.T) {
	parts := append(minimalParts(zipPart{body: `<worksheet/>`}),
		zipPart{name: "xl/sharedStrings.xml", body: `<sst>` + strings.Repeat(`<si><t>x</t></si>`, 11) + `</sst>`})
	pkg := buildPackage(t, parts)

	lim := testLimits()
	lim.Cells = 10
	if _, err := openWithLimits(pkg, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("openWithLimits(11 shared strings, cap 10) err = %v, want ErrLimitExceeded", err)
	}
}

// What one parsed cell costs, measured, so the memory a cap implies is a
// number and not a guess: MaxCells documents ~70 bytes live and ~700
// allocated per cell. The densest sheet shape (one-character cells) is
// measured at 500,000 cells and held to a per-cell budget with headroom; the
// implied cost at the default cap is logged. The review that prompted the cell
// cap measured a 26 MB sheet at 645 MB allocated and 140 MB live.
func TestImportMemoryPerCell(t *testing.T) {
	const (
		n               = 500_000
		allocPerCellCap = 900
		livePerCellCap  = 128
	)
	pkg := buildPackage(t, minimalParts(zipPart{body: sheetOfCells(n)}))

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	wb, err := Open(pkg)
	runtime.GC()
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("Open(%d cells) = %v, want it accepted", n, err)
	}
	alloc := int64(after.TotalAlloc - before.TotalAlloc)
	live := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	runtime.KeepAlive(wb)
	t.Logf("%d cells: %d B allocated and %d B live per cell; at MaxCells (%d) that is ~%d MB allocated, ~%d MB live",
		n, alloc/n, live/n, MaxCells, alloc/n*MaxCells>>20, live/n*MaxCells>>20)
	if alloc/n > allocPerCellCap || live/n > livePerCellCap {
		t.Errorf("per-cell cost %d B allocated / %d B live exceeds the %d / %d budget MaxCells is documented against",
			alloc/n, live/n, allocPerCellCap, livePerCellCap)
	}
}

// The compression ratio is the zip-bomb signature, and it is what a bomb
// cannot hide: a part far under every byte cap that inflates more than
// MaxRatio times its compressed size is refused, on the bytes actually
// inflated. Real worksheet XML compresses around 5-20x.
func TestOpenRefusesBombByCompressionRatio(t *testing.T) {
	pkg := buildPackage(t, minimalParts(zipPart{fill: ' ', n: 8 << 20}))

	_, err := Open(pkg)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Open(8 MiB of filler in a %d-byte package) err = %v, want ErrLimitExceeded", len(pkg), err)
	}
	if !strings.Contains(err.Error(), "compression ratio") {
		t.Fatalf("refusal %q does not name the compression-ratio limit", err)
	}
}

func newDeflater(w io.Writer) (io.WriteCloser, error) {
	return flate.NewWriter(w, flate.DefaultCompression)
}

// realisticSheetPackage builds an Excel-shaped workbook: a sales ledger of
// rows x 8 columns with shared strings, style indexes, numbers, dates and a
// formula per row, from a seeded generator so it is varied but repeatable.
// It is the shape the ratio guard must never refuse.
func realisticSheetPackage(t *testing.T, rows int) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(42))
	first := []string{"Ava", "Liam", "Noah", "Emma", "Olivia", "Mateo", "Sofia", "Yuki", "Arjun", "Chloe", "Kwame", "Ines", "Lars", "Priya", "Diego", "Mei"}
	last := []string{"Smith", "Garcia", "Okafor", "Tanaka", "Nguyen", "Kowalski", "Haddad", "Silva", "Novak", "Patel", "Murphy", "Jensen", "Rossi", "Kim"}
	regions := []string{"North", "South", "East", "West", "Central", "EMEA", "APAC", "LATAM"}
	status := []string{"Open", "Paid", "Overdue", "Refunded", "Disputed"}

	var shared []string
	index := map[string]int{}
	si := func(v string) int {
		if i, ok := index[v]; ok {
			return i
		}
		index[v] = len(shared)
		shared = append(shared, v)
		return index[v]
	}
	var sh strings.Builder
	sh.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r := 1; r <= rows; r++ {
		row := strconv.Itoa(r)
		name := first[rng.Intn(len(first))] + " " + last[rng.Intn(len(last))]
		note := "INV-" + strconv.Itoa(100000+rng.Intn(900000))
		fmt.Fprintf(&sh, `<row r="%s" spans="1:8">`, row)
		fmt.Fprintf(&sh, `<c r="A%s"><v>%d</v></c>`, row, r)
		fmt.Fprintf(&sh, `<c r="B%s" s="1"><v>%d</v></c>`, row, 44927+rng.Intn(1100))
		fmt.Fprintf(&sh, `<c r="C%s" t="s"><v>%d</v></c>`, row, si(name))
		fmt.Fprintf(&sh, `<c r="D%s" t="s"><v>%d</v></c>`, row, si(regions[rng.Intn(len(regions))]))
		fmt.Fprintf(&sh, `<c r="E%s" s="2"><v>%.2f</v></c>`, row, rng.Float64()*9999)
		fmt.Fprintf(&sh, `<c r="F%s"><v>%d</v></c>`, row, 1+rng.Intn(250))
		fmt.Fprintf(&sh, `<c r="G%s" s="2"><f>E%s*F%s</f><v>0</v></c>`, row, row, row)
		fmt.Fprintf(&sh, `<c r="H%s" t="s"><v>%d</v></c></row>`, row, si(status[rng.Intn(len(status))]+" "+note))
	}
	sh.WriteString(`</sheetData></worksheet>`)
	var sst strings.Builder
	fmt.Fprintf(&sst, `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`, len(shared), len(shared))
	for _, v := range shared {
		sst.WriteString(`<si><t>` + v + `</t></si>`)
	}
	sst.WriteString(`</sst>`)

	parts := minimalParts(zipPart{body: sh.String()})
	return buildPackage(t, append(parts, zipPart{name: "xl/sharedStrings.xml", body: sst.String()}))
}

// partRatios reports each part's inflated/compressed ratio.
func partRatios(t *testing.T, pkg []byte) map[string]float64 {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, f := range zr.File {
		if f.CompressedSize64 > 0 && f.UncompressedSize64 >= RatioFloorBytes {
			out[f.Name] = float64(f.UncompressedSize64) / float64(f.CompressedSize64)
		}
	}
	return out
}

// Large realistic workbooks -- an Excel-shaped ledger with shared strings,
// and this package's own writer output -- import at the defaults, and their
// parts compress far inside MaxRatio. The measured ratios are logged.
func TestRealisticWorkbookImportsAtDefaults(t *testing.T) {
	ledger := realisticSheetPackage(t, 60_000)

	wb := NewWorkbook("written")
	sheet, err := wb.AddSheet("Data")
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(7))
	for r := 0; r < 40_000; r++ {
		for c, v := range []string{
			strconv.Itoa(r + 1),
			"Customer " + strconv.Itoa(rng.Intn(5000)),
			strconv.FormatFloat(rng.Float64()*1e5, 'f', 2, 64),
			"=C" + strconv.Itoa(r+1) + "*1.2",
			[]string{"Open", "Closed", "Pending"}[rng.Intn(3)],
		} {
			if err := sheet.Set(r, c, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	written, err := wb.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	for name, pkg := range map[string][]byte{"excel-shaped ledger, 60k x 8": ledger, "eden-press writer, 40k x 5": written} {
		ratios := partRatios(t, pkg)
		t.Logf("%s: %d-byte package, part ratios %v", name, len(pkg), ratios)
		for part, r := range ratios {
			if r > MaxRatio/4 {
				t.Errorf("%s: %s compresses %.1fx, within 4x of MaxRatio (%d)", name, part, r, MaxRatio)
			}
		}
		if _, err := Open(pkg); err != nil {
			t.Errorf("%s: Open at the defaults = %v, want it accepted", name, err)
		}
	}
}

// Every limit is the caller's to set, a zero field keeps its default, and
// the refusal names the limit and its value so a user can be told why.
func TestOpenWithLimitsHonoursCustomLimits(t *testing.T) {
	cells := buildPackage(t, minimalParts(zipPart{body: sheetOfCells(6)}))
	cases := []struct {
		name  string
		pkg   []byte
		lim   Limits
		limit string
		value int64
		msg   string
	}{
		{"cells", cells, Limits{Cells: 5}, "cells", 5, "over the cells limit of 5"},
		{"parts", cells, Limits{Parts: 2}, "parts", 2, "over the parts limit of 2"},
		{"part bytes", cells, Limits{PartBytes: 64}, "part bytes", 64, "over the part bytes limit of 64 bytes"},
		{"total bytes", cells, Limits{TotalBytes: 200}, "total bytes", 200, "over the total bytes limit of 200 bytes"},
		{"ratio", realisticSheetPackage(t, 60_000), Limits{Ratio: 3}, "compression ratio", 3, "more than 3x its compressed size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Open(tc.pkg); err != nil {
				t.Fatalf("Open at the defaults = %v; the custom limit must be what refuses", err)
			}
			_, err := OpenWithLimits(tc.pkg, tc.lim)
			var le *LimitError
			if !errors.As(err, &le) || le.Limit != tc.limit || le.Value != tc.value {
				t.Fatalf("OpenWithLimits(%+v) err = %v, want a %q LimitError of %d", tc.lim, err, tc.limit, tc.value)
			}
			if !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("refusal %q does not say %q", err, tc.msg)
			}
		})
	}

	if got := (Limits{Cells: 5}).withDefaults(); got.Ratio != MaxRatio || got.PartBytes != MaxPartBytes || got.Cells != 5 {
		t.Errorf("Limits{Cells: 5} with defaults = %+v; want Cells 5 and every other field at its default", got)
	}
}
