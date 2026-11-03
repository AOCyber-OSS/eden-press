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

package press

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/press/sanitize"
)

// RenderModel renders a v7 docmodel through the same pipeline as [Render] and
// returns the same [Output]: themed, profiled, sanitized HTML plus the packed
// CSS, so PDF, PNG and the themed/paged HTML no longer need Markdown to start
// from.
//
// # Why it goes through Render
//
// Theme packing, profile resolution, the unit/container markup, pagination and
// the sanitizer are not re-implemented. RenderModel builds a Marp-renderable
// Markdown from the model and hands it to Render, so for a given theme and
// profile Output.CSS is byte-identical to Render's, and Output.HTML carries
// exactly one profile unit element (<section>) per Section, in order -- an empty
// Section is an empty unit, never merged away.
//
// Which kinds go through the Markdown engine and which do not:
//
//   - heading, paragraph, list, code and math are emitted as Markdown, so they
//     get what Render gives Markdown (heading ids, syntax highlighting, MathML,
//     autofit). All model text is escaped: block text is data, so "---", "# x",
//     "<!-- _class: x -->" or "$5" come out as that text and cannot split a
//     page, become a directive or grow elements.
//   - every other kind (toggle, synced, todo, callout, divider, file, sheet,
//     figure, table, image, quote, and any kind this version does not know) is
//     written by [model.BlockHTMLEmbedded], sanitized with
//     [sanitize.SanitizeModel], and substituted for a placeholder paragraph
//     after Render. If a placeholder is missing, duplicated or left outside its
//     own paragraph, or the page's unit count differs from len(Sections),
//     RenderModel returns an error: a block is never silently dropped.
//
// A caller-supplied Options.Sanitize replaces the built-in policy for both the
// page and the substituted fragments.
//
// # Contract
//
// doc is not modified. Output.Model is a validated COPY of it (the validation
// [GetDocJSON] applies: schema version defaulted, block and list-item ids
// ensured, text mirrored from runs), so convert/png and convert/pdf, which walk
// Output.Model.Sections to select pages, accept the result unchanged.
// Output.Meta is Model.Meta, Output.Comments are the Sections' notes in
// document order (notes are never part of the HTML), and Output.Profile is the
// resolved profile id.
//
// Section.Attrs carries the Marp local directives as data-<name> attributes
// (data-class, data-paginate, ...); those, and the bare directive names, are
// re-applied per Section. Meta.Directives become the document's front matter
// except headingDivider: the model's Sections are authoritative, and a
// heading-driven split must not re-cut them.
//
// A document with no Sections is an error. RenderModel is safe for concurrent
// use.
func RenderModel(doc *model.Document, opts Options) (Output, error) {
	if doc == nil {
		return Output{}, errors.New("press: RenderModel: nil document")
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return Output{}, fmt.Errorf("press: RenderModel: copy document: %w", err)
	}
	var d model.Document
	if err := json.Unmarshal(raw, &d); err != nil {
		return Output{}, fmt.Errorf("press: RenderModel: copy document: %w", err)
	}
	return renderModel(&d, opts)
}

// RenderModelJSON is the byte form of [RenderModel], for callers (the CLI, the
// Dart binding) that hold docmodel JSON rather than a *model.Document: it
// decodes docJSON, applies the validation [GetDocJSON] applies, and renders.
// Malformed JSON, or a document with no sections, is an error.
func RenderModelJSON(docJSON []byte, opts Options) (Output, error) {
	var d model.Document
	if err := json.Unmarshal(docJSON, &d); err != nil {
		return Output{}, fmt.Errorf("press: RenderModelJSON: parse document: %w", err)
	}
	return renderModel(&d, opts)
}

