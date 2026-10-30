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

// Package model is Eden Press's own versioned, JSON-serializable document
// model (the "docmodel"): Document{Meta, Sections, Outline}. This is
// differentiator #3 ("output-as-data") made a first-class package on day
// one, per ARCHITECTURE.md Pattern 3.
//
// goldmark's ast.Node tree is a transient parse-time working structure: it
// references source-buffer segments rather than owning copies of text, has
// no built-in JSON marshaling, and its shape is an internal implementation
// detail of whichever goldmark version chase/markdown happens to be built
// against. Document is the durable product a consumer (Eden-Biz, AOCore, an
// LLM ingestion pipeline) actually wants -- outline/notes/metadata WITHOUT
// parsing rendered HTML back out of HTML.
//
// Document is built by Build (see build.go): a read-only, direct recursive
// walk of the SAME finalized *ast.Document chase/markdown/seam.go's Parse
// returns -- the exact tree chase/markdown's own goldmark renderer
// consumes to produce HTML. Never a second parse; never reverse-engineered
// from rendered HTML.
package model

// SchemaVersion identifies the shape of Document below. It is owned and
// versioned independently of goldmark/Marp upstream churn (ARCHITECTURE.md
// Pattern 3's trade-off note: "a schema you now own and must version"). A
// future incompatible change to Document's JSON shape bumps this constant,
// giving a consumer a stable signal to branch on rather than silently
// misinterpreting an old/new shape.
//
// v1 -> v2 (06-01): ADDITIVE per-section body-content enrichment. Section
// gained a Blocks []Block field (ordered paragraph/list/code/math/heading
// content, materialized by the SAME single read-only Build walk). Every new
// field carries `omitempty`, so a v1-shaped (block-less) document's JSON is
// byte-for-byte unchanged apart from this version string -- v2 is a strict
// superset of v1, never a breaking reshape.
//
// v2 -> v3 (EPD-R1, AODex Objective 11): table/image/quote block kinds.
// Mostly additive -- a GFM table's and an image's content were previously
// ABSENT from the model entirely (measured: a table's cell text appeared
// nowhere in the serialized Document, so a consumer reading the model could
// not recover it by any means), and now materialize as table/image Blocks.
// The version bump is NOT merely for those additions, though: a blockquote
// previously emitted an indistinguishable BlockParagraph and now emits
// BlockQuote instead. That RE-CLASSIFIES output a v2 consumer would already
// have seen for the same input, so v3 is a deliberate, signalled change
// rather than a silent v2 extension -- exactly the branch signal this
// constant exists to give.
//
// v3 -> v4 (AODex Objective 16): ADDITIVE source positions. Section, Block and
// OutlineEntry each gained an optional Span -- a half-open [start, stop) BYTE
// range into the Markdown source -- materialized from the text.Segment bounds
// the SAME single Build walk already visited and previously discarded.
//
// Nothing is re-classified. No existing field changed meaning, shape or
// presence: a v3 consumer that ignores unknown keys sees byte-for-byte the
// same document apart from this version string. That is the OPPOSITE of
// v2 -> v3, which moved blockquotes off BlockParagraph and therefore genuinely
// required a consumer to look. The superset claim is PROVEN, not asserted --
// TestV4IsAStrictSupersetOfV3 strips every `span` key from a v4 render and
// compares the result to a golden captured from the pre-bump v3 tree.
//
// The bump happens anyway because this schema versions its SHAPE, not only its
// breaking changes -- the same reason v1 -> v2 bumped for a purely additive
// Blocks field.
//
// Why positions at all: without them an editor cannot map an outline click to
// a source offset, and reconstructing Section boundaries client-side means
// replicating headingDivider's SYNTHETIC thematic breaks (which appear in no
// source text whatsoever) and goldmark's setext-heading precedence in another
// language.
//
// v4 -> v5 (AI doc-ops spike): ADDITIVE stable Block.ID for agent/editor
// apply_ops addressing. Build still leaves ID empty (Markdown has no block
// ids); NewDocument/ApplyOps assign them. omitempty keeps Build JSON a
// strict superset of v4 apart from this version string -- proven by
// TestV5BuildIsAStrictSupersetOfV4.
const SchemaVersion = "eden-press.model/v5"

