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
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
//     inline math and `[text](href)` links;
//   - lists number their items sequentially per level, indent children by the
//     width of the parent's marker, and alternate the bullet (`-`/`*`) or the
//     ordered delimiter (`.`/`)`) between ADJACENT lists so they do not merge;
//   - a code fence is one backtick longer than any backtick run inside the
//     code, and the code between the fences is exactly Block.Text: a trailing
//     newline in Text is a blank line before the closing fence;
//   - display math is `$$`, TeX, `$$` on separate lines with nothing added.
//
// Kinds with no GFM spelling (todo, callout, toggle, synced, file, sheet,
// figure) keep their pre-codec spelling here; they are not yet read back.
func ToMarkdown(doc *Document) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	if len(doc.Meta.Directives) > 0 {
		b.WriteString("---\n")
		for _, k := range sortedMapKeys(doc.Meta.Directives) {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(yamlScalar(doc.Meta.Directives[k]))
			b.WriteByte('\n')
		}
		b.WriteString("---\n\n")
	}
	for i, sec := range doc.Sections {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		for _, k := range sortedMapKeys(sec.Attrs) {
			b.WriteString(fmt.Sprintf("<!-- %s: %s -->\n", k, sec.Attrs[k]))
		}
		for _, note := range sec.Notes {
			b.WriteString(fmt.Sprintf("<!-- %s -->\n", note))
		}
		b.WriteString(renderBlocks(doc, sec.Blocks))
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// renderBlocks joins the blocks with exactly one blank line between them.
// Blocks that render to nothing (an empty paragraph) are skipped, so they never
// leave a stray blank line behind.
func renderBlocks(doc *Document, blocks []Block) string {
	var parts []string
	flip := false
	for i, bl := range blocks {
		if bl.Kind == BlockList {
			// Two lists of the same kind separated only by a blank line are ONE
			// list to a Markdown parser. Alternating the marker keeps them apart.
			if i > 0 && blocks[i-1].Kind == BlockList && blocks[i-1].Ordered == bl.Ordered {
				flip = !flip
			} else {
				flip = false
			}
		}
		if s := renderBlock(doc, bl, flip); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
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
		return renderTable(bl.Headers, bl.Aligns, bl.Rows)
	case BlockImage:
		return renderImage(bl.Text, bl.Src, bl.Title)
	case BlockDivider:
		return "***"
	case BlockToggle:
		head := "<details"
		if !bl.Collapsed {
			head += " open"
		}
		head += ">\n<summary>" + bl.Text + "</summary>"
		return joinNonEmpty([]string{head}, renderBlocks(doc, bl.Children), "</details>")
	case BlockSynced:
		if bl.SyncRef == nil {
			return ""
		}
		comment := fmt.Sprintf("<!-- synced: blockId=%s pageId=%s -->", bl.SyncRef.BlockID, bl.SyncRef.PageID)
		if bl.SyncRef.PageID == "" {
			if src := findBlockPtr(doc, bl.SyncRef.BlockID); src != nil && src.Kind != BlockSynced {
				return comment + "\n" + renderBlock(doc, *src, false)
			}
		}
		return comment
	case BlockTodo:
		mark := "- [ ] "
		if bl.Checked {
			mark = "- [x] "
		}
		return joinNonEmpty([]string{mark + renderRuns(runsOfBlock(bl), inlineFlat)}, renderBlocks(doc, bl.Children))
	case BlockCallout:
		text := renderRuns(runsOfBlock(bl), inlineBlock)
		if bl.Icon != "" {
			text = bl.Icon + " " + text
		}
		return joinNonEmpty([]string{prefixLines(text, "> ")}, renderBlocks(doc, bl.Children))
	case BlockFile:
		name := bl.Name
		if name == "" {
			name = bl.Text
		}
		if name == "" {
			name = bl.Src
		}
		return "[" + escapeInline(name, false, inlineFlat) + "](" + linkDest(bl.Src) + ")"
	case BlockSheet:
		if bl.SheetEmbed != nil && bl.SheetEmbed.Snapshot != nil {
			return renderSheetSnapshotTable(bl.SheetEmbed.Snapshot)
		}
		sheet := ""
		if bl.SheetEmbed != nil {
			sheet = bl.SheetEmbed.Sheet
		}
		return fmt.Sprintf("<!-- eden-sheet: %s -->", sheet)
	case BlockFigure:
		if bl.SVG != "" {
			return strings.TrimSuffix(bl.SVG, "\n")
		}
		if bl.Src != "" {
			alt := bl.Text
			if alt == "" {
				alt = bl.Title
			}
			return renderImage(alt, bl.Src, bl.Title)
		}
		return ""
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
	return fence + bl.Language + "\n" + body + fence
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
		}
		out = append(out, first)
		for _, ln := range lines[1:] {
			out = append(out, strings.Repeat(" ", indent+len(marker))+ln)
		}
	}
	return strings.Join(out, "\n")
}

