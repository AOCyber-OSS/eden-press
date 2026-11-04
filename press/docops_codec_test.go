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
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/model"
)

// codecFixtureDoc builds a stored document through the agent path (NewDocJSON and
// an ops batch, so ids and the outline are the ones ApplyOps makes), covering
// headings, rich text, a list, a table, a toggle with a todo, a callout, a sheet,
// math, code and a section note, then adds page properties and a comment.
func codecFixtureDoc(t *testing.T) []byte {
	t.Helper()
	doc, err := NewDocJSON("Fixture")
	if err != nil {
		t.Fatal(err)
	}
	ops := []byte(`[
		{"op":"insert_block","sectionId":1,"block":{"kind":"heading","level":1,"text":"Plan"}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"paragraph","text":"see docs now","runs":[{"text":"see "},{"text":"docs","link":"https://x.io"},{"text":" "},{"text":"now","marks":["bold"]}]}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"list","items":[{"text":"a"},{"text":"b","level":1}]}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"table","headers":["k","v"],"aligns":["","right"],"rows":[["a","1"]]}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"toggle","text":"More","collapsed":true,"children":[{"kind":"todo","text":"do it","checked":true}]}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"callout","tone":"info","icon":"!","text":"note"}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"sheet","sheetEmbed":{"sheet":"S","snapshot":{"cells":[{"r":0,"c":0,"v":"A"}]}}}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"math","display":true,"text":"x^2"}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"code","language":"go","text":"fmt.Println(1)"}},
		{"op":"insert_block","sectionId":1,"block":{"kind":"heading","level":2,"text":"Émoji 🎉 heading"}}
	]`)
	doc, err = ApplyOpsJSON(doc, ops)
	if err != nil {
		t.Fatal(err)
	}
	var d model.Document
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	d.Sections[0].Notes = []string{"remember"}
	d.Properties = []model.PropertyDef{{ID: "status", Name: "Status", Type: model.PropSelect, Options: []string{"todo", "done"}}}
	d.PropertyValues = map[string]string{"status": "todo"}
	d.Comments = []model.Comment{{ID: "c_1", BlockID: d.Sections[0].Blocks[1].ID, Body: "check this"}}
	out, err := json.MarshalIndent(&d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func decodeDoc(t *testing.T, raw []byte) model.Document {
	t.Helper()
	var d model.Document
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	return d
}

// ---- MarkdownToDocJSON ----

func TestMarkdownToDocJSONStrictRefusalSurfacesUnrepresentableError(t *testing.T) {
	_, err := MarkdownToDocJSON("<div>raw</div>\n", nil, MarkdownImport{})
	var ue *model.UnrepresentableError
	if !errors.As(err, &ue) {
		t.Fatalf("want *model.UnrepresentableError, got %T: %v", err, err)
	}
	if ue.Construct != model.ConstructRawHTML {
		t.Fatalf("construct %q, want %q", ue.Construct, model.ConstructRawHTML)
	}
}

func TestMarkdownToDocJSONLenientMapsInsteadOfRefusing(t *testing.T) {
	out, err := MarkdownToDocJSON("<div>raw</div>\n\n[a](http://x.io \"title\")\n", nil, MarkdownImport{Lenient: true})
	if err != nil {
		t.Fatalf("lenient read failed: %v", err)
	}
	d := decodeDoc(t, out)
	if got := d.Sections[0].Blocks[0].Text; got != "<div>raw</div>" {
		t.Fatalf("raw html maps to %q, want the literal source", got)
	}
}

func TestMarkdownToDocJSONSpansSetOutlineSpans(t *testing.T) {
	md := "# Title\n\ntext\n\n## Émoji 🎉\n"
	for _, spans := range []bool{true, false} {
		out, err := MarkdownToDocJSON(md, nil, MarkdownImport{Spans: spans})
		if err != nil {
			t.Fatal(err)
		}
		d := decodeDoc(t, out)
		if len(d.Outline) != 2 {
			t.Fatalf("outline %+v", d.Outline)
		}
		for _, o := range d.Outline {
			if !spans {
				if o.Span != nil {
					t.Fatalf("Spans=false but %q has a span", o.Text)
				}
				continue
			}
			if o.Span == nil || md[o.Span.Start:o.Span.Stop] != o.Text {
				t.Fatalf("outline %q span %+v covers %q", o.Text, o.Span, md[o.Span.Start:o.Span.Stop])
			}
		}
		if spans && d.Sections[0].Span == nil {
			t.Fatal("section has no span")
		}
		if !spans && strings.Contains(string(out), `"span"`) {
			t.Fatalf("Spans=false but the JSON holds a span:\n%s", out)
		}
	}
}

func TestMarkdownToDocJSONPrevKeepsIDsAcrossAnEdit(t *testing.T) {
	prev := codecFixtureDoc(t)
	md, outline, err := MarkdownSourceViewJSON(prev)
	if err != nil || len(outline) == 0 {
		t.Fatalf("source view: %v %v", outline, err)
	}
	// Edit the Markdown by hand: reword the paragraph, add a new paragraph before
	// the first heading (after the front matter).
	edited := strings.Replace(md, "docs", "manuals", 1)
	edited = strings.Replace(edited, "# Plan", "A brand new first paragraph.\n\n# Plan", 1)
	out, err := MarkdownToDocJSON(edited, prev, MarkdownImport{})
	if err != nil {
		t.Fatal(err)
	}
	was, now := decodeDoc(t, prev), decodeDoc(t, out)

	byText := func(d model.Document) map[string]string {
		m := map[string]string{}
		var walk func(bs []model.Block)
		walk = func(bs []model.Block) {
			for _, b := range bs {
				m[string(b.Kind)+":"+b.Text] = b.ID
				walk(b.Children)
			}
		}
		walk(d.Sections[0].Blocks)
		return m
	}
	wasIDs, nowIDs := byText(was), byText(now)
	for _, key := range []string{"heading:Plan", "callout:note", "math:x^2", "code:fmt.Println(1)", "toggle:More"} {
		if wasIDs[key] == "" || wasIDs[key] != nowIDs[key] {
			t.Errorf("%s: id %q before, %q after the edit", key, wasIDs[key], nowIDs[key])
		}
	}
	if id := nowIDs["paragraph:A brand new first paragraph."]; id == "" {
		t.Error("the new paragraph has no id")
	} else {
		for key, old := range wasIDs {
			if old == id {
				t.Errorf("the new paragraph reuses the id %q of %s", id, key)
			}
		}
	}
	if len(now.Comments) != 1 || now.Comments[0].BlockID != was.Comments[0].BlockID {
		t.Errorf("the comment moved or vanished: %+v", now.Comments)
	}
	// The reworded paragraph is edited in place, so it keeps its id and its comment target.
	if nowIDs["paragraph:see manuals now"] != was.Comments[0].BlockID {
		t.Errorf("the edited paragraph has id %q, want the commented %q", nowIDs["paragraph:see manuals now"], was.Comments[0].BlockID)
	}
}

func TestMarkdownToDocJSONWithoutPrevAssignsIDsAndAcceptsEmptyPrev(t *testing.T) {
	md := "# One\n\n- a\n- b\n\ntext\n"
	a, err := MarkdownToDocJSON(md, nil, MarkdownImport{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarkdownToDocJSON(md, []byte("  "), MarkdownImport{})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("nil and blank prevDoc differ:\n%s\n%s", a, b)
	}
	d := decodeDoc(t, a)
	for _, blk := range d.Sections[0].Blocks {
		if blk.ID == "" {
			t.Fatalf("block %q has no id", blk.Kind)
		}
	}
	if d.Sections[0].Blocks[1].Items[0].ID == "" {
		t.Fatal("list item has no id")
	}
	if _, err := MarkdownToDocJSON(md, []byte("{not json"), MarkdownImport{}); err == nil {
		t.Fatal("a malformed prevDoc must be an error")
	}
}

// ---- VerifyMarkdownRoundTripJSON ----

func TestVerifyMarkdownRoundTripJSONAcceptsAFixture(t *testing.T) {
	if err := VerifyMarkdownRoundTripJSON(codecFixtureDoc(t)); err != nil {
		t.Fatalf("fixture should round-trip: %v", err)
	}
	empty, err := NewDocJSON("")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyMarkdownRoundTripJSON(empty); err != nil {
		t.Fatalf("empty document should round-trip: %v", err)
	}
}

func TestVerifyMarkdownRoundTripJSONAcceptsADocumentWithoutIDsAndWithSpans(t *testing.T) {
	d := decodeDoc(t, codecFixtureDoc(t))
	var strip func(bs []model.Block)
	strip = func(bs []model.Block) {
		for i := range bs {
			bs[i].ID = ""
			for j := range bs[i].Items {
				bs[i].Items[j].ID = ""
			}
			strip(bs[i].Children)
		}
	}
	strip(d.Sections[0].Blocks)
	d.Comments = nil // a comment names a block id
	raw, _ := json.Marshal(&d)
	if err := VerifyMarkdownRoundTripJSON(raw); err != nil {
		t.Fatalf("a document without ids has nothing to lose: %v", err)
	}

	withSpans, err := MarkdownToDocJSON("# T\n\ntext\n", nil, MarkdownImport{Spans: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyMarkdownRoundTripJSON(withSpans); err != nil {
		t.Fatalf("spans are not part of the comparison: %v", err)
	}
}

func TestVerifyMarkdownRoundTripJSONNamesTheFirstDifferingPath(t *testing.T) {
	// A paragraph whose Text is not the flattened runs: ToMarkdown writes the runs,
	// so the read-back Text is "ab" and the stored "zzz" is lost.
	d := decodeDoc(t, codecFixtureDoc(t))
	d.Sections[0].Blocks[1].Text = "zzz"
	raw, _ := json.Marshal(&d)
	err := VerifyMarkdownRoundTripJSON(raw)
	if err == nil {
		t.Fatal("a document that cannot round-trip must be an error")
	}
	for _, want := range []string{"sections[0].blocks[1].text", `"zzz"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// A schema version the codec does not write.
	d = decodeDoc(t, codecFixtureDoc(t))
	d.SchemaVersion = "eden-press.model/v6"
	raw, _ = json.Marshal(&d)
	if err := VerifyMarkdownRoundTripJSON(raw); err == nil || !strings.Contains(err.Error(), "schemaVersion") {
		t.Errorf("want an error naming schemaVersion, got %v", err)
	}
}

func TestVerifyMarkdownRoundTripJSONReportsAStrictRefusalOfItsOwnMarkdown(t *testing.T) {
	// Display math whose TeX holds "$$" is written as a block that cannot be read back.
	d := decodeDoc(t, codecFixtureDoc(t))
	d.Sections[0].Blocks = append(d.Sections[0].Blocks, model.Block{Kind: model.BlockMath, Display: true, Text: "a$$b"})
	raw, _ := json.Marshal(&d)
	err := VerifyMarkdownRoundTripJSON(raw)
	var ue *model.UnrepresentableError
	if !errors.As(err, &ue) {
		t.Fatalf("want a wrapped *model.UnrepresentableError, got %T: %v", err, err)
	}
}

func TestVerifyMarkdownRoundTripJSONRejectsMalformedJSON(t *testing.T) {
	if err := VerifyMarkdownRoundTripJSON([]byte("{nope")); err == nil {
		t.Fatal("malformed JSON must be an error")
	}
}

// ---- MarkdownSourceViewJSON ----

func TestMarkdownSourceViewJSONIsTheExportAndSpansIndexIt(t *testing.T) {
	doc := codecFixtureDoc(t)
	md, outline, err := MarkdownSourceViewJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := ExportMarkdownJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	if md != string(exported) {
		t.Fatalf("source view differs from ExportMarkdownJSON\n got: %q\nwant: %q", md, exported)
	}
	if len(outline) != 2 {
		t.Fatalf("outline %+v", outline)
	}
	for _, o := range outline {
		if o.Span == nil {
			t.Fatalf("heading %q has no span", o.Text)
		}
		if got := md[o.Span.Start:o.Span.Stop]; got != o.Text {
			t.Errorf("md[span] = %q, want the heading %q", got, o.Text)
		}
	}
	if outline[1].Text != "Émoji 🎉 heading" {
		t.Errorf("second heading %q: spans are bytes, not runes", outline[1].Text)
	}
}

func TestMarkdownSourceViewJSONRejectsMalformedJSON(t *testing.T) {
	if _, _, err := MarkdownSourceViewJSON([]byte("[]")); err == nil {
		t.Fatal("a non-document must be an error")
	}
}

// A table an agent inserted with no "aligns" is read back with one empty alignment
// per column. That is the same table (nothing is aligned), so the guard must not
// blame it. Ragged rows, which Markdown cannot spell, are blamed.
func TestVerifyMarkdownRoundTripJSONTreatsMissingAlignsAsUnaligned(t *testing.T) {
	doc, err := NewDocJSON("")
	if err != nil {
		t.Fatal(err)
	}
	ops := []byte(`[{"op":"insert_block","sectionId":1,"block":{"kind":"table","headers":["a","b"],"rows":[["1","2"]]}}]`)
	if doc, err = ApplyOpsJSON(doc, ops); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMarkdownRoundTripJSON(doc); err != nil {
		t.Fatalf("a table without aligns should pass: %v", err)
	}

	ragged := []byte(`[{"op":"insert_block","sectionId":1,"block":{"kind":"table","headers":["a","b"],"rows":[["1","2","3"]]}}]`)
	doc, _ = NewDocJSON("")
	if doc, err = ApplyOpsJSON(doc, ragged); err != nil {
		t.Fatal(err)
	}
	err = VerifyMarkdownRoundTripJSON(doc)
	if err == nil || !strings.Contains(err.Error(), "sections[0].blocks[0].rows[0]") {
		t.Fatalf("a ragged row cannot round-trip, want an error naming rows[0], got %v", err)
	}
}

// ---- formatted table cells (objective 1026, TRD 26, AODex D14) ----

// formattedCellsDoc is a stored document, made through the agent path, whose table
// has a bold header cell, a body cell that is code with a pipe, one that is a link
// and one that is math with a pipe.
func formattedCellsDoc(t *testing.T) []byte {
	t.Helper()
	doc, err := NewDocJSON("Cells")
	if err != nil {
		t.Fatal(err)
	}
	ops := []byte(`[{"op":"insert_block","sectionId":1,"block":{"kind":"table",
		"headers":["k","v"],"aligns":["","right"],
		"headerRuns":[[{"text":"k","marks":["bold"]}],null],
		"rows":[["a","1"],["b|c","see docs"],["x|y","z"]],
		"rowRuns":[
			[[{"text":"a","marks":["bold"]}],[{"text":"1","marks":["code"]}]],
			[[{"text":"b|c","marks":["code"]}],[{"text":"see "},{"text":"docs","link":"https://x.io"}]],
			[[{"text":"x|y","marks":["math"]}],null]
		]}}]`)
	doc, err = ApplyOpsJSON(doc, ops)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// The migration guard accepts a document with formatted cells: identity holds.
func TestVerifyMarkdownRoundTripJSONAcceptsFormattedCells(t *testing.T) {
	doc := formattedCellsDoc(t)
	if err := VerifyMarkdownRoundTripJSON(doc); err != nil {
		t.Fatalf("a table with formatted cells should round-trip: %v", err)
	}
	out, err := ExportMarkdownJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	md := string(out)
	for _, want := range []string{"| **k** | v |", "| **a** | `1` |", "| `b\\|c` | see [docs](https://x.io) |", "| $x\\|y$ | z |"} {
		if !strings.Contains(md, want) {
			t.Errorf("the Markdown should hold %q:\n%s", want, md)
		}
	}
}

// The AODex migration's own path: an LLM-written table (code, bold and links in its
// cells) is read strictly, keeps its formatting in headerRuns/rowRuns and passes the
// guard. It was the most common strict refusal (table-cell-formatting) before D14.
func TestMarkdownToDocJSONStrictReadsFormattedCellsAndPassesTheGuard(t *testing.T) {
	src := "# Report\n\n| Tool | Command | Docs |\n| --- | :---: | --- |\n| **aof** | `aof-tools upgrade --apply` | [guide](https://x.io/g) |\n| plain | `a\\|b` | <https://a.b> |\n"
	doc, err := MarkdownToDocJSON(src, nil, MarkdownImport{})
	if err != nil {
		t.Fatalf("strict read of formatted cells: %v", err)
	}
	if err := VerifyMarkdownRoundTripJSON(doc); err != nil {
		t.Fatalf("the guard should accept what a strict read produced: %v", err)
	}
	d := decodeDoc(t, doc)
	b := d.Sections[0].Blocks[1]
	if b.Kind != model.BlockTable || len(b.RowRuns) != 2 {
		t.Fatalf("want a table with two rows of runs, got %+v", b)
	}
	if got := b.RowRuns[1][1]; len(got) != 1 || got[0].Text != "a|b" || len(got[0].Marks) != 1 || got[0].Marks[0] != "code" {
		t.Errorf("rowRuns[1][1] = %+v, want the code run a|b", got)
	}
	if b.Rows[1][1] != "a|b" {
		t.Errorf("rows[1][1] = %q: the plain text mirrors the runs", b.Rows[1][1])
	}
	if md, err := ExportMarkdownJSON(doc); err != nil || !strings.Contains(string(md), "`a\\|b`") {
		t.Errorf("export should spell the pipe in the code span as \\|: %v\n%s", err, md)
	}

	// Lenient mode keeps the formatting too; it reports no mapping for a cell.
	lenient, err := MarkdownToDocJSON(src, nil, MarkdownImport{Lenient: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(lenient) != string(doc) {
		t.Errorf("lenient and strict differ on a document with nothing to map")
	}

	// A cell still refuses what a paragraph refuses.
	_, err = MarkdownToDocJSON("| a |\n| --- |\n| x<br>y |\n", nil, MarkdownImport{})
	var ue *model.UnrepresentableError
	if !errors.As(err, &ue) || ue.Construct != model.ConstructRawHTML {
		t.Fatalf("raw HTML in a cell is still raw-html, got %v", err)
	}
}

// A cell's runs that the Markdown cannot spell are blamed by path: a mark the codec
// does not write is lost, and the guard names the cell.
func TestVerifyMarkdownRoundTripJSONNamesTheCellRunsThatDoNotSurvive(t *testing.T) {
	d := decodeDoc(t, formattedCellsDoc(t))
	b := &d.Sections[0].Blocks[0]
	b.RowRuns[0][1] = []model.TextRun{{Text: "1", Marks: []string{"underline"}}}
	raw, _ := json.Marshal(&d)
	err := VerifyMarkdownRoundTripJSON(raw)
	if err == nil {
		t.Fatal("a cell run the Markdown cannot write must be an error")
	}
	for _, want := range []string{"sections[0].blocks[0].rowRuns[0][1]", "underline"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
