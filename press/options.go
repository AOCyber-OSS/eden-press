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
	"github.com/microcosm-cc/bluemonday"

	"go.aocyber.ai/eden-press/chase/model"
)

// Options configures press.Render (defined in TRD 03-09). This is the frozen
// API-03 input surface: it is defined once, here, and every wave-2 battery TRD
// and Objective 7's Dart binding builds against it, so it must stay stable --
// add a field only when a named consumer needs it, never speculatively.
//
// Its ZERO VALUE (press.Options{}) is a valid, Marp-Core-matching
// configuration: every field's zero value MEANS "do what Marp Core does by
// default", so press.Render(md, press.Options{}) just works. Two fields are
// deliberately shaped so their zero value is the SAFE default rather than an
// off/empty one -- NoHighlight is inverted (zero => highlighting ON) and
// Sanitize is nil-defaulted (zero => the built-in always-on policy applies).
type Options struct {
	// Theme selects the named theme. "" resolves the deck's own front-matter
	// `theme:` directive, and absent that, Marp Core's built-in "default"
	// theme. A non-empty value overrides the front matter.
	Theme string

	// Profile selects the chase/profile.Profile that supplies the unit element
	// and scaffold CSS. "" resolves profile.Default() (today: "slides", the
	// only registered profile).
	Profile string

	// InlineSVG selects the inline-<svg><foreignObject> container mode. The
	// 03-09 baseline turns this on to match Marp Core; the flag's own zero
	// value is false and is documented here without any speculative inversion.
	//
	// It only matters to a profile that HAS an inline-SVG layer (slides, whose
	// markup is always SVG-wrapped). A profile with one container chain (paged:
	// physical pages) is never SVG-wrapped, whatever this says -- the
	// profile decides, so its markup always matches its CSS.
	InlineSVG bool

	// MathMode selects the math rendering backend. "" resolves to "mathml"
	// (the CORE-07/08 baseline); "off" disables math rendering. Objective 8
	// hardens the fallback rules on top of this baseline.
	MathMode string

	// NoHighlight disables syntax highlighting when true. It is INVERTED on
	// purpose: the zero value (false) leaves highlighting ON, matching Marp
	// Core's default, so an Options{} caller keeps chroma highlighting without
	// having to opt in.
	NoHighlight bool

	// HighlightStyle names the chroma style used when highlighting is on. ""
	// resolves the default style pre-verified against the .hljs class remap
	// (CORE-04/05, TRD 03-05).
	HighlightStyle string

	// Sanitize is the HTML sanitization policy applied to the composed output.
	// nil (the zero value) does NOT disable sanitization -- it selects the
	// built-in always-on policy. CORE-05 sanitization is not optional; a
	// caller may supply a stricter/looser *bluemonday.Policy, but never turn
	// it off by leaving this nil.
	Sanitize *bluemonday.Policy

	// ThemeCSS is raw custom-theme CSS TEXT, additive to press.Render (TRD
	// 04-01): each entry is a self-contained theme stylesheet that names
	// itself via its own leading `/* @theme <name> */` comment (the same
	// requirement chase/theme.Load enforces for the 3 bundled themes). The
	// caller (e.g. the CLI's `--theme-set`) reads any theme file(s) from
	// disk; press/ NEVER touches the filesystem itself -- Render stays a
	// pure function of (md, opts). A nil/empty slice (the zero value) is a
	// no-op: no custom themes are registered, and press.Render(md,
	// Options{}) behaves exactly as it did before this field existed.
	//
	// Each entry is registered via the SAME chase/theme.Load +
	// ThemeSet.Add path the 3 embedded themes (default/gaia/uncover) use,
	// so a custom theme becomes selectable by name through the normal
	// opts.Theme / front-matter `theme:` resolution chain -- it is not a
	// separate lookup mechanism.
	ThemeCSS []string
}

// Output is press.Render's result -- the frozen API-03 output contract, the
// stable shape Objective 7's Dart binding serializes to JSON. It mirrors
// chase.Output (HTML/CSS/Model/Meta) and adds Comments, the flattened speaker
// notes already carried by the model.
type Output struct {
	// HTML is the battery-composed, post-sanitize rendered deck HTML.
	HTML string

	// CSS is the packed theme CSS (chase/theme ThemeSet.Pack output).
	CSS string

	// Model is the JSON-serializable document model, exactly as
	// chase/model.Build produces it -- unchanged by the press batteries.
	Model *model.Document

	// Meta is a convenience alias for Model.Meta (deck-level front-matter
	// metadata), surfaced top-level so a caller wanting only metadata need not
	// reach through Model.
	Meta model.Meta

	// Comments is the deck's speaker notes flattened into document order: the
	// aggregation of every model.Section.Notes entry across Model.Sections.
	// It is []string (never a nested shape) -- the exact contract the Dart
	// binding serializes. It is NOT a fresh AST walk; model.Section.Notes is
	// already populated by chase/model.Build.
	Comments []string

	// Profile is the ID of the chase/profile.Profile that produced this
	// Output ("slides", "paged"). Recorded so a downstream exporter can
	// resolve the SAME size table this render used, instead of guessing.
	//
	// Before this field existed, convert/pdf hardcoded profiles/slides' table
	// (so an A4 paged document exported at 1280x720) and convert/png called
	// profile.Default(), whose "first registered wins" rule depends on the
	// final binary's import graph rather than on anything the caller decided --
	// the hazard press.Render itself was fixed to avoid (see
	// defaultProfileName's comment in press.go).
	//
	// Render records the RESOLVED profile's own ID, never opts.Profile: an
	// empty opts.Profile resolves to "slides" by name, and recording "" would
	// leave the exporters guessing again for every default render.
	//
	// This is a press.Output field, NOT a model.Document field: it carries no
	// JSON schema contract and so needs no model.SchemaVersion bump.
	Profile string
}
