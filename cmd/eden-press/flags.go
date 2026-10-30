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

import "github.com/spf13/cobra"

// registerPersistentFlags registers the flag surface shared by every mode
// (root-default convert + all four subcommands, since it is bound on the
// ROOT command and every subcommand inherits persistent flags). These are
// exactly the flags buildOptions reads through cfg -- see options.go.
//
// No opt-in browser-side auto-fit splice flag exists here (Objective 8,
// 08-06): auto-fit is Flutter-only via the Dart binding's native
// TextPainter fit (08-07). The assembled HTML document never ships
// viewer-side auto-fit JavaScript.
func registerPersistentFlags(root *cobra.Command) {
	f := root.PersistentFlags()

	f.String("theme", "", `theme name; "" resolves the deck's front-matter "theme:", then "default" (press.Render's own fallback -- not re-implemented here)`)
	f.StringSlice("theme-set", nil, "path(s) to custom theme CSS file(s) (repeatable); resolved by 04-05's themeCSS")
	f.String("profile", "", `chase/profile id; "" resolves profile.Default() ("slides")`)
	f.String("math", "", `math rendering mode: "mathml" (default) or "off"`)
	f.Bool("no-highlight", false, "disable chroma syntax highlighting")
	f.String("highlight-style", "", `chroma highlight style name; "" resolves chroma's own default`)
	f.Bool("inline-svg", false, "select the inline-<svg><foreignObject> container mode (forward-compat; effectively always-on today)")
	f.String("config", "", "explicit config file path override (read by 04-04's loadConfigSources)")
	f.String("format", "html", "output format: html (default) | json | pptx | xlsx (convert/default mode)")
}

// registerConvertFlags registers convert's local flags: --output/-o, the
// destination file for the rendered HTML ("" resolves stdout -- 04-03's
// default, not implemented here).
func registerConvertFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout (04-03)`)
}

// registerWatchFlags registers watch's local flags: --output/-o, the
// destination file for each rebuild ("" resolves "<input-stem>.html" --
// 04-06's default, not implemented here).
func registerWatchFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", `output file path; "" resolves "<input-stem>.html" (04-06)`)
}

// registerServeFlags registers serve's local flags: --port (product
// default 8321, NEVER 8080) and --host ("" resolves "127.0.0.1" -- 04-07's
// default, not implemented here).
func registerServeFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.Int("port", 8321, "listen port (product default 8321; NEVER 8080)")
	f.String("host", "", `listen host; "" resolves "127.0.0.1" (04-07)`)
}
