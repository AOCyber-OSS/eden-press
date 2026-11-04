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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/convert"
	"go.aocyber.ai/eden-press/convert/chrome"
	"go.aocyber.ai/eden-press/press"

	// Blank imports: populate the chase/profile registry resolveSize looks
	// up out.Profile in. pdf.go itself imports NO profiles package -- that
	// is the point of this change, and these two lines are what a real
	// consumer's own import graph supplies (press blank-imports slides; a
	// paged consumer imports profiles/paged).
	_ "go.aocyber.ai/eden-press/profiles/paged"
	_ "go.aocyber.ai/eden-press/profiles/slides"
)

// --- shared fixtures & helpers ------------------------------------------

const (
	deckHTMLPath = "testdata/deck.html"
	deckCSSPath  = "testdata/deck.css"

	bgRuleStart = "/* bg-rule-start */"
	bgRuleEnd   = "/* bg-rule-end */"
)

// newTestSession is the shared Chrome-presence gate every test in this file
// opens with: t.Skip cleanly (never fail) when no Chrome/Chromium is
// discoverable, matching convert/chrome's own live-Chrome test pattern
// (session_test.go's TestSessionMultiTab, load_test.go's
// TestLoadHTMLReadsBackRealContent) -- this sandbox has none.
func newTestSession(t *testing.T) *chrome.Session {
	t.Helper()
	if _, _, err := chrome.Discover(chrome.DiscoverOptions{}); err != nil {
		t.Skipf("no Chrome discovered, skipping live-Chrome pdf test: %v", err)
	}
	sess, err := chrome.New(convert.Options{})
	if err != nil {
		t.Skipf("could not start a Chrome session, skipping live-Chrome pdf test: %v", err)
	}
	return sess
}

// readFixture reads a testdata file, FAILING the test (not skipping) on
// error -- a missing/unreadable fixture is a test-authoring bug, not a
// live-Chrome environment gap.
func readFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", path, err)
	}
	return string(b)
}

// fixtureOutput builds a hand-built press.Output from deck.html/deck.css,
// decoupled from press.Render (05-RESEARCH: "testable against hand-written
// static HTML fixtures"), plus a 3-section model.Document so every test's
// expected page count is pinned to len(out.Model.Sections) rather than a
// bare magic number.
func fixtureOutput(t *testing.T) press.Output {
	t.Helper()
	return press.Output{
		HTML: readFixture(t, deckHTMLPath),
		CSS:  readFixture(t, deckCSSPath),
		Model: &model.Document{
			SchemaVersion: model.SchemaVersion,
			Sections: []model.Section{
				{ID: 1}, {ID: 2}, {ID: 3},
			},
		},
		// A hand-built Output must record a Profile like a real
		// press.Render Output does -- resolveSize errors rather than
		// silently falling back to slides when it is absent.
		Profile: "slides",
	}
}

// stripBGRule deterministically removes the `.bg{...}` rule bracketed by
// deck.css's "bg-rule-start"/"bg-rule-end" markers -- a byte-slice, not a
// regeneration, so the "backgrounds stripped" variant stays hand-built and
// never drifts from deck.css's own text.
func stripBGRule(t *testing.T, css string) string {
	t.Helper()
	start := strings.Index(css, bgRuleStart)
	end := strings.Index(css, bgRuleEnd)
	if start == -1 || end == -1 {
		t.Fatalf("deck.css fixture: bg-rule markers not found (start=%d end=%d)", start, end)
	}
	return css[:start] + css[end+len(bgRuleEnd):]
}

// pageTypeRE matches a PDF `/Type /Page` LEAF object -- not `/Type /Pages`,
// the page-TREE root every PDF has exactly one of. The trailing \b asserts
// a non-word byte (or end of input) immediately after "Page": a word-word
// position is never a boundary, so the pattern never matches the "s" of
// "Pages" at all. This is a byte-scan structural proxy -- sufficient for
// these small fixtures (05-RESEARCH's own recommendation), not a
// general-purpose PDF parser.
var pageTypeRE = regexp.MustCompile(`/Type\s*/Page\b`)

func countPDFPages(data []byte) int {
	return len(pageTypeRE.FindAll(data, -1))
}

