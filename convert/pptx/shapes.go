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

// shapes.go is the SINGLE place a Block-kind -> DrawingML mapping lives (06-04
// key-link): the <p:sp> editable text-box builder, the <a:p> paragraph/list
// builder, and the <p:grpSp> identity group builder that slide.go composes and
// that 06-05 (speaker notes) extends rather than duplicates. Every user string
// (title/paragraph/list-item text, shape name) is routed through xml.EscapeText
// so a `<`/`&`/`"` in source content can never corrupt the OOXML (06-RESEARCH
// escaping trap); all positions/extents are 06-02 EMU values and font sizes are
// 06-02 centipoints (a:rPr/@sz), never EMU.
package pptx

import (
	"encoding/xml"
	"fmt"
	"strings"

	"go.aocyber.ai/eden-press/internal/xmltext"
)

// bulletGlyph is the U+2022 BULLET emitted in an unordered list paragraph's
// <a:buChar char="…"/>. Declared once so the builder and its tests agree on
// the exact character.
const bulletGlyph = "•"

// shapeIDGen hands out the monotonic, per-slide-unique cNvPr ids DrawingML
// requires. The slide's own spTree group shape is always id 1, so shape ids
// start at 2; a SINGLE generator is threaded through every builder on a slide
// (grouped and ungrouped alike) so a group and its children can never collide
// on an id -- the id-uniqueness gotcha this TRD calls out.
type shapeIDGen struct {
	next int
}

// newShapeIDGen returns a generator whose first id is 2 (id 1 is reserved for
// the slide's spTree group).
func newShapeIDGen() *shapeIDGen {
	return &shapeIDGen{next: 2}
}

// nextID returns the next unused shape id.
func (g *shapeIDGen) nextID() int {
	id := g.next
	g.next++
	return id
}

// bulletKind selects a paragraph's list decoration.
type bulletKind int

const (
	// bulletNone is a plain paragraph: no <a:pPr> bullet decoration.
	bulletNone bulletKind = iota
	// bulletChar is an unordered-list item: <a:buChar char="•"/>.
	bulletChar
	// bulletAutoNum is an ordered-list item: <a:buAutoNum type="arabicPeriod"/>.
	bulletAutoNum
)

// paragraph is one <a:p>: a single editable text run plus its font size
// (centipoints), boldness, and optional list decoration/nesting level. v1
// emits exactly one <a:r> run per paragraph.
type paragraph struct {
	// text is the run's user-supplied plain text (escaped on emission).
	text string
	// sz is the run font size in CENTIPOINTS (06-02 Centipoints), the a:rPr/@sz
	// unit -- NOT EMU.
	sz int
	// bold sets a:rPr/@b="1".
	bold bool
	// bullet selects the list decoration (none/char/autonum).
	bullet bulletKind
	// level is the list nesting depth (a:pPr/@lvl); 0 for top level or a
	// non-list paragraph.
	level int
}

// textBox is one editable <p:sp> text-box shape: an EMU-placed rectangle
// (06-02 Point/Extent) carrying one or more paragraphs.
type textBox struct {
	// id is the shape's cNvPr id (from a shapeIDGen).
	id int
	// name is the shape's cNvPr name (escaped on emission).
	name string
	// off/ext are the shape's slide-EMU position/size.
	off Point
	ext Extent
	// paras are the shape's paragraphs, in order.
	paras []paragraph
}

// groupShape is a <p:grpSp> wrapping one or more child text boxes under a
// group transform. 06-04 v1 uses only 06-02's IdentityGroupTransform (chOff==
// off, chExt==ext), so a child's own off/ext are already literal slide EMU and
// render unchanged -- the criterion-3 grouped-shape case, free of coordinate
// scaling math.
type groupShape struct {
	// id is the group shape's own cNvPr id.
	id int
	// name is the group shape's cNvPr name (escaped on emission).
	name string
	// xf is the group's transform (off/ext/chOff/chExt).
	xf GroupTransform
	// children are the grouped text boxes, in order.
	children []textBox
}

