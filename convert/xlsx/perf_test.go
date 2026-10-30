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
	"time"
)

// Spreadsheet performance plan for the file formats.
//
// Size follows occupied cells. A sheet whose corners are far apart stays
// small. Tall, wide, and square sheets of the same cell count stay in the
// same band. Each case below is a budget: set, xlsx write, xlsx open, and
// the v2 JSON snapshot. The Flutter grid and formula ranges are covered by
// bind/eden_ui/test/spreadsheet_perf_test.dart.
func TestPerformanceBudgets(t *testing.T) {
	cases := []perfCase{
		{
			name: "excel-corners",
			fill: func(s *Sheet) {
				mustSet(t, s, maxSheetRows-1, 0, "9")
				mustSet(t, s, 0, maxSheetCols-1, "8")
			},
			cells:    2,
			wantRows: 2,
			check: func(t *testing.T, s *Sheet) {
				assertCell(t, s, maxSheetRows-1, 0, "9")
				assertCell(t, s, 0, maxSheetCols-1, "8")
			},
			maxXLSX: 64 << 10, maxJSON: 4 << 10,
			maxSet: 100 * time.Millisecond, maxWrite: 100 * time.Millisecond,
			maxOpen: 100 * time.Millisecond, maxJSONTime: 50 * time.Millisecond,
		},
		{
			name: "hundred-thousand-row-hole",
			fill: func(s *Sheet) {
				mustSet(t, s, 0, 0, "1")
				mustSet(t, s, 99999, 25, "2")
			},
			cells:    2,
			wantRows: 2,
			check: func(t *testing.T, s *Sheet) {
				assertCell(t, s, 0, 0, "1")
				assertCell(t, s, 99999, 25, "2")
			},
			maxXLSX: 64 << 10, maxJSON: 4 << 10,
			maxSet: 100 * time.Millisecond, maxWrite: 100 * time.Millisecond,
			maxOpen: 100 * time.Millisecond, maxJSONTime: 50 * time.Millisecond,
		},
		dense("column-1k", 1000, 1, 1<<20, 1<<20, time.Second),
		dense("column-10k", 10000, 1, 4<<20, 4<<20, 2*time.Second),
		dense("column-100k", 100000, 1, 16<<20, 16<<20, 5*time.Second),
		dense("grid-1k-by-10", 1000, 10, 4<<20, 4<<20, 2*time.Second),
		dense("grid-200-by-50", 200, 50, 4<<20, 4<<20, 2*time.Second),
		dense("full-width-row", 1, maxSheetCols, 8<<20, 8<<20, 3*time.Second),
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runPerfCase(t, tc)
		})
	}
}

type perfCase struct {
	name        string
	fill        func(*Sheet)
	cells       int
	wantRows    int
	check       func(*testing.T, *Sheet)
	maxXLSX     int
	maxJSON     int
	maxSet      time.Duration
	maxWrite    time.Duration
	maxOpen     time.Duration
	maxJSONTime time.Duration
}

func dense(name string, rows, cols, maxXLSX, maxJSON int, limit time.Duration) perfCase {
	return perfCase{
		name: name,
		fill: func(s *Sheet) {
			for r := 0; r < rows; r++ {
				for c := 0; c < cols; c++ {
					if err := s.Set(r, c, "1"); err != nil {
						panic(err)
					}
				}
			}
		},
		cells:    rows * cols,
		wantRows: rows,
		check: func(t *testing.T, s *Sheet) {
			assertCell(t, s, 0, 0, "1")
			assertCell(t, s, rows-1, cols-1, "1")
		},
		maxXLSX: maxXLSX, maxJSON: maxJSON,
		maxSet: limit, maxWrite: limit, maxOpen: limit, maxJSONTime: limit,
	}
}

func runPerfCase(t *testing.T, tc perfCase) {
	t.Helper()
	wb := NewWorkbook(tc.name)
	sheet, err := wb.AddSheet("Grid")
	if err != nil {
		t.Fatal(err)
	}
	setStart := time.Now()
	tc.fill(sheet)
	setD := time.Since(setStart)

	writeStart := time.Now()
	pkg, err := wb.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	writeD := time.Since(writeStart)

	xml := readPart(t, pkg, "xl/worksheets/sheet1.xml")
	rows := strings.Count(xml, "<row ")

	openStart := time.Now()
	opened, err := Open(pkg)
	if err != nil {
		t.Fatal(err)
	}
	openD := time.Since(openStart)

	jsonStart := time.Now()
	raw, err := wb.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	jsonD := time.Since(jsonStart)

	t.Logf("PERF\t%s\tcells=%d\tset_ms=%d\twrite_ms=%d\topen_ms=%d\txlsx_bytes=%d\tjson_ms=%d\tjson_bytes=%d\trow_tags=%d",
		tc.name, tc.cells, setD.Milliseconds(), writeD.Milliseconds(), openD.Milliseconds(),
		len(pkg), jsonD.Milliseconds(), len(raw), rows)

	if setD > tc.maxSet || writeD > tc.maxWrite || openD > tc.maxOpen || jsonD > tc.maxJSONTime {
		t.Fatalf("time over budget: set %s (max %s), write %s (max %s), open %s (max %s), json %s (max %s)",
			setD, tc.maxSet, writeD, tc.maxWrite, openD, tc.maxOpen, jsonD, tc.maxJSONTime)
	}
	if len(pkg) > tc.maxXLSX {
		t.Fatalf("xlsx is %d bytes, budget %d", len(pkg), tc.maxXLSX)
	}
	if len(raw) > tc.maxJSON {
		t.Fatalf("json is %d bytes, budget %d", len(raw), tc.maxJSON)
	}
	if rows != tc.wantRows {
		t.Fatalf("row tags = %d, want %d", rows, tc.wantRows)
	}
	got, ok := opened.Sheet("Grid")
	if !ok {
		t.Fatal("opened workbook has no Grid sheet")
	}
	tc.check(t, got)
	again, ok := parsed.Sheet("Grid")
	if !ok {
		t.Fatal("parsed snapshot has no Grid sheet")
	}
	tc.check(t, again)
}

func mustSet(t *testing.T, s *Sheet, row, col int, value string) {
	t.Helper()
	if err := s.Set(row, col, value); err != nil {
		t.Fatal(err)
	}
}

func assertCell(t *testing.T, s *Sheet, row, col int, want string) {
	t.Helper()
	got, ok := s.Get(row, col)
	if !ok || got != want {
		t.Fatalf("cell %d,%d = %q ok=%v, want %q", row, col, got, ok, want)
	}
}