// mediaBoxRE captures a PDF `/MediaBox [x0 y0 x1 y1]` array's four numbers
// -- used to compare page dimensions STRUCTURALLY across two runs (Test
// list case 2), never against a fixed expected value here (that
// confirmation belongs to convert/chrome's own PageCSSInches unit tests).
var mediaBoxRE = regexp.MustCompile(`/MediaBox\s*\[\s*([\d.\-]+)\s+([\d.\-]+)\s+([\d.\-]+)\s+([\d.\-]+)\s*\]`)

func mediaBoxes(data []byte) []string {
	matches := mediaBoxRE.FindAllStringSubmatch(string(data), -1)
	boxes := make([]string, 0, len(matches))
	for _, m := range matches {
		boxes = append(boxes, strings.Join(m[1:], " "))
	}
	return boxes
}

// chromeProduct t.Logs the live Chrome/Chromium product string (e.g.
// "HeadlessChrome/120.0.6099.0") via CDP's Browser.getVersion -- feeds
// 05-05's version-pin + PDF-revalidation process (this TRD's error_recovery:
// "Record the Chrome version tested"). Returns "" (logging why) if the CDP
// call itself fails -- never fails the test on its own account.
func chromeProduct(t *testing.T, sess *chrome.Session) string {
	t.Helper()
	tab, cancel := sess.NewTab()
	defer cancel()

	var product string
	err := chromedp.Run(tab, chromedp.ActionFunc(func(ctx context.Context) error {
		_, p, _, _, _, versionErr := browser.GetVersion().Do(ctx)
		product = p
		return versionErr
	}))
	if err != nil {
		t.Logf("could not read Chrome version via Browser.getVersion: %v", err)
		return ""
	}
	return product
}

// --- Test list case 1 -----------------------------------------------------

// TestToPDFStructuralPageCount is Test-list case 1: ToPDF on the hand-built
// multi-slide fixture returns bytes starting with the PDF magic header and
// containing exactly one `/Type /Page` leaf object per fixture slide.
func TestToPDFStructuralPageCount(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	out := fixtureOutput(t)
	data, err := ToPDF(sess, out, Options{})
	if err != nil {
		t.Fatalf("ToPDF: %v", err)
	}

	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("ToPDF output does not start with the %%PDF- magic header: first bytes %q", data[:min(16, len(data))])
	}

	want := len(out.Model.Sections)
	if got := countPDFPages(data); got != want {
		t.Fatalf("countPDFPages = %d, want %d (fixture slide count)", got, want)
	}
}

// --- Test list case 2 -----------------------------------------------------

// TestToPDFDeterministicStructure is Test-list case 2: two ToPDF runs of the
// identical fixture must produce PDFs with identical page count AND
// identical page dimensions (structural equality). The real acceptance bar
// this stands in for is pixel-diff-under-threshold in a full image pipeline
// (05-RESEARCH Pitfall C -- Chrome has acknowledged PRNG-based rendering
// non-determinism) -- this test deliberately does NOT compare the two PDFs'
// raw bytes for equality.
func TestToPDFDeterministicStructure(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	out := fixtureOutput(t)

	data1, err := ToPDF(sess, out, Options{})
	if err != nil {
		t.Fatalf("ToPDF (run 1): %v", err)
	}
	data2, err := ToPDF(sess, out, Options{})
	if err != nil {
		t.Fatalf("ToPDF (run 2): %v", err)
	}

	if p1, p2 := countPDFPages(data1), countPDFPages(data2); p1 != p2 {
		t.Fatalf("page count differs across runs: run1=%d run2=%d", p1, p2)
	}

	boxes1, boxes2 := mediaBoxes(data1), mediaBoxes(data2)
	if len(boxes1) == 0 {
		t.Fatal("no /MediaBox entries found in run 1's PDF -- cannot assert dimension equality")
	}
	if len(boxes1) != len(boxes2) {
		t.Fatalf("MediaBox entry count differs across runs: run1=%d run2=%d", len(boxes1), len(boxes2))
	}
	for i := range boxes1 {
		if boxes1[i] != boxes2[i] {
			t.Fatalf("MediaBox[%d] differs across runs: run1=%q run2=%q", i, boxes1[i], boxes2[i])
		}
	}
}

// --- Test list case 3 -----------------------------------------------------

