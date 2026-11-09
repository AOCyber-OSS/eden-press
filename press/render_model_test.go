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
	"html"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/microcosm-cc/bluemonday"

	"go.aocyber.ai/eden-press/chase/model"
)

// --- helpers ----------------------------------------------------------------

var (
	unitRE = regexp.MustCompile(`(?s)<section[ >].*?</section>`)
	brRE   = regexp.MustCompile(`<br\s*/?>\n?`)
	tagRE  = regexp.MustCompile(`<[^>]*>`)
)

// unitsOf returns the profile unit elements ("section") of rendered HTML, in
// order. RenderModel must keep this equal to len(doc.Sections): convert/png and
// convert/pdf pick pages by position.
func unitsOf(h string) []string { return unitRE.FindAllString(h, -1) }

// textOf is the visible text of one unit, <br> as a newline.
func textOf(unit string) string {
	return strings.TrimSpace(html.UnescapeString(tagRE.ReplaceAllString(brRE.ReplaceAllString(unit, "\n"), "")))
}

// strictTextPolicy keeps the page skeleton and paragraphs only: no aside, no
// details, nothing the built-in model policy would have allowed.
func strictTextPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "div", "section", "svg", "foreignObject")
	p.AllowAttrs("id").OnElements("section")
	return p
}

func para(text string) model.Block { return model.Block{Kind: model.BlockParagraph, Text: text} }

func head(level int, text string) model.Block {
	return model.Block{Kind: model.BlockHeading, Level: level, Text: text}
}

func docOf(meta map[string]string, secs ...[]model.Block) *model.Document {
	d := &model.Document{SchemaVersion: model.SchemaVersion}
	if meta != nil {
		d.Meta.Directives = meta
	}
	for i, bl := range secs {
		d.Sections = append(d.Sections, model.Section{ID: i + 1, Blocks: bl})
	}
	return d
}

func mustRenderModel(t *testing.T, doc *model.Document, opts Options) Output {
	t.Helper()
	out, err := RenderModel(doc, opts)
	if err != nil {
		t.Fatalf("RenderModel: %v", err)
	}
	return out
}

// validatedCopy is the expectation for Output.Model: the input after
// GetDocJSON's validation (schema default, block ids, list-item ids, run
// invariant), decoded back into a document.
func validatedCopy(t *testing.T, doc *model.Document) *model.Document {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	norm, err := GetDocJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	var want model.Document
	if err := json.Unmarshal(norm, &want); err != nil {
		t.Fatal(err)
	}
	return &want
}

// --- 1. CSS parity ------------------------------------------------------------

const aodexThemeCSS = "/* @theme aodex-x */\nsection { color: #123456; }"

const parityBody = "# Title\n\nBody\n\n---\n\n## Two\n"

func parityDoc(meta map[string]string) *model.Document {
	return docOf(meta,
		[]model.Block{head(1, "Title"), para("Body")},
		[]model.Block{head(2, "Two")},
	)
}

// TestRenderModelCSSParity: for the same theme and profile, RenderModel's CSS
// is byte-identical to press.Render's for the Markdown twin of the document --
// whether the theme arrives as an Option or as the document's own `theme`
// directive. The packer is shared, not re-implemented.
func TestRenderModelCSSParity(t *testing.T) {
	for _, profile := range []string{"paged", "slides"} {
		for _, theme := range []string{"default", "gaia", "aodex-x"} {
			t.Run(profile+"/"+theme+"/option", func(t *testing.T) {
				opts := Options{Profile: profile, Theme: theme, ThemeCSS: []string{aodexThemeCSS}}
				want, err := Render(parityBody, opts)
				if err != nil {
					t.Fatalf("Render: %v", err)
				}
				got := mustRenderModel(t, parityDoc(nil), opts)
				if got.CSS != want.CSS {
					t.Errorf("CSS differs from Render's (len %d vs %d)", len(got.CSS), len(want.CSS))
				}
				if theme == "aodex-x" && !strings.Contains(got.CSS, "#123456") {
					t.Error("caller theme was not packed (parity would be vacuous)")
				}
			})
			t.Run(profile+"/"+theme+"/directive", func(t *testing.T) {
				opts := Options{Profile: profile, ThemeCSS: []string{aodexThemeCSS}}
				want, err := Render("---\ntheme: "+theme+"\n---\n\n"+parityBody, opts)
				if err != nil {
					t.Fatalf("Render: %v", err)
				}
				got := mustRenderModel(t, parityDoc(map[string]string{"theme": theme}), opts)
				if got.CSS != want.CSS {
					t.Errorf("CSS differs from Render's (len %d vs %d)", len(got.CSS), len(want.CSS))
				}
			})
		}
	}

	// opts.Theme still wins over the document's directive, exactly as in Render.
	got := mustRenderModel(t, parityDoc(map[string]string{"theme": "gaia"}), Options{Theme: "uncover"})
	want, err := Render("---\ntheme: gaia\n---\n\n"+parityBody, Options{Theme: "uncover"})
	if err != nil {
		t.Fatal(err)
	}
	if got.CSS != want.CSS {
		t.Error("opts.Theme did not override the document theme directive the way Render does")
	}
}

// --- 2. unit elements ---------------------------------------------------------

