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
	"reflect"
	"strings"
	"testing"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"

	"go.aocyber.ai/eden-press/press/sanitize"

	// Blank import: registers profiles/paged via its init() side-effect, so
	// TestRenderRecordsProfile can exercise a SECOND profile. press.go itself
	// blank-imports only profiles/slides, so without this line
	// Options{Profile: "paged"} would be an unknown profile here -- the exact
	// gap the bind/capi/core blank import closes for the C/Dart binding.
	_ "go.aocyber.ai/eden-press/profiles/paged"
)

// everyBatteryDeck exercises GFM tables, strikethrough (<s>), a heading slug,
// a shortcode + unicode emoji, a fenced code block (highlight), inline + block
// math, a speaker note, size + fit directives, and an XSS payload -- the single
// deck Test-list case 1 (and the option/sanitize cases) render through.
const everyBatteryDeck = "---\n" +
	"marp: true\n" +
	"---\n\n" +
	"# Title :smile: ❤\n\n" +
	"<!-- a speaker note -->\n\n" +
	"~~struck~~ text with `inline`\n\n" +
	"| col a | col b |\n" +
	"| ----- | ----- |\n" +
	"| 1     | 2     |\n\n" +
	"```go\nfunc main() {}\n```\n\n" +
	"Inline $x^2$ and block:\n\n" +
	"$$\\frac{1}{2}$$\n\n" +
	"# <!--fit--> Big heading\n\n" +
	"<script>alert('xss')</script>\n"

// TestRenderComposesEveryBattery is Test-list case 1: a single Render over a
// deck using every battery returns non-empty HTML/CSS/Model/Meta and the
// expected Comments, with each battery's output present in the composed HTML.
func TestRenderComposesEveryBattery(t *testing.T) {
	out, err := Render(everyBatteryDeck, Options{})
	if err != nil {
		t.Fatalf("Render: unexpected error: %v", err)
	}

	if out.HTML == "" {
		t.Error("Output.HTML is empty")
	}
	if out.CSS == "" {
		t.Error("Output.CSS is empty")
	}
	if out.Model == nil {
		t.Fatal("Output.Model is nil")
	}
	if len(out.Model.Sections) == 0 {
		t.Error("Output.Model.Sections is empty")
	}
	if out.Meta.Directives["marp"] != "true" {
		t.Errorf("Output.Meta.Directives[marp] = %q, want \"true\"", out.Meta.Directives["marp"])
	}

	// Every battery's fingerprint present in the composed HTML.
	wantBattery := map[string]string{
		"inline-SVG container (seam)": `<foreignObject`,
		"strikethrough <s> (03-03)":   `<s>struck</s>`,
		"twemoji <img> (03-04)":       `class="emoji"`,
		"GFM table":                   `<table>`,
		"highlight .hljs-* (03-05)":   `hljs-`,
		"autofit shrink wrap (03-07)": `marp-fit-shrink`,
		"autofit fit marker (03-07)":  `data-auto-scaling="fit"`,
		"inline MathML (03-06)":       `<math`,
		"block MathML (03-06)":        `display="block"`,
		"heading slug":                `id="title-smile`,
	}
	for label, want := range wantBattery {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("composed HTML missing %s: want substring %q", label, want)
		}
	}

	// Comments aggregated from the speaker note.
	if got := strings.Join(out.Comments, "|"); got != "a speaker note" {
		t.Errorf("Output.Comments = %v, want [\"a speaker note\"]", out.Comments)
	}
}

// TestRenderZeroValueOptions is Test-list case 1 (tail): press.Render(md,
// Options{}) works at the zero value -- no panic, no error, populated output.
func TestRenderZeroValueOptions(t *testing.T) {
	out, err := Render("# hello\n", Options{})
	if err != nil {
		t.Fatalf("Render(_, Options{}): %v", err)
	}
	if out.HTML == "" || out.CSS == "" || out.Model == nil {
		t.Fatalf("zero-value Options produced an under-populated Output: html=%d css=%d model=%v",
			len(out.HTML), len(out.CSS), out.Model)
	}
}