// Span is a half-open byte range [Start, Stop) into the Markdown source that
// produced the node carrying it: source[Start:Stop] re-slices that node's
// original text. Half-open matches Go slice semantics exactly, so the re-slice
// needs no arithmetic at the call site.
//
// OFFSETS ARE BYTES -- not runes, and not UTF-16 code units. A client whose
// strings are UTF-16 (Dart, JavaScript) MUST convert before indexing, or every
// position after the first multi-byte character will be wrong. Emitting bytes
// is deliberate rather than convenient: text.Segment carries bytes, this
// package's Go consumers (the DOCX/PPTX/XLSX writers) want bytes, and baking
// one client's string model into a general-purpose library would be the wrong
// trade. TestSpansAreByteOffsets pins this with an accented word and an emoji,
// and proves a rune-indexed and a UTF-16-indexed read of the same numbers are
// both wrong.
//
// Span is a POINTER on its owners, with NON-omitempty inner fields. That shape
// is load-bearing, not stylistic: `Start int json:"start,omitempty"` would drop
// a legitimate offset 0 -- the first block of every document, i.e. exactly the
// node a "scroll to top" targets -- making it indistinguishable from an
// unpositioned node. Nil-or-present at the outer level; 0 serializes fine at
// the inner level. It also keeps v3 JSON byte-for-byte unchanged for any node
// whose position cannot be determined, preserving the strict-superset property
// v2 established. TestFirstBlockSpanSurvivesJSON pins both halves.
//
// A node whose position cannot be determined carries a NIL Span, never a
// fabricated {0, 0}: a fabricated span sends an editor's cursor to the top of
// the document, which is worse than no cursor at all.
type Span struct {
	// Start is the inclusive byte offset of the node's first source byte.
	Start int `json:"start"`

	// Stop is the exclusive byte offset one past the node's last source byte.
	Stop int `json:"stop"`
}

// BlockKind enumerates the kinds of body-content block a Section can carry.
type BlockKind string

const (
	// BlockParagraph is a plain-text paragraph (editable prose).
	BlockParagraph BlockKind = "paragraph"
	// BlockList is a bullet or ordered list of items.
	BlockList BlockKind = "list"
	// BlockCode is a fenced or indented code block (raw source + language).
	BlockCode BlockKind = "code"
	// BlockMath is a `$…$`/`$$…$$` math construct (raw TeX + display flag).
	BlockMath BlockKind = "math"
	// BlockHeading is a heading (level + text); mirrors an Outline entry.
	BlockHeading BlockKind = "heading"
	// BlockTable is a GFM table (Headers + Rows + per-column Aligns). Added
	// in v3; before that a table's content was absent from the model.
	BlockTable BlockKind = "table"
	// BlockImage is an image (Src + alt Text + optional Title). Added in v3;
	// before that an image's content was absent from the model.
	BlockImage BlockKind = "image"
	// BlockQuote is a blockquote. Added in v3; before that a blockquote's
	// text was emitted as an indistinguishable BlockParagraph.
	BlockQuote BlockKind = "quote"
)

