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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/press"
)

// newDocCmd is the agent-facing docmodel surface: create / get / apply /
// import. It settles the "CLI + AGENTS.md first" agent API (MCP can wrap
// these later). Errors always use the JSON envelope so agents never parse
// free-text stderr here.
func newDocCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Create, read, and edit docmodel JSON (agent write surface)",
		Long: `doc operates on eden-press.model JSON documents (the AI-first source of truth).

Subcommands:
  create   empty page JSON
  get      validate / pretty-print a page JSON
  apply    apply_ops batch to a page JSON
  import   Markdown -> docmodel JSON via press.Render (assigns block ids)
  export   docmodel JSON -> Marp-compatible Markdown

Pitfall: a file literally named "doc" conflicts with this subcommand; use
"eden-press convert -- doc" for Markdown convert of such a path.`,
	}
	cmd.AddCommand(
		newDocCreateCmd(),
		newDocGetCmd(),
		newDocApplyCmd(),
		newDocImportCmd(),
		newDocExportCmd(),
	)
	return cmd
}

func newDocExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <doc.json|->",
		Short: "Export docmodel JSON to Markdown or standalone HTML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			format, _ := cmd.Flags().GetString("format")
			var raw []byte
			switch strings.ToLower(format) {
			case "", "md", "markdown":
				raw, err = press.ExportMarkdownJSON(in)
			case "html":
				raw, err = press.ExportHTMLJSON(in)
			default:
				return docFail(cmd, exitUsage, fmt.Errorf("doc export: unknown --format %q (want md|html)", format))
			}
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocOutputBytes(cmd, raw)
		},
	}
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	cmd.Flags().String("format", "md", `export format: md (Marp-compatible Markdown) or html (standalone Docs HTML)`)
	return cmd
}

func newDocCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an empty docmodel page as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			title, _ := cmd.Flags().GetString("title")
			raw, err := press.NewDocJSON(title)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocJSONOutput(cmd, raw)
		},
	}
	cmd.Flags().String("title", "", "optional page title stored in meta.directives.title")
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newDocGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <doc.json|->",
		Short: "Validate and pretty-print a docmodel JSON document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			raw, err := press.GetDocJSON(in)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocJSONOutput(cmd, raw)
		},
	}
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newDocApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <doc.json|->",
		Short: "Apply an ops JSON batch to a docmodel document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opsPath, _ := cmd.Flags().GetString("ops")
			if opsPath == "" {
				return docFail(cmd, exitUsage, fmt.Errorf("doc apply: --ops is required"))
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			docRaw, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			opsRaw, err := os.ReadFile(opsPath)
			if err != nil {
				return docFail(cmd, exitRender, fmt.Errorf("read ops %q: %w", opsPath, err))
			}
			out, err := press.ApplyOpsJSONOpts(docRaw, opsRaw, dryRun)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocJSONOutput(cmd, out)
		},
	}
	cmd.Flags().String("ops", "", "path to ops JSON (array or {\"ops\":[...]})")
	cmd.Flags().Bool("dry-run", false, "validate ops without mutating the document")
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newDocImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <in.md|->",
		Short: "Import Markdown into docmodel JSON via press.Render",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			md, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			opts, err := buildOptions(cmd)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			raw, err := press.ImportMarkdownModelJSON(md, opts)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocJSONOutput(cmd, raw)
		},
	}
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func readPathOrStdin(cmd *cobra.Command, arg string) ([]byte, error) {
	if arg == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	b, err := os.ReadFile(arg)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", arg, err)
	}
	return b, nil
}

// writeDocJSONOutput writes a docmodel JSON result indented. press returns
// compact JSON (eden-press#29); the CLI keeps printing it as it always has,
// and the nesting limit press enforces keeps the indentation bounded.
func writeDocJSONOutput(cmd *cobra.Command, raw []byte) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return fmt.Errorf("indent output: %w", err)
	}
	return writeDocOutputBytes(cmd, buf.Bytes())
}

func writeDocOutputBytes(cmd *cobra.Command, raw []byte) error {
	path := ""
	if f := cmd.Flags().Lookup("output"); f != nil {
		path = f.Value.String()
	}
	payload := append(append([]byte{}, raw...), '\n')
	if path == "" {
		_, err := cmd.OutOrStdout().Write(payload)
		return err
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write file %q: %w", path, err)
	}
	return nil
}

// docFail always emits the JSON error envelope (agent surface), then
// returns a printed cliError so main does not double-print.
func docFail(cmd *cobra.Command, code int, err error) error {
	b, marshalErr := json.MarshalIndent(jsonError{Error: jsonErrorBody{Code: code, Message: err.Error()}}, "", "  ")
	if marshalErr != nil {
		return &cliError{code: code, err: err, printed: false}
	}
	fmt.Fprintln(cmd.ErrOrStderr(), string(b))
	return &cliError{code: code, err: err, printed: true}
}
