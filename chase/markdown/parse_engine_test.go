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

package markdown

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseWithEngineParityWithParse covers TRD 03-01 Test-list case 1: given
// the SAME caller-supplied engine shape the package builds internally
// (NewEngine()), ParseWithEngine returns a finalized *ast.Document with the
// SAME shape Parse returns -- identical Section count AND byte-identical
// rendered HTML -- proving the engine-parameterized copy is faithful and drops
// no transform.
func TestParseWithEngineParityWithParse(t *testing.T) {
	md := "<!-- _class: lead -->\n\n# Slide 1\n\nfirst\n\n---\n\n# Slide 2\n\nsecond\n"
	source := []byte(md)

	engine := NewEngine()
	docEng, _ := ParseWithEngine(md, engine)
	docDef, _ := Parse(md)

	if got, want := countSectionsAnywhere(docEng), countSectionsAnywhere(docDef); got != want {
		t.Fatalf("Section count via ParseWithEngine = %d, want %d (Parse)", got, want)
	}
	if got := countSectionsAnywhere(docEng); got != 2 {
		t.Fatalf("multi-slide deck: got %d Sections, want 2", got)
	}

	// Render both finalized trees -- the ParseWithEngine tree through the
	// CALLER's engine, the Parse tree through defaultEngine -- and require
	// byte-identical HTML. Same input + same engine config => same output;
	// any divergence means the copy dropped a pre-seed line or a transform.
	var bufEng bytes.Buffer
	if err := engine.Renderer().Render(&bufEng, source, docEng); err != nil {
		t.Fatalf("render ParseWithEngine doc: %v", err)
	}
	var bufDef bytes.Buffer
	if err := defaultEngine.Renderer().Render(&bufDef, source, docDef); err != nil {
		t.Fatalf("render Parse doc: %v", err)
	}
	if bufEng.String() != bufDef.String() {
		t.Fatalf("ParseWithEngine render != Parse render:\nengine:\n%s\ndefault:\n%s", bufEng.String(), bufDef.String())
	}
}

// TestParseWithEnginePreSeedsContext covers TRD 03-01 Test-list case 2:
// ParseWithEngine pre-seeds the parser.Context IDENTICALLY to Parse --
// SvgOptionsKey with Enabled=true always, and, for a deck whose front matter
// carries headingDivider: 2, the resolved HeadingDividerKey []int{1,2}.
func TestParseWithEnginePreSeedsContext(t *testing.T) {
	// SvgOptionsKey: enabled on a plain deck, exactly as Parse sets it.
	{
		_, pc := ParseWithEngine("# Hello\n", NewEngine())
		v, ok := pc.Get(SvgOptionsKey).(*SvgOptions)
		if !ok || v == nil {
			t.Fatalf("SvgOptionsKey missing/wrong type on ParseWithEngine context: %#v", pc.Get(SvgOptionsKey))
		}
		if !v.Enabled {
			t.Fatalf("SvgOptionsKey.Enabled = false, want true (Parse pre-seed parity)")
		}
	}

	// HeadingDividerKey: resolved to []int{1,2} for headingDivider: 2, and
	// identical to what Parse resolves for the same source.
	{
		md := "---\nheadingDivider: 2\n---\n\n# H1\n\n## H2\n"
		_, pcEng := ParseWithEngine(md, NewEngine())
		_, pcDef := Parse(md)

		levelsEng, okEng := pcEng.Get(HeadingDividerKey).([]int)
		if !okEng {
			t.Fatalf("HeadingDividerKey missing/wrong type on ParseWithEngine context: %#v", pcEng.Get(HeadingDividerKey))
		}
		levelsDef, okDef := pcDef.Get(HeadingDividerKey).([]int)
		if !okDef {
			t.Fatalf("HeadingDividerKey missing/wrong type on Parse context: %#v", pcDef.Get(HeadingDividerKey))
		}
		if want := []int{1, 2}; !intsEqual(levelsEng, want) {
			t.Fatalf("ParseWithEngine HeadingDividerKey = %v, want %v", levelsEng, want)
		}
		if !intsEqual(levelsEng, levelsDef) {
			t.Fatalf("ParseWithEngine HeadingDividerKey %v != Parse %v", levelsEng, levelsDef)
		}
	}
}

