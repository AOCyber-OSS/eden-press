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

	"go.aocyber.ai/eden-press/conformance/corpus/xlsxformula"
)

func main() {
	b, err := xlsxformula.CasesJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gencases: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("cases.json", b, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gencases: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("gencases: wrote cases.json (%d bytes, %d cases)\n", len(b), len(xlsxformula.Cases))
}
