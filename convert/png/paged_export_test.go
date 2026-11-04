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

package png

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/chase/profile"
	"go.aocyber.ai/eden-press/convert/chrome"
	"go.aocyber.ai/eden-press/convert/pdf"
	"go.aocyber.ai/eden-press/press"
)

// D15 (TRD 1026-27): the `paged` profile exports as real pages. These tests are
// Chrome-gated (newTestSession skips cleanly without a Chrome), and put the
// whole path under measurement: press.Render / press.RenderModel -> packed CSS
// -> ToImages / ToPDF -> pixels.

// pagedExportMD is a three-page document whose middle page is EMPTY: the case
// that screenshotted as 0px tall ("Cannot take screenshot with 0 height").
const pagedExportMD = "# First\n\nalpha\n\n---\n\n---\n\n# Third\n\nomega\n"

func pagedExportModel(directives map[string]string) *model.Document {
	d := &model.Document{SchemaVersion: model.SchemaVersion, Sections: []model.Section{
		{ID: 1, Blocks: []model.Block{
			{Kind: model.BlockHeading, Level: 1, Text: "First"},
			{Kind: model.BlockParagraph, Text: "alpha"},
		}},
		{ID: 2},
		{ID: 3, Blocks: []model.Block{
			{Kind: model.BlockHeading, Level: 1, Text: "Third"},
			{Kind: model.BlockParagraph, Text: "omega"},
		}},
	}}
	d.Meta.Directives = directives
	return d
}

