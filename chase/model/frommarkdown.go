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
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"go.aocyber.ai/eden-press/chase/directive"
)

// FromMarkdownOptions configures FromMarkdown.
type FromMarkdownOptions struct {
	// Prev is the previously stored Document, for reconciling block ids across
	// a source-view edit. Reserved for TRD 02; ignored today.
	Prev *Document

	// Spans asks for byte spans on blocks and outline entries. Reserved for
	// TRD 02; ignored today.
	Spans bool

	// Lenient applies the documented, deterministic mapping for a construct the
	// model cannot carry exactly (see the Construct* constants) instead of
	// returning *UnrepresentableError.
	Lenient bool
}

// FromMarkdown reads Markdown into a v7 Document. It is the reading half of the
// lossless codec: for a Document d in canonical form (the form ToMarkdown and
// FromMarkdown produce), FromMarkdown(ToMarkdown(d)) equals d, ids and spans
// aside.
//
// It does not go through press.Render and the HTML sanitizer: it walks a
// goldmark AST (CommonMark, GFM tables, strikethrough and task lists, plus
// $...$ / $$...$$ math) directly, so nothing is dropped on the way in.
//
// Strict mode (the default) returns *UnrepresentableError for the first
// construct the model cannot carry exactly; it never drops or alters content
// silently. Lenient mode maps those constructs deterministically instead.
//
// Some differences carry no content and are normalized in both modes: bullet
// character, fence character, emphasis delimiter, list tightness and numbering
// (a list must still start at 1), setext vs ATX headings, reference vs inline
// links, and character-reference spelling. A soft line break (a plain newline
// inside a paragraph) reads as a space, as CommonMark defines it, so prose
// wrapped at 80 columns reflows; a hard break (backslash or two trailing spaces)
// reads as "\n".
//
// Reading rules worth knowing:
//   - a leading `---` / key: value lines / `---` block is front matter and
//     becomes Meta.Directives; any other `---` rule is a section break;
//   - `***`, `___` and `* * *` rules are divider blocks;
//   - a paragraph holding only one image is an image block, and one holding only
//     one $...$ is a non-display math block;
//   - a blockquote's leading paragraphs are the quote's Text/Runs (joined by a
//     blank line) and its other blocks are Children;
//   - a bullet list made only of GFM task items is one todo block per item, the
//     item's other blocks being its Children; a task item anywhere else is
//     ConstructListMixedTodo;
//   - a whole top-level `<!-- ... -->` comment is not a block: `eden:attrs` and
//     `eden:note` (the canonical spellings) and Marp's plain comments become the
//     section's Attrs and Notes (see sectionComment), and `eden:model` is the
//     trailer holding Properties, PropertyValues and Comments (see codec_ids.go);
//   - a `:::kind {json}` ... `:::` container is a toggle, synced, callout, file,
//     sheet or figure block (see frommarkdown_directives.go): the first paragraph
//     of the body is the block's Text/Runs and the rest are its Children;
//   - Outline is derived from the headings by rebuildOutline, the function
//     ApplyOps uses, so a document is the same whether it came from Markdown or
//     from ops.
//
// An empty source yields the Document NewDocument produces: one empty Section.
func FromMarkdown(md string, opts FromMarkdownOptions) (*Document, error) {
	if strings.Contains(md, "\r") {
		md = strings.ReplaceAll(strings.ReplaceAll(md, "\r\n", "\n"), "\r", "\n")
	}
	r := &mdReader{lenient: opts.Lenient}
	meta, bodyStart := r.frontMatter(md)
	if r.err != nil {
		return nil, r.err
	}
	r.src = []byte(md[bodyStart:])
	r.base = bodyStart
	root := mdEngine.Parser().Parse(text.NewReader(r.src))

	doc := &Document{
		SchemaVersion: SchemaVersion,
		Meta:          meta,
		Sections:      r.sections(root),
	}
	if r.err != nil {
		return nil, r.err
	}
	if tr := r.trailer; tr != nil {
		doc.Properties, doc.PropertyValues, doc.Comments = tr.Properties, tr.PropertyValues, tr.Comments
	}
	rebuildOutline(doc)
	return doc, nil
}

// mdEngine is the codec's goldmark: CommonMark plus GFM tables, strikethrough
// and task lists, and the math parsers below. No press batteries, no chase
// extender, no HTML renderer: the AST is read directly. Linkify is not
// registered, so a bare URL stays text (and needs no escaping on the way out).
var mdEngine = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList),
	goldmark.WithParserOptions(
		parser.WithBlockParsers(
			util.Prioritized(&mathBlockParser{}, 650),
			util.Prioritized(&directiveParser{}, 660),
		),
		parser.WithInlineParsers(util.Prioritized(&mathInlineParser{}, 450)),
	),
)

