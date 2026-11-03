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
	"errors"

	"go.aocyber.ai/eden-press/chase/model"
)

// RED-phase stubs: signatures only. The real implementation lands in the
// GREEN commit.

var errRenderModelNotImplemented = errors.New("press: RenderModel: not implemented")

// RenderModel is a RED-phase stub.
func RenderModel(doc *model.Document, opts Options) (Output, error) {
	return Output{}, errRenderModelNotImplemented
}

// RenderModelJSON is a RED-phase stub.
func RenderModelJSON(docJSON []byte, opts Options) (Output, error) {
	return Output{}, errRenderModelNotImplemented
}

// fragment is one substituted block: the placeholder token standing in for it
// in the generated Markdown, and its already-sanitized HTML.
type fragment struct {
	token string
	html  string
}

func placeholderToken(nonce string, i int) string {
	return "EPBLK" + nonce + "X" + string(rune('0'+i)) + "X"
}

func spliceFragments(html, nonce string, frags []fragment) (string, error) {
	return "", errRenderModelNotImplemented
}
