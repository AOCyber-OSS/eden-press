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

package press_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/press"

	// Blank import registers profiles/paged. press itself only blank-imports
	// profiles/slides, so a consumer opting into a second profile opts in by
	// importing it -- the same registry contract slides uses.
	_ "go.aocyber.ai/eden-press/profiles/paged"
)

const pagedDoc = `# Q3 Report

Body text.

---

## Appendix

| Metric | Q3 |
|---|---|
| p95 | 550ms |
`

// TestPagedProfileEndToEnd is the payoff for the EPD-R2 container-class seam:
// selecting a non-slides profile must produce markup AND CSS that agree. It
// would have passed vacuously before the seam existed, because the container
// class was a "marpit" literal regardless of profile.
func TestPagedProfileEndToEnd(t *testing.T) {
	out, err := press.Render(pagedDoc, press.Options{Profile: "paged"})
	if err != nil {
		t.Fatalf("Render(paged): %v", err)
	}

	if !strings.Contains(out.HTML, `<div class="edenpress-paged">`) {
		t.Errorf("paged container class not emitted: %s", out.HTML)
	}
	if strings.Contains(out.HTML, `class="marpit"`) {
		t.Errorf("marpit container leaked into paged output: %s", out.HTML)
	}

	// The scaffold's paged-specific CSS reaches the packed stylesheet,
	// INCLUDING its @media print block — the rules that enforce one section
	// per physical page, which the packer used to discard silently.
	for _, want := range []string{
		"edenpress-paged",
		"counter-increment: edenpress-page",
		"@media print {",
		"page-break-after: always",
	} {
		if !strings.Contains(out.CSS, want) {
			t.Errorf("packed CSS missing %q", want)
		}
	}

	// Rules are scoped under THIS profile's container, not the slides chain.
	if strings.Contains(out.CSS, "div.marpit") {
		t.Errorf("paged CSS is scoped under the slides container chain: %s", firstLines(out.CSS, 3))
	}
	if !strings.Contains(out.CSS, "div.edenpress-paged") {
		t.Errorf("paged CSS is not scoped under its own container: %s", firstLines(out.CSS, 3))
	}

	// Both sections survive, and the model is profile-agnostic as designed.
	if got := len(out.Model.Sections); got != 2 {
		t.Errorf("sections = %d, want 2", got)
	}
}

// TestPagedPipelineGapsClosed guards the two chase/theme packing defects that
// EPD-R2 uncovered and fixed, so a regression is caught here rather than
// showing up as silently wrong CSS.
//
// Gap 1 — block at-rule contents were discarded. chase/theme/parse.go recorded
// only an at-rule's OPENING into Stylesheet.Atoms and dropped its body, so
// `@media print { ... }` packed to the literal, invalid `@media print;`. It hit
// a shipped theme: themes/uncover.css's @media print block lost its
// print-specific pagination styling in every packed stylesheet. It survived
// because profiles/slides' scaffold contains zero at-rules, so nothing
// exercised the path until profiles/paged needed print rules.
//
// Gap 2 — Profile.Container() was never wired into the packing pipeline. Its
// own doc claimed it "de-hardcodes chase/theme/selector/scope.go's
// inlineSVGChain / nonSVGChain", but press never passed it, so those package
// vars stayed the real source of truth and paged's rules were scoped under
// "div.marpit > svg > foreignObject" — CSS that could never match its own
// markup. Closed by ThemeSet.SetContainerChains.
func TestPagedPipelineGapsClosed(t *testing.T) {
	// Gap 1, on the shipped theme that was actually affected.
	uncover, err := press.Render(pagedDoc, press.Options{Theme: "uncover"})
	if err != nil {
		t.Fatalf("Render(uncover): %v", err)
	}
	if strings.Contains(uncover.CSS, "@media print;") {
		t.Error("GAP 1 REGRESSION: at-rule block emitted in the invalid statement form")
	}
	if !strings.Contains(uncover.CSS, "@media print {") {
		t.Error("GAP 1 REGRESSION: uncover's @media print block lost its body again")
	}

	// Gap 2: each profile's rules scope under its OWN container chain.
	slides, err := press.Render(pagedDoc, press.Options{Profile: "slides"})
	if err != nil {
		t.Fatalf("Render(slides): %v", err)
	}
	if !strings.Contains(slides.CSS, "div.marpit") {
		t.Error("GAP 2 REGRESSION: slides no longer scopes under the marpit chain")
	}

	paged, err := press.Render(pagedDoc, press.Options{Profile: "paged"})
	if err != nil {
		t.Fatalf("Render(paged): %v", err)
	}
	if strings.Contains(paged.CSS, "div.marpit") {
		t.Error("GAP 2 REGRESSION: paged is scoped under the slides container chain")
	}
}