// mdReader holds the state of one FromMarkdown call.
type mdReader struct {
	src     []byte
	base    int // offset of src[0] in the (normalized) input
	lenient bool
	err     *UnrepresentableError

	trailer *modelTrailer // the eden:model comment, once seen
}

// refuse records the first strict-mode refusal. Callers go on to apply the
// lenient mapping unconditionally; in strict mode the result is discarded
// because err is set, so the mapping code is written once.
func (r *mdReader) refuse(pos int, construct, detail string) {
	if r.lenient || r.err != nil {
		return
	}
	if pos < 0 {
		pos = 0
	}
	r.err = &UnrepresentableError{Offset: r.base + pos, Construct: construct, Detail: detail}
}

// ---- front matter ----

var frontMatterKey = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_.-]*):(.*)$`)

// frontMatter reads a leading `---` ... `---` block of `key: value` lines and
// returns the directives and the offset at which the body starts. A leading
// rule that does not open such a block (no closing rule, or a blank first line)
// is a section break and is left for the body.
func (r *mdReader) frontMatter(src string) (Meta, int) {
	if !strings.HasPrefix(src, "---\n") {
		return Meta{}, 0
	}
	type fmLine struct {
		text string
		off  int
	}
	var inner []fmLine
	bodyStart := -1
	for pos := 4; pos < len(src); {
		line, next := src[pos:], len(src)
		if nl := strings.IndexByte(src[pos:], '\n'); nl >= 0 {
			line, next = src[pos:pos+nl], pos+nl+1
		}
		if strings.TrimRight(line, " \t") == "---" {
			bodyStart = next
			break
		}
		inner = append(inner, fmLine{line, pos})
		pos = next
	}
	if bodyStart < 0 || len(inner) == 0 || strings.TrimSpace(inner[0].text) == "" {
		return Meta{}, 0
	}
	directives := map[string]string{}
	for _, ln := range inner {
		if strings.TrimSpace(ln.text) == "" {
			continue
		}
		m := frontMatterKey.FindStringSubmatch(ln.text)
		if m == nil {
			r.refuse(ln.off, ConstructFrontMatter, "not a key: value line")
			continue
		}
		val, ok := r.frontMatterValue(ln.off, m[2])
		if !ok {
			continue
		}
		if _, dup := directives[m[1]]; dup {
			r.refuse(ln.off, ConstructFrontMatter, "duplicate key "+m[1])
		}
		directives[m[1]] = val
	}
	if len(directives) == 0 {
		return Meta{}, bodyStart
	}
	return Meta{Directives: directives}, bodyStart
}

// frontMatterValue reads one scalar: double-quoted (Go/YAML escapes),
// single-quoted (a doubled quote is one quote) or plain. ok is false when the value is
// malformed (the refusal is already recorded; lenient mode skips the key).
func (r *mdReader) frontMatterValue(off int, raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(v, `"`):
		s, err := strconv.Unquote(v)
		if err != nil {
			r.refuse(off, ConstructFrontMatter, "bad double-quoted value")
			return "", false
		}
		return s, true
	case len(v) >= 2 && strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'"):
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'"), true
	default:
		return v, true
	}
}

// ---- sections and blocks ----

func (r *mdReader) sections(root ast.Node) []Section {
	secs := []Section{{ID: 1}}
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		if tb, ok := c.(*ast.ThematicBreak); ok && r.isSectionRule(tb) {
			secs = append(secs, Section{ID: len(secs) + 1})
			continue
		}
		cur := &secs[len(secs)-1]
		if hb, ok := c.(*ast.HTMLBlock); ok && r.sectionComment(hb, cur) {
			continue
		}
		cur.Blocks = append(cur.Blocks, r.block(c)...)
	}
	return secs
}

// ---- comments: section attrs, notes and the trailer ----

