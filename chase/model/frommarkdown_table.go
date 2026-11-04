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
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Wide table rows (objective 1026, TRD 31, AODex D18).
//
// GFM ignores the cells of a body row that lie beyond the header's width, and
// goldmark's table parser obeys: parseRow stops at the header's column count, so
// the AST never holds an excess cell and a codec that only walks the AST loses it
// without a word, in strict mode too (TRD 26 found `| x | zqsentinel |` under a
// one-column header reading as `x`). The operator chose padding over a refusal:
// FromMarkdown reads such a table as one as wide as its widest row. The header
// gains empty cells, the alignments gain "" entries, a narrower row gains empty
// cells, and every cell of every row is kept.
//
// gfmTable is goldmark's table extension with that one addition. It runs goldmark's
// own paragraph transformer first, so everything else about a table (what is a
// table, how its header and delimiter row are read, alignment, the pipe escapes in
// code spans) is goldmark's, and then puts back the cells goldmark left out. The
// excess cells are real TableCell nodes with their own source lines, so goldmark's
// inline parser reads them like any other cell (bold, code, math, links, the cell
// reading of `\|`), strict refusals point at the right byte, and a block's span
// covers them.
type gfmTable struct{}

// Extend registers the transformers. The HTML renderer extension.Table also adds is
// left out: the codec reads the AST directly and never renders it.
func (gfmTable) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithParagraphTransformers(
			util.Prioritized(wideRowTransformer{stock: extension.NewTableParagraphTransformer()}, 200),
		),
		parser.WithASTTransformers(
			util.Prioritized(extension.NewTableASTTransformer(), 0),
			util.Prioritized(wideCellTransformer{}, 1),
		),
	)
}

// wideEscapedKey is the parse-context key under which wideRowTransformer leaves the
// excess cells that need wideCellTransformer.
var wideEscapedKey = parser.NewContextKey()

// wideEscaped is an excess cell with a `\|` after a backtick, where goldmark's
// code span parser needs the backslash taken out (see wideCellTransformer).
type wideEscaped struct {
	cell  *east.TableCell
	pipes []int // offsets in the source of the backslash of each such `\|`
}

// wideRowTransformer wraps goldmark's table paragraph transformer. It is stateless,
// like goldmark's: one engine parses many documents at once.
type wideRowTransformer struct {
	stock parser.ParagraphTransformer
}

func (t wideRowTransformer) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
	lines := node.Lines()
	n := lines.Len()
	if n < 2 { // a table needs a header and a delimiter row; goldmark returns at once
		t.stock.Transform(node, reader, pc)
		return
	}
	// goldmark turns the paragraph's lines into rows and then slices them off the
	// paragraph, so keep them: the body rows are the last of them.
	segs := make([]text.Segment, n)
	for i := range segs {
		segs[i] = lines.At(i)
	}
	parent, prev := node.Parent(), node.PreviousSibling()
	t.stock.Transform(node, reader, pc)

	// The table, if goldmark made one, is the paragraph's next sibling, or sits in
	// the paragraph's place when no line of text came before the header.
	var tbl ast.Node
	switch {
	case node.Parent() != nil:
		if node.Lines().Len() == n {
			return // not a table
		}
		tbl = node.NextSibling()
	case prev != nil:
		tbl = prev.NextSibling()
	case parent != nil:
		tbl = parent.FirstChild()
	}
	if table, ok := tbl.(*east.Table); ok {
		widenTable(table, segs, reader.Source(), pc)
	}
}

// widenTable pads table to its widest body row and adds the cells goldmark dropped.
// segs are the paragraph's lines before goldmark consumed them: the table's body
// rows came from the last len(rows) of them, one line each.
func widenTable(table *east.Table, segs []text.Segment, source []byte, pc parser.Context) {
	header, ok := table.FirstChild().(*east.TableHeader)
	if !ok {
		return
	}
	h := len(table.Alignments) // goldmark made the header exactly this wide
	base := len(segs) - (table.ChildCount() - 1)
	if base < 0 {
		return
	}

	// Read the whole line of every body row and find the width. Only the rows wider
	// than the header keep their cells.
	width := h
	var excess [][]tableCell
	k := 0
	for row := header.NextSibling(); row != nil; row, k = row.NextSibling(), k+1 {
		cells := splitTableRow(segs[base+k], source)
		if len(cells) <= h {
			continue
		}
		if excess == nil {
			excess = make([][]tableCell, table.ChildCount()-1)
		}
		excess[k] = cells
		width = max(width, len(cells))
	}
	if width == h {
		return
	}

	for i := h; i < width; i++ {
		header.AppendChild(header, east.NewTableCell())
		table.Alignments = append(table.Alignments, east.AlignNone)
	}
	k = 0
	for row := header.NextSibling(); row != nil; row, k = row.NextSibling(), k+1 {
		have := h
		if excess != nil && excess[k] != nil {
			for _, c := range excess[k][h:] {
				row.AppendChild(row, newTableCell(c, source, pc))
				have++
			}
		}
		for ; have < width; have++ {
			row.AppendChild(row, east.NewTableCell())
		}
	}
}

