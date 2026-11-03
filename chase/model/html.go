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
	"html"
	"strings"
)

// DocsCSS is a minimal stylesheet for [ToHTML] flowing documents. Browsers
// that request --format html / serve ?format=html get this instead of the
// Marp slide scaffold when the document is rendered from the docmodel.
const DocsCSS = `
.eden-docs { max-width: 45rem; margin: 0 auto; padding: 24px; font: 16px/1.45 system-ui, sans-serif; color: #202124; }
.eden-docs h1 { font-size: 1.75rem; margin: 0.6em 0 0.4em; }
.eden-docs h2 { font-size: 1.4rem; margin: 0.6em 0 0.4em; }
.eden-docs h3 { font-size: 1.15rem; margin: 0.6em 0 0.4em; }
.eden-docs p, .eden-docs ul, .eden-docs ol { margin: 0 0 0.75em; }
.eden-docs blockquote { margin: 0.75em 0; padding: 0.25em 0 0.25em 1em; border-left: 3px solid #bdbdbd; color: #5f6368; font-style: italic; }
.eden-docs pre { background: #f6f8fa; border: 1px solid #e0e0e0; border-radius: 8px; padding: 12px; overflow: auto; }
.eden-docs code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.9em; }
.eden-docs table { border-collapse: collapse; margin: 0.75em 0; width: 100%; }
.eden-docs th, .eden-docs td { border: 1px solid #dadce0; padding: 6px 8px; text-align: left; }
.eden-docs .callout { border-radius: 8px; border: 1px solid #d2e3fc; background: #e8f0fe; padding: 12px; margin: 0.75em 0; }
.eden-docs .callout.warn { background: #fff8e1; border-color: #ffecb3; }
.eden-docs .callout.danger { background: #fce8e6; border-color: #fad2cf; }
.eden-docs .callout.success { background: #e6f4ea; border-color: #ceecd5; }
.eden-docs .todo { list-style: none; padding-left: 0; }
.eden-docs .todo li { margin: 0.35em 0; }
.eden-docs .sheet-embed { border: 1px solid #dadce0; border-radius: 8px; margin: 0.75em 0; overflow: hidden; }
.eden-docs .sheet-embed header { background: #f1f3f4; padding: 8px 12px; font-weight: 600; }
.eden-docs .figure { margin: 0.75em 0; }
.eden-docs .figure figcaption { color: #5f6368; font-size: 0.85em; font-style: italic; margin-top: 6px; }
.eden-docs .file { display: flex; gap: 8px; align-items: center; border: 1px solid #e0e0e0; border-radius: 8px; padding: 10px 12px; background: #f8f9fa; margin: 0.75em 0; }
.eden-docs hr { border: 0; border-top: 1px solid #e0e0e0; margin: 1em 0; }
`

