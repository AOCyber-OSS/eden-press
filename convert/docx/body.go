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

package docx

import (
	"fmt"
	"strings"

	"go.aocyber.ai/eden-press/chase/model"
)

// escapeXML escapes the five XML metacharacters in user-supplied text. Every
// string that reaches document.xml passes through here: document titles and
// body text are untrusted input (an AODex document is written by an LLM from a
// user's conversation), and an unescaped "&" alone is enough to make Word
// reject the whole package as corrupt.
//
// encoding/xml's EscapeText is deliberately not used: it also escapes newlines
// and tabs as character references, which would corrupt the code-block runs
// that rely on literal whitespace under xml:space="preserve".
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

// runXML renders one <w:r> text run. preserve keeps significant leading and
// trailing whitespace (Word collapses it otherwise), which code blocks need
// and prose does not.
func runXML(text string, preserve bool) string {
	space := ""
	if preserve {
		space = ` xml:space="preserve"`
	}
	return fmt.Sprintf(`<w:r><w:t%s>%s</w:t></w:r>`, space, escapeXML(text))
}

// paraXML renders one <w:p> with an optional pStyle and pre-rendered inner
// content (runs and/or additional pPr children).
func paraXML(style, extraPPr, inner string) string {
	var pPr string
	if style != "" || extraPPr != "" {
		pPr = "<w:pPr>"
		if style != "" {
			pPr += fmt.Sprintf(`<w:pStyle w:val="%s"/>`, style)
		}
		pPr += extraPPr + "</w:pPr>"
	}
	return "<w:p>" + pPr + inner + "</w:p>"
}

// headingStyle maps a docmodel heading level onto Word's built-in Heading1..6
// style ids. Levels outside 1..6 are clamped: markdown permits only h1-h6, but
// the model's Level is a plain int and a malformed document must not produce a
// dangling style reference Word cannot resolve.
func headingStyle(level int) string {
	switch {
	case level < 1:
		level = 1
	case level > 6:
		level = 6
	}
	return fmt.Sprintf("Heading%d", level)
}

// renderBlock renders one docmodel Block as WordprocessingML.
func renderBlock(b model.Block) string {
	switch b.Kind {
	case model.BlockHeading:
		return paraXML(headingStyle(b.Level), "", runXML(b.Text, false))

	case model.BlockParagraph:
		return paraXML("", "", runXML(b.Text, false))

	case model.BlockQuote:
		// EPD-R1 made this distinguishable from prose upstream; here is where
		// that pays off -- an indented, italic Quote style rather than a
		// paragraph indistinguishable from the surrounding body.
		var sb strings.Builder
		sb.WriteString(renderMultilineParagraph("Quote", b.Text))
		sb.WriteString(renderChildBlocks(b.Children))
		return sb.String()

	case model.BlockCode:
		return renderCode(b)

	case model.BlockList:
		return renderList(b)

	case model.BlockTable:
		return renderTable(b)

	case model.BlockImage:
		return renderImage(b)

	case model.BlockMath:
		// Math is emitted as its RAW TeX in the Code style rather than
		// converted to OMML. Converting TeX to Office MathML is a genuine
		// sub-project (the same one press/math solves for MathML), and a
		// silently dropped equation is worse than a visible verbatim one.
		// Upgrading this to real OMML is tracked as follow-up work.
		return renderMultilineParagraph("Code", b.Text)

	case model.BlockToggle:
		// Flatten: title as a heading-like paragraph, then children in order.
		var sb strings.Builder
		if strings.TrimSpace(b.Text) != "" {
			sb.WriteString(paraXML(headingStyle(3), "", runXML(b.Text, false)))
		}
		sb.WriteString(renderChildBlocks(b.Children))
		return sb.String()

	case model.BlockSynced:
		// Synced instances are host-resolved; DOCX flattens them away.
		return ""

	case model.BlockTodo:
		mark := "☐ "
		if b.Checked {
			mark = "☑ "
		}
		var sb strings.Builder
		sb.WriteString(paraXML("", "", runXML(mark+b.Text, false)))
		sb.WriteString(renderChildBlocks(b.Children))
		return sb.String()

	case model.BlockCallout:
		var sb strings.Builder
		sb.WriteString(renderMultilineParagraph("Quote", b.Text))
		sb.WriteString(renderChildBlocks(b.Children))
		return sb.String()

	case model.BlockDivider:
		return paraXML("", "", runXML("―", false))

	case model.BlockFile:
		name := b.Name
		if name == "" {
			name = b.Text
		}
		if name == "" {
			name = b.Src
		}
		return paraXML("Caption", "", runXML(fmt.Sprintf("[%s](%s)", name, b.Src), false))

	case model.BlockSheet:
		if b.SheetEmbed != nil && b.SheetEmbed.Snapshot != nil {
			return renderSheetSnapshot(b.SheetEmbed.Snapshot)
		}
		sheet := ""
		if b.SheetEmbed != nil {
			sheet = b.SheetEmbed.Sheet
		}
		return paraXML("Caption", "", runXML("[sheet:"+sheet+"]", false))

	case model.BlockFigure:
		alt := b.Text
		if alt == "" {
			alt = b.Title
		}
		if alt == "" {
			alt = "figure"
		}
		if b.Src != "" {
			return paraXML("Caption", "", runXML(fmt.Sprintf("[%s: %s]", alt, b.Src), false))
		}
		return paraXML("Caption", "", runXML("["+alt+"]", false))

	default:
		// An unknown future block kind degrades to its text rather than
		// vanishing -- the model is versioned and may grow kinds this
		// exporter predates.
		var sb strings.Builder
		if strings.TrimSpace(b.Text) != "" {
			sb.WriteString(paraXML("", "", runXML(b.Text, false)))
		}
		sb.WriteString(renderChildBlocks(b.Children))
		return sb.String()
	}
}