// newTableCell is the cell goldmark would have made for c: the same position, the
// same trimmed source line, no alignment (a column the header never had is
// unaligned). A cell with an escaped pipe after a backtick is noted for
// wideCellTransformer.
func newTableCell(c tableCell, source []byte, pc parser.Context) *east.TableCell {
	cell := east.NewTableCell()
	cell.Alignment = east.AlignNone
	cell.SetPos(c.pos)
	seg := text.NewSegment(c.start, c.stop)
	seg = seg.TrimLeftSpace(source)
	seg = seg.TrimRightSpace(source)
	cell.Lines().Append(seg)
	if len(c.pipes) > 0 {
		lst, _ := pc.ComputeIfAbsent(wideEscapedKey, func() any { return []*wideEscaped(nil) }).([]*wideEscaped)
		pc.Set(wideEscapedKey, append(lst, &wideEscaped{cell: cell, pipes: c.pipes}))
	}
	return cell
}

// tableCell is one cell of a row as goldmark splits it, before the inline parse.
type tableCell struct {
	start, stop int   // the cell's text in the source, before trimming its spaces
	pos         int   // the position goldmark gives the cell node
	pipes       []int // offsets of the backslash of each `\|` that follows a backtick in the cell
}

// splitTableRow splits one row line at its unescaped pipes. It is goldmark's
// parseRow (extension/table.go, v1.8.4) without the stop at the header's width and
// without building nodes, because that function is unexported and cannot be asked
// for the cells it leaves out. Where goldmark changes, TestCodecCellsRowSplitterMatchesGoldmark
// says so. Notably: an outer pipe is dropped even if it is escaped, a pipe
// preceded by a backslash never splits (however many backslashes come before that
// one), and a backtick anywhere earlier in the cell makes an escaped pipe count for
// the code span rule below.
func splitTableRow(npos text.Segment, source []byte) []tableCell {
	segment := npos.TrimLeftSpace(source)
	segment = segment.TrimRightSpace(source)
	line := segment.Value(source)
	pos, limit := 0, len(line)
	if len(line) > 0 && line[pos] == '|' {
		pos++
	}
	if len(line) > 0 && line[limit-1] == '|' {
		limit--
	}
	var cells []tableCell
	for pos < limit {
		c := tableCell{pos: npos.Start + pos - npos.Padding}
		hasBacktick := false
		closure := pos
		for ; closure < limit; closure++ {
			if line[closure] == '`' {
				hasBacktick = true
			}
			if line[closure] == '|' {
				if closure == 0 || line[closure-1] != '\\' {
					break
				} else if hasBacktick {
					c.pipes = append(c.pipes, segment.Start+closure-1)
				}
			}
		}
		c.start, c.stop = segment.Start+pos, segment.Start+closure
		cells = append(cells, c)
		pos = closure + 1
	}
	return cells
}

// wideCellTransformer takes the backslash out of an escaped pipe inside a code span
// of an excess cell, as goldmark's own AST transformer does for the cells it made
// (a code span reads its raw text, so `a\|b` in a cell is code `a|b`: GFM removes
// the backslash that escaped the pipe from the row split).
type wideCellTransformer struct{}

func (wideCellTransformer) Transform(_ *ast.Document, _ text.Reader, pc parser.Context) {
	lst, _ := pc.Get(wideEscapedKey).([]*wideEscaped)
	if lst == nil {
		return
	}
	pc.Set(wideEscapedKey, nil)
	for _, e := range lst {
		_ = ast.Walk(e.cell, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering || n.Kind() != ast.KindCodeSpan {
				return ast.WalkContinue, nil
			}
			for c := n.FirstChild(); c != nil; {
				next := c.NextSibling()
				if c.Kind() != ast.KindText {
					c = next
					continue
				}
				cur := c
				for _, pipe := range e.pipes {
					seg := cur.(*ast.Text).Segment
					if seg.Start <= pipe && pipe < seg.Stop {
						before := ast.NewRawTextSegment(seg.WithStop(pipe))
						after := ast.NewRawTextSegment(seg.WithStart(pipe + 1))
						n.InsertAfter(n, cur, before)
						n.InsertAfter(n, before, after)
						n.RemoveChild(n, cur)
						cur = after
					}
				}
				c = next
			}
			return ast.WalkContinue, nil
		})
	}
}
