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
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"

	"go.aocyber.ai/eden-press/chase/markdown"
	"go.aocyber.ai/eden-press/chase/model"
	"go.aocyber.ai/eden-press/chase/profile"
	"go.aocyber.ai/eden-press/chase/theme"
	pmath "go.aocyber.ai/eden-press/press/math"
	"go.aocyber.ai/eden-press/press/sanitize"
	"go.aocyber.ai/eden-press/press/themes"

	// Blank import: registers profiles/slides (default) and profiles/paged
	// (docs / article alias) via init() side-effects, so a consumer that
	// only imports "press" still resolves both. Mirrors chase/chase.go's
	// blank import posture.
	_ "go.aocyber.ai/eden-press/profiles/paged"
	_ "go.aocyber.ai/eden-press/profiles/slides"
)

// defaultThemeName is the terminal fallback in press.Render's theme-resolution
// chain (opts.Theme -> front-matter theme: -> "default"). press deliberately
// deviates from chase/chase.go's scaffold-only default to match Marp Core,
// whose own default deck theme is the bundled "default" theme -- not a bare
// scaffold (03-RESEARCH Open Question #2, RESOLVED).
const defaultThemeName = "default"

// parseWithEngine is the seam's one-parse entrypoint, held in a package-level
// variable ONLY so the load-bearing one-parse invariant is runtime-verifiable:
// press_test.go's TestOneParseInvariant wraps this with a call counter to prove
// Render invokes it EXACTLY once (03-09 must_haves: "calling
// chase/markdown.ParseWithEngine EXACTLY ONCE"). Production behavior is
// identical to calling markdown.ParseWithEngineSVG directly -- the indirection
// is the idiomatic Go way to make a package-function call countable without a
// mock framework. The third argument is the inline-SVG decision, made per
// profile (see Render step 3).
var parseWithEngine = markdown.ParseWithEngineSVG

// Render is the PUBLIC one-parse-two-sinks composition (API-01): it wires every
// press battery (03-03 strikethrough, 03-04 emoji, 03-05 highlight, 03-06 math,
// 03-07 autofit) into a SINGLE battery-laden goldmark engine, runs the seam's
// ONE parse over md, and forks that single finalized *ast.Document to two
// independent sinks -- the HTML renderer (sink 1) and chase/model.Build (sink
// 2) -- then sanitizes the composed HTML LAST, packs the theme CSS from the
// embedded ThemeSet, and aggregates speaker notes into Comments.
//
// It is a SIBLING to chase.Render, never a wrapper: it builds its own engine
// via markdown.NewEngine(pressExtraOpts...) and drives markdown.ParseWithEngine
// directly, so chase/chase.go is neither called nor modified (03-09
// anti_patterns) and the one-parse invariant is preserved -- building on top of
// chase.Render would re-parse.
//
// CRITICAL: exactly ONE markdown.ParseWithEngine call happens in this function.
// Every render-affecting Option is honored; press.Render(md, Options{}) works at
// the zero value (Marp-Core-matching defaults).
func Render(md string, opts Options) (Output, error) {
	// 1. Resolve the active profile (supplies the unit element + scaffold CSS
	//    the ThemeSet is keyed on). opts.Profile == "" -> profile.Default()
	//    (today "slides", the only registered profile); a named profile is
	//    looked up in the registry.
	p, err := resolveProfile(opts.Profile)
	if err != nil {
		return Output{}, err
	}

	// 2. Build the battery-laden engine: ALL battery options bundled into
	//    NewEngine's extensibility hook. highlight is omitted when
	//    opts.NoHighlight is set; math honors opts.MathMode ("off" => no-op).
	engine := markdown.NewEngine(pressExtraOpts(opts, p)...)

	// 3. THE ONE PARSE -- via the seam, using the caller's battery engine.
	//    Whether each section is wrapped in <svg><foreignObject> is the
	//    PROFILE's call, not the seam's: slides (Container(true) differs from
	//    Container(false)) is wrapped, so the viewer can scale a fixed-aspect
	//    slide; paged (one container chain, fixed physical page boxes) is not,
	//    because its stylesheet is scoped to "div.edenpress-paged > section" and
	//    a wrapper would put every page outside that chain, so no page-box rule
	//    would match (D15).
	source := []byte(md)
	doc, pc := parseWithEngine(md, engine, profile.UsesInlineSVG(p))

	// 4. Sink 1: HTML, rendered from the SAME finalized doc by the SAME
	//    engine's renderer (never a second parse / md.Convert), then the
	//    chroma-short-class -> .hljs-* remap (a bounded post-render string
	//    pass; a no-op when highlighting was disabled and no chroma classes
	//    were emitted).
	var buf bytes.Buffer
	if err := engine.Renderer().Render(&buf, source, doc); err != nil {
		return Output{}, fmt.Errorf("press: Render: html render: %w", err)
	}
	html := remapHLJS(buf.String())

	// 5. Sink 2: the JSON-serializable Model, built from the SAME doc + pc.
	m := model.Build(doc, source, pc)

	// 6. Sanitize LAST -- over the COMPLETE composed HTML string only, never
	//    the CSS or Model. A nil opts.Sanitize selects the built-in always-on
	//    pipeline sanitize.Sanitize (Policy().Sanitize PLUS the SVG
	//    element/attribute case restoration that preserves the inline-SVG
	//    <svg><foreignObject><section> container chain -- bare
	//    Policy().Sanitize() would lowercase foreignObject/viewBox and break
	//    the deck). A non-nil override replaces the built-in wholesale; the
	//    caller then owns any case restoration it needs.
	if opts.Sanitize != nil {
		html = opts.Sanitize.Sanitize(html)
	} else {
		html = sanitize.Sanitize(html)
	}

	// 7. Pack CSS from the embedded ThemeSet, theme resolved
	//    opts.Theme -> front-matter theme: -> "default".
	css, err := packThemeCSS(p, pc, m.Meta, opts)
	if err != nil {
		return Output{}, err
	}

	return Output{
		HTML:     html,
		CSS:      css,
		Model:    m,
		Meta:     m.Meta,
		Comments: flattenNotes(m),
		// Record the profile that ACTUALLY produced this Output, by its own
		// ID rather than by opts.Profile -- the two differ in the case that
		// matters most: opts.Profile == "" resolves to defaultProfileName
		// ("slides"), and recording "" there would leave every default
		// render's geometry unresolvable downstream.
		Profile: p.ID(),
	}, nil
}

