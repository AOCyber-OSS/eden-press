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

// Package xmltext is the one escaping layer the hand-written OOXML writers
// (convert/xlsx, convert/docx) put user text through.
//
// Escaping the five metacharacters is not enough. XML 1.0 forbids most C0
// controls (U+0000-U+0008, U+000B, U+000C, U+000E-U+001F), U+FFFE, U+FFFF
// and surrogates outright -- not even a character reference may name them --
// and a part that contains one is not well-formed, so Excel and Word report
// the whole file as corrupt (eden-press#39, aodex#1239). Such characters are
// common in exported chat text: ANSI colour codes carry ESC, pasted terminal
// output carries form feeds and vertical tabs.
//
// Illegal runes, and invalid UTF-8 bytes (which is how a lone surrogate looks
// in a Go string), are dropped rather than replaced with U+FFFD: they carry
// no visible content, and a replacement glyph would litter the document.
// Tab, LF and CR are legal and kept.
package xmltext

import (
	"strings"
	"unicode/utf8"
)

// IsChar reports whether r matches the XML 1.0 Char production.
func IsChar(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

// Clean drops every rune that may not appear in an XML 1.0 document, and
// every byte that is not part of a valid UTF-8 sequence. It returns s itself
// when nothing needs dropping.
func Clean(s string) string {
	clean := true
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || !IsChar(r) {
			clean = false
			break
		}
		i += size
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !(r == utf8.RuneError && size == 1) && IsChar(r) {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

var metachars = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

// Escape returns s safe for an XML 1.0 text node or attribute value: illegal
// characters are dropped (see Clean) and the five metacharacters escaped.
// Whitespace is left literal -- unlike encoding/xml's EscapeText, which turns
// tabs and newlines into character references and so breaks DOCX code runs
// under xml:space="preserve".
func Escape(s string) string {
	return metachars.Replace(Clean(s))
}