func renderChildBlocks(children []model.Block) string {
	var sb strings.Builder
	for _, c := range children {
		sb.WriteString(renderBlock(c))
	}
	return sb.String()
}

func renderSheetSnapshot(snap *model.SheetSnapshot) string {
	if snap == nil || len(snap.Cells) == 0 {
		return ""
	}
	maxR, maxC := 0, 0
	grid := map[[2]int]string{}
	for _, c := range snap.Cells {
		if c.R > maxR {
			maxR = c.R
		}
		if c.C > maxC {
			maxC = c.C
		}
		v := c.V
		if c.F != "" {
			v = "=" + c.F
		}
		grid[[2]int{c.R, c.C}] = v
	}
	rows := make([][]string, 0, maxR+1)
	for r := 0; r <= maxR; r++ {
		row := make([]string, maxC+1)
		for c := 0; c <= maxC; c++ {
			row[c] = grid[[2]int{r, c}]
		}
		rows = append(rows, row)
	}
	headers := rows[0]
	body := rows[1:]
	return renderTable(model.Block{Kind: model.BlockTable, Headers: headers, Rows: body})
}

// renderMultilineParagraph renders text as a single styled paragraph, mapping
// embedded newlines onto <w:br/> so multi-paragraph quotes and multi-line math
// keep their line structure inside one block.
func renderMultilineParagraph(style, text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var inner strings.Builder
	for i, ln := range lines {
		if i > 0 {
			inner.WriteString("<w:br/>")
		}
		inner.WriteString(runXML(ln, true))
	}
	return paraXML(style, "", inner.String())
}

// renderCode renders a fenced/indented code block. Word has no <pre>: each
// source line becomes its own run separated by <w:br/>, under xml:space=
// "preserve" so indentation survives. Joining the lines into one run would
// collapse the entire listing onto a single line.
func renderCode(b model.Block) string {
	return renderMultilineParagraph("Code", b.Text)
}

// renderList renders a list Block as one ListParagraph per item, each carrying
// the numPr (numId + ilvl) that binds it to a numbering definition in
// numbering.xml. numId 1 is bullets and numId 2 is decimal -- the two
// instances parts_static.go emits, and the only two this exporter references.
func renderList(b model.Block) string {
	numID := 1
	if b.Ordered {
		numID = 2
	}
	var sb strings.Builder
	for _, item := range b.Items {
		lvl := item.Level
		if lvl < 0 {
			lvl = 0
		}
		if lvl > 8 { // numbering.xml defines ilvl 0..8.
			lvl = 8
		}
		numPr := fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, lvl, numID)
		sb.WriteString(paraXML("ListParagraph", numPr, runXML(item.Text, false)))
	}
	return sb.String()
}