// ToHTML serializes doc as a flowing HTML fragment (no <html> wrapper).
// Used by browsers / HTML clients; Flutter clients should consume JSON and
// paint with the eden_docs package instead.
func ToHTML(doc *Document) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<article class="eden-docs">`)
	b.WriteByte('\n')
	for i, sec := range doc.Sections {
		if i > 0 {
			b.WriteString("<hr class=\"section-break\"/>\n")
		}
		writeHTMLBlocks(&b, doc, sec.Blocks, htmlMode{})
	}
	b.WriteString("</article>\n")
	return b.String()
}

// BlockHTML returns the HTML fragment [ToHTML] emits for bl, byte for byte:
// the per-block form of the flowing writer, for callers that place single
// blocks into their own container (press.RenderModel substitutes v7-only kinds
// into a Marp section this way).
//
// doc supplies context a block alone cannot: a same-document synced block
// resolves its source through doc. A nil doc is tolerated; a synced block then
// renders its labelled placeholder.
//
// The fragment is NOT sanitized. Text is HTML-escaped, but a figure's SVG is
// emitted verbatim (authored interchange markup), so a caller that serves the
// fragment to a browser must run it through its own sanitizer.
func BlockHTML(doc *Document, bl Block) string {
	return blockFragment(doc, bl, htmlMode{})
}

// BlockHTMLEmbedded is [BlockHTML] for a block that is placed INSIDE another
// sectioning container (a paginated unit). It differs in exactly two ways:
//
//   - a sheet embed is a <div class="sheet-embed">, not a <section>, so the
//     paginated unit stays the only <section> in its page and per-unit
//     selectors (and unit counts) are not confused by a nested one;
//   - a formula cell shows its cached value (V), falling back to "=" + F
//     only when no value was stored. A rendered page shows what the formula
//     computed; the flowing editor view keeps showing the formula.
//
// Every other kind is byte-identical to BlockHTML. The same sanitization
// caveat applies.
func BlockHTMLEmbedded(doc *Document, bl Block) string {
	return blockFragment(doc, bl, htmlMode{embedded: true})
}

func blockFragment(doc *Document, bl Block, m htmlMode) string {
	if doc == nil {
		doc = &Document{}
	}
	var b strings.Builder
	writeHTMLBlock(&b, doc, bl, m)
	return b.String()
}

// maxSyncDepth bounds synced-block resolution. A synced block resolves to its
// source block, which may itself be synced; a document that points a synced
// block at itself (or two at each other) would otherwise recurse until the Go
// stack overflows -- a fatal error no caller can recover from. Real documents
// chain one or two deep, and ApplyOps refuses to build a cycle, but
// press.RenderModelJSON renders documents it did not author. Past the limit the
// block renders its labelled placeholder, exactly like an unresolvable ref.
const maxSyncDepth = 8

// htmlMode carries the per-render switches through the recursive writer. The
// zero value is the flowing view ToHTML has always produced.
type htmlMode struct {
	// embedded selects the [BlockHTMLEmbedded] variant.
	embedded bool
	// syncDepth counts synced resolutions on the current path.
	syncDepth int
}

func writeHTMLBlocks(b *strings.Builder, doc *Document, blocks []Block, m htmlMode) {
	for _, bl := range blocks {
		writeHTMLBlock(b, doc, bl, m)
	}
}

func writeHTMLBlock(b *strings.Builder, doc *Document, bl Block, m htmlMode) {
	switch bl.Kind {
	case BlockHeading:
		level := bl.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		fmt.Fprintf(b, "<h%d>%s</h%d>\n", level, htmlRunsOrText(bl), level)
	case BlockParagraph:
		fmt.Fprintf(b, "<p>%s</p>\n", htmlRunsOrText(bl))
	case BlockQuote:
		b.WriteString("<blockquote>")
		fmt.Fprintf(b, "<p>%s</p>", htmlRunsOrText(bl))
		writeHTMLBlocks(b, doc, bl.Children, m)
		b.WriteString("</blockquote>\n")
	case BlockCode:
		fmt.Fprintf(b, "<pre><code class=\"language-%s\">%s</code></pre>\n",
			html.EscapeString(bl.Language), html.EscapeString(bl.Text))
	case BlockMath:
		tag := "span"
		if bl.Display {
			tag = "div"
		}
		fmt.Fprintf(b, "<%s class=\"math\">%s</%s>\n", tag, html.EscapeString(bl.Text), tag)
	case BlockList:
		tag := "ul"
		if bl.Ordered {
			tag = "ol"
		}
		fmt.Fprintf(b, "<%s>\n", tag)
		for _, it := range bl.Items {
			fmt.Fprintf(b, "<li>%s</li>\n", html.EscapeString(it.Text))
		}
		fmt.Fprintf(b, "</%s>\n", tag)
	case BlockTable:
		b.WriteString("<table>\n")
		if len(bl.Headers) > 0 {
			b.WriteString("<thead><tr>")
			for _, h := range bl.Headers {
				fmt.Fprintf(b, "<th>%s</th>", html.EscapeString(h))
			}
			b.WriteString("</tr></thead>\n")
		}
		b.WriteString("<tbody>\n")
		for _, row := range bl.Rows {
			b.WriteString("<tr>")
			for _, cell := range row {
				fmt.Fprintf(b, "<td>%s</td>", html.EscapeString(cell))
			}
			b.WriteString("</tr>\n")
		}
		b.WriteString("</tbody></table>\n")
	case BlockImage:
		alt := html.EscapeString(bl.Text)
		src := html.EscapeString(bl.Src)
		title := ""
		if bl.Title != "" {
			title = fmt.Sprintf(` title="%s"`, html.EscapeString(bl.Title))
		}
		fmt.Fprintf(b, `<figure class="figure"><img src="%s" alt="%s"%s/><figcaption>%s</figcaption></figure>`+"\n",
			src, alt, title, alt)
	case BlockToggle:
		open := ""
		if !bl.Collapsed {
			open = " open"
		}
		fmt.Fprintf(b, "<details%s><summary>%s</summary>\n", open, htmlRunsOrText(bl))
		writeHTMLBlocks(b, doc, bl.Children, m)
		b.WriteString("</details>\n")
	case BlockSynced:
		ref := ""
		if bl.SyncRef != nil {
			ref = bl.SyncRef.BlockID
			if bl.SyncRef.PageID == "" && m.syncDepth < maxSyncDepth {
				if src := findBlockPtr(doc, bl.SyncRef.BlockID); src != nil {
					m.syncDepth++
					writeHTMLBlock(b, doc, *src, m)
					return
				}
			}
		}
		fmt.Fprintf(b, `<p class="synced">Synced block %s</p>`+"\n", html.EscapeString(ref))
	case BlockTodo:
		checked := ""
		if bl.Checked {
			checked = " checked"
		}
		b.WriteString(`<ul class="todo"><li>`)
		fmt.Fprintf(b, `<label><input type="checkbox" disabled%s/> %s</label>`, checked, htmlRunsOrText(bl))
		b.WriteString("</li></ul>\n")
		writeHTMLBlocks(b, doc, bl.Children, m)
	case BlockCallout:
		tone := bl.Tone
		if tone == "" {
			tone = "info"
		}
		fmt.Fprintf(b, `<aside class="callout %s">`, html.EscapeString(tone))
		if bl.Icon != "" {
			fmt.Fprintf(b, `<span class="icon">%s</span> `, html.EscapeString(bl.Icon))
		}
		b.WriteString(htmlRunsOrText(bl))
		writeHTMLBlocks(b, doc, bl.Children, m)
		b.WriteString("</aside>\n")
	case BlockDivider:
		b.WriteString("<hr/>\n")
	case BlockFile:
		name := bl.Name
		if name == "" {
			name = bl.Text
		}
		if name == "" {
			name = bl.Src
		}
		fmt.Fprintf(b, `<div class="file"><a href="%s">%s</a></div>`+"\n",
			html.EscapeString(bl.Src), html.EscapeString(name))
	case BlockSheet:
		name := ""
		if bl.SheetEmbed != nil {
			name = bl.SheetEmbed.Sheet
		}
		tag := "section"
		if m.embedded {
			tag = "div"
		}
		fmt.Fprintf(b, `<%s class="sheet-embed">`, tag)
		fmt.Fprintf(b, "<header>%s</header>\n", html.EscapeString(orDefault(name, "Sheet")))
		if bl.SheetEmbed != nil && bl.SheetEmbed.Snapshot != nil {
			writeHTMLSheetSnapshot(b, bl.SheetEmbed.Snapshot, m)
		} else {
			b.WriteString("<p>Live sheet reference (no offline snapshot)</p>\n")
		}
		fmt.Fprintf(b, "</%s>\n", tag)
	case BlockFigure:
		b.WriteString(`<figure class="figure">`)
		if bl.SVG != "" {
			// SVG is authored interchange markup from the design tool path.
			b.WriteString(bl.SVG)
		} else if bl.Src != "" {
			alt := bl.Text
			if alt == "" {
				alt = bl.Title
			}
			fmt.Fprintf(b, `<img src="%s" alt="%s"/>`, html.EscapeString(bl.Src), html.EscapeString(alt))
		}
		cap := bl.Text
		if cap == "" {
			cap = bl.Title
		}
		if cap != "" {
			fmt.Fprintf(b, "<figcaption>%s</figcaption>", html.EscapeString(cap))
		}
		b.WriteString("</figure>\n")
	default:
		if t := htmlRunsOrText(bl); t != "" {
			fmt.Fprintf(b, "<p>%s</p>\n", t)
		}
		writeHTMLBlocks(b, doc, bl.Children, m)
	}
}

// Snapshot grid limits. A snapshot is a sparse, offline subset of a sheet, but
// the writer emits a dense grid out to the largest populated cell, so a single
// cell at {R: 1<<30} would make it loop (and allocate) without bound. The
// JSON a caller renders may not be its own, so cells beyond these limits are
// not rendered. The limits are far above any embed a person would author.
const (
	maxSnapshotRows = 1000
	maxSnapshotCols = 100
)

func writeHTMLSheetSnapshot(b *strings.Builder, snap *SheetSnapshot, m htmlMode) {
	if snap == nil || len(snap.Cells) == 0 {
		b.WriteString("<p>Empty sheet snapshot</p>\n")
		return
	}
	maxR, maxC := 0, 0
	grid := map[[2]int]string{}
	for _, c := range snap.Cells {
		if c.R < 0 || c.C < 0 || c.R >= maxSnapshotRows || c.C >= maxSnapshotCols {
			continue
		}
		if c.R > maxR {
			maxR = c.R
		}
		if c.C > maxC {
			maxC = c.C
		}
		v := c.V
		if c.F != "" && (!m.embedded || v == "") {
			v = "=" + c.F
		}
		grid[[2]int{c.R, c.C}] = v
	}
	b.WriteString("<table>\n")
	for r := 0; r <= maxR; r++ {
		b.WriteString("<tr>")
		for c := 0; c <= maxC; c++ {
			fmt.Fprintf(b, "<td>%s</td>", html.EscapeString(grid[[2]int{r, c}]))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}

func htmlRunsOrText(bl Block) string {
	if len(bl.Runs) > 0 {
		return renderRunsHTML(bl.Runs)
	}
	return html.EscapeString(bl.Text)
}

func renderRunsHTML(runs []TextRun) string {
	var b strings.Builder
	for _, r := range runs {
		t := html.EscapeString(r.Text)
		for _, m := range r.Marks {
			switch m {
			case "bold":
				t = "<strong>" + t + "</strong>"
			case "italic":
				t = "<em>" + t + "</em>"
			case "code":
				t = "<code>" + t + "</code>"
			case "strike":
				t = "<s>" + t + "</s>"
			}
		}
		if r.Link != "" {
			t = fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(r.Link), t)
		}
		b.WriteString(t)
	}
	return b.String()
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
