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

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/press"
)

// inlineSVGSlideTag is the start of the per-slide wrapper chase/markdown
// writes in inline-SVG mode (`<svg data-marpit-svg="" viewBox="0 0 W H">`).
// Deck text can never produce it: the sanitizer escapes a literal `<`.
const inlineSVGSlideTag = "<svg data-marpit-svg"

// renderedInlineSVG reports whether out was rendered in inline-SVG container
// mode, read from the markup itself -- the mode png.ToImages' slide selector
// must match. press.Render decides that mode (it wraps a profile that has an
// SVG layer, slides, whatever press.Options.InlineSVG says, and never wraps
// paged), so the --inline-svg flag alone is not a reliable answer.
func renderedInlineSVG(out press.Output) bool {
	return strings.Contains(out.HTML, inlineSVGSlideTag)
}

// resolveInput resolves arg into markdown text: "-" reads all of stdin (via
// the injected reader, so tests never touch the real process stdin); any
// other value is a file path read from disk. It reports whether stdin was
// used, so writePDF/writePNGs/stemFor can apply their own filename policy on
// top.
//
// This is intentionally DUPLICATED from cmd/eden-press/input.go rather than
// shared: eden-press-export is a separate package main and must not import
// cmd/eden-press (the user-chosen clean-standalone-binary design).
func resolveInput(arg string, stdin io.Reader) (md string, isStdin bool, err error) {
	if arg == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", true, fmt.Errorf("resolveInput: read stdin: %w", err)
		}
		return string(b), true, nil
	}

	b, err := os.ReadFile(arg)
	if err != nil {
		return "", false, fmt.Errorf("resolveInput: read file %q: %w", arg, err)
	}
	return string(b), false, nil
}

// buildPressOptions maps the render-knob flags 1:1 onto press.Options.
// There is no koanf, no config file, and no --theme-set file loading here --
// press.Render owns every ""-> fallback chain itself, so every flag's zero
// value is passed straight through verbatim; buildPressOptions never
// substitutes a default of its own.
func buildPressOptions(cmd *cobra.Command) press.Options {
	g := cmd.Flags()

	theme, _ := g.GetString("theme")
	profile, _ := g.GetString("profile")
	mathMode, _ := g.GetString("math")
	noHighlight, _ := g.GetBool("no-highlight")
	highlightStyle, _ := g.GetString("highlight-style")
	inlineSVG, _ := g.GetBool("inline-svg")

	return press.Options{
		Theme:          theme,
		Profile:        profile,
		MathMode:       mathMode,
		NoHighlight:    noHighlight,
		HighlightStyle: highlightStyle,
		InlineSVG:      inlineSVG,
	}
}

// browserPath returns the --browser-path flag's value. "" lets
// chrome.Discover fall through to its own CHROME_PATH-then-auto-detection
// tiers; browserPath itself applies no fallback.
func browserPath(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("browser-path")
	return v
}

// stemFor derives the output-filename stem used by writePDF's default
// "<stem>.pdf" and writePNGs' "<stem>-NNN.png" convention: the input file's
// basename with its extension stripped, or the literal "deck" for stdin
// input (which has no filename to derive a stem from).
func stemFor(arg string, isStdin bool) string {
	if isStdin {
		return "deck"
	}
	base := filepath.Base(arg)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// printfVerbRE matches a printf integer verb (%d, %03d, %-4d, ...) inside an
// -o value -- the discriminator pngOutputPath uses to choose its
// Sprintf-pattern branch over its directory-join branch.
var printfVerbRE = regexp.MustCompile(`%[-+ #0]*[0-9]*d`)

// pngOutputPath resolves the file path for 1-based slide n (of total, not
// needed by the formula itself), given the raw -o flag value o and the
// input stem:
//
//   - o contains a printf integer verb -> fmt.Sprintf(o, n) (e.g.
//     "frame-%03d.png" -> "frame-001.png").
//   - otherwise o names a DIRECTORY (created by the caller, writePNGs) ->
//     filepath.Join(o, "<stem>-NNN.png").
//   - o == "" -> filepath.Join(".", "<stem>-NNN.png") (the CWD).
//
// pngOutputPath is a pure function -- no I/O -- so it is directly
// unit-testable for both branches without touching the filesystem.
func pngOutputPath(o, stem string, n int) string {
	if printfVerbRE.MatchString(o) {
		return fmt.Sprintf(o, n)
	}

	dir := o
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%03d.png", stem, n))
}

// writePDF writes b (a %PDF- byte stream from pdf.ToPDF) to the -o path
// when set; otherwise, for a file input, to "<stem>.pdf" in the current
// directory. PDF export from stdin with no -o is a usage error (exit 2) --
// there is no filename to derive a default destination from, and writing an
// arbitrary %PDF- byte stream to a terminal is never useful.
func writePDF(cmd *cobra.Command, arg string, isStdin bool, b []byte) error {
	o, _ := cmd.Flags().GetString("output")

	dest := o
	if dest == "" {
		if isStdin {
			return fail(exitUsage, fmt.Errorf("PDF export from stdin requires -o <file> (no filename to derive a default from)"))
		}
		dest = stemFor(arg, isStdin) + ".pdf"
	}

	if err := os.WriteFile(dest, b, 0o644); err != nil {
		return fail(exitRender, fmt.Errorf("writePDF: write file %q: %w", dest, err))
	}
	return nil
}

// writePNGs writes one file per slide via pngOutputPath. When -o names a
// plain directory (no printf verb), that directory is created first
// (os.MkdirAll) so a not-yet-existing "-o dir/" just works; the printf-verb
// branch never creates a directory of its own (the caller is expected to
// have supplied a pattern whose containing directory already exists, mirroring
// how any Sprintf-templated path is used elsewhere in this CLI ecosystem).
func writePNGs(cmd *cobra.Command, arg string, isStdin bool, imgs [][]byte) error {
	o, _ := cmd.Flags().GetString("output")
	stem := stemFor(arg, isStdin)

	if !printfVerbRE.MatchString(o) {
		dir := o
		if dir == "" {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail(exitRender, fmt.Errorf("writePNGs: mkdir %q: %w", dir, err))
		}
	}

	for i, img := range imgs {
		dest := pngOutputPath(o, stem, i+1)
		if err := os.WriteFile(dest, img, 0o644); err != nil {
			return fail(exitRender, fmt.Errorf("writePNGs: write file %q: %w", dest, err))
		}
	}
	return nil
}