// pagedFrontMatter renders directives as a Markdown front-matter block.
func pagedFrontMatter(directives map[string]string) string {
	if len(directives) == 0 {
		return ""
	}
	keys := make([]string, 0, len(directives))
	for k := range directives {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range keys {
		b.WriteString(k + ": " + directives[k] + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// pagedEntries are the two public render paths; both must export identically.
var pagedEntries = []struct {
	name   string
	render func(t *testing.T, directives map[string]string) press.Output
}{
	{"Render", func(t *testing.T, directives map[string]string) press.Output {
		t.Helper()
		out, err := press.Render(pagedFrontMatter(directives)+pagedExportMD, press.Options{Profile: "paged"})
		if err != nil {
			t.Fatalf("press.Render: %v", err)
		}
		return out
	}},
	{"RenderModel", func(t *testing.T, directives map[string]string) press.Output {
		t.Helper()
		out, err := press.RenderModel(pagedExportModel(directives), press.Options{Profile: "paged"})
		if err != nil {
			t.Fatalf("press.RenderModel: %v", err)
		}
		return out
	}},
}

// exportTimeout bounds one export. A selector that matches nothing makes
// chromedp.Screenshot wait for the node forever, so without a bound a regression
// of THIS bug hangs the whole test binary instead of failing the test.
const exportTimeout = 45 * time.Second

// toImagesWithin runs ToImages under exportTimeout. On timeout the call is
// abandoned; the deferred sess.Close() in the caller cancels the browser context,
// which unblocks the leaked goroutine.
func toImagesWithin(t *testing.T, sess *chrome.Session, out press.Output, opts Options) [][]byte {
	t.Helper()
	type result struct {
		imgs [][]byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		imgs, err := ToImages(sess, out, opts)
		ch <- result{imgs, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("ToImages(%+v): %v", opts, r.err)
		}
		return r.imgs
	case <-time.After(exportTimeout):
		t.Fatalf("ToImages(%+v) did not return within %v: its selector matches nothing in the markup", opts, exportTimeout)
		return nil
	}
}

func imageSize(t *testing.T, buf []byte) (w, h int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("decoding screenshot: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

// TestPagedExportGeometry (Test list 5): ToImages returns one image per section
// at EXACTLY the profile's page size -- including the empty section -- for both
// render entry points and both exporter options. A4 by default, the named size
// when a `size` directive picks one.
func TestPagedExportGeometry(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	for _, size := range []struct {
		name       string
		directives map[string]string
		w, h       int
	}{
		{"default A4", nil, 794, 1123},
		{"size letter", map[string]string{"size": "letter"}, 816, 1056},
	} {
		for _, ep := range pagedEntries {
			for _, opts := range []Options{{}, {InlineSVG: true}} {
				t.Run(fmt.Sprintf("%s/%s/InlineSVG=%v", size.name, ep.name, opts.InlineSVG), func(t *testing.T) {
					out := ep.render(t, size.directives)
					imgs := toImagesWithin(t, sess, out, opts)
					if len(imgs) != 3 {
						t.Fatalf("len(images) = %d, want 3 (one per section, the empty one included)", len(imgs))
					}
					for i, buf := range imgs {
						if w, h := imageSize(t, buf); w != size.w || h != size.h {
							t.Errorf("page %d is %dx%d px, want %dx%d", i+1, w, h, size.w, size.h)
						}
					}
				})
			}
		}
	}
}

// TestUnitSelector (Test list 4, no Chrome): the selector ToImages screenshots
// page k with. The paged profile has no SVG layer, so its pages are addressed
// as direct children of the container under Options{} AND Options{InlineSVG:
// true} -- the caller's flag cannot point the exporter at a wrapper the markup
// does not have. Slides keeps its <svg> selector, and (new) also gets it under
// Options{} when its markup carries the wrapper, which every press.Render slides
// output does.
func TestUnitSelector(t *testing.T) {
	paged, err := press.Render(pagedExportMD, press.Options{Profile: "paged"})
	if err != nil {
		t.Fatalf("press.Render(paged): %v", err)
	}
	slides, err := press.Render("# a\n\n---\n\n# b\n", press.Options{})
	if err != nil {
		t.Fatalf("press.Render(slides): %v", err)
	}
	if !strings.Contains(slides.HTML, "<svg data-marpit-svg") {
		t.Fatalf("precondition: press.Render slides output is not SVG-wrapped: %s", slides.HTML)
	}
	// A hand-built plain-mode slides output (what the plain fixture tests use).
	plain := press.Output{Profile: "slides", HTML: `<div class="marpit"><section></section><section></section></div>`}

	const (
		pagedSel = "div.edenpress-paged > section:nth-of-type(%d)"
		svgSel   = "div.marpit > svg:nth-of-type(%d) > foreignObject > section"
		plainSel = "div.marpit > section:nth-of-type(%d)"
	)
	for _, tc := range []struct {
		name string
		out  press.Output
		opts Options
		k    int
		want string
	}{
		{"paged, Options{}", paged, Options{}, 2, fmt.Sprintf(pagedSel, 2)},
		{"paged, Options{InlineSVG}", paged, Options{InlineSVG: true}, 3, fmt.Sprintf(pagedSel, 3)},
		{"slides, Options{InlineSVG}", slides, Options{InlineSVG: true}, 1, fmt.Sprintf(svgSel, 1)},
		{"slides SVG markup, Options{}", slides, Options{}, 2, fmt.Sprintf(svgSel, 2)},
		{"slides plain markup, Options{}", plain, Options{}, 2, fmt.Sprintf(plainSel, 2)},
		{"slides plain markup, Options{InlineSVG}", plain, Options{InlineSVG: true}, 1, fmt.Sprintf(svgSel, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := profile.Get(tc.out.Profile)
			if !ok {
				t.Fatalf("profile %q not registered", tc.out.Profile)
			}
			if got := unitSelector(p, tc.out, tc.opts, tc.k); got != tc.want {
				t.Errorf("unitSelector = %q, want %q", got, tc.want)
			}
		})
	}
}

var (
	pdfPageRE     = regexp.MustCompile(`/Type\s*/Page\b`)
	pdfMediaBoxRE = regexp.MustCompile(`/MediaBox\s*\[\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\]`)
)

// pdfGeometry returns the number of /Type /Page objects in a PDF and the first
// page's MediaBox width and height in points.
func pdfGeometry(t *testing.T, data []byte) (pages int, wpt, hpt float64) {
	t.Helper()
	pages = len(pdfPageRE.FindAll(data, -1))
	m := pdfMediaBoxRE.FindSubmatch(data)
	if m == nil {
		t.Fatalf("no /MediaBox in the PDF (%d bytes)", len(data))
	}
	f := func(b []byte) float64 {
		v, err := strconv.ParseFloat(string(b), 64)
		if err != nil {
			t.Fatalf("MediaBox number %q: %v", b, err)
		}
		return v
	}
	return pages, f(m[3]) - f(m[1]), f(m[4]) - f(m[2])
}

// TestPagedPDFOnePagePerSection (Test list 6): ToPDF prints one page per
// section, on the profile's paper (A4 = 595x842 pt by default, Letter 612x792
// pt for `size: letter`).
func TestPagedPDFOnePagePerSection(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	for _, size := range []struct {
		name       string
		directives map[string]string
		wpt, hpt   float64
	}{
		{"default A4", nil, 595, 842},
		{"size letter", map[string]string{"size": "letter"}, 612, 792},
	} {
		for _, ep := range pagedEntries {
			t.Run(size.name+"/"+ep.name, func(t *testing.T) {
				data, err := pdf.ToPDF(sess, ep.render(t, size.directives), pdf.Options{})
				if err != nil {
					t.Fatalf("ToPDF: %v", err)
				}
				pages, w, h := pdfGeometry(t, data)
				if pages != 3 {
					t.Errorf("PDF has %d pages, want 3 (one per section)", pages)
				}
				if abs(w-size.wpt) > 2 || abs(h-size.hpt) > 2 {
					t.Errorf("PDF MediaBox is %.1fx%.1f pt, want %.0fx%.0f (+-2)", w, h, size.wpt, size.hpt)
				}
			})
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// longSectionMD is ONE section of paragraphs (no `---`), the shape most AODex
// documents have, ending in a sentinel paragraph.
func longSectionMD(paragraphs int) string {
	const filler = "The quarterly review covered revenue, churn, hiring and the roadmap for the " +
		"next two releases, and the team agreed to revisit the pricing model before the end of the year. "
	var b strings.Builder
	b.WriteString("# Long report\n\n")
	for i := 1; i <= paragraphs; i++ {
		fmt.Fprintf(&b, "Paragraph %d. %s\n\n", i, filler)
	}
	b.WriteString("SENTINEL-END-OF-DOCUMENT\n")
	return b.String()
}

// longSectionMetrics are the numbers the characterization reads out of Chrome.
type longSectionMetrics struct {
	PageHeight     float64 `json:"pageHeight"`     // the section's border box
	ClientHeight   float64 `json:"clientHeight"`   // the clip edge (overflow: hidden)
	ScrollHeight   float64 `json:"scrollHeight"`   // the content's natural height
	SentinelTop    float64 `json:"sentinelTop"`    // relative to the section's top
	SentinelBottom float64 `json:"sentinelBottom"` // relative to the section's top
}

const longSectionJS = `(() => {
  const s = document.querySelector('div.edenpress-paged > section');
  const r = s.getBoundingClientRect();
  const ps = s.querySelectorAll('p');
  const q = ps[ps.length - 1].getBoundingClientRect();
  return JSON.stringify({pageHeight: r.height, clientHeight: s.clientHeight, scrollHeight: s.scrollHeight,
    sentinelTop: q.top - r.top, sentinelBottom: q.bottom - r.top});
})()`

// measureLongSection loads out the way ToImages does and reads the geometry
// under the given media ("screen" for PNG, "print" for PDF).
func measureLongSection(t *testing.T, sess *chrome.Session, out press.Output, media string) longSectionMetrics {
	t.Helper()
	tab, cancel := sess.NewTab()
	defer cancel()
	ctx, cancelT := context.WithTimeout(tab, exportTimeout)
	defer cancelT()

	if err := chrome.ApplyDeterminism(ctx, 794, 1123); err != nil {
		t.Fatalf("ApplyDeterminism: %v", err)
	}
	doc := "<!doctype html><html><head><meta charset=\"utf-8\"><style>" +
		chrome.ComposeCSS(out.CSS) + "</style></head><body>" + out.HTML + "</body></html>"
	if err := chrome.LoadHTML(ctx, doc); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	var raw string
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetEmulatedMedia().WithMedia(media).Do(ctx)
		}),
		chromedp.Evaluate(longSectionJS, &raw),
	); err != nil {
		t.Fatalf("measuring (%s): %v", media, err)
	}
	var m longSectionMetrics
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decoding metrics %q: %v", raw, err)
	}
	return m
}

// TestPagedLongSectionCharacterization (Test list 7): ONE section holding about
// three pages of paragraphs. It records what the page box does with content that
// does not fit, so the operator can decide before the tag. It asserts only what
// the measurement shows.
func TestPagedLongSectionCharacterization(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	out, err := press.Render(longSectionMD(longSectionParagraphs), press.Options{Profile: "paged"})
	if err != nil {
		t.Fatalf("press.Render: %v", err)
	}
	if len(out.Model.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(out.Model.Sections))
	}

	screen := measureLongSection(t, sess, out, "screen")
	printed := measureLongSection(t, sess, out, "print")

	imgs := toImagesWithin(t, sess, out, Options{})
	if len(imgs) != 1 {
		t.Fatalf("len(images) = %d, want 1", len(imgs))
	}
	_, pngH := imageSize(t, imgs[0])

	data, err := pdf.ToPDF(sess, out, pdf.Options{})
	if err != nil {
		t.Fatalf("ToPDF: %v", err)
	}
	pages, _, _ := pdfGeometry(t, data)

	t.Logf("LONG SECTION (%d paragraphs, 1 section): content is %.0f px tall = %.2f A4 pages (1123 px)",
		longSectionParagraphs, screen.ScrollHeight, screen.ScrollHeight/1123)
	t.Logf("  PNG height: %d px; PDF pages: %d", pngH, pages)
	t.Logf("  screen: page box %.0f px, clip edge %.0f px, sentinel paragraph spans %.0f..%.0f px from the page top",
		screen.PageHeight, screen.ClientHeight, screen.SentinelTop, screen.SentinelBottom)
	t.Logf("  print:  page box %.0f px, clip edge %.0f px, sentinel paragraph spans %.0f..%.0f px from the page top",
		printed.PageHeight, printed.ClientHeight, printed.SentinelTop, printed.SentinelBottom)

	// What the measurement shows, pinned so the behaviour is visible and a
	// change to it is deliberate. The scaffold gives every section a fixed page
	// box (height: A4, overflow: hidden) and one section is one page, so content
	// past the first page is CLIPPED, not flowed onto more pages: a long
	// single-section document exports as ONE page showing its first A4 worth.
	// If a product decision makes long sections flow, this is the test to change.
	if screen.ScrollHeight < 2.5*1123 {
		t.Fatalf("content is only %.0f px tall (%.2f pages); the sample is not a multi-page section", screen.ScrollHeight, screen.ScrollHeight/1123)
	}
	if pngH != 1123 {
		t.Errorf("PNG height = %d, want 1123 (the page box, not the content)", pngH)
	}
	if pages != 1 {
		t.Errorf("PDF pages = %d, want 1 (a fixed page box does not paginate its overflow)", pages)
	}
	for name, m := range map[string]longSectionMetrics{"screen": screen, "print": printed} {
		if m.SentinelTop < m.ClientHeight {
			t.Errorf("%s: the last paragraph starts at %.0f px, inside the %.0f px page box; content is no longer clipped", name, m.SentinelTop, m.ClientHeight)
		}
	}
}

// longSectionParagraphs is sized so the section holds roughly three A4 pages of
// content under the bundled default theme.
const longSectionParagraphs = 17
