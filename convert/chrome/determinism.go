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

package chrome

// determinism.go holds the SHARED determinism substrate: ComposeCSS +
// PageCSSInches (pure CSS transforms, no chromedp import) and
// ApplyDeterminism (the ordered live-Chrome CDP recipe). Both 05-03 (pdf) and
// 05-04 (png) call exactly these -- the recipe is never re-implemented in
// either exporter.
//
// IMPORTANT: "deterministic" here means pixel-diff-under-threshold, NOT
// byte-identical (05-RESEARCH Pitfall C) -- Chrome's own rendering pipeline
// has acknowledged PRNG-based non-determinism (font hinting/anti-aliasing,
// GPU-vs-software rasterization paths) that this recipe cannot fully
// eliminate, only minimize. Contrast with the pure-Go press/ path, which CAN
// claim byte-identical output for identical input.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/chase/theme"
)

// animationKillCSS is the guaranteed !important override ComposeCSS appends
// LAST (after the caller's base CSS) so it always wins the cascade,
// regardless of whether the packed theme CSS itself honors
// @media(prefers-reduced-motion) (Marp theme CSS does not -- 05-RESEARCH
// Pattern 2 item 4). convert/ fully controls the CSS fed to Chrome, so this
// is a more reliable determinism lever than trusting the theme.
const animationKillCSS = `*,*::before,*::after{animation:none!important;transition:none!important;scroll-behavior:auto!important;}`

// documentResetCSS zeroes the user-agent margin and padding of html and body
// in every exported document. The default 8px body margin otherwise offsets
// every slide: in a PDF, N one-page slides lay out 16px taller than N pages
// (an extra page once Chrome 154+ stopped shrinking content to fit), and an
// inline-SVG slide sized by its container screenshots at 1264x711 instead of
// the 1280x720 viewport.
const documentResetCSS = `html,body{margin:0;padding:0;}`

// ComposeCSS returns baseCSS with the animation/transition/scroll-behavior
// kill override, the html/body margin reset and the bundled STIX Two Math
// @font-face data-URI appended, in that order -- base first, then overrides
// LAST, so the overrides' !important declarations and later cascade
// position both win.
//
// This is a pure string transform: it performs no I/O and drives no Chrome
// tab, which is what makes it independently unit-testable without a live
// browser.
func ComposeCSS(baseCSS string) string {
	var b strings.Builder
	b.WriteString(baseCSS)
	b.WriteString(animationKillCSS)
	b.WriteString(documentResetCSS)
	b.WriteString(FontFaceDataURI())
	return b.String()
}

// pxPerInch is the CSS reference pixel density (96px = 1in) PageCSSInches
// converts a profiles/slides pixel size against.
const pxPerInch = 96.0

// PageCSSInches returns an `@page{size:<w>in <h>in;margin:0;}` rule for the
// given slide size, converting pixels to inches at 96px/in (the CSS
// reference-pixel definition). This is the PDF page-size mechanism 05-03
// pairs with chromedp's WithPreferCSSPageSize(true): 1280x720 (16:9) ->
// "13.333in 7.5in"; 960x720 (4:3) -> "10in 7.5in".
//
// Like ComposeCSS, this is a pure function -- no chromedp import, no I/O --
// unit-tested directly against the profiles/slides size table.
func PageCSSInches(size theme.Size) string {
	w := float64(size.WidthPx) / pxPerInch
	h := float64(size.HeightPx) / pxPerInch
	return fmt.Sprintf("@page{size:%sin %sin;margin:0;}", formatInches(w), formatInches(h))
}

// PageSizeCSSInches returns an `@page{size:<w>in <h>in;}` rule: PageCSSInches'
// paper size with NO margin declaration, so the page margin an earlier @page
// rule in the document sets still applies. convert/pdf uses it for a profile
// without an SVG layer (paged), whose scaffold carries the page margin that a
// section breaking across pages needs on every page; slides keep PageCSSInches.
func PageSizeCSSInches(size theme.Size) string {
	w := float64(size.WidthPx) / pxPerInch
	h := float64(size.HeightPx) / pxPerInch
	return fmt.Sprintf("@page{size:%sin %sin;}", formatInches(w), formatInches(h))
}

// formatInches renders an inch value to at most 3 decimal places, trimming
// trailing zeros (and a trailing bare "." if the value is a whole number) so
// 13.333333... -> "13.333", 7.5 -> "7.5", and 10.0 -> "10" -- matching the
// CSS @page size values a rendered PDF is verified against in 05-03.
func formatInches(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// ApplyDeterminism pins every source of Chrome-render variance on a tab
// BEFORE content loads: fixed viewport + device-scale-factor, UTC timezone,
// en-US locale, and reduced-motion media emulation (defense-in-depth
// alongside ComposeCSS's guaranteed CSS kill). This is the SINGLE shared
// recipe both 05-03 (pdf) and 05-04 (png) call -- it must never be
// re-implemented in either exporter.
//
// ctx must be a chromedp tab context (e.g. from Session.NewTab()). The
// actions run in order via one chromedp.Run so a failure part-way through
// (e.g. the tab closing) surfaces as a single wrapped error.
func ApplyDeterminism(ctx context.Context, viewportW, viewportH int64) error {
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(viewportW, viewportH, chromedp.EmulateScale(1.0)),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetTimezoneOverride("UTC").Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetLocaleOverride().WithLocale("en-US").Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetEmulatedMedia().
				WithMedia("screen").
				WithFeatures([]*emulation.MediaFeature{
					{Name: "prefers-reduced-motion", Value: "reduce"},
				}).
				Do(ctx)
		}),
	)
	if err != nil {
		return fmt.Errorf("convert/chrome: applying determinism recipe (viewport %dx%d): %w", viewportW, viewportH, err)
	}
	return nil
}