// defaultProfileName is press's own documented default profile. It is resolved
// BY NAME rather than through profile.Default(), which returns the
// first-registered profile.
//
// That distinction became load-bearing the moment a second profile existed.
// profile.Default()'s "first registered wins" rule is deterministic only for a
// fixed registration order, and registration happens in package init() — so
// which profile is first depends on the final binary's import graph, not on
// anything the caller decides. A program that merely imports profiles/paged
// anywhere could silently flip the default for every press.Render(md,
// Options{}) call in the process. Pinning the name here makes press's default
// a property of press, which is what its documentation always promised.
const defaultProfileName = "slides"

// resolveProfile maps opts.Profile to a chase/profile.Profile: "" resolves
// press's own default (profiles/slides, imported for its init side-effect
// above); a non-empty id is looked up in the registry and errors if
// unregistered.
func resolveProfile(id string) (profile.Profile, error) {
	if id == "" {
		if p, ok := profile.Get(defaultProfileName); ok {
			return p, nil
		}
		// Fall back to the registry default only if the named default is
		// somehow absent, so a consumer that vendors a different profile set
		// still gets something rather than a hard error.
		p := profile.Default()
		if p == nil {
			return nil, fmt.Errorf("press: Render: no profile registered (import a profiles/* package for its init side-effect)")
		}
		return p, nil
	}
	// Docs clients often write profile: article; map it onto the paged
	// long-form profile when registered.
	if id == "article" {
		if p, ok := profile.Get("paged"); ok {
			return p, nil
		}
	}
	p, ok := profile.Get(id)
	if !ok {
		return nil, fmt.Errorf("press: Render: unknown profile %q", id)
	}
	return p, nil
}

// pressExtraOpts assembles the six battery goldmark.Options folded into the one
// engine, honoring the Options that gate them:
//   - strikethrough (03-03): always on (Marp emits <s>).
//   - emoji (03-04): always on (twemoji <img> + unicode-literal parser).
//   - highlight (03-05): on UNLESS opts.NoHighlight; opts.HighlightStyle names
//     the chroma style ("" => library default).
//   - math (03-06): pmath.Option(opts.MathMode) -- "off" yields a no-op Option
//     so $x$ stays literal.
//   - autofit (03-07): always on (fit markers + shrink wrapper).
//
// Each battery owns a distinct NodeKind/priority (strikethrough
// KindStrikethrough, emoji east.Emoji, highlight KindFencedCodeBlock, math its
// own node, autofit a transformer + wrapper), so bundling them into one engine
// never double-registers a renderer.
func pressExtraOpts(opts Options, p profile.Profile) []goldmark.Option {
	extra := []goldmark.Option{
		strikethroughOption(),
		emojiOption(),
		// The active profile owns the container <div>'s class, so its
		// Container() selector matches the markup actually emitted. Without
		// this the class is a "marpit" literal and any non-slides profile
		// generates CSS that cannot match its own DOM.
		goldmark.WithRendererOptions(markdown.WithContainerClass(p.ContainerClass())),
	}
	if !opts.NoHighlight {
		extra = append(extra, highlightOption(opts.HighlightStyle))
	}
	extra = append(extra,
		pmath.Option(opts.MathMode),
		autofitOption(),
	)
	return extra
}