// firstLines returns at most n lines of s, for readable failure output.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// --- D15: the paged markup and CSS agree, at the profile's page size -----------
//
// TestPagedProfileEndToEnd above only ever checked substrings, which is why a
// profile whose CSS matched NOTHING in its own markup survived: Render wrapped
// every section in <svg><foreignObject> while the packed CSS was scoped to
// "div.edenpress-paged > section", and the theme's 1280x720 section size won
// over the profile's 794x1123. The tests below parse the HTML and compare real
// ancestor chains against the selector chain the CSS uses.

// pagedThreeMD is a three-page document whose middle page is EMPTY: the case
// that exported as a 0px-tall page and failed PNG export.
const pagedThreeMD = "# First\n\nalpha\n\n---\n\n---\n\n# Third\n\nomega\n"

func pagedThreeModel(directives map[string]string) *model.Document {
	d := &model.Document{SchemaVersion: model.SchemaVersion, Sections: []model.Section{
		{ID: 1, Blocks: []model.Block{
			{Kind: model.BlockHeading, Level: 1, Text: "First"},
			{Kind: model.BlockParagraph, Text: "alpha"},
		}},
		{ID: 2},
		{ID: 3, Blocks: []model.Block{
			{Kind: model.BlockHeading, Level: 1, Text: "Third"},
			{Kind: model.BlockParagraph, Text: "omega"},
		}},
	}}
	d.Meta.Directives = directives
	return d
}

