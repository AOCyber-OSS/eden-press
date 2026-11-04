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

package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ParseWithEngine is the engine-parameterized twin of Parse (seam.go): it
// runs Phase 1 of the two-phase seam over md using a CALLER-SUPPLIED goldmark
// engine instead of the package-level defaultEngine, pre-seeding the
// parser.Context with EXACTLY the same state Parse pre-seeds -- inline-SVG
// mode enabled (SvgOptionsKey) and, when md's front matter carries a
// "headingDivider" directive, that directive's fully resolved level range
// (HeadingDividerKey). It is the composition seam a SIBLING renderer needs:
// press.Render (Objective 3, TRD 03-09) builds its OWN battery-laden engine
// via NewEngine(pressExtraOpts...) and drives it through this SAME one-parse
// flow, without press ever re-implementing (or chase ever exposing a mutable
// hook into) the defaultEngine path Parse/Render/RenderFunc use.
//
// It is purely ADDITIVE. Parse, Render, RenderDoc, RenderFunc, and
// defaultEngine are untouched; every existing caller of the seam is
// byte-for-byte unaffected. The only difference from Parse is line "the ONE
// parse": engine.Parser().Parse(...) is substituted for
// defaultEngine.Parser().Parse(...). The frontMatterHeadingDividerLevels
// pre-scan (seam.go) exists to bridge the same headingDividerTransformer
// chicken-and-egg gap Parse documents; ParseWithEngine reuses that already
// package-private helper directly.
//
// Like Parse, it returns the finalized *ast.Document AND the parser.Context
// used to produce it, both already inspectable BETWEEN phases -- rendering
// has NOT happened yet. The caller renders that exact tree in a separate
// Phase-2 step via engine.Renderer().Render(&buf, source, doc) (source being
// []byte(md)), never md.Convert(), preserving the one-parse-two-sinks
// invariant. The returned engine is the caller's own, so ParseWithEngine
// carries no hidden coupling to defaultEngine's renderer.
func ParseWithEngine(md string, engine goldmark.Markdown) (*ast.Document, parser.Context) {
	return ParseWithEngineSVG(md, engine, true)
}

// ParseWithEngineSVG is ParseWithEngine with the inline-SVG decision handed to
// the caller instead of fixed at "on". inlineSVG=true is exactly
// ParseWithEngine: every section is wrapped in <svg><foreignObject>. With
// inlineSVG=false SvgOptionsKey is seeded Enabled=false, so svgTransformer
// leaves every section a direct child of the container <div> -- the only shape
// a profile with ONE container chain has CSS for (profiles/paged: its pages are
// fixed physical boxes, and the stylesheet is scoped to "div.edenpress-paged >
// section"). A caller that knows the profile (press.Render) makes the decision;
// this seam does not.
//
// Everything else -- the headingDivider pre-seed, the one parse, the returned
// parser.Context -- is identical to ParseWithEngine.
func ParseWithEngineSVG(md string, engine goldmark.Markdown, inlineSVG bool) (*ast.Document, parser.Context) {
	source := []byte(md)
	pc := parser.NewContext()
	pc.Set(SvgOptionsKey, &SvgOptions{Enabled: inlineSVG})
	if levels, ok := frontMatterHeadingDividerLevels(md); ok {
		pc.Set(HeadingDividerKey, levels)
	}

	reader := text.NewReader(source)
	doc := engine.Parser().Parse(reader, parser.WithContext(pc)) // ★ the ONE parse, via the caller's engine
	return doc.(*ast.Document), pc
}
