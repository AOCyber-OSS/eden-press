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
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Open reads an .xlsx package back into a Workbook so a caller can keep
// editing it. Cells this package writes — inline strings, numbers, and
// formulas — come back through Get. Shared strings are resolved when the
// package has xl/sharedStrings.xml. A cached formula result (<v> next to
// <f>) is ignored: the formula is the cell.
func Open(b []byte) (*Workbook, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("xlsx: Open: empty package")
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("xlsx: Open: %w", err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	wbXML, err := readZipFile(files, "xl/workbook.xml")
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

	relsXML, err := readZipFile(files, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	targets, err := parseWorksheetRels(relsXML)
	if err != nil {
		return nil, err
	}

	var shared []string
	if raw, err := readZipFile(files, "xl/sharedStrings.xml"); err == nil {
		shared, err = parseSharedStrings(raw)
		if err != nil {
			return nil, err
		}
	}

	title := "Workbook"
	if raw, err := readZipFile(files, "docProps/core.xml"); err == nil {
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
		raw, err := readZipFile(files, path)
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

func readZipFile(files map[string]*zip.File, name string) ([]byte, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("xlsx: Open: missing part %q", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return b, nil
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