// frontMatter renders directives as a Markdown front-matter block ("" for none),
// keys sorted so the source is deterministic.
func frontMatter(directives map[string]string) string {
	if len(directives) == 0 {
		return ""
	}
	keys := make([]string, 0, len(directives))
	for k := range directives {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range keys {
		b.WriteString(k + ": " + directives[k] + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// pagedEntryPoints are the two public render paths. They share one pipeline, so
// each must be held to the same geometry contract.
var pagedEntryPoints = []struct {
	name   string
	render func(t *testing.T, opts press.Options, directives map[string]string) press.Output
}{
	{"Render", func(t *testing.T, opts press.Options, directives map[string]string) press.Output {
		t.Helper()
		out, err := press.Render(frontMatter(directives)+pagedThreeMD, opts)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		return out
	}},
	{"RenderModel", func(t *testing.T, opts press.Options, directives map[string]string) press.Output {
		t.Helper()
		out, err := press.RenderModel(pagedThreeModel(directives), opts)
		if err != nil {
			t.Fatalf("RenderModel: %v", err)
		}
		return out
	}},
}

// htmlSections parses rendered HTML and returns its <section> elements in
// document order.
func htmlSections(t *testing.T, h string) []*html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader("<!doctype html><html><body>" + h + "</body></html>"))
	if err != nil {
		t.Fatalf("parse rendered HTML: %v", err)
	}
	var secs []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "section" {
			secs = append(secs, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return secs
}

// cssUnitChainRE captures the container chain a packed rule puts before the
// unit element: "div.edenpress-paged" for paged, "div.marpit > svg >
// foreignObject" for slides. It only reads rules that START a line, so rules
// nested inside an at-rule on one line are not part of the sample.
var cssUnitChainRE = regexp.MustCompile(`(?m)^(div\.[\w-]+(?: > [\w.-]+)*?) > section\b`)

// cssContainerChains returns the distinct container chains the packed CSS
// scopes its unit rules under, and how many rules it read.
func cssContainerChains(css string) (chains []string, rules int) {
	seen := map[string]bool{}
	for _, m := range cssUnitChainRE.FindAllStringSubmatch(css, -1) {
		rules++
		if !seen[m[1]] {
			seen[m[1]] = true
			chains = append(chains, m[1])
		}
	}
	sort.Strings(chains)
	return chains, rules
}

// compoundMatches reports whether element n satisfies one compound selector of
// the form tag.class1.class2.
func compoundMatches(n *html.Node, compound string) bool {
	parts := strings.Split(compound, ".")
	if n.Type != html.ElementNode || n.Data != parts[0] {
		return false
	}
	have := map[string]bool{}
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				have[c] = true
			}
		}
	}
	for _, want := range parts[1:] {
		if !have[want] {
			return false
		}
	}
	return true
}

// ancestorChainMatches reports whether the element's ancestors, nearest first,
// are exactly the child-combinator chain ("a > b > c": c is the parent).
func ancestorChainMatches(n *html.Node, chain string) bool {
	compounds := strings.Split(chain, " > ")
	anc := n.Parent
	for i := len(compounds) - 1; i >= 0; i-- {
		if anc == nil || !compoundMatches(anc, compounds[i]) {
			return false
		}
		anc = anc.Parent
	}
	return true
}

// TestPagedMarkupAndCSSAgree (Test list 1): for a three-section document with an
// empty middle section, every <section>'s real ancestor chain is the container
// chain the packed CSS puts before "section". Render and RenderModel, the
// `paged` profile and its `article` alias, and with Options.InlineSVG set (the
// profile, not the caller, decides whether paged pages are SVG-wrapped).
func TestPagedMarkupAndCSSAgree(t *testing.T) {
	for _, ep := range pagedEntryPoints {
		for _, profile := range []string{"paged", "article"} {
			for _, inline := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/InlineSVG=%v", ep.name, profile, inline)
				t.Run(name, func(t *testing.T) {
					out := ep.render(t, press.Options{Profile: profile, InlineSVG: inline}, nil)

					secs := htmlSections(t, out.HTML)
					if len(secs) != 3 {
						t.Fatalf("parsed %d <section> elements, want 3:\n%s", len(secs), out.HTML)
					}
					if strings.Contains(out.HTML, "<svg") || strings.Contains(out.HTML, "foreignObject") {
						t.Errorf("paged pages are wrapped in inline SVG; the packed CSS cannot match them:\n%s", out.HTML)
					}

					chains, rules := cssContainerChains(out.CSS)
					if rules < 5 {
						t.Fatalf("read only %d unit rules from the packed CSS; the sample is vacuous", rules)
					}
					if len(chains) != 1 {
						t.Fatalf("packed CSS scopes unit rules under %d container chains %q, want exactly 1", len(chains), chains)
					}
					for i, sec := range secs {
						if !ancestorChainMatches(sec, chains[0]) {
							t.Errorf("section %d is not a child of the CSS container chain %q; its parent is <%s>", i+1, chains[0], sec.Parent.Data)
						}
					}
				})
			}
		}
	}
}

// unitRuleRE matches a top-level `div.edenpress-paged > section { ... }` rule.
var unitRuleRE = regexp.MustCompile(`(?m)^div\.edenpress-paged > section \{([^}]*)\}`)

// lastUnitSize returns the width and height the LAST top-level unit rule in css
// declares, which is the one the cascade applies.
func lastUnitSize(t *testing.T, css string) (w, h int) {
	t.Helper()
	all := unitRuleRE.FindAllStringSubmatch(css, -1)
	if len(all) == 0 {
		t.Fatal("no top-level `div.edenpress-paged > section` rule in the packed CSS")
	}
	body := all[len(all)-1][1]
	dim := func(prop string) int {
		m := regexp.MustCompile(`(?:^|[;\s])` + prop + `:\s*(\d+)px`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("last unit rule declares no %s: {%s}", prop, body)
		}
		v, _ := strconv.Atoi(m[1])
		return v
	}
	return dim("width"), dim("height")
}