// sectionComment reads a top-level HTML comment that is nothing but a comment
// and reports whether it did. In order:
//   - `eden:attrs {json}` is the section's Attrs, `eden:note "text"` is one of its
//     Notes, and `eden:model {json}` is the trailer (see codec_ids.go), which
//     belongs to no section;
//   - any other comment is Marp's: when it parses as YAML whose keys are all
//     known directives it is Attrs, otherwise (including a mix of known and
//     unknown keys, so free text is never taken for a directive) it is a Note.
//
// A canonical comment whose payload cannot be read is ConstructDirectiveJSON;
// lenient mode keeps its text as a Note. A comment with anything after it, or in
// a container, is raw HTML (htmlBlock).
func (r *mdReader) sectionComment(v *ast.HTMLBlock, cur *Section) bool {
	body, ok := commentBody(r.htmlSource(v))
	if !ok {
		return false
	}
	bad := func(err error) {
		r.refuse(v.Pos(), ConstructDirectiveJSON, err.Error())
		cur.Notes = append(cur.Notes, body)
	}
	switch {
	case edenWord(body, "eden:attrs"):
		var attrs map[string]string
		if err := decodeJSONStrict(body[len("eden:attrs"):], &attrs, false); err != nil {
			bad(fmt.Errorf("eden:attrs: %w", err))
			break
		}
		cur.addAttrs(attrs)
	case edenWord(body, "eden:note"):
		var note string
		if err := decodeJSONStrict(body[len("eden:note"):], &note, false); err != nil {
			bad(fmt.Errorf("eden:note: %w", err))
			break
		}
		cur.Notes = append(cur.Notes, note)
	case edenWord(body, "eden:model"):
		var tr modelTrailer
		err := decodeJSONStrict(body[len("eden:model"):], &tr, true)
		switch {
		case err == nil && tr.Version != trailerVersion:
			err = fmt.Errorf("unsupported version %d", tr.Version)
		case err == nil && r.trailer != nil:
			err = errors.New("a second eden:model comment")
		}
		if err != nil {
			bad(fmt.Errorf("eden:model: %w", err))
			break
		}
		r.trailer = &tr
	case isNote(body):
		cur.Notes = append(cur.Notes, body)
	default:
		attrs := map[string]string{}
		for _, kv := range directive.ParseComment(body) {
			attrs[kv.Key] = stringifyRawValue(kv.Val)
		}
		cur.addAttrs(attrs)
	}
	return true
}

// addAttrs merges attrs into the section's Attrs, a later value winning.
func (s *Section) addAttrs(attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}
	if s.Attrs == nil {
		s.Attrs = map[string]string{}
	}
	for k, v := range attrs {
		s.Attrs[k] = v
	}
}

// commentBody returns the trimmed text of src when src is exactly one HTML
// comment (nothing after its `-->`).
func commentBody(src string) (string, bool) {
	t := strings.TrimSpace(src)
	if len(t) < 7 || !strings.HasPrefix(t, "<!--") || !strings.HasSuffix(t, "-->") {
		return "", false
	}
	inner := t[4 : len(t)-3]
	if strings.Contains(inner, "-->") {
		return "", false
	}
	return strings.TrimSpace(inner), true
}

// edenWord reports whether body starts with the whole word w (`eden:note` but not
// `eden:notes`).
func edenWord(body, w string) bool {
	if !strings.HasPrefix(body, w) {
		return false
	}
	rest := body[len(w):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n'
}

// decodeJSONStrict reads s as exactly one JSON value (never null) into v. With
// noUnknown, an object key v has no field for is an error.
func decodeJSONStrict(s string, v any, noUnknown bool) error {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return errors.New("no JSON payload")
	}
	dec := json.NewDecoder(strings.NewReader(s))
	if noUnknown {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("text after the JSON payload")
	}
	return nil
}

// isSectionRule reports whether a top-level thematic break is spelled with
// dashes (`---`, `- - -`): those separate sections (Marp's slide break). Every
// other spelling is a divider block.
func (r *mdReader) isSectionRule(tb *ast.ThematicBreak) bool {
	p := tb.Pos()
	return p >= 0 && p < len(r.src) && r.src[p] == '-'
}

// block converts one block node into zero or more Blocks.
func (r *mdReader) block(n ast.Node) []Block {
	switch v := n.(type) {
	case *ast.Heading:
		// A heading is one line; a setext heading can span several, which read as
		// one line of text.
		txt, runs := richFields(singleLine(r.inline(v)))
		return []Block{{Kind: BlockHeading, Level: v.Level, Text: txt, Runs: runs}}
	case *ast.Paragraph, *ast.TextBlock:
		return r.paragraph(v)
	case *ast.List:
		return r.list(v)
	case *directiveNode:
		return r.directive(v)
	case *ast.FencedCodeBlock:
		return []Block{r.fencedCode(v)}
	case *ast.CodeBlock:
		return []Block{{Kind: BlockCode, Text: codeText(v.Lines(), r.src)}}
	case *ast.Blockquote:
		return []Block{r.quote(v)}
	case *ast.ThematicBreak:
		return []Block{{Kind: BlockDivider}}
	case *ast.HTMLBlock:
		return []Block{r.htmlBlock(v)}
	case *east.Table:
		return []Block{r.table(v)}
	case *ast.LinkReferenceDefinition:
		// A `[label]: url` definition is not content: the links that use it are
		// already resolved to inline links, and an unused one renders nothing.
		return nil
	case *mathBlockNode:
		if !v.closed {
			r.refuse(v.Pos(), ConstructMathUnclosed, "")
		}
		return []Block{{Kind: BlockMath, Display: true, Text: strings.Join(v.lines, "\n")}}
	default:
		r.refuse(n.Pos(), ConstructUnsupported, n.Kind().String())
		return nil
	}
}

