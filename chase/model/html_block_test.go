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
	"strings"
	"testing"
)

// flowFragment is the characterization oracle for BlockHTML: the fragment
// ToHTML emits for the block at sections[0].Blocks[idx], recovered by
// stripping ToHTML's <article> wrapper from a single-section render. ToHTML is
// the unchanged ground truth; BlockHTML must never drift from it.
func flowFragment(t *testing.T, doc *Document, blocks ...Block) string {
	t.Helper()
	one := &Document{SchemaVersion: doc.SchemaVersion, Sections: []Section{{ID: 1, Blocks: blocks}}}
	got := ToHTML(one)
	const pre = "<article class=\"eden-docs\">\n"
	const post = "</article>\n"
	if !strings.HasPrefix(got, pre) || !strings.HasSuffix(got, post) {
		t.Fatalf("ToHTML wrapper changed: %q", got)
	}
	return strings.TrimSuffix(strings.TrimPrefix(got, pre), post)
}

// blockHTMLCase pins one block's fragment literally AND against ToHTML.
type blockHTMLCase struct {
	name  string
	block Block
	want  string
}

func blockHTMLCases() []blockHTMLCase {
	return []blockHTMLCase{
		{"heading", Block{Kind: BlockHeading, Level: 2, Text: "Hi"}, "<h2>Hi</h2>\n"},
		{"heading level clamped", Block{Kind: BlockHeading, Level: 9, Text: "Deep"}, "<h6>Deep</h6>\n"},
		{"paragraph escapes", Block{Kind: BlockParagraph, Text: "a < b & c"}, "<p>a &lt; b &amp; c</p>\n"},
		{"paragraph runs", Block{Kind: BlockParagraph, Runs: []TextRun{
			{Text: "x", Marks: []string{"bold"}},
			{Text: " y", Link: "https://e.com/?a=1&b=2"},
		}}, "<p><strong>x</strong><a href=\"https://e.com/?a=1&amp;b=2\"> y</a></p>\n"},
		{"list ordered", Block{Kind: BlockList, Ordered: true, Items: []ListItem{{Text: "one"}, {Text: "two"}}},
			"<ol>\n<li>one</li>\n<li>two</li>\n</ol>\n"},
		{"list bullet", Block{Kind: BlockList, Items: []ListItem{{Text: "a&b"}}}, "<ul>\n<li>a&amp;b</li>\n</ul>\n"},
		{"code", Block{Kind: BlockCode, Language: "go", Text: "a<b"}, "<pre><code class=\"language-go\">a&lt;b</code></pre>\n"},
		{"math display", Block{Kind: BlockMath, Display: true, Text: "x^2"}, "<div class=\"math\">x^2</div>\n"},
		{"math inline", Block{Kind: BlockMath, Text: "x"}, "<span class=\"math\">x</span>\n"},
		{"table", Block{Kind: BlockTable, Headers: []string{"A", "B"}, Rows: [][]string{{"1", "<2>"}}},
			"<table>\n<thead><tr><th>A</th><th>B</th></tr></thead>\n<tbody>\n<tr><td>1</td><td>&lt;2&gt;</td></tr>\n</tbody></table>\n"},
		{"image", Block{Kind: BlockImage, Text: "logo", Src: "a.png", Title: "T"},
			"<figure class=\"figure\"><img src=\"a.png\" alt=\"logo\" title=\"T\"/><figcaption>logo</figcaption></figure>\n"},
		{"quote", Block{Kind: BlockQuote, Text: "q", Children: []Block{{Kind: BlockParagraph, Text: "c"}}},
			"<blockquote><p>q</p><p>c</p>\n</blockquote>\n"},
		{"toggle open", Block{Kind: BlockToggle, Text: "More", Children: []Block{{Kind: BlockParagraph, Text: "inner"}}},
			"<details open><summary>More</summary>\n<p>inner</p>\n</details>\n"},
		{"toggle collapsed", Block{Kind: BlockToggle, Text: "More", Collapsed: true, Children: []Block{{Kind: BlockParagraph, Text: "inner"}}},
			"<details><summary>More</summary>\n<p>inner</p>\n</details>\n"},
		{"todo unchecked", Block{Kind: BlockTodo, Text: "Ship"},
			"<ul class=\"todo\"><li><label><input type=\"checkbox\" disabled/> Ship</label></li></ul>\n"},
		{"todo checked with child", Block{Kind: BlockTodo, Text: "Ship", Checked: true, Children: []Block{{Kind: BlockParagraph, Text: "n"}}},
			"<ul class=\"todo\"><li><label><input type=\"checkbox\" disabled checked/> Ship</label></li></ul>\n<p>n</p>\n"},
		{"callout default tone", Block{Kind: BlockCallout, Text: "Note"}, "<aside class=\"callout info\">Note</aside>\n"},
		{"callout tone icon child", Block{Kind: BlockCallout, Text: "Watch", Tone: "warn", Icon: "!", Children: []Block{{Kind: BlockParagraph, Text: "c"}}},
			"<aside class=\"callout warn\"><span class=\"icon\">!</span> Watch<p>c</p>\n</aside>\n"},
		{"divider", Block{Kind: BlockDivider}, "<hr/>\n"},
		{"file name", Block{Kind: BlockFile, Name: "r.pdf", Src: "https://x/r.pdf"}, "<div class=\"file\"><a href=\"https://x/r.pdf\">r.pdf</a></div>\n"},
		{"file src fallback", Block{Kind: BlockFile, Src: "https://x/y"}, "<div class=\"file\"><a href=\"https://x/y\">https://x/y</a></div>\n"},
		{"sheet snapshot", Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "Tasks", Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{
			{R: 0, C: 0, V: "A"}, {R: 0, C: 1, V: "6", F: "SUM(A1:A3)"},
		}}}},
			"<section class=\"sheet-embed\"><header>Tasks</header>\n<table>\n<tr><td>A</td><td>=SUM(A1:A3)</td></tr>\n</table>\n</section>\n"},
		{"sheet live ref", Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "Tasks"}},
			"<section class=\"sheet-embed\"><header>Tasks</header>\n<p>Live sheet reference (no offline snapshot)</p>\n</section>\n"},
		{"figure svg", Block{Kind: BlockFigure, Text: "logo", SVG: "<svg/>"}, "<figure class=\"figure\"><svg/><figcaption>logo</figcaption></figure>\n"},
		{"figure src", Block{Kind: BlockFigure, Text: "logo", Src: "a.png"},
			"<figure class=\"figure\"><img src=\"a.png\" alt=\"logo\"/><figcaption>logo</figcaption></figure>\n"},
		{"unknown kind falls back", Block{Kind: BlockKind("mystery"), Text: "t", Children: []Block{{Kind: BlockDivider}}}, "<p>t</p>\n<hr/>\n"},
	}
}