// renderModel validates d IN PLACE (the callers hand it a private copy) and
// renders it.
func renderModel(d *model.Document, opts Options) (Output, error) {
	if d.SchemaVersion == "" {
		d.SchemaVersion = model.SchemaVersion
	}
	model.EnsureBlockIDs(d)
	model.EnsureListItemIDs(d)
	model.EnsureTextRunsInvariant(d)
	if len(d.Sections) == 0 {
		return Output{}, errors.New("press: RenderModel: document has no sections")
	}

	p, err := resolveProfile(opts.Profile)
	if err != nil {
		return Output{}, fmt.Errorf("press: RenderModel: %w", err)
	}
	nonce, err := newNonce()
	if err != nil {
		return Output{}, fmt.Errorf("press: RenderModel: %w", err)
	}

	sanitizeFragment := func(h string) string {
		if opts.Sanitize != nil {
			return opts.Sanitize.Sanitize(h)
		}
		return sanitize.SanitizeModel(h)
	}
	mb := &mdBuilder{doc: d, nonce: nonce, sanitize: sanitizeFragment}
	md := mb.build()

	out, err := Render(md, opts)
	if err != nil {
		return Output{}, fmt.Errorf("press: RenderModel: %w", err)
	}

	// Pages are selected by position downstream, so the page count is checked
	// before anything is substituted: it must be exactly the section count.
	unit := regexp.MustCompile(`<` + regexp.QuoteMeta(p.UnitElement()) + `[\s>/]`)
	if n := len(unit.FindAllStringIndex(out.HTML, -1)); n != len(d.Sections) {
		return Output{}, fmt.Errorf("press: RenderModel: rendered %d <%s> units for %d sections", n, p.UnitElement(), len(d.Sections))
	}

	out.HTML, err = spliceFragments(out.HTML, nonce, mb.frags)
	if err != nil {
		return Output{}, fmt.Errorf("press: RenderModel: %w", err)
	}
	out.Model = d
	out.Meta = d.Meta
	out.Comments = flattenNotes(d)
	return out, nil
}

// --- Markdown generation ------------------------------------------------------

// fragment is one substituted block: the placeholder token standing in for it
// in the generated Markdown, and its already-sanitized HTML.
type fragment struct {
	token string
	html  string
}

// placeholderToken is the text of the one-paragraph placeholder for fragment i.
// It is letters and digits only, so no Markdown rule rewrites it, and it embeds
// a per-render random nonce, so document text cannot forge or collide with it.
func placeholderToken(nonce string, i int) string {
	return "EPBLK" + nonce + "X" + strconv.Itoa(i) + "X"
}