func (r *mdReader) children(parent ast.Node) []Block {
	var out []Block
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, r.block(c)...)
	}
	return out
}

// trimLineBreaks drops line breaks from the start and end of runs: a break
// there is not content (Markdown ignores it), and a block's text never starts or
// ends with one.
func trimLineBreaks(runs []TextRun) []TextRun {
	out := append([]TextRun(nil), runs...)
	for len(out) > 0 {
		out[0].Text = strings.TrimLeft(out[0].Text, "\n")
		if out[0].Text != "" {
			break
		}
		out = out[1:]
	}
	for len(out) > 0 {
		last := len(out) - 1
		out[last].Text = strings.TrimRight(out[last].Text, "\n")
		if out[last].Text != "" {
			break
		}
		out = out[:last]
	}
	return canonRuns(out)
}

// singleLine turns every line break in runs into a space.
func singleLine(runs []TextRun) []TextRun {
	out := make([]TextRun, len(runs))
	for i, r := range runs {
		r.Text = strings.ReplaceAll(r.Text, "\n", " ")
		out[i] = r
	}
	return out
}

// richFields splits runs into the Text/Runs pair a Block carries: Runs only when
// the text is styled or linked, and Text always the flattened text.
func richFields(runs []TextRun) (string, []TextRun) {
	runs = trimLineBreaks(canonRuns(runs))
	if len(runs) == 0 {
		return "", nil
	}
	txt := FlattenRuns(runs)
	if len(runs) == 1 && len(runs[0].Marks) == 0 && runs[0].Link == "" {
		return txt, nil
	}
	return txt, runs
}

// paragraph turns a paragraph into a paragraph block, or into an image block /
// non-display math block when that is all it holds.
func (r *mdReader) paragraph(n ast.Node) []Block {
	var significant []ast.Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok && len(bytes.TrimSpace(t.Segment.Value(r.src))) == 0 {
			continue
		}
		significant = append(significant, c)
	}
	if len(significant) == 1 {
		switch v := significant[0].(type) {
		case *ast.Image:
			return []Block{r.imageBlock(v)}
		case *mathInlineNode:
			return []Block{{Kind: BlockMath, Display: v.display, Text: v.tex}}
		}
	}
	txt, runs := richFields(r.inline(n))
	if txt == "" {
		return nil
	}
	if len(runs) == 1 && runs[0].Link == "" && equalMarks(runs[0].Marks, []string{"math"}) {
		// Several adjacent $...$ merge into one run; a paragraph of nothing but
		// math is a math block, however many $...$ it was spelled with.
		return []Block{{Kind: BlockMath, Text: txt}}
	}
	return []Block{{Kind: BlockParagraph, Text: txt, Runs: runs}}
}

func (r *mdReader) imageBlock(img *ast.Image) Block {
	alt := FlattenRuns(singleLine(r.inline(img)))
	return Block{
		Kind:  BlockImage,
		Src:   decodeText(img.Destination),
		Text:  alt,
		Title: decodeText(img.Title),
	}
}

// ---- code, html, math ----

func (r *mdReader) fencedCode(v *ast.FencedCodeBlock) Block {
	bl := Block{Kind: BlockCode, Text: codeText(v.Lines(), r.src)}
	if v.Info != nil {
		info := strings.TrimSpace(decodeText(v.Info.Segment.Value(r.src)))
		if words := strings.Fields(info); len(words) > 0 {
			bl.Language = words[0]
			if len(words) > 1 {
				r.refuse(v.Pos(), ConstructCodeInfoString, info)
			}
		}
	}
	return bl
}

// codeText is the content between the fences, exactly: the lines joined by "\n"
// (every line segment carries its own newline, so one is trimmed off the end).
func codeText(lines *text.Segments, src []byte) string {
	var b []byte
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b = append(b, seg.Value(src)...)
	}
	return strings.TrimSuffix(string(b), "\n")
}

