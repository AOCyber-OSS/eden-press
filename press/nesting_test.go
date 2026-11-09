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

package press

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.aocyber.ai/eden-press/chase/model"
)

// The byte APIs and the nesting limit (eden-press#29). A guard on one path is
// not a fix: every function that decodes a stored document, and every one that
// reads Markdown, must hold the limit.
//
// Test list:
//
//	P1 every entry point that decodes a document refuses one nested one level
//	   past model.MaxNestingDepth with a *model.NestingDepthError
//	P2 the same entry points accept a document exactly at the limit
//	P3 MarkdownToDocJSON of '>'x4900: lenient is fast and under 1 MB; strict is
//	   an UnrepresentableError naming nesting-depth
//	P4 byte-API output grows linearly with depth (compact JSON: the bytes per
//	   level do not grow with the level)
//	P5 ImportMarkdownModelJSON (the press.Render path) refuses a list nested
//	   past the limit

// chainJSON is a one-section document whose blocks nest depth deep: depth-1
// quotes, each the only child of the one above, and a divider at the bottom.
// The leaf is a divider, not a paragraph, so the document is canonical: a
// quote's leading paragraph reads back as the quote's own text. Built
// bottom-up, so the generator never recurses.
func chainJSON(t *testing.T, depth int) []byte {
	t.Helper()
	b := model.Block{Kind: model.BlockDivider}
	for n := depth - 1; n >= 1; n-- {
		b = model.Block{Kind: model.BlockQuote, Text: fmt.Sprintf("l%d", n), Children: []model.Block{b}}
	}
	d := model.NewDocument("")
	d.Sections[0].Blocks = []model.Block{b}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// docEntryPoints is every press function that decodes a stored document.
var docEntryPoints = []struct {
	name string
	call func(doc []byte) error
}{
	{"GetDocJSON", func(d []byte) error { _, err := GetDocJSON(d); return err }},
	{"ApplyOpsJSONOpts", func(d []byte) error {
		_, err := ApplyOpsJSONOpts(d, []byte(`[{"op":"set_meta_directives","directives":{"k":"v"}}]`), false)
		return err
	}},
	{"ApplyOpsJSONOpts dry run", func(d []byte) error {
		_, err := ApplyOpsJSONOpts(d, []byte(`[{"op":"set_meta_directives","directives":{"k":"v"}}]`), true)
		return err
	}},
	{"ExportMarkdownJSON", func(d []byte) error { _, err := ExportMarkdownJSON(d); return err }},
	{"ExportHTMLJSON", func(d []byte) error { _, err := ExportHTMLJSON(d); return err }},
	{"MarkdownToDocJSON prevDoc", func(d []byte) error { _, err := MarkdownToDocJSON("x", d, MarkdownImport{}); return err }},
	{"VerifyMarkdownRoundTripJSON", func(d []byte) error { return VerifyMarkdownRoundTripJSON(d) }},
	{"MarkdownSourceViewJSON", func(d []byte) error { _, _, err := MarkdownSourceViewJSON(d); return err }},
	{"RenderModelJSON", func(d []byte) error { _, err := RenderModelJSON(d, Options{}); return err }},
	{"RenderModel", func(d []byte) error {
		var doc model.Document
		if err := json.Unmarshal(d, &doc); err != nil {
			return err
		}
		_, err := RenderModel(&doc, Options{})
		return err
	}},
}

func TestNesting_P1_EveryDocumentEntryPointRefusesPastTheLimit(t *testing.T) {
	deep := chainJSON(t, model.MaxNestingDepth+1)
	for _, ep := range docEntryPoints {
		t.Run(ep.name, func(t *testing.T) {
			var ne *model.NestingDepthError
			if err := ep.call(deep); !errors.As(err, &ne) {
				t.Fatalf("err = %v, want *model.NestingDepthError", err)
			}
		})
	}
}

func TestNesting_P2_EveryDocumentEntryPointAcceptsTheLimit(t *testing.T) {
	ok := chainJSON(t, model.MaxNestingDepth)
	for _, ep := range docEntryPoints {
		t.Run(ep.name, func(t *testing.T) {
			if err := ep.call(ok); err != nil {
				t.Fatalf("a document exactly %d deep is refused: %v", model.MaxNestingDepth, err)
			}
		})
	}
}

func TestNesting_P3_DeepQuoteMarkdown(t *testing.T) {
	md := strings.Repeat(">", 4900) + " x"
	best := time.Hour
	var out []byte
	for i := 0; i < 3; i++ { // best of three: one GC pause must not fail it
		start := time.Now()
		var err error
		if out, err = MarkdownToDocJSON(md, nil, MarkdownImport{Lenient: true}); err != nil {
			t.Fatal(err)
		}
		best = min(best, time.Since(start))
	}
	if len(out) >= 1<<20 {
		t.Fatalf("lenient output is %d bytes, want under 1 MB", len(out))
	}
	if !testing.Short() && best > 50*time.Millisecond {
		t.Fatalf("lenient MarkdownToDocJSON of 4900 quotes took %v, want under 50ms", best)
	}
	_, err := MarkdownToDocJSON(md, nil, MarkdownImport{})
	var ue *model.UnrepresentableError
	if !errors.As(err, &ue) || ue.Construct != model.ConstructNestingDepth {
		t.Fatalf("strict err = %v, want UnrepresentableError %q", err, model.ConstructNestingDepth)
	}
}

func TestNesting_P4_ByteAPIOutputGrowsLinearlyWithDepth(t *testing.T) {
	size := func(d int) int {
		out, err := GetDocJSON(chainJSON(t, d))
		if err != nil {
			t.Fatal(err)
		}
		return len(out)
	}
	base := size(1)
	per := func(d int) float64 { return float64(size(d)-base) / float64(d-1) }
	q, f := per(model.MaxNestingDepth/4), per(model.MaxNestingDepth)
	// Linear output adds the same bytes per level; indenting by depth makes
	// the deeper levels cost more (O(depth^2) in all).
	if f > 1.25*q {
		t.Fatalf("bytes per level: %.0f at depth %d, %.0f at depth %d: output is not linear in depth", q, model.MaxNestingDepth/4, f, model.MaxNestingDepth)
	}
}

func TestNesting_P5_ImportMarkdownModelJSONRefusesDeepLists(t *testing.T) {
	var md strings.Builder
	for k := 0; k <= model.MaxNestingDepth; k++ {
		fmt.Fprintf(&md, "%s- i%d\n", strings.Repeat("  ", k), k)
	}
	_, err := ImportMarkdownModelJSON([]byte(md.String()), Options{})
	var ne *model.NestingDepthError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want *model.NestingDepthError", err)
	}
}

// BenchmarkGetDocJSONDepth reports time and bytes per depth from 1 to the
// limit: go test ./press -run '^$' -bench GetDocJSONDepth. Both grow linearly.
func BenchmarkGetDocJSONDepth(b *testing.B) {
	for _, d := range []int{1, 8, 16, 32} {
		b.Run(fmt.Sprintf("depth=%d", d), func(b *testing.B) {
			t := &testing.T{}
			in := chainJSON(t, d)
			var n int
			for i := 0; i < b.N; i++ {
				out, err := GetDocJSON(in)
				if err != nil {
					b.Fatal(err)
				}
				n = len(out)
			}
			b.ReportMetric(float64(n), "bytes")
		})
	}
}
