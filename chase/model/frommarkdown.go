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
	"reflect"
	"regexp"
	"sort"
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

	"go.aocyber.ai/eden-press/chase/markdown"
)

// FromMarkdownOptions configures FromMarkdown.
type FromMarkdownOptions struct {
	// Prev is the previously stored Document, for keeping block and list-item ids
	// across a source-view edit: blocks of the new Document that match blocks of
	// Prev (unchanged, moved or edited in place; see codec_ids.go) take their ids
	// and everything else gets the next free id. Without Prev the trailer in the
	// Markdown, if it carries ids, plays the same part; with neither, no ids are
	// assigned. Prev is only read for ids: properties, property values and
	// comments come from the Markdown's trailer, so Markdown without a trailer
	// has none. Prev is not modified.
	Prev *Document

	// Spans asks for byte spans: every block, section and outline entry gets a
	// Span into the input (a half-open byte range, see Span), and without Spans
	// no Span is set. A span covers what goldmark parsed as the node's lines: a
	// paragraph or heading's text (an ATX heading without its "## " and closing
	// hashes, so for a plain heading md[span] is exactly the outline text), a
	// code block's content between the fences, a list, table or quote from its
	// first positioned descendant to its last, and a directive, display-math
	// block or divider as written, fences included. A section's span runs from
	// its first to its last node, comments included and the "---" rules not.
	// Offsets are into the string as given, so CRLF input is counted in its
	// original bytes.
	Spans bool

	// Lenient applies the documented, deterministic mapping for a construct the
	// model cannot carry exactly (see the Construct* constants) instead of
	// returning *UnrepresentableError.
	Lenient bool
}

// FromMarkdown reads Markdown into a v7 Document. It is the reading half of the
// lossless codec: for a Document d in canonical form (the form ToMarkdown and
// FromMarkdown produce), FromMarkdown(ToMarkdown(d)) equals d. Block, list-item
// and comment ids come back too when d has them, from the trailer (see
// codec_ids.go) or from FromMarkdownOptions.Prev; spans are only set when asked
// for, so they are never part of the comparison.
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
	md, crAt := normalizeNewlines(md, opts.Spans)
	r := &mdReader{lenient: opts.Lenient, spans: opts.Spans, crAt: crAt}
	meta, bodyStart := r.frontMatter(md)
	if r.err != nil {
		return nil, r.err
	}
	r.src = []byte(md[bodyStart:])
	r.base = bodyStart
	pc := parser.NewContext()
	root := mdEngine.Parser().Parse(text.NewReader(r.src), parser.WithContext(pc))
	if at, ok := pc.Get(markdown.NestingLimitKey).(int); ok {
		// The parse declined a container past the limit; its marker and the
		// rest of the line were read as text (lenient's mapping).
		r.refuse(at, ConstructNestingDepth, fmt.Sprintf("more than %d nested quotes or list items", MaxNestingDepth))
	}

	doc := &Document{
		SchemaVersion: SchemaVersion,
		Meta:          meta,
		Sections:      r.sections(root),
	}
	if r.err == nil {
		r.limitNesting(doc)
	}
	if r.err != nil {
		return nil, r.err
	}
	if tr := r.trailer; tr != nil {
		doc.Properties, doc.PropertyValues, doc.Comments = tr.Properties, tr.PropertyValues, tr.Comments
	}
	rebuildOutline(doc)
	if opts.Spans {
		r.outlineSpans(doc)
	}
	r.reconcileIDs(doc, opts.Prev)
	return doc, nil
}

// limitNesting enforces MaxNestingDepth on the sections just read
// (ConstructNestingDepth): strict mode refuses the first block or list item
// past the limit, lenient mode flattens what is below it (flattenNesting).
//
// The check runs on the finished tree rather than during the walk, because a
// quote's leading paragraphs become its own text, not children: the depth of
// a block is only known once its container has been read.
func (r *mdReader) limitNesting(doc *Document) {
	for si := range doc.Sections {
		p, _ := firstTooDeep(doc.Sections[si].Blocks, 1)
		if p == nil {
			continue
		}
		if r.lenient {
			for sj := si; sj < len(doc.Sections); sj++ {
				flattenNesting(doc.Sections[sj].Blocks, 1)
			}
			return
		}
		r.refuse(r.nestingOffset(si, p), ConstructNestingDepth, fmt.Sprintf("limit %d levels", MaxNestingDepth))
		return
	}
}

