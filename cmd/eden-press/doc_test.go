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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocAgentCRUDFlow is the spike proof that an agent can create, edit,
// and read a docmodel page entirely through the CLI surface (no MCP yet).
func TestDocAgentCRUDFlow(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "page.json")
	ops := filepath.Join(dir, "ops.json")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "create", "--title", "Spike", "-o", page})
	if err := root.Execute(); err != nil {
		t.Fatalf("create: %v", err)
	}

	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["schemaVersion"] != "eden-press.model/v6" {
		t.Fatalf("schemaVersion = %v", doc["schemaVersion"])
	}

	if err := os.WriteFile(ops, []byte(`[
		{"op":"insert_block","sectionId":1,"block":{"id":"b_title","kind":"heading","level":1,"text":"Alpha"}},
		{"op":"insert_block","sectionId":1,"block":{"id":"b_p","kind":"paragraph","text":"Body"}},
		{"op":"update_block","blockId":"b_title","block":{"id":"b_title","kind":"heading","level":1,"text":"Q3 plan"}}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}

	root = newRootCmd()
	out.Reset()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "apply", page, "--ops", ops, "-o", page})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}

	raw, err = os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"Q3 plan"`) {
		t.Fatalf("apply did not update title: %s", raw)
	}
	if !strings.Contains(string(raw), `"Body"`) {
		t.Fatalf("apply missing body: %s", raw)
	}

	root = newRootCmd()
	out.Reset()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "get", page})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out.String(), `"Q3 plan"`) {
		t.Fatalf("get output: %s", out.String())
	}
}

func TestDocImportMarkdown(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "deck.md")
	outPath := filepath.Join(dir, "deck.json")
	if err := os.WriteFile(mdPath, []byte("# Title\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "import", mdPath, "-o", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("import: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schemaVersion": "eden-press.model/v6"`) &&
		!strings.Contains(string(raw), `"schemaVersion":"eden-press.model/v6"`) {
		t.Fatalf("import JSON: %s", raw)
	}
	if !strings.Contains(string(raw), "Title") {
		t.Fatalf("missing Title: %s", raw)
	}
}

func TestDocImportApplyExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "deck.md")
	page := filepath.Join(dir, "page.json")
	ops := filepath.Join(dir, "ops.json")
	outMD := filepath.Join(dir, "out.md")
	if err := os.WriteFile(mdPath, []byte("---\ntheme: default\n---\n\n# Title\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "import", mdPath, "-o", page})
	if err := root.Execute(); err != nil {
		t.Fatalf("import: %v", err)
	}
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"id"`) {
		t.Fatalf("import must assign block ids: %s", raw)
	}

	if err := os.WriteFile(ops, []byte(`[
		{"op":"insert_block","sectionId":1,"block":{"kind":"toggle","text":"More","children":[
			{"kind":"paragraph","text":"Nested"}
		]}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"code","language":"go","text":"fmt.Println(1)"}}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "apply", page, "--ops", ops, "-o", page})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}

	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doc", "export", page, "-o", outMD})
	if err := root.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}
	md, err := os.ReadFile(outMD)
	if err != nil {
		t.Fatal(err)
	}
	got := string(md)
	if !strings.Contains(got, "# Title") || !strings.Contains(got, "<details") || !strings.Contains(got, "```go") {
		t.Fatalf("export md = %s", got)
	}
}

func TestDocApplyMissingOpsIsUsageError(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "page.json")
	root := newRootCmd()
	var stderr bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"doc", "create", "-o", page})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = newRootCmd()
	stderr.Reset()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"doc", "apply", page})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != exitUsage {
		t.Fatalf("want usage cliError, got %v", err)
	}
	if !strings.Contains(stderr.String(), `"code": 2`) && !strings.Contains(stderr.String(), `"code":2`) {
		t.Fatalf("stderr JSON: %s", stderr.String())
	}
}
