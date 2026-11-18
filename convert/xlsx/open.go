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
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Import limits. Open reads untrusted uploads (press.ImportWorkbookXLSXJSON is
// public API), and a zip part's compression ratio is the attacker's choice: a
// few hundred kilobytes can inflate to gigabytes.
//
// The main defence is the compression ratio (MaxRatio): a bomb is a part that
// inflates far beyond its compressed size, and it is refused while it is being
// read, long before any byte cap. The byte, part and cell caps sit well above
// any realistic workbook and bound what a package that passes the ratio can
// cost. A caller with tighter needs (a small upload limit, a small server)
// passes stricter Limits to OpenWithLimits.
const (
	// MaxPartBytes caps the inflated size of any single part. A full-height
	// Excel column of short values is ~40 MB of sheet XML; this is 12x that.
	MaxPartBytes = 512 << 20
	// MaxTotalBytes caps the inflated bytes of all parts Open reads together.
	MaxTotalBytes = 1 << 30
	// MaxParts caps the number of entries in the package. A workbook this
	// package writes has a dozen; a real one with media rarely has hundreds.
	MaxParts = 10000
	// MaxCells caps the cells Open builds across all sheets, and the entries
	// of the shared-strings table. Ten million is Google Sheets' own
	// per-spreadsheet ceiling; Excel's grid (1,048,576 x 16,384) is not a
	// practical bound, since Excel itself runs out of memory long before it.
	// Parsed, a cell costs about 70 bytes live and 700 bytes allocated in
	// passing (TestImportMemoryPerCell), so a package at this cap holds
	// roughly 0.7 GB live.
	MaxCells = 10_000_000
	// MaxRatio caps a part's inflated bytes at this multiple of its
	// compressed size. Measured on generated realistic workbooks, sheet parts
	// compress about 5-20x (TestRealisticWorkbookImportsAtDefaults); a bomb
	// is hundreds to a thousand times. Parts that inflate to no more than
	// RatioFloorBytes are exempt, since a small, very regular part (an empty
	// sheet, a styles table) can compress past any ratio and costs nothing.
	MaxRatio = 200
	// RatioFloorBytes is the inflated size below which MaxRatio is not applied.
	RatioFloorBytes = 1 << 20
)

// Limits are the bounds one import enforces. A zero field takes the default
// (the Max* constant of the same name), so Limits{Cells: 100_000} tightens one
// bound and keeps the rest.
type Limits struct {
	// PartBytes caps the inflated bytes of one part (default MaxPartBytes).
	PartBytes int64
	// TotalBytes caps the inflated bytes of all parts read (MaxTotalBytes).
	TotalBytes int64
	// Parts caps the entries in the package (MaxParts).
	Parts int
	// Cells caps the cells built and the shared strings parsed (MaxCells).
	Cells int
	// Ratio caps inflated bytes at Ratio x compressed bytes for any part that
	// inflates past RatioFloorBytes (MaxRatio).
	Ratio int64
}

// DefaultLimits returns the limits Open applies.
func DefaultLimits() Limits {
	return Limits{PartBytes: MaxPartBytes, TotalBytes: MaxTotalBytes, Parts: MaxParts, Cells: MaxCells, Ratio: MaxRatio}
}

// withDefaults fills every zero field from DefaultLimits.
func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.PartBytes <= 0 {
		l.PartBytes = d.PartBytes
	}
	if l.TotalBytes <= 0 {
		l.TotalBytes = d.TotalBytes
	}
	if l.Parts <= 0 {
		l.Parts = d.Parts
	}
	if l.Cells <= 0 {
		l.Cells = d.Cells
	}
	if l.Ratio <= 0 {
		l.Ratio = d.Ratio
	}
	return l
}

// ErrLimitExceeded matches (errors.Is) every refusal for an import limit.
var ErrLimitExceeded = errors.New("xlsx: package exceeds import limit")

// LimitError is the refusal for an import limit. It names the limit and its
// value so a caller can tell its user why the file was refused; errors.Is
// matches it to ErrLimitExceeded.
type LimitError struct {
	// Limit is which bound was hit: "part bytes", "total bytes", "parts",
	// "cells" or "compression ratio".
	Limit string
	// Value is that limit's value in force (bytes, a count, or the ratio).
	Value int64
	// Part names the zip part being read, when there was one.
	Part string
}