// htmlSource is the source text of an HTML block: its lines and, for a block
// that ended on a closing line, that line.
func (r *mdReader) htmlSource(v *ast.HTMLBlock) string {
	var b []byte
	for i := 0; i < v.Lines().Len(); i++ {
		seg := v.Lines().At(i)
		b = append(b, seg.Value(r.src)...)
	}
	if v.HasClosure() {
		b = append(b, v.ClosureLine.Value(r.src)...)
	}
	return string(b)
}

func (r *mdReader) htmlBlock(v *ast.HTMLBlock) Block {
	r.refuse(v.Pos(), ConstructRawHTML, "")
	return Block{Kind: BlockParagraph, Text: strings.TrimRight(r.htmlSource(v), "\n")}
}

// mathBlockNode is a $$ ... $$ display math block. Its TeX is kept verbatim as
// lines; nothing inside is parsed as Markdown.
type mathBlockNode struct {
	ast.BaseBlock
	lines  []string
	closed bool
}

var kindMathBlock = ast.NewNodeKind("EdenMathBlock")

func (n *mathBlockNode) Kind() ast.NodeKind { return kindMathBlock }

func (n *mathBlockNode) IsRaw() bool { return true }

func (n *mathBlockNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"tex": strings.Join(n.lines, "\n")}, nil)
}

type mathBlockParser struct{}