// renderTable renders an EPD-R1 table Block as a real Word table.
//
// The column count is fixed by the header row (or by the widest body row when
// the table has no header), and every row is padded or truncated to it. The
// docmodel deliberately reports rows exactly as authored -- normalizing is an
// exporter's decision -- but WordprocessingML has no tolerance for a ragged
// <w:tbl>: a row with the wrong <w:tc> count makes Word report the document as
// corrupt, so the normalization has to happen here.
func renderTable(b model.Block) string {
	cols := len(b.Headers)
	for _, r := range b.Rows {
		if len(r) > cols && len(b.Headers) == 0 {
			cols = len(r)
		}
	}
	if cols == 0 {
		return ""
	}

	// Equal-width columns across a 9360-twip (6.5in) text block -- US Letter
	// with 1in margins, which is also within A4's printable width.
	const tableWidth = 9360
	colWidth := tableWidth / cols

	var sb strings.Builder
	sb.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/>`)
	sb.WriteString(fmt.Sprintf(`<w:tblW w:w="%d" w:type="dxa"/>`, tableWidth))
	sb.WriteString(`<w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	for i := 0; i < cols; i++ {
		sb.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, colWidth))
	}
	sb.WriteString(`</w:tblGrid>`)

	if len(b.Headers) > 0 {
		sb.WriteString(renderTableRow(b.Headers, cols, colWidth, b.Aligns, true))
	}
	for _, row := range b.Rows {
		sb.WriteString(renderTableRow(row, cols, colWidth, b.Aligns, false))
	}
	sb.WriteString(`</w:tbl>`)

	// A table immediately followed by another table (or ending the body) needs
	// a trailing empty paragraph, or Word merges them / rejects the body.
	sb.WriteString("<w:p/>")
	return sb.String()
}

// renderTableRow renders one <w:tr>, normalized to exactly cols cells.
func renderTableRow(cells []string, cols, colWidth int, aligns []string, header bool) string {
	var sb strings.Builder
	sb.WriteString("<w:tr>")
	if header {
		// tblHeader repeats this row at the top of every page the table spans.
		sb.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
	}
	for i := 0; i < cols; i++ {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		jc := ""
		if i < len(aligns) && aligns[i] != "" {
			jc = fmt.Sprintf(`<w:jc w:val="%s"/>`, aligns[i])
		}
		run := runXML(text, false)
		if header {
			run = fmt.Sprintf(`<w:r><w:rPr><w:b/></w:rPr><w:t>%s</w:t></w:r>`, escapeXML(text))
		}
		sb.WriteString("<w:tc>")
		sb.WriteString(fmt.Sprintf(`<w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>`, colWidth))
		sb.WriteString(paraXML("", jc, run))
		sb.WriteString("</w:tc>")
	}
	sb.WriteString("</w:tr>")
	return sb.String()
}

// renderImage renders an image Block as a captioned placeholder rather than an
// embedded picture.
//
// Embedding the real bytes would mean fetching the image: a docmodel image
// carries only a URL (or a document-relative path), and this package has no
// I/O, no network, and no base directory to resolve against -- all three are
// the caller's context, not the exporter's. Silently dropping the image would
// lose content, so the alt text and source are emitted visibly. Accepting a
// caller-supplied image resolver is the natural follow-up.
func renderImage(b model.Block) string {
	label := b.Text
	if label == "" {
		label = b.Title
	}
	if label == "" {
		label = "image"
	}
	caption := fmt.Sprintf("[%s: %s]", label, b.Src)
	return paraXML("Caption", "", runXML(caption, false))
}

// renderSections renders every section's blocks in order, inserting a page
// break between consecutive sections unless continuous is set. A Section is a
// slide/page boundary upstream (profiles/slides splits on "---"), so honoring
// it as a page break is the faithful default for a paged format.
func renderSections(sections []model.Section, continuous bool) string {
	var sb strings.Builder
	for i, s := range sections {
		if i > 0 && !continuous {
			sb.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
		for _, b := range s.Blocks {
			sb.WriteString(renderBlock(b))
		}
	}
	return sb.String()
}
