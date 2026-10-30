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
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkbookCreateApplyExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	book := filepath.Join(dir, "book.json")
	ops := filepath.Join(dir, "ops.json")
	xlsxOut := filepath.Join(dir, "book.xlsx")

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "create", "--title", "Tasks", "-o", book})
	if err := root.Execute(); err != nil {
		t.Fatalf("create: %v", err)
	}
	raw, err := os.ReadFile(book)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schema": "eden-press.workbook/v3"`) &&
		!strings.Contains(string(raw), `"schema":"eden-press.workbook/v3"`) {
		t.Fatalf("create schema: %s", raw)
	}

	if err := os.WriteFile(ops, []byte(`[
		{"op":"set_cell","sheet":"Sheet1","row":0,"column":0,"value":"Alpha"},
		{"op":"set_cell","sheet":"Sheet1","row":1,"column":0,"value":"Beta"},
		{"op":"ensure_row_id","sheet":"Sheet1","row":0,"rowId":"r_a"},
		{"op":"ensure_row_id","sheet":"Sheet1","row":1,"rowId":"r_b"},
		{"op":"set_column","sheet":"Sheet1","columnId":"rel","name":"Related","type":"relation"},
		{"op":"set_relation","sheet":"Sheet1","rowId":"r_a","columnId":"rel","rels":[{"rowId":"r_b"}]},
		{"op":"set_header_row","sheet":"Sheet1","row":0,"header":true},
		{"op":"set_align","sheet":"Sheet1","column":0,"align":"left"},
		{"op":"add_sheet","sheet":"Extra"},
		{"op":"rename_sheet","sheet":"Extra","newName":"More"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}

	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "apply", book, "--ops", ops, "-o", book})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply: %v", err)
	}

	raw, err = os.ReadFile(book)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if snap["schema"] != "eden-press.workbook/v3" {
		t.Fatalf("schema = %v", snap["schema"])
	}
	if !strings.Contains(string(raw), `"rowId": "r_b"`) && !strings.Contains(string(raw), `"rowId":"r_b"`) {
		t.Fatalf("missing relation: %s", raw)
	}
	if !strings.Contains(string(raw), `"More"`) {
		t.Fatalf("missing renamed sheet: %s", raw)
	}

	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "export", book, "--format", "xlsx", "-o", xlsxOut})
	if err := root.Execute(); err != nil {
		t.Fatalf("export xlsx: %v", err)
	}
	data, err := os.ReadFile(xlsxOut)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatalf("not a zip: %x", data[:2])
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("xlsx missing [Content_Types].xml")
	}

	// Re-import JSON path and get
	root = newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "get", book})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out.String(), "eden-press.workbook/v3") {
		t.Fatalf("get: %s", out.String())
	}
}

func TestWorkbookImportXLSXRoundTrip(t *testing.T) {
	dir := t.TempDir()
	book := filepath.Join(dir, "book.json")
	xlsxPath := filepath.Join(dir, "round.xlsx")
	reimport := filepath.Join(dir, "again.json")

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "create", "-o", book})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	ops := filepath.Join(dir, "ops.json")
	if err := os.WriteFile(ops, []byte(`[
		{"op":"set_cell","sheet":"Sheet1","row":0,"column":0,"value":"Hi"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "apply", book, "--ops", ops, "-o", book})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "export", book, "-o", xlsxPath})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root = newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"workbook", "import", xlsxPath, "-o", reimport})
	if err := root.Execute(); err != nil {
		t.Fatalf("import: %v", err)
	}
	raw, err := os.ReadFile(reimport)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Hi") {
		t.Fatalf("reimport: %s", raw)
	}
	if !strings.Contains(string(raw), `"id"`) {
		t.Fatalf("import must ensure row ids: %s", raw)
	}
}
