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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocV7FullFeatureCLIRoundTrip(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "page.json")
	ops := filepath.Join(dir, "ops.json")
	outMD := filepath.Join(dir, "out.md")
	badOps := filepath.Join(dir, "bad.json")

	run := func(args ...string) error {
		root := newRootCmd()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)
		return root.Execute()
	}

	if err := run("doc", "create", "--title", "Full", "-o", page); err != nil {
		t.Fatalf("create: %v", err)
	}

	payload := `[
		{"op":"insert_block","sectionId":1,"block":{"id":"h","kind":"heading","level":1,"text":"Plan","runs":[{"text":"Plan","marks":["bold"]}]}},
		{"op":"insert_block","sectionId":1,"block":{"id":"todo","kind":"todo","text":"Write tests","checked":false}},
		{"op":"insert_block","sectionId":1,"block":{"id":"call","kind":"callout","text":"Important","icon":"!","tone":"info"}},
		{"op":"insert_block","sectionId":1,"block":{"id":"div","kind":"divider"}},
		{"op":"insert_block","sectionId":1,"block":{"id":"file","kind":"file","src":"spec.pdf","name":"Spec","mime":"application/pdf","size":12}},
		{"op":"insert_block","sectionId":1,"block":{"id":"fig","kind":"figure","svg":"<svg/>","text":"mark"}},
		{"op":"insert_block","sectionId":1,"block":{"id":"sh","kind":"sheet","sheetEmbed":{
			"sheet":"Tasks","pageId":"p1","range":{"r0":0,"r1":1,"c0":0,"c1":1,"columnIds":["title"]},
			"snapshot":{"columns":[{"id":"title","name":"Title","type":"text"}],"cells":[{"r":0,"c":0,"v":"A"},{"r":0,"c":1,"v":"B"},{"r":1,"c":0,"v":"1"},{"r":1,"c":1,"v":"2"}]}
		}}},
		{"op":"set_properties","properties":[
			{"id":"status","name":"Status","type":"select","options":["todo","done"]},
			{"id":"due","name":"Due","type":"date"}
		]},
		{"op":"set_property_value","propertyId":"status","value":"todo"},
		{"op":"set_property_value","propertyId":"due","value":"2026-11-01"},
		{"op":"add_comment","comment":{"blockId":"todo","body":"please","suggestion":{"text":"Write thorough tests"}}},
		{"op":"patch_block","blockId":"todo","block":{"checked":true}},
		{"op":"replace_text","old":"Plan","new":"Roadmap","blockId":"h"},
		{"op":"insert_text","blockId":"call","offset":0,"text":"Note: "}
	]`
	if err := os.WriteFile(ops, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run("doc", "apply", page, "--ops", ops, "-o", page); err != nil {
		t.Fatalf("apply: %v", err)
	}

	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["schemaVersion"] != "eden-press.model/v7" {
		t.Fatalf("schemaVersion=%v", doc["schemaVersion"])
	}
	props := doc["propertyValues"].(map[string]any)
	if props["status"] != "todo" || props["due"] != "2026-11-01" {
		t.Fatalf("propertyValues=%v", props)
	}
	comments := doc["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("comments=%v", comments)
	}
	got := string(raw)
	for _, want := range []string{`"kind": "todo"`, `"kind": "callout"`, `"kind": "divider"`, `"kind": "file"`, `"kind": "figure"`, `"kind": "sheet"`, `"sheetEmbed"`, `"Roadmap"`, `"Note: Important"`} {
		// JSON indent may space after colon
		compact := strings.ReplaceAll(want, `: "`, `:"`)
		if !strings.Contains(got, want) && !strings.Contains(got, compact) {
			// also try without space variants for nested text
			if want == `"Note: Important"` && !strings.Contains(got, "Note: Important") {
				t.Fatalf("missing %s in %s", want, got)
			} else if want != `"Note: Important"` && !strings.Contains(got, strings.Trim(want, `"`)) {
				t.Fatalf("missing %s in %s", want, got)
			}
		}
	}

	// apply suggestion via second ops batch
	cid := comments[0].(map[string]any)["id"].(string)
	sug := filepath.Join(dir, "sug.json")
	if err := os.WriteFile(sug, []byte(`[{"op":"apply_suggestion","commentId":"`+cid+`"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run("doc", "apply", page, "--ops", sug, "-o", page); err != nil {
		t.Fatalf("suggestion: %v", err)
	}
	raw, _ = os.ReadFile(page)
	if !strings.Contains(string(raw), "Write thorough tests") {
		t.Fatalf("suggestion not applied: %s", raw)
	}

	if err := run("doc", "export", page, "-o", outMD); err != nil {
		t.Fatalf("export: %v", err)
	}
	md, _ := os.ReadFile(outMD)
	mds := string(md)
	for _, want := range []string{"# Roadmap", "- [x] Write thorough tests", "Note: Important", "[Spec](spec.pdf)", "<svg/>", "| A | B |", "| 1 | 2 |"} {
		if !strings.Contains(mds, want) {
			t.Fatalf("md missing %q in:\n%s", want, mds)
		}
	}

	// dry-run failure stays non-mutating
	before, _ := os.ReadFile(page)
	if err := os.WriteFile(badOps, []byte(`[{"op":"replace_text","old":"ZZZ_NOT_FOUND","new":"nope"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd()
	var stderr bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"doc", "apply", page, "--ops", badOps, "--dry-run", "-o", page})
	if err := root.Execute(); err == nil {
		t.Fatal("expected dry-run failure")
	}
	after, _ := os.ReadFile(page)
	if !bytes.Equal(before, after) {
		t.Fatal("failed dry-run mutated page")
	}
	if !strings.Contains(stderr.String(), `"code"`) {
		t.Fatalf("stderr envelope: %s", stderr.String())
	}
}

func TestWorkbookV7NamedRangeAndSelectColumnCLI(t *testing.T) {
	dir := t.TempDir()
	book := filepath.Join(dir, "book.json")
	ops := filepath.Join(dir, "ops.json")

	run := func(args ...string) error {
		root := newRootCmd()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)
		return root.Execute()
	}
	if err := run("workbook", "create", "--title", "WB", "-o", book); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ops, []byte(`[
		{"op":"set_cell","sheet":"Sheet1","row":0,"column":0,"value":"A"},
		{"op":"set_column","sheet":"Sheet1","columnId":"status","name":"Status","type":"select","options":["todo","done"]},
		{"op":"set_column","sheet":"Sheet1","columnId":"due","name":"Due","type":"date"},
		{"op":"set_column","sheet":"Sheet1","columnId":"ok","name":"OK","type":"checkbox"},
		{"op":"set_named_range","rangeId":"nr1","name":"Top","sheet":"Sheet1","r0":0,"r1":2,"c0":0,"c1":1}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run("workbook", "apply", book, "--ops", ops, "-o", book); err != nil {
		t.Fatalf("apply: %v", err)
	}
	raw, _ := os.ReadFile(book)
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if snap["schema"] != "eden-press.workbook/v3" {
		t.Fatalf("schema=%v", snap["schema"])
	}
	nrs := snap["namedRanges"].([]any)
	if len(nrs) != 1 || nrs[0].(map[string]any)["id"] != "nr1" {
		t.Fatalf("namedRanges=%v", snap["namedRanges"])
	}
	sheets := snap["sheets"].([]any)
	cols := sheets[0].(map[string]any)["columns"].([]any)
	if len(cols) != 3 {
		t.Fatalf("columns=%v", cols)
	}
	foundSelect := false
	for _, c := range cols {
		m := c.(map[string]any)
		if m["id"] == "status" {
			foundSelect = true
			opts := m["options"].([]any)
			if len(opts) != 2 {
				t.Fatalf("options=%v", opts)
			}
		}
	}
	if !foundSelect {
		t.Fatal("select column missing")
	}
}
