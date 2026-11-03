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
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The corpus test (objective 1026, TRD 03) reads every Markdown file of the
// repository's conformance corpus with the strict codec and records, per case,
// either that it was accepted or the Construct that refused it and where. The
// report is a golden file, so a change to what the codec refuses shows up as a
// diff in review; `go test ./chase/model -run TestCodecCorpus -update` rewrites it.
//
// The corpus is Marp-authored slides, so refusals here (raw HTML for backgrounds,
// say) are expected and are exactly what an operator wants to see before a
// migration. Every case that parses must satisfy the identity properties, and every
// case, refused or not, must read leniently into a Document that does too.

var updateCorpusReport = flag.Bool("update", false, "rewrite testdata/codec_corpus_report.txt")

const corpusReportPath = "testdata/codec_corpus_report.txt"

// corpusCases returns name -> path of every Markdown file of the corpus, sorted by name.
func corpusCases(t *testing.T) (names []string, paths map[string]string) {
	t.Helper()
	paths = map[string]string{}
	found, err := filepath.Glob(filepath.Join("..", "..", "conformance", "corpus", "cases", "*", "input.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range found {
		paths[filepath.Base(filepath.Dir(p))] = p
	}
	// The superset fixture is the repository's own Marp deck that exercises every
	// directive; it belongs to the same sweep.
	paths["testdata/superset_fixture"] = filepath.Join("testdata", "superset_fixture.md")
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, paths
}

func TestCodecCorpus(t *testing.T) {
	names, paths := corpusCases(t)
	if len(names) < 10 {
		t.Fatalf("found only %d corpus cases (%v): the corpus moved?", len(names), names)
	}

	var report strings.Builder
	report.WriteString("# Strict and lenient reads of the conformance corpus (go test ./chase/model -run TestCodecCorpus -update).\n")
	report.WriteString("# case<TAB>strict<TAB>lenient; strict is OK or construct@byte-offset.\n")
	byConstruct := map[string]int{}
	ok := 0
	for _, name := range names {
		raw, err := os.ReadFile(paths[name])
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)

		strict := "OK"
		d, err := FromMarkdown(src, FromMarkdownOptions{})
		var ue *UnrepresentableError
		switch {
		case err == nil:
			ok++
			if perr := propCheck(d); perr != nil {
				t.Errorf("%s: strict read is accepted but violates a property: %v", name, perr)
			}
		case errors.As(err, &ue):
			strict = fmt.Sprintf("%s@%d", ue.Construct, ue.Offset)
			byConstruct[ue.Construct]++
		default:
			t.Fatalf("%s: strict read failed with a %T that is not an UnrepresentableError: %v", name, err, err)
		}

		lenient := "OK"
		l, err := FromMarkdown(src, FromMarkdownOptions{Lenient: true})
		if err != nil {
			t.Fatalf("%s: lenient read must not fail: %v", name, err)
		}
		if todoEmptyTailHazard(l) {
			lenient = "OK (known limit: todo empty tail, identity not checked)"
		} else if perr := propCheck(l); perr != nil {
			t.Errorf("%s: lenient read violates a property: %v", name, perr)
		}
		fmt.Fprintf(&report, "%s\t%s\t%s\n", name, strict, lenient)
	}

	report.WriteString("#\n")
	fmt.Fprintf(&report, "# total %d, accepted by strict mode %d, refused %d\n", len(names), ok, len(names)-ok)
	var cs []string
	for c := range byConstruct {
		cs = append(cs, c)
	}
	sort.Strings(cs)
	for _, c := range cs {
		fmt.Fprintf(&report, "# refused by %s: %d\n", c, byConstruct[c])
	}

	if *updateCorpusReport {
		if err := os.WriteFile(corpusReportPath, []byte(report.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(corpusReportPath)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if string(want) != report.String() {
		t.Errorf("corpus report changed (run with -update if intended)\n--- golden ---\n%s\n--- now ---\n%s", want, report.String())
	}
}
