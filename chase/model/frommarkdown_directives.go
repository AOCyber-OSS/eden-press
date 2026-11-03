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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Container directives are the Markdown spelling of the six v7-only block kinds
// that have no GFM form (toggle, synced, callout, file, sheet, figure):
//
//	:::kind {"json":"header"}
//	body
//	:::
//
// The fence is three or more colons and the closing fence must be at least as
// long as the opening one, so a body holding `:::` is wrapped in a longer fence
// exactly as code fences grow (ToMarkdown computes the length). The header is one
// JSON object on the opening line, holding the block's scalar fields; it may be
// omitted when every field is empty. The body is the block's text (the first
// paragraph) followed by its children, parsed as Markdown, except for a figure,
// whose body is its inline SVG, verbatim. (A todo is not a directive: it is a
// GFM task item.)
//
// goldmark has no directive extension, so this is a container block parser: the
// body's blocks are parsed by goldmark itself (inside quotes, lists and other
// directives alike) and every offset, including spans, stays an offset into the
// one source buffer.

// dirHeader is the JSON header of a directive. Field order is the order the
// writer emits, so output is byte-deterministic.
type dirHeader struct {
	Icon       string      `json:"icon,omitempty"`
	Tone       string      `json:"tone,omitempty"`
	Collapsed  bool        `json:"collapsed,omitempty"`
	Name       string      `json:"name,omitempty"`
	Mime       string      `json:"mime,omitempty"`
	Size       int64       `json:"size,omitempty"`
	Src        string      `json:"src,omitempty"`
	Title      string      `json:"title,omitempty"`
	Alt        string      `json:"alt,omitempty"` // a figure's Text; every other kind's text is the body
	SheetEmbed *SheetEmbed `json:"sheetEmbed,omitempty"`
	SyncRef    *SyncRef    `json:"syncRef,omitempty"`

	// NoText says the body has no text: its first paragraph is a child. Without
	// it the first paragraph of the body is the block's Text/Runs.
	NoText bool `json:"noText,omitempty"`
}

// directiveKinds maps a directive name to the block kind it stands for.
var directiveKinds = map[string]BlockKind{
	"toggle":  BlockToggle,
	"synced":  BlockSynced,
	"callout": BlockCallout,
	"file":    BlockFile,
	"sheet":   BlockSheet,
	"figure":  BlockFigure,
}

// decodeDirHeader reads s as exactly one JSON object holding only known fields.
func decodeDirHeader(s string, h *dirHeader) error {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(h); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("text after the JSON header")
	}
	return nil
}

// ---- the goldmark node and parser ----

type directiveNode struct {
	ast.BaseBlock
	name   string // as written, e.g. "toggle"
	header string // the text after the name, trimmed; "" when there is none
	fence  int    // colons in the opening fence
	raw    bool   // the body is verbatim lines, not Markdown (figure)
	closed bool   // a closing fence was seen
	stop   int    // offset one past the last byte of the construct seen so far
}

var kindDirective = ast.NewNodeKind("EdenDirective")

func (n *directiveNode) Kind() ast.NodeKind { return kindDirective }

func (n *directiveNode) IsRaw() bool { return n.raw }

func (n *directiveNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"name": n.name, "header": n.header}, nil)
}

type directiveParser struct{}

func (*directiveParser) Trigger() []byte { return []byte{':'} }

func (*directiveParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) {
		return nil, parser.NoChildren
	}
	fence := colonRun(line[pos:])
	if fence < 3 {
		return nil, parser.NoChildren
	}
	rest := strings.TrimSpace(string(line[pos+fence:]))
	nameLen := 0
	for nameLen < len(rest) && isDirectiveNameByte(rest[nameLen], nameLen == 0) {
		nameLen++
	}
	if nameLen == 0 {
		return nil, parser.NoChildren
	}
	n := &directiveNode{
		name:   rest[:nameLen],
		header: strings.TrimSpace(rest[nameLen:]),
		fence:  fence,
	}
	n.raw = n.name == "figure"
	n.stop = segment.Start + pos + len(bytes.TrimRight(line[pos:], " \t\r\n"))
	reader.AdvanceToEOL()
	if n.raw {
		return n, parser.NoChildren
	}
	return n, parser.HasChildren
}

