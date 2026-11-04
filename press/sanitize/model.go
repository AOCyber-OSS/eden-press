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
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// ModelPolicy is the sanitization policy for HTML fragments that
// chase/model.BlockHTML writes for the v7-only block kinds (toggle, todo,
// callout, figure, ...). It is Policy() plus the elements those fragments need
// and nothing else:
//
//   - details[open], summary, aside, label        (toggle, callout, todo)
//   - input[type=checkbox][checked][disabled]     (todo; see SanitizeModel)
//   - a conservative SVG drawing subset           (figure.svg)
//
// Policy() stays untouched on purpose. It is the Marp-parity policy
// press.Render applies to Markdown-derived HTML, and widening it would change
// what every existing Render caller's output can contain. Only
// press.RenderModel, for blocks it substitutes itself, uses this one.
//
// The SVG subset is shapes, text and gradients only. Absent on purpose: script,
// style, use, image, a, animate/set, filter, mask, pattern, marker and
// foreignObject children are not added to what Policy() already allows --
// every one of them can execute, restyle, or fetch an external resource.
// Presentation attributes are value-checked: paint values accept colours and
// same-document url(#id) references only (never url(https://...)), and there is
// no style attribute, so no CSS can reach the page.
func ModelPolicy() *bluemonday.Policy {
	p := Policy()

	p.AllowElements("details", "summary", "aside", "label")
	p.AllowNoAttrs().OnElements("details", "summary", "aside", "label")
	p.AllowAttrs("open").OnElements("details")

	// Todo checkbox. The element is only meaningful as a disabled checkbox;
	// SanitizeModel drops any other <input> after this pass, because
	// bluemonday cannot make an element conditional on an attribute value.
	p.AllowAttrs("type").Matching(reCheckbox).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")

	p.AllowElements(svgShapeElements...)
	p.AllowNoAttrs().OnElements(svgShapeElements...)

	p.AllowAttrs("xmlns").Matching(reSVGNamespace).OnElements("svg")
	p.AllowAttrs("preserveaspectratio").Matching(reWords).OnElements("svg")
	p.AllowAttrs("width", "height").Matching(reLength).OnElements("svg")

	geometry := []string{"x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry", "width", "height", "offset"}
	p.AllowAttrs(geometry...).Matching(reLength).OnElements(svgShapeElements...)
	p.AllowAttrs("d").Matching(rePathData).OnElements("path")
	p.AllowAttrs("points").Matching(reNumberList).OnElements("polyline", "polygon")
	p.AllowAttrs("transform", "gradienttransform").Matching(reTransform).OnElements(svgShapeElements...)

	p.AllowAttrs("fill", "stroke", "stop-color").Matching(rePaint).OnElements(svgShapeElements...)
	p.AllowAttrs("stroke-width", "font-size", "stroke-miterlimit").Matching(reLength).OnElements(svgShapeElements...)
	p.AllowAttrs("opacity", "fill-opacity", "stroke-opacity", "stop-opacity").Matching(reNumber).OnElements(svgShapeElements...)
	p.AllowAttrs("stroke-dasharray").Matching(reNumberList).OnElements(svgShapeElements...)
	p.AllowAttrs("stroke-linecap", "stroke-linejoin", "text-anchor", "font-weight", "font-style",
		"fill-rule", "clip-rule", "dominant-baseline", "gradientunits", "spreadmethod").Matching(reWords).OnElements(svgShapeElements...)
	p.AllowAttrs("font-family").Matching(reFontFamily).OnElements(svgShapeElements...)

	return p
}

// svgShapeElements is the drawing subset ModelPolicy adds. svg itself is
// already allowed by Policy() (it is the inline-SVG container element).
var svgShapeElements = []string{
	"g", "defs", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon",
	"text", "tspan", "lineargradient", "radialgradient", "stop",
}

var (
	reCheckbox     = regexp.MustCompile(`^checkbox$`)
	reSVGNamespace = regexp.MustCompile(`^http://www\.w3\.org/2000/svg$`)
	reLength       = regexp.MustCompile(`^-?[0-9]*\.?[0-9]+(px|em|rem|pt|%)?$`)
	reNumber       = regexp.MustCompile(`^-?[0-9]*\.?[0-9]+([eE][+-]?[0-9]+)?$`)
	reNumberList   = regexp.MustCompile(`^[0-9eE.,\s%+-]+$`)
	rePathData     = regexp.MustCompile(`^[MmLlHhVvCcSsQqTtAaZz0-9eE.,\s+-]+$`)
	reTransform    = regexp.MustCompile(`^[A-Za-z0-9(),.\s+-]+$`)
	reWords        = regexp.MustCompile(`^[A-Za-z0-9 -]{1,40}$`)
	reFontFamily   = regexp.MustCompile(`^[A-Za-z0-9 ,_-]{1,100}$`)
	// rePaint accepts a keyword/named colour, #hex, rgb()/hsl() or a
	// same-document url(#id). url(https://...) is the external-fetch vector.
	rePaint = regexp.MustCompile(`^(none|currentColor|transparent|#[0-9a-fA-F]{3,8}|[a-zA-Z]{3,20}|(rgb|hsl)a?\([0-9., %]+\)|url\(#[A-Za-z0-9_-]{1,64}\))$`)

	reInputTag = regexp.MustCompile(`<input\b[^>]*>`)

	// The SVG names ModelPolicy adds that are camelCase in SVG but that the
	// sanitizer's tokenizer lowercases (see Policy's doc comment). An HTML
	// parser re-capitalises them itself; restoring them here keeps the
	// markup correct for any consumer that is not one.
	reGradientTag  = regexp.MustCompile(`(?i)<(/?)(linear|radial)gradient\b`)
	reSVGCamelAttr = regexp.MustCompile(`(?i)\b(gradientunits|gradienttransform|preserveaspectratio)=`)
)

var (
	svgCamelTag  = strings.NewReplacer("lineargradient", "linearGradient", "radialgradient", "radialGradient")
	svgCamelAttr = map[string]string{
		"gradientunits=":       "gradientUnits=",
		"gradienttransform=":   "gradientTransform=",
		"preserveaspectratio=": "preserveAspectRatio=",
	}
)

// SanitizeModel is Sanitize for model-originated fragments: ModelPolicy plus
// the same SVG case restoration, then removal of any <input> that is not a
// checkbox. It is idempotent.
func SanitizeModel(html string) string {
	out := ModelPolicy().Sanitize(html)
	out = reForeignObjectTag.ReplaceAllString(out, "<${1}foreignObject")
	out = reViewBoxAttr.ReplaceAllString(out, "viewBox=")
	out = reGradientTag.ReplaceAllStringFunc(out, func(tag string) string {
		return svgCamelTag.Replace(strings.ToLower(tag))
	})
	out = reSVGCamelAttr.ReplaceAllStringFunc(out, func(attr string) string {
		return svgCamelAttr[strings.ToLower(attr)]
	})
	return reInputTag.ReplaceAllStringFunc(out, func(tag string) string {
		if strings.Contains(tag, `type="checkbox"`) {
			return tag
		}
		return ""
	})
}