// packThemeCSS packs the resolved theme's CSS from the embedded ThemeSet,
// mirroring chase/chase.go's packCSS profile->primitives bridge but selecting a
// user/front-matter/default NAMED theme rather than the bare scaffold identity.
//
// The theme name resolves in the fixed order opts.Theme -> Meta.Directives
// ["theme"] (the deck's own front-matter theme: directive) -> defaultThemeName.
// The ThemeSet is built once from the active profile's unit element + scaffold /
// advanced-background CSS + bare-@size fallback table (press/themes.ThemeSet).
//
// inlineSVG is derived from pc (svgEnabled) OR'd with opts.InlineSVG, for a
// profile that has an inline-SVG layer (slides): Render enables inline-SVG mode
// for it, so the rendered HTML ALWAYS carries the <svg><foreignObject> wrapper
// and the packed CSS must match that container chain, so it is packed
// InlineSVG=true to stay consistent (a non-SVG pack would scope theme rules onto
// div.marpit and mis-target the SVG-wrapped sections). This mirrors
// chase/chase.go's packCSS, which likewise reads inline-SVG mode from pc rather
// than trusting a caller flag.
//
// A profile with a single container chain (paged) never has an SVG layer:
// opts.InlineSVG does not turn one on, and its page box is stated here. The
// packed theme carries the theme's own section size (the bundled themes are
// 1280x720 slide themes), so the profile's resolved page size is appended as the
// LAST unit rule -- same selector, later in the cascade -- and wins. The size is
// profile.ResolveSize(directive "size" -> the profile's table -> its default),
// the same resolver convert/png and convert/pdf capture at.
func packThemeCSS(p profile.Profile, pc parser.Context, meta model.Meta, opts Options) (string, error) {
	scaffoldCSS := p.Scaffold(false)
	// Recover the advanced-background CSS suffix the same way chase/chase.go's
	// packCSS does: Scaffold(true) is Scaffold(false) with the advanced-bg CSS
	// appended, and NewThemeSet/Pack want it as a separate string spliced in
	// only on InlineSVG packs.
	advancedBackgroundCSS := strings.TrimPrefix(p.Scaffold(true), scaffoldCSS)

	ts, err := themes.ThemeSet(p.UnitElement(), scaffoldCSS, advancedBackgroundCSS, p.Sizes().ByName,
		p.Container(false), p.Container(true))
	if err != nil {
		return "", fmt.Errorf("press: Render: build theme set: %w", err)
	}

	// Register any caller-supplied custom themes (opts.ThemeCSS, TRD 04-01)
	// through the EXACT SAME intake path the 3 embedded themes just used
	// above: theme.Load (requires + records the block's own leading
	// `/* @theme name */` metadata) then ts.Add, so a custom theme becomes
	// just another registered name the opts.Theme/front-matter resolution
	// chain below can select. `range nil` is a no-op, so an empty/nil
	// opts.ThemeCSS (the Options{} zero value) leaves ts exactly as
	// themes.ThemeSet built it -- today's behavior, unchanged.
	for _, css := range opts.ThemeCSS {
		th, err := theme.Load(css, p.UnitElement(), p.Sizes().ByName)
		if err != nil {
			return "", fmt.Errorf("press: Render: load custom theme CSS: %w", err)
		}
		ts.Add(th) // registered under th.Name (= the block's own @theme name)
	}

	name := resolveThemeName(opts.Theme, meta)
	hasSVGLayer := profile.UsesInlineSVG(p)
	inlineSVG := hasSVGLayer && (svgEnabled(pc) || opts.InlineSVG)
	css, err := ts.Pack(name, theme.PackOptions{InlineSVG: inlineSVG})
	if err != nil {
		return "", fmt.Errorf("press: Render: pack theme %q: %w", name, err)
	}
	if !hasSVGLayer {
		css += "\n" + pageSizeRule(p, profile.ResolveSize(p, meta.Directives))
	}
	return css, nil
}

// pageSizeRule is the unit rule that gives every page the resolved physical
// size, scoped exactly as the packer scopes the unit's own rules: the profile's
// container chain, a child combinator, the unit element.
func pageSizeRule(p profile.Profile, size theme.Size) string {
	return fmt.Sprintf("%s > %s { width: %dpx; height: %dpx; }", p.Container(false), p.UnitElement(), size.WidthPx, size.HeightPx)
}

// resolveThemeName applies the fixed fallback chain opts.Theme -> front-matter
// theme: (Meta.Directives["theme"]) -> defaultThemeName. A non-empty opts.Theme
// overrides the front matter; absent both, the bundled "default" theme is used
// (NOT the bare scaffold -- press matches Marp Core's default-deck behavior).
func resolveThemeName(optTheme string, meta model.Meta) string {
	if optTheme != "" {
		return optTheme
	}
	if meta.Directives != nil {
		if fm := meta.Directives["theme"]; fm != "" {
			return fm
		}
	}
	return defaultThemeName
}

// svgEnabled reports whether pc carries markdown.SvgOptionsKey with
// Enabled=true -- the exact parser.Context value chase/markdown's svgTransformer
// reads to decide whether to wrap Sections in <svg><foreignObject>. packThemeCSS
// mirrors chase/chase.go's identical helper so the packed CSS's container chain
// always matches how doc was actually rendered.
func svgEnabled(pc parser.Context) bool {
	v, ok := pc.Get(markdown.SvgOptionsKey).(*markdown.SvgOptions)
	return ok && v != nil && v.Enabled
}