func TestRenderModelUnitElements(t *testing.T) {
	doc := docOf(nil,
		[]model.Block{head(1, "First"), para("alpha")},
		nil, // an empty section must still be a page
		[]model.Block{para("omega")},
	)
	for _, profile := range []string{"paged", "slides", ""} {
		t.Run("profile="+profile, func(t *testing.T) {
			out := mustRenderModel(t, doc, Options{Profile: profile})
			us := unitsOf(out.HTML)
			if len(us) != 3 {
				t.Fatalf("units = %d, want 3:\n%s", len(us), out.HTML)
			}
			if !strings.Contains(us[0], `id="1"`) || !strings.Contains(us[0], "alpha") {
				t.Errorf("unit 1 = %q", us[0])
			}
			if us[1] != `<section id="2"></section>` {
				t.Errorf("empty section should be an empty unit, got %q", us[1])
			}
			if !strings.Contains(us[2], `id="3"`) || !strings.Contains(us[2], "omega") {
				t.Errorf("unit 3 = %q", us[2])
			}
		})
	}

	// First and consecutive empty sections: the places a Markdown separator
	// could be mistaken for front matter or merged away.
	edge := docOf(nil, nil, nil, []model.Block{para("tail")}, nil)
	out := mustRenderModel(t, edge, Options{})
	us := unitsOf(out.HTML)
	if len(us) != 4 {
		t.Fatalf("leading/trailing empties: units = %d, want 4:\n%s", len(us), out.HTML)
	}
	if !strings.Contains(us[2], "tail") || us[0] != `<section id="1"></section>` || us[3] != `<section id="4"></section>` {
		t.Errorf("empty units misplaced: %q", us)
	}
}

// --- 3. v7 kinds ----------------------------------------------------------------

func TestRenderModelV7Kinds(t *testing.T) {
	src := para("shared text")
	src.ID = "src"
	cases := []struct {
		name     string
		block    model.Block
		want     []string
		notWant  []string
		setupSec []model.Block // blocks placed in section 1 ahead of the block under test
	}{
		{"toggle open", model.Block{Kind: model.BlockToggle, Text: "More details", Children: []model.Block{para("hidden body")}},
			[]string{`<details open="">`, `<summary>More details</summary>`, "hidden body"}, nil, nil},
		{"toggle collapsed", model.Block{Kind: model.BlockToggle, Text: "More details", Collapsed: true, Children: []model.Block{para("hidden body")}},
			[]string{`<details>`, `<summary>More details</summary>`, "hidden body"}, []string{`open=`}, nil},
		{"todo checked", model.Block{Kind: model.BlockTodo, Text: "Ship it", Checked: true},
			[]string{`type="checkbox"`, `checked=""`, "Ship it"}, nil, nil},
		{"todo unchecked", model.Block{Kind: model.BlockTodo, Text: "Ship it"},
			[]string{`type="checkbox"`, "Ship it"}, []string{`checked`}, nil},
		{"callout", model.Block{Kind: model.BlockCallout, Text: "Careful", Tone: "warn", Icon: "!"},
			[]string{`<aside class="callout warn">`, `<span class="icon">!</span>`, "Careful"}, nil, nil},
		{"divider", model.Block{Kind: model.BlockDivider}, []string{`<hr`}, nil, nil},
		{"file", model.Block{Kind: model.BlockFile, Name: "report.pdf", Src: "https://files.test/report.pdf"},
			[]string{`<a href="https://files.test/report.pdf">report.pdf</a>`}, nil, nil},
		{"sheet snapshot", model.Block{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "Budget", Snapshot: &model.SheetSnapshot{Cells: []model.SheetCellSnap{
			{R: 0, C: 0, V: "Total"}, {R: 0, C: 1, V: "42", F: "SUM(B2:B9)"},
		}}}},
			[]string{`class="sheet-embed"`, `<header>Budget</header>`, `<table>`, `<td>Total</td>`, `<td>42</td>`},
			[]string{`SUM(`, `<section class="sheet-embed"`}, nil},
		{"sheet live ref", model.Block{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "Budget"}},
			[]string{`class="sheet-embed"`, "Live sheet reference"}, nil, nil},
		{"figure svg", model.Block{Kind: model.BlockFigure, Text: "Logo", SVG: `<svg viewBox="0 0 10 10"><rect width="5" height="5" fill="#d4a853"/></svg>`},
			[]string{`<figure class="figure">`, `<svg `, `<rect `, `fill="#d4a853"`, `<figcaption>Logo</figcaption>`}, nil, nil},
		{"figure asset", model.Block{Kind: model.BlockFigure, Text: "Chart", Src: "https://img.test/c.png"},
			[]string{`<figure class="figure">`, `src="https://img.test/c.png"`, `<figcaption>Chart</figcaption>`}, nil, nil},
		{"synced same page", model.Block{Kind: model.BlockSynced, SyncRef: &model.SyncRef{BlockID: "src"}},
			[]string{"shared text"}, []string{`class="synced"`}, []model.Block{src}},
		{"table", model.Block{Kind: model.BlockTable, Headers: []string{"Metric", "Q3"}, Rows: [][]string{{"p95", "550ms"}}},
			[]string{`<table>`, `<th>Metric</th>`, `<td>550ms</td>`}, nil, nil},
		{"quote", model.Block{Kind: model.BlockQuote, Text: "Said once"},
			[]string{`<blockquote>`, "Said once"}, nil, nil},
		{"image", model.Block{Kind: model.BlockImage, Text: "a diagram", Src: "https://img.test/d.png"},
			[]string{`src="https://img.test/d.png"`, `alt="a diagram"`}, nil, nil},
		{"unknown kind", model.Block{Kind: model.BlockKind("mystery"), Text: "weird"}, []string{"weird"}, nil, nil},
		{"empty paragraph", para(""), []string{`<p></p>`}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := append([]model.Block{para("opening")}, tc.setupSec...)
			second := []model.Block{head(2, "Subject"), tc.block, para("closing")}
			out := mustRenderModel(t, docOf(nil, first, second), Options{Profile: "paged"})
			us := unitsOf(out.HTML)
			if len(us) != 2 {
				t.Fatalf("units = %d, want 2 (a nested <section> would break page selection):\n%s", len(us), out.HTML)
			}
			prev := strings.Index(us[1], "Subject")
			if prev < 0 {
				t.Fatalf("section 2 lost its heading: %s", us[1])
			}
			for _, w := range tc.want {
				i := strings.Index(us[1], w)
				if i < 0 {
					t.Errorf("section 2 missing %q:\n%s", w, us[1])
					continue
				}
				if i < prev {
					t.Errorf("%q rendered before the heading that precedes it: block order lost\n%s", w, us[1])
				}
			}
			if i, j := strings.Index(us[1], "closing"), strings.LastIndex(us[1], tc.want[len(tc.want)-1]); i < 0 || j > i {
				t.Errorf("block rendered after the paragraph that follows it\n%s", us[1])
			}
			for _, w := range tc.notWant {
				if strings.Contains(us[1], w) {
					t.Errorf("section 2 must not contain %q:\n%s", w, us[1])
				}
			}
			if tc.name != "synced same page" {
				for _, w := range tc.want {
					if strings.Contains(us[0], w) {
						t.Errorf("section 1 leaked %q", w)
					}
				}
			}
		})
	}
}

