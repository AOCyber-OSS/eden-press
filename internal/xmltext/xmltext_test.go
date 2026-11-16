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

package xmltext

import "testing"

func TestClean(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "café 🚀 &<>", "café 🚀 &<>"},
		{"tab LF CR kept", "a\tb\nc\rd", "a\tb\nc\rd"},
		{"C0 controls dropped", "a\x00b\x01\x08\x0b\x0c\x0e\x1b\x1fc", "abc"},
		{"U+FFFE U+FFFF dropped", "a￾b￿c", "abc"},
		{"U+FFFD itself is legal", "a�b", "a�b"},
		{"lone surrogate bytes dropped", "a\xed\xa0\x80b", "ab"},
		{"truncated UTF-8 dropped", "a\xe2\x82b", "ab"},
		{"supplementary plane kept", "\U0010FFFF\U00010000", "\U0010FFFF\U00010000"},
		{"only illegal", "\x00\x0b￿\xff", ""},
		{"DEL and C1 are legal in XML 1.0", "\x7f\u0085", "\x7f\u0085"},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("%s: Clean(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestEscape(t *testing.T) {
	if got, want := Escape("a\x00&<>\"'\tb"), "a&amp;&lt;&gt;&quot;&apos;\tb"; got != want {
		t.Errorf("Escape = %q, want %q", got, want)
	}
}
