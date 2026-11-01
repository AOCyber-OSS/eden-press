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

// slide.go turns one chase/model.Section into one ppt/slides/slideN.xml (+ its
// .rels -> slideLayout1): the title comes from the Section's lowest-Level
// Outline heading (06-01), the body from Section.Blocks in document order
// (paragraph -> body run, list -> bulleted/numbered paragraphs, heading ->
// heading text box, code/math -> raw-text body run). Body shapes are wrapped in
// a single identity <p:grpSp> (06-02) so every content-bearing slide carries
// the criterion-3 grouped-shape case. It reads Section.Blocks directly -- never
// HTML, never chromedp -- and composes shapes via shapes.go's builders.
package pptx

import (
	"fmt"
	"strings"

	"go.aocyber.ai/eden-press/chase/model"
)

// Default slide layout geometry, all in EMU via 06-02 conversions. These are
// the writer's placement CONTRACT (asserted in tests), not a pixel-perfect
// design system: a left/right margin, a title band near the top, and a body
// region below it whose shapes stack vertically with a fixed gap.
var (
	slideMarginX = Inches(0.5)  // left (and mirrored right) margin
	titleTopY    = Inches(0.3)  // title band top
	titleHeightY = Inches(1.2)  // title band height
	bodyTopY     = Inches(1.7)  // body region top (below the title band)
	bodyGapY     = Inches(0.15) // vertical gap between stacked body shapes
	paraHeightY  = Inches(0.6)  // a paragraph/code/math body shape's height
	headingHght  = Inches(0.7)  // a secondary-heading body shape's height
	listLineY    = Inches(0.3)  // per-item height contribution of a list shape
	listPaddingY = Inches(0.2)  // fixed padding added to a list shape's height
)

// Font sizes in POINTS; emitted as centipoints (06-02 Centipoints) in a:rPr/@sz.
const (
	titlePt   = 44.0 // title text-box run
	headingPt = 28.0 // secondary-heading body run
	bodyPt    = 18.0 // paragraph / list-item / code / math body run
)

// sectionTitle returns the title text for section: the Text of its
// lowest-Level Outline entry (h1 beats h2), first-wins on a Level tie. ok is
// false when the section has no Outline heading at all (an untitled slide --
// the caller must then emit no title shape and fabricate nothing).
func sectionTitle(section model.Section, outline []model.OutlineEntry) (text string, level int, ok bool) {
	for _, e := range outline {
		if e.SectionID != section.ID {
			continue
		}
		if !ok || e.Level < level {
			text, level, ok = e.Text, e.Level, true
		}
	}
	return text, level, ok
}

// skipTitleHeading returns blocks with the FIRST heading Block matching the
// title (same Level and Text) removed -- that heading is rendered as the slide
// title, so it must not ALSO render as a body heading shape. Every OTHER
// heading is preserved (additional headings become body shapes, never dropped).
func skipTitleHeading(blocks []model.Block, level int, text string) []model.Block {
	out := make([]model.Block, 0, len(blocks))
	skipped := false
	for _, b := range blocks {
		if !skipped && b.Kind == model.BlockHeading && b.Level == level && b.Text == text {
			skipped = true
			continue
		}
		out = append(out, b)
	}
	return out
}

// bodyRunText returns the editable run text for a non-list body block: a
// paragraph/heading's Text, or a code/math block's RAW source/TeX (lossless
// body text; syntax highlighting and math typesetting are out of scope here).
func bodyRunText(b model.Block) string {
	return b.Text
}

// buildBodyShapes maps body blocks (title heading already removed) to a stack
// of EMU-placed child text boxes starting at startY, using a shared per-slide
// id generator. It returns the children and bottomY, the slide-EMU bottom edge
// of the last shape (so the enclosing group's ext can span exactly the body).
func buildBodyShapes(blocks []model.Block, gen *shapeIDGen, width, startY int64) (children []textBox, bottomY int64) {
	y := startY
	bottomY = startY
	for _, b := range flattenBlocks(blocks) {
		id := gen.nextID()
		switch b.Kind {
		case model.BlockList:
			bk := bulletChar
			if b.Ordered {
				bk = bulletAutoNum
			}
			paras := make([]paragraph, 0, len(b.Items))
			for _, it := range b.Items {
				paras = append(paras, paragraph{
					text:   it.Text,
					sz:     Centipoints(bodyPt),
					bullet: bk,
					level:  it.Level,
				})
			}
			n := len(b.Items)
			if n == 0 {
				n = 1
			}
			h := listPaddingY + listLineY*int64(n)
			children = append(children, textBox{
				id: id, name: fmt.Sprintf("Content %d", id),
				off: Point{X: slideMarginX, Y: y}, ext: Extent{CX: width, CY: h},
				paras: paras,
			})
			bottomY = y + h
			y = bottomY + bodyGapY
		case model.BlockHeading:
			h := headingHght
			children = append(children, textBox{
				id: id, name: fmt.Sprintf("Heading %d", id),
				off: Point{X: slideMarginX, Y: y}, ext: Extent{CX: width, CY: h},
				paras: []paragraph{{text: b.Text, sz: Centipoints(headingPt), bold: true}},
			})
			bottomY = y + h
			y = bottomY + bodyGapY
		default: // paragraph, code, math, toggle title -> a plain body run
			h := paraHeightY
			children = append(children, textBox{
				id: id, name: fmt.Sprintf("Content %d", id),
				off: Point{X: slideMarginX, Y: y}, ext: Extent{CX: width, CY: h},
				paras: []paragraph{{text: bodyRunText(b), sz: Centipoints(bodyPt)}},
			})
			bottomY = y + h
			y = bottomY + bodyGapY
		}
	}
	return children, bottomY
}

