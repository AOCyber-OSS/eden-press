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
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"go.aocyber.ai/eden-press/chase/model"
)

// illegalXML is one of every XML-1.0-illegal class: NUL, other C0 controls,
// vertical tab, form feed, ESC (ANSI colour), U+FFFE, U+FFFF, and a UTF-8
// encoded lone surrogate (invalid UTF-8 in a Go string).
const illegalXML = "\x00\x01\x08\x0b\x0c\x0e\x1b\x1f￾￿\xed\xa0\x80"

// legalXMLChar is the XML 1.0 Char production.
func legalXMLChar(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

// assertStrictParts checks every part of the package the way a strict
// consumer does: valid UTF-8, no illegal rune, and a full strict decode.
func assertStrictParts(t *testing.T, pkg []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		s := string(raw)
		parts[f.Name] = s
		if !utf8.ValidString(s) {
			t.Errorf("%s: invalid UTF-8", f.Name)
		}
		for i, r := range s {
			if !legalXMLChar(r) {
				t.Errorf("%s: illegal rune %U at byte %d", f.Name, r, i)
				break
			}
		}
		dec := xml.NewDecoder(strings.NewReader(s))
		dec.Strict = true
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s: strict XML decode: %v", f.Name, err)
				break
			}
		}
	}
	return parts
}

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