func (e *LimitError) Error() string {
	where := ""
	if e.Part != "" {
		where = fmt.Sprintf(" (part %q)", e.Part)
	}
	switch e.Limit {
	case "compression ratio":
		return fmt.Sprintf("xlsx: workbook refused: a part inflates more than %dx its compressed size%s, the compression ratio limit", e.Value, where)
	case "part bytes", "total bytes":
		return fmt.Sprintf("xlsx: workbook refused: over the %s limit of %d bytes%s", e.Limit, e.Value, where)
	default:
		return fmt.Sprintf("xlsx: workbook refused: over the %s limit of %d%s", e.Limit, e.Value, where)
	}
}

// Is makes errors.Is(err, ErrLimitExceeded) true for every LimitError.
func (e *LimitError) Is(target error) bool { return target == ErrLimitExceeded }

// partReader reads parts of one package under one set of limits.
type partReader struct {
	files map[string]*zip.File
	lim   Limits
	// pkgBytes is the package's own size, which no part's compressed size can
	// honestly exceed.
	pkgBytes int64
	// used counts the inflated bytes handed out so far, against TotalBytes.
	used int64
	// cellsLeft is the cell budget not yet spent, across every sheet.
	cellsLeft int
}

// Open reads an .xlsx package back into a Workbook so a caller can keep
// editing it, under DefaultLimits. Cells this package writes — inline strings,
// numbers, and formulas — come back through Get. Shared strings are resolved
// when the package has xl/sharedStrings.xml. A cached formula result (<v> next
// to <f>) is ignored: the formula is the cell.
func Open(b []byte) (*Workbook, error) {
	return OpenWithLimits(b, Limits{})
}

// OpenWithLimits is Open under lim; a zero field of lim takes its default. A
// package over a limit is refused with a *LimitError.
func OpenWithLimits(b []byte, lim Limits) (*Workbook, error) {
	return openWithLimits(b, lim.withDefaults())
}

func openWithLimits(b []byte, lim Limits) (*Workbook, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("xlsx: Open: empty package")
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("xlsx: Open: %w", err)
	}
	if len(zr.File) > lim.Parts {
		return nil, &LimitError{Limit: "parts", Value: int64(lim.Parts)}
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	pr := &partReader{files: files, lim: lim, pkgBytes: int64(len(b)), cellsLeft: lim.Cells}

	wbXML, err := pr.read("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	sheets, err := parseWorkbookSheets(wbXML)
	if err != nil {
		return nil, err
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx: Open: workbook has no sheets")
	}

	relsXML, err := pr.read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	targets, err := parseWorksheetRels(relsXML)
	if err != nil {
		return nil, err
	}

	var shared []string
	if raw, ok, err := pr.readOptional("xl/sharedStrings.xml"); err != nil {
		return nil, err
	} else if ok {
		shared, err = parseSharedStrings(raw, lim.Cells)
		if err != nil {
			return nil, err
		}
	}

	title := "Workbook"
	if raw, ok, err := pr.readOptional("docProps/core.xml"); err != nil {
		return nil, err
	} else if ok {
		if t := parseCoreTitle(raw); t != "" {
			title = t
		}
	}

	wb := NewWorkbook(title)
	for _, sh := range sheets {
		target, ok := targets[sh.rel]
		if !ok {
			return nil, fmt.Errorf("xlsx: Open: sheet %q relationship %s has no target", sh.name, sh.rel)
		}
		path := worksheetPath(target)
		raw, err := pr.read(path)
		if err != nil {
			return nil, fmt.Errorf("xlsx: Open: sheet %q: %w", sh.name, err)
		}
		sheet, err := wb.AddSheet(sh.name)
		if err != nil {
			return nil, err
		}
		if err := fillSheet(sheet, raw, shared, &pr.cellsLeft, lim.Cells); err != nil {
			return nil, fmt.Errorf("xlsx: Open: sheet %q: %w", sh.name, err)
		}
	}
	return wb, nil
}

type workbookSheetRef struct {
	name string
	rel  string
}

func (pr *partReader) read(name string) ([]byte, error) {
	f, ok := pr.files[name]
	if !ok {
		return nil, fmt.Errorf("xlsx: Open: missing part %q", name)
	}

	// The tightest bound on this part's inflated bytes, and the error for
	// passing it.
	max, over := pr.lim.PartBytes, &LimitError{Limit: "part bytes", Value: pr.lim.PartBytes, Part: name}
	if left := pr.lim.TotalBytes - pr.used; left < max {
		max, over = left, &LimitError{Limit: "total bytes", Value: pr.lim.TotalBytes, Part: name}
	}
	compressed := int64(f.CompressedSize64)
	if compressed > pr.pkgBytes || compressed < 0 {
		compressed = pr.pkgBytes
	}
	if byRatio := compressed * pr.lim.Ratio; byRatio < max && byRatio/pr.lim.Ratio == compressed {
		if byRatio < RatioFloorBytes {
			byRatio = RatioFloorBytes
		}
		if byRatio < max {
			max, over = byRatio, &LimitError{Limit: "compression ratio", Value: pr.lim.Ratio, Part: name}
		}
	}

	// The declared size is checked first so an honest bomb costs nothing,
	// but it is the attacker's to write: the bound below is on the bytes
	// actually inflated.
	if f.UncompressedSize64 > uint64(max) {
		return nil, over
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, over
	}
	pr.used += int64(len(b))
	return b, nil
}

// readOptional is read for a part the package may omit: ok is false only when
// the part is absent. Any other failure -- a limit above all -- is an error,
// never a part quietly treated as missing.
func (pr *partReader) readOptional(name string) (b []byte, ok bool, err error) {
	if _, present := pr.files[name]; !present {
		return nil, false, nil
	}
	b, err = pr.read(name)
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func parseWorkbookSheets(raw []byte) ([]workbookSheetRef, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var out []workbookSheetRef
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("workbook.xml: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			continue
		}
		ref := workbookSheetRef{}
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "name":
				ref.name = a.Value
			case "id":
				ref.rel = a.Value
			}
		}
		if ref.name == "" || ref.rel == "" {
			return nil, fmt.Errorf("xlsx: Open: sheet element missing name or relationship id")
		}
		out = append(out, ref)
	}
}

