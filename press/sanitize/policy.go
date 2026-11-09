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

// Package sanitize authors CORE-05's always-on HTML allow-list sanitization
// policy: a bluemonday policy that behaviorally matches Marp's current
// (v4-era) `xss` allow-list, co-designed against every wave-2 battery's
// documented output shape so legitimate deck markup survives while XSS
// vectors are neutralized.
//
// This package authors the POLICY only. press.Render (TRD 03-09) applies it
// as the ABSOLUTE LAST step over the fully-composed Output.HTML string --
// after every battery NodeRenderer and `.marpit` wrapping have run, never
// per-AST-node, and never over CSS or the Model.
package sanitize

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

// Policy returns the built-in, always-on Marp-parity HTML sanitization
// policy. It is a blank-slate bluemonday.Policy: nothing is permitted unless
// explicitly allow-listed below, so no attribute/element added by a future
// battery can leak through unreviewed.
//
// IMPORTANT for callers (see Sanitize below): bluemonday sanitizes through
// golang.org/x/net/html's low-level Tokenizer, whose Token() method
// unconditionally lowercases tag AND attribute names (TagName() and
// TagAttr() both call the tokenizer's internal lower()). That is harmless
// for ordinary HTML, but SVG's foreign-content parsing model is
// case-SENSITIVE: a browser only recognizes "foreignObject" (not
// "foreignobject") as the element that switches content model back to HTML,
// and only recognizes "viewBox" (not "viewbox") as the SVG viewBox
// attribute. Policy().Sanitize() ALONE does not restore this casing --
// bluemonday has no tag-name-casing hook. Callers (03-09) MUST use
// sanitize.Sanitize(html), which wraps Policy().Sanitize() with the
// required case restoration, to correctly preserve the inline-SVG
// container chain (<svg><foreignObject><section>).
func Policy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// Structural / layout / inline-formatting elements (Marp v4-era deck
	// output + Marpit's own wrapping): headings, paragraphs, lists,
	// tables, quotes, breaks, emphasis, strikethrough (03-03).
	p.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "div", "span", "br", "hr",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td",
		"blockquote",
		"a",
		"pre", "code",
		"strong", "em", "sub", "sup", "s", "del", "ins", "u", "b", "i",
		"figure", "figcaption",
		"header", "footer",
		"section",
	)

	// Inline-SVG container chain (Marpit InlineSVG rendering mode):
	// <div class="marpit"><svg data-marpit-svg viewBox="0 0 W H">
	//   <foreignObject width height [x] [data-marpit-advanced-background]>
	//     <section>...</section>
	// grounded against chase/markdown/inlinesvg.go + advancedbg.go.
	p.AllowElements("svg", "foreignObject")
	p.AllowAttrs("data-marpit-svg", "viewBox").OnElements("svg")
	p.AllowAttrs(
		"width", "height", "x",
		"data-marpit-advanced-background",
	).OnElements("foreignObject")

	// Advanced-background layer structure (chase/markdown/advancedbg.go):
	// background/content/pseudo layers.
	p.AllowAttrs(
		"data-marpit-advanced-background-container",
		"data-marpit-advanced-background-direction",
		"data-marpit-advanced-background",
		"data-marpit-advanced-background-split",
	).Globally()

	// Pagination attrs (chase/markdown/apply.go).
	p.AllowAttrs("data-marpit-pagination", "data-marpit-pagination-total").OnElements("section")

	// Heading slugs (goldmark parser.WithAutoHeadingID(), read via
	// chase/model/build.go's headingSlug) + universal id/class (chroma
	// .hljs-* spans, Marpit structural classes, CORE-09 fit marker).
	p.AllowAttrs("id").Globally()
	p.AllowAttrs("class").Globally()

	// CORE-09 fit marker (03-07 not frozen at authoring time -- candidate
	// baseline shapes are a `fit` class, already covered by the global
	// class allow above, OR a bare data-auto-scaling="fit" attribute on
	// the heading): hedge for the attribute form too.
	p.AllowAttrs("data-auto-scaling").OnElements("h1", "h2", "h3", "h4", "h5", "h6")

	// Links: href only, scheme-gated below.
	p.AllowAttrs("href").OnElements("a")

	// Images: twemoji <img> (03-04) + MathML PNG data-URI fallback
	// (03-06/03-08's go-latex/latex drawtex/drawimg path). draggable is
	// part of goldmark-emoji's literal Twemoji <img> template.
	p.AllowAttrs("src", "alt", "draggable").OnElements("img")

	// MathML (03-06): the real element/attribute set emitted by the
	// internal/latex2mathml fork's Convert(), grounded against its
	// converter.go/commands.go rather than assumed.
	// mathml_coverage_test.go derives the names from the converter's own
	// output: msqrt/mroot (\sqrt, \sqrt[n]), mphantom and menclose (\boxed)
	// were missing until eden-press#31, so every radical lost its root sign.
	// All four are presentation-only layout elements with no URL or script
	// attributes.
	mathElements := []string{
		"math", "mrow", "mi", "mn", "mo", "mtext", "mspace", "mstyle",
		"mpadded", "mtr", "mtd", "mtable",
		"msub", "msup", "msubsup", "mfrac", "mover", "munder", "munderover",
		"msqrt", "mroot", "mphantom", "menclose",
	}
	p.AllowElements(mathElements...)
	// Most MathML leaf elements (<mi>x</mi>, <mo>+</mo>, <mn>1</mn>,
	// <mrow>...) commonly carry NO attributes at all. bluemonday only
	// keeps a zero-attribute tag if the element is explicitly marked
	// AllowNoAttrs -- AllowElements alone is not sufficient for the
	// attribute-less case (see sanitize.go's allowNoAttrs gate).
	p.AllowNoAttrs().OnElements(mathElements...)
	p.AllowAttrs(
		"xmlns", "display", "displaystyle", "scriptlevel", "mathvariant",
		"mathsize", "stretchy", "accent", "fence", "form", "largeop",
		"movablelimits", "minsize", "maxsize", "linethickness",
		"rowspacing", "columnspacing", "columnalign", "rowlines",
		"depth", "voffset",
		// notation is menclose's box style ("box"); mathcolor is \color's
		// colour, a plain value bluemonday passes through as text.
		"notation", "mathcolor",
	).OnElements(mathElements...)
	// width/height are shared with <img>/<foreignObject> above; allow them
	// on the MathML elements too (real attrs emitted by the fork for
	// sized glyphs/tables).
	p.AllowAttrs("width", "height").OnElements(mathElements...)

	// URL scheme gating: http/https for ordinary links, data: for the
	// base64 PNG math fallback. RequireParseableURLs is the backstop that
	// blocks javascript: schemes outright regardless of attribute.
	p.AllowURLSchemes("http", "https", "data")
	p.RequireParseableURLs(true)

	// GFM disallowed-raw-HTML tags (tagfilter.go): strip tag AND content
	// uniformly across all nine, even the three not in bluemonday's own
	// hardcoded skip-content set (textarea, xmp, plaintext).
	p.SkipElementsContent(GFMDisallowedTags...)

	// Deliberately DO NOT AllowAttrs("style") anywhere -- bluemonday has
	// no CSS-value sanitizer (research security table); excluding style
	// entirely is the safe, documented posture, even though it degrades
	// the ![bg] background-image and advanced-background
	// split/color-inherit features for press.Render callers (tracked as a
	// known limitation in 03-08-SUMMARY.md, not silently patched around).
	//
	// Deliberately DO NOT AllowAttrs("on...") anywhere -- on* event
	// handler attributes (onclick, onerror, ...) are never allow-listed,
	// so bluemonday's blank-slate default strips them unconditionally.

	return p
}

