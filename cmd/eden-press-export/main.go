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

// Command eden-press-export is the 05.1-01 turnkey raster export CLI: a
// SEPARATE, Chrome-permitting binary from cmd/eden-press. It does not
// import cmd/eden-press -- a small duplicated input+flag helper lives here
// instead (output.go) -- so the core eden-press binary stays provably
// chromedp-free (scripts/check-no-chromedp.sh, re-scoped by this same TRD)
// while this binary is the ONE opt-in place PDF/PNG raster export lives.
//
// Pipeline (runExport, below): resolveInput -> buildPressOptions ->
// press.Render -> a PRE-FLIGHT chrome.Discover (so a missing Chrome fails
// fast with a documented remedy, exit 3, before a real launch attempt) ->
// chrome.New -> convert/pdf.ToPDF or convert/png.ToImages -> write bytes.
// Every sink (pdf.ToPDF, png.ToImages) already exists from Objective 5; this
// binary only wires a command around them.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/convert"
	"go.aocyber.ai/eden-press/convert/chrome"
	"go.aocyber.ai/eden-press/convert/pdf"
	"go.aocyber.ai/eden-press/convert/png"
	"go.aocyber.ai/eden-press/press"
)

// Exit codes -- eden-press-export's own documented, stable failure
// contract. It intentionally ADDS exitNoChrome (3) on top of cmd/eden-press's
// two-code contract (1=render, 2=usage): a missing Chrome/Chromium is a
// distinct, common, and actionable failure mode specific to this binary,
// worth its own code rather than folding into exitRender.
const (
	// exitRender is returned for input/render/runtime failures: resolveInput,
	// press.Render, chrome.New, pdf.ToPDF/png.ToImages, or a write failure.
	exitRender = 1
	// exitUsage is returned for usage/flag errors: an unknown --format
	// value, a PDF-from-stdin-with-no--o request, or a cobra/pflag
	// flag-parse failure (SetFlagErrorFunc below).
	exitUsage = 2
	// exitNoChrome is returned when chrome.Discover cannot resolve a
	// Chrome/Chromium executable via --browser-path, CHROME_PATH, or PATH
	// auto-detection -- distinct from exitRender so a caller (a script, an
	// agent) can tell "no browser available" apart from "render failed".
	exitNoChrome = 3
)

// cliError carries a stable, documented exit code through main's single
// exit-code sink. Mirrors cmd/eden-press/format.go's identically-named
// type; duplicated here rather than shared because eden-press-export does
// not import cmd/eden-press.
type cliError struct {
	code int
	err  error
}

// Error implements the error interface, delegating to the wrapped error.
func (e *cliError) Error() string { return e.err.Error() }

// Unwrap exposes the wrapped error for errors.Is/errors.As chains.
func (e *cliError) Unwrap() error { return e.err }

// fail wraps err in a *cliError carrying code -- the single construction
// point runExport's non-Chrome failure paths use.
func fail(code int, err error) error { return &cliError{code: code, err: err} }

// chromeUnavailableError builds the exit-3 failure for a chrome.Discover
// miss: a clear remedy naming BOTH the --browser-path flag and the
// CHROME_PATH env var it complements, plus the documented pinned-download
// fallback (mirrors convert/chrome.ErrChromeNotFound's own remedy text one
// level up, in CLI-facing language).
func chromeUnavailableError(cause error) error {
	return &cliError{code: exitNoChrome, err: fmt.Errorf(
		"no Chrome/Chromium found for raster export: %w\n"+
			"  supply one with --browser-path <path> or the CHROME_PATH env var,\n"+
			"  or install a pinned chromedp/headless-shell (see convert/EXPORT.md)", cause)}
}

// main builds the root command and executes it, translating a non-nil
// error into a non-zero, documented process exit code. main is the SOLE
// exit-code sink and the SOLE error printer -- newRootCmd sets
// SilenceErrors:true, so cobra itself never prints, and every failure path
// returns a *cliError so it is printed exactly once, here.
func main() {
	if err := newRootCmd().Execute(); err != nil {
		var ce *cliError
		if errors.As(err, &ce) {
			fmt.Fprintln(os.Stderr, ce.Error())
			os.Exit(ce.code)
		}

		// A non-cliError failure should not normally occur once every
		// runExport return path is wrapped via fail/chromeUnavailableError,
		// but is kept as a safety net falling back to the historical
		// exit(1) plain print.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitRender)
	}
}

