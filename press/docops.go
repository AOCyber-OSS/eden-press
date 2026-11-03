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
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"go.aocyber.ai/eden-press/chase/model"
)

// NewDocJSON returns a fresh empty docmodel page as JSON (create_page for
// agents). Title is stored in model.meta.directives.title when non-empty.
// The CLI imports only press/, so agents reach chase/model through these
// byte-oriented helpers rather than the model types directly.
func NewDocJSON(title string) ([]byte, error) {
	return json.MarshalIndent(model.NewDocument(title), "", "  ")
}

// ApplyOpsJSON applies an ops batch to a docmodel JSON document and returns
// the updated document JSON. opsJSON may be a bare array of ops or an
// object {"ops":[...]}.
func ApplyOpsJSON(docJSON, opsJSON []byte) ([]byte, error) {
	return ApplyOpsJSONOpts(docJSON, opsJSON, false)
}

// ApplyOpsJSONOpts is ApplyOpsJSON with optional dry-run (validate without
// mutating; on success returns the unchanged input document indented).
func ApplyOpsJSONOpts(docJSON, opsJSON []byte, dryRun bool) ([]byte, error) {
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("press.ApplyOpsJSON: parse document: %w", err)
	}
	ops, err := parseOpsJSON(opsJSON)
	if err != nil {
		return nil, fmt.Errorf("press.ApplyOpsJSON: parse ops: %w", err)
	}
	if err := model.ApplyOpsWithOptions(&doc, ops, model.ApplyOpsOptions{DryRun: dryRun}); err != nil {
		return nil, err
	}
	if dryRun {
		// Re-parse original so the caller sees an unchanged document.
		var orig model.Document
		_ = json.Unmarshal(docJSON, &orig)
		return json.MarshalIndent(&orig, "", "  ")
	}
	return json.MarshalIndent(&doc, "", "  ")
}

// ImportMarkdownModelJSON renders md through press.Render and returns only
// the docmodel JSON (no html/css envelope). Block ids are assigned so the
// result is immediately editable via ApplyOps.
func ImportMarkdownModelJSON(md []byte, opts Options) ([]byte, error) {
	out, err := Render(string(md), opts)
	if err != nil {
		return nil, fmt.Errorf("press.ImportMarkdownModelJSON: %w", err)
	}
	if out.Model == nil {
		return nil, fmt.Errorf("press.ImportMarkdownModelJSON: render returned nil model")
	}
	model.EnsureBlockIDs(out.Model)
	model.EnsureListItemIDs(out.Model)
	out.Model.SchemaVersion = model.SchemaVersion
	return json.MarshalIndent(out.Model, "", "  ")
}

// GetDocJSON validates docJSON as a Document, ensures block ids, and returns
// indented JSON.
func GetDocJSON(docJSON []byte) ([]byte, error) {
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("press.GetDocJSON: %w", err)
	}
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = model.SchemaVersion
	}
	model.EnsureBlockIDs(&doc)
	model.EnsureListItemIDs(&doc)
	model.EnsureTextRunsInvariant(&doc)
	return json.MarshalIndent(&doc, "", "  ")
}

// ExportMarkdownJSON converts a docmodel JSON document to Marp-compatible
// Markdown bytes.
func ExportMarkdownJSON(docJSON []byte) ([]byte, error) {
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("press.ExportMarkdownJSON: %w", err)
	}
	return []byte(model.ToMarkdown(&doc)), nil
}

// MarkdownImport selects how MarkdownToDocJSON reads Markdown.
type MarkdownImport struct {
	// Lenient applies the documented mapping for a construct the document model
	// cannot carry exactly (the model.Construct* constants) instead of failing.
	Lenient bool

	// Spans sets a byte span on every block, section and outline entry. A span
	// indexes the Markdown string given, so compute it against the same string the
	// editor shows (see MarkdownSourceViewJSON).
	Spans bool
}