// nestingOffset is the byte offset, relative to r.src as refuse expects, at
// which the block p names starts in section si (for a list item, its list).
// Blocks only carry spans when they were asked for, so on this error path
// alone the body is read again with spans on: the converters are
// deterministic and map strict and lenient constructs alike, so the second
// tree has the same shape and p finds the same block.
func (r *mdReader) nestingOffset(si int, p *nestPath) int {
	again := &mdReader{src: r.src, lenient: true, spans: true}
	secs := again.sections(mdEngine.Parser().Parse(text.NewReader(again.src)))
	if si >= len(secs) {
		return 0
	}
	off := 0
	bs := secs[si].Blocks
	for _, step := range p.steps {
		if step >= len(bs) {
			break
		}
		if sp := bs[step].Span; sp != nil {
			off = sp.Start // the deepest block on the path that has a span
		}
		bs = bs[step].Children
	}
	return off
}

// normalizeNewlines turns "\r\n" and "\r" into "\n". When track is set it also
// returns, in order, the offsets in the result of each "\n" that replaced a
// "\r\n", which is what orig needs to count a span in the original bytes.
func normalizeNewlines(md string, track bool) (string, []int) {
	if !strings.Contains(md, "\r") {
		return md, nil
	}
	var b strings.Builder
	var crAt []int
	for i := 0; i < len(md); i++ {
		if md[i] != '\r' {
			b.WriteByte(md[i])
			continue
		}
		if i+1 < len(md) && md[i+1] == '\n' {
			if track {
				crAt = append(crAt, b.Len())
			}
			continue
		}
		b.WriteByte('\n')
	}
	return b.String(), crAt
}

// mdEngine is the codec's goldmark: CommonMark plus GFM tables, strikethrough
// and task lists, and the math parsers below. No press batteries, no chase
// extender, no HTML renderer: the AST is read directly. Linkify is not
// registered, so a bare URL stays text (and needs no escaping on the way out).
var mdEngine = goldmark.New(
	goldmark.WithParser(newCodecParser()),
	goldmark.WithExtensions(gfmTable{}, extension.Strikethrough, extension.TaskList),
	goldmark.WithParserOptions(
		parser.WithBlockParsers(
			util.Prioritized(&mathBlockParser{}, 650),
			util.Prioritized(&directiveParser{}, 660),
		),
		parser.WithInlineParsers(util.Prioritized(&mathInlineParser{}, 450)),
	),
)