// TestBlockHTMLMatchesToHTML characterizes every kind: BlockHTML returns
// exactly the fragment ToHTML emits for that block today, and that fragment
// is pinned literally so a change to either side is caught.
func TestBlockHTMLMatchesToHTML(t *testing.T) {
	cases := blockHTMLCases()
	kinds := map[BlockKind]bool{}
	for _, tc := range cases {
		kinds[tc.block.Kind] = true
		t.Run(tc.name, func(t *testing.T) {
			doc := &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: []Block{tc.block}}}}
			got := BlockHTML(doc, tc.block)
			if got != tc.want {
				t.Errorf("BlockHTML literal drift:\n got:  %q\n want: %q", got, tc.want)
			}
			if oracle := flowFragment(t, doc, tc.block); got != oracle {
				t.Errorf("BlockHTML != ToHTML fragment:\n got:    %q\n oracle: %q", got, oracle)
			}
		})
	}
	for _, k := range []BlockKind{
		BlockParagraph, BlockList, BlockCode, BlockMath, BlockHeading, BlockTable, BlockImage, BlockQuote,
		BlockToggle, BlockSynced, BlockTodo, BlockCallout, BlockDivider, BlockFile, BlockSheet, BlockFigure,
	} {
		if k == BlockSynced {
			continue // covered by TestBlockHTMLSynced (needs a source block in the same doc)
		}
		if !kinds[k] {
			t.Errorf("kind %q has no BlockHTML case", k)
		}
	}
}

// TestBlockHTMLSynced pins same-doc synced resolution: BlockHTML keeps the
// doc parameter so a synced block renders its source's fragment, and an
// unresolvable ref renders the labelled placeholder paragraph.
func TestBlockHTMLSynced(t *testing.T) {
	src := Block{ID: "src1", Kind: BlockParagraph, Text: "shared"}
	resolved := Block{ID: "s1", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "src1"}}
	dangling := Block{ID: "s2", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "nope"}}
	remote := Block{ID: "s3", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "src1", PageID: "other"}}
	doc := &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: []Block{src, resolved, dangling, remote}}}}

	if got, want := BlockHTML(doc, resolved), "<p>shared</p>\n"; got != want {
		t.Errorf("resolved synced = %q, want %q", got, want)
	}
	if got, want := BlockHTML(doc, dangling), "<p class=\"synced\">Synced block nope</p>\n"; got != want {
		t.Errorf("dangling synced = %q, want %q", got, want)
	}
	if got, want := BlockHTML(doc, remote), "<p class=\"synced\">Synced block src1</p>\n"; got != want {
		t.Errorf("cross-page synced = %q, want %q", got, want)
	}
	for _, bl := range []Block{resolved, dangling, remote} {
		if got, oracle := BlockHTML(doc, bl), flowFragment(t, doc, src, bl); !strings.HasSuffix(oracle, got) {
			t.Errorf("synced fragment %q is not the tail of ToHTML's %q", got, oracle)
		}
	}
}

