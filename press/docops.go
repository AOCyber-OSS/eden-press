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
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("press.ApplyOpsJSON: parse document: %w", err)
	}
	ops, err := parseOpsJSON(opsJSON)
	if err != nil {
		return nil, fmt.Errorf("press.ApplyOpsJSON: parse ops: %w", err)
	}
	if err := model.ApplyOps(&doc, ops); err != nil {
		return nil, err
	}
	return json.MarshalIndent(&doc, "", "  ")
}

// ImportMarkdownModelJSON renders md through press.Render and returns only
// the docmodel JSON (no html/css envelope). This is the agent import_markdown
// path: prose in, structured Document out.
func ImportMarkdownModelJSON(md []byte, opts Options) ([]byte, error) {
	out, err := Render(string(md), opts)
	if err != nil {
		return nil, fmt.Errorf("press.ImportMarkdownModelJSON: %w", err)
	}
	if out.Model == nil {
		return nil, fmt.Errorf("press.ImportMarkdownModelJSON: render returned nil model")
	}
	return json.MarshalIndent(out.Model, "", "  ")
}

// GetDocJSON validates docJSON as a Document and returns indented JSON.
// Useful as a read/normalize step for agents.
func GetDocJSON(docJSON []byte) ([]byte, error) {
	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("press.GetDocJSON: %w", err)
	}
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = model.SchemaVersion
	}
	return json.MarshalIndent(&doc, "", "  ")
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
