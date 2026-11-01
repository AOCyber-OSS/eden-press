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
	"strings"
)

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// SpreadsheetML + OPC namespaces.
const (
	nsSpreadsheet   = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRelationships = "http://schemas.openxmlformats.org/package/2006/relationships"
	nsDocRels       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsContentTypes  = "http://schemas.openxmlformats.org/package/2006/content-types"
	nsCoreProps     = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	nsExtendedProps = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
	nsDublinCore    = "http://purl.org/dc/elements/1.1/"
	nsXSI           = "http://www.w3.org/2001/XMLSchema-instance"
)

const (
	ctWorkbook      = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	ctWorksheet     = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	ctStyles        = "application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"
	ctCorePropsType = "application/vnd.openxmlformats-package.core-properties+xml"
	ctExtendedType  = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
)

const (
	relTypeOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relTypeCoreProps      = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	relTypeExtendedProps  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties"
	relTypeWorksheet      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"
	relTypeStyles         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
)

type contentTypeOverride struct {
	PartName    string
	ContentType string
}

func buildContentTypesXML(overrides []contentTypeOverride) []byte {
	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(fmt.Sprintf(`<Types xmlns="%s">`, nsContentTypes))
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	for _, o := range overrides {
		b.WriteString(fmt.Sprintf(`<Override PartName="%s" ContentType="%s"/>`, o.PartName, o.ContentType))
	}
	b.WriteString(`</Types>`)
	return []byte(b.String())
}

type relationship struct {
	ID     string
	Type   string
	Target string
}

func buildRelsXML(rels []relationship) []byte {
	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(fmt.Sprintf(`<Relationships xmlns="%s">`, nsRelationships))
	for _, r := range rels {
		b.WriteString(fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, r.ID, r.Type, r.Target))
	}
	b.WriteString(`</Relationships>`)
	return []byte(b.String())
}

func rootRelsXML() []byte {
	return buildRelsXML([]relationship{
		{ID: "rId1", Type: relTypeOfficeDocument, Target: "xl/workbook.xml"},
		{ID: "rId2", Type: relTypeCoreProps, Target: "docProps/core.xml"},
		{ID: "rId3", Type: relTypeExtendedProps, Target: "docProps/app.xml"},
	})
}

// sheetRef ties a worksheet's display name and sheetId to the relationship id
// workbook.xml references it by.
type sheetRef struct {
	Name    string
	SheetID int
	RelID   string
	Target  string
}

func workbookXML(sheets []sheetRef, fullCalcOnLoad bool) []byte {
	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(fmt.Sprintf(`<workbook xmlns="%s" xmlns:r="%s">`, nsSpreadsheet, nsDocRels))
	// When formulas are present without cached <v>, force Excel to recalculate
	// on open (Objective 9 / XLSX-F04). Omit the element when there are no
	// formulas so value-only workbooks stay byte-identical to prior exports.
	if fullCalcOnLoad {
		b.WriteString(`<calcPr fullCalcOnLoad="1"/>`)
	}
	b.WriteString(`<sheets>`)
	for _, s := range sheets {
		b.WriteString(fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="%s"/>`,
			escapeXML(s.Name), s.SheetID, s.RelID))
	}
	b.WriteString(`</sheets></workbook>`)
	return []byte(b.String())
}

func workbookRelsXML(sheets []sheetRef) []byte {
	rels := make([]relationship, 0, len(sheets)+1)
	for _, s := range sheets {
		rels = append(rels, relationship{ID: s.RelID, Type: relTypeWorksheet, Target: s.Target})
	}
	// Styles takes the id after the last sheet so numbering stays contiguous.
	rels = append(rels, relationship{
		ID:     fmt.Sprintf("rId%d", len(sheets)+1),
		Type:   relTypeStyles,
		Target: "styles.xml",
	})
	return buildRelsXML(rels)
}

// docPropsCoreXML omits timestamps: dcterms:created/modified would make every
// export of the same document differ, defeating determinism.
func docPropsCoreXML(title string) []byte {
	return []byte(xmlDeclaration + fmt.Sprintf(
		`<cp:coreProperties xmlns:cp="%s" xmlns:dc="%s" xmlns:xsi="%s">`+
			`<dc:title>%s</dc:title><dc:creator>Eden Press</dc:creator>`+
			`<cp:lastModifiedBy>Eden Press</cp:lastModifiedBy></cp:coreProperties>`,
		nsCoreProps, nsDublinCore, nsXSI, escapeXML(title)))
}

func docPropsAppXML() []byte {
	return []byte(xmlDeclaration + fmt.Sprintf(
		`<Properties xmlns="%s"><Application>Eden Press</Application></Properties>`,
		nsExtendedProps))
}

// stylesXML is the minimal style table SpreadsheetML requires, plus the cell
// formats sheet.go references by index: bold for header rows, and the
// alignment variants that carry model.Block.Aligns into the workbook.
//
// The element ORDER here is fixed by the schema (numFmts, fonts, fills,
// borders, cellStyleXfs, cellXfs, ...) -- Excel rejects the workbook if they
// appear in any other sequence, and it is not lenient about the two mandatory
// placeholder fills.
//
// New cell formats are APPENDED to cellXfs, never inserted. Indices 0 and 1
// are load-bearing: every cell already emitted as s="1" means bold, and
// shifting that would silently restyle every workbook. And <cellXfs count>
// must equal the number of <xf> children -- a mismatch is one of the few ways
// to produce a file Excel rejects with no useful message.
func stylesXML() []byte {
	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(fmt.Sprintf(`<styleSheet xmlns="%s">`, nsSpreadsheet))

	// Font 0 = regular, font 1 = bold.
	b.WriteString(`<fonts count="2">`)
	b.WriteString(`<font><sz val="11"/><name val="Calibri"/></font>`)
	b.WriteString(`<font><b/><sz val="11"/><name val="Calibri"/></font>`)
	b.WriteString(`</fonts>`)

	// Fills 0 and 1 are reserved placeholders the schema requires; Excel
	// treats a file lacking them as corrupt even though neither is used.
	b.WriteString(`<fills count="2">`)
	b.WriteString(`<fill><patternFill patternType="none"/></fill>`)
	b.WriteString(`<fill><patternFill patternType="gray125"/></fill>`)
	b.WriteString(`</fills>`)

	b.WriteString(`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>`)
	b.WriteString(`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>`)

	// The cell formats, in the order sheet.go's xf* constants name them:
	//   0 default          1 bold
	//   2 right            3 center
	//   4 bold+right       5 bold+center
	b.WriteString(fmt.Sprintf(`<cellXfs count="%d">`, xfCount))
	b.WriteString(`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>`)
	b.WriteString(`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>`)
	b.WriteString(`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment horizontal="right"/></xf>`)
	b.WriteString(`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment horizontal="center"/></xf>`)
	b.WriteString(`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment horizontal="right"/></xf>`)
	b.WriteString(`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment horizontal="center"/></xf>`)
	b.WriteString(`</cellXfs>`)

	b.WriteString(`</styleSheet>`)
	return []byte(b.String())
}
