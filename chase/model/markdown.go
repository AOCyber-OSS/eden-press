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

package model

import (
	"fmt"
	"strings"
)

// ToMarkdown serializes doc to Marp-compatible Markdown. Nested toggles become
// <details>, synced blocks become HTML comments plus resolved same-doc text
// when possible. Some Marp features (fit, advanced backgrounds) are lossy.
func ToMarkdown(doc *Document) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	if len(doc.Meta.Directives) > 0 {
		b.WriteString("---\n")
		for k, v := range doc.Meta.Directives {
			b.WriteString(k)
			b.WriteString(": ")
			if needsYAMLQuotes(v) {
				b.WriteString(fmt.Sprintf("%q", v))
			} else {
				b.WriteString(v)
			}
			b.WriteByte('\n')
		}
		b.WriteString("---\n\n")
	}
	for i, sec := range doc.Sections {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		for k, v := range sec.Attrs {
			b.WriteString(fmt.Sprintf("<!-- %s: %s -->\n", k, v))
		}
		for _, note := range sec.Notes {
			b.WriteString(fmt.Sprintf("<!-- %s -->\n", note))
		}
		writeBlocks(&b, doc, sec.Blocks, 0)
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func writeBlocks(b *strings.Builder, doc *Document, blocks []Block, depth int) {
	for _, bl := range blocks {
		writeBlock(b, doc, bl, depth)
	}
}

func writeBlock(b *strings.Builder, doc *Document, bl Block, depth int) {
	pad := strings.Repeat(">", depth)
	if depth > 0 {
		pad += " "
	}
	switch bl.Kind {
	case BlockHeading:
		b.WriteString(pad)
		b.WriteString(strings.Repeat("#", bl.Level))
		b.WriteByte(' ')
		b.WriteString(runsOrText(bl))
		b.WriteString("\n\n")
	case BlockParagraph:
		b.WriteString(pad)
		b.WriteString(runsOrText(bl))
		b.WriteString("\n\n")
	case BlockQuote:
		b.WriteString("> ")
		b.WriteString(strings.ReplaceAll(runsOrText(bl), "\n", "\n> "))
		b.WriteString("\n\n")
		writeBlocks(b, doc, bl.Children, depth+1)
	case BlockCode:
		b.WriteString("```")
		b.WriteString(bl.Language)
		b.WriteByte('\n')
		b.WriteString(bl.Text)
		if !strings.HasSuffix(bl.Text, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("```\n\n")
	case BlockMath:
		if bl.Display {
			b.WriteString("$$\n")
			b.WriteString(bl.Text)
			b.WriteString("\n$$\n\n")
		} else {
			b.WriteString("$")
			b.WriteString(bl.Text)
			b.WriteString("$\n\n")
		}
	case BlockList:
		for _, it := range bl.Items {
			indent := strings.Repeat("  ", it.Level)
			mark := "- "
			if bl.Ordered {
				mark = "1. "
			}
			b.WriteString(indent)
			b.WriteString(mark)
			b.WriteString(it.Text)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	case BlockTable:
		if len(bl.Headers) > 0 {
			b.WriteString("| ")
			b.WriteString(strings.Join(bl.Headers, " | "))
			b.WriteString(" |\n| ")
			for i := range bl.Headers {
				if i > 0 {
					b.WriteString(" | ")
				}
				align := ""
				if i < len(bl.Aligns) {
					align = bl.Aligns[i]
				}
				switch align {
				case "center":
					b.WriteString(":---:")
				case "right":
					b.WriteString("---:")
				default:
					b.WriteString("---")
				}
			}
			b.WriteString(" |\n")
		}
		for _, row := range bl.Rows {
			b.WriteString("| ")
			b.WriteString(strings.Join(row, " | "))
			b.WriteString(" |\n")
		}
		b.WriteByte('\n')
	case BlockImage:
		title := ""
		if bl.Title != "" {
			title = fmt.Sprintf(` "%s"`, bl.Title)
		}
		b.WriteString(fmt.Sprintf("![%s](%s%s)\n\n", bl.Text, bl.Src, title))
	case BlockToggle:
		b.WriteString("<details")
		if !bl.Collapsed {
			b.WriteString(" open")
		}
		b.WriteString(">\n<summary>")
		b.WriteString(bl.Text)
		b.WriteString("</summary>\n\n")
		writeBlocks(b, doc, bl.Children, depth)
		b.WriteString("</details>\n\n")
	case BlockSynced:
		page := ""
		if bl.SyncRef != nil {
			page = bl.SyncRef.PageID
			b.WriteString(fmt.Sprintf("<!-- synced: blockId=%s pageId=%s -->\n", bl.SyncRef.BlockID, page))
			if page == "" {
				if src := findBlockPtr(doc, bl.SyncRef.BlockID); src != nil {
					writeBlock(b, doc, *src, depth)
					return
				}
			}
		}
		b.WriteByte('\n')
	case BlockTodo:
		mark := "- [ ] "
		if bl.Checked {
			mark = "- [x] "
		}
		b.WriteString(mark)
		b.WriteString(runsOrText(bl))
		b.WriteString("\n\n")
		writeBlocks(b, doc, bl.Children, depth)
	case BlockCallout:
		b.WriteString("> ")
		if bl.Icon != "" {
			b.WriteString(bl.Icon)
			b.WriteByte(' ')
		}
		b.WriteString(strings.ReplaceAll(runsOrText(bl), "\n", "\n> "))
		b.WriteString("\n\n")
		writeBlocks(b, doc, bl.Children, depth)
	case BlockDivider:
		b.WriteString("---\n\n")
	case BlockFile:
		name := bl.Name
		if name == "" {
			name = bl.Text
		}
		if name == "" {
			name = bl.Src
		}
		b.WriteString(fmt.Sprintf("[%s](%s)\n\n", name, bl.Src))
	case BlockSheet:
		if bl.SheetEmbed != nil && bl.SheetEmbed.Snapshot != nil {
			writeSheetSnapshotTable(b, bl.SheetEmbed.Snapshot)
		} else {
			sheet := ""
			if bl.SheetEmbed != nil {
				sheet = bl.SheetEmbed.Sheet
			}
			b.WriteString(fmt.Sprintf("<!-- eden-sheet: %s -->\n\n", sheet))
		}
	case BlockFigure:
		if bl.SVG != "" {
			b.WriteString(bl.SVG)
			if !strings.HasSuffix(bl.SVG, "\n") {
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
		} else if bl.Src != "" {
			alt := bl.Text
			if alt == "" {
				alt = bl.Title
			}
			title := ""
			if bl.Title != "" {
				title = fmt.Sprintf(` "%s"`, bl.Title)
			}
			b.WriteString(fmt.Sprintf("![%s](%s%s)\n\n", alt, bl.Src, title))
		}
	default:
		if t := runsOrText(bl); t != "" {
			b.WriteString(t)
			b.WriteString("\n\n")
		}
		writeBlocks(b, doc, bl.Children, depth)
	}
}

func runsOrText(bl Block) string {
	if len(bl.Runs) > 0 {
		return renderRunsMarkdown(bl.Runs)
	}
	return bl.Text
}

func renderRunsMarkdown(runs []TextRun) string {
	var b strings.Builder
	for _, r := range runs {
		t := r.Text
		for _, m := range r.Marks {
			switch m {
			case "bold":
				t = "**" + t + "**"
			case "italic":
				t = "*" + t + "*"
			case "code":
				t = "`" + t + "`"
			case "strike":
				t = "~~" + t + "~~"
			}
		}
		if r.Link != "" {
			t = "[" + t + "](" + r.Link + ")"
		}
		b.WriteString(t)
	}
	return b.String()
}

func writeSheetSnapshotTable(b *strings.Builder, snap *SheetSnapshot) {
	if snap == nil || len(snap.Cells) == 0 {
		b.WriteString("<!-- eden-sheet empty -->\n\n")
		return
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
	for r := 0; r <= maxR; r++ {
		cells := make([]string, maxC+1)
		for c := 0; c <= maxC; c++ {
			cells[c] = grid[[2]int{r, c}]
		}
		b.WriteString("| ")
		b.WriteString(strings.Join(cells, " | "))
		b.WriteString(" |\n")
		if r == 0 {
			b.WriteString("| ")
			for i := 0; i <= maxC; i++ {
				if i > 0 {
					b.WriteString(" | ")
				}
				b.WriteString("---")
			}
			b.WriteString(" |\n")
		}
	}
	b.WriteByte('\n')
}

func needsYAMLQuotes(v string) bool {
	return strings.ContainsAny(v, ":#{}[],&*?|>!%@`") || strings.TrimSpace(v) != v
}