// MarkdownToDocJSON reads Markdown into docmodel JSON with the lossless codec
// (model.FromMarkdown), not press.Render: nothing goes through the HTML
// sanitizer, and nothing is dropped. It is the import AODex's editor and its
// migration use; ImportMarkdownModelJSON (the press.Render path behind
// `eden-press doc import`) is unchanged.
//
// prevDoc is the stored document the Markdown was exported from, or nil. When
// given, a block that survives the edit keeps its id (and a comment keeps its
// anchor); without it ids come from the Markdown's own eden:model trailer. Ids a
// block still lacks are then assigned, as ImportMarkdownModelJSON does, so the
// result is immediately editable via ApplyOps.
//
// In the default (strict) mode a construct the model cannot carry exactly is an
// error that wraps *model.UnrepresentableError: use errors.As to read its
// Construct and byte Offset.
func MarkdownToDocJSON(md string, prevDoc []byte, opt MarkdownImport) ([]byte, error) {
	var prev *model.Document
	if len(bytes.TrimSpace(prevDoc)) > 0 {
		prev = &model.Document{}
		if err := json.Unmarshal(prevDoc, prev); err != nil {
			return nil, fmt.Errorf("press.MarkdownToDocJSON: parse previous document: %w", err)
		}
	}
	doc, err := model.FromMarkdown(md, model.FromMarkdownOptions{Prev: prev, Spans: opt.Spans, Lenient: opt.Lenient})
	if err != nil {
		return nil, fmt.Errorf("press.MarkdownToDocJSON: %w", err)
	}
	model.EnsureBlockIDs(doc)
	model.EnsureListItemIDs(doc)
	return json.MarshalIndent(doc, "", "  ")
}

// VerifyMarkdownRoundTripJSON reports whether a stored document survives being
// written as Markdown and read back: ToMarkdown, then strict FromMarkdown with the
// document as Prev, then a comparison in which ids count and spans do not. It
// returns nil when the document is identical, and otherwise an error naming the
// first differing JSON path (sections[0].blocks[2].text) with both values, or
// wrapping the *model.UnrepresentableError when the Markdown it wrote is refused.
//
// A document with no ids is compared as if it had the ones EnsureBlockIDs would
// give it, so it is not blamed for ids it never had. It is an AODex migration's
// own guard: the migration never compares documents itself.
func VerifyMarkdownRoundTripJSON(docJSON []byte) error {
	var stored model.Document
	if err := json.Unmarshal(docJSON, &stored); err != nil {
		return fmt.Errorf("press.VerifyMarkdownRoundTripJSON: parse document: %w", err)
	}
	if stored.SchemaVersion == "" {
		stored.SchemaVersion = model.SchemaVersion
	}
	model.EnsureBlockIDs(&stored)
	model.EnsureListItemIDs(&stored)
	clearSpans(&stored)

	md := model.ToMarkdown(&stored)
	back, err := model.FromMarkdown(md, model.FromMarkdownOptions{Prev: &stored})
	if err != nil {
		return fmt.Errorf("press.VerifyMarkdownRoundTripJSON: the Markdown written for the document is refused when read back: %w", err)
	}
	clearSpans(back)

	want, err := jsonTree(&stored)
	if err != nil {
		return fmt.Errorf("press.VerifyMarkdownRoundTripJSON: %w", err)
	}
	got, err := jsonTree(back)
	if err != nil {
		return fmt.Errorf("press.VerifyMarkdownRoundTripJSON: %w", err)
	}
	if path, a, b := firstJSONDiff("", want, got); path != "" {
		return fmt.Errorf("press.VerifyMarkdownRoundTripJSON: the document does not survive a Markdown round trip at %s: stored %s, read back %s", path, a, b)
	}
	return nil
}

// MarkdownSourceViewJSON returns the Markdown an editor shows for a stored
// document, and its outline with spans into that same string, from one pass:
// md is exactly what ExportMarkdownJSON returns, and md[o.Span.Start:o.Span.Stop]
// is the text of heading o. Spans are byte offsets; a UTF-16 client converts
// them. The Markdown includes the eden:model trailer comment when the document
// has ids, properties or comments; keep it in the view or re-attach it before
// MarkdownToDocJSON, or the ids and comments are lost.
func MarkdownSourceViewJSON(docJSON []byte) (md string, outline []model.OutlineEntry, err error) {
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return "", nil, fmt.Errorf("press.MarkdownSourceViewJSON: parse document: %w", err)
	}
	md = model.ToMarkdown(&doc)
	back, err := model.FromMarkdown(md, model.FromMarkdownOptions{Spans: true, Prev: &doc})
	if err != nil {
		return "", nil, fmt.Errorf("press.MarkdownSourceViewJSON: the Markdown written for the document is refused when read back: %w", err)
	}
	return md, back.Outline, nil
}

// clearSpans removes every span from d: a span depends on the Markdown it was
// read from and is never part of a document's identity.
func clearSpans(d *model.Document) {
	var walk func(bs []model.Block)
	walk = func(bs []model.Block) {
		for i := range bs {
			bs[i].Span = nil
			walk(bs[i].Children)
		}
	}
	for i := range d.Sections {
		d.Sections[i].Span = nil
		walk(d.Sections[i].Blocks)
	}
	for i := range d.Outline {
		d.Outline[i].Span = nil
	}
}

// jsonTree is d as the generic tree encoding/json decodes, with every empty array
// and object (and null) dropped, and a table's all-empty "aligns": an empty
// collection and an absent one are the same JSON document for this comparison.
func jsonTree(d *model.Document) (any, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return dropEmpty(tree), nil
}

