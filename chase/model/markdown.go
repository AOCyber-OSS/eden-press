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
		b.WriteString(bl.Text)
		b.WriteString("\n\n")
	case BlockParagraph:
		b.WriteString(pad)
		b.WriteString(bl.Text)
		b.WriteString("\n\n")
	case BlockQuote:
		b.WriteString("> ")
		b.WriteString(strings.ReplaceAll(bl.Text, "\n", "\n> "))
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
	default:
		if bl.Text != "" {
			b.WriteString(bl.Text)
			b.WriteString("\n\n")
		}
		writeBlocks(b, doc, bl.Children, depth)
	}
}

func needsYAMLQuotes(v string) bool {
	return strings.ContainsAny(v, ":#{}[],&*?|>!%@`") || strings.TrimSpace(v) != v
}
