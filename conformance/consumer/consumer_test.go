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

package consumer

import (
	"os/exec"
	"strings"
	"testing"

	"go.aocyber.ai/eden-press/press"
)

// eden-press#31: the patched latex2mathml must reach consumers.
//
// Test list:
//
//	C1 the consumer's module graph holds no git.sr.ht/~mekyt/latex2mathml
//	   (the unpatched upstream a replace-directive fork leaves consumers with)
//	C2 \sqrt[3]{x} rendered through press keeps its radicand and index in an
//	   <mroot> (the 08-02 fix the upstream converter lacks)

func TestC1_ModuleGraphHasNoUpstreamLatex2mathml(t *testing.T) {
	out, err := exec.Command("go", "mod", "graph").CombinedOutput()
	if err != nil {
		t.Fatalf("go mod graph: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "git.sr.ht/~mekyt/latex2mathml") {
		t.Fatalf("consumer module graph requires the upstream latex2mathml, so it builds the unpatched converter:\n%s", grepLines(string(out), "latex2mathml"))
	}
}

func TestC2_SqrtIndexKeepsRadicandForConsumers(t *testing.T) {
	out, err := press.Render(`$\sqrt[3]{x}$`, press.Options{})
	if err != nil {
		t.Fatal(err)
	}
	html := out.HTML
	i := strings.Index(html, "<mroot>")
	if i < 0 {
		t.Fatalf(`\sqrt[3]{x}: no <mroot> in the consumer's render: %s`, html)
	}
	root := html[i:]
	if j := strings.Index(root, "</mroot>"); j >= 0 {
		root = root[:j]
	}
	if !strings.Contains(root, "<mi>x</mi>") || !strings.Contains(root, "<mn>3</mn>") {
		t.Fatalf(`\sqrt[3]{x}: <mroot> lost its radicand or index: %s`, root)
	}
}

func grepLines(s, sub string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}