// TestToPDFPrintsBackgrounds is Test-list case 3: the fixture's solid slide
// background-color (deck.css's `.bg` rule) must survive into the printed
// PDF. ToPDF always sets WithPrintBackground(true), but CDP's own default is
// PrintBackground=false ("Print background graphics. Defaults to false."),
// so a regression that dropped that call would silently blank every theme
// background. Proven here by comparing the fixture's PDF byte size against
// the SAME fixture with the `.bg` rule removed -- with the background
// present, the PDF must come out materially larger.
func TestToPDFPrintsBackgrounds(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	withBG := fixtureOutput(t)

	noBG := withBG
	noBG.CSS = stripBGRule(t, withBG.CSS)

	dataWithBG, err := ToPDF(sess, withBG, Options{})
	if err != nil {
		t.Fatalf("ToPDF (with background): %v", err)
	}
	dataNoBG, err := ToPDF(sess, noBG, Options{})
	if err != nil {
		t.Fatalf("ToPDF (backgrounds stripped): %v", err)
	}

	if len(dataWithBG) <= len(dataNoBG) {
		t.Fatalf("expected the with-background PDF to be larger than the backgrounds-stripped one (WithPrintBackground working): withBG=%d bytes, noBG=%d bytes", len(dataWithBG), len(dataNoBG))
	}
}

// --- Test list case 4 -----------------------------------------------------

// inlineSVGFixtureHTML is a hand-built (no_llm_test_data) Pitfall-4/A
// regression fixture: each of deck.html's three slides re-expressed inside
// Marpit's own inline-SVG container shape (chase/profile's Container(true):
// "div.marpit > svg > foreignObject") -- one <svg> per slide, each wrapping
// exactly one <foreignObject> with an explicit width/height and one
// namespaced <section>. Both the foreignObject's explicit width/height AND
// the inner xmlns are REQUIRED: their absence is the documented
// silent-non-render trap -- a foreignObject that renders empty in the PDF
// path while an HTML string comparison would still pass.
const inlineSVGFixtureHTML = `<svg viewBox="0 0 1280 720"><foreignObject width="1280" height="720"><section xmlns="http://www.w3.org/1999/xhtml">
<h1>Welcome</h1>
<p>A hand-built multi-slide fixture for convert/pdf's structural tests.</p>
</section></foreignObject></svg>
<svg viewBox="0 0 1280 720"><foreignObject width="1280" height="720"><section xmlns="http://www.w3.org/1999/xhtml" class="bg">
<h1>Background Slide</h1>
<p>This slide carries a solid background-color via the .bg class in deck.css.</p>
</section></foreignObject></svg>
<svg viewBox="0 0 1280 720"><foreignObject width="1280" height="720"><section xmlns="http://www.w3.org/1999/xhtml">
<h1>Closing</h1>
<p>Three slides total -- the structural page-count baseline every ToPDF test in this package pins against.</p>
</section></foreignObject></svg>
`

// TestToPDFInlineSVGFixture is Test-list case 4 -- the PDF-path-only
// regression smoke for Pitfall A/4 (Chrome >=108 SVG-in-PDF issues,
// foreignObject fragility): rasterizing inlineSVGFixtureHTML must still
// produce a PDF with one leaf page per slide and a non-trivial byte size
// (not a blank/cropped page). The Chrome product string is t.Logged to feed
// 05-05's version-pin + PDF-revalidation process.
func TestToPDFInlineSVGFixture(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	if product := chromeProduct(t, sess); product != "" {
		t.Logf("inline-SVG PDF smoke tested against Chrome product: %s", product)
	}

	base := fixtureOutput(t)
	out := press.Output{
		HTML:    inlineSVGFixtureHTML,
		CSS:     base.CSS,
		Model:   base.Model,
		Profile: base.Profile,
	}

	data, err := ToPDF(sess, out, Options{})
	if err != nil {
		t.Fatalf("ToPDF (inline-SVG fixture): %v", err)
	}

	want := len(out.Model.Sections)
	if got := countPDFPages(data); got != want {
		t.Fatalf("countPDFPages = %d, want %d (fixture slide count) -- a foreignObject may have rendered empty/collapsed (Pitfall 4/A)", got, want)
	}

	const nonTrivialSize = 1024
	if len(data) < nonTrivialSize {
		t.Fatalf("inline-SVG PDF is only %d bytes -- suspiciously small, likely a blank page (Pitfall 4/A foreignObject silent-non-render trap)", len(data))
	}
}