// flattenBlocks expands toggle/quote/todo/callout children into a flat list
// for PPTX (synced instances are skipped; sheet embeds become table-like
// paragraphs from snapshot text; hosts resolve live refs).
func flattenBlocks(blocks []model.Block) []model.Block {
	out := make([]model.Block, 0, len(blocks))
	for _, b := range blocks {
		switch b.Kind {
		case model.BlockSynced:
			continue
		case model.BlockDivider:
			out = append(out, model.Block{Kind: model.BlockParagraph, Text: "—"})
		case model.BlockSheet:
			text := sheetEmbedPlain(b)
			out = append(out, model.Block{Kind: model.BlockParagraph, Text: text})
		case model.BlockFigure:
			alt := b.Text
			if alt == "" {
				alt = b.Title
			}
			if alt == "" {
				alt = "[figure]"
			}
			out = append(out, model.Block{Kind: model.BlockParagraph, Text: alt})
		case model.BlockFile:
			name := b.Name
			if name == "" {
				name = b.Src
			}
			out = append(out, model.Block{Kind: model.BlockParagraph, Text: name})
		case model.BlockToggle, model.BlockTodo, model.BlockCallout:
			title := b
			title.Children = nil
			out = append(out, title)
			out = append(out, flattenBlocks(b.Children)...)
		default:
			leaf := b
			kids := b.Children
			leaf.Children = nil
			out = append(out, leaf)
			if len(kids) > 0 {
				out = append(out, flattenBlocks(kids)...)
			}
		}
	}
	return out
}

func sheetEmbedPlain(b model.Block) string {
	if b.SheetEmbed == nil {
		return "[sheet]"
	}
	if b.SheetEmbed.Snapshot != nil && len(b.SheetEmbed.Snapshot.Cells) > 0 {
		var parts []string
		for _, c := range b.SheetEmbed.Snapshot.Cells {
			if c.V != "" {
				parts = append(parts, c.V)
			} else if c.F != "" {
				parts = append(parts, "="+c.F)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " | ")
		}
	}
	if b.SheetEmbed.Sheet != "" {
		return "[sheet:" + b.SheetEmbed.Sheet + "]"
	}
	return "[sheet]"
}

// buildSlide renders one Section to a ppt/slides/slideN.xml document and its
// sibling slideN.xml.rels (-> slideLayout1). The spTree begins with its own
// mandatory group shape (id 1); the title shape (if any) is id 2; body shapes
// are wrapped in a single identity <p:grpSp> so a content-bearing slide always
// carries the grouped-shape case.
func buildSlide(section model.Section, outline []model.OutlineEntry, size SlideSize) (slideXML []byte, relsXML []byte) {
	gen := newShapeIDGen()
	width := size.CX - 2*slideMarginX

	var sb strings.Builder
	sb.WriteString(xmlDeclaration)
	fmt.Fprintf(&sb, `<p:sld xmlns:a="%s" xmlns:r="%s" xmlns:p="%s">`, nsDrawingML, nsRelationships, nsPresentationML)
	sb.WriteString(`<p:cSld><p:spTree>`)
	sb.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`)
	sb.WriteString(`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`)

	title, titleLevel, hasTitle := sectionTitle(section, outline)
	bodyBlocks := section.Blocks
	if hasTitle {
		id := gen.nextID()
		buildTextBox(&sb, textBox{
			id: id, name: "Title 1",
			off: Point{X: slideMarginX, Y: titleTopY}, ext: Extent{CX: width, CY: titleHeightY},
			paras: []paragraph{{text: title, sz: Centipoints(titlePt), bold: true}},
		})
		bodyBlocks = skipTitleHeading(section.Blocks, titleLevel, title)
	}

	if len(bodyBlocks) > 0 {
		groupID := gen.nextID()
		children, bottomY := buildBodyShapes(bodyBlocks, gen, width, bodyTopY)
		groupOff := Point{X: slideMarginX, Y: bodyTopY}
		groupExt := Extent{CX: width, CY: bottomY - bodyTopY}
		xf := IdentityGroupTransform(groupOff, groupExt)
		buildGroupShape(&sb, groupShape{id: groupID, name: "Body Group", xf: xf, children: children})
	}

	sb.WriteString(`</p:spTree></p:cSld></p:sld>`)

	rels := buildRelsXML([]relationship{
		{ID: rIDSlideLayout1, Type: relTypeSlideLayout, Target: "../slideLayouts/slideLayout1.xml"},
	})
	return []byte(sb.String()), rels
}