// Block is one ordered piece of a Section's body content, materialized by the
// same read-only AST walk that builds Sections/Outline (see build.go). Its
// zero-value fields are `omitempty` so a Section with no blocks serializes
// without a `blocks` key, and a block that does not use a field (e.g. a
// paragraph has no Language) omits it -- keeping v2 JSON a strict, additive
// superset of v1.
//
// This is the ONE shared schema-v2 surface both downstream objectives consume:
// Objective 6's PPTX writer maps paragraph/list/heading blocks into editable
// text-box shapes; Objective 7's DART-04 serializes code (source+language) and
// math (rawTeX+display) blocks verbatim for native Flutter rendering. Both the
// raw code source and the raw math TeX are JSON-native and lossless here --
// neither is recoverable from the rendered Output.HTML.
type Block struct {
	// ID is a stable block identifier for apply_ops / editor addressing
	// (schema v5). Empty on Build()-produced blocks (Markdown has none);
	// agents and ApplyOps assign ids when mutating a live document.
	// omitempty so a Build render without ids stays a strict v4 superset
	// apart from SchemaVersion.
	ID string `json:"id,omitempty"`

	// Kind is the block's discriminator (paragraph/list/code/math/heading).
	Kind BlockKind `json:"kind"`

	// Text is the block's primary text payload: paragraph/heading plain text,
	// a code block's RAW source (pre-chroma), or a math construct's RAW TeX.
	// Empty for a pure list block (its content lives in Items).
	Text string `json:"text,omitempty"`

	// Level is the heading level 1-6 (heading blocks only).
	Level int `json:"level,omitempty"`

	// Language is the fenced-code info-string language, e.g. "go" (code blocks
	// only; empty for an indented code block or a fence with no info string).
	Language string `json:"language,omitempty"`

	// Display reports display ($$…$$) vs. inline ($…$) math (math blocks only).
	Display bool `json:"display,omitempty"`

	// Ordered reports a numbered vs. bullet list (list blocks only).
	Ordered bool `json:"ordered,omitempty"`

	// Items are the entries of a list block, in document order (list blocks
	// only).
	Items []ListItem `json:"items,omitempty"`

	// Headers are a table block's header-row cell texts, left to right
	// (table blocks only). A header-less table omits this.
	Headers []string `json:"headers,omitempty"`

	// Rows are a table block's body rows, each a left-to-right slice of cell
	// texts (table blocks only). Ragged rows are preserved as authored --
	// GFM pads/truncates at render time, but the model reports what was
	// written so an exporter can decide for itself.
	Rows [][]string `json:"rows,omitempty"`

	// Aligns are a table block's per-column alignments, one per column, each
	// "left" / "right" / "center" / "" (unspecified) (table blocks only).
	// Load-bearing for convert/pptx, convert/docx and a future convert/xlsx:
	// a right-aligned numeric column must survive into the export.
	Aligns []string `json:"aligns,omitempty"`

	// Src is an image block's destination URL (image blocks only). The alt
	// text lives in Text and the optional title in Title.
	Src string `json:"src,omitempty"`

	// Title is an image block's optional title attribute -- the
	// `![alt](src "title")` third argument (image blocks only).
	Title string `json:"title,omitempty"`

	// Span is the byte range of the Markdown source this block came from
	// (schema v4). Taken from the node's own line segments where it has them,
	// otherwise derived from its positioned descendants.
	//
	// What it covers, per kind -- stated rather than left to be discovered:
	//   - paragraph/heading: the node's text lines. An ATX heading's span
	//     EXCLUDES its "## " prefix (a setext heading has no prefix); the
	//     prefix is deliberately not reconstructed by arithmetic, which would
	//     break on setext.
	//   - code: the content BETWEEN the fences, not the ``` fences themselves
	//     (right for "show me this code", not for "replace this block").
	//   - list/table/quote: derived from the positioned descendants, so the
	//     span DOES include the source's "- " bullets, "|" pipes and "> "
	//     markers even though the extracted Items/Rows/Text do not.
	//   - image: an image is an INLINE node with no lines of its own, so its
	//     span covers its ALT TEXT range only, not the surrounding
	//     `![...](...)` construct. An image with empty alt text has no
	//     positioned descendant at all and carries a nil Span.
	//
	// Nil when no position could be determined. See Span.
	Span *Span `json:"span,omitempty"`
}

// ListItem is one entry of a list Block, carrying its editable Text and its
// 0-based nesting Level (0 = top level, 1 = one level of indentation, ...).
type ListItem struct {
	// Text is the item's editable plain text.
	Text string `json:"text"`

	// Level is the item's 0-based nesting depth within the list.
	Level int `json:"level,omitempty"`
}

// Document is the root of the docmodel: deck-level Meta, the ordered list
// of Sections (the generic unit a profile happens to give its own presentation
// name to -- see chase/profile and ARCHITECTURE.md Anti-Pattern 1, which is
// exactly why this package never bakes in a profile-specific name for it),
// and a flat Outline of every heading in the deck.
type Document struct {
	// SchemaVersion is always the package constant SchemaVersion; present
	// as a field (not just a package constant) so it round-trips through
	// JSON along with the rest of the document.
	SchemaVersion string `json:"schemaVersion"`

	// Meta carries deck-level metadata resolved from front matter.
	Meta Meta `json:"meta"`

	// Sections is one entry per Section, in document order. Nil (not an
	// empty, non-nil slice) for a deck with no Sections at all, e.g. an
	// empty source document.
	Sections []Section `json:"sections"`

	// Outline lists every heading across the whole Document in document
	// order, each entry carrying the SectionID of its owning Section --
	// i.e. already grouped by owning Section, without needing a nested
	// shape.
	Outline []OutlineEntry `json:"outline"`
}