// --- composeDocument: slides byte-identical, paged prints on margins ------
//
// D16 (TRD 1026-30): a paged section flows across as many pages as it needs,
// and every page, continuation pages included, needs a top and a bottom margin.
// That margin lives in the paged scaffold's `@page`, so the composed document
// must not re-zero it. Slides compose exactly as before.

// composeSlidesDeck is a slides deck through the inline-SVG layered path (an
// advanced background) with an EMPTY slide.
const composeSlidesDeck = "---\nmarp: true\npaginate: true\n---\n\n# Deck\n\nFirst slide.\n\n---\n\n![bg left](bg.png)\n\n## Second\n\n---\n\n---\n\n### Fourth\n"

// composeSlidesGolden are sha256 digests of composeDocument's output for slides
// Outputs, captured from the code BEFORE the D16 change (eden-press 22a0550). A
// paged change must not move a byte of the slides PDF input.
var composeSlidesGolden = map[string]string{
	"fixture":           "2d73f54183b955c2c095683a8e05d4565b1705254a924e4f9c4c08b54ac1bf6c",
	"Render/16:9":       "e3e2764cab2bce14b79291c6c77883029c9da2f72c7b7ee2949c84be13d8e2b1",
	"Render/4:3":        "3ef4ba9e2ef65219df54122dfbfea130634a0f7ca713f9696d5cbcd99da30ed8",
	"Render/theme gaia": "f36a65a24deabea6e7e419dc523d96345f5cedf9dca75e539c5e1e170a564346",
}

func TestComposeDocumentSlidesUnchanged(t *testing.T) {
	render := func(md string, opts press.Options) press.Output {
		t.Helper()
		out, err := press.Render(md, opts)
		if err != nil {
			t.Fatalf("press.Render: %v", err)
		}
		return out
	}
	for _, tc := range []struct {
		key string
		out press.Output
	}{
		{"fixture", fixtureOutput(t)},
		{"Render/16:9", render(composeSlidesDeck, press.Options{})},
		{"Render/4:3", render(strings.Replace(composeSlidesDeck, "paginate: true\n", "paginate: true\nsize: 4:3\n", 1), press.Options{})},
		{"Render/theme gaia", render(composeSlidesDeck, press.Options{Theme: "gaia"})},
	} {
		t.Run(tc.key, func(t *testing.T) {
			size, err := resolveSize(tc.out)
			if err != nil {
				t.Fatalf("resolveSize: %v", err)
			}
			sum := sha256.Sum256([]byte(composeDocument(tc.out, size)))
			if got, want := hex.EncodeToString(sum[:]), composeSlidesGolden[tc.key]; got != want {
				t.Errorf("slides composeDocument changed: sha256 %s, golden %s", got, want)
			}
		})
	}
}

// pageAtRuleRE captures one `@page { ... }` body; the composed document's
// @page rules are flat (no nested margin boxes).
var pageAtRuleRE = regexp.MustCompile(`@page\s*\{([^{}]*)\}`)

// pageDecls returns the declarations of each @page rule in css, in order.
func pageDecls(css string) []map[string]string {
	var out []map[string]string
	for _, m := range pageAtRuleRE.FindAllStringSubmatch(css, -1) {
		decls := map[string]string{}
		for _, d := range strings.Split(m[1], ";") {
			if name, value, ok := strings.Cut(d, ":"); ok {
				decls[strings.TrimSpace(name)] = strings.TrimSpace(value)
			}
		}
		out = append(out, decls)
	}
	return out
}

// styleOf returns the text of the composed document's one <style> element.
func styleOf(t *testing.T, doc string) string {
	t.Helper()
	_, rest, ok := strings.Cut(doc, "<style>")
	if !ok {
		t.Fatalf("composed document has no <style>")
	}
	css, _, ok := strings.Cut(rest, "</style>")
	if !ok {
		t.Fatalf("composed document's <style> is not closed")
	}
	return css
}