// TestPagedPackedCSSCarriesProfileSize (Test list 2): the page box is the
// profile's size, not the theme's 1280x720. A4 by default, the named size when a
// `size` directive picks one, A4 again for a name the table does not have.
func TestPagedPackedCSSCarriesProfileSize(t *testing.T) {
	cases := []struct {
		name       string
		directives map[string]string
		w, h       int
	}{
		{"default is A4", nil, 794, 1123},
		{"size: a4", map[string]string{"size": "a4"}, 794, 1123},
		{"size: letter", map[string]string{"size": "letter"}, 816, 1056},
		{"size: a5", map[string]string{"size": "a5"}, 559, 794},
		{"size: legal", map[string]string{"size": "legal"}, 816, 1344},
		{"size: tabloid", map[string]string{"size": "tabloid"}, 1056, 1632},
		{"size: nonsense falls back to A4", map[string]string{"size": "nonsense"}, 794, 1123},
	}
	for _, ep := range pagedEntryPoints {
		for _, profile := range []string{"paged", "article"} {
			for _, tc := range cases {
				t.Run(ep.name+"/"+profile+"/"+tc.name, func(t *testing.T) {
					out := ep.render(t, press.Options{Profile: profile}, tc.directives)
					w, h := lastUnitSize(t, out.CSS)
					if w != tc.w || h != tc.h {
						t.Errorf("last unit rule is %dx%d, want %dx%d", w, h, tc.w, tc.h)
					}
				})
			}
		}
	}
}

// slidesGoldenMD and slidesGoldenModel are one deck with an advanced background
// (the inline-SVG layered path), a list, and an EMPTY slide.
const slidesGoldenMD = "---\nmarp: true\npaginate: true\n---\n\n# Deck\n\nFirst slide.\n\n---\n\n![bg left](bg.png)\n\n## Second\n\n- a\n- b\n\n---\n\n---\n\n### Fourth\n"

func slidesGoldenModel() *model.Document {
	return &model.Document{SchemaVersion: model.SchemaVersion, Sections: []model.Section{
		{ID: 1, Blocks: []model.Block{
			{Kind: model.BlockHeading, Level: 1, Text: "Deck"},
			{Kind: model.BlockParagraph, Text: "First slide."},
		}},
		{ID: 2, Blocks: []model.Block{{Kind: model.BlockHeading, Level: 2, Text: "Second"}}},
		{ID: 3},
		{ID: 4, Blocks: []model.Block{{Kind: model.BlockParagraph, Text: "Fourth"}}},
	}}
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// slidesGolden are the sha256 digests of the slides HTML and packed CSS,
// captured from the code BEFORE the D15 paged fix. The fix must not move a
// single byte of slides output.
var slidesGolden = map[string]struct{ html, css string }{
	"Render/default":      {"5a7948c10f2db56266161b2edc5ffb9be644d730945349d73c362bd105a20551", "c1b666d1383e1641404a4d752e51a944057314544c196ab1d4b0f195c1a9eb74"},
	"Render/gaia":         {"5a7948c10f2db56266161b2edc5ffb9be644d730945349d73c362bd105a20551", "337a6349fffeafa054846267e699c662c1a4e65b8a70d13b3882d9b2aa2f9ff0"},
	"RenderModel/default": {"db35f5550c2966cde690c6af6a6b6df61be18d9044ad01d219f93f69727759b0", "c1b666d1383e1641404a4d752e51a944057314544c196ab1d4b0f195c1a9eb74"},
	"RenderModel/gaia":    {"db35f5550c2966cde690c6af6a6b6df61be18d9044ad01d219f93f69727759b0", "337a6349fffeafa054846267e699c662c1a4e65b8a70d13b3882d9b2aa2f9ff0"},
}

// TestSlidesOutputUnchangedByPagedFix (Test list 3): slides HTML and CSS are
// byte-identical to the goldens, for Render and RenderModel, default and gaia
// themes.
func TestSlidesOutputUnchangedByPagedFix(t *testing.T) {
	for _, theme := range []string{"default", "gaia"} {
		opts := press.Options{Profile: "slides", Theme: theme}

		r, err := press.Render(slidesGoldenMD, opts)
		if err != nil {
			t.Fatalf("Render slides/%s: %v", theme, err)
		}
		m, err := press.RenderModel(slidesGoldenModel(), opts)
		if err != nil {
			t.Fatalf("RenderModel slides/%s: %v", theme, err)
		}

		for _, c := range []struct {
			key string
			out press.Output
		}{{"Render/" + theme, r}, {"RenderModel/" + theme, m}} {
			want := slidesGolden[c.key]
			if got := sha(c.out.HTML); got != want.html {
				t.Errorf("%s: slides HTML changed: sha256 %s, golden %s", c.key, got, want.html)
			}
			if got := sha(c.out.CSS); got != want.css {
				t.Errorf("%s: slides CSS changed: sha256 %s, golden %s", c.key, got, want.css)
			}
		}
	}
}