// escapeXML returns s with every XML metacharacter escaped via
// xml.EscapeText, so the result is safe both as element content (<a:t>…</a:t>)
// and inside a double-quoted attribute value. This is the ONE escaping seam
// all user-supplied text funnels through. XML-illegal characters are dropped
// first (xmltext.Clean), matching the xlsx and docx writers; on its own
// EscapeText would turn each into a visible U+FFFD (eden-press#39).
func escapeXML(s string) string {
	var b strings.Builder
	// xml.EscapeText only errors if the writer errors; strings.Builder never
	// does, so the error is unreachable here.
	_ = xml.EscapeText(&b, []byte(xmltext.Clean(s)))
	return b.String()
}

// listMarL is the left margin (EMU) of a list paragraph at the given 0-based
// nesting level: one 0.5in step of indentation per level (06-02 Inches), so
// nested items hang progressively further right.
func listMarL(level int) int64 {
	return Inches(0.5) * int64(level+1)
}

// listIndent is the hanging indent (EMU, negative) that pulls the bullet/number
// back to the start of the margin step -- a standard 0.5in hang.
var listIndent = Inches(-0.5)

// buildParagraph appends one <a:p> for p to sb: an optional list <a:pPr>
// (bullet or auto-number, with lvl/marL/indent), then a single <a:r> run whose
// <a:t> carries p.text (escaped).
func buildParagraph(sb *strings.Builder, p paragraph) {
	sb.WriteString("<a:p>")
	switch p.bullet {
	case bulletChar:
		fmt.Fprintf(sb, `<a:pPr lvl="%d" marL="%d" indent="%d"><a:buChar char="%s"/></a:pPr>`,
			p.level, listMarL(p.level), listIndent, bulletGlyph)
	case bulletAutoNum:
		fmt.Fprintf(sb, `<a:pPr lvl="%d" marL="%d" indent="%d"><a:buAutoNum type="arabicPeriod"/></a:pPr>`,
			p.level, listMarL(p.level), listIndent)
	}
	sb.WriteString("<a:r>")
	if p.bold {
		fmt.Fprintf(sb, `<a:rPr lang="en-US" sz="%d" b="1"/>`, p.sz)
	} else {
		fmt.Fprintf(sb, `<a:rPr lang="en-US" sz="%d"/>`, p.sz)
	}
	fmt.Fprintf(sb, `<a:t>%s</a:t>`, escapeXML(p.text))
	sb.WriteString("</a:r>")
	sb.WriteString("</a:p>")
}

// buildTextBox appends a full editable <p:sp> text box for tb to sb: nvSpPr
// (cNvPr id/name, cNvSpPr txBox="1", empty nvPr), spPr (EMU xfrm + rect
// prstGeom), and a txBody whose paragraphs are rendered by buildParagraph.
func buildTextBox(sb *strings.Builder, tb textBox) {
	fmt.Fprintf(sb, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>`,
		tb.id, escapeXML(tb.name))
	sb.WriteString(`<p:spPr>`)
	fmt.Fprintf(sb, `<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`,
		tb.off.X, tb.off.Y, tb.ext.CX, tb.ext.CY)
	sb.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`)
	sb.WriteString(`<p:txBody><a:bodyPr/><a:lstStyle/>`)
	for _, p := range tb.paras {
		buildParagraph(sb, p)
	}
	sb.WriteString(`</p:txBody></p:sp>`)
}

// buildGroupShape appends a <p:grpSp> for g to sb: nvGrpSpPr (cNvPr id/name),
// grpSpPr with the group's <a:xfrm> off/ext/chOff/chExt (from g.xf), then each
// child text box (which, under the identity transform, carries literal slide
// EMU coordinates).
func buildGroupShape(sb *strings.Builder, g groupShape) {
	fmt.Fprintf(sb, `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="%d" name="%s"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`,
		g.id, escapeXML(g.name))
	sb.WriteString(`<p:grpSpPr>`)
	fmt.Fprintf(sb, `<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/><a:chOff x="%d" y="%d"/><a:chExt cx="%d" cy="%d"/></a:xfrm>`,
		g.xf.Off.X, g.xf.Off.Y, g.xf.Ext.CX, g.xf.Ext.CY,
		g.xf.ChOff.X, g.xf.ChOff.Y, g.xf.ChExt.CX, g.xf.ChExt.CY)
	sb.WriteString(`</p:grpSpPr>`)
	for _, c := range g.children {
		buildTextBox(sb, c)
	}
	sb.WriteString(`</p:grpSp>`)
}
