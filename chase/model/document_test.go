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

package model

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/markdown"
)

// TestDocumentSchemaVersionConstant covers Test-list case 5's constant half:
// SchemaVersion must be a non-empty, stable constant. 06-01 bumps it v1 -> v2
// for the additive Section.Blocks enrichment; EPD-R1 bumps it v2 -> v3 for the
// table/image/quote block kinds (the quote kind re-classifies output a v2
// consumer would already have seen, so the bump is not merely additive); AODex
// Objective 16 bumps it v3 -> v4 for the additive source Spans; the AI doc-ops
// spike bumps it v4 -> v5 for additive Block.ID -- additive, but bumped anyway
// because this schema versions its SHAPE, not only its breaking changes (see
// TestV4IsAStrictSupersetOfV3 / TestV5BuildOmitsEmptyBlockID).
func TestDocumentSchemaVersionConstant(t *testing.T) {
	if SchemaVersion == "" {
		t.Fatal("SchemaVersion must be non-empty")
	}
	const want = "eden-press.model/v6"
	if SchemaVersion != want {
		t.Fatalf("SchemaVersion = %q, want %q", SchemaVersion, want)
	}
}

// TestDocumentJSONRoundTrip covers Test-list case 5: json.Marshal(doc) then
// json.Unmarshal round-trips to an equal Document, and the marshaled bytes
// contain "schemaVersion" with the constant value (now v3).
func TestDocumentJSONRoundTrip(t *testing.T) {
	doc := &Document{
		SchemaVersion: SchemaVersion,
		Meta: Meta{
			Directives: map[string]string{"theme": "gaia", "size": "16:9"},
		},
		Sections: []Section{
			{
				ID:    1,
				Attrs: map[string]string{"class": "lead"},
				Notes: []string{"a presenter note"},
			},
			{ID: 2},
		},
		Outline: []OutlineEntry{
			{SectionID: 1, Level: 1, Text: "Hello", Slug: "hello"},
		},
	}

	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"schemaVersion":"eden-press.model/v6"`) {
		t.Fatalf("marshaled JSON missing expected schemaVersion field: %s", b)
	}

	var got Document
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*doc, got) {
		t.Fatalf("round-trip mismatch:\n got  = %+v\n want = %+v", got, *doc)
	}
}

// TestBlockOmitemptyAdditive covers Test-list case 1 (additive-non-breaking,
// outermost regression): a v1-era document carrying only headings/notes (NO
// extracted Blocks) marshals with NO `blocks` key anywhere (omitempty), and its
// JSON differs from the frozen v1 shape by ONLY the schemaVersion string. This
// is the proof that Section.Blocks + every new Block field is purely additive:
// a block-less document's bytes are byte-for-byte identical to v1 once the
// version string is normalized back.
func TestBlockOmitemptyAdditive(t *testing.T) {
	doc := &Document{
		SchemaVersion: SchemaVersion,
		Meta: Meta{
			Directives: map[string]string{"theme": "gaia"},
		},
		Sections: []Section{
			{ID: 1, Notes: []string{"a note"}},
		},
		Outline: []OutlineEntry{
			{SectionID: 1, Level: 1, Text: "H", Slug: "h"},
		},
	}

	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)

	if strings.Contains(got, "blocks") {
		t.Fatalf("block-less document must NOT serialize a `blocks` key (omitempty violated): %s", got)
	}
	if !strings.Contains(got, `"schemaVersion":"eden-press.model/v6"`) {
		t.Fatalf("expected v4 schemaVersion: %s", got)
	}

	// The FROZEN v1 shape: this exact byte string is what a pre-06-01 build of
	// the identical document produced. Normalizing v4 -> v1 must reproduce it
	// byte-for-byte -- i.e. the ONLY difference is the version string. Still
	// load-bearing at v4: it proved EPD-R1's five new Block fields
	// (Headers/Rows/Aligns/Src/Title) did not disturb the envelope, and now
	// proves the same for Objective 16's three Span fields -- a span-less
	// document must still serialize with no `span` key anywhere.
	const frozenV1 = `{"schemaVersion":"eden-press.model/v1","meta":{"directives":{"theme":"gaia"}},"sections":[{"id":1,"notes":["a note"]}],"outline":[{"sectionId":1,"level":1,"text":"H","slug":"h"}]}`
	normalized := strings.ReplaceAll(got, "eden-press.model/v6", "eden-press.model/v1")
	if normalized != frozenV1 {
		t.Fatalf("block-less v4 JSON differs from frozen v1 by more than the version string:\n got  = %s\n want = %s", normalized, frozenV1)
	}
}

// TestSchemaV3RoundTrip covers Test-list case 2: a Document carrying one Block
// of EVERY kind (heading, paragraph, list w/ nested items, code w/ language,
// display math, and EPD-R1's table w/ aligns, image w/ title and quote)
// round-trips byte-stably (Marshal -> Unmarshal -> Marshal is identical) and
// reports the current SchemaVersion (now v4).
func TestSchemaV3RoundTrip(t *testing.T) {
	doc := &Document{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{Directives: map[string]string{"theme": "default"}},
		Sections: []Section{
			{
				ID: 1,
				Blocks: []Block{
					{Kind: BlockHeading, Level: 2, Text: "H2"},
					{Kind: BlockParagraph, Text: "a paragraph"},
					{Kind: BlockList, Ordered: true, Items: []ListItem{
						{Text: "one"},
						{Text: "two", Level: 1},
					}},
					{Kind: BlockCode, Language: "go", Text: "fmt.Println()\n"},
					{Kind: BlockMath, Text: "E=mc^2", Display: true},
					{Kind: BlockTable,
						Headers: []string{"Metric", "Q3"},
						Rows:    [][]string{{"p95", "550ms"}},
						Aligns:  []string{"left", "right"}},
					{Kind: BlockImage, Src: "c.png", Text: "chart", Title: "Quarterly"},
					{Kind: BlockQuote, Text: "Synthesized from the sync."},
				},
			},
		},
		Outline: []OutlineEntry{
			{SectionID: 1, Level: 2, Text: "H2", Slug: "h2"},
		},
	}

	if doc.SchemaVersion != "eden-press.model/v6" {
		t.Fatalf("SchemaVersion = %q, want v6", doc.SchemaVersion)
	}

	b1, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal #1: %v", err)
	}

	var got Document
	if err := json.Unmarshal(b1, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*doc, got) {
		t.Fatalf("round-trip mismatch:\n got  = %+v\n want = %+v", got, *doc)
	}

	b2, err := json.Marshal(&got)
	if err != nil {
		t.Fatalf("Marshal #2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("Marshal->Unmarshal->Marshal not byte-stable:\n b1 = %s\n b2 = %s", b1, b2)
	}
}

// TestBlockNewFieldsOmitempty is the per-Block half of the additivity proof
// TestBlockOmitemptyAdditive makes at envelope level: EPD-R1 added five fields
// to Block (Headers/Rows/Aligns/Src/Title), and a block that does not use them
// must serialize exactly as it did at v2. Asserted as an exact byte string, not
// a substring check, so a stray non-omitempty tag cannot slip through.
func TestBlockNewFieldsOmitempty(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    Block
		want string
	}{
		{"paragraph", Block{Kind: BlockParagraph, Text: "a paragraph"},
			`{"kind":"paragraph","text":"a paragraph"}`},
		{"heading", Block{Kind: BlockHeading, Level: 2, Text: "H2"},
			`{"kind":"heading","text":"H2","level":2}`},
		{"code", Block{Kind: BlockCode, Language: "go", Text: "x"},
			`{"kind":"code","text":"x","language":"go"}`},
		{"math", Block{Kind: BlockMath, Text: "E=mc^2", Display: true},
			`{"kind":"math","text":"E=mc^2","display":true}`},
		// A table carries no Src/Title; an image carries no Headers/Rows/Aligns.
		{"table", Block{Kind: BlockTable, Headers: []string{"A"}, Rows: [][]string{{"1"}}, Aligns: []string{"left"}},
			`{"kind":"table","headers":["A"],"rows":[["1"]],"aligns":["left"]}`},
		{"image", Block{Kind: BlockImage, Src: "c.png", Text: "alt"},
			`{"kind":"image","text":"alt","src":"c.png"}`},
		{"quote", Block{Kind: BlockQuote, Text: "quoted"},
			`{"kind":"quote","text":"quoted"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.b)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(b) != tc.want {
				t.Errorf("Block JSON = %s, want %s", b, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Schema v4 additivity (AODex Objective 16, 16-E2).
// ---------------------------------------------------------------------------

// stripSpans removes every `span` key from v, recursively. The walk must be
// recursive because spans live at three depths: on sections, on their nested
// blocks, and on outline entries. It returns the count removed so a caller can
// prove the strip actually did something -- otherwise a v4 document with NO
// spans would satisfy the superset test vacuously.
func stripSpans(v any, removed *int) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if k == "span" {
				*removed++
				continue
			}
			out[k] = stripSpans(val, removed)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = stripSpans(val, removed)
		}
		return out
	default:
		return v
	}
}

