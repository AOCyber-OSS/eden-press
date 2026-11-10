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

// PARSE-01: the canonical, callable two-phase render seam this package's own
// doc comment (markdown.go) mandates but never itself exposed as a single
// entrypoint -- until now. seam.go is the Objective-2 hook: Parse returns the
// finalized *ast.Document + parser.Context BETWEEN phases (inspectable,
// mutable), Render drives both phases (never md.Convert()), and RenderFunc
// adapts Render to conformance/runner.RenderFunc's shape so the Objective-0
// corpus harness (conformance/runner/chase_corpus_test.go) can drive this
// engine exactly like it drives the goldmark baseline (engine.go).
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"

	"go.aocyber.ai/eden-press/chase/directive"
)

// NewEngine returns a chase engine: GFM (tables/strikethrough/linkify/task
// lists) + raw-HTML passthrough (WithUnsafe) + hard line breaks
// (WithHardWraps) + auto heading IDs (WithAutoHeadingID) + chase/markdown's
// own Extender (New()) -- comment detection, front-matter, slide-split,
// directive-apply, inline-SVG. This mirrors Marp Core's own markdown-it
// setup one step further than conformance/runner/engine.go's
// NewGoldmarkMarp (which chase's corpus-facing config approximates): the
// extra AutoHeadingID option and the chase Extender itself are what let the
// Marpit-mechanic corpus cases htmldiff-pass (01-RESEARCH.md
// error_recovery: "enable parser.WithAutoHeadingID() ... a parser option,
// not a battery").
//
// extra goldmark.Option values, if given, are appended AFTER the baked-in
// ones -- an extensibility hook for a caller (e.g. a later objective) that
// needs to layer additional parser/renderer options onto this same base
// configuration without duplicating it.
func NewEngine(extra ...goldmark.Option) goldmark.Markdown {
	opts := append([]goldmark.Option{
		// goldmark's parser with the container parsers bounded
		// (MaxContainerNesting): its parse is otherwise quadratic in the
		// containers one line opens (eden-press#29).
		goldmark.WithParser(NewNestingLimitedParser()),
		goldmark.WithExtensions(extension.GFM, New()),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(ghtml.WithUnsafe(), ghtml.WithHardWraps()),
	}, extra...)
	return goldmark.New(opts...)
}

// defaultEngine is the single chase engine instance Parse/Render/RenderFunc
// drive. A goldmark.Markdown's configured parser/renderer options never
// change across calls (all per-render variance -- inline-SVG mode,
// headingDivider -- flows through parser.Context, not engine construction),
// so one shared instance is safe to reuse and avoids re-registering every
// extension/transformer on every Render call.
var defaultEngine = NewEngine()

// Parse runs Phase 1 of the two-phase seam (Parser().Parse()) over md,
// pre-seeding the parser.Context with inline-SVG mode enabled
// (SvgOptionsKey) and, if md's front matter carries a "headingDivider"
// directive, that directive's fully resolved level range (HeadingDividerKey)
// -- the exact parser.Context values a caller driving the seam by hand would
// need to set (see markdown_test.go's TestHeadingDividerInsertsSyntheticBreaks).
//
// This pre-scan exists because of a chicken-and-egg gap between two already-
// locked pipeline stages: headingDividerTransformer (priority 100) runs
// BEFORE slide-split and needs headingDivider's value ALREADY resolved to a
// []int (headingdivider.go: "this transformer does not re-derive the
// scalar-to-range expansion itself"), but front matter is only parsed by the
// frontMatterBlockParser DURING Parse itself (populating FrontMatterKey,
// which directiveApplyTransformer at priority 300 reads back out for the
// data-heading-divider ATTRIBUTE materialization pass) -- too late for
// priority 100 to consume. Parse bridges that gap by independently running
// the SAME pure directive.DetectFrontMatter/ParseFrontMatter/CoerceGlobal
// calls before the real Parser().Parse() begins, so headingDividerTransformer
// has a resolved value to read the moment it runs.
//
// It returns the finalized *ast.Document AND the parser.Context used to
// produce it, both already inspectable -- Render (below) has not run yet.
// This is the Objective-2 seam hook: a caller can inspect or mutate doc here
// before ever calling Render.
func Parse(md string) (*ast.Document, parser.Context) {
	source := []byte(md)
	pc := parser.NewContext()
	pc.Set(SvgOptionsKey, &SvgOptions{Enabled: true})
	if levels, ok := frontMatterHeadingDividerLevels(md); ok {
		pc.Set(HeadingDividerKey, levels)
	}

	reader := text.NewReader(source)
	doc := defaultEngine.Parser().Parse(reader, parser.WithContext(pc))
	return doc.(*ast.Document), pc
}

// Render is the CANONICAL two-phase call PARSE-01 requires: Parse (above,
// Phase 1) followed, in a SEPARATE step, by Renderer().Render() (Phase 2) --
// NEVER md.Convert(), which collapses both phases into a single call and
// leaves no seam for a caller to inspect the finalized AST between them
// (criterion 1; see seam_test.go).
//
// opts is accepted for conformance/runner.RenderFunc interface compatibility
// (the corpus harness's pluggable-engine hook) but not consulted here --
// every render-affecting option this TRD's corpus cases need (inline-SVG
// mode, headingDivider) is instead derived from the document's own front
// matter, exactly like real Marp/Marpit resolves them -- mirroring
// conformance/runner/engine.go's GoldmarkRenderFunc, which documents the
// identical "opts not consulted, baked into engine/parse instead" stance.
func Render(md string, opts map[string]any) (string, error) {
	source := []byte(md)
	doc, _ := Parse(md)

	var buf bytes.Buffer
	if err := defaultEngine.Renderer().Render(&buf, source, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderFunc returns a func matching conformance/runner.RenderFunc's exact
// signature -- (markdown string, opts map[string]any) (string, error) --
// deliberately left as an UNNAMED function type so chase/markdown never
// needs to import conformance/runner (chase/markdown must stay a leaf
// package the test harness depends on, not the reverse). Go's assignability
// rules let a caller assign this return value directly to a
// runner.RenderFunc-typed variable: identical underlying type, and this side
// is unnamed.
func RenderFunc() func(markdown string, opts map[string]any) (string, error) {
	return Render
}

// frontMatterHeadingDividerLevels pre-scans md's front matter (if any) for a
// "headingDivider" directive, resolving it through
// chase/directive.CoerceGlobal -- the SAME coercion headingdivider.go's
// synthetic-break transformer requires its HeadingDividerKey value to
// already carry (a fully resolved []int, never a raw scalar). Returns
// ok=false when there is no front matter, it carries no headingDivider key,
// or that key does not coerce to a []int (e.g. "false" or an out-of-range
// value) -- in every such case headingDividerTransformer's own default
// (nothing configured -> insert no synthetic breaks) is exactly right.
func frontMatterHeadingDividerLevels(md string) ([]int, bool) {
	body, _, ok := directive.DetectFrontMatter(md)
	if !ok {
		return nil, false
	}
	for _, kv := range directive.ParseFrontMatter(body) {
		if kv.Key != "headingDivider" {
			continue
		}
		v, isKnown := directive.CoerceGlobal("headingDivider", kv.Val, nil)
		if !isKnown {
			return nil, false
		}
		levels, ok := v.([]int)
		if !ok {
			return nil, false
		}
		return levels, true
	}
	return nil, false
}
