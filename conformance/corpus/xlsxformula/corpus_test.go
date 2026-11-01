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

package xlsxformula

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCorpusJSONIsCurrent(t *testing.T) {
	want, err := CasesJSON()
	if err != nil {
		t.Fatalf("CasesJSON: %v", err)
	}
	got, err := os.ReadFile("cases.json")
	if err != nil {
		t.Fatalf("read cases.json (run go generate ./conformance/corpus/xlsxformula): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("cases.json is stale (%d vs %d bytes). Regenerate with go generate ./conformance/corpus/xlsxformula",
			len(got), len(want))
	}
}

func TestCasesJSONIsDeterministic(t *testing.T) {
	a, err := CasesJSON()
	if err != nil {
		t.Fatalf("CasesJSON: %v", err)
	}
	for i := 0; i < 5; i++ {
		b, err := CasesJSON()
		if err != nil {
			t.Fatalf("CasesJSON #%d: %v", i+2, err)
		}
		if string(a) != string(b) {
			t.Fatalf("CasesJSON nondeterministic on call %d", i+2)
		}
	}
}

func TestCasesJSONRoundTrips(t *testing.T) {
	b, err := CasesJSON()
	if err != nil {
		t.Fatalf("CasesJSON: %v", err)
	}
	var back Corpus
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Schema != SchemaID {
		t.Errorf("schema = %q, want %q", back.Schema, SchemaID)
	}
	if back.Decisions != Locked {
		t.Errorf("decisions = %+v, want %+v", back.Decisions, Locked)
	}
	if len(back.Cases) != len(Cases) {
		t.Fatalf("cases = %d, want %d", len(back.Cases), len(Cases))
	}
}

func TestCasesAreWellFormed(t *testing.T) {
	if len(Cases) == 0 {
		t.Fatal("empty corpus asserts nothing")
	}
	ids := map[string]bool{}
	for i, c := range Cases {
		if c.ID == "" {
			t.Errorf("case %d missing id", i)
		}
		if ids[c.ID] {
			t.Errorf("duplicate id %q", c.ID)
		}
		ids[c.ID] = true
		if c.FromSheet == "" {
			t.Errorf("%s: missing fromSheet", c.ID)
		}
		if c.Expr == "" {
			t.Errorf("%s: missing expr", c.ID)
		}
		if c.ExpectError == "" && c.Expect == nil {
			t.Errorf("%s: need expect or expectError", c.ID)
		}
		if c.ExpectError != "" && c.Expect != nil {
			t.Errorf("%s: expect and expectError are mutually exclusive", c.ID)
		}
		if c.Reason == "" {
			t.Errorf("%s: missing reason", c.ID)
		}
		switch c.Phase {
		case "shipped", "grammar", "A", "B", "C":
		default:
			t.Errorf("%s: unknown phase %q", c.ID, c.Phase)
		}
	}
}

func TestDecisionLock(t *testing.T) {
	if Locked.EngineThroughPhaseB != "dart" {
		t.Fatalf("engine = %q, want dart", Locked.EngineThroughPhaseB)
	}
	if Locked.OOXMLWriter != "hand-rolled-convert-xlsx" {
		t.Fatalf("writer = %q", Locked.OOXMLWriter)
	}
	if Locked.CachedFormulaValue != "omit" {
		t.Fatalf("cached value policy = %q, want omit", Locked.CachedFormulaValue)
	}
	if !Locked.FullCalcOnLoadWhenFormulas {
		t.Fatal("fullCalcOnLoadWhenFormulas must be true")
	}
}

func TestInventoryPresent(t *testing.T) {
	b, err := os.ReadFile("inventory.json")
	if err != nil {
		t.Fatalf("inventory.json: %v", err)
	}
	var inv struct {
		Schema  string `json:"schema"`
		Pinned  string `json:"pinned"`
		Counts  map[string]int `json:"counts"`
		Functions []struct {
			Name  string `json:"name"`
			Phase string `json:"phase"`
		} `json:"functions"`
	}
	if err := json.Unmarshal(b, &inv); err != nil {
		t.Fatalf("unmarshal inventory: %v", err)
	}
	if inv.Schema != "eden-press.xlsxformula.inventory/v1" {
		t.Errorf("schema = %q", inv.Schema)
	}
	if inv.Pinned == "" {
		t.Error("inventory missing pinned date")
	}
	if inv.Counts["inventory"] < 200 {
		t.Errorf("inventory count %d looks too small for a denominator", inv.Counts["inventory"])
	}
	if inv.Counts["phaseA"] < 40 {
		t.Errorf("phaseA count %d < 40 target", inv.Counts["phaseA"])
	}
	if len(inv.Functions) != inv.Counts["inventory"] {
		t.Errorf("functions len %d != counts.inventory %d", len(inv.Functions), inv.Counts["inventory"])
	}
}

func TestHeadlineShippedCasesPresent(t *testing.T) {
	want := []string{"arith-precedence", "sum-range", "div-zero", "unknown-fn", "identifier-007-not-number"}
	have := map[string]bool{}
	for _, c := range Cases {
		have[c.ID] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("missing headline case %q", id)
		}
	}
}