// TestBlockHTMLSyncedCycleTerminates guards untrusted input: a synced block
// that (transitively) points at itself must not recurse forever. A Go stack
// overflow is a fatal error that no caller can recover from, and
// press.RenderModelJSON feeds this writer documents it did not author.
func TestBlockHTMLSyncedCycleTerminates(t *testing.T) {
	self := Block{ID: "a", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "a"}}
	ping := Block{ID: "b", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "c"}}
	pong := Block{ID: "c", Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "b"}}
	doc := &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: []Block{self, ping, pong}}}}

	for _, bl := range []Block{self, ping} {
		got := BlockHTML(doc, bl)
		if !strings.Contains(got, `class="synced"`) {
			t.Errorf("cyclic synced block %q should end in the labelled placeholder, got %q", bl.ID, got)
		}
		if got2 := BlockHTMLEmbedded(doc, bl); !strings.Contains(got2, `class="synced"`) {
			t.Errorf("embedded: cyclic synced block %q should end in the labelled placeholder, got %q", bl.ID, got2)
		}
	}
	// ToHTML shares the writer, so it terminates too.
	if out := ToHTML(doc); !strings.Contains(out, `class="synced"`) {
		t.Errorf("ToHTML did not terminate on a synced cycle with a placeholder: %q", out)
	}
}

// TestBlockHTMLNilDoc keeps BlockHTML total: a nil document renders every
// non-synced kind and degrades a synced block to its placeholder.
func TestBlockHTMLNilDoc(t *testing.T) {
	if got, want := BlockHTML(nil, Block{Kind: BlockParagraph, Text: "p"}), "<p>p</p>\n"; got != want {
		t.Errorf("nil doc paragraph = %q, want %q", got, want)
	}
	synced := Block{Kind: BlockSynced, SyncRef: &SyncRef{BlockID: "x"}}
	if got, want := BlockHTML(nil, synced), "<p class=\"synced\">Synced block x</p>\n"; got != want {
		t.Errorf("nil doc synced = %q, want %q", got, want)
	}
}

// TestBlockHTMLEmbedded pins the single difference between the flowing and
// the embedded writer: a sheet embed is a <div> (a paginated unit must stay
// the only <section> in its page) and a formula cell shows its cached value.
// Every other kind is byte-identical.
func TestBlockHTMLEmbedded(t *testing.T) {
	for _, tc := range blockHTMLCases() {
		if tc.block.Kind == BlockSheet {
			continue
		}
		doc := &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: []Block{tc.block}}}}
		if got, want := BlockHTMLEmbedded(doc, tc.block), BlockHTML(doc, tc.block); got != want {
			t.Errorf("%s: embedded differs from flowing:\n got:  %q\n want: %q", tc.name, got, want)
		}
	}

	sheet := Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "Tasks", Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{
		{R: 0, C: 0, V: "A"},
		{R: 0, C: 1, V: "6", F: "SUM(A1:A3)"},
		{R: 1, C: 0, F: "NOW()"},
	}}}}
	doc := &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: []Block{sheet}}}}
	got := BlockHTMLEmbedded(doc, sheet)
	want := "<div class=\"sheet-embed\"><header>Tasks</header>\n<table>\n<tr><td>A</td><td>6</td></tr>\n<tr><td>=NOW()</td><td></td></tr>\n</table>\n</div>\n"
	if got != want {
		t.Errorf("embedded sheet:\n got:  %q\n want: %q", got, want)
	}
	if strings.Contains(got, "<section") {
		t.Errorf("embedded sheet must not emit <section>: %q", got)
	}

	live := Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "Live"}}
	if g := BlockHTMLEmbedded(doc, live); !strings.HasPrefix(g, "<div class=\"sheet-embed\">") || !strings.HasSuffix(g, "</div>\n") {
		t.Errorf("embedded live sheet = %q", g)
	}

	// Nested: a sheet inside a toggle is still embedded.
	toggle := Block{Kind: BlockToggle, Text: "T", Children: []Block{sheet}}
	if g := BlockHTMLEmbedded(doc, toggle); strings.Contains(g, "<section") || !strings.Contains(g, "<td>6</td>") {
		t.Errorf("nested embedded sheet = %q", g)
	}
}

// TestSheetSnapshotFarCellTerminates guards untrusted input: one cell at an
// absurd coordinate must not make the dense-grid writer loop or allocate
// without bound (the writer emits every row/column up to the largest
// populated cell). Cells beyond the documented limits are not rendered.
func TestSheetSnapshotFarCellTerminates(t *testing.T) {
	sheet := Block{Kind: BlockSheet, SheetEmbed: &SheetEmbed{Sheet: "Far", Snapshot: &SheetSnapshot{Cells: []SheetCellSnap{
		{R: 0, C: 0, V: "near"},
		{R: 1 << 30, C: 1 << 30, V: "far"},
		{R: -1, C: -1, V: "negative"},
	}}}}
	doc := &Document{SchemaVersion: SchemaVersion, Sections: []Section{{ID: 1, Blocks: []Block{sheet}}}}
	for name, got := range map[string]string{"flow": BlockHTML(doc, sheet), "embedded": BlockHTMLEmbedded(doc, sheet)} {
		if !strings.Contains(got, "<td>near</td>") {
			t.Errorf("%s: in-range cell lost: %q", name, got)
		}
		if strings.Contains(got, "far") || strings.Contains(got, "negative") {
			t.Errorf("%s: out-of-range cell rendered: %q", name, got)
		}
	}
}
