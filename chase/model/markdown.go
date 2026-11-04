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
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// ToMarkdown serializes doc to Marp-compatible Markdown. It is the writing half
// of the lossless Markdown codec (FromMarkdown is the reading half).
//
// The output is DETERMINISTIC: the same Document always yields the same bytes.
// Meta.Directives and Section.Attrs are emitted in sorted key order, and only
// what the Document holds is emitted: nothing is derived from the theme or the
// directives (an earlier probe saw injected data-theme/style comments, but
// those were Section.Attrs the importer had stored, not something this function
// invented).
//
// Spelling of the GFM kinds, each chosen so that FromMarkdown reads it back as
// the same Block:
//   - sections are separated by a `---` rule; a divider BLOCK is `***`, so the
//     two are distinguishable (Marp reads `---` as a slide break);
//   - plain text is backslash-escaped wherever it would otherwise be read as
//     Markdown (see escapeInline); a "\n" inside a paragraph is a hard line
//     break (backslash + newline);
//   - rich text uses `**` bold, `*` italic, `~~` strike, backtick code, `$`
//     inline math (a run marked "math") and `[text](href)` links; a block whose
//     delimiters would not read back (CommonMark's flanking rules) is written
//     with <strong>, <em> and <del> instead (see renderRuns);
//   - lists number their items sequentially per level, indent children by the
//     width of the parent's marker, and alternate the bullet (`-`/`*`) or the
//     ordered delimiter (`.`/`)`) between ADJACENT lists so they do not merge;
//   - a code fence is one backtick longer than any backtick run inside the
//     code, and the code between the fences is exactly Block.Text: a trailing
//     newline in Text is a blank line before the closing fence;
//   - display math is `$$`, TeX, `$$` on separate lines with nothing added;
//   - a table cell is its runs when the block has HeaderRuns/RowRuns for it, and
//     its plain text otherwise. Runs are written exactly as in a paragraph
//     (delimiters, or tags where a delimiter would not read back), checked by
//     reading the cell back inside a one-column table. Every `|` is escaped: `\|` in
//     text, in a link destination, in a code span (GFM un-escapes a code span's
//     pipe before it parses the span) and in math TeX (the reader does the same
//     for TeX inside a cell, so TeX `a|b` is `$a\|b$` there, and TeX `a\|b` is
//     `$a\\|b$`).
//
// Identity holds for Documents in canonical form (see canonRuns and FromMarkdown):
// styled runs have no edge whitespace and no two neighbours share marks and
// link; a list starts at level 0 and never skips a level; Section IDs are 1..n;
// text never starts or ends with a newline or contains a blank line; a paragraph
// is never only one image or only inline math (those are image and math blocks);
// a math run is not followed by a digit; a table's cell runs are in the canonical
// form EnsureTextRunsInvariant gives them (a plain cell is nil, no run holds a
// newline) and every row has as many cells as the header; display math TeX holds no unescaped "$$"
// and inline math TeX no unescaped "$" (TeX reserves it; write \$); a todo's child
// list does not end in an empty item while more children follow it (goldmark closes
// the todo there); and a Document has at least one Section. ToMarkdown still writes
// something sensible for anything else, but the reader may not give the same
// Document back.
//
// Kinds with no GFM spelling:
//   - todo is a GFM task item (`- [ ] text` / `- [x] text`), its children indented
//     under it; consecutive childless todos form one tight list;
//   - toggle, synced, callout, file, sheet and figure are container directives,
//     `:::kind {json}` ... `:::`, the JSON holding the scalar fields, the body the
//     text paragraph and then the children (a figure's body is its SVG,
//     verbatim). A body holding `:::` is wrapped in a longer fence, and prose that
//     starts a line with `:::` is escaped (see renderDirective).
func ToMarkdown(doc *Document) string {
	if doc == nil {
		return ""
	}
	var parts []string
	if len(doc.Meta.Directives) > 0 {
		var fm strings.Builder
		fm.WriteString("---\n")
		for _, k := range sortedMapKeys(doc.Meta.Directives) {
			fm.WriteString(k)
			fm.WriteString(": ")
			fm.WriteString(yamlScalar(doc.Meta.Directives[k]))
			fm.WriteByte('\n')
		}
		fm.WriteString("---")
		parts = append(parts, fm.String())
	}
	var body strings.Builder
	for i, sec := range doc.Sections {
		content := renderSection(doc, sec)
		if i > 0 {
			body.WriteString("\n\n---")
			if content != "" {
				body.WriteString("\n\n")
			}
		}
		body.WriteString(content)
	}
	// Trim newlines only. strings.TrimSpace would also eat a Unicode space that is
	// the first or last character of the document ("\u00a0x", "x\f"), which is
	// content: CommonMark keeps every space that is not ASCII space or tab.
	parts = append(parts, strings.Trim(body.String(), "\n"), renderTrailer(doc))
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n") + "\n"
}

