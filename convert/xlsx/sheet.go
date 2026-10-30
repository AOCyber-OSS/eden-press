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
	"fmt"
	"strconv"
	"strings"

	"go.aocyber.ai/eden-press/chase/model"
)

// escapeXML escapes the five XML metacharacters. Cell text is untrusted (an
// AODex document is LLM-written from a user's conversation); a single
// unescaped "&" makes Excel report the workbook as corrupt.
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

// columnRef converts a 0-based column index to its spreadsheet letters
// (0 -> "A", 25 -> "Z", 26 -> "AA"). This is bijective base-26, NOT ordinary
// base-26: there is no zero digit, so each step subtracts one before taking
// the remainder. Getting that wrong is the classic spreadsheet-writer bug,
// producing "A" followed by "BA" at the 26 -> 27 rollover.
func columnRef(idx int) string {
	if idx < 0 {
		idx = 0
	}
	var sb []byte
	for idx >= 0 {
		sb = append([]byte{byte('A' + idx%26)}, sb...)
		idx = idx/26 - 1
	}
	return string(sb)
}

// isNumeric reports whether s should be written as a spreadsheet NUMBER rather
// than text. This is the difference between a workbook you can sum, sort and
// chart and one that merely looks like a table.
//
// The rule is NOT defined here. It is published as executable data in
// conformance/corpus/xlsxtyping, because AODex's workbook exporter must
// classify identically and a prose contract between two repositories rots. The
// corpus drives TestNumericDetection; change the corpus, not this comment.
//
// Deliberately stricter than strconv.ParseFloat alone:
//   - A value longer than one character that starts with "0" or "+" and holds
//     no "." is an authored IDENTIFIER, not a quantity. "007" is the headline:
//     ParseFloat accepts it, so an unguarded writer stores the number 7 and
//     the leading zeros are gone. The "." escape hatch is load-bearing in both
//     directions -- "0.4" and "+3.5" are quantities and stay numbers, and
//     dropping it would turn every fractional value in every document into
//     text, a far larger blast radius than the bug being fixed. Length > 1
//     keeps a bare "0" a number.
//   - "NaN" and "Inf" parse as floats in Go but are not spreadsheet numbers;
//     writing them into a <v> element produces a cell Excel cannot evaluate.
//   - "1.2%", "550ms" and "1,200" are rejected. A percentage needs a number
//     format (not a bare value), a unit suffix is not a number at all, and
//     thousands separators are locale-dependent -- guessing at any of them
//     would silently corrupt the value.
//
// Surrounding whitespace is not meaningful and is ignored.
func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// The identifier guard, mirroring AODex's coerceInput exactly (decision
	// D6: the Dart rule wins for user data, because its input is a cell a
	// person authored). Both halves of the condition matter -- see above.
	if len(s) > 1 && (s[0] == '0' || s[0] == '+') && !strings.Contains(s, ".") {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "nan") || strings.Contains(lower, "inf") {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// Cell format indices into styles.xml's <cellXfs>. These are a CONTRACT with
// parts_static.go: the numbers are written into every cell as s="N", so the
// order of the <xf> children there is not cosmetic. Append, never insert.
const (
	xfDefault    = 0 // General alignment, regular weight
	xfBold       = 1 // header rows
	xfRight      = 2
	xfCenter     = 3
	xfBoldRight  = 4
	xfBoldCenter = 5

	// xfCount is the number of <xf> children stylesXML must emit. Excel
	// rejects the workbook when <cellXfs count> disagrees with reality, and
	// says nothing useful about why.
	xfCount = 6
)

// styleIndex picks the cell format for a cell from whether its row is a header
// and what its column's alignment is.
//
// "left" deliberately maps to the default (General) index rather than to an
// explicit horizontal="left". Excel's General alignment already left-aligns
// text, which is what a left-marked column holds; emitting an explicit left
// would ALSO override Excel's right-alignment of numbers, which is a change no
// one asked for and which the shared typing corpus has not agreed. This is a
// deliberate divergence from convert/docx, which does emit w:jc for left --
// Word has no content-dependent default alignment to preserve, so there the
// explicit value costs nothing.
func styleIndex(header bool, align string) int {
	switch align {
	case "right":
		if header {
			return xfBoldRight
		}
		return xfRight
	case "center":
		if header {
			return xfBoldCenter
		}
		return xfCenter
	}
	if header {
		return xfBold
	}
	return xfDefault
}

// alignAt reads the alignment for column col, tolerating a short or absent
// slice. Rows are preserved as authored by the docmodel, so a row can be wider
// than its table's Aligns; an unguarded index would panic on real input.
func alignAt(aligns []string, col int) string {
	if col < 0 || col >= len(aligns) {
		return ""
	}
	return aligns[col]
}

// cellXML renders one <c> cell at ref.
//
// Numbers are written as a typed value; everything else as an inline string.
// Inline strings are used in preference to a shared-string table: the table
// would add a second part, a second relationship and a global index every
// sheet must agree on, all to save bytes this exporter does not care about --
// and every extra shared index is another chance to emit a workbook that opens
// with the wrong text in the wrong cell.
func cellXML(ref, text string, xf int) string {
	style := ""
	if xf != xfDefault {
		// s="N" selects a <xf> from styles.xml's <cellXfs>.
		style = fmt.Sprintf(` s="%d"`, xf)
	}
	if isNumeric(text) {
		return fmt.Sprintf(`<c r="%s"%s><v>%s</v></c>`, ref, style, strings.TrimSpace(text))
	}
	if text == "" {
		return fmt.Sprintf(`<c r="%s"%s/>`, ref, style)
	}
	return fmt.Sprintf(`<c r="%s"%s t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
		ref, style, escapeXML(text))
}

// formulaCell reports whether grid cell (r, c) is a formula. A nil or short
// formula grid means "no" — the document exporter never sets one.
func formulaCell(g sheetRows, r, c int) bool {
	if r < 0 || r >= len(g.formula) || c < 0 || c >= len(g.formula[r]) {
		return false
	}
	return g.formula[r][c]
}

// formulaXML writes a formula cell. formula is the Excel expression without
// the leading "=". No cached <v> is emitted: this writer does not evaluate,
// and a stale cached value would lie about the formula until Excel opened it.
func formulaXML(ref, formula string, xf int) string {
	style := ""
	if xf != xfDefault {
		style = fmt.Sprintf(` s="%d"`, xf)
	}
	return fmt.Sprintf(`<c r="%s"%s><f>%s</f></c>`, ref, style, escapeXML(formula))
}

// sheetRows is an ordered grid of cell text plus a flag marking which rows are
// header rows. A header row is rendered bold (style xf 1), and when the FIRST
// row of the sheet is a header it is also frozen -- see freezesHeader for why
// only the first row qualifies.
type sheetRows struct {
	rows      [][]string
	headerRow []bool
	// aligns is per ROW, not per sheet: one section can stack several tables
	// with different column counts and different alignments, so each row
	// carries its own table's Aligns (nil when the table specified none).
	aligns [][]string
	// formula, when non-nil, marks cells whose text is an Excel formula
	// WITHOUT the leading "=". Nil (the document exporter's path) leaves
	// every cell a value, so a table that happens to contain "=" stays text.
	formula [][]bool
	// rowIndex, when non-nil, is the 0-based sheet row for each entry in
	// rows. The workbook writer sets it so a cell on the last row does not
	// pull every row above it into the file. Nil means rows are contiguous
	// from row 0, which is the document exporter.
	rowIndex []int
	// freezeHeader forces a frozen first row. The workbook writer sets it
	// from the sheet, because a sparse rowIndex makes headerRow[0] the
	// first occupied row rather than sheet row 0.
	freezeHeader bool
	// cols is sheet-level column alignment. The workbook writer sets it so
	// an empty cell in an aligned column inherits the style in Excel.
	// Document export leaves it nil: stacked tables carry alignment per row.
	cols []string
}

func (s *sheetRows) add(cells []string, header bool) {
	s.addAligned(cells, header, nil)
}

func (s *sheetRows) addAligned(cells []string, header bool, aligns []string) {
	s.rows = append(s.rows, cells)
	s.headerRow = append(s.headerRow, header)
	s.aligns = append(s.aligns, aligns)
}

// freezesHeader reports whether this sheet should emit a frozen pane.
//
// The pane splits at ySplit="1", so it freezes row 1 and nothing else. That
// makes the question narrower than "does the sheet have a header anywhere":
// stacked tables put a second header partway down, and freezing row 1 on a
// sheet whose row 1 is a data row would pin an arbitrary record to the top of
// the user's view -- worse than not freezing at all.
func (s sheetRows) freezesHeader() bool {
	if s.freezeHeader {
		return true
	}
	if s.rowIndex != nil {
		return false
	}
	return len(s.headerRow) > 0 && s.headerRow[0]
}

// buildSheetXML renders a worksheet part from a grid.
//
// Empty cells with the default style are omitted. A blank authored row, such
// as the separator between stacked tables, stays an empty <row> so the gap
// survives. Column alignment from g.cols is written as <cols>, which Excel
// applies to cells that were never stored.
func buildSheetXML(g sheetRows) []byte {
	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(fmt.Sprintf(`<worksheet xmlns="%s">`, nsSpreadsheet))

	// Child order is fixed by the worksheet schema: dimension, sheetViews,
	// cols, sheetData. A pane emitted after the data is silently ignored.
	if ref := sheetDimension(g); ref != "" {
		b.WriteString(`<dimension ref="` + ref + `"/>`)
	}
	if g.freezesHeader() {
		b.WriteString(`<sheetViews><sheetView workbookViewId="0">`)
		b.WriteString(`<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>`)
		b.WriteString(`</sheetView></sheetViews>`)
	}
	b.WriteString(colsXML(g.cols))

	b.WriteString(`<sheetData>`)
	for r, cells := range g.rows {
		rowNum := r
		if g.rowIndex != nil {
			rowNum = g.rowIndex[r]
		}
		b.WriteString(fmt.Sprintf(`<row r="%d">`, rowNum+1))
		header := r < len(g.headerRow) && g.headerRow[r]
		var aligns []string
		if r < len(g.aligns) {
			aligns = g.aligns[r]
		}
		for c, text := range cells {
			xf := styleIndex(header, alignAt(aligns, c))
			isFormula := formulaCell(g, r, c)
			if text == "" && !isFormula && xf == xfDefault {
				continue
			}
			ref := columnRef(c) + strconv.Itoa(rowNum+1)
			if isFormula {
				b.WriteString(formulaXML(ref, text, xf))
				continue
			}
			b.WriteString(cellXML(ref, text, xf))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return []byte(b.String())
}

// sheetDimension is the bounding box of the cells buildSheetXML will write.
func sheetDimension(g sheetRows) string {
	minR, maxR := -1, -1
	minC, maxC := -1, -1
	consider := func(row, col int) {
		if minR < 0 || row < minR {
			minR = row
		}
		if row > maxR {
			maxR = row
		}
		if minC < 0 || col < minC {
			minC = col
		}
		if col > maxC {
			maxC = col
		}
	}
	for i, cells := range g.rows {
		rowNum := i
		if g.rowIndex != nil {
			rowNum = g.rowIndex[i]
		}
		header := i < len(g.headerRow) && g.headerRow[i]
		var aligns []string
		if i < len(g.aligns) {
			aligns = g.aligns[i]
		}
		for c, text := range cells {
			xf := styleIndex(header, alignAt(aligns, c))
			if text == "" && !formulaCell(g, i, c) && xf == xfDefault {
				continue
			}
			consider(rowNum, c)
		}
	}
	if minR < 0 {
		return ""
	}
	start := columnRef(minC) + strconv.Itoa(minR+1)
	end := columnRef(maxC) + strconv.Itoa(maxR+1)
	if start == end {
		return start
	}
	return start + ":" + end
}

// colsXML writes <cols> for right- and center-aligned columns. "left" and
// "" stay General, which is Excel's default, so they produce no <col>.
func colsXML(aligns []string) string {
	var b strings.Builder
	open := false
	for i, align := range aligns {
		xf := xfDefault
		switch align {
		case "right":
			xf = xfRight
		case "center":
			xf = xfCenter
		default:
			continue
		}
		if !open {
			b.WriteString(`<cols>`)
			open = true
		}
		b.WriteString(fmt.Sprintf(`<col min="%d" max="%d" style="%d"/>`, i+1, i+1, xf))
	}
	if !open {
		return ""
	}
	b.WriteString(`</cols>`)
	return b.String()
}

// sectionGrid flattens one section's table blocks into a single rectangular
// grid. Multiple tables in one section are stacked with a blank separator row;
// each keeps its own header. Rows are padded and truncated to the owning
// table's column count -- the docmodel reports rows exactly as authored, and a
// ragged sheet would put values under the wrong headers.
func sectionGrid(sec model.Section) (sheetRows, bool) {
	var g sheetRows
	any := false
	for _, blk := range sec.Blocks {
		if blk.Kind != model.BlockTable {
			continue
		}
		cols := len(blk.Headers)
		for _, r := range blk.Rows {
			if len(blk.Headers) == 0 && len(r) > cols {
				cols = len(r)
			}
		}
		if cols == 0 {
			continue
		}
		if any {
			g.add(nil, false) // blank separator between stacked tables
		}
		any = true
		// blk.Aligns is per-column and travels with every row of ITS table,
		// so a second table stacked below keeps its own alignments.
		if len(blk.Headers) > 0 {
			g.addAligned(normalize(blk.Headers, cols), true, blk.Aligns)
		}
		for _, r := range blk.Rows {
			g.addAligned(normalize(r, cols), false, blk.Aligns)
		}
	}
	return g, any
}

// normalize pads or truncates cells to exactly n entries.
func normalize(cells []string, n int) []string {
	out := make([]string, n)
	copy(out, cells)
	return out
}

// textGrid is the fallback for a document containing no tables at all: a
// workbook must have at least one sheet, and an empty one is worse than a
// sparse one carrying the document's prose. One line per text-bearing block.
func textGrid(doc *model.Document) sheetRows {
	var g sheetRows
	for _, sec := range doc.Sections {
		for _, blk := range sec.Blocks {
			switch blk.Kind {
			case model.BlockHeading:
				g.add([]string{blk.Text}, true)
			case model.BlockParagraph, model.BlockQuote, model.BlockCode:
				if strings.TrimSpace(blk.Text) != "" {
					g.add([]string{blk.Text}, false)
				}
			case model.BlockList:
				for _, it := range blk.Items {
					g.add([]string{strings.Repeat("    ", it.Level) + it.Text}, false)
				}
			}
		}
	}
	if len(g.rows) == 0 {
		g.add([]string{""}, false)
	}
	return g
}

// sheetName derives a sheet name from a section's first heading, then
// sanitizes it to Excel's rules: no : \ / ? * [ ], at most 31 characters, and
// non-empty. Excel refuses to open a workbook that breaks any of them.
func sheetName(sec model.Section, index int) string {
	raw := ""
	for _, blk := range sec.Blocks {
		if blk.Kind == model.BlockHeading && strings.TrimSpace(blk.Text) != "" {
			raw = blk.Text
			break
		}
	}
	if raw == "" {
		return fmt.Sprintf("Sheet%d", index)
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			return -1
		}
		return r
	}, raw)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return fmt.Sprintf("Sheet%d", index)
	}
	return truncateRunes(cleaned, 31)
}

// truncateRunes caps s at n runes (never bytes -- Excel's limit is characters,
// and slicing a multi-byte name mid-rune would emit invalid UTF-8).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// dedupeSheetNames makes every name unique, since Excel rejects a workbook with
// two identically-named sheets. Collisions get a " (2)", " (3)" suffix, with
// the base trimmed so the result still fits in 31 characters.
func dedupeSheetNames(names []string) []string {
	seen := make(map[string]int, len(names))
	out := make([]string, len(names))
	for i, n := range names {
		candidate := n
		for k := 2; seen[strings.ToLower(candidate)] > 0; k++ {
			suffix := fmt.Sprintf(" (%d)", k)
			candidate = truncateRunes(n, 31-len([]rune(suffix))) + suffix
		}
		seen[strings.ToLower(candidate)]++
		out[i] = candidate
	}
	return out
}
