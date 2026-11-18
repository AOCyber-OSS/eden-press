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

import "strings"

// SafeHref reports whether raw is a link target a document may carry, and
// returns it trimmed when it is. It is an allowlist, not a blocklist: http,
// https and mailto URLs, a same-document "#fragment", or a schemeless relative
// reference ("files/report.pdf"), which by construction names no scheme.
// Everything else is refused -- javascript:, data:, vbscript:, file:, blob:
// and any scheme not listed (eden-press#23, #24).
//
// The scheme is read the way a browser reads it: leading spaces and control
// characters are ignored, case does not matter, and a tab or newline inside
// the value (which a browser strips, so "java\tscript:" is javascript:) makes
// the whole value unsafe.
//
// The result must still be attribute-escaped by the caller.
func SafeHref(raw string) (string, bool) {
	s := strings.TrimFunc(raw, func(r rune) bool { return r <= ' ' })
	if s == "" {
		return "", false
	}
	if strings.IndexFunc(s, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 {
		return "", false
	}
	if s[0] == '#' {
		return s, true
	}
	// A scheme is what precedes the first ':' when that ':' comes before any
	// '/', '?' or '#'. "a.png?x=1:2" and "//host/x" have none.
	if i := strings.IndexAny(s, ":/?#"); i >= 0 && s[i] == ':' {
		switch strings.ToLower(s[:i]) {
		case "http", "https", "mailto":
			return s, true
		}
		return "", false
	}
	return s, true
}