// renderSection writes one section: its attrs (one comment, first), its blocks,
// then its notes (one comment each, in order, last). Notes have no position in
// the model, so they are written in this one place.
//
// Attrs and notes use the same `<!-- ... -->` syntax in Marp, which makes them
// ambiguous, so each has a canonical spelling that names itself and carries its
// payload as JSON: `<!-- eden:attrs {"k":"v"} -->` and `<!-- eden:note "text" -->`.
// json.Marshal escapes `>`, so no payload can hold `-->`, and newlines are
// escaped, so each comment is one line. FromMarkdown reads both and also the
// plain Marp spellings.
func renderSection(doc *Document, sec Section) string {
	var parts []string
	if len(sec.Attrs) > 0 {
		parts = append(parts, "<!-- eden:attrs "+jsonLine(sec.Attrs)+" -->")
	}
	if body := renderBlocks(doc, sec.Blocks); body != "" {
		parts = append(parts, body)
	}
	if len(sec.Notes) > 0 {
		lines := make([]string, len(sec.Notes))
		for i, note := range sec.Notes {
			lines[i] = "<!-- eden:note " + jsonLine(note) + " -->"
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// renderBlocks joins the blocks with exactly one blank line between them, except
// that consecutive todo blocks without children form one tight list. Blocks that
// render to nothing (an empty paragraph) are skipped, so they never leave a
// stray blank line behind.
//
// Two lists written one after the other with a blank line between them are ONE
// list to a Markdown parser unless their markers differ, so the marker
// alternates (`-`/`*`, or `.`/`)` for ordered lists) between adjacent lists. A
// todo is a bullet list item, so the same holds between a todo and a bullet
// list; consecutive todos share a marker, because they are meant to be one list.
func renderBlocks(doc *Document, blocks []Block) string {
	return joinParts(renderParts(doc, blocks))
}

// renderedBlock is one block that rendered to something, with how it joins the
// block before it.
type renderedBlock struct {
	bl    *Block
	s     string
	tight bool // joined to the previous block by one newline, not a blank line
}

// renderParts renders each block once, dropping those that render to nothing.
func renderParts(doc *Document, blocks []Block) []renderedBlock {
	var parts []renderedBlock
	var prev *Block // the last block that rendered to something
	prevAlt := false
	bullet := func(k *Block) bool { return k != nil && (k.Kind == BlockTodo || (k.Kind == BlockList && !k.Ordered)) }
	for i := range blocks {
		bl := &blocks[i]
		alt := false
		switch {
		case bl.Kind == BlockList && bl.Ordered:
			alt = prev != nil && prev.Kind == BlockList && prev.Ordered && !prevAlt
		case bullet(bl) && bullet(prev):
			if bl.Kind == BlockTodo && prev.Kind == BlockTodo {
				alt = prevAlt
			} else {
				alt = !prevAlt
			}
		}
		s := renderBlock(doc, *bl, alt)
		if s == "" {
			continue
		}
		tight := bl.Kind == BlockTodo && prev != nil && prev.Kind == BlockTodo && len(prev.Children) == 0
		parts = append(parts, renderedBlock{bl: bl, s: s, tight: tight})
		prev, prevAlt = bl, alt
	}
	return parts
}

func joinParts(parts []renderedBlock) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			if p.tight {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(p.s)
	}
	return b.String()
}

// renderBlock renders one block without a trailing newline.
func renderBlock(doc *Document, bl Block, flip bool) string {
	switch bl.Kind {
	case BlockHeading:
		level := bl.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		hashes := strings.Repeat("#", level)
		text := escapeHeadingTail(renderRuns(runsOfBlock(bl), inlineFlat))
		if text == "" {
			return hashes
		}
		return hashes + " " + text
	case BlockParagraph:
		return renderRuns(runsOfBlock(bl), inlineBlock)
	case BlockQuote:
		return prefixLines(joinNonEmpty(renderParagraphs(bl), renderBlocks(doc, bl.Children)), "> ")
	case BlockCode:
		return renderCode(bl)
	case BlockMath:
		if bl.Display {
			if bl.Text == "" {
				return "$$\n$$"
			}
			return "$$\n" + bl.Text + "\n$$"
		}
		return "$" + bl.Text + "$"
	case BlockList:
		return renderList(bl, flip)
	case BlockTable:
		return renderTable(bl)
	case BlockImage:
		return renderImage(bl.Text, bl.Src, bl.Title)
	case BlockDivider:
		return "***"
	case BlockToggle:
		return renderDirective(doc, bl, "toggle")
	case BlockSynced:
		return renderDirective(doc, bl, "synced")
	case BlockTodo:
		return renderTodo(doc, bl, flip)
	case BlockCallout:
		return renderDirective(doc, bl, "callout")
	case BlockFile:
		return renderDirective(doc, bl, "file")
	case BlockSheet:
		return renderDirective(doc, bl, "sheet")
	case BlockFigure:
		return renderDirective(doc, bl, "figure")
	default:
		return joinNonEmpty([]string{renderRuns(runsOfBlock(bl), inlineBlock)}, renderBlocks(doc, bl.Children))
	}
}

// joinNonEmpty joins the non-empty parts (and the non-empty strings in head)
// with a blank line.
func joinNonEmpty(head []string, rest ...string) string {
	var parts []string
	for _, s := range head {
		if s != "" {
			parts = append(parts, s)
		}
	}
	for _, s := range rest {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

// prefixLines prefixes every line with prefix; a blank line gets the prefix with
// its trailing space removed so no line ends in whitespace.
func prefixLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln == "" {
			lines[i] = strings.TrimRight(prefix, " ")
		} else {
			lines[i] = prefix + ln
		}
	}
	return strings.Join(lines, "\n")
}

// ---- todo and container directives ----

// renderTodo writes a todo as a GFM task item. Its children are indented to the
// item's content column, after a blank line.
func renderTodo(doc *Document, bl Block, alt bool) string {
	marker := "- "
	if alt {
		marker = "* "
	}
	box := "[ ]"
	if bl.Checked {
		box = "[x]"
	}
	lines := strings.Split(renderRuns(runsOfBlock(bl), inlineBlock), "\n")
	head := marker + box
	if lines[0] != "" {
		head += " " + lines[0]
	}
	out := []string{head}
	for _, ln := range lines[1:] {
		out = append(out, strings.Repeat(" ", len(marker))+ln)
	}
	return joinNonEmpty([]string{strings.Join(out, "\n")}, indentLines(renderBlocks(doc, bl.Children), strings.Repeat(" ", len(marker))))
}

// indentLines prefixes every non-empty line with prefix.
func indentLines(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln != "" {
			lines[i] = prefix + ln
		}
	}
	return strings.Join(lines, "\n")
}

// renderDirective writes toggle, synced, callout, file, sheet and figure as a
// container directive (see frommarkdown_directives.go). The scalar fields go in
// a one-line JSON header (json.Marshal escapes `<`, `>` and `&`, so a value can
// hold `-->` or markup and the header is still one line). The body is the
// block's text paragraph and then its children; a figure's body is its SVG,
// verbatim. The fence is one colon longer than the longest run of colons at the
// start of any body line, so nothing inside can close it.
func renderDirective(doc *Document, bl Block, name string) string {
	h := dirHeader{
		Icon: bl.Icon, Tone: bl.Tone, Collapsed: bl.Collapsed, Name: bl.Name, Mime: bl.Mime, Size: bl.Size,
		Src: bl.Src, Title: bl.Title, SheetEmbed: bl.SheetEmbed, SyncRef: bl.SyncRef,
	}
	var body string
	if bl.Kind == BlockFigure {
		h.Alt = bl.Text
		if bl.SVG != "" {
			body = bl.SVG + "\n"
		}
	} else {
		text := renderRuns(runsOfBlock(bl), inlineBlock)
		parts := renderParts(doc, bl.Children)
		kids := joinParts(parts)
		if text == "" && len(parts) > 0 && startsAsParagraph(parts[0].bl) {
			h.NoText = true
		}
		body = joinNonEmpty([]string{text}, kids)
		if body != "" {
			body += "\n"
		}
	}
	fence := strings.Repeat(":", colonFenceLen(body))
	open := fence + name
	if hb, err := json.Marshal(h); err == nil && string(hb) != "{}" {
		open += " " + string(hb)
	}
	return open + "\n" + body + fence
}

// colonFenceLen is the length of a fence that no line of body can close: three,
// or one more than the longest run of colons at the start of a body line.
func colonFenceLen(body string) int {
	n := 3
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimLeft(ln, " \t")
		run := 0
		for run < len(t) && t[run] == ':' {
			run++
		}
		if run >= n {
			n = run + 1
		}
	}
	return n
}