// TestParseWithEngineIsAdditiveNonBreaking covers TRD 03-01 Test-list case 3:
// the existing seam is untouched, so Render(md, nil) still succeeds and
// produces output byte-identical to driving the NEW ParseWithEngine seam
// through the same engine. (git diff on seam.go/chase.go being empty plus the
// full ./chase/... suite staying green complete the "before == after" proof.)
func TestParseWithEngineIsAdditiveNonBreaking(t *testing.T) {
	md := "<!-- _class: lead -->\n\n# Slide 1\n\nfirst\n\n---\n\n# Slide 2\n\nsecond\n"
	source := []byte(md)

	// Existing seam entrypoint, unchanged.
	seamOut, err := Render(md, nil)
	if err != nil {
		t.Fatalf("existing Render(md, nil): %v", err)
	}

	// New additive seam driven manually through NewEngine().
	engine := NewEngine()
	doc, _ := ParseWithEngine(md, engine)
	var buf bytes.Buffer
	if err := engine.Renderer().Render(&buf, source, doc); err != nil {
		t.Fatalf("render via ParseWithEngine: %v", err)
	}

	if buf.String() != seamOut {
		t.Fatalf("ParseWithEngine seam output != existing Render output (seam should be unaffected):\nnew:\n%s\nexisting:\n%s", buf.String(), seamOut)
	}
}

// TestParseWithEngineSVGMode: the caller decides whether sections are wrapped in
// inline SVG. inlineSVG=true is exactly ParseWithEngine (the wrapper, the
// SvgOptionsKey pre-seed); inlineSVG=false leaves every section a direct child of
// the container, which is the only shape a profile with a single container chain
// (paged) has CSS for. Either way the front-matter headingDivider pre-seed holds.
func TestParseWithEngineSVGMode(t *testing.T) {
	md := "# One\n\n---\n\n---\n\n# Three\n"
	render := func(inlineSVG bool) (string, bool) {
		engine := NewEngine()
		doc, pc := ParseWithEngineSVG(md, engine, inlineSVG)
		var buf bytes.Buffer
		if err := engine.Renderer().Render(&buf, []byte(md), doc); err != nil {
			t.Fatalf("render (inlineSVG=%v): %v", inlineSVG, err)
		}
		v, ok := pc.Get(SvgOptionsKey).(*SvgOptions)
		return buf.String(), ok && v != nil && v.Enabled
	}

	wrapped, enabled := render(true)
	if !enabled || strings.Count(wrapped, "<svg") != 3 {
		t.Errorf("inlineSVG=true: enabled=%v, <svg count=%d, want enabled and 3:\n%s", enabled, strings.Count(wrapped, "<svg"), wrapped)
	}
	engine := NewEngine()
	doc, _ := ParseWithEngine(md, engine)
	var buf bytes.Buffer
	if err := engine.Renderer().Render(&buf, []byte(md), doc); err != nil {
		t.Fatal(err)
	}
	if buf.String() != wrapped {
		t.Errorf("ParseWithEngineSVG(true) differs from ParseWithEngine:\n%s\nvs\n%s", wrapped, buf.String())
	}

	flat, enabled := render(false)
	if enabled {
		t.Error("inlineSVG=false: SvgOptionsKey still reports Enabled=true")
	}
	if strings.Contains(flat, "<svg") || strings.Contains(flat, "foreignObject") {
		t.Errorf("inlineSVG=false: output still SVG-wrapped:\n%s", flat)
	}
	if got := strings.Count(flat, "<section"); got != 3 {
		t.Errorf("inlineSVG=false: %d <section> elements, want 3 (the empty middle one included):\n%s", got, flat)
	}

	// The headingDivider pre-seed is independent of the SVG decision.
	hd := "---\nheadingDivider: 2\n---\n\n# H1\n\n## H2\n"
	_, pc := ParseWithEngineSVG(hd, NewEngine(), false)
	if levels, ok := pc.Get(HeadingDividerKey).([]int); !ok || !intsEqual(levels, []int{1, 2}) {
		t.Errorf("HeadingDividerKey with inlineSVG=false = %#v, want [1 2]", pc.Get(HeadingDividerKey))
	}
}

// intsEqual reports whether two int slices are element-wise equal.
func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
