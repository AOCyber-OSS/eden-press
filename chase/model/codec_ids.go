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

package model

import (
	"encoding/json"
)

// The trailer is the one place document-level data lives in the Markdown: a
// single comment at the end of the document,
//
//	<!-- eden:model {"v":1, ...} -->
//
// holding what has no spelling in the body: the typed Properties and their
// values, and the review Comments (with their anchors and suggestions). A
// document with none of that has no trailer, so simple Markdown stays clean.
// The same comment carries the block and list-item ids that make a Markdown
// round trip stable (see the Blocks field below); that part is added with the id
// reconciliation.
//
// The payload is one json.Marshal on one line. encoding/json sorts map keys and
// escapes `<`, `>` and `&`, so the output is byte-deterministic and can never
// hold `-->`.

// trailerVersion is the only payload version this package reads.
const trailerVersion = 1

// modelTrailer is the JSON of the trailer. FromMarkdown reads it strictly: an
// unknown key or version is ConstructDirectiveJSON.
type modelTrailer struct {
	Version        int               `json:"v"`
	Properties     []PropertyDef     `json:"properties,omitempty"`
	PropertyValues map[string]string `json:"propertyValues,omitempty"`
	Comments       []Comment         `json:"comments,omitempty"`
}

// renderTrailer returns the trailer comment for doc, or "" when doc has nothing
// to put in it.
func renderTrailer(doc *Document) string {
	tr := modelTrailer{
		Version:        trailerVersion,
		Properties:     doc.Properties,
		PropertyValues: doc.PropertyValues,
		Comments:       doc.Comments,
	}
	if len(tr.Properties) == 0 && len(tr.PropertyValues) == 0 && len(tr.Comments) == 0 {
		return ""
	}
	return "<!-- eden:model " + jsonLine(tr) + " -->"
}

// jsonLine is v as one line of JSON. The types passed here (strings, string
// maps and the trailer) cannot fail to marshal.
func jsonLine(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