// canonicalJSON re-marshals v through encoding/json, which sorts object keys,
// producing a byte string that is equal for two documents exactly when the two
// JSON VALUES are equal -- independent of the key order either side happened to
// emit. Comparing canonical forms is therefore a complete test for "no field
// was added, removed, renamed, reordered into a different value, or changed".
func canonicalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return string(b)
}

// jsonKeyPaths collects every "a.b.c" key path present in v, so a mismatch can
// be reported as the exact set of keys that appeared or vanished rather than as
// two walls of JSON. Array indices collapse to "[]" so paths are comparable
// across documents of different length.
func jsonKeyPaths(v any, prefix string, into map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			into[p] = true
			jsonKeyPaths(val, p, into)
		}
	case []any:
		for _, val := range t {
			jsonKeyPaths(val, prefix+"[]", into)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestV4IsAStrictSupersetOfV3 PROVES the additivity claim the v3 -> v4 comment
// makes, rather than asserting it.
//
// testdata/v3_golden.json was captured from the tree BEFORE the constant was
// bumped and before spans existed -- a golden captured after the change would
// make this test tautological. Rendering the same fixture at v4, stripping
// every `span` key and normalizing the version string must reproduce that
// golden exactly.
//
// This is the evidence AODex's schema guard (TRD 16-04) needs when 16-06 moves
// the dependency and the guard fires on the version change: v4 is additive, no
// field was re-classified, and the guard may accept it.
func TestV4IsAStrictSupersetOfV3(t *testing.T) {
	src, err := os.ReadFile("testdata/superset_fixture.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	goldenBytes, err := os.ReadFile("testdata/v3_golden.json")
	if err != nil {
		t.Fatalf("read v3 golden: %v", err)
	}

	doc, pc := markdown.Parse(string(src))
	v4Bytes, err := json.Marshal(Build(doc, src, pc))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// The golden really is a v3 capture, not a stale copy of the current tree.
	if !strings.Contains(string(goldenBytes), `"eden-press.model/v3"`) {
		t.Fatal("testdata/v3_golden.json is not a v3 capture; the superset proof would be tautological")
	}
	if strings.Contains(string(goldenBytes), `"span"`) {
		t.Fatal("testdata/v3_golden.json contains spans; it was captured AFTER the change and proves nothing")
	}

	var v4, v3 any
	if err := json.Unmarshal(v4Bytes, &v4); err != nil {
		t.Fatalf("unmarshal v4: %v", err)
	}
	if err := json.Unmarshal(goldenBytes, &v3); err != nil {
		t.Fatalf("unmarshal v3 golden: %v", err)
	}

	// Removing the spans must actually remove something, or the comparison
	// below would pass for a build that never populated a span at all.
	removed := 0
	stripped := stripSpans(v4, &removed)
	if removed < 12 {
		t.Fatalf("only %d span keys stripped from the v4 render; the fixture's 15 positionable nodes should carry far more", removed)
	}

	// Normalize the ONE field that is allowed to differ. Current schema is
	// v5; Build still emits no block ids, so stripping spans alone recovers
	// the v3 golden after rewriting the version string.
	if m, ok := stripped.(map[string]any); ok {
		if got := m["schemaVersion"]; got != "eden-press.model/v6" {
			t.Fatalf("current render reports schemaVersion %v, want eden-press.model/v6", got)
		}
		m["schemaVersion"] = "eden-press.model/v3"
	} else {
		t.Fatal("current render is not a JSON object")
	}

	gotJSON, wantJSON := canonicalJSON(t, stripped), canonicalJSON(t, v3)
	if gotJSON != wantJSON {
		gotKeys, wantKeys := map[string]bool{}, map[string]bool{}
		jsonKeyPaths(stripped, "", gotKeys)
		jsonKeyPaths(v3, "", wantKeys)
		var added, missing []string
		for _, k := range sortedKeys(gotKeys) {
			if !wantKeys[k] {
				added = append(added, k)
			}
		}
		for _, k := range sortedKeys(wantKeys) {
			if !gotKeys[k] {
				missing = append(missing, k)
			}
		}
		t.Fatalf("v4 is NOT a strict superset of v3.\n keys added beyond span:   %v\n keys missing from v4:     %v\n got  = %s\n want = %s",
			added, missing, gotJSON, wantJSON)
	}

	// And the converse, stated as its own assertion: the ONLY key v4 adds
	// anywhere in the document is `span`.
	v4Keys, v3Keys := map[string]bool{}, map[string]bool{}
	jsonKeyPaths(v4, "", v4Keys)
	jsonKeyPaths(v3, "", v3Keys)
	for _, k := range sortedKeys(v4Keys) {
		if v3Keys[k] {
			continue
		}
		if !strings.HasSuffix(k, "span") && !strings.Contains(k, "span.") {
			t.Errorf("v4 introduced key %q, which is not a span -- v4 would not be additive", k)
		}
	}
	for _, k := range sortedKeys(v3Keys) {
		if !v4Keys[k] {
			t.Errorf("v4 DROPPED v3 key %q", k)
		}
	}
}

// TestV5BuildOmitsEmptyBlockID proves Build()-produced blocks carry no id key
// (Markdown has none), so a v5 Build render is a strict v4 superset apart from
// the schemaVersion string.
func TestV5BuildOmitsEmptyBlockID(t *testing.T) {
	src := []byte("# Hello\n\nBody.\n")
	astDoc, pc := markdown.Parse(string(src))
	raw, err := json.Marshal(Build(astDoc, src, pc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schemaVersion":"eden-press.model/v6"`) {
		t.Fatalf("missing v6 version: %s", raw)
	}
	// Section.ID is numeric (`"id":1`). Block.ID is a string key (`"id":"..."`).
	// Build must not emit the latter.
	if strings.Contains(string(raw), `"id":"`) {
		t.Fatalf("Build must omit empty block id keys: %s", raw)
	}
}

// TestSpanKeyShapeInJSON pins the serialized shape of a span itself: the inner
// ints carry NO omitempty, so a span at offset 0 emits `"start":0` rather than
// vanishing, while a node with no span emits no key at all. Asserted as exact
// byte strings so a stray tag cannot slip through.
func TestSpanKeyShapeInJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    Block
		want string
	}{
		{"span at offset zero", Block{Kind: BlockParagraph, Text: "x", Span: &Span{Start: 0, Stop: 1}},
			`{"kind":"paragraph","text":"x","span":{"start":0,"stop":1}}`},
		{"span with both ends zero", Block{Kind: BlockParagraph, Text: "x", Span: &Span{}},
			`{"kind":"paragraph","text":"x","span":{"start":0,"stop":0}}`},
		{"no span", Block{Kind: BlockParagraph, Text: "x"},
			`{"kind":"paragraph","text":"x"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.b)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(b) != tc.want {
				t.Errorf("Block JSON = %s, want %s", b, tc.want)
			}
		})
	}
}