// newCodecParser is goldmark's default parser with its fenced-code parser guarded.
// goldmark v1.8.4 marks a line as having no block start (BlockOffset -1) when its
// indent width in columns reaches its length in bytes, which a tab does on a short
// line ("> \t`"), and its fenced-code parser then indexes the line with -1 and
// panics. Later goldmark releases return early there; this does the same without
// a change to go.mod.
func newCodecParser() parser.Parser {
	var blocks []util.PrioritizedValue
	// The container parsers are bounded at MaxNestingDepth (see
	// markdown.NestingLimitedBlockParsers): goldmark's parse is otherwise
	// quadratic in the containers one line opens.
	for _, pv := range markdown.NestingLimitedBlockParsers() {
		if bp, ok := pv.Value.(parser.BlockParser); ok && bytes.IndexByte(bp.Trigger(), '`') >= 0 {
			pv.Value = &guardedFence{bp}
		}
		blocks = append(blocks, pv)
	}
	return parser.NewParser(
		parser.WithBlockParsers(blocks...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

// guardedFence is goldmark's fenced-code block parser that declines a line that
// has no block start instead of indexing it.
type guardedFence struct{ parser.BlockParser }

func (g *guardedFence) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if pc.BlockOffset() < 0 {
		return nil, parser.NoChildren
	}
	return g.BlockParser.Open(parent, reader, pc)
}

// mdReader holds the state of one FromMarkdown call.
type mdReader struct {
	src     []byte
	base    int // offset of src[0] in the (normalized) input
	lenient bool
	err     *UnrepresentableError

	trailer     *modelTrailer // the eden:model comment, once seen
	trailerNode ast.Node      // the HTML block it was read from

	spans bool  // Spans were asked for
	crAt  []int // see normalizeNewlines
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
	ext := []extent{{}}
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		if tb, ok := c.(*ast.ThematicBreak); ok && r.isSectionRule(tb) {
			secs = append(secs, Section{ID: len(secs) + 1})
			ext = append(ext, extent{})
			continue
		}
		cur := &secs[len(secs)-1]
		if hb, ok := c.(*ast.HTMLBlock); ok && r.sectionComment(hb, cur) {
			if c != r.trailerNode {
				r.addExtent(&ext[len(ext)-1], c)
			}
			continue
		}
		r.addExtent(&ext[len(ext)-1], c)
		cur.Blocks = append(cur.Blocks, r.block(c)...)
	}
	if r.spans {
		for i := range secs {
			if ext[i].ok {
				secs[i].Span = &Span{Start: r.orig(ext[i].lo), Stop: r.orig(ext[i].hi)}
			}
		}
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
		r.trailer, r.trailerNode = &tr, v
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
	// Canonical form is what one more trip through JSON leaves: an empty array or
	// object under an omitempty field (`"rowIds":[]`) is nil, exactly as when the
	// writer omits it and the field is read back. Without this a Document read from
	// such a header is not a fixed point of write-then-read.
	canon, err := json.Marshal(v)
	if err != nil {
		return err
	}
	reflect.ValueOf(v).Elem().SetZero()
	return json.Unmarshal(canon, v)
}

// isSectionRule reports whether a top-level thematic break is spelled with
// dashes (`---`, `- - -`): those separate sections (Marp's slide break). Every
// other spelling is a divider block.
func (r *mdReader) isSectionRule(tb *ast.ThematicBreak) bool {
	p := tb.Pos()
	return p >= 0 && p < len(r.src) && r.src[p] == '-'
}

// block converts one block node into zero or more Blocks. With Spans, a single
// block takes the node's span unless its converter set a more exact one.
func (r *mdReader) block(n ast.Node) []Block {
	out := r.convert(n)
	if r.spans && len(out) == 1 && out[0].Span == nil {
		out[0].Span = r.span(n)
	}
	return out
}

func (r *mdReader) convert(n ast.Node) []Block {
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
		out := []Block{{Kind: BlockMath, Display: true, Text: strings.Join(v.lines, "\n")}}
		if v.trail != "" {
			r.refuse(v.trailAt, ConstructMathTrailingText, "")
			out = append(out, Block{Kind: BlockParagraph, Text: v.trail})
			if r.spans {
				out[0].Span = r.span(v)
				out[1].Span = &Span{Start: r.orig(v.trailAt), Stop: r.orig(v.trailEnd)}
			}
		}
		return out
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
	stop   int // offset one past the last byte of the block seen so far

	// trail is the text that follows the closing $$ on its line, and trailAt and
	// trailEnd are its byte offsets (ConstructMathTrailingText).
	trail             string
	trailAt, trailEnd int
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
	line, segment := reader.PeekLine()
	width, pos := util.IndentWidth(line, reader.LineOffset())
	if width > 3 || !bytes.HasPrefix(line[pos:], []byte("$$")) {
		return nil, parser.NoChildren
	}
	rest := strings.TrimRight(string(line[pos+2:]), "\n")
	node := &mathBlockNode{stop: segment.Start + len(bytes.TrimRight(line, "\n"))}
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
	line, segment := reader.PeekLine()
	n.stop = segment.Start + len(bytes.TrimRight(line, "\n"))
	content := strings.TrimRight(string(line), "\n")
	if idx := indexDoubleDollar([]byte(content)); idx >= 0 {
		if before := content[:idx]; strings.TrimSpace(before) != "" {
			n.lines = append(n.lines, before)
		}
		if after := content[idx+2:]; strings.TrimSpace(after) != "" {
			lead := len(after) - len(strings.TrimLeft(after, " \t"))
			n.trail = strings.TrimSpace(after)
			n.trailAt = segment.Start + idx + 2 + lead
			n.trailEnd = n.trailAt + len(n.trail)
			n.stop = segment.Start + idx + 2 // the block ends at its closing $$
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
			node := &mathInlineNode{tex: cellTeX(parent, tex), display: true}
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
			node := &mathInlineNode{tex: cellTeX(parent, line[1:j])}
			block.Advance(j + 1)
			return node
		}
	}
	return nil
}

// cellTeX is the TeX a math node holds. In a table cell GFM reads `\|` as `|` in
// every inline span, so a `|` in TeX is written `\|` there and read back the
// same way; ToMarkdown writes a backslash before every `|` of a cell's TeX. TeX
// that is itself `\|` is therefore spelled `\\|` (the first backslash is read as
// itself, the second escapes the pipe). Outside a cell the TeX is read verbatim.
func cellTeX(parent ast.Node, tex []byte) string {
	if parent == nil || parent.Kind() != east.KindTableCell {
		return string(tex)
	}
	return unescapeCellPipes(string(tex))
}

// unescapeCellPipes turns each `\|` of s into `|`, reading left to right, so
// that any other backslash stays itself.
func unescapeCellPipes(s string) string {
	if !strings.Contains(s, `\|`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '|' {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
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
		if r.spans {
			bl.Span = r.span(li)
		}
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

// table reads a GFM table. A cell is read through the inline walker, the one a
// paragraph uses, so a cell holds bold, italic, strike, code, inline math, links
// and autolinks and refuses exactly what a paragraph refuses (AODex D14). The
// cells' text goes to Headers and Rows; the runs of a styled cell go to
// HeaderRuns and RowRuns, put in the canonical form the model defines (a plain
// cell is null, a table with no styled cell has neither field).
func (r *mdReader) table(t *east.Table) Block {
	bl := Block{Kind: BlockTable, Aligns: alignNames(t.Alignments)}
	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *east.TableHeader:
			bl.Headers, bl.HeaderRuns = r.cells(row)
		case *east.TableRow:
			cells, runs := r.cells(row)
			bl.Rows = append(bl.Rows, cells)
			bl.RowRuns = append(bl.RowRuns, runs)
		}
	}
	syncTableCellRuns(&bl)
	return bl
}

// cells reads a row: each cell's text and, for a styled cell, its runs (nil for a
// plain one).
func (r *mdReader) cells(row ast.Node) ([]string, [][]TextRun) {
	var cells []string
	var runs [][]TextRun
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*east.TableCell); ok {
			txt, rs := r.cell(cell)
			cells = append(cells, txt)
			runs = append(runs, rs)
		}
	}
	return cells, runs
}

// cell reads one cell as a one-line paragraph would be read: the same walker, the
// same refusals, and a line break (which a cell cannot hold) as a space. The runs
// are nil when the cell is plain.
func (r *mdReader) cell(cell *east.TableCell) (string, []TextRun) {
	return richFields(singleLine(r.inline(cell)))
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

// ---- spans ----

// extent is the byte range, in the reader's source, of a run of nodes.
type extent struct {
	lo, hi int
	ok     bool
}

func (r *mdReader) addExtent(e *extent, n ast.Node) {
	if !r.spans {
		return
	}
	lo, hi, ok := r.localSpan(n)
	if !ok {
		return
	}
	if !e.ok {
		*e = extent{lo, hi, true}
		return
	}
	e.lo, e.hi = min(e.lo, lo), max(e.hi, hi)
}

// span is the Span of n in the input as given, or nil when Spans was not asked
// for or n has no position.
func (r *mdReader) span(n ast.Node) *Span {
	if !r.spans {
		return nil
	}
	lo, hi, ok := r.localSpan(n)
	if !ok {
		return nil
	}
	return &Span{Start: r.orig(lo), Stop: r.orig(hi)}
}

// localSpan is n's byte range in r.src (see FromMarkdownOptions.Spans). It reuses
// spanOf, which Build uses for the same purpose: the node's own lines, else the
// union of its positioned descendants. A node with neither (a divider, an empty
// code block) takes its own source line. Trailing newlines are not part of a span.
func (r *mdReader) localSpan(n ast.Node) (int, int, bool) {
	var start, stop int
	switch v := n.(type) {
	case *directiveNode:
		start, stop = v.Pos(), v.stop
	case *mathBlockNode:
		start, stop = v.Pos(), v.stop
	case *ast.HTMLBlock:
		lines := v.Lines()
		if lines.Len() == 0 {
			return 0, 0, false
		}
		start, stop = lines.At(0).Start, lines.At(lines.Len()-1).Stop
		if v.HasClosure() {
			stop = v.ClosureLine.Stop
		}
	default:
		if sp := spanOf(n); sp != nil {
			start, stop = sp.Start, sp.Stop
		} else {
			start = n.Pos()
			stop = start
			for stop < len(r.src) && r.src[stop] != '\n' {
				stop++
			}
		}
	}
	for stop > start && stop <= len(r.src) && r.src[stop-1] == '\n' {
		stop--
	}
	if start < 0 || stop > len(r.src) || start > stop {
		return 0, 0, false
	}
	return start, stop, true
}

// orig maps an offset in r.src to the same position in the input as given: the
// front matter that precedes r.src is added back, and so is every "\r" that
// normalizeNewlines took out before the position.
func (r *mdReader) orig(off int) int {
	p := r.base + off
	return p + sort.SearchInts(r.crAt, p)
}

// outlineSpans copies each heading block's span onto its outline entry. Outline
// entries are in the order rebuildOutline walks the blocks.
func (r *mdReader) outlineSpans(doc *Document) {
	i := 0
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == BlockHeading && i < len(doc.Outline) {
				if b.Span != nil {
					sp := *b.Span
					doc.Outline[i].Span = &sp
				}
				i++
			}
			walk(b.Children)
		}
	}
	for _, sec := range doc.Sections {
		walk(sec.Blocks)
	}
}
