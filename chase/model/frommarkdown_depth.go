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

// limitLinePrefixes bounds, before goldmark sees the body, how many container
// markers (block quote '>' and list markers "-", "*", "+", "1." / "1)") one
// line may open. goldmark re-reads a line's offset for every block it opens on
// it, so its parse is quadratic in the markers on one line: '- 'x19600 took
// 10 s, and AODex accepts 400 KB of Markdown (eden-press#29). The tree-level
// limit (limitNesting) runs after the parse and cannot prevent that.
//
// For each line whose leading container prefix holds more than MaxNestingDepth
// markers:
//   - strict: at is the body offset of the first marker past the limit, and
//     the caller refuses ConstructNestingDepth there (out is body unchanged);
//   - lenient: the markers past the limit are deleted. They open only empty
//     containers (a prefix holds nothing but markers), so no text is lost: the
//     line's content lands in the deepest container allowed, which is where
//     limitNesting's flattening would put it. drops lists, for each deleted
//     byte, the offset in out at which it was removed, so spans can be mapped
//     back to the input (see mdReader.orig).
//
// It is a lexical approximation of CommonMark's container rules, deliberately
// cheap and linear: lines inside a top-level fenced code block and thematic
// breaks ("* * * *") are left alone. A code line inside a quoted or listed
// fence is still scanned; it would only be touched if it began with more than
// MaxNestingDepth markers.
func limitLinePrefixes(body string, lenient bool) (out string, drops []int, at int) {
	var b strings.Builder
	at = -1
	fence := "" // the open top-level fence's marker run ("```", "~~~~"), or ""
	changed := false
	for start := 0; start < len(body); {
		end := strings.IndexByte(body[start:], '\n')
		if end < 0 {
			end = len(body)
		} else {
			end += start + 1 // keep the newline with its line
		}
		line := body[start:end]
		cut := -1 // offset in line where the excess markers start
		var cutEnd int
		switch {
		case fence != "":
			if closesFence(line, fence) {
				fence = ""
			}
		case opensFence(line) != "":
			fence = opensFence(line)
		case !isThematicBreak(line):
			if ends := prefixMarkerEnds(line, MaxNestingDepth+1); len(ends) > MaxNestingDepth {
				cut = ends[MaxNestingDepth-1]
				cutEnd = prefixEnd(line, ends[len(ends)-1])
			}
		}
		if cut >= 0 {
			if !lenient {
				// The first byte of marker MaxNestingDepth+1, past the
				// whitespace that belongs to marker MaxNestingDepth.
				return body, nil, start + skipSpaces(line, cut)
			}
			if !changed {
				b.Grow(len(body))
				b.WriteString(body[:start])
				changed = true
			}
			b.WriteString(line[:cut])
			for i := cut; i < cutEnd; i++ {
				drops = append(drops, b.Len())
			}
			b.WriteString(line[cutEnd:])
		} else if changed {
			b.WriteString(line)
		}
		start = end
	}
	if !changed {
		return body, nil, at
	}
	return b.String(), drops, at
}

// prefixMarkerEnds returns, for each container marker at the start of line in
// order, the offset just past it and the whitespace it owns. It stops after
// limit markers, which is all limitLinePrefixes needs: the cost is bounded by
// limit, not by the length of the prefix.
func prefixMarkerEnds(line string, limit int) []int {
	var ends []int
	i := 0
	for len(ends) < limit {
		j := i
		for n := 0; n < 3 && j < len(line) && line[j] == ' '; n++ {
			j++
		}
		if j >= len(line) {
			break
		}
		switch c := line[j]; {
		case c == '>':
			j++
			if j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
		case (c == '-' || c == '*' || c == '+') && j+1 < len(line) && (line[j+1] == ' ' || line[j+1] == '\t'):
			j = afterListMarker(line, j+1)
		case c >= '0' && c <= '9':
			k := j
			for k < len(line) && k-j < 9 && line[k] >= '0' && line[k] <= '9' {
				k++
			}
			if k+1 >= len(line) || (line[k] != '.' && line[k] != ')') || (line[k+1] != ' ' && line[k+1] != '\t') {
				return ends
			}
			j = afterListMarker(line, k+1)
		default:
			return ends
		}
		ends = append(ends, j)
		i = j
	}
	return ends
}

// afterListMarker skips the one to four spaces after a list marker (more
// would start an indented code block, which is content, not a marker).
func afterListMarker(line string, j int) int {
	for n := 0; n < 4 && j < len(line) && (line[j] == ' ' || line[j] == '\t'); n++ {
		j++
	}
	return j
}

// prefixEnd is the end of the whole container prefix of line, from from (the
// end of a marker already found) onwards: every further marker is skipped.
// Unlike prefixMarkerEnds it is not bounded; each step is O(1), so a prefix
// costs its own length once.
func prefixEnd(line string, from int) int {
	for {
		ends := prefixMarkerEnds(line[from:], 1)
		if len(ends) == 0 {
			return from
		}
		from += ends[0]
	}
}

// skipSpaces returns the offset of the first non-space byte at or after i.
func skipSpaces(line string, i int) int {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

// opensFence returns the marker run of a top-level fence opening on line
// ("```", "~~~~"), or "".
func opensFence(line string) string {
	s := strings.TrimLeft(line, " ")
	if len(line)-len(s) > 3 || len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return ""
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	if n < 3 || (s[0] == '`' && strings.Contains(s[n:], "`")) {
		return ""
	}
	return s[:n]
}

// closesFence reports whether line closes the fence opened by run.
func closesFence(line, run string) bool {
	s := strings.TrimLeft(line, " ")
	if len(line)-len(s) > 3 {
		return false
	}
	n := 0
	for n < len(s) && s[n] == run[0] {
		n++
	}
	return n >= len(run) && strings.TrimSpace(s[n:]) == ""
}

// isThematicBreak reports whether line is a thematic break: three or more of
// one of '-', '*', '_', with only spaces or tabs besides.
func isThematicBreak(line string) bool {
	var c byte
	n := 0
	for i := 0; i < len(line); i++ {
		switch ch := line[i]; ch {
		case ' ', '\t', '\n':
		case '-', '*', '_':
			if c != 0 && ch != c {
				return false
			}
			c = ch
			n++
		default:
			return false
		}
	}
	return n >= 3
}
