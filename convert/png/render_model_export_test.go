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
	"image"
	"image/png"
	"regexp"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/convert/pdf"
	"go.aocyber.ai/eden-press/press"
)

// exportPageRE matches a PDF `/Type /Page` leaf object (never the `/Type
// /Pages` tree root): the same structural page count convert/pdf's own tests
// and the export capstone use.
var exportPageRE = regexp.MustCompile(`/Type\s*/Page\b`)

// renderModelExportDoc is a hand-built v7 document: a text/todo page, a page
// of the kinds that only exist in the model (callout, sheet, figure), and an
// EMPTY page. Unit count drives page count downstream, so the empty section
// is the case that matters most.
func renderModelExportDoc(emptyThird bool) *model.Document {
	d := &model.Document{
		SchemaVersion: model.SchemaVersion,
		Sections: []model.Section{
			{ID: 1, Blocks: []model.Block{
				{Kind: model.BlockHeading, Level: 1, Text: "Q3 review"},
				{Kind: model.BlockParagraph, Text: "Revenue is up."},
				{Kind: model.BlockTodo, Text: "Send the deck", Checked: true},
			}},
			{ID: 2, Blocks: []model.Block{
				{Kind: model.BlockCallout, Text: "Watch churn", Tone: "warn", Icon: "!"},
				{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "Budget", Snapshot: &model.SheetSnapshot{Cells: []model.SheetCellSnap{
					{R: 0, C: 0, V: "Total"}, {R: 0, C: 1, V: "42", F: "SUM(B2:B9)"},
				}}}},
				{Kind: model.BlockFigure, Text: "Mark", SVG: `<svg viewBox="0 0 40 40" width="40" height="40"><rect width="40" height="40" fill="#d4a853"/></svg>`},
			}},
			{ID: 3},
		},
	}
	if !emptyThird {
		d.Sections[2].Blocks = []model.Block{{Kind: model.BlockParagraph, Text: "Closing page."}}
	}
	return d
}

// TestRenderModelExport is Test-list case 7: the raster exporters accept
// press.RenderModel output unchanged. ToImages must return one image per
// Model section (it addresses each unit by position, so one extra or missing
// <section> would shift every later page), at the profile's own page size,
// and ToPDF must produce a PDF with one page per section.
//
// Chrome-gated: newTestSession skips cleanly when no Chrome is discoverable.
func TestRenderModelExport(t *testing.T) {
	sess := newTestSession(t)
	defer sess.Close()

	// KNOWN LIMITATION, not caused by RenderModel (press.Render on Markdown
	// shows it identically): the paged profile's CSS is scoped to
	// "div.edenpress-paged > section", but the Render seam always wraps each
	// section in <svg><foreignObject>, so none of the paged page-box rules
	// match. A paged page is therefore as tall as its content rather than A4,
	// and an EMPTY paged section is 0px tall -- chromedp refuses to screenshot
	// it. So the paged case below uses non-empty sections and asserts the page
	// COUNT (what RenderModel controls), not the A4 geometry; the slides case
	// keeps the empty section and the exact size.
	for _, tc := range []struct {
		profile    string
		w, h       int // asserted only when exactSize
		exactSize  bool
		emptyThird bool
	}{
		{"slides", 1280, 720, true, true},
		{"paged", 794, 1123, false, false},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			doc := renderModelExportDoc(tc.emptyThird)
			out, err := press.RenderModel(doc, press.Options{Profile: tc.profile})
			if err != nil {
				t.Fatalf("press.RenderModel: %v", err)
			}
			if out.Model == nil || len(out.Model.Sections) != len(doc.Sections) {
				t.Fatalf("Output.Model has %v sections, want %d", out.Model, len(doc.Sections))
			}

			images, err := ToImages(sess, out, Options{InlineSVG: true})
			if err != nil {
				t.Fatalf("ToImages: %v", err)
			}
			if len(images) != len(doc.Sections) {
				t.Fatalf("len(images) = %d, want %d (== len(doc.Sections))", len(images), len(doc.Sections))
			}
			decoded := make([]image.Image, len(images))
			for i, buf := range images {
				img, err := png.Decode(bytes.NewReader(buf))
				if err != nil {
					t.Fatalf("page %d: decoding PNG: %v", i+1, err)
				}
				if got := img.Bounds(); got.Dx() != tc.w || (tc.exactSize && got.Dy() != tc.h) {
					t.Errorf("page %d: bounds = %v, want width %d (height %d when exact)", i+1, got, tc.w, tc.h)
				}
				decoded[i] = img
			}
			// Pages carry different content, so their pixels differ: a render
			// that captured the same unit three times (or only blank ones)
			// would be byte-identical.
			if bytes.Equal(images[0], images[2]) {
				t.Error("page 1 (text) and page 3 (empty) are byte-identical: content not rendered")
			}
			if bytes.Equal(images[0], images[1]) {
				t.Error("page 1 and page 2 are byte-identical: pages are not selected by position")
			}

			pdfBytes, err := pdf.ToPDF(sess, out, pdf.Options{})
			if err != nil {
				t.Fatalf("pdf.ToPDF: %v", err)
			}
			if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
				t.Fatalf("not a PDF: first bytes %q", pdfBytes[:min(16, len(pdfBytes))])
			}
			if got := len(exportPageRE.FindAll(pdfBytes, -1)); got != len(doc.Sections) {
				t.Errorf("PDF pages = %d, want %d (== len(doc.Sections))", got, len(doc.Sections))
			}
		})
	}
}