func dropEmpty(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if e = dropEmpty(e); e == nil || (k == "aligns" && allEmptyStrings(e)) {
				delete(x, k)
			} else {
				x[k] = e
			}
		}
		if len(x) == 0 {
			return nil
		}
		return x
	case []any:
		for i := range x {
			x[i] = dropEmptyKeep(x[i])
		}
		if len(x) == 0 {
			return nil
		}
		return x
	}
	return v
}

// allEmptyStrings reports whether v is an array of only empty strings. A table with
// no "aligns" and one with an empty alignment per column are the same table: the
// reader writes the second form, an ops batch often leaves the first.
func allEmptyStrings(v any) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, e := range list {
		if s, ok := e.(string); !ok || s != "" {
			return false
		}
	}
	return true
}

// dropEmptyKeep is dropEmpty for an array element, which must stay in place: an
// element that is itself empty stays as an empty object or array.
func dropEmptyKeep(v any) any {
	switch x := v.(type) {
	case map[string]any:
		if out := dropEmpty(x); out != nil {
			return out
		}
		return map[string]any{}
	case []any:
		if out := dropEmpty(x); out != nil {
			return out
		}
		return []any{}
	}
	return v
}

// firstJSONDiff walks two generic JSON trees in a fixed order (object keys sorted,
// arrays by index) and returns the path of the first difference with both values
// rendered; the path is "" when they are equal.
func firstJSONDiff(path string, a, b any) (string, string, string) {
	if reflect.DeepEqual(a, b) {
		return "", "", ""
	}
	at := func(sub string) string {
		if path == "" {
			return sub
		}
		return path + "." + sub
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			break
		}
		keys := make([]string, 0, len(x)+len(y))
		for k := range x {
			keys = append(keys, k)
		}
		for k := range y {
			if _, dup := x[k]; !dup {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if p, va, vb := firstJSONDiff(at(k), x[k], y[k]); p != "" {
				return p, va, vb
			}
		}
		return "", "", ""
	case []any:
		y, ok := b.([]any)
		if !ok {
			break
		}
		for i := 0; i < len(x) && i < len(y); i++ {
			if p, va, vb := firstJSONDiff(path+"["+strconv.Itoa(i)+"]", x[i], y[i]); p != "" {
				return p, va, vb
			}
		}
		if len(x) != len(y) {
			return path + "[" + strconv.Itoa(min(len(x), len(y))) + "]", describeJSON(elem(x, len(y))), describeJSON(elem(y, len(x)))
		}
		return "", "", ""
	}
	if path == "" {
		path = "document"
	}
	return path, describeJSON(a), describeJSON(b)
}

// elem is s[i], or nil (absent) when s is shorter.
func elem(s []any, i int) any {
	if i < len(s) {
		return s[i]
	}
	return nil
}

// describeJSON renders a value for an error message: JSON, cut to 80 bytes, or
// "absent" for nil.
func describeJSON(v any) string {
	if v == nil {
		return "absent"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	if s := string(raw); len(s) <= 80 {
		return s
	} else {
		return s[:77] + "..."
	}
}

// ExportHTMLJSON converts a docmodel JSON document to a standalone zero-JS
// HTML document (flowing article markup + DocsCSS). Browsers / HTML clients
// use this path; Flutter clients should request JSON and paint with the
// bind/eden_docs package instead.
func ExportHTMLJSON(docJSON []byte) ([]byte, error) {
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("press.ExportHTMLJSON: %w", err)
	}
	return []byte(AssembleDocsHTML(&doc)), nil
}

// AssembleDocsHTML wraps model.ToHTML in a complete standalone document with
// DocsCSS. Title comes from meta.directives.title when present.
func AssembleDocsHTML(doc *model.Document) string {
	title := ""
	if doc != nil && doc.Meta.Directives != nil {
		title = doc.Meta.Directives["title"]
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	if title != "" {
		b.WriteString("<title>")
		b.WriteString(html.EscapeString(title))
		b.WriteString("</title>\n")
	}
	b.WriteString("<style>\n")
	b.WriteString(model.DocsCSS)
	b.WriteString("\n</style>\n</head>\n<body>\n")
	b.WriteString(model.ToHTML(doc))
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

type opsEnvelope struct {
	Ops []model.Op `json:"ops"`
}

func parseOpsJSON(raw []byte) ([]model.Op, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty ops")
	}
	if raw[0] == '[' {
		var ops []model.Op
		if err := json.Unmarshal(raw, &ops); err != nil {
			return nil, err
		}
		return ops, nil
	}
	var env opsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return env.Ops, nil
}