// reForeignObjectTag and reViewBoxAttr restore the exact camelCase spelling
// of the SVG foreign-content names that bluemonday's underlying tokenizer
// lowercases (see Policy's doc comment). Scoped to exactly what this
// project's battery output emits (chase/markdown/inlinesvg.go,
// advancedbg.go): foreignObject is the only case-sensitive element and
// viewBox the only case-sensitive attribute in play.
var (
	reForeignObjectTag = regexp.MustCompile(`(?i)<(/?)foreignobject\b`)
	reViewBoxAttr      = regexp.MustCompile(`(?i)\bviewbox=`)
)

// Sanitize is the RECOMMENDED full sanitize pipeline: Policy().Sanitize
// followed by SVG element/attribute case restoration. press.Render (TRD
// 03-09) MUST call Sanitize -- not bare Policy().Sanitize() -- as the
// absolute last step over the fully-composed Output.HTML string, to
// correctly preserve the inline-SVG container chain
// (<svg><foreignObject><section>). See Policy's doc comment for why the
// restoration is necessary, and TestPreserveInlineSVGCaseRestoration for the
// regression proof.
func Sanitize(html string) string {
	out := Policy().Sanitize(html)
	// NOTE: "${1}", not "$1foreignObject" -- Go's regexp ReplaceAllString
	// treats "$1f..." as a reference to a (nonexistent) submatch NAMED
	// "1foreignObject", silently substituting empty string. The braced
	// form disambiguates the group boundary.
	out = reForeignObjectTag.ReplaceAllString(out, "<${1}foreignObject")
	out = reViewBoxAttr.ReplaceAllString(out, "viewBox=")
	return out
}