// ---- tables ----

// renderTable writes a GFM table. Cell text is plain text, so it is escaped
// (pipes included); a "left" alignment is written explicitly as `:---`.
func renderTable(headers, aligns []string, rows [][]string) string {
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
			cells[i] = cellText(headers[i])
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
	for _, row := range rows {
		escaped := make([]string, len(row))
		for i, c := range row {
			escaped[i] = cellText(c)
		}
		lines = append(lines, tableRow(escaped))
	}
	return strings.Join(lines, "\n")
}

func tableRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func cellText(s string) string {
	return padEdges(escapeInline(s, false, inlineFlat))
}

// renderSheetSnapshotTable renders a sheet snapshot as a table whose first row
// is the header row.
func renderSheetSnapshotTable(snap *SheetSnapshot) string {
	if snap == nil || len(snap.Cells) == 0 {
		return "<!-- eden-sheet empty -->"
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
	var headers []string
	var rows [][]string
	for r := 0; r <= maxR; r++ {
		cells := make([]string, maxC+1)
		for c := 0; c <= maxC; c++ {
			cells[c] = grid[[2]int{r, c}]
		}
		if r == 0 {
			headers = cells
		} else {
			rows = append(rows, cells)
		}
	}
	return renderTable(headers, nil, rows)
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

func escapeTitle(t string) string {
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch {
		case c == '\\' || c == '"':
			b.WriteByte('\\')
			b.WriteByte(c)
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

// canonRuns drops empty runs, canonicalizes marks and merges adjacent runs that
// carry the same marks and link, so any split of the same styled text yields
// the same Markdown.
func canonRuns(runs []TextRun) []TextRun {
	var out []TextRun
	for _, r := range runs {
		if r.Text == "" {
			continue
		}
		marks := canonMarks(r.Marks)
		if n := len(out); n > 0 && out[n-1].Link == r.Link && equalMarks(out[n-1].Marks, marks) {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, TextRun{Text: r.Text, Marks: marks, Link: r.Link})
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
)

// renderRuns writes runs as Markdown inline syntax.
func renderRuns(runs []TextRun, mode inlineMode) string {
	runs = canonRuns(runs)
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
				inner.WriteString(renderMarked(r, false, mode))
			}
			out += "[" + inner.String() + "](" + linkDest(link) + ")"
		} else {
			for _, r := range runs[i:j] {
				out += renderMarked(r, atLine(), mode)
			}
		}
		i = j
	}
	return padEdges(out)
}

// renderMarked writes one run with its marks: code and math are leaves written
// verbatim; everything else is escaped text. italic is wrapped first, then
// bold, then strike, so the outermost delimiter is the earliest in markOrder.
func renderMarked(r TextRun, lineStart bool, mode inlineMode) string {
	wrapped := hasMark(r, "italic") || hasMark(r, "bold") || hasMark(r, "strike")
	var s string
	switch {
	case hasMark(r, "code"):
		s = codeSpan(r.Text)
	case hasMark(r, "math"):
		s = "$" + r.Text + "$"
	default:
		s = escapeInline(r.Text, lineStart && !wrapped, mode)
	}
	if hasMark(r, "italic") {
		s = "*" + s + "*"
	}
	if hasMark(r, "bold") {
		s = "**" + s + "**"
	}
	if hasMark(r, "strike") {
		s = "~~" + s + "~~"
	}
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
		case '\\', '`', '*', '[', ']', '<', '|', '~', '$':
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
			if atLine && (spaceOrEndAhead(s, i) || (c == '-' && restIs(s[i:], '-'))) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		case '=':
			if atLine && restIs(s[i:], '=') {
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
