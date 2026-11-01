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
	"bytes"
	"encoding/json"
	"fmt"

	"go.aocyber.ai/eden-press/convert/xlsx"
)

// NewWorkbookJSON returns an empty workbook snapshot JSON.
func NewWorkbookJSON(title string) ([]byte, error) {
	wb := xlsx.NewWorkbook(title)
	if _, err := wb.AddSheet("Sheet1"); err != nil {
		return nil, err
	}
	return marshalWorkbook(wb)
}

// ApplyWorkbookOpsJSON applies workbook ops to a snapshot and returns the
// updated snapshot. opsJSON may be a bare array or {"ops":[...]}.
func ApplyWorkbookOpsJSON(wbJSON, opsJSON []byte) ([]byte, error) {
	wb, err := xlsx.ParseWorkbook(wbJSON)
	if err != nil {
		return nil, fmt.Errorf("press.ApplyWorkbookOpsJSON: %w", err)
	}
	ops, err := parseWbOpsJSON(opsJSON)
	if err != nil {
		return nil, fmt.Errorf("press.ApplyWorkbookOpsJSON: parse ops: %w", err)
	}
	if err := xlsx.ApplyWorkbookOps(wb, ops); err != nil {
		return nil, err
	}
	return marshalWorkbook(wb)
}

// GetWorkbookJSON validates and re-emits a workbook snapshot (ensuring row ids).
func GetWorkbookJSON(wbJSON []byte) ([]byte, error) {
	wb, err := xlsx.ParseWorkbook(wbJSON)
	if err != nil {
		return nil, fmt.Errorf("press.GetWorkbookJSON: %w", err)
	}
	xlsx.EnsureRowIDs(wb)
	return marshalWorkbook(wb)
}

// ExportWorkbookXLSXJSON packs a workbook snapshot to .xlsx bytes.
func ExportWorkbookXLSXJSON(wbJSON []byte) ([]byte, error) {
	wb, err := xlsx.ParseWorkbook(wbJSON)
	if err != nil {
		return nil, fmt.Errorf("press.ExportWorkbookXLSXJSON: %w", err)
	}
	return wb.Bytes()
}

// ImportWorkbookXLSXJSON opens .xlsx bytes into a workbook snapshot JSON.
func ImportWorkbookXLSXJSON(xlsxBytes []byte) ([]byte, error) {
	wb, err := xlsx.Open(xlsxBytes)
	if err != nil {
		return nil, fmt.Errorf("press.ImportWorkbookXLSXJSON: %w", err)
	}
	xlsx.EnsureRowIDs(wb)
	return marshalWorkbook(wb)
}

func marshalWorkbook(wb *xlsx.Workbook) ([]byte, error) {
	raw, err := wb.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return raw, nil
	}
	return buf.Bytes(), nil
}

type wbOpsEnvelope struct {
	Ops []xlsx.WbOp `json:"ops"`
}

func parseWbOpsJSON(raw []byte) ([]xlsx.WbOp, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty ops")
	}
	if raw[0] == '[' {
		var ops []xlsx.WbOp
		if err := json.Unmarshal(raw, &ops); err != nil {
			return nil, err
		}
		return ops, nil
	}
	var env wbOpsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return env.Ops, nil
}