func parseWorksheetRels(raw []byte) (map[string]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	out := map[string]string{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("workbook.xml.rels: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		var id, typ, target string
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "Id":
				id = a.Value
			case "Type":
				typ = a.Value
			case "Target":
				target = a.Value
			}
		}
		if strings.HasSuffix(typ, "/worksheet") && id != "" {
			out[id] = target
		}
	}
}

func worksheetPath(target string) string {
	target = strings.TrimPrefix(target, "/")
	if strings.HasPrefix(target, "xl/") {
		return target
	}
	return "xl/" + target
}

func parseCoreTitle(raw []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "title" {
			continue
		}
		var text string
		if err := dec.DecodeElement(&text, &start); err != nil {
			return ""
		}
		return text
	}
}

// errTooManyCells is the cell-budget refusal, for parseSharedStrings and
// fillSheet.
func errTooManyCells(max int) error {
	return &LimitError{Limit: "cells", Value: int64(max)}
}

func parseSharedStrings(raw []byte, max int) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var out []string
	inSI := false
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("sharedStrings.xml: %w", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if tok.Name.Local == "si" {
				inSI = true
				b.Reset()
			}
		case xml.EndElement:
			if tok.Name.Local == "si" && inSI {
				if len(out) >= max {
					return nil, errTooManyCells(max)
				}
				out = append(out, b.String())
				inSI = false
			}
		case xml.CharData:
			if inSI {
				b.Write(tok)
			}
		}
	}
}

