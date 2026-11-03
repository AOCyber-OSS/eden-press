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
	"strings"
	"testing"
)

func TestApplyOpsJSONOptsDryRunAndExportMarkdown(t *testing.T) {
	doc, err := NewDocJSON("T")
	if err != nil {
		t.Fatal(err)
	}
	ops := []byte(`[
		{"op":"insert_block","sectionId":1,"block":{"id":"p","kind":"paragraph","text":"Hello"}},
		{"op":"insert_block","sectionId":1,"block":{"id":"todo","kind":"todo","text":"X","checked":true}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"sheet","sheetEmbed":{"sheet":"S","snapshot":{"cells":[{"r":0,"c":0,"v":"A"}]}}}}
	]`)
	out, err := ApplyOpsJSONOpts(doc, ops, true)
	if err != nil {
		t.Fatalf("dry-run success: %v", err)
	}
	if strings.Contains(string(out), `"Hello"`) {
		// dry-run returns original empty doc — should NOT contain Hello
		t.Fatalf("dry-run returned mutated doc: %s", out)
	}
	out, err = ApplyOpsJSON(doc, ops)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatal(err)
	}
	if probe["schemaVersion"] != "eden-press.model/v7" {
		t.Fatalf("schema=%v", probe["schemaVersion"])
	}
	md, err := ExportMarkdownJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(md)
	if !strings.Contains(s, "Hello") || !strings.Contains(s, "- [x] X") || !strings.Contains(s, ":::sheet {\"sheetEmbed\":{\"sheet\":\"S\",\"snapshot\":{\"cells\":[{\"r\":0,\"c\":0,\"v\":\"A\"}]}}}") {
		t.Fatalf("md=%s", s)
	}
	if _, err := ApplyOpsJSONOpts(out, []byte(`[{"op":"replace_text","old":"missing","new":"x"}]`), true); err == nil {
		t.Fatal("expected dry-run error")
	}
}
