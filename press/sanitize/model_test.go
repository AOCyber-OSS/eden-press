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

// v7Fragments are the shapes chase/model.BlockHTML emits for the v7-only block
// kinds, one per kind. ModelPolicy exists because the base Policy (Marp deck
// parity) deliberately allow-lists none of details/summary/aside/input/label or
// any SVG child element, so every one of these would be reduced to bare text.
var v7Fragments = []struct {
	name string
	in   string
	want []string
}{
	{"toggle open", `<details open><summary>More</summary><p>inner</p></details>`,
		[]string{`<details open="">`, `<summary>More</summary>`, `<p>inner</p>`}},
	{"toggle collapsed", `<details><summary>More</summary><p>inner</p></details>`,
		[]string{`<details>`, `<summary>More</summary>`}},
	{"todo checked", `<ul class="todo"><li><label><input type="checkbox" disabled checked/> Ship</label></li></ul>`,
		[]string{`class="todo"`, `<label>`, `type="checkbox"`, `checked`, `disabled`, `Ship`}},
	{"todo unchecked", `<ul class="todo"><li><label><input type="checkbox" disabled/> Ship</label></li></ul>`,
		[]string{`type="checkbox"`, `disabled`, `Ship`}},
	{"callout", `<aside class="callout warn"><span class="icon">!</span> Watch</aside>`,
		[]string{`<aside class="callout warn">`, `<span class="icon">!</span>`, `Watch`, `</aside>`}},
	{"file", `<div class="file"><a href="https://x.test/r.pdf">r.pdf</a></div>`,
		[]string{`class="file"`, `<a href="https://x.test/r.pdf"`, `r.pdf`}},
	{"sheet", `<div class="sheet-embed"><header>Tasks</header><table><tr><td>6</td></tr></table></div>`,
		[]string{`class="sheet-embed"`, `<header>Tasks</header>`, `<td>6</td>`}},
	{"divider", `<hr/>`, []string{`<hr`}},
	{"figure svg", `<figure class="figure"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50">` +
		`<defs><linearGradient id="g1" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#d4a853"/></linearGradient></defs>` +
		`<g transform="translate(2 2)"><rect x="1" y="2" width="30" height="20" rx="3" fill="url(#g1)" stroke="#000" stroke-width="2"/>` +
		`<circle cx="50" cy="25" r="10" fill="rgb(10,20,30)"/><path d="M0 0L10 10Z" fill="none" stroke="currentColor"/>` +
		`<polyline points="0,0 5,5 10,0" fill="none"/><text x="5" y="40" font-size="12" text-anchor="middle">Hi</text></g></svg>` +
		`<figcaption>logo</figcaption></figure>`,
		[]string{`<figure class="figure">`, `viewBox="0 0 100 50"`, `<linearGradient id="g1" gradientUnits="userSpaceOnUse">`, `</linearGradient>`, `width="100"`, `<rect `, `fill="url(#g1)"`, `<circle `,
			`<path `, `d="M0 0L10 10Z"`, `<polyline `, `<text `, `text-anchor="middle"`, `Hi`, `<figcaption>logo</figcaption>`}},
}

func TestModelPolicyPreservesV7Fragments(t *testing.T) {
	for _, tc := range v7Fragments {
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeModel(tc.in)
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("SanitizeModel(%q)\n = %q\nwant it to contain %q", tc.in, out, want)
				}
			}
		})
	}
}

