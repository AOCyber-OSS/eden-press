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
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/convert/pptx"
	"go.aocyber.ai/eden-press/convert/xlsx"
	"go.aocyber.ai/eden-press/press"
)

// Exit codes (04.1-01's documented, stable failure contract): 0 is cobra's
// own implicit success; a *cliError carries one of the two below through
// main's single exit-code sink (main.go).
const (
	// exitRender is returned for input/render/runtime failures (resolveInput,
	// buildOptions, press.Render, or a json.Marshal failure inside
	// writeJSON).
	exitRender = 1
	// exitUsage is returned for usage/flag errors: an unknown --format
	// value, the not-yet-wired pptx case, and cobra/pflag flag-parse
	// failures (root.go's FlagErrorFunc).
	exitUsage = 2
)

// jsonEnvelope is the lowercase-keyed, agent-facing view of the FULL
// press.Output -- {html, css, model, comments, meta}. Model and Meta are
// typed `any` and fed out.Model/out.Meta VERBATIM: this is the ONLY way to
// surface chase/model.Document's own schema-v2 JSON shape (sections[].
// blocks[], outline[]) without cmd/eden-press importing chase/model
// directly, which would trip scripts/check-cli-imports.sh's
// `(chase|profiles)` grep. json.Marshal(out) directly is NOT used here --
// press.Output's Go fields carry no json tags, which would yield
// UPPERCASE keys (HTML/CSS/Model/...); this view is the deliberate
// lowercase re-shaping.
type jsonEnvelope struct {
	HTML string `json:"html"`
	CSS  string `json:"css"`
	// Model is out.Model (*chase/model.Document), marshaled through the
	// model package's OWN json tags -- schema-v2 verbatim: a code block is
	// {kind:"code", text:<raw source>, language}, a math block is
	// {kind:"math", text:<raw TeX>, display}. There are NO renamed
	// source/tex keys.
	Model any `json:"model"`
	// Comments is out.Comments -- the deck's speaker notes flattened into
	// document order.
	Comments []string `json:"comments"`
	// Meta is out.Meta (chase/model.Meta), surfaced top-level as a
	// convenience alias identical to model.meta.
	Meta any `json:"meta"`
}

// jsonErrorBody is the machine-readable failure payload nested under the
// top-level "error" key -- see jsonError.
type jsonErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// jsonError is the JSON error envelope written to stderr when a failure
// occurs while --format json is active: {"error":{"code":<int>,
// "message":<string>}}. It is gated strictly on cfg.String("format") ==
// "json" inside cliFail -- html/pptx failures keep the plain-text stderr
// path (main.go prints ce.Error() verbatim).
type jsonError struct {
	Error jsonErrorBody `json:"error"`
}

// cliError is the single error type main.go's exit-code sink classifies via
// errors.As: it carries a stable, documented exit code (exitRender/
// exitUsage) and reports whether its message has ALREADY been printed
// (printed=true for a json-format failure, whose message went out as the
// JSON envelope inside cliFail itself) so main never double-prints.
type cliError struct {
	code    int
	err     error
	printed bool
}

// Error implements the error interface, delegating to the wrapped error.
func (e *cliError) Error() string { return e.err.Error() }

// Unwrap exposes the wrapped error for errors.Is/errors.As chains.
func (e *cliError) Unwrap() error { return e.err }

// cliFail is the SINGLE failure sink runConvert and emitFormat both call.
// When --format json is active (cfg.String("format") == "json") it writes
// the JSON error envelope to cmd.ErrOrStderr() and returns a *cliError with
// printed=true, so main.go's sink does not re-print the message in plain
// text. Otherwise it returns a *cliError with printed=false, leaving the
// plain-text print to main.go -- exactly ONE print, on exactly ONE path,
// for every failure.
func cliFail(cmd *cobra.Command, code int, err error) error {
	if cfg.String("format") == "json" {
		b, marshalErr := json.MarshalIndent(jsonError{Error: jsonErrorBody{Code: code, Message: err.Error()}}, "", "  ")
		if marshalErr == nil {
			fmt.Fprintln(cmd.ErrOrStderr(), string(b))
			return &cliError{code: code, err: err, printed: true}
		}
		// Marshaling the error envelope itself failed (should not happen for
		// a plain string message) -- fall through to the plain-text path
		// rather than silently swallowing the original error.
	}
	return &cliError{code: code, err: err}
}