// --- 4. sanitizer ---------------------------------------------------------------

func TestRenderModelSanitizes(t *testing.T) {
	doc := docOf(nil, []model.Block{
		para(`<script>alert(1)</script>`),
		head(2, `<img src=x onerror=alert(2)>`),
		{Kind: model.BlockParagraph, Runs: []model.TextRun{
			{Text: "click", Link: "javascript:alert(3)"},
			{Text: " fine", Link: "https://ok.test/"},
		}},
		{Kind: model.BlockList, Items: []model.ListItem{{Text: `<b onmouseover=alert(4)>x</b>`}}},
		{Kind: model.BlockCallout, Text: `<script>alert(5)</script>`},
		{Kind: model.BlockToggle, Text: `<script>alert(6)</script>`, Children: []model.Block{para(`<iframe src="https://evil.test"></iframe>`)}},
		{Kind: model.BlockFigure, Text: "fig", SVG: `<svg viewBox="0 0 10 10" onload="alert(7)"><script>alert(8)</script>` +
			`<rect width="5" height="5" onclick="alert(9)"/><a href="javascript:alert(10)"><circle r="2"/></a></svg>`},
		{Kind: model.BlockFile, Name: "f", Src: "javascript:alert(11)"},
		{Kind: model.BlockImage, Text: "i", Src: "javascript:alert(12)"},
	})
	out := mustRenderModel(t, doc, Options{Profile: "paged"})
	h := out.HTML

	for _, banned := range []string{"<script", "<iframe", "<img src=x", "javascript:", "onload", "onclick", "onerror=alert(2)>", "onmouseover=alert(4)>"} {
		if strings.Contains(h, banned) {
			t.Errorf("output contains %q:\n%s", banned, h)
		}
	}
	// Hostile text is shown as text, escaped -- never dropped.
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;script&gt;alert(5)&lt;/script&gt;", "&lt;script&gt;alert(6)&lt;/script&gt;"} {
		if !strings.Contains(h, want) {
			t.Errorf("output lost escaped text %q", want)
		}
	}
	// What is legitimate survives next to what is not.
	for _, want := range []string{`<a href="https://ok.test/">`, `<rect `, `<circle `, `<figcaption>fig</figcaption>`} {
		if !strings.Contains(h, want) {
			t.Errorf("sanitizer over-stripped, missing %q", want)
		}
	}
}

// A caller-supplied policy replaces the built-in one for the whole render,
// substituted fragments included.
func TestRenderModelHonoursCallerSanitizePolicy(t *testing.T) {
	doc := docOf(nil, []model.Block{para("kept"), {Kind: model.BlockCallout, Text: "callout text"}})
	out := mustRenderModel(t, doc, Options{Sanitize: strictTextPolicy()})
	if strings.Contains(out.HTML, "<aside") {
		t.Errorf("caller policy did not apply to substituted fragments: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, "callout text") {
		t.Errorf("callout text dropped by caller policy: %s", out.HTML)
	}
}

// --- 5. contract -------------------------------------------------------------------

func TestRenderModelOutputContract(t *testing.T) {
	doc := docOf(map[string]string{"title": "Contract", "size": "4:3"},
		[]model.Block{head(1, "One"), {Kind: model.BlockList, Items: []model.ListItem{{Text: "a"}, {Text: "b"}}}},
		[]model.Block{{Kind: model.BlockCallout, Text: "two", Tone: "info"}},
	)
	doc.Sections[0].Notes = []string{"note one", "note two"}
	doc.Sections[1].Notes = []string{"note three"}

	pristine := validatedCopy(t, doc)
	rawBefore, _ := json.Marshal(doc)

	for _, tc := range []struct {
		opts    Options
		profile string
	}{
		{Options{Profile: "paged"}, "paged"},
		{Options{}, "slides"},
		{Options{Profile: "article"}, "paged"},
	} {
		out := mustRenderModel(t, doc, tc.opts)
		if out.Profile != tc.profile {
			t.Errorf("Profile = %q, want %q for %+v", out.Profile, tc.profile, tc.opts)
		}
		if !reflect.DeepEqual(out.Model, pristine) {
			t.Errorf("Model is not the validated input.\n got:  %+v\n want: %+v", out.Model, pristine)
		}
		if out.Model.Sections[0].Blocks[0].ID == "" {
			t.Error("Model blocks have no ids (EnsureBlockIDs not applied)")
		}
		if out.Model.Sections[0].Blocks[1].Items[0].ID == "" {
			t.Error("Model list items have no ids (EnsureListItemIDs not applied)")
		}
		if !reflect.DeepEqual(out.Meta, out.Model.Meta) || out.Meta.Directives["size"] != "4:3" {
			t.Errorf("Meta = %+v, want the document's meta", out.Meta)
		}
		if want := []string{"note one", "note two", "note three"}; !reflect.DeepEqual(out.Comments, want) {
			t.Errorf("Comments = %q, want %q", out.Comments, want)
		}
		for _, n := range []string{"note one", "note two", "note three"} {
			if strings.Contains(out.HTML, n) {
				t.Errorf("speaker note %q leaked into HTML", n)
			}
		}
	}

	// The caller's document is not modified: no ids appear on it.
	rawAfter, _ := json.Marshal(doc)
	if string(rawBefore) != string(rawAfter) {
		t.Errorf("RenderModel mutated its input:\n before: %s\n after:  %s", rawBefore, rawAfter)
	}
	if doc.Sections[0].Blocks[0].ID != "" {
		t.Error("input block received an id")
	}

	// Deterministic: placeholders never leak into, or vary, the output.
	a := mustRenderModel(t, doc, Options{Profile: "paged"})
	b := mustRenderModel(t, doc, Options{Profile: "paged"})
	if a.HTML != b.HTML || a.CSS != b.CSS {
		t.Error("two renders of the same document differ")
	}
	if strings.Contains(a.HTML, "EPBLK") {
		t.Errorf("placeholder leaked into HTML: %s", a.HTML)
	}
}

func TestRenderModelRejectsBadInput(t *testing.T) {
	if _, err := RenderModel(nil, Options{}); err == nil {
		t.Error("nil document: want an error")
	}
	if _, err := RenderModel(&model.Document{SchemaVersion: model.SchemaVersion}, Options{}); err == nil {
		t.Error("document with no sections: want an error")
	}
	if _, err := RenderModel(parityDoc(nil), Options{Profile: "no-such-profile"}); err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("unknown profile: err = %v", err)
	}
	if _, err := RenderModel(parityDoc(nil), Options{Theme: "no-such-theme"}); err == nil {
		t.Error("unknown theme: want an error")
	}
	if _, err := RenderModel(parityDoc(map[string]string{"theme": "no-such-theme"}), Options{}); err == nil {
		t.Error("unknown document theme: want an error")
	}
}