// Section is the JSON-serializable projection of a *markdown.Section: its
// 1-based ID, its materialized directive-derived attributes, any speaker
// notes (non-directive HTML comments) found within it, and -- as of schema
// v2 (06-01) -- its ordered body-content Blocks.
//
// Blocks is ADDITIVE (schema v2): the ID/Attrs/Notes fields, their JSON tags,
// and their order are FROZEN v1 shape and unchanged. Blocks carries `omitempty`
// so a Section with no extracted body content serializes exactly as it did
// under v1 (no `blocks` key). Build materializes Blocks in the SAME single
// read-only AST walk that produces ID/Attrs/Notes/Outline -- never a second
// parse, never reverse-engineered from rendered HTML.
type Section struct {
	// ID is the Section's 1-based index, mirroring *markdown.Section.ID.
	ID int `json:"id"`

	// Attrs holds directive-derived attribute values (data-*/style/class/
	// lang/...), materialized from *markdown.Section.Attrs. A Go map is
	// fine here: declaration ORDER only matters for HTML attribute
	// emission (chase/markdown's job, done before this package ever sees
	// the Section), not for this JSON sink.
	Attrs map[string]string `json:"attrs,omitempty"`

	// Notes holds this Section's speaker notes: the trimmed body of every
	// HTML comment within it that did NOT resolve to a recognized
	// directive, in document order.
	Notes []string `json:"notes,omitempty"`

	// Blocks holds this Section's ordered body-content blocks (paragraph,
	// list, code, math, heading), in document order -- the editable/lossless
	// per-section surface schema v2 adds for downstream data sinks (PPTX text
	// shapes, native Flutter rendering). Nil (and omitted from JSON) for a
	// Section with no extracted body content.
	Blocks []Block `json:"blocks,omitempty"`

	// Span is the byte range of the Markdown source this Section covers
	// (schema v4), DERIVED from its positioned descendants (min start, max
	// stop) because a *markdown.Section is synthesized by the splitter and has
	// no source construct -- and therefore no line segments -- of its own.
	//
	// It covers the Section's CONTENT: NOT the "---" separator that introduced
	// it, and not the blank lines around it. Where `headingDivider:` synthesized
	// the break there is no separator in the source at all, so there is nothing
	// to include even in principle. Both are correct for "scroll to this
	// section"; neither is correct for "select this whole section including its
	// delimiter".
	//
	// Nil for a Section with no positioned content at all. See Span.
	Span *Span `json:"span,omitempty"`
}

// Meta carries deck-level metadata resolved from the document's front
// matter (theme, size, class, and any other front-matter directive
// present) -- read from the parser.Context state chase/markdown's
// front-matter block parser already produced, never re-derived from
// rendered HTML.
type Meta struct {
	// Directives holds every front-matter key/value pair, materialized to
	// a string. A Go map is fine here for the same reason as
	// Section.Attrs: declaration order does not matter for a JSON sink.
	Directives map[string]string `json:"directives,omitempty"`
}

// OutlineEntry is one heading in Document.Outline: its owning Section's ID,
// heading Level (1-6), materialized Text, and AutoHeadingID Slug.
type OutlineEntry struct {
	// SectionID is the ID of the Section that owns this heading.
	SectionID int `json:"sectionId"`

	// Level is the heading's level, 1-6.
	Level int `json:"level"`

	// Text is the heading's materialized text content -- an owned string,
	// never a text.Segment held past the AST walk that produced it.
	Text string `json:"text"`

	// Slug is the heading's AutoHeadingID "id" attribute value, or "" if
	// the heading had none (e.g. an empty heading -- see build.go's
	// headingSlug).
	Slug string `json:"slug"`

	// Span is the byte range of the heading TEXT in the Markdown source
	// (schema v4) -- the "outline click -> scroll the source to that heading"
	// affordance this field exists for.
	//
	// For an ATX heading the span EXCLUDES the "## " prefix, because that is
	// what the heading node's own lines cover; a setext heading has no prefix
	// to exclude. The prefix is deliberately not added back by arithmetic,
	// which would be wrong for setext.
	//
	// Nil for a heading with no line information. See Span.
	Span *Span `json:"span,omitempty"`
}
