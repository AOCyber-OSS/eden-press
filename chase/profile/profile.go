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

package profile

import "go.aocyber.ai/eden-press/chase/theme"

// profile.go defines chase/profile's core abstraction: the Profile
// interface plus its two shared value types (SizeTable,
// PaginationRule). See doc.go for the package-level decision record
// (why chase/* is exported, and why Boundary()/Directives() from
// ARCHITECTURE.md Pattern 2's future-profile sketch are deliberately
// absent from this method set).
//
// Every method below is validated bottom-up: it de-hardcodes one
// SPECIFIC call-site that is hardcoded today in chase/theme (cited in
// each method's doc) and will become profile-supplied once 02-03
// wires profiles/slides in and 02-04 builds the chase entrypoint. No
// method exists without a named consumer in this objective.

// Profile is the output-profile abstraction (differentiator #1 —
// PROJECT.md, ARCHITECTURE.md Pattern 2): it owns everything that
// differs between output kinds ("Marp slides" today; "paged
// report"/"article"/"EPUB" later) and NOTHING else. Parsing, directive
// resolution, chase/model's docmodel schema, and chase/theme's
// CSS-scoping passes themselves are all profile-agnostic; a Profile
// only supplies the tables/predicates those passes consult.
type Profile interface {
	// ID is the profile's registry key (e.g. "slides"). Register,
	// Get, and Default (registry.go) index profiles by this value;
	// the chase entrypoint (02-04) uses it to select a profile by
	// name/flag.
	ID() string

	// UnitElement is the element chase/theme's scoping passes scope
	// theme rules onto — "section" for slides. De-hardcodes
	// chase/theme/selector/scope.go's package-level slideChain,
	// which is a fixed "section" literal today.
	UnitElement() string

	// Container returns the physical container-chain selector text a
	// unit is wrapped in for rendering — e.g. "div.marpit > svg >
	// foreignObject" when inlineSVG is true, "div.marpit" when it is
	// false. De-hardcodes chase/theme/selector/scope.go's
	// inlineSVGChain / nonSVGChain package-level vars today.
	Container(inlineSVG bool) string

	// ContainerClass is the class attribute of the <div> the whole
	// rendered run is wrapped in — "marpit" for slides. It MUST be the
	// class the selector Container() returns actually matches.
	//
	// This method exists because Container() alone was not enough to
	// describe a container: chase/markdown/render.go's renderDocument
	// wrote `<div class="marpit">` as a literal, so a profile returning
	// any other Container() selector would have generated CSS that could
	// not match its own markup. Adding a second Profile implementation
	// (profiles/paged) is what surfaced the gap — the first real test of
	// this abstraction, and evidence that "a new profile needs no change
	// to chase/*" held only while there was exactly one profile.
	//
	// Threaded to the renderer via markdown.WithContainerClass.
	ContainerClass() string

	// Sizes resolves the profile's named-size table plus its default
	// size (16:9 for slides). De-hardcodes chase/theme/meta.go's
	// bareSizeFallback map and chase/theme/stylesheet.go's
	// defaultWidthPx/defaultHeightPx constants today.
	Sizes() SizeTable

	// Pagination describes the render-time pagination CSS shape a
	// profile wants injected. De-hardcodes
	// chase/theme/pass_pagination.go's hardcoded "::after"-targeting
	// / "attr(data-marpit-pagination)" literals today.
	Pagination() PaginationRule

	// Scaffold returns the base CSS text to prepend before a packed
	// theme's own rules — the slide-reset scaffold, plus the
	// advanced-background support CSS appended when inlineSVG is
	// true. De-hardcodes chase/theme/scaffold.go's ScaffoldCSS /
	// AdvancedBackgroundCSS constants today.
	Scaffold(inlineSVG bool) string
}

// DefaultThemer is an OPTIONAL interface a Profile may also implement, to name
// the theme a document gets when it names none itself. It is not part of
// Profile, so existing profiles keep compiling; a renderer checks for it by
// type assertion (press does, in its theme packing):
//
//	if dt, ok := p.(profile.DefaultThemer); ok { name, css := dt.DefaultTheme() }
//
// DefaultTheme returns the theme's name and its CSS text. The CSS must name
// itself with a leading `@theme <name>` comment, as every theme does
// (chase/theme.Load enforces it), and that name must equal the name returned.
// The renderer registers it BEFORE any caller-supplied theme, so a caller theme
// of the same name replaces it, and an explicit theme choice (an option or the
// document's `theme` directive) always wins over it.
//
// profiles/paged implements it with a document theme (an 11pt body), because
// the bundled Marp themes are slide themes; profiles/slides does not, and keeps
// the Marp `default` theme.
type DefaultThemer interface {
	DefaultTheme() (name, css string)
}

// SizeTable is a profile's named-size table plus its default size —
// e.g. slides' {"4:3": ..., "16:9": ...} with Default set to the
// "16:9" entry. It reuses chase/theme.Size (the existing
// Name/WidthPx/HeightPx value type, see chase/theme/stylesheet.go)
// rather than inventing a parallel one — the one deliberate exception
// to chase/profile's "no chase/theme dependency beyond this" rule
// (chase/theme itself MUST NOT import chase/profile back; see doc.go
// for the one-way edge this preserves).
type SizeTable struct {
	ByName  map[string]theme.Size
	Default theme.Size
}

// PaginationRule is the render-time pagination CSS shape a Profile
// supplies: the pseudo-element Selector pagination counters attach to
// (e.g. "section::after" for slides) and the CSS ContentExpr used for
// the counter's default `content` value (e.g.
// "attr(data-marpit-pagination)"). This is deliberately the minimal
// shape needed by chase/theme/pass_pagination.go's one known call-site
// in this objective — recognizing/preserving the default pagination
// content declaration while neutralizing any other author-authored
// `content` on the same pseudo-element — not a general-purpose CSS
// rule model.
type PaginationRule struct {
	Selector    string
	ContentExpr string
}

// ResolveSize returns the size a document renders at under p: the document's
// `size` directive looked up in p's own size table, or the table's Default when
// the directive is absent, empty, or names a size p does not define.
//
// It is the ONE resolver. The CSS packer (press), the PNG exporter and the PDF
// exporter all call it, so the page box the stylesheet declares and the viewport
// and @page size the exporters capture at come from the same table and cannot
// disagree. directives is the document's resolved front-matter directive map
// (model.Meta.Directives); nil is fine.
func ResolveSize(p Profile, directives map[string]string) theme.Size {
	table := p.Sizes()
	if name := directives["size"]; name != "" {
		if sz, ok := table.ByName[name]; ok {
			return sz
		}
	}
	return table.Default
}

// UsesInlineSVG reports whether p lays its units out inside an inline-SVG layer
// (Marpit's <svg><foreignObject> wrapper, which scales a fixed-aspect slide to
// any viewport). A profile does exactly when its two container chains differ:
// Container(true) is the SVG chain, Container(false) the plain one. A profile
// whose chains are equal (paged, whose pages are physical paper) has no SVG
// layer, and a renderer must not wrap its units in one -- the packed CSS is
// scoped to the one chain, so a wrapper would put every unit outside it.
//
// Decided by comparing the chains rather than by a new Profile method, so the
// interface other profiles implement does not change.
func UsesInlineSVG(p Profile) bool {
	return p.Container(true) != p.Container(false)
}