// emitFormat is the ONE format-dispatch seam: html routes through the
// existing assembleHTML/writeOutput string pipeline (unchanged --
// Objective 4's CLI-01 byte-for-byte output), json routes through
// writeJSON, pptx routes through writePPTX, and xlsx routes through
// writeXLSX (the stdlib SpreadsheetML exporter). An unknown value is a
// usage error (exit 2).
func emitFormat(cmd *cobra.Command, out press.Output) error {
	switch f := cfg.String("format"); f {
	case "", "html":
		doc := assembleHTML(out, htmlDocOptions{})
		return writeOutput(cmd, doc)
	case "json":
		return writeJSON(cmd, out)
	case "pptx":
		return writePPTX(cmd, out)
	case "xlsx":
		return writeXLSX(cmd, out)
	default:
		return cliFail(cmd, exitUsage, fmt.Errorf("unknown --format %q (want html|json|pptx|xlsx)", f))
	}
}

// writeXLSX renders out.Model through convert/xlsx.ToXLSX. The workbook is
// the Sheets document; this flag is the file interchange, same posture as
// pptx. A nil model fails through cliFail(exitRender).
func writeXLSX(cmd *cobra.Command, out press.Output) error {
	b, err := xlsx.ToXLSX(out.Model, xlsx.Options{})
	if err != nil {
		return cliFail(cmd, exitRender, fmt.Errorf("format xlsx: %w", err))
	}
	return writeOutputBytes(cmd, b)
}

// writePPTX renders out.Model -- already a *chase/model.Document, typed by
// press.Render -- through convert/pptx.ToPPTX: a stdlib-OOXML exporter with
// ZERO chromedp. cmd/eden-press never imports chase/model directly to name
// that type; out.Model is passed straight through, already correctly typed.
// The resulting bytes are written via writeOutputBytes (04.1-01's shared
// --output/-o-or-stdout sink, the same one writeJSON uses), and any ToPPTX
// failure is reported through cliFail(exitRender) -- the SAME single
// failure sink every other format uses, so a pptx render failure is
// classified and (under --format json) enveloped identically to a json
// marshal failure.
func writePPTX(cmd *cobra.Command, out press.Output) error {
	b, err := pptx.ToPPTX(out.Model, pptx.Options{})
	if err != nil {
		return cliFail(cmd, exitRender, fmt.Errorf("format pptx: %w", err))
	}
	return writeOutputBytes(cmd, b)
}

// writeJSON marshals the FULL press.Output through the lowercase
// jsonEnvelope view and writes it via writeOutputBytes (stdout, or
// --output/-o when set).
func writeJSON(cmd *cobra.Command, out press.Output) error {
	env := jsonEnvelope{
		HTML:     out.HTML,
		CSS:      out.CSS,
		Model:    out.Model,
		Comments: out.Comments,
		Meta:     out.Meta,
	}

	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return cliFail(cmd, exitRender, fmt.Errorf("format json: marshal: %w", err))
	}

	return writeOutputBytes(cmd, append(b, '\n'))
}

// writeOutputBytes mirrors writeOutput (convert.go) but for a []byte
// payload (json today; pptx's binary payload in 04.1-02): writes to the
// --output/-o path if the flag is registered AND set, else to
// cmd.OutOrStdout(). The defensive Lookup (not GetString) mirrors
// writeOutput's own rationale -- runConvert backs both root's bare default
// (no --output registered) and the explicit "convert" subcommand.
func writeOutputBytes(cmd *cobra.Command, b []byte) error {
	if f := cmd.Flags().Lookup("output"); f != nil && f.Value.String() != "" {
		path := f.Value.String()
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return fmt.Errorf("writeOutputBytes: write file %q: %w", path, err)
		}
		return nil
	}

	_, err := fmt.Fprint(cmd.OutOrStdout(), string(b))
	return err
}