func newNonce() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("placeholder nonce: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// localDirectives are the Marp local directives a Section can carry, with the
// kebab-case suffix Marp materializes onto the <section> as data-<kebab>.
var localDirectives = []struct{ camel, kebab string }{
	{"backgroundColor", "background-color"},
	{"backgroundImage", "background-image"},
	{"backgroundPosition", "background-position"},
	{"backgroundRepeat", "background-repeat"},
	{"backgroundSize", "background-size"},
	{"color", "color"},
	{"class", "class"},
	{"footer", "footer"},
	{"header", "header"},
	{"paginate", "paginate"},
}

var directiveKeyRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// mdBuilder writes the Markdown Render will parse. Everything that is not
// Markdown-expressible is parked in frags behind a placeholder paragraph.
type mdBuilder struct {
	doc      *model.Document
	nonce    string
	sanitize func(string) string

	sb    strings.Builder
	frags []fragment
	// lists counts lists written so far. It alternates the list marker so two
	// adjacent lists stay two lists instead of merging into one.
	lists int
}

func (b *mdBuilder) build() string {
	b.frontMatter()
	for i, sec := range b.doc.Sections {
		if i > 0 {
			b.sb.WriteString("---\n\n")
		}
		b.sectionDirectives(sec.Attrs)
		for _, bl := range sec.Blocks {
			b.block(bl)
		}
	}
	return b.sb.String()
}

// frontMatter always writes a front-matter block, even an empty one: with it,
// a document whose first sections are empty cannot be mistaken for one that
// merely starts with a "---" fence.
func (b *mdBuilder) frontMatter() {
	keys := make([]string, 0, len(b.doc.Meta.Directives))
	for k := range b.doc.Meta.Directives {
		// headingDivider would re-cut the model's own sections.
		if k != "headingDivider" && directiveKeyRE.MatchString(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	b.sb.WriteString("---\n")
	for _, k := range keys {
		v := oneLine(b.doc.Meta.Directives[k])
		b.sb.WriteString(k + ": " + `"` + v + `"` + "\n")
	}
	b.sb.WriteString("---\n\n")
}

// sectionDirectives re-applies a Section's Marp local directives as spot
// directives (<!-- _class: x -->), which scope to this section only. Values
// that could end the comment are dropped.
func (b *mdBuilder) sectionDirectives(attrs map[string]string) {
	for _, d := range localDirectives {
		v := attrs["data-"+d.kebab]
		if v == "" {
			v = attrs[d.camel]
		}
		if v == "" {
			v = attrs[d.kebab]
		}
		v = oneLine(v)
		if v == "" || strings.Contains(v, "-->") || strings.Contains(v, "--!>") || strings.Contains(v, "<!--") {
			continue
		}
		b.sb.WriteString("<!-- _" + d.camel + `: "` + v + `" -->` + "\n\n")
	}
}

func (b *mdBuilder) block(bl model.Block) {
	switch bl.Kind {
	case model.BlockHeading:
		if t := inlineMarkdown(bl.Text, bl.Runs, false); t != "" {
			level := bl.Level
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			b.sb.WriteString(strings.Repeat("#", level) + " " + t + "\n\n")
			return
		}
	case model.BlockParagraph:
		if t := inlineMarkdown(bl.Text, bl.Runs, true); t != "" {
			b.sb.WriteString(t + "\n\n")
			return
		}
	case model.BlockList:
		if b.list(bl) {
			return
		}
	case model.BlockCode:
		b.code(bl)
		return
	case model.BlockMath:
		if b.math(bl) {
			return
		}
	}
	b.placeholder(bl)
}

// placeholder parks bl behind a one-paragraph token. Its HTML is produced and
// sanitized now; spliceFragments swaps it in after Render.
func (b *mdBuilder) placeholder(bl model.Block) {
	tok := placeholderToken(b.nonce, len(b.frags))
	frag := b.sanitize(model.BlockHTMLEmbedded(b.doc, bl))
	b.frags = append(b.frags, fragment{token: tok, html: strings.TrimRight(frag, "\n")})
	b.sb.WriteString(tok + "\n\n")
}

// list writes bl as nested Markdown list items. It reports false for a list
// with no items (the placeholder path then renders it).
func (b *mdBuilder) list(bl model.Block) bool {
	if len(bl.Items) == 0 {
		return false
	}
	marker := "-"
	switch {
	case bl.Ordered && b.lists%2 == 0:
		marker = "1."
	case bl.Ordered:
		marker = "1)"
	case b.lists%2 == 1:
		marker = "*"
	}
	prev := -1
	for _, it := range bl.Items {
		// An item may nest at most one level under its predecessor; a deeper
		// jump would read as an indented code block.
		lvl := it.Level
		if lvl < 0 {
			lvl = 0
		}
		if lvl > prev+1 {
			lvl = prev + 1
		}
		prev = lvl
		b.sb.WriteString(strings.Repeat("    ", lvl) + marker + " " + inlineMarkdown(it.Text, it.Runs, false) + "\n")
	}
	b.sb.WriteString("\n")
	b.lists++
	return true
}

// code writes bl as a fenced block whose fence is longer than any backtick run
// in its text, so the text can never close it.
func (b *mdBuilder) code(bl model.Block) {
	text := strings.TrimSuffix(strings.ReplaceAll(bl.Text, "\r\n", "\n"), "\n")
	fence := strings.Repeat("`", maxInt(3, longestRun(text, '`')+1))
	b.sb.WriteString(fence + codeInfo(bl.Language) + "\n")
	if text != "" {
		b.sb.WriteString(text + "\n")
	}
	b.sb.WriteString(fence + "\n\n")
}

var codeInfoRE = regexp.MustCompile(`[^A-Za-z0-9_+#.-]`)

// codeInfo reduces a language to its first word, restricted to characters that
// cannot end the fence line or start another construct.
func codeInfo(lang string) string {
	f := strings.Fields(lang)
	if len(f) == 0 {
		return ""
	}
	info := codeInfoRE.ReplaceAllString(f[0], "")
	if len(info) > 32 {
		info = info[:32]
	}
	return info
}

// math writes bl as a single-line $$…$$ / $…$ construct, which the math
// battery renders to MathML. It reports false when the TeX cannot be written
// that way without risk (empty, or containing '$' or '%'), and the placeholder
// path then shows it as plain math source instead.
func (b *mdBuilder) math(bl model.Block) bool {
	tex := strings.TrimSpace(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(bl.Text))
	if tex == "" || strings.ContainsAny(tex, "$%") {
		return false
	}
	if bl.Display {
		b.sb.WriteString("$$" + tex + "$$\n\n")
	} else {
		b.sb.WriteString("$" + tex + "$\n\n")
	}
	return true
}

// inlineMarkdown renders a block's inline content. With runs, marks become HTML
// tags rather than Markdown delimiters: CommonMark's flanking rules make
// "**bold.**Next" fail to emphasize, raw tags have no such rule. All text is
// escaped. multiline keeps line breaks (the engine renders them as <br>);
// otherwise they collapse to spaces. Blank lines are dropped, since one would
// end the paragraph, and each line is trimmed, since four leading spaces would
// start a code block.
func inlineMarkdown(text string, runs []model.TextRun, multiline bool) string {
	s := escapeMarkdown(text)
	if len(runs) > 0 {
		s = runsMarkdown(runs)
	}
	var lines []string
	for _, l := range strings.Split(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if multiline {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines, " ")
}

func runsMarkdown(runs []model.TextRun) string {
	var sb strings.Builder
	for _, r := range runs {
		t := escapeMarkdown(r.Text)
		if len(r.Marks) > 0 || r.Link != "" {
			// A tag alone on a line would start a raw HTML block, whose body
			// is not Markdown-processed; keep marked text on one line.
			t = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(t)
		}
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
			href := strings.NewReplacer("\r", "", "\n", "").Replace(r.Link)
			t = `<a href="` + html.EscapeString(href) + `">` + t + "</a>"
		}
		sb.WriteString(t)
	}
	return sb.String()
}

// escapeMarkdown backslash-escapes every ASCII punctuation character, which
// CommonMark guarantees renders as the literal character. It is deliberately
// blunt: the generated Markdown is never shown, so readability costs nothing,
// and any narrower set is a set someone has to prove complete against
// "---", "1.", "#", "<!--", "$", ":emoji:" and the autolinker.
func escapeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	for i := 0; i < len(s); i++ {
		if isASCIIPunct(s[i]) {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isASCIIPunct(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s))
}

func longestRun(s string, c byte) int {
	best, cur := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// spliceFragments replaces each placeholder paragraph in the rendered page with
// its fragment. Every fragment must be found exactly once, in a paragraph of
// its own, and no placeholder text may remain anywhere afterwards -- otherwise
// a block would be dropped, duplicated or leaked, and that is an error.
func spliceFragments(pageHTML, nonce string, frags []fragment) (string, error) {
	prefix := "EPBLK" + nonce
	re := regexp.MustCompile(`<p>` + regexp.QuoteMeta(prefix) + `X(\d+)X</p>`)
	seen := make([]int, len(frags))
	var bad error
	out := re.ReplaceAllStringFunc(pageHTML, func(m string) string {
		idx, err := strconv.Atoi(re.FindStringSubmatch(m)[1])
		if err != nil || idx < 0 || idx >= len(frags) {
			bad = fmt.Errorf("placeholder %q does not name a block", m)
			return m
		}
		seen[idx]++
		return frags[idx].html
	})
	if bad != nil {
		return "", bad
	}
	for i, n := range seen {
		if n != 1 {
			return "", fmt.Errorf("block %d (%s) appeared %d times in the rendered page, want exactly 1", i, frags[i].token, n)
		}
	}
	if strings.Contains(out, prefix) {
		return "", errors.New("a block placeholder was left outside its own paragraph in the rendered page")
	}
	return out, nil
}