// newRootCmd builds the eden-press-export root command: a single command
// (no subcommands -- unlike cmd/eden-press's convert/watch/serve/preview
// surface, this binary does exactly one thing) taking at most one
// positional argument (the input deck; "-" or omitted reads stdin).
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "eden-press-export [flags] <deck.md|->",
		Short: "Export a Marp-compatible Markdown deck to PDF or PNG via headless Chrome",
		Long: `eden-press-export renders a Marp-compatible Markdown deck (via press.Render)
and drives a headless Chrome/Chromium session (via convert/chrome, convert/pdf,
convert/png) to produce PDF or per-slide PNG output.

It is a SEPARATE binary from eden-press: the core CLI (eden-press) stays
chromedp-free, and eden-press-export is the one opt-in place PDF/PNG raster
export lives. Chrome is discovered via --browser-path, the CHROME_PATH
environment variable, or PATH auto-detection; if none resolves, this command
exits 3 with a remedy message instead of attempting a launch.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runExport,
	}

	registerExportFlags(root)

	// SilenceErrors (above) stops cobra from printing an error itself, so
	// main's exit-code sink is the ONLY place any error is ever printed.
	// FlagErrorFunc classifies a cobra/pflag flag-parse failure as a
	// *cliError{code: exitUsage}, the same documented exit-2 usage-error
	// contract runExport's unknown-format case uses.
	root.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return &cliError{code: exitUsage, err: e}
	})

	return root
}

// registerExportFlags registers every flag this binary understands: the
// render knobs (mapped 1:1 onto press.Options by buildPressOptions,
// output.go) plus the export-specific --format/--output/--browser-path
// trio. There is no --theme-set or --config here -- this binary is a lean
// turnkey exporter with no koanf/config-file machinery (that full surface
// stays in cmd/eden-press). Auto-fit is Flutter-only (Objective 8, 08-06/
// 08-07) and was never a flag on this exporter either way.
func registerExportFlags(cmd *cobra.Command) {
	f := cmd.Flags()

	f.String("theme", "", `theme name; "" resolves the deck's front-matter "theme:", then "default" (press.Render's own fallback)`)
	f.String("profile", "", `chase/profile id; "" resolves profile.Default() ("slides")`)
	f.String("math", "", `math rendering mode: "mathml" (default) or "off"`)
	f.Bool("no-highlight", false, "disable chroma syntax highlighting")
	f.String("highlight-style", "", `chroma highlight style name; "" resolves chroma's own default`)
	f.Bool("inline-svg", false, "select the inline-<svg><foreignObject> container mode; wired to BOTH press.Render and png.ToImages")

	f.StringP("format", "f", "pdf", "output format: pdf (default) | png")
	f.StringP("output", "o", "", "output path: a PDF file path (--format pdf), or a directory / printf %d-pattern (--format png)")
	f.String("browser-path", "", "explicit Chrome/Chromium executable path; complements the CHROME_PATH environment variable")
}

// runExport is the 05.1-01 capstone pipeline: resolveInput -> buildPressOptions
// -> press.Render -> a pre-flight chrome.Discover -> chrome.New -> the
// --format-selected exporter (pdf.ToPDF or png.ToImages) -> write bytes.
//
// The pre-flight chrome.Discover call (immediately before chrome.New) is
// deliberate even though chrome.New already calls Discover internally: it
// exists purely so a missing Chrome is reported with THIS binary's clean,
// exit-3 remedy message before any launch attempt is made, rather than
// surfacing as an opaque chrome.New failure through the generic exitRender
// path.
func runExport(cmd *cobra.Command, args []string) error {
	arg := "-"
	if len(args) == 1 {
		arg = args[0]
	}

	md, isStdin, err := resolveInput(arg, cmd.InOrStdin())
	if err != nil {
		return fail(exitRender, err)
	}

	opts := buildPressOptions(cmd)

	out, err := press.Render(md, opts)
	if err != nil {
		return fail(exitRender, err)
	}

	bp := browserPath(cmd)
	if _, _, derr := chrome.Discover(chrome.DiscoverOptions{BrowserPath: bp}); derr != nil {
		return chromeUnavailableError(derr)
	}

	sess, err := chrome.New(convert.Options{BrowserPath: bp})
	if err != nil {
		return fail(exitRender, err)
	}
	defer sess.Close()

	switch f := cmd.Flags().Lookup("format").Value.String(); f {
	case "", "pdf":
		b, err := pdf.ToPDF(sess, out, pdf.Options{})
		if err != nil {
			return fail(exitRender, err)
		}
		return writePDF(cmd, arg, isStdin, b)

	case "png":
		// LOAD-BEARING: png.ToImages must use the container mode press.Render
		// ACTUALLY rendered, read from out.HTML, not the --inline-svg flag.
		// press.Render emits the inline-<svg><foreignObject> wrapper for a
		// profile that has an SVG layer (slides), whatever the flag says, so
		// passing the flag's default false made ToImages' `div.marpit > section`
		// selector match nothing and chromedp.Screenshot wait forever; a
		// mismatched mode the other way makes the nth-of-type selector
		// resolve position 1 N times. (A profile without an SVG layer, paged,
		// is addressed as plain children whatever is passed: png.unitSelector.)
		imgs, err := png.ToImages(sess, out, png.Options{InlineSVG: renderedInlineSVG(out)})
		if err != nil {
			return fail(exitRender, err)
		}
		return writePNGs(cmd, arg, isStdin, imgs)

	default:
		return fail(exitUsage, fmt.Errorf("unknown --format %q (want pdf|png)", f))
	}
}