// TestModelPolicyStripsActiveContent is the safety half: the extra allow-list
// must not open an XSS or external-fetch path. Each input pairs one
// survivable element with one hostile construct.
func TestModelPolicyStripsActiveContent(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		banned []string
		kept   []string
	}{
		{"script and handlers in svg",
			`<svg viewBox="0 0 10 10" onload="alert(1)"><script>alert(2)</script><rect onclick="alert(3)" width="5" height="5"/></svg>`,
			[]string{"onload", "onclick", "<script", "alert"}, []string{`<rect `}},
		{"javascript href on an svg anchor",
			`<svg viewBox="0 0 10 10"><a href="javascript:alert(1)" xlink:href="javascript:alert(2)"><circle r="3"/></a></svg>`,
			[]string{"javascript", "alert", "xlink"}, []string{`<circle `}},
		{"use and image can fetch or reference external content",
			`<svg viewBox="0 0 10 10"><use href="https://evil.test/x.svg#a"/><image href="https://evil.test/t.png"/><rect width="1" height="1"/></svg>`,
			[]string{"<use", "<image", "evil.test"}, []string{`<rect `}},
		{"external paint server",
			`<svg viewBox="0 0 10 10"><rect width="1" height="1" fill="url(https://evil.test/p.svg#x)" stroke="url(//evil.test/q)"/></svg>`,
			[]string{"evil.test", "url("}, []string{`<rect `}},
		{"style element and attribute",
			`<svg viewBox="0 0 10 10"><style>rect{fill:red}</style><rect width="1" height="1" style="fill:url(https://evil.test/x)"/></svg>`,
			[]string{"<style", "style=", "evil.test", "rect{"}, []string{`<rect `}},
		{"html inside foreignObject is still policed",
			`<svg viewBox="0 0 10 10"><foreignObject width="5" height="5"><iframe src="https://evil.test"></iframe><p onclick="x()">t</p></foreignObject></svg>`,
			[]string{"iframe", "onclick", "evil.test"}, []string{`<p>t</p>`}},
		{"details cannot carry handlers",
			`<details open ontoggle="alert(1)"><summary onclick="alert(2)">s</summary>b</details>`,
			[]string{"ontoggle", "onclick", "alert"}, []string{`<summary>s</summary>`}},
		{"non-checkbox input is dropped",
			`<p><input type="text" value="x"><input><input type="password" name="p"><input type="checkbox" disabled checked></p>`,
			[]string{`type="text"`, "password", `value=`, "name="}, []string{`type="checkbox"`}},
		{"input cannot change form target",
			`<input type="checkbox" formaction="https://evil.test" form="f" disabled>`,
			[]string{"formaction", "evil.test", "form="}, []string{`type="checkbox"`}},
		{"aside link scheme",
			`<aside class="callout"><a href="javascript:alert(1)">x</a><a href="data:text/html,<script>1</script>">y</a></aside>`,
			[]string{"javascript", "<script"}, []string{`<aside class="callout">`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeModel(tc.in)
			for _, b := range tc.banned {
				if strings.Contains(out, b) {
					t.Errorf("SanitizeModel(%q)\n = %q\nmust not contain %q", tc.in, out, b)
				}
			}
			for _, k := range tc.kept {
				if !strings.Contains(out, k) {
					t.Errorf("SanitizeModel(%q)\n = %q\nwant it to keep %q", tc.in, out, k)
				}
			}
		})
	}
}

// TestModelPolicyIsASupersetOfBase proves the extension only ADDS: whatever
// the base Policy keeps, ModelPolicy keeps byte for byte. The inline-SVG
// container chain press.Render depends on must survive unchanged.
func TestModelPolicyIsASupersetOfBase(t *testing.T) {
	in := `<div class="marpit"><svg data-marpit-svg="" viewBox="0 0 1280 720"><foreignObject width="1280" height="720">` +
		`<section id="1" data-marpit-pagination="1"><h1 id="t">T</h1><p><strong>b</strong> <code>c</code></p>` +
		`<table><thead><tr><th>A</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table></section></foreignObject></svg></div>`
	if got, want := SanitizeModel(in), Sanitize(in); got != want {
		t.Errorf("SanitizeModel diverged from Sanitize on base markup:\n got:  %q\n want: %q", got, want)
	}
}

// TestSanitizeModelIdempotent: sanitizing already-sanitized output changes
// nothing, so a fragment can safely pass through more than one stage.
func TestSanitizeModelIdempotent(t *testing.T) {
	for _, tc := range v7Fragments {
		once := SanitizeModel(tc.in)
		if twice := SanitizeModel(once); twice != once {
			t.Errorf("%s: not idempotent:\n once:  %q\n twice: %q", tc.name, once, twice)
		}
	}
}
