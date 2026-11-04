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

package press_test

import (
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/press"
)

// D17 (TRD 1026-30): a paged document with no theme gets a DOCUMENT theme (an
// 11pt body, a heading scale, the scaffold's 20 mm margins), not the Marp
// slides `default` theme (29px type, 78.5px padding). Any explicit theme still
// wins, and slides keep the Marp default.

// unitDecl reports whether some rule scoped exactly to the paged unit declares
// prop: value.
func unitDecl(css, prop, value string) bool {
	for _, r := range parseCSSRules(css) {
		if r.targets(pagedUnit) && r.Decls[prop] == value {
			return true
		}
	}
	return false
}

// customDocumentTheme is a caller theme that happens to be named `document`,
// as AODex's synthesized theme for a row named "Document" would be.
const customDocumentTheme = "/* @theme document */\nsection { color: #123456; }\n"

func TestPagedThemeResolution(t *testing.T) {
	for _, ep := range pagedEntryPoints {
		for _, profile := range []string{"paged", "article"} {
			prefix := ep.name + "/" + profile + "/"

			t.Run(prefix+"no theme is the document theme", func(t *testing.T) {
				out := ep.render(t, press.Options{Profile: profile}, nil)
				if !unitDecl(out.CSS, "font-size", "11pt") {
					t.Errorf("no paged unit rule sets font-size: 11pt; the document theme is not the default")
				}
				for _, slide := range []string{"font-size: 29px", "padding: 78.5px"} {
					if strings.Contains(out.CSS, slide) {
						t.Errorf("packed CSS carries the slides theme's %q", slide)
					}
				}
			})

			t.Run(prefix+"Options.Theme default wins", func(t *testing.T) {
				out := ep.render(t, press.Options{Profile: profile, Theme: "default"}, nil)
				if !unitDecl(out.CSS, "font-size", "29px") {
					t.Errorf("Theme: default did not select the Marp default theme (no 29px unit rule)")
				}
				if unitDecl(out.CSS, "font-size", "11pt") {
					t.Errorf("Theme: default still carries the document theme")
				}
			})

			t.Run(prefix+"theme directive gaia wins", func(t *testing.T) {
				out := ep.render(t, press.Options{Profile: profile}, map[string]string{"theme": "gaia"})
				if !unitDecl(out.CSS, "font-size", "35px") {
					t.Errorf("the theme directive gaia did not select gaia (no 35px unit rule)")
				}
				if unitDecl(out.CSS, "font-size", "11pt") {
					t.Errorf("the theme directive gaia still carries the document theme")
				}
			})

			t.Run(prefix+"caller ThemeCSS named document replaces the built-in", func(t *testing.T) {
				for _, opts := range []press.Options{
					{Profile: profile, Theme: "document", ThemeCSS: []string{customDocumentTheme}},
					{Profile: profile, ThemeCSS: []string{customDocumentTheme}},
				} {
					out := ep.render(t, opts, nil)
					if !strings.Contains(out.CSS, "#123456") {
						t.Errorf("Theme %q: the caller's `document` theme was not used", opts.Theme)
					}
					if strings.Contains(out.CSS, "11pt") {
						t.Errorf("Theme %q: the built-in document theme survived the caller's theme of the same name", opts.Theme)
					}
				}
			})
		}
	}
}

func TestPagedUnknownThemeStillErrors(t *testing.T) {
	const want = `unknown theme "no-such-theme"`
	for _, profile := range []string{"paged", "article"} {
		opts := press.Options{Profile: profile, Theme: "no-such-theme"}
		if _, err := press.Render(pagedThreeMD, opts); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Render(%s): err = %v, want it to contain %s", profile, err, want)
		}
		if _, err := press.RenderModel(pagedThreeModel(nil), opts); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("RenderModel(%s): err = %v, want it to contain %s", profile, err, want)
		}
	}
}

// TestSlidesDefaultThemeUnchanged: slides with no theme still pack the Marp
// `default` theme, byte for byte the golden TestSlidesOutputUnchangedByPagedFix
// pins for an explicit Theme: default.
func TestSlidesDefaultThemeUnchanged(t *testing.T) {
	for _, opts := range []press.Options{{}, {Profile: "slides"}} {
		r, err := press.Render(slidesGoldenMD, opts)
		if err != nil {
			t.Fatalf("Render(%+v): %v", opts, err)
		}
		m, err := press.RenderModel(slidesGoldenModel(), opts)
		if err != nil {
			t.Fatalf("RenderModel(%+v): %v", opts, err)
		}
		if got, want := sha(r.CSS), slidesGolden["Render/default"].css; got != want {
			t.Errorf("Render(%+v) slides CSS sha256 %s, want the default-theme golden %s", opts, got, want)
		}
		if got, want := sha(m.CSS), slidesGolden["RenderModel/default"].css; got != want {
			t.Errorf("RenderModel(%+v) slides CSS sha256 %s, want the default-theme golden %s", opts, got, want)
		}
		if strings.Contains(r.CSS, "11pt") {
			t.Errorf("slides CSS carries the paged document theme")
		}
	}
}