// TestComposeDocumentPagedKeepsPageMargin: for a paged Output, the @page rule
// the composer appends after the packed CSS states the paper SIZE and nothing
// else, so the scaffold's `@page { margin: 20mm 0; }` is the margin that applies.
func TestComposeDocumentPagedKeepsPageMargin(t *testing.T) {
	for _, tc := range []struct {
		name       string
		directives string
		size       string
	}{
		{"A4", "", "8.271in 11.698in"},
		{"letter", "---\nsize: letter\n---\n\n", "8.5in 11in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := press.Render(tc.directives+"# Report\n\nBody.\n", press.Options{Profile: "paged"})
			if err != nil {
				t.Fatalf("press.Render: %v", err)
			}
			size, err := resolveSize(out)
			if err != nil {
				t.Fatalf("resolveSize: %v", err)
			}
			css := styleOf(t, composeDocument(out, size))
			appended, ok := strings.CutPrefix(css, chrome.ComposeCSS(out.CSS))
			if !ok {
				t.Fatalf("composed CSS does not start with ComposeCSS(out.CSS)")
			}

			sized := false
			for _, d := range pageDecls(appended) {
				if m, ok := d["margin"]; ok {
					t.Errorf("the composer's @page re-states margin: %s; it must leave the scaffold's page margin alone", m)
				}
				if d["size"] == tc.size {
					sized = true
				}
			}
			if !sized {
				t.Errorf("no appended @page states size: %s (appended: %q)", tc.size, appended)
			}

			effective := ""
			for _, d := range pageDecls(css) {
				if m, ok := d["margin"]; ok {
					effective = m
				}
			}
			if effective != "20mm 0" {
				t.Errorf("effective @page margin = %q, want %q (the paged scaffold's)", effective, "20mm 0")
			}
		})
	}
}

// --- resolveSize: the Chrome-free proof --------------------------------

// TestResolveSize is where the real proof of this exporter's geometry lives:
// resolveSize is a pure (press.Output) -> (theme.Size, error) function, so the
// headline claim -- an A4 PAGED document exports at A4, not at the slides
// table's 1280x720 -- is provable with no Chrome at all. The Chrome-gated
// ToPDF tests above corroborate; they are not the proof, because they skip
// entirely wherever Chrome is absent.
//
// The paged/no-directive row is the bug this change closes. The
// paged/unknown-directive row is nearly as load-bearing: an unrecognized size
// name must fall back to the RESOLVED profile's default, never to slides'.
func TestResolveSize(t *testing.T) {
	out := func(profileID, sizeDirective string) press.Output {
		o := press.Output{Profile: profileID}
		if sizeDirective != "" {
			o.Meta = model.Meta{Directives: map[string]string{"size": sizeDirective}}
		}
		return o
	}

	cases := []struct {
		name             string
		out              press.Output
		wantW, wantH     int
		wantName         string
		wantErrSubstring string
	}{
		{name: "slides, no directive -> 16:9", out: out("slides", ""), wantW: 1280, wantH: 720, wantName: "16:9"},
		{name: "slides, 4:3 directive", out: out("slides", "4:3"), wantW: 960, wantH: 720, wantName: "4:3"},
		{name: "paged, no directive -> A4 (the headline case)", out: out("paged", ""), wantW: 794, wantH: 1123, wantName: "a4"},
		{name: "paged, letter directive", out: out("paged", "letter"), wantW: 816, wantH: 1056, wantName: "letter"},
		{name: "paged, a5 directive", out: out("paged", "a5"), wantW: 559, wantH: 794, wantName: "a5"},
		{name: "paged, unknown directive -> PAGED's default, not slides'", out: out("paged", "nonsense"), wantW: 794, wantH: 1123, wantName: "a4"},
		{name: "no recorded profile -> named error, never a silent slides fallback", out: out("", "16:9"), wantErrSubstring: "carries no Profile"},
		{name: "unregistered profile -> named error", out: out("epub", "16:9"), wantErrSubstring: `unknown profile "epub"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSize(tc.out)

			if tc.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("resolveSize(%+v) = %+v, want an error containing %q", tc.out, got, tc.wantErrSubstring)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Errorf("error = %q, want it to contain %q", err, tc.wantErrSubstring)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveSize(%+v): %v", tc.out, err)
			}
			if got.WidthPx != tc.wantW || got.HeightPx != tc.wantH {
				t.Errorf("resolveSize = %dx%d (%q), want %dx%d (%q)",
					got.WidthPx, got.HeightPx, got.Name, tc.wantW, tc.wantH, tc.wantName)
			}
			if got.Name != tc.wantName {
				t.Errorf("resolveSize name = %q, want %q", got.Name, tc.wantName)
			}
		})
	}
}
