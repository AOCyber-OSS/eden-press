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