// TestOneParseInvariant is Test-list case 2: Render invokes the seam's
// ParseWithEngine EXACTLY once (the load-bearing one-parse-two-sinks
// invariant), proven by a counting wrapper around the parseWithEngine seam
// variable.
func TestOneParseInvariant(t *testing.T) {
	orig := parseWithEngine
	t.Cleanup(func() { parseWithEngine = orig })

	var calls int
	parseWithEngine = func(md string, engine goldmark.Markdown, inlineSVG bool) (*ast.Document, parser.Context) {
		calls++
		return orig(md, engine, inlineSVG)
	}

	if _, err := Render(everyBatteryDeck, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if calls != 1 {
		t.Errorf("Render called ParseWithEngine %d times, want exactly 1 (one-parse invariant violated)", calls)
	}
}

// TestThemeResolution is Test-list case 3: opts.Theme -> front-matter theme:
// -> "default", each resolving to distinct packed CSS.
func TestThemeResolution(t *testing.T) {
	deck := "---\nmarp: true\n---\n\n# Hi\n"

	def, err := Render(deck, Options{})
	if err != nil {
		t.Fatalf("default Render: %v", err)
	}
	gaia, err := Render(deck, Options{Theme: "gaia"})
	if err != nil {
		t.Fatalf("gaia Render: %v", err)
	}
	uncover, err := Render(deck, Options{Theme: "uncover"})
	if err != nil {
		t.Fatalf("uncover Render: %v", err)
	}

	// (a) explicit opts.Theme selects distinct themes.
	if def.CSS == gaia.CSS {
		t.Error("Options{} and Options{Theme:gaia} packed identical CSS; theme selection ignored")
	}
	if gaia.CSS == uncover.CSS {
		t.Error("gaia and uncover packed identical CSS; theme selection ignored")
	}

	// (b) front-matter theme: directive selects the theme when opts.Theme is "".
	fm, err := Render("---\nmarp: true\ntheme: uncover\n---\n\n# Hi\n", Options{})
	if err != nil {
		t.Fatalf("front-matter theme Render: %v", err)
	}
	if fm.Meta.Directives["theme"] != "uncover" {
		t.Errorf("front-matter theme: not materialized: Directives[theme]=%q", fm.Meta.Directives["theme"])
	}
	if fm.CSS != uncover.CSS {
		t.Error("front-matter theme: uncover did not pack the same CSS as Options{Theme:uncover}")
	}

	// (c) explicit opts.Theme OVERRIDES front-matter.
	override, err := Render("---\nmarp: true\ntheme: uncover\n---\n\n# Hi\n", Options{Theme: "gaia"})
	if err != nil {
		t.Fatalf("override Render: %v", err)
	}
	if override.CSS != gaia.CSS {
		t.Error("opts.Theme=gaia did not override front-matter theme: uncover")
	}

	// (d) default fallback is the bundled "default" theme, NOT the bare
	//     scaffold: resolveThemeName returns "default" and Options{} packs it.
	//     Slides supplies no profile default theme (""); a profile that does
	//     (paged: "document", D17) gets it before the "default" fallback.
	if got := resolveThemeName("", def.Meta, ""); got != "default" {
		t.Errorf("resolveThemeName(\"\", no-theme-meta, \"\") = %q, want \"default\"", got)
	}
	if got := resolveThemeName("", def.Meta, "document"); got != "document" {
		t.Errorf("resolveThemeName(\"\", no-theme-meta, \"document\") = %q, want \"document\"", got)
	}
	explicitDefault, err := Render(deck, Options{Theme: "default"})
	if err != nil {
		t.Fatalf("explicit default Render: %v", err)
	}
	if def.CSS != explicitDefault.CSS {
		t.Error("Options{} did not fall back to the bundled \"default\" theme")
	}
}

// TestOptionsHonored is Test-list case 4: NoHighlight, MathMode:"off", and a
// non-nil Sanitize override are each honored.
func TestOptionsHonored(t *testing.T) {
	// NoHighlight leaves code unhighlighted (no .hljs-* classes).
	code := "---\nmarp: true\n---\n\n```go\nfunc x() {}\n```\n"
	on, err := Render(code, Options{})
	if err != nil {
		t.Fatalf("highlight-on Render: %v", err)
	}
	off, err := Render(code, Options{NoHighlight: true})
	if err != nil {
		t.Fatalf("NoHighlight Render: %v", err)
	}
	if !strings.Contains(on.HTML, "hljs-") {
		t.Error("highlighting-on deck has no .hljs-* classes")
	}
	if strings.Contains(off.HTML, "hljs-") {
		t.Error("NoHighlight:true still emitted .hljs-* classes")
	}

	// MathMode:"off" leaves $x$ literal.
	mathMd := "---\nmarp: true\n---\n\n$x^2$\n"
	mOn, err := Render(mathMd, Options{})
	if err != nil {
		t.Fatalf("math-on Render: %v", err)
	}
	mOff, err := Render(mathMd, Options{MathMode: "off"})
	if err != nil {
		t.Fatalf("MathMode:off Render: %v", err)
	}
	if !strings.Contains(mOn.HTML, "<math") {
		t.Error("math-on deck emitted no <math>")
	}
	if strings.Contains(mOff.HTML, "<math") {
		t.Error("MathMode:off still emitted <math>")
	}
	if !strings.Contains(mOff.HTML, "$x^2$") {
		t.Error("MathMode:off did not leave $x^2$ literal")
	}

	// A non-nil Sanitize override REPLACES the built-in: a policy that strips
	// everything yields no <s> where the built-in would keep it.
	strikeMd := "---\nmarp: true\n---\n\n~~gone~~\n"
	builtin, err := Render(strikeMd, Options{})
	if err != nil {
		t.Fatalf("built-in sanitize Render: %v", err)
	}
	if !strings.Contains(builtin.HTML, "<s>gone</s>") {
		t.Error("built-in policy dropped <s> (unexpected)")
	}
	strict := stripEverythingPolicy()
	overridden, err := Render(strikeMd, Options{Sanitize: strict})
	if err != nil {
		t.Fatalf("override sanitize Render: %v", err)
	}
	if strings.Contains(overridden.HTML, "<s>") {
		t.Error("non-nil Sanitize override was ignored: <s> survived a strip-all policy")
	}
}

// TestSanitizeLast is Test-list case 5: a crafted <script> is absent from
// Output.HTML while every battery's markup survives -- proving sanitize ran
// LAST over the fully composed output, not before/per-node.
func TestSanitizeLast(t *testing.T) {
	out, err := Render(everyBatteryDeck, Options{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if strings.Contains(out.HTML, "<script") || strings.Contains(out.HTML, "alert('xss')") {
		t.Error("sanitize did not remove the <script> payload")
	}

	// Battery markup that must SURVIVE the last-pass sanitize.
	survive := []string{
		`<s>struck</s>`,     // strikethrough
		`class="emoji"`,     // twemoji <img>
		`<math`,             // MathML
		`hljs-`,             // chroma spans (remapped)
		`data-auto-scaling`, // fit marker
		`<foreignObject`,    // inline-SVG container (case restored)
		`viewBox=`,          // SVG viewBox (case restored)
	}
	for _, s := range survive {
		if !strings.Contains(out.HTML, s) {
			t.Errorf("sanitize stripped battery markup that must survive: %q", s)
		}
	}
}

// TestCommentsAggregation is Test-list case 6: Output.Comments equals the
// flattened Model.Sections[*].Notes in document order (pure aggregation).
func TestCommentsAggregation(t *testing.T) {
	deck := "---\nmarp: true\n---\n\n# One\n\n<!-- note A -->\n\n---\n\n# Two\n\n<!-- note B -->\n\n<!-- note C -->\n"
	out, err := Render(deck, Options{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var want []string
	for i := range out.Model.Sections {
		want = append(want, out.Model.Sections[i].Notes...)
	}
	if !reflect.DeepEqual(out.Comments, want) {
		t.Errorf("Output.Comments = %v, want flattened Model notes %v", out.Comments, want)
	}
	// And the actual content, in document order.
	if got := strings.Join(out.Comments, "|"); got != "note A|note B|note C" {
		t.Errorf("Output.Comments content/order = %q, want \"note A|note B|note C\"", got)
	}
}

// TestRenderRecordsProfile proves Render records the profile it ACTUALLY
// resolved on Output.Profile, so a downstream exporter (convert/pdf,
// convert/png) can resolve the same size table instead of guessing at one.
//
// The zero-value case is the load-bearing one: Options{} is the common call,
// and it must record "slides" -- Render's own resolved default -- not "". A
// recorded "" would leave every default render's geometry unresolvable, which
// is the bug this field exists to close.
func TestRenderRecordsProfile(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"zero value records the resolved default, not the empty string", Options{}, "slides"},
		{"explicit slides", Options{Profile: "slides"}, "slides"},
		{"explicit paged", Options{Profile: "paged"}, "paged"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render("# hello\n", tc.opts)
			if err != nil {
				t.Fatalf("Render(_, %+v): %v", tc.opts, err)
			}
			if out.Profile != tc.want {
				t.Errorf("Output.Profile = %q, want %q", out.Profile, tc.want)
			}
		})
	}
}

// TestRenderUnknownProfileStillErrors pins the pre-existing error path: an
// unregistered profile id is rejected by resolveProfile before anything is
// recorded, unchanged by the Output.Profile addition.
func TestRenderUnknownProfileStillErrors(t *testing.T) {
	out, err := Render("# hello\n", Options{Profile: "nonexistent"})
	if err == nil {
		t.Fatalf("Render(_, Options{Profile: \"nonexistent\"}) = %+v, want an error", out)
	}
	if !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("error = %q, want it to name the unknown profile", err)
	}
	if out.Profile != "" {
		t.Errorf("errored Render recorded Profile = %q, want \"\"", out.Profile)
	}
}

// stripEverythingPolicy returns a bluemonday policy that allows nothing, used
// to prove a non-nil Options.Sanitize override replaces the built-in.
func stripEverythingPolicy() *bluemonday.Policy {
	return bluemonday.NewPolicy()
}

// compile-time assurance the built-in pipeline is reachable from the test
// (documents the intended default path).
var _ = sanitize.Sanitize
