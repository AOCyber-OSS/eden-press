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
	"testing"

	l2m "go.aocyber.ai/eden-press/internal/latex2mathml"
)

// The sanitizer must keep every element and attribute the latex2mathml fork
// emits (eden-press#31). Its allowlist was written before the 08-02 fix made
// \sqrt[n] produce <mroot>, and it never listed <msqrt>: press.Render stripped
// both radicals and left the radicand and index as loose siblings, so neither
// eden-press nor a consumer ever showed a root sign.
//
// Test list:
//
//	M1 for TeX covering each converter tag with no allowlist entry (msqrt,
//	   mroot, mphantom, menclose, mathcolor), every start tag and attribute
//	   name in the converter's output survives Sanitize
//
// The names come from the converter output itself, so a tag the converter
// starts emitting later is caught by adding its TeX here, never by
// hand-copying the allowlist into the test.

var (
	startTag = regexp.MustCompile(`<([a-z]+)((?:\s+[a-z:]+="[^"]*")*)\s*/?>`)
	attrName = regexp.MustCompile(`\s([a-z:]+)="`)
)

// names returns each start-tag name and each "tag@attr" pair in html.
func names(html string) map[string]bool {
	out := map[string]bool{}
	for _, m := range startTag.FindAllStringSubmatch(html, -1) {
		out[m[1]] = true
		for _, a := range attrName.FindAllStringSubmatch(m[2], -1) {
			out[m[1]+"@"+a[1]] = true
		}
	}
	return out
}

func TestSanitize_M1_KeepsEveryConverterTagAndAttribute(t *testing.T) {
	for _, tex := range []string{
		`\sqrt{x}`, `\sqrt[3]{x}`, `\phantom{x}`, `\boxed{x}`, `\color{red}{x}`,
	} {
		raw := l2m.Convert(tex, "http://www.w3.org/1998/Math/MathML", "inline", 0)
		kept := names(Sanitize(raw))
		for n := range names(raw) {
			if !kept[n] {
				t.Errorf("%s: Sanitize dropped %q\nconverter: %s\nsanitized: %s", tex, n, raw, Sanitize(raw))
			}
		}
	}
}
