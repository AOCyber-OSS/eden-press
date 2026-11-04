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

package pdf

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/chase/profile"
	"go.aocyber.ai/eden-press/chase/theme"
	"go.aocyber.ai/eden-press/convert/chrome"
	"go.aocyber.ai/eden-press/press"
)

// Options is convert/pdf's exporter-specific knob surface. It is kept
// separate from convert.Options (the knob surface chrome.New consumes at
// Session-construction time) because ToPDF receives an ALREADY-BUILT
// *chrome.Session -- BrowserPath has nothing left to do by the time ToPDF
// runs. It is intentionally empty today: no named consumer has needed a
// PDF-specific knob yet, and it exists as a distinct type (not a bare
// struct{} inlined into ToPDF's signature) purely so a future knob (e.g. a
// print scale override) can be added without changing ToPDF's call
// signature.
type Options struct{}

// ToPDF exports out -- an already-rendered deck, typically press.Render's
// Output but equally a hand-built equivalent for testing -- to a
// deterministic PDF (see the package doc comment for the precise
// determinism bar this claims).
//
// Flow:
//  1. Resolve the page/slide size from out.Meta's "size" front-matter
//     directive, looked up in the size table of the profile that ACTUALLY
//     produced out (out.Profile), falling back to THAT profile's default
//     when the directive is absent or unknown. An Output with no recorded
//     profile is an error, not a slides fallback.
//  2. Compose a self-contained HTML document: out.HTML wrapped in a
//     <style> combining chrome.ComposeCSS(out.CSS) (base CSS + the
//     animation-kill override + the embedded STIX font) with
//     an @page rule (the one ToPDF's WithPreferCSSPageSize(true) call
//     honors; see composeDocument for the slides and paged forms).
//  3. Open a fresh tab on sess, apply the 05-02 ApplyDeterminism recipe
//     pinned to size's pixel dimensions (then, for a paged document, emulate
//     print media; see printMedia), then load the composed document via
//     chrome.LoadHTML (SetDocumentContent, not a data: URL or file://).
//  4. Run page.PrintToPDF INSIDE a chromedp.ActionFunc -- it is not a
//     first-class chromedp.Action; Do(ctx) returns three values (data,
//     stream, err) -- with WithPrintBackground(true),
//     WithPreferCSSPageSize(true), and all four margins pinned to 0.
//
// The returned []byte is the raw PDF payload (PrintToPDF's default
// TransferMode returns it inline as base64-decoded bytes, which is fine for
// slide-deck-sized output; a large-document streaming path is out of
// scope here).
func ToPDF(sess *chrome.Session, out press.Output, opts Options) ([]byte, error) {
	size, err := resolveSize(out)
	if err != nil {
		return nil, err
	}

	tab, cancel := sess.NewTab()
	defer cancel()

	if err := chrome.ApplyDeterminism(tab, int64(size.WidthPx), int64(size.HeightPx)); err != nil {
		return nil, fmt.Errorf("convert/pdf: ToPDF: %w", err)
	}

	// ApplyDeterminism emulates screen media (it is shared with the PNG path).
	// A paged document is laid out for PAPER: its @media print rules start
	// every section on a new page and hand the vertical margin to @page so a
	// long section breaks across pages on margins. Emulate print for it, keeping
	// the reduced-motion feature. Slides keep the screen media they always had.
	if printMedia(out) {
		err := chromedp.Run(tab, chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetEmulatedMedia().
				WithMedia("print").
				WithFeatures([]*emulation.MediaFeature{
					{Name: "prefers-reduced-motion", Value: "reduce"},
				}).
				Do(ctx)
		}))
		if err != nil {
			return nil, fmt.Errorf("convert/pdf: ToPDF: emulating print media: %w", err)
		}
	}

	if err := chrome.LoadHTML(tab, composeDocument(out, size)); err != nil {
		return nil, fmt.Errorf("convert/pdf: ToPDF: %w", err)
	}

	var data []byte
	err = chromedp.Run(tab, chromedp.ActionFunc(func(ctx context.Context) error {
		// PrintToPDF returns (data, stream, err) -- three values, so it is
		// NOT a first-class chromedp.Action and MUST be invoked from
		// inside an ActionFunc like this one (05-RESEARCH Pattern 1).
		pdfData, _, printErr := page.PrintToPDF().
			WithPrintBackground(true).
			WithPreferCSSPageSize(true).
			WithMarginTop(0).
			WithMarginBottom(0).
			WithMarginLeft(0).
			WithMarginRight(0).
			Do(ctx)
		if printErr != nil {
			return printErr
		}
		data = pdfData
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("convert/pdf: ToPDF: PrintToPDF: %w", err)
	}

	return data, nil
}

// resolveSize resolves the page/slide size ToPDF renders at from the profile
// that ACTUALLY produced out (out.Profile, recorded by press.Render), rather
// than from a hardcoded table: out.Meta's "size" front-matter directive value
// (Output.Meta is press.Render's top-level alias for Model.Meta) looked up in
// THAT profile's own size table, falling back to THAT table's Default when the
// directive is absent or names a size the profile does not define.
//
// Before this, the table was profiles/slides' unconditionally, so an A4 paged
// document exported at 1280x720 -- discarding the entire reason the paged
// profile exists. Physical page geometry is that profile's whole point.
//
// An Output with no recorded profile is an ERROR, never a slides fallback. A
// silent fallback is exactly what made the original bug invisible: nobody sees
// 1280x720 and thinks "that was a guess".
func resolveSize(out press.Output) (theme.Size, error) {
	if out.Profile == "" {
		return theme.Size{}, fmt.Errorf(
			"convert/pdf: resolveSize: Output carries no Profile; re-render with a press version that records it")
	}
	p, ok := profile.Get(out.Profile)
	if !ok {
		return theme.Size{}, fmt.Errorf(
			"convert/pdf: resolveSize: unknown profile %q (import the profiles/%s package for its init side-effect)",
			out.Profile, out.Profile)
	}
	// The ONE size resolver, shared with the CSS packer and convert/png.
	return profile.ResolveSize(p, out.Meta.Directives), nil
}

// printLayoutCSS makes every inline-SVG slide occupy exactly one PDF page: the
// slide container (the profile's `div.<class> > svg`, or a bare top-level
// <svg>) becomes a block exactly one page box in size. Left inline, an <svg>
// sits on a text line whose descender space makes the line box taller than
// the page, and its width follows the body rather than the page.
//
// Older Chrome hid that overflow by shrinking the content to fit the page;
// Chrome 154+ prints at 100% under preferCSSPageSize, so it became extra pages
// (with only chrome.ComposeCSS's html/body margin reset, an inline-SVG deck of
// 3 slides printed 6 pages). Appended last so it wins over any base CSS.
func printLayoutCSS(out press.Output, size theme.Size) string {
	containers := "body>svg"
	if p, ok := profile.Get(out.Profile); ok && p.ContainerClass() != "" {
		containers += ",div." + p.ContainerClass() + ">svg"
	}
	return fmt.Sprintf("%s{display:block;width:%dpx;height:%dpx;}", containers, size.WidthPx, size.HeightPx)
}

// printMedia reports whether out prints under print media: true for a profile
// without an inline-SVG layer (paged), whose stylesheet is written for paper.
// Slides (and an Output whose profile is not registered) keep screen media.
func printMedia(out press.Output) bool {
	p, ok := profile.Get(out.Profile)
	return ok && !profile.UsesInlineSVG(p)
}

// composeDocument wraps out's rendered body-fragment HTML into a
// self-contained HTML document Chrome can load directly via
// page.SetDocumentContent: chrome.ComposeCSS(out.CSS) folds in the
// animation-kill override plus the embedded STIX font, and an @page rule
// drives WithPreferCSSPageSize(true)'s paper sizing.
//
// For slides that rule is chrome.PageCSSInches(size), which also zeroes the
// page margin: a slide is exactly one page box. A paged document gets the
// size-only chrome.PageSizeCSSInches(size) instead, because its scaffold's
// `@page` margin, earlier in out.CSS, is what puts every page of a section that
// breaks across pages on margins (D16); a margin here would override it.
func composeDocument(out press.Output, size theme.Size) string {
	pageRule := chrome.PageCSSInches(size)
	if printMedia(out) {
		pageRule = chrome.PageSizeCSSInches(size)
	}
	css := chrome.ComposeCSS(out.CSS) + pageRule + printLayoutCSS(out, size)

	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><style>`)
	b.WriteString(css)
	b.WriteString(`</style></head><body>`)
	b.WriteString(out.HTML)
	b.WriteString(`</body></html>`)
	return b.String()
}