func (*mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (*mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	width, pos := util.IndentWidth(line, reader.LineOffset())
	if width > 3 || !bytes.HasPrefix(line[pos:], []byte("$$")) {
		return nil, parser.NoChildren
	}
	rest := strings.TrimRight(string(line[pos+2:]), "\n")
	node := &mathBlockNode{}
	if idx := indexDoubleDollar([]byte(rest)); idx >= 0 {
		if strings.TrimSpace(rest[idx+2:]) != "" {
			return nil, parser.NoChildren // $$x$$ followed by text is inline math
		}
		node.lines = append(node.lines, rest[:idx])
		node.closed = true
		reader.AdvanceToEOL()
		return node, parser.NoChildren
	}
	if strings.TrimSpace(rest) != "" {
		node.lines = append(node.lines, rest)
	}
	reader.AdvanceToEOL()
	return node, parser.Continue | parser.NoChildren
}

func (*mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n := node.(*mathBlockNode)
	if n.closed {
		return parser.Close
	}
	line, _ := reader.PeekLine()
	content := strings.TrimRight(string(line), "\n")
	if idx := indexDoubleDollar([]byte(content)); idx >= 0 {
		if before := content[:idx]; strings.TrimSpace(before) != "" {
			n.lines = append(n.lines, before)
		}
		n.closed = true
		reader.AdvanceToEOL()
		return parser.Close
	}
	n.lines = append(n.lines, content)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (*mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (*mathBlockParser) CanInterruptParagraph() bool { return true }

func (*mathBlockParser) CanAcceptIndentedLine() bool { return false }

// mathInlineNode is inline math. display is set for `$$x$$` that shares a
// paragraph with other content, which the model cannot hold as inline math.
type mathInlineNode struct {
	ast.BaseInline
	tex     string
	display bool
}

var kindMathInline = ast.NewNodeKind("EdenMathInline")

func (n *mathInlineNode) Kind() ast.NodeKind { return kindMathInline }

func (n *mathInlineNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"tex": n.tex}, nil)
}

// mathInlineParser reads `$tex$` by the usual rule (it is what keeps "$5 and
// $6" as text): the opening $ is not followed by whitespace, the closing $ is
// not preceded by whitespace and not followed by a digit, a backslash escapes
// the next byte, and the TeX never spans a line.
type mathInlineParser struct{}

func (*mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (*mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 2 || line[0] != '$' {
		return nil
	}
	if line[1] == '$' {
		if idx := indexDoubleDollar(line[2:]); idx > 0 {
			tex := line[2 : 2+idx]
			after := 2 + idx + 2
			if isMathSpace(tex[0]) || isMathSpace(tex[len(tex)-1]) || hasBareDollar(tex) ||
				(after < len(line) && line[after] >= '0' && line[after] <= '9') {
				return nil // as for $...$, padding, a stray $ or a digit right after means this is not math
			}
			node := &mathInlineNode{tex: string(tex), display: true}
			block.Advance(2 + idx + 2)
			return node
		}
		return nil
	}
	if isMathSpace(line[1]) {
		return nil
	}
	for j := 1; j < len(line); j++ {
		switch c := line[j]; {
		case c == '\\':
			j++
		case c == '$':
			if isMathSpace(line[j-1]) || (j+1 < len(line) && line[j+1] >= '0' && line[j+1] <= '9') {
				continue
			}
			node := &mathInlineNode{tex: string(line[1:j])}
			block.Advance(j + 1)
			return node
		}
	}
	return nil
}

// indexDoubleDollar is the index of the first "$$" in s that is not escaped by a
// backslash, or -1.
func indexDoubleDollar(s []byte) int {
	for i := 0; i+1 < len(s); i++ {
		switch {
		case s[i] == '\\':
			i++
		case s[i] == '$' && s[i+1] == '$':
			return i
		}
	}
	return -1
}

// hasBareDollar reports whether tex contains a $ that is not backslash-escaped.
func hasBareDollar(tex []byte) bool {
	for i := 0; i < len(tex); i++ {
		switch tex[i] {
		case '\\':
			i++
		case '$':
			return true
		}
	}
	return false
}

func isMathSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

// ---- quotes, lists, tables ----

func (r *mdReader) quote(q *ast.Blockquote) Block {
	kids := r.children(q)
	i := 0
	var runs []TextRun
	for i < len(kids) && kids[i].Kind == BlockParagraph {
		if i > 0 {
			runs = append(runs, TextRun{Text: "\n\n"})
		}
		runs = append(runs, runsOfBlock(kids[i])...)
		i++
	}
	bl := Block{Kind: BlockQuote}
	bl.Text, bl.Runs = richFields(runs)
	if rest := kids[i:]; len(rest) > 0 {
		bl.Children = rest
	}
	return bl
}

// list reads a list. A bullet list made only of GFM task items is one todo block
// per item (the item's other blocks are the todo's Children); any other list is
// one list block, and a task item in it is ConstructListMixedTodo.
func (r *mdReader) list(l *ast.List) []Block {
	if !l.IsOrdered() && allTaskItems(l) {
		return r.todos(l)
	}
	bl := Block{Kind: BlockList, Ordered: l.IsOrdered()}
	r.listItems(l, 0, &bl)
	return []Block{bl}
}

// taskBox returns the checkbox that makes a list item a GFM task item, or nil.
// goldmark puts it first in the item's first text block.
func taskBox(item ast.Node) *east.TaskCheckBox {
	switch first := item.FirstChild().(type) {
	case *ast.TextBlock, *ast.Paragraph:
		box, _ := first.FirstChild().(*east.TaskCheckBox)
		return box
	}
	return nil
}

func allTaskItems(l *ast.List) bool {
	n := 0
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		if taskBox(li) == nil {
			return false
		}
		n++
	}
	return n > 0
}

// todos reads a task list as todo blocks.
func (r *mdReader) todos(l *ast.List) []Block {
	var out []Block
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		first := li.FirstChild()
		bl := Block{Kind: BlockTodo, Checked: taskBox(li).IsChecked}
		w := &inlineWalker{r: r, taskBox: true}
		w.children(first)
		bl.Text, bl.Runs = richFields(w.runs)
		for c := first.NextSibling(); c != nil; c = c.NextSibling() {
			bl.Children = append(bl.Children, r.block(c)...)
		}
		out = append(out, bl)
	}
	return out
}

// listItems flattens l (and the lists nested in its items) into bl.Items, each
// nested list one level deeper than the item that owns it.
func (r *mdReader) listItems(l *ast.List, level int, bl *Block) {
	if l.IsOrdered() && l.Start != 1 {
		r.refuse(l.Pos(), ConstructListStart, "starts at "+strconv.Itoa(l.Start))
	}
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		item, ok := li.(*ast.ListItem)
		if !ok {
			continue
		}
		var runs []TextRun
		var nested []*ast.List
		haveText := false
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.TextBlock, *ast.Paragraph:
				if !haveText {
					runs, haveText = r.inline(v), true
					continue
				}
				r.refuse(c.Pos(), ConstructListItemBlocks, "second paragraph")
				runs = append(runs, TextRun{Text: "\n"})
				runs = append(runs, r.inline(v)...)
			case *ast.List:
				nested = append(nested, v)
			default:
				r.refuse(c.Pos(), ConstructListItemBlocks, c.Kind().String())
				if t := r.plainText(c); t != "" {
					runs = append(runs, TextRun{Text: "\n" + t})
				}
			}
		}
		it := ListItem{Level: level}
		it.Text, it.Runs = richFields(runs)
		bl.Items = append(bl.Items, it)
		for _, nl := range nested {
			if nl.IsOrdered() != bl.Ordered {
				r.refuse(nl.Pos(), ConstructListMixedType, "")
			}
			r.listItems(nl, level+1, bl)
		}
	}
}

