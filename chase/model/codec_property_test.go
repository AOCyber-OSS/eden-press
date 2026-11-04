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

// The all-kinds property test of the Markdown codec (objective 1026, TRD 03, D10).
//
// genDocument builds a random Document in CANONICAL form (the form ToMarkdown
// documents and FromMarkdown produces) over all 16 block kinds, with nesting,
// rich runs, todo state, sheet embeds, section notes and attrs, front matter,
// page properties, comments and ids. Only the standard library is used
// (math/rand/v2, fixed seeds), so there is no property-testing dependency and
// every failure reproduces from the two numbers it prints.
//
// Four properties hold for every generated d:
//
//	P1  FromMarkdown(ToMarkdown(d), {Prev: d})       == d  (ids included)
//	P2  FromMarkdown(ToMarkdown(d), {})              == d  modulo ids, and the
//	    trailer alone gives the ids back too (P2b)
//	P3  ToMarkdown(FromMarkdown(ToMarkdown(d)))      == ToMarkdown(d)
//	P4  ToMarkdown(d) is byte-identical on every call
//
// A document the generator makes must first pass propValid (the model's own
// validateEditableBlock plus the invariants of GetDocJSON), otherwise a failure
// would be a generator bug and not a codec bug.

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shrinkBudget bounds the time spent minimizing one failing document.
const shrinkBudget = 20 * time.Second

const (
	propSeeds       = 20 // seeds
	propDocsPerSeed = 50 // documents per seed: 1,000 in all
	propMinPerKind  = 20 // every kind must appear at least this often
	propDepth       = 4  // container nesting
)

// propKinds is the whole block vocabulary of schema v7.
var propKinds = []BlockKind{
	BlockParagraph, BlockList, BlockCode, BlockMath, BlockHeading, BlockTable, BlockImage, BlockQuote,
	BlockToggle, BlockSynced, BlockTodo, BlockCallout, BlockDivider, BlockFile, BlockSheet, BlockFigure,
}

// propRand returns the random stream of one document. The pair (seed, index) is
// all a failure needs to reproduce.
func propRand(seed, index uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, index))
}

// ---- the generator ----

type docGen struct {
	r     *rand.Rand
	depth int
}

// genCtx is where a block is being generated.
type genCtx struct {
	level  int  // container nesting so far
	inTodo bool // inside a todo, which is a list item to Markdown
}

func (g *docGen) chance(pct int) bool { return g.r.IntN(100) < pct }

func (g *docGen) pick(ss []string) string { return ss[g.r.IntN(len(ss))] }

func (g *docGen) tokens(alpha []string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(alpha[g.r.IntN(len(alpha))])
	}
	return b.String()
}

// Alphabets are heavy on Markdown and fence punctuation on purpose.
var (
	propTextAlpha = []string{
		"a", "B", "1", ".", " ", "*", "_", "`", "~", "#", ">", "-", "+", "|", "$", "<", "[", "]", "!", "&", "\\",
		":", "::", ":::", "{", "}", "\"", "é", "x", "日", "☕", "&amp;", "<b>", "-->", "(", ")",
	}
	propItemAlpha = []string{"a", "b", "1", ".", "-", "*", " ", "#", ">", "_", "`", "[", "]", "é"}
	propCellAlpha = []string{"a", "b", "1", " ", "*", "_", "`", "~", "|", "$", "[", "]", "(", ")", "<", ">", "&", "\\", ".", "-", "#", "!", ":", "é", "amp;"}
	propCodeAlpha = []string{"a", "`", "``", "```", "\n", " ", "~", "$", "#", "-", ":::", "<", "é"}
	// propTeXAlpha has no bare "$": TeX itself reserves it, and a "$$" inside display
	// math cannot be spelled in a $$ block (a canonical-form limit, see AGENTS.md).
	propTeXAlpha = []string{"a", "x", "^", "_", "{", "}", "\\", " ", "\n", "#", "-", "|", "<", ">", "&", "1", "\\$"}
	propSVGAlpha = []string{"<svg>", "</svg>", "\n", "\n\n", ":::", "::::", " ", "a", "<", "&", "\\", "\"", ">"}
	propSrcAlpha = []string{"a", "b", "/", ".", " ", "(", ")", "<", ">", "&", "amp;", "\\", "%", "#", "é"}
	propTitle    = []string{"a", " ", "\"", "\\", "&", "amp;", "'", "é"}
	propStrAlpha = []string{"a", "B", " ", "-", "--", "-->", "<!--", "<", ">", "&", "\"", "\\", "\n", ":", "eden:", "é", "☕", "{", "}", "#", "_", "x"}
	propKeyAlpha = []string{"a", "b", "C", "0", "_", "-", ".", "x"}

	// propWords are the run texts of marked runs: none starts or ends with
	// whitespace, which a marked run cannot carry in canonical form.
	propWords = []string{
		"a", "bb", "x y", "(p)", "q.", "*", "_", "1", "é", ",", "x_1", "$", "a|b", "-", "[", "!", "**", "~~", "`",
		"a\\", "\\", ")", "](", "<em>", "&amp;", "# h", "1. ", "> q", "x*y", "2*3*4", "a_b_c", "((", "))",
	}
	propMathTeX = []string{"a^2", "x_1", "y", "\\alpha", "a b", "x^{2}"}
	propLinks   = []string{"", "", "", "", "https://x.io", "dir/a b.md", "https://x.io/a_(b)?q=1&r=2"}
	propMarks   = [][]string{
		nil, nil, nil, {"bold"}, {"italic"}, {"bold", "italic"}, {"strike"}, {"code"}, {"bold", "strike"},
		{"italic", "strike"}, {"bold", "italic", "strike"}, {"math"},
	}
)

// okText reports whether s is a legal Text: a text is never split by a blank line
// (a paragraph break is a new block), so no line of a multi-line text is blank.
func okText(s string) bool {
	if !strings.Contains(s, "\n") {
		return true
	}
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "" {
			return false
		}
	}
	return true
}

func nonBlank(s string) bool { return strings.TrimSpace(s) != "" }

// plain returns a plain Text; with multi it may hold hard line breaks.
func (g *docGen) plain(multi bool) string {
	for try := 0; try < 8; try++ {
		s := g.tokens(propTextAlpha, g.r.IntN(7))
		for multi && g.chance(20) {
			s += "\n" + g.tokens(propTextAlpha, 1+g.r.IntN(6))
		}
		if okText(s) {
			return s
		}
	}
	return "t"
}

func marksKey(m []string) string { return strings.Join(m, ",") }

