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

// Package xlsx renders a chase/model.Document to an Excel .xlsx workbook,
// built DIRECTLY from the docmodel as hand-rolled OOXML SpreadsheetML -- zero
// rendered HTML, zero chromedp, zero Node. Sibling of convert/pptx and
// convert/docx, with the same Chrome-free posture and deterministic packager.
//
// A spreadsheet is fundamentally tables, so this exporter is driven by the
// docmodel's table blocks: each section carrying at least one table becomes
// one worksheet, named from that section's first heading. A section with no
// table is skipped rather than producing an empty sheet, and a document with
// no tables anywhere falls back to a single sheet carrying the prose (a
// workbook with zero sheets is invalid).
//
// Table blocks exist in the docmodel only because of schema v3 (EPD-R1):
// before it, a GFM table's content was absent from the model entirely, which
// made this exporter impossible rather than merely unwritten.
//
// The load-bearing detail is cell typing. A numeric cell is written as a real
// spreadsheet number, not text -- otherwise the workbook cannot be summed,
// sorted or charted, and exporting to .xlsx rather than a table in a document
// would buy nothing. See isNumeric in sheet.go for what deliberately does NOT
// count as a number.
package xlsx

import (
	"fmt"

	"go.aocyber.ai/eden-press/chase/model"
)

// Options configures ToXLSX.
type Options struct {
	// Title sets the workbook's dc:title core property. When empty, the
	// first level-1 outline entry is used, then "Workbook".
	Title string
}

// ToXLSX renders doc to an .xlsx byte slice, directly from the docmodel.
// Output is deterministic: the same document twice yields identical bytes.
func ToXLSX(doc *model.Document, opts Options) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("xlsx: ToXLSX: doc is nil")
	}

	type sheet struct {
		name string
		grid sheetRows
	}
	var sheets []sheet

	for i, sec := range doc.Sections {
		grid, hasTable := sectionGrid(sec)
		if !hasTable {
			continue
		}
		sheets = append(sheets, sheet{name: sheetName(sec, i+1), grid: grid})
	}

	// A workbook MUST contain at least one sheet; Excel reports a sheet-less
	// package as corrupt. Falling back to the document's prose keeps the
	// export honest (the user asked for a spreadsheet of a document that
	// happened to have no tables) instead of failing or emitting a husk.
	if len(sheets) == 0 {
		sheets = append(sheets, sheet{name: "Document", grid: textGrid(doc)})
	}

	names := make([]string, len(sheets))
	for i, s := range sheets {
		names[i] = s.name
	}
	names = dedupeSheetNames(names)

	title := opts.Title
	if title == "" {
		title = inferTitle(doc)
	}

	packed := make([]packedSheet, len(sheets))
	for i, s := range sheets {
		packed[i] = packedSheet{name: names[i], grid: s.grid}
	}
	return packWorkbook(title, packed)
}

// packedSheet is one worksheet ready for the OPC packager.
type packedSheet struct {
	name string
	grid sheetRows
}

// packWorkbook writes the OOXML package. sheets must be non-empty: a workbook
// with zero worksheets is not a valid xlsx file.
func packWorkbook(title string, sheets []packedSheet) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx: workbook has no sheets")
	}
	if title == "" {
		title = "Workbook"
	}

	overrides := []contentTypeOverride{
		{PartName: "/xl/workbook.xml", ContentType: ctWorkbook},
		{PartName: "/xl/styles.xml", ContentType: ctStyles},
		{PartName: "/docProps/core.xml", ContentType: ctCorePropsType},
		{PartName: "/docProps/app.xml", ContentType: ctExtendedType},
	}

	// Worksheet relationships start at rId1; styles takes the id after the
	// last sheet, so the numbering stays contiguous and predictable.
	var sheetRefs []sheetRef
	var sheetParts []part
	for i, s := range sheets {
		num := i + 1
		name := fmt.Sprintf("xl/worksheets/sheet%d.xml", num)
		sheetRefs = append(sheetRefs, sheetRef{
			Name:    s.name,
			SheetID: num,
			RelID:   fmt.Sprintf("rId%d", num),
			Target:  fmt.Sprintf("worksheets/sheet%d.xml", num),
		})
		sheetParts = append(sheetParts, part{name: name, content: buildSheetXML(s.grid)})
		overrides = append(overrides, contentTypeOverride{PartName: "/" + name, ContentType: ctWorksheet})
	}

	parts := []part{
		{name: "[Content_Types].xml", content: buildContentTypesXML(overrides)},
		{name: "_rels/.rels", content: rootRelsXML()},
		{name: "docProps/core.xml", content: docPropsCoreXML(title)},
		{name: "docProps/app.xml", content: docPropsAppXML()},
		{name: "xl/workbook.xml", content: workbookXML(sheetRefs)},
		{name: "xl/_rels/workbook.xml.rels", content: workbookRelsXML(sheetRefs)},
		{name: "xl/styles.xml", content: stylesXML()},
	}
	parts = append(parts, sheetParts...)

	return buildZip(parts)
}

// inferTitle picks a workbook title from the outline, then any heading.
func inferTitle(doc *model.Document) string {
	for _, o := range doc.Outline {
		if o.Level == 1 && o.Text != "" {
			return o.Text
		}
	}
	for _, s := range doc.Sections {
		for _, b := range s.Blocks {
			if b.Kind == model.BlockHeading && b.Text != "" {
				return b.Text
			}
		}
	}
	return "Workbook"
}
