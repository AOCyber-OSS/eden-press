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

// Package paged implements chase/profile.Profile for long-form paged
// documents — reports and articles on A4/Letter stock, as opposed to
// profiles/slides' 16:9 deck.
//
// It is the SECOND Profile implementation, and the first real test of whether
// chase/profile is the extensible abstraction it was designed to be. It is
// mostly not: building this profile required adding
// Profile.ContainerClass() and a matching seam in chase/markdown, because the
// container's DOM class was a "marpit" literal in renderDocument while the
// interface supplied only the CSS selector. Everything else — the unit
// element, the size table, the pagination rule, the scaffold CSS — did slot in
// as designed.
//
// What this profile deliberately does NOT provide, because the Profile
// interface has no method for it: a table of contents. Page geometry, running
// headers/footers and page numbers are all expressible as @page rules and CSS
// counters in Scaffold(), and are implemented here. A TOC needs GENERATED DOM
// built from model.Document.Outline, and a Profile only supplies CSS-shaped
// tables and predicates — it never injects content. A TOC therefore belongs at
// the press/ level (or needs a content-injection method on the interface); it
// is not something this package could deliver by itself.
package paged

import (
	"go.aocyber.ai/eden-press/chase/profile"
	"go.aocyber.ai/eden-press/chase/theme"
)

// paged implements profile.Profile for long-form paged documents.
type paged struct{}

// New returns the paged Profile implementation.
func New() profile.Profile { return paged{} }

// init registers the paged profile at import time, so any binary importing
// this package can select it via profile.Get("paged") with no further wiring.
func init() { profile.Register(New()) }

// ID is the profile's registry key.
func (paged) ID() string { return "paged" }

// UnitElement is the element theme rules are scoped onto. It stays "section"
// — the same generic unit slides uses. chase/model deliberately carries no
// profile-specific name for a Section, and a page and a slide are the same
// structural unit split by the same "---" boundary; only their geometry and
// pagination differ.
func (paged) UnitElement() string { return "section" }

// ContainerClass is the class the rendered run's wrapping <div> carries.
// Deliberately NOT "marpit": this profile has nothing to do with Marp, and
// borrowing the name would leak Marp vocabulary into non-Marp output and make
// a stylesheet written for one profile silently apply to the other.
func (paged) ContainerClass() string { return "edenpress-paged" }

// Container returns the container-chain selector matching ContainerClass.
//
// The inlineSVG branch that slides uses does not apply here: Marpit's
// inline-SVG mode exists to scale a fixed-aspect slide to any viewport, which
// is meaningless for paged output whose whole point is fixed physical page
// dimensions. Both branches therefore return the same chain, so a caller that
// enables inline SVG against this profile gets CSS that still matches.
//
// That equality is load-bearing, not just defensive: it is how press.Render
// learns this profile has no SVG layer (profile.UsesInlineSVG compares the two
// chains), so it leaves each section a direct child of the container instead of
// wrapping it in <svg><foreignObject>. With a wrapper the sections sit outside
// this chain and none of the page-box rules in ScaffoldCSS would match (D15).
// Do not make the two branches differ without also giving the pages an SVG
// layer the stylesheet is scoped to.
func (paged) Container(bool) string { return "div.edenpress-paged" }

// Sizes returns the paged size table in CSS pixels at 96dpi, the unit
// chase/theme's Size carries.
//
// A4 is 210x297mm -> 210/25.4*96 = 793.7 -> 794 x 1123 px.
// Letter is 8.5x11in -> 816 x 1056 px.
// Both are rounded to whole pixels; sub-pixel page boxes cause visible
// 1px seams when a renderer rasterizes page edges.
//
// A4 is the default: it is the standard for the large majority of the world,
// and Letter remains one directive away.
func (paged) Sizes() profile.SizeTable {
	a4 := theme.Size{Name: "a4", WidthPx: 794, HeightPx: 1123}
	return profile.SizeTable{
		ByName: map[string]theme.Size{
			"a4":      a4,
			"letter":  {Name: "letter", WidthPx: 816, HeightPx: 1056},
			"a5":      {Name: "a5", WidthPx: 559, HeightPx: 794},
			"legal":   {Name: "legal", WidthPx: 816, HeightPx: 1344},
			"tabloid": {Name: "tabloid", WidthPx: 1056, HeightPx: 1632},
		},
		Default: a4,
	}
}

// Pagination describes where the page-number counter attaches. Like slides it
// targets the unit's ::after pseudo-element, but the default content
// expression is this profile's own CSS counter rather than Marpit's
// data-marpit-pagination attribute — a paged document numbers pages from a
// counter that increments per section, not from an attribute the Marp
// directive system stamps on.
func (paged) Pagination() profile.PaginationRule {
	return profile.PaginationRule{
		Selector:    "section::after",
		ContentExpr: "counter(edenpress-page)",
	}
}

// Scaffold returns the base CSS prepended before a packed theme's own rules.
//
// The inlineSVG argument is ignored: advanced backgrounds are a slide-deck
// construct built on Marpit's inline-SVG layer, which this profile does not
// use (see Container; press.Render does not emit one for it).
//
// The scaffold does not state the page SIZE. The packed theme carries its own
// section size, so press appends the resolved page size (Sizes, picked by the
// document's `size` directive) as the last unit rule, after the theme.
func (paged) Scaffold(bool) string { return ScaffoldCSS }