// startsAsParagraph reports whether the Markdown of bl begins as a paragraph, so
// that, first in a directive body, it would be read as the directive's text.
func startsAsParagraph(bl *Block) bool {
	if bl == nil {
		return false
	}
	switch bl.Kind {
	case BlockHeading, BlockList, BlockCode, BlockTable, BlockQuote, BlockDivider, BlockToggle, BlockSynced,
		BlockTodo, BlockCallout, BlockFile, BlockSheet, BlockFigure:
		return false
	case BlockMath:
		return !bl.Display
	}
	return true
}

// ---- code ----

// renderCode fences the code. The fence is longer than any run of the fence
// character inside the code, and the content between the fences is exactly
// bl.Text (no newline is added or dropped), so a Text that ends in a newline is
// written with a blank line before the closing fence and reads back unchanged.
func renderCode(bl Block) string {
	ch := byte('`')
	if strings.ContainsRune(bl.Language, '`') {
		ch = '~'
	}
	n := longestRun(bl.Text, ch) + 1
	if n < 3 {
		n = 3
	}
	fence := strings.Repeat(string(ch), n)
	body := ""
	if bl.Text != "" {
		body = bl.Text + "\n"
	}
	return fence + escapeInfo(bl.Language) + "\n" + body + fence
}

// escapeInfo spells a fence's language: the info string takes backslash escapes
// and character references like any other text.
func escapeInfo(lang string) string {
	var b strings.Builder
	for i := 0; i < len(lang); i++ {
		switch c := lang[i]; {
		case i == 0 && (c == '~' || c == '`'):
			b.WriteByte('\\') // would run into the fence
			b.WriteByte(c)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '&' && entityAhead(lang[i:]):
			b.WriteString(`\&`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func longestRun(s string, ch byte) int {
	best, cur := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

// ---- lists ----

// renderList writes the items of a list block. Each item's level controls its
// indentation, which is the width of its parent's marker so a child always
// lands inside its parent's content column ("10. " is wider than "9. ").
func renderList(bl Block, flip bool) string {
	var out []string
	var widths []int   // widths[l] = content column of the latest item at level l
	var counters []int // counters[l] = ordinal of the latest item at level l
	prevLevel, prevHasText := -1, false
	for _, it := range bl.Items {
		lvl := it.Level
		if lvl < 0 {
			lvl = 0
		}
		if lvl > len(widths) {
			lvl = len(widths)
		}
		widths = widths[:lvl]
		for len(counters) <= lvl {
			counters = append(counters, 0)
		}
		counters = counters[:lvl+1]
		counters[lvl]++

		marker := "- "
		switch {
		case bl.Ordered && flip:
			marker = strconv.Itoa(counters[lvl]) + ") "
		case bl.Ordered:
			marker = strconv.Itoa(counters[lvl]) + ". "
		case flip:
			marker = "* "
		}
		indent := 0
		if lvl > 0 {
			indent = widths[lvl-1]
		}
		widths = append(widths, indent+len(marker))

		lines := strings.Split(renderRuns(runsOfItem(it), inlineBlock), "\n")
		first := strings.Repeat(" ", indent) + marker + lines[0]
		if lines[0] == "" {
			first = strings.TrimRight(first, " ")
			if lvl > prevLevel && prevHasText {
				// An empty item cannot interrupt its parent's text line (it would
				// read as a continuation of it), so a blank line precedes it.
				out = append(out, "")
			}
		}
		prevLevel, prevHasText = lvl, lines[len(lines)-1] != "" || len(lines) > 1
		out = append(out, first)
		for _, ln := range lines[1:] {
			out = append(out, strings.Repeat(" ", indent+len(marker))+ln)
		}
	}
	return strings.Join(out, "\n")
}

// ---- tables ----

// renderTable writes a GFM table. A cell is its runs when it has them and its
// plain text otherwise, written in the cell inline mode, so every pipe is
// escaped; a "left" alignment is written explicitly as `:---`.
func renderTable(bl Block) string {
	headers, aligns, rows := bl.Headers, bl.Aligns, bl.Rows
	n := len(headers)
	if n == 0 {
		n = len(aligns)
	}
	if n == 0 && len(rows) > 0 {
		n = len(rows[0])
	}
	if n == 0 {
		return ""
	}
	cells := make([]string, n)
	for i := range cells {
		if i < len(headers) {
			cells[i] = cellText(headers[i], cellRunsAt(bl.HeaderRuns, i))
		}
	}
	delim := make([]string, n)
	for i := range delim {
		align := ""
		if i < len(aligns) {
			align = aligns[i]
		}
		switch align {
		case "left":
			delim[i] = ":---"
		case "center":
			delim[i] = ":---:"
		case "right":
			delim[i] = "---:"
		default:
			delim[i] = "---"
		}
	}
	lines := []string{tableRow(cells), tableRow(delim)}
	for r, row := range rows {
		escaped := make([]string, len(row))
		for i, c := range row {
			var runs []TextRun
			if r < len(bl.RowRuns) {
				runs = cellRunsAt(bl.RowRuns[r], i)
			}
			escaped[i] = cellText(c, runs)
		}
		lines = append(lines, tableRow(escaped))
	}
	return strings.Join(lines, "\n")
}

func tableRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

// cellRunsAt is the runs of cell i of a row of runs, or nil for a plain cell.
func cellRunsAt(runs [][]TextRun, i int) []TextRun {
	if i < len(runs) {
		return runs[i]
	}
	return nil
}

// cellText writes one cell: its runs when it has them (they win over text, as a
// block's Runs do), else its plain text.
func cellText(text string, runs []TextRun) string {
	if len(runs) == 0 && text != "" {
		runs = []TextRun{{Text: text}}
	}
	return renderRuns(runs, inlineCell)
}

// ---- images and links ----

func renderImage(alt, src, title string) string {
	s := "![" + padEdges(escapeInline(alt, false, inlineFlat)) + "](" + linkDest(src)
	if title != "" {
		s += ` "` + escapeTitle(title) + `"`
	}
	return s + ")"
}

// linkDest spells a link or image destination. A destination that is empty,
// contains whitespace, or starts with "<" goes in angle brackets; otherwise
// parentheses and backslashes are backslash-escaped.
func linkDest(u string) string {
	if u == "" || strings.ContainsAny(u, " \t\r\n") || strings.HasPrefix(u, "<") {
		return "<" + escapeDest(u, true) + ">"
	}
	return escapeDest(u, false)
}

func escapeDest(u string, angle bool) string {
	var b strings.Builder
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\r' || c == '\n':
			b.WriteString("%0A")
		case angle && (c == '<' || c == '>'):
			b.WriteByte('\\')
			b.WriteByte(c)
		case !angle && (c == '(' || c == ')'):
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '&' && entityAhead(u[i:]):
			b.WriteString(`\&`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeTitle spells a link or image title. CommonMark lets a title span lines, so
// a newline is kept as long as it does not leave a blank line inside the title (a
// blank line ends the paragraph); a newline that would, and any "\r", becomes a
// space.
func escapeTitle(t string) string {
	var b strings.Builder
	lineStart := 0 // where the current line of t starts
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch {
		case c == '\\' || c == '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n' && (lineStart == 0 || strings.TrimSpace(t[lineStart:i]) != ""):
			b.WriteByte('\n')
			lineStart = i + 1
			// The reader drops the spaces that start a continuation line, so the first
			// one is spelled as a character reference.
			if lineStart < len(t) && (t[lineStart] == ' ' || t[lineStart] == '\t') {
				b.WriteString(spaceEntity(t[lineStart]))
				i++
			}
		case c == '\n' || c == '\r':
			b.WriteByte(' ')
		case c == '&' && entityAhead(t[i:]):
			b.WriteString(`\&`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// ---- rich text ----

// markOrder is the canonical order of text-run marks: it is the order
// FromMarkdown produces and the order ToMarkdown nests them in (the last
// wrapping mark is the innermost). Marks outside this list have no Markdown
// spelling and are not written.
var markOrder = []string{"bold", "italic", "strike", "code", "math"}

// canonMarks returns marks de-duplicated, in markOrder, with unknown marks
// dropped; nil when none remain.
func canonMarks(marks []string) []string {
	if len(marks) == 0 {
		return nil
	}
	var out []string
	for _, want := range markOrder {
		for _, m := range marks {
			if m == want {
				out = append(out, want)
				break
			}
		}
	}
	return out
}

func hasMark(r TextRun, mark string) bool {
	for _, m := range r.Marks {
		if m == mark {
			return true
		}
	}
	return false
}

func equalMarks(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// canonRuns puts runs in canonical form, the one FromMarkdown produces and
// ToMarkdown writes: empty runs are dropped, marks are in markOrder, whitespace
// at the edge of a styled or linked run moves out into the unstyled text next to
// it (Markdown cannot open or close emphasis next to a space, and the space
// carries no style), and adjacent runs with the same marks and link merge, so
// any split of the same styled text yields the same Markdown.
//
// Code and math runs are exempt from the whitespace rule: their text is
// verbatim and their delimiters can pad.
func canonRuns(runs []TextRun) []TextRun {
	var out []TextRun
	add := func(r TextRun) {
		if r.Text == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Link == r.Link && equalMarks(out[n-1].Marks, r.Marks) {
			out[n-1].Text += r.Text
			return
		}
		out = append(out, r)
	}
	// Merge first, so whitespace between two pieces of one styled stretch is not
	// mistaken for whitespace at its edge.
	for _, r := range runs {
		add(TextRun{Text: r.Text, Marks: canonMarks(r.Marks), Link: r.Link})
	}
	merged := out
	out = nil
	for _, r := range merged {
		verbatim := hasMark(r, "code") || hasMark(r, "math")
		if (len(r.Marks) == 0 && r.Link == "") || verbatim {
			add(r)
			continue
		}
		core := strings.TrimFunc(r.Text, unicode.IsSpace)
		if core == "" {
			add(TextRun{Text: r.Text})
			continue
		}
		lead := len(r.Text) - len(strings.TrimLeftFunc(r.Text, unicode.IsSpace))
		add(TextRun{Text: r.Text[:lead]})
		add(TextRun{Text: core, Marks: r.Marks, Link: r.Link})
		add(TextRun{Text: r.Text[lead+len(core):]})
	}
	return out
}

func runsOfBlock(bl Block) []TextRun {
	if len(bl.Runs) > 0 {
		return bl.Runs
	}
	if bl.Text == "" {
		return nil
	}
	return []TextRun{{Text: bl.Text}}
}

func runsOfItem(it ListItem) []TextRun {
	if len(it.Runs) > 0 {
		return it.Runs
	}
	if it.Text == "" {
		return nil
	}
	return []TextRun{{Text: it.Text}}
}

// renderParagraphs splits a quote's text on blank lines ("\n\n") into
// paragraphs and renders each. A styled run that spans a paragraph break is
// split there: Markdown cannot style across a blank line.
func renderParagraphs(bl Block) []string {
	groups := [][]TextRun{nil}
	for _, r := range runsOfBlock(bl) {
		for i, piece := range strings.Split(r.Text, "\n\n") {
			if i > 0 {
				groups = append(groups, nil)
			}
			if piece != "" {
				groups[len(groups)-1] = append(groups[len(groups)-1], TextRun{Text: piece, Marks: r.Marks, Link: r.Link})
			}
		}
	}
	var out []string
	for _, g := range groups {
		if s := renderRuns(g, inlineBlock); s != "" {
			out = append(out, s)
		}
	}
	return out
}

type inlineMode int

const (
	// inlineBlock renders text that starts a source line in a block context
	// (paragraph, list item, quote): line-start constructs are escaped.
	inlineBlock inlineMode = iota
	// inlineFlat renders text where block constructs cannot occur (heading,
	// table cell, image alt text) and a newline cannot be written.
	inlineFlat
	// inlineCell renders the content of a table cell. Like inlineFlat nothing
	// line-leading is escaped and a newline cannot be written; in addition every
	// `|` is escaped (`\|`), inside code spans, math and link destinations too,
	// and the read-back check parses a one-column table instead of a heading.
	inlineCell
)

// renderRuns writes runs as Markdown inline syntax.
//
// Styled text is written with delimiters (`**` `*` `~~`) whenever a Markdown
// parser reads them back as the same runs. Delimiters are unreliable next to
// punctuation and letters (CommonMark's flanking rules: `x~~**y**~~` does not
// open), so the result is read back, and if it differs the block is written
// with the tags <strong>, <em> and <del> instead, which FromMarkdown reads as
// the same three marks. Plain text never needs the check.
func renderRuns(runs []TextRun, mode inlineMode) string {
	canon := canonRuns(runs)
	out := renderCanonRuns(canon, mode, false)
	if !hasEmphasis(canon) || readsBackAs(out, canon, mode) {
		return out
	}
	if alt := renderCanonRuns(canon, mode, true); readsBackAs(alt, canon, mode) {
		return alt
	}
	return out
}

func hasEmphasis(runs []TextRun) bool {
	for _, r := range runs {
		if hasMark(r, "bold") || hasMark(r, "italic") || hasMark(r, "strike") {
			return true
		}
	}
	return false
}

// readsBackAs reports whether out, read as the content of a paragraph (or, in
// flat mode, of a heading), gives exactly the runs want.
func readsBackAs(out string, want []TextRun, mode inlineMode) bool {
	if mode == inlineCell {
		return cellReadsBackAs(out, want)
	}
	src := out
	if mode == inlineFlat {
		src = "# " + out
	}
	r := &mdReader{src: []byte(src), lenient: true}
	root := mdEngine.Parser().Parse(text.NewReader(r.src))
	first := root.FirstChild()
	if first == nil || first.NextSibling() != nil {
		return false
	}
	switch first.(type) {
	case *ast.Paragraph, *ast.Heading:
		return reflect.DeepEqual(r.inline(first), want)
	}
	return false
}

// cellReadsBackAs reports whether out, written as the only cell of a table's header
// row, reads back as exactly one cell holding the runs want. Reading it inside a
// table is the point: GFM splits a row at every unescaped pipe before it parses
// any inline syntax, and un-escapes a pipe in a code span, so neither a paragraph
// nor a heading read says what a cell does.
func cellReadsBackAs(out string, want []TextRun) bool {
	r := &mdReader{src: []byte("| " + out + " |\n| --- |\n"), lenient: true}
	root := mdEngine.Parser().Parse(text.NewReader(r.src))
	first := root.FirstChild()
	if first == nil || first.NextSibling() != nil {
		return false
	}
	tbl, ok := first.(*east.Table)
	if !ok {
		return false
	}
	head, ok := tbl.FirstChild().(*east.TableHeader)
	if !ok || head.ChildCount() != 1 {
		return false
	}
	cell, ok := head.FirstChild().(*east.TableCell)
	if !ok {
		return false
	}
	return reflect.DeepEqual(r.inline(cell), want)
}

// renderCanonRuns writes already-canonical runs. With tags set, bold, italic
// and strike are written as HTML tags instead of delimiters.
func renderCanonRuns(runs []TextRun, mode inlineMode, tags bool) string {
	var out string
	atLine := func() bool {
		return mode == inlineBlock && (out == "" || strings.HasSuffix(out, "\n"))
	}
	for i := 0; i < len(runs); {
		j := i + 1
		for j < len(runs) && runs[j].Link == runs[i].Link {
			j++
		}
		if link := runs[i].Link; link != "" {
			var inner strings.Builder
			for _, r := range runs[i:j] {
				inner.WriteString(renderMarked(r, false, mode, tags))
			}
			if strings.HasSuffix(out, "!") {
				// "![" would start an image.
				out = out[:len(out)-1] + `\!`
			}
			dest := linkDest(link)
			if mode == inlineCell {
				dest = escapeCellPipes(dest)
			}
			out += "[" + inner.String() + "](" + dest + ")"
		} else {
			for _, r := range runs[i:j] {
				out += renderMarked(r, atLine(), mode, tags)
			}
		}
		i = j
	}
	return padEdges(out)
}

// renderMarked writes one run with its marks: code and math are leaves written
// verbatim; everything else is escaped text. italic is wrapped first, then
// bold, then strike, so the outermost wrapper is the last in that order.
func renderMarked(r TextRun, lineStart bool, mode inlineMode, tags bool) string {
	wrapped := hasMark(r, "italic") || hasMark(r, "bold") || hasMark(r, "strike")
	var s string
	switch {
	case hasMark(r, "code"):
		s = codeSpan(r.Text)
		if mode == inlineCell {
			s = escapeCellPipes(s)
		}
	case hasMark(r, "math"):
		tex := r.Text
		if mode == inlineCell {
			tex = escapeCellPipes(tex)
		}
		s = "$" + tex + "$"
	default:
		s = escapeInline(r.Text, lineStart && !wrapped, mode)
	}
	wrap := func(mark, delim, open, closeTag string) {
		if !hasMark(r, mark) {
			return
		}
		if tags {
			s = open + s + closeTag
		} else {
			s = delim + s + delim
		}
	}
	wrap("italic", "*", "<em>", "</em>")
	wrap("bold", "**", "<strong>", "</strong>")
	wrap("strike", "~~", "<del>", "</del>")
	return s
}

// codeSpan writes s as an inline code span: the delimiter is one backtick longer
// than any backtick run in s, and a space of padding keeps edge backticks and
// edge spaces from being swallowed.
func codeSpan(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	delim := strings.Repeat("`", longestRun(s, '`')+1)
	pad := ""
	spaced := len(s) > 0 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.Trim(s, " ") != ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") || spaced {
		pad = " "
	}
	return delim + pad + s + pad + delim
}

// escapeCellPipes writes a backslash before every `|` of s, which is how a pipe is
// spelled inside a table cell wherever the text is not escaped for Markdown:
// code spans (GFM un-escapes the pipe before the code span is read), math (the
// reader does the same, see cellTeX) and link destinations (a backslash escape).
func escapeCellPipes(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// escapeInline backslash-escapes the characters of plain text that a Markdown
// parser would otherwise read as syntax, so the text parses back unchanged.
//
// Always escaped: \ ` * [ ] < | ~ $ . An underscore is escaped unless it sits
// between two letters/digits (intraword underscores never open emphasis). An
// ampersand is escaped only where it would start an entity reference.
//
// At the start of a line in inlineBlock mode (lineStart, and after every hard
// break) the block markers are escaped too: `#` heading, `>` quote, `-` `+`
// bullet or rule, `=` setext rule, and the `.`/`)` of `1.`-style ordered
// markers. They are escaped only where the marker would really fire, so
// "#hashtag" and "-5" stay readable.
//
// In inlineBlock mode a "\n" is written as a hard line break (backslash,
// newline); in inlineFlat mode it becomes a space.
func escapeInline(s string, lineStart bool, mode inlineMode) string {
	var b strings.Builder
	atLine := lineStart && mode == inlineBlock
	for i := 0; i < len(s); {
		c := s[i]
		if c >= utf8.RuneSelf {
			_, size := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+size])
			i += size
			atLine = false
			continue
		}
		switch c {
		case '\n':
			if mode == inlineBlock {
				b.WriteString("\\\n")
				atLine = true
			} else {
				b.WriteByte(' ')
			}
			i++
			continue
		case '\\':
			if mode == inlineBlock && i+1 < len(s) && s[i+1] == '\n' {
				// A backslash before a hard break: `\\` + `\` + newline is not read
				// as an escaped backslash then a break, so spell this one as a
				// character reference.
				b.WriteString("&#92;")
			} else {
				b.WriteString(`\\`)
			}
		case '`', '*', '[', ']', '<', '|', '~', '$':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '_':
			if !intraword(s, i) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '&':
			if entityAhead(s[i:]) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '#':
			if atLine && headingMarkerAhead(s[i:]) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '>':
			if atLine {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '-', '+':
			if atLine && (spaceOrEndAhead(s, i) || (c == '-' && (restIs(s[i:], '-') || delimiterRowAhead(s[i:])))) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '=':
			if atLine && restIs(s[i:], '=') {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case ':':
			if atLine && (delimiterRowAhead(s[i:]) || strings.HasPrefix(s[i:], ":::")) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if atLine && j-i <= 9 && j < len(s) && (s[j] == '.' || s[j] == ')') && spaceOrEndAhead(s, j) {
				b.WriteString(s[i:j])
				b.WriteByte('\\')
				b.WriteByte(s[j])
				i = j + 1
				atLine = false
				continue
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
		atLine = false
		i++
	}
	return b.String()
}

// intraword reports whether the byte at i sits between two letters or digits.
func intraword(s string, i int) bool {
	if i == 0 || i+1 >= len(s) {
		return false
	}
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	next, _ := utf8.DecodeRuneInString(s[i+1:])
	return (unicode.IsLetter(prev) || unicode.IsDigit(prev)) && (unicode.IsLetter(next) || unicode.IsDigit(next))
}

// spaceOrEndAhead reports whether the byte after i is a space, tab, newline or
// the end of s.
func spaceOrEndAhead(s string, i int) bool {
	return i+1 >= len(s) || s[i+1] == ' ' || s[i+1] == '\t' || s[i+1] == '\n'
}

// restIs reports whether the rest of the line starting at s[0] consists only of
// ch, spaces and tabs.
func restIs(s string, ch byte) bool {
	for i := 0; i < len(s) && s[i] != '\n'; i++ {
		if s[i] != ch && s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// delimiterRowAhead reports whether the line starting at s[0] is the shape of a
// table delimiter cell (":-", "-:", ":---:"): after a line of text it would turn
// that text into a table header.
func delimiterRowAhead(s string) bool {
	i := 0
	if i < len(s) && s[i] == ':' {
		i++
	}
	dashes := 0
	for i < len(s) && s[i] == '-' {
		i++
		dashes++
	}
	if dashes == 0 {
		return false
	}
	if i < len(s) && s[i] == ':' {
		i++
	}
	for i < len(s) && s[i] != '\n' {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
		i++
	}
	return true
}

// headingMarkerAhead reports whether s starts with 1-6 '#' followed by a space,
// tab, newline or the end of s: the shape of an ATX heading marker.
func headingMarkerAhead(s string) bool {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return false
	}
	return n == len(s) || s[n] == ' ' || s[n] == '\t' || s[n] == '\n'
}

// entityAhead reports whether s starts with something a Markdown parser reads
// as a character reference: &#123; &#x1F; or &name;.
func entityAhead(s string) bool {
	if len(s) < 3 || s[0] != '&' {
		return false
	}
	i := 1
	switch {
	case s[1] == '#':
		i = 2
		hex := i < len(s) && (s[i] == 'x' || s[i] == 'X')
		if hex {
			i++
		}
		start := i
		for i < len(s) && isEntityDigit(s[i], hex) {
			i++
		}
		return i > start && i < len(s) && s[i] == ';'
	default:
		start := i
		for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' || s[i] >= '0' && s[i] <= '9') {
			i++
		}
		return i > start && i-start <= 32 && i < len(s) && s[i] == ';'
	}
}

func isEntityDigit(c byte, hex bool) bool {
	if c >= '0' && c <= '9' {
		return true
	}
	return hex && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')
}

// padEdges writes a space or tab that would be trimmed away (at the start of a
// line, or at the very end of the text) as a character reference, which a
// parser keeps. The inner characters are untouched.
func padEdges(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln != "" && (ln[0] == ' ' || ln[0] == '\t') {
			lines[i] = spaceEntity(ln[0]) + ln[1:]
		}
	}
	last := lines[len(lines)-1]
	if last != "" && (last[len(last)-1] == ' ' || last[len(last)-1] == '\t') {
		lines[len(lines)-1] = last[:len(last)-1] + spaceEntity(last[len(last)-1])
	}
	return strings.Join(lines, "\n")
}

func spaceEntity(c byte) string {
	if c == '\t' {
		return "&#9;"
	}
	return "&#32;"
}

// escapeHeadingTail protects a heading from a closing sequence: a trailing run
// of '#' preceded by a space (or standing alone) would be read as the optional
// closing hashes of an ATX heading and dropped, so its first '#' is escaped.
func escapeHeadingTail(s string) string {
	end := len(s)
	start := end
	for start > 0 && s[start-1] == '#' {
		start--
	}
	if start == end {
		return s
	}
	if start > 0 && s[start-1] != ' ' && s[start-1] != '\t' {
		return s
	}
	return s[:start] + `\` + s[start:]
}

// ---- front matter ----

func yamlScalar(v string) string {
	if needsYAMLQuotes(v) {
		return strconv.Quote(v)
	}
	return v
}

func needsYAMLQuotes(v string) bool {
	return v == "" || strings.ContainsAny(v, ":#{}[],&*?|>!%@`\"'\\\n\r\t") ||
		strings.TrimSpace(v) != v || strings.HasPrefix(v, "- ")
}
