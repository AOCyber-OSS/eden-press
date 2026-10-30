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
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/press"
)

const xlsxFormatDeck = "# Metrics\n\n" +
	"| Metric | Q3 |\n" +
	"| --- | --: |\n" +
	"| p95 | 550 |\n"

func TestFormatXLSXWritesWorkbook(t *testing.T) {
	resetCfg()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "deck.md")
	if err := os.WriteFile(inPath, []byte(xlsxFormatDeck), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	outPath := filepath.Join(dir, "out.xlsx")

	root := newRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"convert", inPath, "--output", outPath, "--format", "xlsx"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, errBuf.String())
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	var sheet string
	for _, f := range zr.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open sheet: %v", err)
		}
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(rc)
		rc.Close()
		sheet = buf.String()
	}
	if !strings.Contains(sheet, "<v>550</v>") {
		t.Fatalf("sheet missing numeric 550:\n%s", sheet)
	}
}

func TestEmitFormatXLSXNilModel(t *testing.T) {
	resetCfg()
	if err := cfg.Set("format", "xlsx"); err != nil {
		t.Fatalf("cfg.Set: %v", err)
	}
	cmd := newRootCmd()
	err := emitFormat(cmd, press.Output{})
	var ce *cliError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v (%T)", err, err)
	}
	if ce.code != exitRender {
		t.Fatalf("code = %d, want %d", ce.code, exitRender)
	}
}
