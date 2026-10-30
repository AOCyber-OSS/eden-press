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
	"github.com/spf13/cobra"
)

// newRootCmd builds the eden-press root command: it runs CONVERT BY DEFAULT
// when invoked with zero or one positional argument and no subcommand
// (`eden-press deck.md` just works), while also registering convert/watch/
// serve/preview as explicit subcommands.
//
// Pitfall 1 (cobra positional-arg vs subcommand-name collision, research
// Pitfall 1): cobra resolves a bare positional argument that EXACTLY
// MATCHES a registered subcommand's Use name (convert/watch/serve/preview)
// AS that subcommand, not as a filename -- so `eden-press watch` runs the
// watch subcommand even if the user meant a file literally named "watch".
// The documented escape hatch is `--`, which tells cobra/pflag "everything
// after this is a positional argument, never a subcommand or flag":
// `eden-press convert -- watch.md` unambiguously treats "watch.md" as the
// input file. This is surfaced in the root's Long help (below) rather than
// silently worked around, per the TRD's Pitfall-1 guard.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "eden-press [flags] <in.md|->",
		Short: "Render Marp-compatible Markdown decks -- no JS, no Node, no browser",
		Long: `eden-press renders a Marp-compatible Markdown deck to zero-JS static HTML.

Running "eden-press <in.md>" with no subcommand converts the deck by
default. Reading from stdin: "eden-press -" (or piping into "eden-press").

Pitfall: a positional argument that EXACTLY matches a subcommand name
(convert, watch, serve, preview) is resolved by cobra AS that subcommand,
not as a filename. If your input file happens to be named e.g. "watch.md"
(or bare "watch"), disambiguate with "--":

    eden-press convert -- watch.md
    eden-press -- watch`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return applyConfig(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd, args)
		},
	}

	registerPersistentFlags(root)
	root.AddCommand(newConvertCmd(), newWatchCmd(), newServeCmd(), newPreviewCmd(), newDocCmd())

	// SilenceErrors (above) stops cobra from printing an error itself, so
	// main.go's exit-code sink (main.go) is the ONLY place any error is ever
	// printed. FlagErrorFunc classifies a cobra/pflag flag-parse failure
	// (e.g. an unknown flag) as a *cliError{code: exitUsage} -- the SAME
	// documented exit-2 usage-error contract emitFormat's unknown-format
	// case uses -- so main's errors.As sink handles both uniformly.
	root.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return &cliError{code: exitUsage, err: e}
	})

	return root
}
