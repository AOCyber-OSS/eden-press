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

package paged

import (
	"regexp"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/chase/profile"
	"go.aocyber.ai/eden-press/chase/theme"
)

// TestRegisteredUnderOwnID: importing this package registers "paged" without
// displacing "slides" (Register is additive; profile.Default stays slides).
func TestRegisteredUnderOwnID(t *testing.T) {
	p, found := profile.Get("paged")
	if !found {
		t.Fatal("profile.Get(paged) not found; init() did not register")
	}
	if p.ID() != "paged" {
		t.Errorf("ID = %q, want paged", p.ID())
	}
}

// TestContainerSelectorMatchesClass is the invariant the whole EPD-R2
// container-class seam exists to guarantee: the CSS selector a profile scopes
// rules under must match the class its markup actually carries. A profile
// whose Container() and ContainerClass() disagree generates a stylesheet that
// silently applies to nothing.
func TestContainerSelectorMatchesClass(t *testing.T) {
	p := New()
	class := p.ContainerClass()
	if class == "" {
		t.Fatal("ContainerClass is empty")
	}
	if class == "marpit" {
		t.Error("paged must not borrow the marpit container class")
	}
	for _, inlineSVG := range []bool{false, true} {
		sel := p.Container(inlineSVG)
		if !strings.Contains(sel, "."+class) {
			t.Errorf("Container(%v) = %q does not match ContainerClass %q", inlineSVG, sel, class)
		}
	}
}

// TestSizes: A4 default plus the named alternatives, at 96dpi CSS pixels.
func TestSizes(t *testing.T) {
	st := New().Sizes()
	if st.Default.Name != "a4" {
		t.Errorf("default size = %q, want a4", st.Default.Name)
	}
	for name, want := range map[string][2]int{
		"a4":     {794, 1123},
		"letter": {816, 1056},
	} {
		got, ok := st.ByName[name]
		if !ok {
			t.Errorf("size %q missing from the table", name)
			continue
		}
		if got.WidthPx != want[0] || got.HeightPx != want[1] {
			t.Errorf("size %q = %dx%d, want %dx%d", name, got.WidthPx, got.HeightPx, want[0], want[1])
		}
	}
	// Whole pixels only: a sub-pixel page box shows 1px seams when rasterized.
	for name, s := range st.ByName {
		if s.WidthPx <= 0 || s.HeightPx <= 0 {
			t.Errorf("size %q has a non-positive dimension: %dx%d", name, s.WidthPx, s.HeightPx)
		}
		if s.HeightPx <= s.WidthPx {
			t.Errorf("size %q is not portrait (%dx%d); paged stock is portrait by default", name, s.WidthPx, s.HeightPx)
		}
	}
}

// TestPaginationUsesACounter: page numbers must come from a CSS counter, not
// Marpit's data-marpit-pagination attribute, so numbering survives sections
// being added or reordered without re-running the directive pass.
func TestPaginationUsesACounter(t *testing.T) {
	pr := New().Pagination()
	if !strings.Contains(pr.ContentExpr, "counter(") {
		t.Errorf("ContentExpr = %q, want a CSS counter", pr.ContentExpr)
	}
	if strings.Contains(pr.ContentExpr, "marpit") {
		t.Errorf("ContentExpr borrows Marpit's pagination attribute: %q", pr.ContentExpr)
	}
	if !strings.HasPrefix(pr.Selector, New().UnitElement()) {
		t.Errorf("Selector %q does not target the unit element %q", pr.Selector, New().UnitElement())
	}
}

// flatRuleRE captures one flat `selector { declarations }` rule. The document
// theme has no nested at-rules (TestDefaultThemeIsDocument asserts that), so
// a flat match reads all of it.
var flatRuleRE = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)

var commentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)

// TestDefaultThemeIsDocument (D17, TRD 1026-30 Test list 8): the paged profile
// supplies its own default theme, `document`, through the optional
// profile.DefaultThemer interface. Its CSS loads through theme.Load (so its
// leading comment names it), sets the body type, and leaves the page geometry
// (width, height, padding) to the scaffold and press.
func TestDefaultThemeIsDocument(t *testing.T) {
	dt, ok := New().(interface{ DefaultTheme() (name, css string) })
	if !ok {
		t.Fatal("paged does not implement DefaultTheme() (name, css string)")
	}
	name, css := dt.DefaultTheme()
	if name != "document" {
		t.Errorf("DefaultTheme name = %q, want document", name)
	}
	th, err := theme.Load(css, New().UnitElement(), New().Sizes().ByName)
	if err != nil {
		t.Fatalf("theme.Load(DefaultTheme CSS): %v", err)
	}
	if th.Name != name {
		t.Errorf("the CSS names itself %q, DefaultTheme returns %q", th.Name, name)
	}

	body := commentRE.ReplaceAllString(css, "")
	if strings.Contains(body, "@media") || strings.Contains(body, "light-dark(") {
		t.Errorf("document theme uses @media or light-dark(); it is one flat light stylesheet")
	}
	sawBody := false
	for _, m := range flatRuleRE.FindAllStringSubmatch(body, -1) {
		isUnit := false
		for _, sel := range strings.Split(m[1], ",") {
			if strings.TrimSpace(sel) == "section" {
				isUnit = true
			}
		}
		if !isUnit {
			continue
		}
		for _, decl := range strings.Split(m[2], ";") {
			prop, value, _ := strings.Cut(decl, ":")
			prop, value = strings.TrimSpace(prop), strings.TrimSpace(value)
			switch {
			case strings.TrimPrefix(strings.TrimPrefix(prop, "min-"), "max-") == "width",
				strings.TrimPrefix(strings.TrimPrefix(prop, "min-"), "max-") == "height",
				prop == "padding", strings.HasPrefix(prop, "padding-"):
				t.Errorf("document theme sets %s: %s on section; the scaffold and press own the page geometry", prop, value)
			case prop == "font-size" && value == "11pt":
				sawBody = true
			}
		}
	}
	if !sawBody {
		t.Error("document theme does not set font-size: 11pt on section")
	}
}

// TestScaffoldCoversPagedConcerns: the scaffold must carry the three things a
// paged document needs and a slide deck does not — a print @page rule, the
// page counter, and per-section page breaks in print.
func TestScaffoldCoversPagedConcerns(t *testing.T) {
	css := New().Scaffold(false)
	for _, want := range []string{
		"@media print",
		"@page",
		// No explicit counter-reset: an un-reset CSS counter is implicitly
		// created at the root scope, so per-section increment numbers pages
		// from 1 without a container-level rule -- which the scoping pass
		// would prepend the container chain to a second time.
		"counter-increment: edenpress-page",
		"page-break-after: always",
		"display: table-header-group", // repeat table headers across pages
	} {
		if !strings.Contains(css, want) {
			t.Errorf("scaffold missing %q", want)
		}
	}
	// The scaffold scopes to this profile's own container, never marpit's.
	if strings.Contains(css, ".marpit") {
		t.Error("paged scaffold references the marpit container")
	}
	// inlineSVG is meaningless for paged output, so both branches agree.
	if New().Scaffold(true) != css {
		t.Error("Scaffold(true) differs from Scaffold(false); paged has no inline-SVG layer")
	}
}