func fillSheet(sheet *Sheet, raw []byte, shared []string, cellsLeft *int, maxCells int) error {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var (
		inCell   bool
		inInline bool
		ref      string
		typ      string
		style    int
		hasStyle bool
		formula  string
		value    string
		inline   string
	)
	flush := func() error {
		if !inCell {
			return nil
		}
		defer func() {
			inCell = false
			ref, typ, formula, value, inline = "", "", "", "", ""
			hasStyle = false
			style = 0
		}()
		if *cellsLeft <= 0 {
			return errTooManyCells(maxCells)
		}
		*cellsLeft--
		col, row, ok := parseCellRef(ref)
		if !ok {
			return fmt.Errorf("cell reference %q", ref)
		}
		text, isFormula, err := cellText(typ, formula, value, inline, shared)
		if err != nil {
			return err
		}
		if isFormula {
			if err := sheet.Set(row, col, "="+text); err != nil {
				return err
			}
		} else if err := sheet.Set(row, col, text); err != nil {
			return err
		}
		if hasStyle {
			header, align := presentationFromStyle(style)
			if header {
				if err := sheet.SetHeader(row, true); err != nil {
					return err
				}
			}
			if align != "" {
				if err := sheet.SetAlign(col, align); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return flush()
		}
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			switch tok.Name.Local {
			case "col":
				if err := applyColumnStyle(sheet, tok); err != nil {
					return err
				}
			case "c":
				if err := flush(); err != nil {
					return err
				}
				inCell = true
				for _, a := range tok.Attr {
					switch a.Name.Local {
					case "r":
						ref = a.Value
					case "t":
						typ = a.Value
					case "s":
						n, err := strconv.Atoi(a.Value)
						if err != nil {
							return fmt.Errorf("style index %q", a.Value)
						}
						style = n
						hasStyle = true
					}
				}
			case "is":
				if inCell {
					inInline = true
				}
			case "f":
				if inCell {
					var text string
					if err := dec.DecodeElement(&text, &tok); err != nil {
						return err
					}
					formula = text
				}
			case "v":
				if inCell && !inInline {
					var text string
					if err := dec.DecodeElement(&text, &tok); err != nil {
						return err
					}
					value = text
				}
			case "t":
				if inCell && inInline {
					var text string
					if err := dec.DecodeElement(&text, &tok); err != nil {
						return err
					}
					inline += text
				}
			}
		case xml.EndElement:
			switch tok.Name.Local {
			case "is":
				inInline = false
			case "c":
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}

// applyColumnStyle reads a <col> and stores its alignment. min and max are
// 1-based. Header weight is not taken from a column: bold belongs to rows.
func applyColumnStyle(sheet *Sheet, tok xml.StartElement) error {
	min, max := 0, 0
	style := 0
	hasStyle := false
	for _, a := range tok.Attr {
		switch a.Name.Local {
		case "min":
			n, err := strconv.Atoi(a.Value)
			if err != nil {
				return fmt.Errorf("column min %q", a.Value)
			}
			min = n
		case "max":
			n, err := strconv.Atoi(a.Value)
			if err != nil {
				return fmt.Errorf("column max %q", a.Value)
			}
			max = n
		case "style":
			n, err := strconv.Atoi(a.Value)
			if err != nil {
				return fmt.Errorf("column style %q", a.Value)
			}
			style = n
			hasStyle = true
		}
	}
	if !hasStyle || min < 1 || max < min {
		return nil
	}
	_, align := presentationFromStyle(style)
	if align == "" {
		return nil
	}
	if max > maxSheetCols {
		max = maxSheetCols
	}
	for col := min - 1; col < max; col++ {
		if err := sheet.SetAlign(col, align); err != nil {
			return err
		}
	}
	return nil
}

func cellText(typ, formula, value, inline string, shared []string) (string, bool, error) {
	if formula != "" {
		return formula, true, nil
	}
	switch typ {
	case "inlineStr":
		return inline, false, nil
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || idx < 0 || idx >= len(shared) {
			return "", false, fmt.Errorf("shared string index %q", value)
		}
		return shared[idx], false, nil
	default:
		return value, false, nil
	}
}

func presentationFromStyle(style int) (header bool, align string) {
	switch style {
	case xfBold:
		return true, ""
	case xfRight:
		return false, "right"
	case xfCenter:
		return false, "center"
	case xfBoldRight:
		return true, "right"
	case xfBoldCenter:
		return true, "center"
	default:
		return false, ""
	}
}

// parseCellRef converts an A1-style reference to 0-based indexes.
func parseCellRef(ref string) (col, row int, ok bool) {
	if ref == "" {
		return 0, 0, false
	}
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		col = col*26 + int(ref[i]-'A'+1)
		i++
	}
	if i == 0 || i == len(ref) {
		return 0, 0, false
	}
	col--
	r, err := strconv.Atoi(ref[i:])
	if err != nil || r < 1 {
		return 0, 0, false
	}
	return col, r - 1, true
}
