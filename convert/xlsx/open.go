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
// few hundred kilobytes can inflate to gigabytes. Every part is read through a
// bound, and a package that would pass one is refused with ErrLimitExceeded.
const (
	// MaxPartBytes caps the inflated size of any single part.
	MaxPartBytes = 128 << 20
	// MaxTotalBytes caps the inflated bytes of all parts Open reads together.
	MaxTotalBytes = 256 << 20
	// MaxParts caps the number of entries in the package. A workbook this
	// package writes has a dozen; a real one with media rarely has hundreds.
	MaxParts = 10000
)

// limits is the set of bounds one Open enforces. Open uses defaultLimits; the
// tests drive smaller ones through openWithLimits so a total-size or
// part-count case does not have to inflate hundreds of megabytes.
type limits struct {
	partBytes  int64
	totalBytes int64
	parts      int
}

var defaultLimits = limits{partBytes: MaxPartBytes, totalBytes: MaxTotalBytes, parts: MaxParts}

// partReader reads parts of one package under one set of limits.
type partReader struct {
	files map[string]*zip.File
	lim   limits
	// used counts the inflated bytes handed out so far, against totalBytes.
	used int64
}

// ErrLimitExceeded is returned (wrapped) when a package exceeds an import limit.
var ErrLimitExceeded = errors.New("xlsx: package exceeds import limit")

// Open reads an .xlsx package back into a Workbook so a caller can keep
// editing it. Cells this package writes — inline strings, numbers, and
// formulas — come back through Get. Shared strings are resolved when the
// package has xl/sharedStrings.xml. A cached formula result (<v> next to
// <f>) is ignored: the formula is the cell.
func Open(b []byte) (*Workbook, error) {
	return openWithLimits(b, defaultLimits)
}

func openWithLimits(b []byte, lim limits) (*Workbook, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("xlsx: Open: empty package")
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("xlsx: Open: %w", err)
	}
	if len(zr.File) > lim.parts {
		return nil, fmt.Errorf("xlsx: Open: package has %d parts, over the %d-part limit: %w",
			len(zr.File), lim.parts, ErrLimitExceeded)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	pr := &partReader{files: files, lim: lim}

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
		shared, err = parseSharedStrings(raw)
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
		if err := fillSheet(sheet, raw, shared); err != nil {
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
	max := pr.lim.partBytes
	if left := pr.lim.totalBytes - pr.used; left < max {
		max = left
	}
	f, ok := pr.files[name]
	if !ok {
		return nil, fmt.Errorf("xlsx: Open: missing part %q", name)
	}
	// The declared size is checked first so an honest bomb costs nothing,
	// but it is the attacker's to write: the bound below is on the bytes
	// actually inflated.
	if f.UncompressedSize64 > uint64(max) {
		return nil, pr.overLimit(name)
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
		return nil, pr.overLimit(name)
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

// overLimit names the part and the limits in force when name would exceed one.
func (pr *partReader) overLimit(name string) error {
	return fmt.Errorf("xlsx: Open: part %q exceeds the import limits (%d bytes per part, %d bytes in total): %w",
		name, pr.lim.partBytes, pr.lim.totalBytes, ErrLimitExceeded)
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

func parseSharedStrings(raw []byte) ([]string, error) {
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

func fillSheet(sheet *Sheet, raw []byte, shared []string) error {
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