// --- 6. JSON -----------------------------------------------------------------------------

func TestRenderModelJSON(t *testing.T) {
	// No schemaVersion, no ids, runs without a text mirror: GetDocJSON's
	// validation must be applied before rendering.
	raw := []byte(`{"meta":{"directives":{"title":"T"}},"sections":[` +
		`{"id":1,"blocks":[{"kind":"heading","level":1,"text":"Hello"},` +
		`{"kind":"paragraph","runs":[{"text":"bold","marks":["bold"]},{"text":" rest"}]},` +
		`{"kind":"todo","text":"do it","checked":true}]},` +
		`{"id":2,"blocks":[{"kind":"callout","text":"second","tone":"warn"}]}]}`)

	out, err := RenderModelJSON(raw, Options{Profile: "paged"})
	if err != nil {
		t.Fatalf("RenderModelJSON: %v", err)
	}
	if got := len(unitsOf(out.HTML)); got != 2 {
		t.Fatalf("units = %d, want 2", got)
	}
	if out.Model.SchemaVersion != model.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want default %q", out.Model.SchemaVersion, model.SchemaVersion)
	}
	for _, bl := range out.Model.Sections[0].Blocks {
		if bl.ID == "" {
			t.Errorf("block %q has no id", bl.Kind)
		}
	}
	if got := out.Model.Sections[0].Blocks[1].Text; got != "bold rest" {
		t.Errorf("run invariant not applied: Text = %q", got)
	}
	for _, want := range []string{"<strong>bold</strong>", `checked=""`, `class="callout warn"`} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("HTML missing %q:\n%s", want, out.HTML)
		}
	}

	// Equivalent to rendering the decoded document.
	var d model.Document
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	viaDoc := mustRenderModel(t, &d, Options{Profile: "paged"})
	if viaDoc.HTML != out.HTML || viaDoc.CSS != out.CSS || !reflect.DeepEqual(viaDoc.Model, out.Model) {
		t.Error("RenderModelJSON and RenderModel disagree on the same document")
	}

	for name, bad := range map[string]string{
		"empty":       "",
		"malformed":   `{"sections":[`,
		"wrong top":   `[1,2,3]`,
		"wrong type":  `{"sections":"nope"}`,
		"null":        `null`,
		"no sections": `{"meta":{}}`,
	} {
		if _, err := RenderModelJSON([]byte(bad), Options{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// --- 7. text is literal --------------------------------------------------------------------

// TestRenderModelTextIsLiteral: block text is data. Whatever Markdown, HTML or
// directive syntax it contains must come out as that text -- it must not
// restructure the page (a "---" is not a page break), become a directive, or
// grow elements the model did not ask for.
func TestRenderModelTextIsLiteral(t *testing.T) {
	hostile := []string{
		"---", "***", "# not a heading", "## still not", "1. not a list", "1) nor this", "- not a bullet", "+ nor this",
		"> not a quote", "```", "~~~", "    indented like code", "Title\n===", "Title\n---",
		"<!-- _class: evil -->", "<!-- theme: gaia -->", "<!-- a note -->", "<div>raw</div>",
		"| a | b |\n|---|---|", "$5 and $10", "$$x$$", "*not em* _not em_ __nor__", "~~not struck~~",
		"[x](https://e.test)", "![img](https://e.test/a.png)", "https://autolink.test/path", ":smile:",
		"&amp; stays &lt;literal&gt;", "back\\slash and \\* escaped", "tab\there", "line one\nline two",
		"[ ] [x] task-looking", "{#id .class}", "footnote[^1]", "a  \nhard break",
	}
	for _, h := range hostile {
		t.Run(h, func(t *testing.T) {
			doc := docOf(nil, []model.Block{para(h)}, []model.Block{para("tail")})
			out := mustRenderModel(t, doc, Options{})
			us := unitsOf(out.HTML)
			if len(us) != 2 {
				t.Fatalf("text restructured the page: %d units, want 2:\n%s", len(us), out.HTML)
			}
			if got := strings.Count(us[0], "<p>"); got != 1 {
				t.Errorf("want exactly one paragraph, got %d:\n%s", got, us[0])
			}
			for _, tag := range []string{"<h1", "<h2", "<ul", "<ol", "<li", "<blockquote", "<table", "<pre", "<hr", "<em>", "<strong>", "<s>", "<a ", "<img", "<div", "<math", "<input"} {
				if strings.Contains(us[0], tag) {
					t.Errorf("text grew %q:\n%s", tag, us[0])
				}
			}
			var lines []string
			for _, l := range strings.Split(h, "\n") {
				if l = strings.TrimSpace(l); l != "" {
					lines = append(lines, l)
				}
			}
			if got, want := textOf(us[0]), strings.Join(lines, "\n"); got != want {
				t.Errorf("visible text = %q, want %q", got, want)
			}
			if textOf(us[1]) != "tail" {
				t.Errorf("second unit = %q, want tail", textOf(us[1]))
			}
			if strings.Contains(us[0], `class="evil"`) || strings.Contains(out.CSS, "evil") {
				t.Error("text became a directive")
			}
		})
	}
}

// --- 8. GFM kinds: same markup as Render ---------------------------------------------------

func TestRenderModelGFMKindsMatchRender(t *testing.T) {
	md := "# Title\n\nHello world\n\n- one\n- two\n\n```go\nx := 1\n```\n\n---\n\n## Second\n\n1. first\n2. second\n"
	// Ordered items are numbered by the renderer, not by their text.
	doc := docOf(nil,
		[]model.Block{
			head(1, "Title"), para("Hello world"),
			{Kind: model.BlockList, Items: []model.ListItem{{Text: "one"}, {Text: "two"}}},
			{Kind: model.BlockCode, Language: "go", Text: "x := 1\n"},
		},
		[]model.Block{
			head(2, "Second"),
			{Kind: model.BlockList, Ordered: true, Items: []model.ListItem{{Text: "first"}, {Text: "second"}}},
		},
	)
	for _, profile := range []string{"slides", "paged"} {
		want, err := Render(md, Options{Profile: profile})
		if err != nil {
			t.Fatal(err)
		}
		got := mustRenderModel(t, doc, Options{Profile: profile})
		if got.HTML != want.HTML {
			t.Errorf("%s: HTML differs from the Markdown twin:\n got:  %s\n want: %s", profile, got.HTML, want.HTML)
		}
	}
}

func TestRenderModelRichText(t *testing.T) {
	doc := docOf(nil, []model.Block{
		{Kind: model.BlockParagraph, Runs: []model.TextRun{
			{Text: "bold.", Marks: []string{"bold"}}, {Text: "Next "},
			{Text: "it", Marks: []string{"italic"}}, {Text: " "},
			{Text: "code_x", Marks: []string{"code"}}, {Text: " "},
			{Text: "gone", Marks: []string{"strike"}}, {Text: " "},
			{Text: "both", Marks: []string{"bold", "italic"}}, {Text: " "},
			{Text: "link", Link: "https://e.test/?a=1&b=2"},
		}},
		{Kind: model.BlockHeading, Level: 3, Runs: []model.TextRun{{Text: "Head", Marks: []string{"italic"}}, {Text: " line"}}},
		{Kind: model.BlockList, Items: []model.ListItem{{Runs: []model.TextRun{{Text: "li", Marks: []string{"bold"}}, {Text: " item"}}}}},
	})
	h := mustRenderModel(t, doc, Options{}).HTML
	for _, want := range []string{
		"<strong>bold.</strong>Next ", "<em>it</em>", "<code>code_x</code>", "<s>gone</s>",
		"<em><strong>both</strong></em>", `<a href="https://e.test/?a=1&amp;b=2">link</a>`,
		"<em>Head</em> line", "<strong>li</strong> item",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML missing %q:\n%s", want, h)
		}
	}
}

func TestRenderModelLists(t *testing.T) {
	doc := docOf(nil, []model.Block{
		{Kind: model.BlockList, Items: []model.ListItem{
			{Text: "a"}, {Text: "b", Level: 1}, {Text: "c", Level: 2}, {Text: "d", Level: 1}, {Text: "e"},
			{Text: "jump", Level: 7}, // deeper than one step: clamped, never an indented code block
		}},
		{Kind: model.BlockList, Items: []model.ListItem{{Text: "second list"}}}, // adjacent lists must not merge
		{Kind: model.BlockList, Ordered: true, Items: []model.ListItem{{Text: "x"}, {Text: "y", Level: 1}}},
		{Kind: model.BlockList, Ordered: true, Items: []model.ListItem{{Text: "z"}}},
	})
	h := mustRenderModel(t, doc, Options{}).HTML
	if strings.Contains(h, "<pre") {
		t.Errorf("a list item became a code block:\n%s", h)
	}
	// First list opens a <ul> at the top and under a, b (for c) and e (for
	// "jump"); the second list is a separate <ul>.
	if got, want := strings.Count(h, "<ul>"), 5; got != want {
		t.Errorf("<ul> count = %d, want %d (two separate lists, nested levels):\n%s", got, want, h)
	}
	if got, want := strings.Count(h, "<ol>"), 3; got != want {
		t.Errorf("<ol> count = %d, want %d (two separate ordered lists, one nested):\n%s", got, want, h)
	}
	for _, leaf := range []string{"c", "d", "jump", "second list", "y", "z"} {
		if !strings.Contains(h, "<li>"+leaf+"</li>") {
			t.Errorf("list item %q lost or mis-nested:\n%s", leaf, h)
		}
	}

	// An empty list has nothing to show but must not disturb the page.
	empty := mustRenderModel(t, docOf(nil, []model.Block{{Kind: model.BlockList}, para("after")}), Options{})
	if us := unitsOf(empty.HTML); len(us) != 1 || !strings.Contains(us[0], "after") {
		t.Errorf("empty list disturbed the page: %s", empty.HTML)
	}
}

func TestRenderModelCodeAndMath(t *testing.T) {
	doc := docOf(nil, []model.Block{
		{Kind: model.BlockCode, Language: "go", Text: "fmt.Println(\"```\")\n````\n"},
		{Kind: model.BlockCode, Language: "go\n# evil", Text: "x"},
		{Kind: model.BlockCode, Text: "plain <b>&</b>"},
		{Kind: model.BlockMath, Display: true, Text: "x^2 + 1"},
		{Kind: model.BlockMath, Text: "y"},
		{Kind: model.BlockMath, Display: true, Text: "a\n=\nb"},   // multi-line TeX is joined, not split into a heading
		{Kind: model.BlockMath, Display: true, Text: "cost \\$5"}, // a dollar sign: shown as source, not parsed
		{Kind: model.BlockMath, Text: ""},                         // empty
	}, []model.Block{para("tail")})
	out := mustRenderModel(t, doc, Options{})
	us := unitsOf(out.HTML)
	if len(us) != 2 {
		t.Fatalf("units = %d, want 2:\n%s", len(us), out.HTML)
	}
	h := us[0]
	if got := strings.Count(h, "<pre"); got != 3 {
		t.Errorf("<pre> blocks = %d, want 3 (backtick runs in code must not end the fence):\n%s", got, h)
	}
	if !strings.Contains(h, "plain &lt;b&gt;&amp;&lt;/b&gt;") {
		t.Errorf("code text not escaped literally:\n%s", h)
	}
	if got := strings.Count(h, "<math"); got != 3 {
		t.Errorf("MathML count = %d, want 3 (display, inline, joined multi-line):\n%s", got, h)
	}
	if !strings.Contains(h, `class="math"`) {
		t.Errorf("math with a dollar sign should fall back to the plain math block:\n%s", h)
	}
	if strings.Contains(h, "<h1") || strings.Contains(h, "<h2") {
		t.Errorf("multi-line TeX became a heading:\n%s", h)
	}
}

// --- 9. section directives --------------------------------------------------------------------

func TestRenderModelSectionAndDocumentDirectives(t *testing.T) {
	doc := docOf(map[string]string{
		"headingDivider": "1", // the model's sections are authoritative: this must not split them
		"title":          `Q3: "quarterly" report`,
		"multi":          "line one\nline two",
		"bad key:":       "dropped",
		"paginate":       "true",
	},
		[]model.Block{head(1, "A"), para("x"), head(1, "B"), para("y")},
		[]model.Block{head(1, "C")},
	)
	doc.Sections[0].Attrs = map[string]string{"data-class": "lead", "unrelated": "ignored", "data-paginate": "true"}
	doc.Sections[1].Attrs = map[string]string{"class": "invert"}

	out := mustRenderModel(t, doc, Options{Profile: "paged"})
	us := unitsOf(out.HTML)
	if len(us) != 2 {
		t.Fatalf("headingDivider split the model's sections: %d units, want 2:\n%s", len(us), out.HTML)
	}
	if !strings.Contains(us[0], `class="lead"`) {
		t.Errorf("section 1 lost class directive: %s", us[0])
	}
	if !strings.Contains(us[1], `class="invert"`) {
		t.Errorf("section 2 lost class attr: %s", us[1])
	}
	if !strings.Contains(us[0], "data-marpit-pagination=") {
		t.Errorf("paginate not applied: %s", us[0])
	}
	if strings.Contains(out.HTML, "unrelated") || strings.Contains(out.HTML, "dropped") {
		t.Error("unknown section attr or invalid directive key leaked")
	}
	if out.Meta.Directives["headingDivider"] != "1" {
		t.Error("Meta must still report the document's own headingDivider")
	}

	// A value that tries to close its own comment cannot reach the page.
	evil := docOf(nil, []model.Block{para("x")}, []model.Block{para("y")})
	evil.Sections[0].Attrs = map[string]string{"data-class": "a --> <script>alert(1)</script> <!--"}
	evilOut := mustRenderModel(t, evil, Options{})
	if len(unitsOf(evilOut.HTML)) != 2 || strings.Contains(evilOut.HTML, "<script") || strings.Contains(evilOut.HTML, "alert") {
		t.Errorf("directive value escaped its comment:\n%s", evilOut.HTML)
	}
}

// --- 10. hostile documents ----------------------------------------------------------------------

func TestRenderModelHostileDocuments(t *testing.T) {
	cycle := docOf(nil, []model.Block{
		{ID: "a", Kind: model.BlockSynced, SyncRef: &model.SyncRef{BlockID: "a"}},
		{ID: "b", Kind: model.BlockSynced, SyncRef: &model.SyncRef{BlockID: "c"}},
		{ID: "c", Kind: model.BlockSynced, SyncRef: &model.SyncRef{BlockID: "b"}},
	})
	if out := mustRenderModel(t, cycle, Options{}); !strings.Contains(out.HTML, `class="synced"`) {
		t.Errorf("cyclic synced blocks should end in the labelled placeholder: %s", out.HTML)
	}

	far := docOf(nil, []model.Block{{Kind: model.BlockSheet, SheetEmbed: &model.SheetEmbed{Sheet: "S", Snapshot: &model.SheetSnapshot{
		Cells: []model.SheetCellSnap{{R: 0, C: 0, V: "near"}, {R: 1 << 30, C: 1 << 30, V: "far"}},
	}}}})
	if out := mustRenderModel(t, far, Options{}); !strings.Contains(out.HTML, "near") || strings.Contains(out.HTML, "far") {
		t.Errorf("far sheet cell not handled: %s", out.HTML)
	}

	// Nesting as deep as the limit renders down to the leaf; one level more is
	// refused with the typed error rather than rendered (eden-press#29: deep
	// nesting made every byte API quadratic).
	deep := &model.Block{Kind: model.BlockToggle, Text: "leaf"}
	for i := 1; i < model.MaxNestingDepth; i++ {
		deep = &model.Block{Kind: model.BlockToggle, Text: "t", Children: []model.Block{*deep}}
	}
	if out := mustRenderModel(t, docOf(nil, []model.Block{*deep}), Options{}); !strings.Contains(out.HTML, "leaf") {
		t.Error("deeply nested toggles lost their leaf")
	}
	tooDeep := model.Block{Kind: model.BlockToggle, Text: "t", Children: []model.Block{*deep}}
	var ne *model.NestingDepthError
	if _, err := RenderModel(docOf(nil, []model.Block{tooDeep}), Options{}); !errors.As(err, &ne) {
		t.Errorf("toggles nested %d deep: err = %v, want *model.NestingDepthError", model.MaxNestingDepth+1, err)
	}
}

// --- 11. placeholder splicing -------------------------------------------------------------------------

func TestSpliceFragments(t *testing.T) {
	const nonce = "0123456789abcdef"
	tok := func(i int) string { return placeholderToken(nonce, i) }
	frags := []fragment{{token: tok(0), html: "<div>zero</div>"}, {token: tok(1), html: "<div>one</div>"}}

	page := "<section><p>" + tok(0) + "</p>\n<h1>mid</h1>\n<p>" + tok(1) + "</p>\n</section>"
	got, err := spliceFragments(page, nonce, frags)
	if err != nil {
		t.Fatalf("splice: %v", err)
	}
	want := "<section><div>zero</div>\n<h1>mid</h1>\n<div>one</div>\n</section>"
	if got != want {
		t.Errorf("splice =\n%q\nwant\n%q", got, want)
	}

	for name, bad := range map[string]string{
		"missing placeholder":    "<section><p>" + tok(0) + "</p></section>",
		"duplicated placeholder": page + "<p>" + tok(1) + "</p>",
		"placeholder in a list":  "<section><ul><li>" + tok(0) + "</li></ul><p>" + tok(0) + "</p><p>" + tok(1) + "</p></section>",
		"leaked elsewhere":       page + "<span>" + placeholderToken(nonce, 7) + "</span>",
	} {
		if _, err := spliceFragments(bad, nonce, frags); err == nil {
			t.Errorf("%s: want an error, not a silently dropped or duplicated block", name)
		}
	}
}

// TestRenderModelConcurrent backs the doc comment's concurrency claim: many
// renders of one shared document, run under -race, neither race nor differ.
func TestRenderModelConcurrent(t *testing.T) {
	doc := docOf(map[string]string{"theme": "gaia"},
		[]model.Block{head(1, "T"), {Kind: model.BlockTodo, Text: "x", Checked: true}, {Kind: model.BlockCode, Language: "go", Text: "a := 1"}},
		[]model.Block{{Kind: model.BlockCallout, Text: "c"}},
	)
	want := mustRenderModel(t, doc, Options{Profile: "paged"})

	const workers = 8
	errs := make(chan string, workers)
	for i := 0; i < workers; i++ {
		go func() {
			out, err := RenderModel(doc, Options{Profile: "paged"})
			switch {
			case err != nil:
				errs <- err.Error()
			case out.HTML != want.HTML || out.CSS != want.CSS:
				errs <- "concurrent render differs from the serial one"
			default:
				errs <- ""
			}
		}()
	}
	for i := 0; i < workers; i++ {
		if msg := <-errs; msg != "" {
			t.Error(msg)
		}
	}
}

// --- 12. formatted table cells (TRD 1026-28) --------------------------------------------------------------

// cellRun is a one-run cell entry.
func cellRun(text string, marks ...string) []model.TextRun {
	return []model.TextRun{{Text: text, Marks: marks}}
}

// formattedCellTable is a table whose cells carry runs: a bold header, a code
// cell, an italic cell, an https link, a javascript: link and hostile text
// inside a styled and an unstyled cell. Its flat text (Headers/Rows) is what
// the runs flatten to, as the canonical form requires.
func formattedCellTable() model.Block {
	return model.Block{
		Kind:    model.BlockTable,
		Headers: []string{"Metric", "Q3", "Where"},
		Rows: [][]string{
			{"p95", "550ms", "docs"},
			{"bad", "<script>alert(2)</script>", "<script>alert(3)</script>"},
			{"plain", "row", "here"},
		},
		HeaderRuns: [][]model.TextRun{cellRun("Metric", "bold"), nil, nil},
		RowRuns: [][][]model.TextRun{
			{cellRun("p95", "italic"), cellRun("550ms", "code"), {{Text: "docs", Link: "https://e.test/docs?a=1&b=2"}}},
			{{{Text: "bad", Link: "javascript:alert(1)"}}, nil, cellRun("<script>alert(3)</script>", "bold")},
			nil, // a plain row
		},
	}
}

// plainCellTable is formattedCellTable without its runs: the document a Markdown
// author of the same text would have, and the oracle for "formatting adds no text".
func plainCellTable() model.Block {
	bl := formattedCellTable()
	bl.HeaderRuns, bl.RowRuns = nil, nil
	return bl
}

func TestRenderModelFormattedTableCells(t *testing.T) {
	for _, profile := range []string{"paged", "slides"} {
		t.Run(profile, func(t *testing.T) {
			secs := func(tbl model.Block) *model.Document {
				return docOf(nil,
					[]model.Block{head(1, "One"), para("before")},
					[]model.Block{head(2, "Two"), tbl, para("after")},
					[]model.Block{para("last")},
				)
			}
			opts := Options{Profile: profile}
			got := mustRenderModel(t, secs(formattedCellTable()), opts)
			plain := mustRenderModel(t, secs(plainCellTable()), opts)

			us := unitsOf(got.HTML)
			if len(us) != 3 {
				t.Fatalf("units = %d, want 3 (one per section):\n%s", len(us), got.HTML)
			}
			if got.CSS != plain.CSS {
				t.Errorf("CSS differs from the plain-table document's (len %d vs %d)", len(got.CSS), len(plain.CSS))
			}
			tbl := us[1]

			// Formatting shows inside the cell it belongs to.
			for _, want := range []string{
				`<th><strong>Metric</strong></th>`,
				`<th>Q3</th>`,
				`<td><em>p95</em></td>`,
				`<td><code>550ms</code></td>`,
				`<td><a href="https://e.test/docs?a=1&amp;b=2">docs</a></td>`,
				`<tr><td>plain</td><td>row</td><td>here</td></tr>`,
			} {
				if !strings.Contains(tbl, want) {
					t.Errorf("table missing %q:\n%s", want, tbl)
				}
			}
			// The same table with no runs has none of it.
			for _, tag := range []string{"<strong>", "<em>", "<code>", "<a "} {
				if strings.Contains(unitsOf(plain.HTML)[1], tag) {
					t.Errorf("plain-table document unexpectedly contains %q", tag)
				}
			}

			// Hostile content: the javascript: link keeps its text and loses its
			// href; script text stays text, in a styled and an unstyled cell.
			if strings.Contains(got.HTML, "javascript:") || strings.Contains(got.HTML, "<script") {
				t.Errorf("active content survived:\n%s", tbl)
			}
			if !strings.Contains(tbl, `<td>bad</td>`) && !strings.Contains(tbl, `<td><a>bad</a></td>`) {
				t.Errorf("the javascript: link cell should keep its text and drop its href:\n%s", tbl)
			}
			for _, want := range []string{
				`<td>&lt;script&gt;alert(2)&lt;/script&gt;</td>`,
				`<td><strong>&lt;script&gt;alert(3)&lt;/script&gt;</strong></td>`,
			} {
				if !strings.Contains(tbl, want) {
					t.Errorf("table missing escaped text %q:\n%s", want, tbl)
				}
			}

			// Styling adds no words: every unit's visible text equals the plain
			// document's.
			pus := unitsOf(plain.HTML)
			for i := range us {
				if textOf(us[i]) != textOf(pus[i]) {
					t.Errorf("unit %d text differs from the plain-table document's:\n got:  %q\n want: %q", i+1, textOf(us[i]), textOf(pus[i]))
				}
			}
			// The Model the caller gets back keeps the runs (it is the input, validated).
			var found bool
			for _, bl := range got.Model.Sections[1].Blocks {
				if bl.Kind == model.BlockTable && len(bl.RowRuns) > 0 && len(bl.HeaderRuns) > 0 {
					found = true
				}
			}
			if !found {
				t.Error("Output.Model lost the table's cell runs")
			}
		})
	}
}

// TestRenderModelFormattedCellsOutOfRangeRuns: RenderModelJSON renders documents
// it did not author, so runs arrays that do not line up with the cells must
// render plain cells and never panic.
func TestRenderModelFormattedCellsOutOfRangeRuns(t *testing.T) {
	doc := docOf(nil, []model.Block{{
		Kind:       model.BlockTable,
		Headers:    []string{"H"},
		Rows:       [][]string{{"a", "b"}, {"c"}},
		HeaderRuns: [][]model.TextRun{cellRun("H", "bold"), cellRun("ghost", "bold")},
		RowRuns:    [][][]model.TextRun{{cellRun("a", "code")}, nil, {cellRun("ghost", "code")}},
	}})
	for _, profile := range []string{"paged", "slides"} {
		out, err := RenderModel(doc, Options{Profile: profile})
		if err != nil {
			t.Fatalf("%s: %v", profile, err)
		}
		if strings.Contains(out.HTML, "ghost") {
			t.Errorf("%s: a runs entry past the last cell rendered:\n%s", profile, out.HTML)
		}
		for _, want := range []string{`<th><strong>H</strong></th>`, `<td><code>a</code></td>`, `<td>b</td>`, `<td>c</td>`} {
			if !strings.Contains(out.HTML, want) {
				t.Errorf("%s: missing %q:\n%s", profile, want, out.HTML)
			}
		}
	}
}

// TestRenderModelJSONFormattedCells: the JSON entry point (the AODex export
// path) renders the cell runs a stored document carries.
func TestRenderModelJSONFormattedCells(t *testing.T) {
	raw, err := json.Marshal(docOf(nil, []model.Block{formattedCellTable()}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"headerRuns":[[{"text":"Metric","marks":["bold"]}],null,null]`, `"rowRuns":[`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("test document lost its wire form %q: %s", want, raw)
		}
	}
	out, err := RenderModelJSON(raw, Options{Profile: "paged"})
	if err != nil {
		t.Fatalf("RenderModelJSON: %v", err)
	}
	for _, want := range []string{`<th><strong>Metric</strong></th>`, `<td><code>550ms</code></td>`} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("JSON render missing %q:\n%s", want, out.HTML)
		}
	}
}

// TestRenderStaysPlainForMarkdownTables: press.Render's Model comes from
// model.Build, which has never produced cell runs and still does not. A
// formatted Markdown cell is flattened to its text there; the formatted form
// is the docmodel codec's (FromMarkdown), not Build's.
func TestRenderStaysPlainForMarkdownTables(t *testing.T) {
	out, err := Render("| **b** |\n|---|\n| `c` |\n", Options{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var tables int
	for _, sec := range out.Model.Sections {
		for _, bl := range sec.Blocks {
			if bl.Kind != model.BlockTable {
				continue
			}
			tables++
			if bl.HeaderRuns != nil || bl.RowRuns != nil {
				t.Errorf("Build emitted cell runs: headerRuns=%v rowRuns=%v", bl.HeaderRuns, bl.RowRuns)
			}
			if !reflect.DeepEqual(bl.Headers, []string{"b"}) || !reflect.DeepEqual(bl.Rows, [][]string{{"c"}}) {
				t.Errorf("flattened cells = %q / %q, want [b] / [[c]]", bl.Headers, bl.Rows)
			}
		}
	}
	if tables != 1 {
		t.Errorf("tables in Output.Model = %d, want 1", tables)
	}
}