// mergeRuns drops empty runs and merges neighbours with the same marks and link,
// which is what canonical runs are.
func mergeRuns(runs []TextRun) []TextRun {
	var out []TextRun
	for _, r := range runs {
		if r.Text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Link == r.Link && marksKey(out[n-1].Marks) == marksKey(r.Marks) {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

func isMathRun(r TextRun) bool { return len(r.Marks) == 1 && r.Marks[0] == "math" }

// runsOK reports whether runs are legal rich text for one block.
func runsOK(runs []TextRun, multi bool) bool {
	if len(runs) == 0 {
		return false
	}
	text := FlattenRuns(runs)
	if !nonBlank(text) || !okText(text) || (!multi && strings.Contains(text, "\n")) {
		return false
	}
	for i, r := range runs {
		if (len(r.Marks) > 0 || r.Link != "") && strings.TrimSpace(r.Text) != r.Text {
			return false
		}
		// A closing $ followed by a digit does not close math.
		if isMathRun(r) && i+1 < len(runs) {
			if c := runs[i+1].Text[0]; c >= '0' && c <= '9' {
				return false
			}
		}
	}
	return true
}

// runs returns canonical rich text.
func (g *docGen) runs(multi bool) []TextRun {
	plainWords := append([]string{" ", " x ", "  ", "plain"}, propWords...)
	if multi {
		plainWords = append(plainWords, "a\nb", "\nb", "a\n")
	}
	for try := 0; try < 12; try++ {
		var runs []TextRun
		for k := 1 + g.r.IntN(5); k > 0; k-- {
			marks := append([]string(nil), propMarks[g.r.IntN(len(propMarks))]...)
			if len(marks) == 0 {
				marks = nil
			}
			link := g.pick(propLinks)
			var text string
			switch {
			case len(marks) == 1 && marks[0] == "math":
				text = g.pick(propMathTeX)
			case len(marks) == 0 && link == "":
				text = g.pick(plainWords)
			default:
				text = g.pick(propWords)
			}
			runs = append(runs, TextRun{Text: text, Marks: marks, Link: link})
		}
		runs = mergeRuns(runs)
		if runsOK(runs, multi) {
			return runs
		}
	}
	return []TextRun{{Text: "r"}}
}

// textFields returns the Text and Runs of a text-bearing block: plain text with
// no Runs, or rich text whose Text is the flattened runs. A paragraph or a quote
// is never only one math run (that is a math block).
func (g *docGen) textFields(multi, soloMathOK bool) (string, []TextRun) {
	if g.chance(55) {
		return g.plain(multi), nil
	}
	runs := g.runs(multi)
	switch {
	case len(runs) == 1 && len(runs[0].Marks) == 0 && runs[0].Link == "":
		return runs[0].Text, nil
	case len(runs) == 1 && isMathRun(runs[0]) && !soloMathOK:
		return g.plain(multi), nil
	}
	return FlattenRuns(runs), runs
}

// blankLinesOut turns every whitespace-only line into an empty one. Code, math
// and SVG nested in a todo cannot keep one (a list item reads it as a blank line).
func blankLinesOut(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func (g *docGen) block(ctx genCtx) Block {
	return g.blockOf(propKinds[g.r.IntN(len(propKinds))], ctx)
}

// kids returns the children of a container at ctx (none past the depth limit).
func (g *docGen) kids(ctx genCtx) []Block {
	if ctx.level >= g.depth {
		return nil
	}
	var out []Block
	for n := g.r.IntN(4); n > 0; n-- {
		out = append(out, g.block(ctx))
	}
	return out
}

// titleFields is the text of a toggle, callout or todo, which may be empty.
func (g *docGen) titleFields() (string, []TextRun) {
	if g.chance(15) {
		return "", nil
	}
	text, runs := g.textFields(true, true)
	if !nonBlank(text) {
		return "", nil
	}
	return text, runs
}

// propCellTeX is the TeX of a math run in a table cell: the ordinary ones, and
// the ones with a pipe, which a cell must spell \| (see cellTeX).
var propCellTeX = []string{"a|b", "|x|", "a\\|b", "\\|", "x^{|y|}", "a||b"}

// propCellCode is the text of a code run in a table cell, every one with a pipe
// (GFM un-escapes \| in a code span) and some with a backslash or a backtick next
// to it.
var propCellCode = []string{"a|b", "|", "x\\|y", "`|", "||", "| |", "a\\", "|`"}

// cell returns one table cell in canonical form: its text, and its runs when it
// is styled or linked (nil for a plain cell). A styled cell never holds a newline.
func (g *docGen) cell() (string, []TextRun) {
	if !g.chance(40) {
		return strings.TrimSpace(g.tokens(propCellAlpha, g.r.IntN(6))), nil
	}
	runs := g.runs(false)
	for i, r := range runs {
		switch {
		case isMathRun(r) && g.chance(40):
			runs[i].Text = g.pick(propCellTeX)
		case hasMark(r, "code") && g.chance(40):
			runs[i].Text = g.pick(propCellCode)
		}
	}
	runs = mergeRuns(runs)
	if len(runs) == 1 && len(runs[0].Marks) == 0 && runs[0].Link == "" {
		return runs[0].Text, nil
	}
	return FlattenRuns(runs), runs
}

// table returns a table with plain and styled cells in canonical form: HeaderRuns,
// RowRuns and each RowRuns[r] are nil unless they hold a styled cell, and a plain
// cell's entry is nil.
func (g *docGen) table() Block {
	n := 1 + g.r.IntN(4)
	b := Block{Kind: BlockTable, Headers: make([]string, n), Aligns: make([]string, n)}
	header := make([][]TextRun, n)
	styled := false
	for k := 0; k < n; k++ {
		b.Headers[k], header[k] = g.cell()
		styled = styled || header[k] != nil
		b.Aligns[k] = g.pick([]string{"", "left", "right", "center"})
	}
	if styled {
		b.HeaderRuns = header
	}
	var rowRuns [][][]TextRun
	anyStyled := false
	for r := g.r.IntN(4); r > 0; r-- {
		row := make([]string, n)
		runs := make([][]TextRun, n)
		styled := false
		for k := range row {
			row[k], runs[k] = g.cell()
			styled = styled || runs[k] != nil
		}
		b.Rows = append(b.Rows, row)
		if !styled {
			runs = nil
		}
		rowRuns = append(rowRuns, runs)
		anyStyled = anyStyled || styled
	}
	if anyStyled {
		b.RowRuns = rowRuns
	}
	return b
}

func (g *docGen) blockOf(kind BlockKind, ctx genCtx) Block {
	inner := genCtx{level: ctx.level + 1, inTodo: ctx.inTodo}
	switch kind {
	case BlockParagraph:
		text, runs := g.textFields(true, false)
		if !nonBlank(text) {
			text, runs = "p", nil
		}
		return Block{Kind: BlockParagraph, Text: text, Runs: runs}

	case BlockHeading:
		text, runs := g.textFields(false, true)
		if !nonBlank(text) {
			text, runs = "h", nil
		}
		return Block{Kind: BlockHeading, Level: 1 + g.r.IntN(6), Text: text, Runs: runs}

	case BlockList:
		return g.list(ctx)

	case BlockCode:
		text := g.tokens(propCodeAlpha, g.r.IntN(8))
		if ctx.inTodo {
			text = blankLinesOut(text)
		}
		return Block{Kind: BlockCode, Language: g.pick([]string{"", "", "go", "c++", "x-y", "sh"}), Text: text}

	case BlockMath:
		if g.chance(35) {
			return Block{Kind: BlockMath, Text: g.pick(propMathTeX)} // a paragraph of only $...$
		}
		text := g.tokens(propTeXAlpha, g.r.IntN(8))
		if ctx.inTodo {
			text = blankLinesOut(text)
		}
		return Block{Kind: BlockMath, Display: true, Text: text}

	case BlockTable:
		return g.table()

	case BlockImage:
		return Block{
			Kind:  BlockImage,
			Src:   "x" + g.tokens(propSrcAlpha, 1+g.r.IntN(6)) + "y",
			Text:  strings.TrimSpace(g.tokens(propCellAlpha, g.r.IntN(5))),
			Title: g.tokens(propTitle, g.r.IntN(4)),
		}

	case BlockQuote:
		text, runs := g.textFields(true, false)
		if runs == nil && nonBlank(text) && g.chance(15) {
			if more := g.plain(true); nonBlank(more) {
				text += "\n\n" + more // a second paragraph: a quote's Text may hold one
			}
		}
		kids := g.kids(inner)
		for len(kids) > 0 && kids[0].Kind == BlockParagraph {
			kids = kids[1:] // a quote's leading paragraphs are its Text
		}
		if len(kids) == 0 {
			kids = nil // the reader gives nil, never an empty slice
		}
		if !nonBlank(text) {
			text, runs = "", nil
			if len(kids) == 0 {
				text = "q"
			}
		}
		return Block{Kind: BlockQuote, Text: text, Runs: runs, Children: kids}

	case BlockToggle:
		text, runs := g.titleFields()
		return Block{Kind: BlockToggle, Text: text, Runs: runs, Collapsed: g.chance(50), Children: g.kids(inner)}

	case BlockCallout:
		text, runs := g.titleFields()
		return Block{
			Kind: BlockCallout, Text: text, Runs: runs,
			Icon: g.pick([]string{"", "", "!", "💡", "⚠️"}), Tone: g.pick([]string{"", "", "info", "warn", "success", "danger"}),
			Children: g.kids(inner),
		}

	case BlockTodo:
		text, runs := g.titleFields()
		inner.inTodo = true
		return Block{Kind: BlockTodo, Text: text, Runs: runs, Checked: g.chance(50), Children: g.kids(inner)}

	case BlockDivider:
		return Block{Kind: BlockDivider}

	case BlockFile:
		return Block{
			Kind: BlockFile, Src: "s" + g.tokens(propTitle, g.r.IntN(5)), Name: g.tokens(propTitle, g.r.IntN(4)),
			Mime: g.pick([]string{"", "text/plain", "application/pdf"}), Size: int64(g.r.IntN(3)) << g.r.IntN(30),
		}

	case BlockSheet:
		return Block{Kind: BlockSheet, SheetEmbed: g.sheet()}

	case BlockFigure:
		b := Block{Kind: BlockFigure}
		if g.chance(60) {
			b.Src = "f" + g.tokens(propTitle, g.r.IntN(4))
		}
		if b.Src == "" || g.chance(60) {
			b.SVG = g.tokens(propSVGAlpha, 1+g.r.IntN(8))
			if ctx.inTodo {
				b.SVG = blankLinesOut(b.SVG)
			}
		}
		if b.Src == "" && b.SVG == "" {
			b.SVG = "<svg/>"
		}
		if g.chance(40) {
			b.Title = g.tokens(propTitle, 1+g.r.IntN(3))
		}
		if g.chance(40) {
			b.Text = g.tokens(propTextAlpha, 1+g.r.IntN(4))
		}
		return b

	case BlockSynced:
		return Block{Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b_0"}} // pointed at a real block by the document
	}
	panic("propKinds holds a kind blockOf does not know: " + string(kind))
}

func (g *docGen) list(ctx genCtx) Block {
	b := Block{Kind: BlockList, Ordered: g.chance(50)}
	n := 1 + g.r.IntN(4)
	if g.chance(10) {
		n = 10 + g.r.IntN(3) // reaches "10." so a child's indent follows a wider marker
	}
	level := 0
	for k := 0; k < n; k++ {
		if k == 0 {
			level = 0
		} else {
			level = g.r.IntN(level + 2)
		}
		it := ListItem{Level: level}
		if g.chance(30) {
			runs := g.runs(false)
			it.Runs, it.Text = runs, FlattenRuns(runs)
			if len(runs) == 1 && len(runs[0].Marks) == 0 && runs[0].Link == "" {
				it.Runs = nil
			}
		} else {
			it.Text = strings.TrimSpace(g.tokens(propItemAlpha, g.r.IntN(5)))
		}
		if ctx.inTodo && it.Text == "" {
			// goldmark ends a todo at the first block after a nested list whose last
			// item is empty, so a list under a todo has no empty item.
			it.Text = "i"
		}
		b.Items = append(b.Items, it)
	}
	return b
}

func (g *docGen) sheet() *SheetEmbed {
	str := func() string { return g.tokens(propTitle, g.r.IntN(4)) }
	e := &SheetEmbed{PageID: str(), WorkbookHint: str(), Sheet: "S" + str()}
	if g.chance(40) {
		e.Range = &SheetRange{R0: g.r.IntN(3), R1: g.r.IntN(9), C0: g.r.IntN(3), C1: g.r.IntN(9)}
		if g.chance(30) {
			e.Range.ColumnIDs = []string{"c1", "c" + str()}
			e.Range.RowIDs = []string{"r1"}
			e.Range.RangeID = "rg" + str()
		}
	}
	if g.chance(50) {
		snap := &SheetSnapshot{}
		for c := 1 + g.r.IntN(3); c > 0; c-- {
			col := SheetColumnSnap{ID: "c" + strconv.Itoa(c), Name: str(), Type: g.pick([]string{"text", "number", "select"})}
			if col.Type == "select" {
				col.Options = []string{str(), "x"}
			}
			snap.Columns = append(snap.Columns, col)
		}
		for r := g.r.IntN(3); r > 0; r-- {
			snap.RowIDs = append(snap.RowIDs, SheetRowIDSnap{R: r, ID: "row" + strconv.Itoa(r)})
		}
		for r := g.r.IntN(4); r > 0; r-- {
			cell := SheetCellSnap{R: g.r.IntN(4), C: g.r.IntN(4), V: str()}
			if g.chance(40) {
				cell.F = "=A" + strconv.Itoa(r)
			}
			if g.chance(30) {
				cell.Rels = []map[string]string{{"row": "row1", "table": str()}}
			}
			snap.Cells = append(snap.Cells, cell)
		}
		e.Snapshot = snap
	}
	if g.chance(15) {
		e.Sheet = "" // a snapshot alone is enough
		if e.Snapshot == nil {
			e.Sheet = "S"
		}
	}
	return e
}

// genDocument builds one random, canonical, valid Document. depth is how deep
// containers (quote, toggle, callout, todo) nest. About 60% of the documents
// carry ids (and so synced references, comments and the trailer's id sequence).
func genDocument(r *rand.Rand, depth int) *Document {
	g := &docGen{r: r, depth: depth}
	str := func() string { return g.tokens(propStrAlpha, g.r.IntN(6)) }

	var secs [][]Block
	for n := 1 + r.IntN(4); n > 0; n-- {
		var blocks []Block
		for k := r.IntN(6); k > 0; k-- {
			blocks = append(blocks, g.block(genCtx{}))
		}
		secs = append(secs, blocks)
	}
	d := fxDoc(secs...)

	for i := range d.Sections {
		if g.chance(35) {
			d.Sections[i].Attrs = map[string]string{}
			for n := 1 + r.IntN(3); n > 0; n-- {
				d.Sections[i].Attrs[str()] = str()
			}
		}
		for n := r.IntN(3); n > 0; n-- {
			d.Sections[i].Notes = append(d.Sections[i].Notes, str())
		}
	}
	if g.chance(40) {
		d.Meta.Directives = map[string]string{}
		for n := 1 + r.IntN(3); n > 0; n-- {
			key := g.tokens(propKeyAlpha, 1+r.IntN(4))
			if key[0] == '-' || key[0] == '.' {
				key = "k" + key
			}
			d.Meta.Directives[key] = str()
		}
	}
	if g.chance(35) {
		types := []PropertyType{PropText, PropNumber, PropSelect, PropMultiSelect, PropDate, PropCheckbox, PropURL, PropEmail, PropRelation}
		for n := 1 + r.IntN(3); n > 0; n-- {
			p := PropertyDef{ID: "p" + strconv.Itoa(n) + str(), Name: str(), Type: types[r.IntN(len(types))]}
			if p.Type == PropSelect || p.Type == PropMultiSelect {
				p.Options = []string{str(), "x"}
			}
			d.Properties = append(d.Properties, p)
		}
		if g.chance(70) {
			d.PropertyValues = map[string]string{}
			for _, p := range d.Properties {
				if g.chance(70) {
					d.PropertyValues[p.ID] = str()
				}
			}
			if len(d.PropertyValues) == 0 {
				d.PropertyValues = nil
			}
		}
	}
	rebuildOutline(d)

	if g.chance(60) {
		fxIDs(d)
		g.pointSyncedBlocks(d, true)
		g.addComments(d)
	} else {
		g.pointSyncedBlocks(d, false)
	}
	return d
}

// pointSyncedBlocks gives every synced block a reference that is valid: a block
// of the same document when the document has ids, otherwise another page's.
func (g *docGen) pointSyncedBlocks(d *Document, ids bool) {
	var targets []string
	for _, b := range flatBlocks(d) {
		if b.Kind != BlockSynced && b.ID != "" {
			targets = append(targets, b.ID)
		}
	}
	for _, b := range flatBlocks(d) {
		if b.Kind != BlockSynced {
			continue
		}
		if ids && len(targets) > 0 && g.chance(75) {
			b.SyncRef = &SyncRef{BlockID: targets[g.r.IntN(len(targets))]}
			continue
		}
		b.SyncRef = &SyncRef{BlockID: "b_" + strconv.Itoa(1+g.r.IntN(30)), PageID: "pg_" + g.tokens(propTitle, 1+g.r.IntN(3))}
	}
}

func (g *docGen) addComments(d *Document) {
	blocks := flatBlocks(d)
	if len(blocks) == 0 || !g.chance(50) {
		return
	}
	str := func() string { return g.tokens(propStrAlpha, g.r.IntN(6)) }
	for n := 1 + g.r.IntN(3); n > 0; n-- {
		c := Comment{ID: "c_" + strconv.Itoa(len(d.Comments)+1), BlockID: blocks[g.r.IntN(len(blocks))].ID, Body: str(), Resolved: g.chance(30)}
		if g.chance(50) {
			c.Start, c.Stop = intp(g.r.IntN(5)), intp(5+g.r.IntN(5))
		}
		if g.chance(40) {
			s := &CommentSuggestion{Text: str()}
			if g.chance(50) {
				s.Runs = g.runs(false)
				s.Text = FlattenRuns(s.Runs)
			}
			c.Suggestion = s
		}
		d.Comments = append(d.Comments, c)
	}
}

// ---- validity: a failure must be the codec's, not the generator's ----

func walkAll(d *Document, f func(b *Block, level int)) {
	var walk func(bs []Block, level int)
	walk = func(bs []Block, level int) {
		for i := range bs {
			f(&bs[i], level)
			walk(bs[i].Children, level+1)
		}
	}
	for i := range d.Sections {
		walk(d.Sections[i].Blocks, 0)
	}
}

func cloneDoc(d *Document) *Document {
	raw, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var out Document
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return &out
}

// propValid applies the model's own validator and the invariants GetDocJSON
// relies on, plus the canonical-form rules of ToMarkdown's contract.
func propValid(d *Document) error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schemaVersion %q", d.SchemaVersion)
	}
	if len(d.Sections) == 0 {
		return fmt.Errorf("no sections")
	}
	ids := map[string]bool{}
	var firstErr error
	fail := func(format string, a ...any) {
		if firstErr == nil {
			firstErr = fmt.Errorf(format, a...)
		}
	}
	for i, sec := range d.Sections {
		if sec.ID != i+1 {
			fail("section %d has id %d", i, sec.ID)
		}
		for _, b := range sec.Blocks {
			if err := validateEditableBlock(b); err != nil {
				fail("section %d: %v", sec.ID, err)
			}
		}
	}
	walkAll(d, func(b *Block, level int) {
		if b.ID != "" {
			if ids[b.ID] {
				fail("duplicate block id %q", b.ID)
			}
			ids[b.ID] = true
		}
		if len(b.Runs) > 0 && b.Text != FlattenRuns(b.Runs) {
			fail("%s block: Text %q != FlattenRuns %q", b.Kind, b.Text, FlattenRuns(b.Runs))
		}
		if len(b.Runs) == 1 && len(b.Runs[0].Marks) == 0 && b.Runs[0].Link == "" {
			fail("%s block: a single plain run must be plain Text", b.Kind)
		}
		// A text is never split by a blank line, except a quote's, whose paragraphs
		// are separated by exactly one.
		switch b.Kind {
		case BlockParagraph, BlockHeading, BlockQuote, BlockToggle, BlockCallout, BlockTodo:
			paragraphs := []string{b.Text}
			if b.Kind == BlockQuote {
				paragraphs = strings.Split(b.Text, "\n\n")
			}
			for _, p := range paragraphs {
				if !okText(p) || (b.Kind == BlockQuote && b.Text != "" && !nonBlank(p)) {
					fail("%s block: Text %q has a blank line or an empty paragraph", b.Kind, b.Text)
				}
			}
		}
		if len(b.Items) > 0 && b.Kind != BlockList {
			fail("%s block has list items", b.Kind)
		}
		if b.Kind == BlockTable && b.Rows != nil && len(b.Rows) == 0 {
			fail("table has an empty non-nil rows (the reader gives nil)")
		}
		if len(b.Children) > 0 {
			switch b.Kind {
			case BlockQuote, BlockToggle, BlockTodo, BlockCallout:
			default:
				fail("%s block has children", b.Kind)
			}
		}
		prev := -1
		for j, it := range b.Items {
			if len(it.Runs) > 0 && it.Text != FlattenRuns(it.Runs) {
				fail("list item %d: Text != FlattenRuns", j)
			}
			if it.Level < 0 || it.Level > prev+1 {
				fail("list item %d: level %d after %d", j, it.Level, prev)
			}
			prev = it.Level
		}
	})
	walkAll(d, func(b *Block, level int) {
		if b.Kind == BlockSynced && b.SyncRef.PageID == "" && !ids[b.SyncRef.BlockID] {
			fail("synced block points at %q, which is not in the document", b.SyncRef.BlockID)
		}
	})
	for _, c := range d.Comments {
		if !ids[c.BlockID] {
			fail("comment %q names missing block %q", c.ID, c.BlockID)
		}
	}
	propIDs := map[string]bool{}
	for _, p := range d.Properties {
		if err := validatePropertyType(p.Type); err != nil {
			fail("property %q: %v", p.ID, err)
		}
		propIDs[p.ID] = true
	}
	for k := range d.PropertyValues {
		if !propIDs[k] {
			fail("property value for unknown property %q", k)
		}
	}
	want := cloneDoc(d)
	rebuildOutline(want)
	if !reflect.DeepEqual(want.Outline, d.Outline) && !(len(want.Outline) == 0 && len(d.Outline) == 0) {
		fail("outline is not what rebuildOutline derives")
	}
	// Runs flatten to Text everywhere (GetDocJSON runs EnsureTextRunsInvariant).
	inv := cloneDoc(d)
	EnsureTextRunsInvariant(inv)
	if !reflect.DeepEqual(inv, cloneDoc(d)) {
		fail("EnsureTextRunsInvariant changes the document")
	}
	return firstErr
}

func hasIDs(d *Document) bool {
	found := false
	walkAll(d, func(b *Block, level int) {
		if b.ID != "" {
			found = true
		}
	})
	return found
}

// deepDiff returns the path of the first difference between a and b, telling nil
// from empty (reflect.DeepEqual does too, and a JSON dump hides it), or "".
func deepDiff(a, b any) string {
	var diff func(path string, x, y reflect.Value) string
	diff = func(path string, x, y reflect.Value) string {
		if x.Kind() != y.Kind() {
			return fmt.Sprintf("%s: kind %s != %s", path, x.Kind(), y.Kind())
		}
		switch x.Kind() {
		case reflect.Pointer:
			if x.IsNil() != y.IsNil() {
				return fmt.Sprintf("%s: nil %v != nil %v", path, x.IsNil(), y.IsNil())
			}
			if x.IsNil() {
				return ""
			}
			return diff(path, x.Elem(), y.Elem())
		case reflect.Slice:
			if x.IsNil() != y.IsNil() {
				return fmt.Sprintf("%s: nil slice %v != nil slice %v (len %d vs %d)", path, x.IsNil(), y.IsNil(), x.Len(), y.Len())
			}
			if x.Len() != y.Len() {
				return fmt.Sprintf("%s: len %d != %d", path, x.Len(), y.Len())
			}
			for i := 0; i < x.Len(); i++ {
				if d := diff(fmt.Sprintf("%s[%d]", path, i), x.Index(i), y.Index(i)); d != "" {
					return d
				}
			}
		case reflect.Map:
			if x.IsNil() != y.IsNil() {
				return fmt.Sprintf("%s: nil map %v != nil map %v", path, x.IsNil(), y.IsNil())
			}
			if x.Len() != y.Len() {
				return fmt.Sprintf("%s: map len %d != %d", path, x.Len(), y.Len())
			}
			for _, k := range x.MapKeys() {
				yv := y.MapIndex(k)
				if !yv.IsValid() {
					return fmt.Sprintf("%s[%v]: missing", path, k)
				}
				if d := diff(fmt.Sprintf("%s[%v]", path, k), x.MapIndex(k), yv); d != "" {
					return d
				}
			}
		case reflect.Struct:
			for i := 0; i < x.NumField(); i++ {
				if d := diff(path+"."+x.Type().Field(i).Name, x.Field(i), y.Field(i)); d != "" {
					return d
				}
			}
		default:
			if !reflect.DeepEqual(x.Interface(), y.Interface()) {
				return fmt.Sprintf("%s: %#v != %#v", path, x.Interface(), y.Interface())
			}
		}
		return ""
	}
	return diff("doc", reflect.ValueOf(a), reflect.ValueOf(b))
}

// ---- the properties ----

func ptrJSON(d *Document) string {
	if d == nil {
		return "nil"
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// propCheck returns the first violated property of d, or nil.
func propCheck(d *Document) error { return propCheckN(d, 10) }

// propCheckN is propCheck with n calls in the determinism check.
func propCheckN(d *Document, n int) error {
	md := ToMarkdown(d)

	// P4: determinism, on the same value and on a fresh copy (fresh maps iterate
	// in a different order).
	for i := 0; i < n; i++ {
		if again := ToMarkdown(d); again != md {
			return fmt.Errorf("P4: ToMarkdown is not deterministic\n first: %q\nthen:  %q", md, again)
		}
	}
	if again := ToMarkdown(cloneDoc(d)); again != md {
		return fmt.Errorf("P4: ToMarkdown of a copy differs\n first: %q\n copy:  %q", md, again)
	}

	// P1: identity with Prev, ids included. A document without ids has nothing
	// for Prev to give back, so the reader numbers the blocks and ids are
	// compared away.
	got, err := FromMarkdown(md, FromMarkdownOptions{Prev: d})
	if err != nil {
		return fmt.Errorf("P1: strict read of canonical Markdown failed: %w\nmarkdown: %q", err, md)
	}
	if hasIDs(d) {
		if !reflect.DeepEqual(got, d) {
			return fmt.Errorf("P1: FromMarkdown(ToMarkdown(d), {Prev: d}) != d at %s\nmarkdown: %q", deepDiff(got, d), md)
		}
	} else if !reflect.DeepEqual(stripIDsAndSpans(cloneDoc(got)), stripIDsAndSpans(cloneDoc(d))) {
		return fmt.Errorf("P1 (no ids): FromMarkdown(ToMarkdown(d), {Prev: d}) != d modulo ids at %s\nmarkdown: %q", deepDiff(stripIDsAndSpans(cloneDoc(got)), stripIDsAndSpans(cloneDoc(d))), md)
	}
	if hasIDs(d) {
		if again := ToMarkdown(got); again != md {
			return fmt.Errorf("P3 (after Prev): Markdown is not a fixed point\n first: %q\nsecond: %q", md, again)
		}
	}

	// P2: identity without Prev, modulo ids; P2b: the trailer gives the ids back.
	got2, err := FromMarkdown(md, FromMarkdownOptions{})
	if err != nil {
		return fmt.Errorf("P2: strict read without Prev failed: %w\nmarkdown: %q", err, md)
	}
	if !reflect.DeepEqual(stripIDsAndSpans(cloneDoc(got2)), stripIDsAndSpans(cloneDoc(d))) {
		return fmt.Errorf("P2: FromMarkdown(ToMarkdown(d), {}) != d modulo ids at %s\nmarkdown: %q", deepDiff(stripIDsAndSpans(cloneDoc(got2)), stripIDsAndSpans(cloneDoc(d))), md)
	}
	if !reflect.DeepEqual(got2, d) {
		return fmt.Errorf("P2b: FromMarkdown(ToMarkdown(d), {}) != d (ids included) at %s\nmarkdown: %q", deepDiff(got2, d), md)
	}

	// P3: serializer idempotence.
	if again := ToMarkdown(got2); again != md {
		return fmt.Errorf("P3: ToMarkdown(FromMarkdown(ToMarkdown(d))) != ToMarkdown(d)\n first: %q\nsecond: %q", md, again)
	}
	return nil
}

// ---- shrinking: a failure prints the smallest document that still fails ----

// shrink greedily removes blocks, sections, children, list items, rich text and
// document-level data from d while cur still fails check and stays valid.
func shrink(d *Document, check func(*Document) error) *Document {
	deadline := time.Now().Add(shrinkBudget)
	first := check(d)
	if first == nil {
		return d
	}
	label := strings.SplitN(first.Error(), ":", 2)[0]
	// A candidate must fail the SAME property, or the shrinker wanders to another bug.
	fails := func(c *Document) bool {
		err := check(c)
		return propValid(c) == nil && err != nil && strings.SplitN(err.Error(), ":", 2)[0] == label
	}
	cur := cloneDoc(d)
	if !fails(cur) {
		return d // the copy no longer fails (nil versus empty): keep the original
	}
	blockCount := func(c *Document) int {
		n := 0
		walkAll(c, func(*Block, int) { n++ })
		return n
	}
	type step struct {
		count func(c *Document) int
		apply func(c *Document, n int) bool
	}
	steps := []step{
		{blockCount, func(c *Document, n int) bool { // remove the nth block
			return mutateNth(c, n, func(list *[]Block, i int) { *list = append((*list)[:i:i], (*list)[i+1:]...) })
		}},
		{blockCount, func(c *Document, n int) bool { // drop the nth block's children
			return mutateNth(c, n, func(list *[]Block, i int) { (*list)[i].Children = nil })
		}},
		{blockCount, func(c *Document, n int) bool { // make the nth block's text plain
			return mutateNth(c, n, func(list *[]Block, i int) { (*list)[i].Runs = nil })
		}},
		{func(c *Document) int { return len(styledCells(c)) }, func(c *Document, n int) bool { // make the nth styled cell plain
			cells := styledCells(c)
			if n >= len(cells) {
				return false
			}
			*cells[n].runs = nil
			syncTableCellRuns(cells[n].b)
			return true
		}},
		{func(c *Document) int { return len(styledCells(c)) }, func(c *Document, n int) bool { // keep the first run of the nth styled cell
			cells := styledCells(c)
			if n >= len(cells) || len(*cells[n].runs) < 2 {
				return false
			}
			*cells[n].runs = (*cells[n].runs)[:1]
			*cells[n].text = FlattenRuns(*cells[n].runs)
			syncTableCellRuns(cells[n].b)
			return true
		}},
		{blockCount, func(c *Document, n int) bool { // make the nth table's cells plain
			ok := false
			mutateNth(c, n, func(list *[]Block, i int) {
				if b := &(*list)[i]; b.HeaderRuns != nil || b.RowRuns != nil {
					b.HeaderRuns, b.RowRuns = nil, nil
					ok = true
				}
			})
			return ok
		}},
		{blockCount, func(c *Document, n int) bool { // drop the last body row of the nth table
			ok := false
			mutateNth(c, n, func(list *[]Block, i int) {
				if b := &(*list)[i]; b.Kind == BlockTable && len(b.Rows) > 0 {
					b.Rows = b.Rows[:len(b.Rows)-1]
					if len(b.Rows) == 0 {
						b.Rows = nil // the reader gives nil, never an empty slice
					}
					if b.RowRuns != nil {
						b.RowRuns = b.RowRuns[:len(b.RowRuns)-1]
					}
					syncTableCellRuns(b)
					ok = true
				}
			})
			return ok
		}},
		{blockCount, func(c *Document, n int) bool { // drop the last column of the nth table
			ok := false
			mutateNth(c, n, func(list *[]Block, i int) {
				b := &(*list)[i]
				if b.Kind != BlockTable || len(b.Headers) < 2 {
					return
				}
				w := len(b.Headers) - 1
				b.Headers, b.Aligns = b.Headers[:w], b.Aligns[:w]
				if b.HeaderRuns != nil {
					b.HeaderRuns = b.HeaderRuns[:w]
				}
				for r := range b.Rows {
					b.Rows[r] = b.Rows[r][:w]
					if b.RowRuns != nil && b.RowRuns[r] != nil {
						b.RowRuns[r] = b.RowRuns[r][:w]
					}
				}
				syncTableCellRuns(b)
				ok = true
			})
			return ok
		}},
		{blockCount, func(c *Document, n int) bool { // drop the last list item of the nth block
			ok := false
			mutateNth(c, n, func(list *[]Block, i int) {
				if it := (*list)[i].Items; len(it) > 1 {
					(*list)[i].Items = it[:len(it)-1]
					ok = true
				}
			})
			return ok
		}},
		{func(c *Document) int { return len(c.Sections) }, func(c *Document, n int) bool { // remove the nth section
			if len(c.Sections) == 1 {
				return false
			}
			c.Sections = append(c.Sections[:n:n], c.Sections[n+1:]...)
			return true
		}},
		{func(*Document) int { return 6 }, func(c *Document, n int) bool { // clear document-level data, one kind per n
			switch n {
			case 0:
				c.Comments = nil
			case 1:
				c.PropertyValues = nil
			case 2:
				c.Properties = nil
			case 3:
				c.Meta.Directives = nil
			case 4:
				for i := range c.Sections {
					c.Sections[i].Attrs = nil
				}
			case 5:
				for i := range c.Sections {
					c.Sections[i].Notes = nil
				}
			}
			return true
		}},
	}
	for progress := true; progress && time.Now().Before(deadline); {
		progress = false
		for _, st := range steps {
			for n := 0; n < st.count(cur) && time.Now().Before(deadline); n++ {
				cand := cloneDoc(cur)
				if !st.apply(cand, n) {
					continue
				}
				for i := range cand.Sections {
					cand.Sections[i].ID = i + 1
				}
				nilEmptyBlocks(cand)
				rebuildOutline(cand)
				if reflect.DeepEqual(cand, cur) {
					continue // a step that changed nothing is not progress (it would loop until the budget ends)
				}
				if fails(cand) {
					cur, progress = cand, true
					n--
				}
			}
		}
	}
	return cur
}

// cellRef points at one styled table cell of a document: its text, its runs and
// the table that owns them.
type cellRef struct {
	b    *Block
	text *string
	runs *[]TextRun
}

// styledCells lists the styled cells of every table of d, in document order.
func styledCells(d *Document) []cellRef {
	var out []cellRef
	walkAll(d, func(b *Block, level int) {
		if b.Kind != BlockTable {
			return
		}
		for i := range b.HeaderRuns {
			if len(b.HeaderRuns[i]) > 0 && i < len(b.Headers) {
				out = append(out, cellRef{b, &b.Headers[i], &b.HeaderRuns[i]})
			}
		}
		for r := range b.RowRuns {
			for i := range b.RowRuns[r] {
				if len(b.RowRuns[r][i]) > 0 && r < len(b.Rows) && i < len(b.Rows[r]) {
					out = append(out, cellRef{b, &b.Rows[r][i], &b.RowRuns[r][i]})
				}
			}
		}
	})
	return out
}

// nilEmptyBlocks turns every empty block list into nil, which is what the
// reader produces (an empty non-nil slice would differ under DeepEqual).
func nilEmptyBlocks(d *Document) {
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for i := range bs {
			if len(bs[i].Children) == 0 {
				bs[i].Children = nil
			}
			walk(bs[i].Children)
		}
	}
	for i := range d.Sections {
		if len(d.Sections[i].Blocks) == 0 {
			d.Sections[i].Blocks = nil
		}
		walk(d.Sections[i].Blocks)
	}
}

// mutateNth applies f to the nth block of d in document order (depth first).
func mutateNth(d *Document, n int, f func(list *[]Block, i int)) bool {
	count := 0
	var walk func(list *[]Block) bool
	walk = func(list *[]Block) bool {
		for i := 0; i < len(*list); i++ {
			if count == n {
				f(list, i)
				return true
			}
			count++
			if walk(&(*list)[i].Children) {
				return true
			}
		}
		return false
	}
	for i := range d.Sections {
		if walk(&d.Sections[i].Blocks) {
			return true
		}
	}
	return false
}

// ---- tests ----

// propScale multiplies the documents per seed when EDEN_CODEC_PROP_SCALE is set,
// for a heavier hunt (EDEN_CODEC_PROP_SCALE=50 is 50,000 documents). The default
// run is the fixed 20 seeds of 50.
func propScale() uint64 {
	if n, err := strconv.ParseUint(os.Getenv("EDEN_CODEC_PROP_SCALE"), 10, 64); err == nil && n > 0 {
		return n
	}
	return 1
}

// propDocs yields every document of the run: propSeeds seeds of propDocsPerSeed.
func propDocs(f func(seed, index uint64, d *Document)) {
	for seed := uint64(1); seed <= propSeeds; seed++ {
		for index := uint64(0); index < propDocsPerSeed*propScale(); index++ {
			f(seed, index, genDocument(propRand(seed, index), propDepth))
		}
	}
}

// TestCodecPropertyGeneratorIsValidAndReproducible: every generated document is
// valid by the model's own rules, and one (seed, index) pair always gives the
// same document, so a printed failure can be replayed.
func TestCodecPropertyGeneratorIsValidAndReproducible(t *testing.T) {
	bad := 0
	propDocs(func(seed, index uint64, d *Document) {
		if err := propValid(d); err != nil && bad < 5 {
			bad++
			t.Errorf("seed=%d index=%d: generator made an invalid document: %v\n%s", seed, index, err, ptrJSON(d))
		}
		if again := genDocument(propRand(seed, index), propDepth); !reflect.DeepEqual(d, again) && bad < 5 {
			bad++
			t.Errorf("seed=%d index=%d: the generator is not deterministic", seed, index)
		}
	})
}

// TestCodecPropertyIdentity is D10's property test: P1 to P4 over 1,000
// generated documents (20 seeds of 50), every kind at least 20 times.
func TestCodecPropertyIdentity(t *testing.T) {
	kinds := map[BlockKind]int{}
	feature := map[string]int{}
	docs, failures := 0, 0
	for seed := uint64(1); seed <= propSeeds; seed++ {
		t.Run("seed-"+strconv.FormatUint(seed, 10), func(t *testing.T) {
			for index := uint64(0); index < propDocsPerSeed*propScale(); index++ {
				d := genDocument(propRand(seed, index), propDepth)
				docs++
				countFeatures(d, kinds, feature)
				if err := propCheck(d); err != nil {
					failures++
					if failures > 3 {
						continue
					}
					small := shrink(d, func(c *Document) error { return propCheckN(c, 2) })
					smallErr := propCheckN(small, 2)
					t.Errorf("property violated: seed=%d index=%d (replay: genDocument(propRand(%d, %d), %d))\n%v\n\n--- smallest failing document ---\n%s\n--- its Markdown ---\n%q\n--- its violation ---\n%v",
						seed, index, seed, index, propDepth, err, ptrJSON(small), ToMarkdown(small), smallErr)
				}
			}
		})
	}
	t.Logf("%d documents, %d property failures", docs, failures)
	if want := int(propSeeds * propDocsPerSeed * propScale()); docs != want {
		t.Fatalf("ran %d documents, want %d", docs, want)
	}
	for _, k := range propKinds {
		t.Logf("kind %-9s %5d", k, kinds[k])
		if kinds[k] < propMinPerKind {
			t.Errorf("block kind %q appeared %d times, want at least %d", k, kinds[k], propMinPerKind)
		}
	}
	for name, n := range feature {
		t.Logf("feature %-26s %5d", name, n)
	}
	for _, name := range []string{
		"ids", "no ids", "comments", "properties", "property values", "front matter", "section attrs", "section notes",
		"several sections", "rich text", "rich list item", "checked todo", "unchecked todo", "sheet snapshot", "sheet range",
		"nested 4 deep", "synced same document", "synced other page", "hard break", "figure svg", "ordered list", "table",
		// formatted table cells (AODex D14)
		"styled header cell", "styled body cell", "cell link", "cell code with pipe", "cell math with pipe", "cell written with tags",
	} {
		if feature[name] < propMinPerKind {
			t.Errorf("feature %q appeared %d times, want at least %d", name, feature[name], propMinPerKind)
		}
	}
}

// countFeatures tallies what d contains (once per document for features).
func countFeatures(d *Document, kinds map[BlockKind]int, feature map[string]int) {
	seen := map[string]bool{}
	mark := func(name string) {
		if !seen[name] {
			seen[name] = true
			feature[name]++
		}
	}
	if hasIDs(d) {
		mark("ids")
	} else {
		mark("no ids")
	}
	if len(d.Comments) > 0 {
		mark("comments")
	}
	if len(d.Properties) > 0 {
		mark("properties")
	}
	if len(d.PropertyValues) > 0 {
		mark("property values")
	}
	if len(d.Meta.Directives) > 0 {
		mark("front matter")
	}
	if len(d.Sections) > 1 {
		mark("several sections")
	}
	for _, s := range d.Sections {
		if len(s.Attrs) > 0 {
			mark("section attrs")
		}
		if len(s.Notes) > 0 {
			mark("section notes")
		}
	}
	walkAll(d, func(b *Block, level int) {
		kinds[b.Kind]++
		if len(b.Runs) > 0 {
			mark("rich text")
		}
		if strings.Contains(b.Text, "\n") {
			mark("hard break")
		}
		if level >= 3 {
			mark("nested 4 deep")
		}
		switch b.Kind {
		case BlockTodo:
			if b.Checked {
				mark("checked todo")
			} else {
				mark("unchecked todo")
			}
		case BlockSheet:
			if b.SheetEmbed.Snapshot != nil {
				mark("sheet snapshot")
			}
			if b.SheetEmbed.Range != nil {
				mark("sheet range")
			}
		case BlockSynced:
			if b.SyncRef.PageID == "" {
				mark("synced same document")
			} else {
				mark("synced other page")
			}
		case BlockFigure:
			if b.SVG != "" {
				mark("figure svg")
			}
		case BlockList:
			if b.Ordered {
				mark("ordered list")
			}
			for _, it := range b.Items {
				if len(it.Runs) > 0 {
					mark("rich list item")
				}
			}
		case BlockTable:
			mark("table")
			countCellFeatures(b, mark)
		}
	})
}

// cellTagFallback matches a mark tag that is markup, not escaped text.
var cellTagFallback = regexp.MustCompile(`(^|[^\\])<(strong|em|del)>`)

// countCellFeatures marks what the styled cells of table b contain (the formatted
// cells of AODex D14), once per document.
func countCellFeatures(b *Block, mark func(string)) {
	cell := func(text string, runs []TextRun, header bool) {
		if len(runs) == 0 {
			return
		}
		if header {
			mark("styled header cell")
		} else {
			mark("styled body cell")
		}
		for _, r := range runs {
			switch {
			case r.Link != "":
				mark("cell link")
			case hasMark(r, "code") && strings.Contains(r.Text, "|"):
				mark("cell code with pipe")
			case hasMark(r, "math") && strings.Contains(r.Text, "|"):
				mark("cell math with pipe")
			}
		}
		if cellTagFallback.MatchString(cellText(text, runs)) {
			mark("cell written with tags")
		}
	}
	for c, runs := range b.HeaderRuns {
		cell(b.Headers[c], runs, true)
	}
	for r, row := range b.RowRuns {
		for c, runs := range row {
			cell(b.Rows[r][c], runs, false)
		}
	}
}

// ---- regressions found by the property and fuzz targets ----

// TestCodecEdgeUnicodeSpaceSurvives: a Text that starts or ends with a Unicode
// space that is not ASCII whitespace (a form feed, a no-break space, an em space)
// is content, and CommonMark keeps it. ToMarkdown once trimmed the whole body
// with strings.TrimSpace, which dropped it from the first and last block of a
// document. Found by FuzzFromMarkdown ("000000000000000Œ\f"), seed f53fcd3813bd1b04.
func TestCodecEdgeUnicodeSpaceSurvives(t *testing.T) {
	spaces := []string{"\f", "\v", " ", " ", "\u0085", " ", "　", "​"}
	for _, sp := range spaces {
		for _, text := range []string{"end" + sp, sp + "start", sp + "both" + sp, sp} {
			for _, kind := range []BlockKind{BlockParagraph, BlockHeading, BlockQuote} {
				blk := Block{Kind: kind, Text: text}
				if kind == BlockHeading {
					blk.Level = 2
				}
				d := fxDoc([]Block{blk})
				rebuildOutline(d)
				name := fmt.Sprintf("%s/%U/%q", kind, []rune(sp)[0], text)
				md := ToMarkdown(d)
				got, err := FromMarkdown(md, FromMarkdownOptions{})
				if err != nil || !reflect.DeepEqual(got, d) {
					t.Errorf("%s does not round-trip (err %v)\n markdown: %q\n want: %q\n  got: %s", name, err, md, text, ptrJSON(got))
				}
			}
		}
	}
	// Only the document's own first and last block touched the trim, but check a
	// middle one and a section's too.
	d := fxDoc([]Block{fxPara("a "), fxPara(" b")}, []Block{fxPara("\f")})
	md := ToMarkdown(d)
	if got, err := FromMarkdown(md, FromMarkdownOptions{}); err != nil || !reflect.DeepEqual(got, d) {
		t.Errorf("multi-section document does not round-trip (err %v)\n markdown: %q\n got: %s", err, md, ptrJSON(got))
	}
}

// TestCodecEmptyJSONCollectionsReadAsNil: a header or trailer that spells an
// empty array or object (`"rowIds":[]`) reads as the same Document as one that
// leaves the field out, because the writer omits an empty field and a Document
// must be a fixed point of write-then-read. Found by FuzzFromMarkdown (seed
// 0ff3a2785fbbe76a): the read gave an empty non-nil slice, the re-read nil.
func TestCodecEmptyJSONCollectionsReadAsNil(t *testing.T) {
	cases := map[string]string{
		"snapshot rowIds":     ":::sheet {\"sheetEmbed\":{\"sheet\":\"S\",\"snapshot\":{\"rowIds\":[]}}}\n:::\n",
		"snapshot all empty":  ":::sheet {\"sheetEmbed\":{\"sheet\":\"S\",\"snapshot\":{\"columns\":[],\"rowIds\":[],\"cells\":[]}}}\n:::\n",
		"column options":      ":::sheet {\"sheetEmbed\":{\"snapshot\":{\"columns\":[{\"id\":\"c\",\"name\":\"n\",\"type\":\"select\",\"options\":[]}]}}}\n:::\n",
		"cell rels":           ":::sheet {\"sheetEmbed\":{\"snapshot\":{\"cells\":[{\"r\":0,\"c\":0,\"rels\":[]}]}}}\n:::\n",
		"range ids":           ":::sheet {\"sheetEmbed\":{\"sheet\":\"S\",\"range\":{\"columnIds\":[],\"rowIds\":[]}}}\n:::\n",
		"trailer properties":  "x\n\n<!-- eden:model {\"v\":1,\"properties\":[],\"propertyValues\":{},\"comments\":[]} -->\n",
		"property options":    "x\n\n<!-- eden:model {\"v\":1,\"properties\":[{\"id\":\"p\",\"name\":\"n\",\"type\":\"select\",\"options\":[]}]} -->\n",
		"suggestion runs":     "x\n\n<!-- eden:model {\"v\":1,\"comments\":[{\"id\":\"c\",\"blockId\":\"b\",\"body\":\"t\",\"suggestion\":{\"text\":\"s\",\"runs\":[]}}]} -->\n",
		"empty attrs object":  "<!-- eden:attrs {} -->\n\nx\n",
		"trailer blocks list": "x\n\n<!-- eden:model {\"v\":1,\"blocks\":[]} -->\n",
	}
	for name, src := range cases {
		for _, lenient := range []bool{false, true} {
			d, err := FromMarkdown(src, FromMarkdownOptions{Lenient: lenient})
			if err != nil {
				continue // refused is fine; only a successful read must be a fixed point
			}
			md := ToMarkdown(d)
			d2, err := FromMarkdown(md, FromMarkdownOptions{})
			if err != nil || !reflect.DeepEqual(d, d2) {
				t.Errorf("%s (lenient=%v): read is not a fixed point (err %v) at %s\n source: %q\n canonical: %q", name, lenient, err, deepDiff(d2, d), src, md)
			}
		}
	}
}

// ---- P5: strict mode never drops content silently ----

// nodropTemplates are Markdown constructs with one or two %s holes. Each hole is
// filled with a unique sentinel word. If strict FromMarkdown accepts the source,
// every sentinel must be somewhere in the Document, because the strict contract
// is "refuse, never silently drop". (An unused `[x]: url` definition and a
// numbering start are normalizations that carry no content, so they have no hole.)
var nodropTemplates = []string{
	"%s", "a %s b", "*%s*", "**%s**", "~~%s~~", "`%s`", "[%s](http://x.io)", "[a](http://%s.io)", "![%s](p.png)", "![a](%s.png)",
	"$%s$", "<http://%s.io>", "&%s;", "text\\\n%s", "line one\n%s", "[a][r]\n\n[r]: http://%s.io",
	"# %s", "%s\n=====", "## a %s ##", "> %s", "> a\n>\n> %s", "- %s\n- %s", "- a\n  - %s", "1. %s\n2. %s", "1. a\n   1. %s",
	"- [ ] %s", "- [x] %s\n- [ ] %s", "- [ ] a\n\n  %s", "```%s\ncode\n```", "```\n%s\n```", "````\n```\n%s\n```\n````", "~~~\n%s\n~~~", "    %s",
	"$$\n%s\n$$", "$$%s$$", "$$\n%s\n$$ %s", "$$\n%s $$", "$$\na\n$$\n%s",
	"| %s | b |\n| - | - |\n| c | %s |", "| a |\n| :-: |\n| %s |", "***", "---", "- - -",
	":::toggle\n%s\n:::", ":::toggle {\"collapsed\":true}\na\n\n%s\n:::", ":::callout {\"icon\":\"%s\"}\n:::", ":::callout {\"tone\":\"%s\"}\nx\n:::",
	":::file {\"name\":\"%s\",\"src\":\"x\"}\n:::", ":::file {\"src\":\"%s\"}\n:::", ":::figure {\"title\":\"%s\"}\n<svg/>\n:::",
	":::figure\n<svg>%s</svg>\n:::", ":::sheet {\"sheetEmbed\":{\"sheet\":\"%s\"}}\n:::", ":::synced {\"syncRef\":{\"blockId\":\"%s\"}}\n:::",
	"<!-- %s -->", "<!-- eden:note \"%s\" -->", "<!-- _class: %s -->", "<!-- eden:attrs {\"k\":\"%s\"} -->",
	"<div>%s</div>", "<b>%s</b>", "a <!-- %s --> b", "- a\n\n  %s", "- a\n  ```\n  %s\n  ```", "> ```\n> %s\n> ```",
	"---\ntitle: %s\n---", "a\n\n---\n\n%s", "a  \n%s", "\\*%s\\*", "[a](http://x.io \"%s\")", "[](http://%s.io)",
	"* %s\n* b", "+ %s", "1) %s", "- a\n    - %s", "- a\n\n- %s", "text[^%s]", "a<br>%s", "<kbd>%s</kbd>", "<details>\n<summary>%s</summary>\n</details>",
	"%s\n---\n", "***%s***", "_%s_ and __b__", "[a](<http://x.io/%s y>)", "![a](p.png \"%s\")", "![%s][i]\n\n[i]: p.png", "http://%s.io", ":%s:", "\\%s",
	"| %s |\n| --- |", "| a | b |\n| - | - |\n| %s |", "| a | b |\n| - | - |\n| %s | **b** |", "| a |\n| - |\n| %s | extra |", "> > %s", "> - %s\n> - b", "> # %s",
	"- a\n  > %s", "- a\n  # %s", "1. a\n   - %s", "- [ ] a\n  - [ ] %s", "- [ ] a\n\n  - %s", "$$\n%s\n$$\n\n$$\nb\n$$", "$%s$ and $b$", "$$%s$$ x",
	":::toggle\n:::toggle\n%s\n:::\n:::", "::::toggle\n:::callout\n%s\n:::\n::::", ":::unknown\n%s\n:::", ":::toggle {\"x\":\"%s\"}\n:::", ":::toggle\n%s",

	// Formatted table cells (AODex D14): a hole in every kind of cell content. Raw
	// HTML, an image and a link title in a cell are refused as in a paragraph, and
	// a refusal drops nothing.
	"| **%s** | b |\n| - | - |\n| c | *%s* |", "| ~~%s~~ |\n| - |\n| `c` |", "| a | b |\n| - | - |\n| `%s` | [x](http://%s.io) |",
	"| a |\n| - |\n| [%s](http://x.io) |", "| a |\n| - |\n| $%s$ |", "| a |\n| - |\n| <http://%s.io> |", "| a |\n| - |\n| `x\\|%s` |",
	"| a |\n| - |\n| $x\\|%s$ |", "| a |\n| - |\n| x<br>%s |", "| a |\n| - |\n| ![%s](p.png) |", "| a |\n| - |\n| ![a](%s.png) |",
	"| a |\n| - |\n| [a](u \"%s\") |", "| a |\n| - |\n| **%s** and `c` and [d](e) |", "| a | b |\n| - | - |\n| *%s* |",
	"| a |\n| - |\n| [a][r] |\n\n[r]: http://%s.io", "| `a` | **%s** |\n| :-: | --: |\n| _%s_ | __c__ |", "| a |\n| - |\n| <strong>%s</strong> |",
	"| a |\n| - |\n| <em>%s |", "| a |\n| - |\n| $$%s$$ |", "| a |\n| - |\n| %s ~~x~~ |",

	// Wide table rows (AODex D18): a hole in a cell beyond the header's width. The
	// reader pads the header, so a cell there is kept, as every sentinel must be.
	"| a |\n| - |\n| x | %s |", "| a | b |\n| - | - |\n| x | y | %s |", "| a |\n| - |\n| x | **%s** | `%s` |",
	"| a |\n| - |\n| x | *%s* |\n| y | z | %s |", "| a |\n| - |\n| x | [%s](http://x.io) | $%s$ |", "| a |\n| - |\n| x | `p\\|%s` |",
	"| a |\n| - |\n| x | $p\\|%s$ |", "| a |\n| - |\n| x | p\\|%s |", "| a |\n| - |\n| x | ![%s](p.png) |", "| a |\n| - |\n| x | y<br>%s |",
	"a | b\n--- | ---\n1 | 2 | %s", "| a |\n| - |\n| x | |\n| %s | y | z |", "| a | b |\n| :-: | --: |\n| %s |\n| x | y | %s |",
	"| a |\n| - |\n| x | <strong>%s</strong> |", "| a |\n| - |\n| x | <em>%s |", "| a |\n| - |\n| x | [a](u \"%s\") |", "| a |\n| - |\n| x | y | z | w | %s |",
}

var nodropContexts = []func(string) string{
	func(s string) string { return s },
	func(s string) string { return prefixLinesTest(s, "> ") },
	func(s string) string { return "- " + strings.ReplaceAll(s, "\n", "\n  ") },
	func(s string) string { return ":::toggle\n" + s + "\n:::" },
	func(s string) string { return ":::callout\nlead\n\n" + s + "\n:::" },
	func(s string) string { return "- [ ] t\n\n  " + strings.ReplaceAll(s, "\n", "\n  ") },
}

func prefixLinesTest(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(prefix+ln, " ")
	}
	return strings.Join(lines, "\n")
}

// TestCodecStrictNeverDropsContent is property P5. The sources are random
// assemblies of nodropTemplates in random containers, so it covers hand-written
// shapes that the canonical-form properties (which only start from a Document)
// cannot reach. Found by reading the math reader: text after a closing `$$` was
// dropped without an error.
func TestCodecStrictNeverDropsContent(t *testing.T) {
	rng := rand.New(rand.NewPCG(1026, 5))
	letters := "abcdefghijklmnopqrstuvwxyz"
	sentinel := func() string {
		b := []byte("zq")
		for i := 0; i < 8; i++ {
			b = append(b, letters[rng.IntN(len(letters))])
		}
		return string(b)
	}
	read, accepted, bad := 0, 0, 0
	byTemplate := map[string]int{}
	for i := 0; i < 20000 && bad < 8; i++ {
		var parts []string
		var words []string
		for n := 1 + rng.IntN(3); n > 0; n-- {
			tmpl := nodropTemplates[rng.IntN(len(nodropTemplates))]
			args := make([]any, strings.Count(tmpl, "%s"))
			for k := range args {
				w := sentinel()
				args[k] = w
				words = append(words, w)
			}
			parts = append(parts, fmt.Sprintf(tmpl, args...))
		}
		src := strings.Join(parts, "\n\n")
		if rng.IntN(3) > 0 {
			src = nodropContexts[rng.IntN(len(nodropContexts))](src)
		}
		src += "\n"
		if strings.Count(src, "<!-- _class:") > 1 || strings.Count(src, "<!-- eden:attrs") > 1 || strings.Count(src, "[r]:") > 1 {
			// By design a later directive of the same key replaces the earlier one
			// (Marp), and CommonMark keeps the first definition of a label.
			continue
		}
		read++
		d, err := FromMarkdown(src, FromMarkdownOptions{})
		if err != nil {
			continue
		}
		accepted++
		raw, _ := json.Marshal(d)
		for _, w := range words {
			if !strings.Contains(string(raw), w) {
				bad++
				byTemplate[w]++
				t.Errorf("strict read accepted the source but dropped %q\n source: %q\n document: %s", w, src, raw)
				break
			}
		}
	}
	t.Logf("%d sources, %d accepted by strict mode, %d dropped content", read, accepted, bad)
	if accepted < read/10 {
		t.Fatalf("only %d of %d sources were accepted, so the test says little", accepted, read)
	}
}

// ---- wide table rows (AODex D18) ----

// wideCell is one kind of cell a generated wide-row table holds: the Markdown with
// holes for sentinels, and what the cell's text reads as. Every kind is one the
// codec accepts in strict mode, so a refusal is a failure of the property.
type wideCell struct {
	md   string
	text func(w []string) string
}

var wideCells = []wideCell{
	{"%s", func(w []string) string { return w[0] }},
	{"**%s**", func(w []string) string { return w[0] }},
	{"*%s*", func(w []string) string { return w[0] }},
	{"~~%s~~", func(w []string) string { return w[0] }},
	{"`%s`", func(w []string) string { return w[0] }},
	{"[%s](http://x.io)", func(w []string) string { return w[0] }},
	{"$%s$", func(w []string) string { return w[0] }},
	{"<strong>%s</strong>", func(w []string) string { return w[0] }},
	{"%s\\|%s", func(w []string) string { return w[0] + "|" + w[1] }},
	{"`%s\\|%s`", func(w []string) string { return w[0] + "|" + w[1] }},
	{"$%s\\|%s$", func(w []string) string { return w[0] + "|" + w[1] }},
	{"[%s\\|%s](http://x.io)", func(w []string) string { return w[0] + "|" + w[1] }},
	{"[a](http://x.io/%s\\|%s)", func(w []string) string { return "a" }},
	{"**%s** and `%s`", func(w []string) string { return w[0] + " and " + w[1] }},
	{"", func(w []string) string { return "" }},
}

// wideSource is a generated GFM table whose body rows may be wider than its header,
// with what a reader must make of it.
type wideSource struct {
	src       string
	width     int        // the table's width after padding: its widest row
	text      [][]string // the text of every cell of every body row, "" where a row is narrower
	sentinels []string   // every sentinel placed, in the header, the body and the excess cells
	excess    int        // sentinels in cells beyond the header's width
}

// genWideSource builds a wide-row table from rng: a header of 1 to 4 columns with
// random alignments, 1 to 4 body rows of 1 to header+4 cells, in one of four
// layouts (outer pipes, bare, text before the table, and the spacing around cells
// varied). Every cell holds a unique sentinel unless it is the empty cell.
func genWideSource(rng *rand.Rand) wideSource {
	letters := "abcdefghijklmnopqrstuvwxyz"
	sentinel := func() string {
		b := []byte("zq")
		for i := 0; i < 8; i++ {
			b = append(b, letters[rng.IntN(len(letters))])
		}
		return string(b)
	}
	var ws wideSource
	h := 1 + rng.IntN(4)
	bare := h > 1 && rng.IntN(5) == 0 // a table without outer pipes needs two columns to be a table
	join := func(cells []string) string {
		if bare {
			return strings.Join(cells, " | ")
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	headers := make([]string, h)
	delims := make([]string, h)
	aligns := []string{"---", ":--", "--:", ":-:"}
	for i := range headers {
		headers[i] = sentinel()
		ws.sentinels = append(ws.sentinels, headers[i])
		delims[i] = aligns[rng.IntN(len(aligns))]
	}
	var lines []string
	if rng.IntN(4) == 0 {
		lead := sentinel()
		ws.sentinels = append(ws.sentinels, lead)
		lines = append(lines, "lead "+lead)
	}
	lines = append(lines, join(headers), join(delims))
	ws.width = h
	rows := 1 + rng.IntN(4)
	wide := false
	for r := 0; r < rows; r++ {
		n := 1 + rng.IntN(h+4)
		if r == rows-1 && !wide {
			n = h + 1 + rng.IntN(3) // at least one wide row in every table
		}
		wide = wide || n > h
		cells := make([]string, n)
		texts := make([]string, n)
		for c := range cells {
			kind := wideCells[rng.IntN(len(wideCells))]
			if kind.md == "" && ((n > h && c == n-1) || (bare && (c == 0 || c == n-1))) {
				// Keep a sentinel in the last excess cell. A bare row has no outer pipes to
				// hold an empty first or last cell, and a bare empty row is a blank line.
				kind = wideCells[0]
			}
			args := make([]any, strings.Count(kind.md, "%s"))
			w := make([]string, len(args))
			for k := range args {
				w[k] = sentinel()
				args[k] = w[k]
				ws.sentinels = append(ws.sentinels, w[k])
				if c >= h {
					ws.excess++
				}
			}
			cells[c] = fmt.Sprintf(kind.md, args...)
			texts[c] = kind.text(w)
		}
		lines = append(lines, join(cells))
		ws.text = append(ws.text, texts)
		ws.width = max(ws.width, n)
	}
	for r := range ws.text {
		for len(ws.text[r]) < ws.width {
			ws.text[r] = append(ws.text[r], "")
		}
	}
	ws.src = strings.Join(lines, "\n") + "\n"
	return ws
}

// TestCodecPropertyWideRowSources is the wide-row half of P5 and of identity (AODex
// D18). Random GFM tables whose body rows run past their header, with a sentinel in
// every cell and in every kind of excess cell (plain, bold, code, math, a link, an
// escaped pipe), in the places a table can be. For each, strict and lenient reads
// accept it with no refusal; the table is as wide as its widest row; every
// sentinel is in the cell it was written in and no other cell is changed; spans lie
// inside the input; and the padded document is a fixed point of write-then-read.
// 2,000 sources by default, EDEN_CODEC_PROP_SCALE times that.
func TestCodecPropertyWideRowSources(t *testing.T) {
	contexts := []struct {
		name string
		wrap func(string) string
	}{
		{"top", func(s string) string { return s }},
		{"quote", func(s string) string { return prefixLinesTest(s, "> ") }},
		{"toggle", func(s string) string { return ":::toggle\n" + s + ":::\n" }},
		{"section", func(s string) string { return "# One\n\nx\n\n---\n\n# Two\n\n" + s }},
		{"callout", func(s string) string { return ":::callout\nlead\n\n" + s + ":::\n" }},
	}
	total := 2000 * propScale()
	failures, excess, dropped, tables := 0, 0, 0, 0
	for i := uint64(0); i < total && failures < 5; i++ {
		rng := propRand(31, i)
		ws := genWideSource(rng)
		ctx := contexts[rng.IntN(len(contexts))]
		src := ctx.wrap(ws.src)
		fail := func(format string, args ...any) {
			failures++
			t.Errorf("seed=31 index=%d (replay: genWideSource(propRand(31, %d))), context %s: %s\n source: %q", i, i, ctx.name, fmt.Sprintf(format, args...), src)
		}

		strict, err := FromMarkdown(src, FromMarkdownOptions{})
		if err != nil {
			fail("strict mode refused a wide row: %v", err)
			continue
		}
		lenient, err := FromMarkdown(src, FromMarkdownOptions{Lenient: true})
		if err != nil || !reflect.DeepEqual(strict, lenient) {
			fail("lenient differs from strict (err %v) at %s", err, deepDiff(lenient, strict))
			continue
		}
		raw, _ := json.Marshal(strict)
		for _, w := range ws.sentinels {
			if !strings.Contains(string(raw), w) {
				dropped++
				fail("dropped the sentinel %q", w)
				break
			}
		}
		excess += ws.excess
		tbs := tablesOf(strict)
		if len(tbs) != 1 {
			fail("want one table, got %d", len(tbs))
			continue
		}
		tables++
		b := tbs[0]
		if len(b.Headers) != ws.width || len(b.Aligns) != ws.width {
			fail("the table is %d headers and %d aligns wide, want %d", len(b.Headers), len(b.Aligns), ws.width)
			continue
		}
		if len(b.Rows) != len(ws.text) {
			fail("%d rows, want %d", len(b.Rows), len(ws.text))
			continue
		}
		for r, row := range b.Rows {
			if !reflect.DeepEqual(row, ws.text[r]) {
				fail("row %d is %q, want %q", r, row, ws.text[r])
				break
			}
		}
		if err := propValid(strict); err != nil {
			fail("the padded document is not valid: %v", err)
			continue
		}
		if err := propCheckN(strict, 1); err != nil {
			fail("the padded document: %v", err)
			continue
		}
		withSpans, err := FromMarkdown(src, FromMarkdownOptions{Spans: true})
		if err != nil {
			fail("read with Spans failed: %v", err)
			continue
		}
		checkSpansInside(t, withSpans, len(src))
	}
	t.Logf("%d sources, %d tables, %d sentinels in excess cells, %d dropped, %d failures", total, tables, excess, dropped, failures)
	if failures == 0 && (tables != int(total) || excess < int(total)) {
		t.Fatalf("ran %d tables of %d sources with %d excess sentinels: the generator says little", tables, total, excess)
	}
}

// TestCodecMathTrailingText: text after the closing `$$` of a display math block
// used to be dropped without an error, which strict mode must never do. Found by
// TestCodecStrictNeverDropsContent.
func TestCodecMathTrailingText(t *testing.T) {
	cases := []struct{ name, src string }{
		{"top level", "$$\nx\n$$ tail\n"},
		{"in a quote", "> $$\n> x\n> $$ tail\n"},
		{"in a toggle", ":::toggle\n$$\nx\n$$ tail\n:::\n"},
		{"after TeX on the closing line", "$$\nx $$ tail\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at := strings.Index(tc.src, "tail")
			_, err := FromMarkdown(tc.src, FromMarkdownOptions{})
			var ue *UnrepresentableError
			if !errors.As(err, &ue) || ue.Construct != ConstructMathTrailingText || ue.Offset != at {
				t.Fatalf("strict read: got %v, want %s at byte %d", err, ConstructMathTrailingText, at)
			}

			d, err := FromMarkdown(tc.src, FromMarkdownOptions{Lenient: true, Spans: true})
			if err != nil {
				t.Fatalf("lenient read failed: %v", err)
			}
			var math, para *Block
			walkAll(d, func(b *Block, level int) {
				switch b.Kind {
				case BlockMath:
					math = b
				case BlockParagraph:
					para = b
				}
			})
			if math == nil || !math.Display || para == nil || para.Text != "tail" {
				t.Fatalf("lenient mapping is not display math followed by the paragraph \"tail\":\n%s", ptrJSON(d))
			}
			if math.Span == nil || para.Span == nil {
				t.Fatalf("both blocks need a span: math %v, paragraph %v", math.Span, para.Span)
			}
			if got := tc.src[para.Span.Start:para.Span.Stop]; got != "tail" {
				t.Errorf("paragraph span covers %q, want \"tail\"", got)
			}
			if got := tc.src[math.Span.Start:math.Span.Stop]; strings.Contains(got, "tail") || !strings.HasSuffix(got, "$$") {
				t.Errorf("math span covers %q, want the block up to its closing $$", got)
			}

			plain, err := FromMarkdown(tc.src, FromMarkdownOptions{Lenient: true})
			if err != nil {
				t.Fatal(err)
			}
			md := ToMarkdown(plain)
			again, err := FromMarkdown(md, FromMarkdownOptions{})
			if err != nil || !reflect.DeepEqual(again, plain) {
				t.Errorf("lenient result is not a fixed point (err %v)\n canonical: %q", err, md)
			}
		})
	}
}

// TestCodecImageTitleKeepsALineBreak: CommonMark lets a title span lines, and the
// reader keeps the newline in Title. The writer once turned it into a space, so the
// Document read back differently. Found by FuzzFromMarkdown (seed 38ad7dda22d0f39e).
func TestCodecImageTitleKeepsALineBreak(t *testing.T) {
	for _, title := range []string{"0\n", "two\nlines", "a\nb\nc", "x\n y", "\nlead"} {
		d := fxDoc([]Block{{Kind: BlockImage, Src: "p.png", Text: "alt", Title: title}})
		md := ToMarkdown(d)
		got, err := FromMarkdown(md, FromMarkdownOptions{})
		if err != nil || !reflect.DeepEqual(got, d) {
			t.Errorf("title %q does not round-trip (err %v)\n markdown: %q\n got: %s", title, err, md, ptrJSON(got))
		}
	}
	// The fuzz input itself: read it, write it, read it again.
	src := "![](<>\"0\n\")"
	d, err := FromMarkdown(src, FromMarkdownOptions{})
	if err != nil {
		t.Fatalf("read %q: %v", src, err)
	}
	if again, err := FromMarkdown(ToMarkdown(d), FromMarkdownOptions{}); err != nil || !reflect.DeepEqual(again, d) {
		t.Errorf("fuzz input is not a fixed point (err %v)\n first: %s\n again: %s", err, ptrJSON(d), ptrJSON(again))
	}
}
