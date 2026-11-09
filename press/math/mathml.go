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

package math

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	// The patched in-repo fork of git.sr.ht/~mekyt/latex2mathml. It is a
	// package of this module, not a module behind a replace directive, because
	// Go ignores a dependency's replace directives: consumers would build the
	// unpatched upstream (eden-press#31).
	l2m "go.aocyber.ai/eden-press/internal/latex2mathml"
)

// mathMLNamespace is the MathML namespace latex2mathml stamps onto every <math>
// element's xmlns attribute.
const mathMLNamespace = "http://www.w3.org/1998/Math/MathML"

// mathRenderer is the from-scratch NodeRenderer for mathNode. It makes the
// route decision on the RAW source FIRST (needsFallback, detect.go), then emits
// either native MathML (renderMathML) or the PNG fallback <img>
// (renderFallbackIMG) — never attempting a conversion before routing.
type mathRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (r *mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMath, r.render)
}

func (r *mathRenderer) render(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	m := n.(*mathNode)
	if needsFallback(m.Raw) {
		_, _ = w.WriteString(renderFallbackIMG(m.Raw, m.Block))
	} else {
		_, _ = w.WriteString(renderMathML(m.Raw, m.Block))
	}
	return ast.WalkSkipChildren, nil
}

// renderMathML converts raw LaTeX to a native MathML `<math>` element via the
// vendored latex2mathml (pinned in 03-01). block selects the display mode:
// `display="block"` for `$$…$$`, `display="inline"` for `$…$`. It is a PURE
// function — no I/O, no globals — so press.Render stays pure.
//
// A recover guard degrades a converter panic (a known latex2mathml bug class —
// research Pitfall 5, Objective 8's hardening job) to a safe escaped <code>
// rather than crashing the whole render. BASELINE only guarantees well-formed
// MathML for simple cases; known-wrong cases are deferred, not blocking.
func renderMathML(raw string, block bool) (out string) {
	display := "inline"
	if block {
		display = "block"
	}
	defer func() {
		if r := recover(); r != nil {
			out = `<code class="math-error">` + string(util.EscapeHTML([]byte(raw))) + `</code>`
		}
	}()
	return l2m.Convert(raw, mathMLNamespace, display, 0)
}