// plainText is the text of any node, for lenient flattening.
func (r *mdReader) plainText(n ast.Node) string {
	switch v := n.(type) {
	case *ast.FencedCodeBlock:
		return codeText(v.Lines(), r.src)
	case *ast.CodeBlock:
		return codeText(v.Lines(), r.src)
	case *ast.HTMLBlock:
		return r.htmlBlock(v).Text
	case *ast.Heading, *ast.Paragraph, *ast.TextBlock:
		return FlattenRuns(r.inline(n))
	}
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t := r.plainText(c); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func (r *mdReader) table(t *east.Table) Block {
	bl := Block{Kind: BlockTable, Aligns: alignNames(t.Alignments)}
	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *east.TableHeader:
			bl.Headers = r.cells(row)
		case *east.TableRow:
			bl.Rows = append(bl.Rows, r.cells(row))
		}
	}
	return bl
}

func (r *mdReader) cells(row ast.Node) []string {
	var cells []string
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*east.TableCell); ok {
			cells = append(cells, r.cellText(cell))
		}
	}
	return cells
}

// cellText is a cell's plain text. Anything but text in a cell is formatting
// the plain-text Rows cannot hold.
func (r *mdReader) cellText(cell *east.TableCell) string {
	var b strings.Builder
	for c := cell.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.WriteString(decodeText(t.Segment.Value(r.src)))
			continue
		}
		r.refuse(c.Pos(), ConstructTableCellFormatting, c.Kind().String())
		w := &inlineWalker{r: r}
		w.walk(c)
		b.WriteString(FlattenRuns(w.runs))
	}
	return b.String()
}

// ---- inline ----

// inline reads the inline children of parent as canonical runs.
func (r *mdReader) inline(parent ast.Node) []TextRun {
	w := &inlineWalker{r: r}
	w.children(parent)
	return canonRuns(w.runs)
}

// inlineWalker flattens an inline subtree into runs, tracking the marks and the
// link in force.
type inlineWalker struct {
	r     *mdReader
	runs  []TextRun
	marks []string
	link  string

	// taskBox is set while reading the text of a todo block, whose checkbox is
	// the block's Checked and is not text.
	taskBox bool
}

func (w *inlineWalker) emit(s string) {
	if s == "" {
		return
	}
	w.runs = append(w.runs, TextRun{Text: s, Marks: canonMarks(w.marks), Link: w.link})
}

func (w *inlineWalker) children(n ast.Node) {
	saved := len(w.marks)
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		w.walk(c)
	}
	if len(w.marks) > saved {
		w.r.refuse(n.Pos(), ConstructRawHTML, "unclosed tag")
		w.marks = w.marks[:saved]
	}
}

// markTags are the only inline HTML tags FromMarkdown reads: the three ToMarkdown
// writes for bold, italic and strike when delimiters would not read back.
var markTags = map[string]struct {
	mark string
	open bool
}{
	"<strong>": {"bold", true}, "</strong>": {"bold", false},
	"<em>": {"italic", true}, "</em>": {"italic", false},
	"<del>": {"strike", true}, "</del>": {"strike", false},
}

// tag applies an opening or closing mark tag; ok is false when a closing tag has
// no opener.
func (w *inlineWalker) tag(mark string, open bool) bool {
	if open {
		w.marks = append(w.marks, mark)
		return true
	}
	for i := len(w.marks) - 1; i >= 0; i-- {
		if w.marks[i] == mark {
			w.marks = append(w.marks[:i], w.marks[i+1:]...)
			return true
		}
	}
	return false
}

func (w *inlineWalker) withMark(mark string, f func()) {
	w.marks = append(w.marks, mark)
	f()
	w.marks = w.marks[:len(w.marks)-1]
}

