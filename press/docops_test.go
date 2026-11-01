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

func TestNewDocJSONAndApplyOpsJSON(t *testing.T) {
	docJSON, err := NewDocJSON("Agent page")
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(docJSON, &probe); err != nil {
		t.Fatal(err)
	}
	if probe["schemaVersion"] != "eden-press.model/v7" {
		t.Fatalf("schemaVersion = %v", probe["schemaVersion"])
	}

	ops := []byte(`[
		{"op":"insert_block","sectionId":1,"block":{"id":"b_title","kind":"heading","level":1,"text":"Hello"}},
		{"op":"insert_block","sectionId":1,"block":{"id":"b_p","kind":"paragraph","text":"from an agent"}},
		{"op":"update_block","blockId":"b_title","block":{"id":"b_title","kind":"heading","level":1,"text":"Hello agents"}}
	]`)
	out, err := ApplyOpsJSON(docJSON, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"Hello agents"`) {
		t.Fatalf("update missing: %s", out)
	}
	if !strings.Contains(string(out), `"from an agent"`) {
		t.Fatalf("paragraph missing: %s", out)
	}
}

func TestImportMarkdownModelJSON(t *testing.T) {
	md := []byte("---\nmarp: true\n---\n\n# Deck\n\nBody.\n")
	raw, err := ImportMarkdownModelJSON(md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["schemaVersion"] != "eden-press.model/v7" {
		t.Fatalf("schemaVersion = %v", doc["schemaVersion"])
	}
	sections, _ := doc["sections"].([]any)
	if len(sections) == 0 {
		t.Fatalf("expected sections: %s", raw)
	}
}

func TestApplyOpsJSONEnvelopeObject(t *testing.T) {
	docJSON, err := NewDocJSON("")
	if err != nil {
		t.Fatal(err)
	}
	// Agents may send {"ops":[...]} or a bare array.
	env := []byte(`{"ops":[{"op":"insert_block","sectionId":1,"block":{"kind":"paragraph","text":"x"}}]}`)
	out, err := ApplyOpsJSON(docJSON, env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"text": "x"`) && !strings.Contains(string(out), `"text":"x"`) {
		t.Fatalf("got %s", out)
	}
}