func (*directiveParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n := node.(*directiveNode)
	line, segment := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w < 4 && pos < len(line) {
		if run := colonRun(line[pos:]); run >= n.fence && util.IsBlank(line[pos+run:]) {
			n.closed = true
			n.stop = segment.Start + pos + run
			reader.AdvanceToEOL()
			return parser.Close
		}
	}
	blank := util.IsBlank(line)
	if !blank {
		n.stop = segment.Start + len(bytes.TrimRight(line, " \t\r\n"))
	}
	if n.raw {
		seg := segment
		seg.ForceNewline = true // EOF counts as a newline
		n.Lines().Append(seg)
		reader.AdvanceToEOL()
		return parser.Continue | parser.NoChildren
	}
	// A blank line is left as it is, whitespace and all: a code block inside the
	// directive keeps a whitespace-only line of its own.
	return parser.Continue | parser.HasChildren
}

func (*directiveParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (*directiveParser) CanInterruptParagraph() bool { return true }

func (*directiveParser) CanAcceptIndentedLine() bool { return false }

// colonRun is the number of ':' at the start of b.
func colonRun(b []byte) int {
	n := 0
	for n < len(b) && b[n] == ':' {
		n++
	}
	return n
}

func isDirectiveNameByte(c byte, first bool) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case !first && (c >= '0' && c <= '9' || c == '-' || c == '_'):
		return true
	}
	return false
}

// ---- reading a directive into a Block ----

// directive turns a directive node into a block, or (unknown name, bad header)
// into its literal source text as paragraphs. A directive that was never closed
// is refused in strict mode and read as if it closed at the end of its container.
func (r *mdReader) directive(n *directiveNode) []Block {
	kind, known := directiveKinds[n.name]
	if !known {
		r.refuse(n.Pos(), ConstructUnknownDirective, n.name)
		return r.literalDirective(n)
	}
	var h dirHeader
	if n.header != "" {
		if err := decodeDirHeader(n.header, &h); err != nil {
			r.refuse(n.Pos(), ConstructDirectiveJSON, err.Error())
			return r.literalDirective(n)
		}
	}
	if !n.closed {
		r.refuse(n.Pos(), ConstructDirectiveUnclosed, n.name)
	}
	bl := Block{
		Kind: kind, Icon: h.Icon, Tone: h.Tone, Collapsed: h.Collapsed,
		Name: h.Name, Mime: h.Mime, Size: h.Size, Src: h.Src, Title: h.Title,
		SheetEmbed: h.SheetEmbed, SyncRef: h.SyncRef,
	}
	if n.raw {
		bl.Text = h.Alt
		bl.SVG = codeText(n.Lines(), r.src)
		return []Block{bl}
	}
	c := n.FirstChild()
	if !h.NoText {
		switch c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			bl.Text, bl.Runs = richFields(r.inline(c))
			c = c.NextSibling()
		}
	}
	for ; c != nil; c = c.NextSibling() {
		bl.Children = append(bl.Children, r.block(c)...)
	}
	return []Block{bl}
}

// literalDirective is lenient mode's reading of a directive it cannot use: the
// source lines from the opening fence to the end of the construct, one
// paragraph per run of non-blank lines.
func (r *mdReader) literalDirective(n *directiveNode) []Block {
	start, stop := n.Pos(), n.stop
	if stop > len(r.src) {
		stop = len(r.src)
	}
	if start < 0 || start > stop {
		return nil
	}
	return literalParagraphs(string(r.src[start:stop]))
}

// literalParagraphs splits s on blank lines into paragraph blocks whose Text is
// the lines as written.
func literalParagraphs(s string) []Block {
	var out []Block
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, Block{Kind: BlockParagraph, Text: strings.Join(cur, "\n")})
			cur = nil
		}
	}
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return out
}