func (w *inlineWalker) walk(n ast.Node) {
	src := w.r.src
	switch v := n.(type) {
	case *ast.Text:
		s := decodeText(v.Segment.Value(src))
		switch {
		case v.HardLineBreak():
			s += "\n"
		case v.SoftLineBreak():
			s += " " // CommonMark: a soft break is whitespace, so wrapped prose reflows
		}
		w.emit(s)
	case *ast.String:
		w.emit(string(v.Value))
	case *ast.CodeSpan:
		var b strings.Builder
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				b.Write(t.Segment.Value(src))
			}
		}
		w.withMark("code", func() { w.emit(strings.ReplaceAll(b.String(), "\n", " ")) })
	case *ast.Emphasis:
		mark := "italic"
		if v.Level >= 2 {
			mark = "bold"
		}
		w.withMark(mark, func() { w.children(v) })
	case *east.Strikethrough:
		w.withMark("strike", func() { w.children(v) })
	case *ast.Link:
		if len(v.Title) > 0 {
			w.r.refuse(v.Pos(), ConstructLinkTitle, "")
		}
		if !v.HasChildren() {
			w.r.refuse(v.Pos(), ConstructEmptyLink, "no link text")
			return
		}
		prev := w.link
		if dest := decodeText(v.Destination); dest != "" {
			w.link = dest
		} else {
			w.r.refuse(v.Pos(), ConstructEmptyLink, "no destination")
		}
		w.children(v)
		w.link = prev
	case *ast.AutoLink:
		prev := w.link
		w.link = string(v.URL(src))
		if v.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(w.link, "mailto:") {
			w.link = "mailto:" + w.link
		}
		w.emit(string(v.Label(src)))
		w.link = prev
	case *ast.Image:
		w.r.refuse(v.Pos(), ConstructInlineImage, "")
		w.children(v)
	case *ast.RawHTML:
		var lit strings.Builder
		for i := 0; i < v.Segments.Len(); i++ {
			seg := v.Segments.At(i)
			lit.Write(seg.Value(src))
		}
		if t, ok := markTags[lit.String()]; ok && w.tag(t.mark, t.open) {
			return
		}
		w.r.refuse(v.Pos(), ConstructRawHTML, "")
		w.emit(lit.String())
	case *mathInlineNode:
		if v.display {
			w.r.refuse(v.Pos(), ConstructInlineDisplayMath, "")
		}
		w.withMark("math", func() { w.emit(v.tex) })
	case *east.TaskCheckBox:
		if w.taskBox {
			return
		}
		w.r.refuse(v.Pos(), ConstructListMixedTodo, "")
		if v.IsChecked {
			w.emit("[x] ")
		} else {
			w.emit("[ ] ")
		}
	default:
		w.r.refuse(n.Pos(), ConstructUnsupported, n.Kind().String())
		w.children(n)
	}
}

// decodeText turns the raw source of a text segment into the text it means: a
// backslash before ASCII punctuation is dropped, and character references
// (&amp; &#65; &#x41;) are resolved. It makes one pass, so `\&amp;` stays the
// literal text "&amp;".
func decodeText(raw []byte) string {
	if bytes.IndexByte(raw, '\\') < 0 && bytes.IndexByte(raw, '&') < 0 {
		return string(raw)
	}
	var b strings.Builder
	for i := 0; i < len(raw); {
		c := raw[i]
		if c == '\\' && i+1 < len(raw) && util.IsPunct(raw[i+1]) {
			b.WriteByte(raw[i+1])
			i += 2
			continue
		}
		if c == '&' {
			if s, n, ok := decodeEntity(raw[i:]); ok {
				b.WriteString(s)
				i += n
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// decodeEntity resolves the character reference at the start of s, returning
// the text, the number of bytes it spans and whether there was one.
func decodeEntity(s []byte) (string, int, bool) {
	if len(s) < 3 || s[0] != '&' {
		return "", 0, false
	}
	if s[1] == '#' {
		i, base := 2, 10
		if i < len(s) && (s[i] == 'x' || s[i] == 'X') {
			i, base = i+1, 16
		}
		start := i
		for i < len(s) && isEntityDigit(s[i], base == 16) {
			i++
		}
		maxDigits := 7
		if base == 16 {
			maxDigits = 6
		}
		if i == start || i-start > maxDigits || i >= len(s) || s[i] != ';' {
			return "", 0, false
		}
		v, err := strconv.ParseUint(string(s[start:i]), base, 32)
		if err != nil {
			return "", 0, false
		}
		ru := rune(v)
		if ru == 0 || ru > unicode.MaxRune || (ru >= 0xD800 && ru <= 0xDFFF) {
			ru = utf8.RuneError
		}
		return string(ru), i + 1, true
	}
	i := 1
	for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' || s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i == 1 || i >= len(s) || s[i] != ';' {
		return "", 0, false
	}
	if e, ok := util.LookUpHTML5EntityByName(string(s[1:i])); ok {
		return string(e.Characters), i + 1, true
	}
	return "", 0, false
}
