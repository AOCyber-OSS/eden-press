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
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/press"
)

func newWorkbookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workbook",
		Short: "Create, read, and edit workbook JSON (agent sheet surface)",
		Long: `workbook operates on eden-press.workbook JSON snapshots (v3).

Subcommands: create, get, apply, import (.xlsx or JSON), export (.xlsx or JSON).`,
	}
	cmd.AddCommand(
		newWorkbookCreateCmd(),
		newWorkbookGetCmd(),
		newWorkbookApplyCmd(),
		newWorkbookImportCmd(),
		newWorkbookExportCmd(),
	)
	return cmd
}

func newWorkbookCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an empty workbook snapshot JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			title, _ := cmd.Flags().GetString("title")
			raw, err := press.NewWorkbookJSON(title)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocOutputBytes(cmd, raw)
		},
	}
	cmd.Flags().String("title", "", "workbook title")
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newWorkbookGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <book.json|->",
		Short: "Validate / pretty-print a workbook snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			raw, err := press.GetWorkbookJSON(in)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocOutputBytes(cmd, raw)
		},
	}
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newWorkbookApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <book.json|->",
		Short: "Apply workbook ops JSON to a snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opsPath, _ := cmd.Flags().GetString("ops")
			if opsPath == "" {
				return docFail(cmd, exitUsage, fmt.Errorf("workbook apply: --ops is required"))
			}
			wbRaw, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			opsRaw, err := os.ReadFile(opsPath)
			if err != nil {
				return docFail(cmd, exitRender, fmt.Errorf("read ops %q: %w", opsPath, err))
			}
			out, err := press.ApplyWorkbookOpsJSON(wbRaw, opsRaw)
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocOutputBytes(cmd, out)
		},
	}
	cmd.Flags().String("ops", "", "path to ops JSON")
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newWorkbookImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <book.xlsx|book.json|->",
		Short: "Import .xlsx or workbook JSON into a v3 snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			var raw []byte
			if args[0] != "-" && strings.HasSuffix(strings.ToLower(args[0]), ".json") {
				raw, err = press.GetWorkbookJSON(in)
			} else if len(in) > 0 && in[0] == '{' {
				raw, err = press.GetWorkbookJSON(in)
			} else {
				raw, err = press.ImportWorkbookXLSXJSON(in)
			}
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocOutputBytes(cmd, raw)
		},
	}
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}

func newWorkbookExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <book.json|->",
		Short: "Export workbook snapshot to .xlsx (default) or JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			in, err := readPathOrStdin(cmd, args[0])
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			var raw []byte
			switch format {
			case "json":
				raw, err = press.GetWorkbookJSON(in)
			case "xlsx", "":
				raw, err = press.ExportWorkbookXLSXJSON(in)
			default:
				return docFail(cmd, exitUsage, fmt.Errorf("workbook export: unknown --format %q (want xlsx|json)", format))
			}
			if err != nil {
				return docFail(cmd, exitRender, err)
			}
			return writeDocOutputBytes(cmd, raw)
		},
	}
	cmd.Flags().String("format", "xlsx", "output format: xlsx | json")
	cmd.Flags().StringP("output", "o", "", `output file path; "" writes to stdout`)
	return cmd
}
