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

// Package xlsxformula publishes the spreadsheet formula acceptance corpus for
// Objective 9 (XLSX-F01). Cases are the source of truth in Go; cases.json is a
// generated projection for the Dart evaluator in bind/eden_ui — the same
// anti-drift pattern as conformance/corpus/xlsxtyping.
//
// Decision lock (Wave 0): Dart evaluates through Phase B; a Go twin is a
// Wave-4 spike only; convert/xlsx stays hand-rolled and omits cached <v>.
package xlsxformula

import (
	"bytes"
	"encoding/json"
)

//go:generate go run ./internal/gencases

// SchemaID identifies the JSON projection.
const SchemaID = "eden-press.xlsxformula/v1"

// Decisions locked in Wave 0. Changing one is a new schema / objective decision,
// not a silent edit.
type Decisions struct {
	EngineThroughPhaseB         string `json:"engineThroughPhaseB"`
	GoTwin                      string `json:"goTwin"`
	OOXMLWriter                 string `json:"ooXmlWriter"`
	CachedFormulaValue          string `json:"cachedFormulaValue"`
	FullCalcOnLoadWhenFormulas  bool   `json:"fullCalcOnLoadWhenFormulas"`
}

// Locked is the Wave-0 decision record embedded in every cases.json.
var Locked = Decisions{
	EngineThroughPhaseB:        "dart",
	GoTwin:                     "wave-4-spike-only",
	OOXMLWriter:                "hand-rolled-convert-xlsx",
	CachedFormulaValue:         "omit",
	FullCalcOnLoadWhenFormulas: true,
}

// Case is one formula evaluation agreement.
type Case struct {
	ID          string                       `json:"id"`
	Phase       string                       `json:"phase"`
	Category    string                       `json:"category"`
	Sheet       map[string]map[string]string `json:"sheet"`
	FromSheet   string                       `json:"fromSheet"`
	Expr        string                       `json:"expr"`
	Expect      any                          `json:"expect,omitempty"`
	ExpectError string                       `json:"expectError,omitempty"`
	// Now is an optional RFC3339 UTC timestamp that pins TODAY/NOW for the case.
	Now string `json:"now,omitempty"`
	// Rand pins RAND/RANDBETWEEN (0..1 exclusive upper semantics for RAND).
	Rand   *float64 `json:"rand,omitempty"`
	Reason string   `json:"reason"`
}

// Cases is the agreed formula surface. Append only — renumbering ids breaks
// Dart failure messages that cite them.
var Cases = []Case{
	// --- shipped (PR #11) ---
	{
		ID: "arith-precedence", Phase: "shipped", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "1+2*3", Expect: 7.0,
		Reason: "operator precedence must match Excel: multiply before add",
	},
	{
		ID: "arith-parens", Phase: "shipped", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "(1+2)*3", Expect: 9.0,
		Reason: "parentheses override precedence",
	},
	{
		ID: "sum-range", Phase: "shipped", Category: "math",
		Sheet: map[string]map[string]string{"FY": {"A1": "1", "A2": "2", "A3": "3"}},
		FromSheet: "FY", Expr: "SUM(A1:A3)", Expect: 6.0,
		Reason: "SUM over a contiguous numeric range",
	},
	{
		ID: "average-range", Phase: "shipped", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {"A1": "1", "A2": "2", "A3": "3"}},
		FromSheet: "FY", Expr: "AVERAGE(A1:A3)", Expect: 2.0,
		Reason: "AVERAGE ignores blanks via sparse visit; three numbers mean 2",
	},
	{
		ID: "sum-skips-blanks", Phase: "shipped", Category: "math",
		Sheet: map[string]map[string]string{"FY": {"A1": "1", "A3": "3"}},
		FromSheet: "FY", Expr: "SUM(A1:A3)", Expect: 4.0,
		Reason: "sparse sheets: blanks are not zeros for SUM/AVERAGE/MIN/MAX",
	},
	{
		ID: "cross-sheet", Phase: "shipped", Category: "grammar",
		Sheet: map[string]map[string]string{
			"FY":    {},
			"Notes": {"A1": "4"},
		},
		FromSheet: "FY", Expr: "Notes!A1*2", Expect: 8.0,
		Reason: "cross-sheet refs without quoting",
	},
	{
		ID: "div-zero", Phase: "shipped", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "1/0", ExpectError: "#DIV/0!",
		Reason: "division by zero is #DIV/0!, not infinity",
	},
	{
		ID: "unknown-fn", Phase: "shipped", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "POW(2,3)", ExpectError: "#NAME?",
		Reason: "unknown functions stay #NAME? so Excel can still calculate the stored formula",
	},
	{
		ID: "identifier-007-not-number", Phase: "shipped", Category: "typing",
		Sheet: map[string]map[string]string{"FY": {"A1": "1", "C3": "007"}},
		FromSheet: "FY", Expr: "A1+C3", ExpectError: "#VALUE!",
		Reason: "xlsxtyping D6: 007 is text; arithmetic on it is #VALUE!",
	},

	// --- grammar wave ---
	{
		ID: "compare-gt", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "3>2", Expect: true,
		Reason: "comparison operators are required before IF/COUNTIF",
	},
	{
		ID: "compare-eq", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "2=2", Expect: true,
		Reason: "equality inside formulas uses =, distinct from the formula-bar prefix",
	},
	{
		ID: "compare-ne", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "2<>3", Expect: true,
		Reason: "Excel inequality is <>",
	},
	{
		ID: "concat", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `"A"&"B"`, Expect: "AB",
		Reason: "ampersand concatenation",
	},
	{
		ID: "string-literal", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `"hello"`, Expect: "hello",
		Reason: "double-quoted string literals",
	},
	{
		ID: "bool-true-literal", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "TRUE", Expect: true,
		Reason: "TRUE/FALSE are literals, not only functions",
	},
	{
		ID: "abs-ref-ignored-for-lookup", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {"A1": "5"}},
		FromSheet: "FY", Expr: "$A$1*2", Expect: 10.0,
		Reason: "absolute markers do not change evaluation of a single cell",
	},
	{
		ID: "quoted-sheet", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{
			"FY":       {},
			"My Sheet": {"A1": "3"},
		},
		FromSheet: "FY", Expr: `'My Sheet'!A1+1`, Expect: 4.0,
		Reason: "sheet names with spaces require single quotes",
	},
	{
		ID: "power-op", Phase: "grammar", Category: "grammar",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "2^3", Expect: 8.0,
		Reason: "Excel exponentiation operator",
	},

	// --- Phase A logical ---
	{
		ID: "if-true", Phase: "A", Category: "logical",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `IF(1=1,"yes","no")`, Expect: "yes",
		Reason: "IF is the daily-driver gate for Phase A",
	},
	{
		ID: "if-false", Phase: "A", Category: "logical",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `IF(1=0,"yes","no")`, Expect: "no",
		Reason: "IF false branch",
	},
	{
		ID: "and-or", Phase: "A", Category: "logical",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "AND(TRUE,OR(FALSE,TRUE))", Expect: true,
		Reason: "AND/OR short-circuit-capable boolean combinators",
	},
	{
		ID: "not", Phase: "A", Category: "logical",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "NOT(FALSE)", Expect: true,
		Reason: "NOT",
	},
	{
		ID: "iferror", Phase: "A", Category: "logical",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `IFERROR(1/0,"x")`, Expect: "x",
		Reason: "IFERROR catches evaluation errors",
	},

	// --- Phase A math ---
	{
		ID: "abs-round", Phase: "A", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "ROUND(ABS(-3.14159),2)", Expect: 3.14,
		Reason: "ABS + ROUND",
	},
	{
		ID: "power-fn", Phase: "A", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "POWER(2,10)", Expect: 1024.0,
		Reason: "POWER function alongside ^",
	},
	{
		ID: "mod-int", Phase: "A", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "MOD(10,3)+INT(3.9)", Expect: 4.0,
		Reason: "MOD and INT",
	},
	{
		ID: "sqrt-pi", Phase: "A", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "ROUND(SQRT(PI()),5)", Expect: 1.77245,
		Reason: "SQRT and PI",
	},
	{
		ID: "product", Phase: "A", Category: "math",
		Sheet: map[string]map[string]string{"FY": {"A1": "2", "A2": "3", "A3": "4"}},
		FromSheet: "FY", Expr: "PRODUCT(A1:A3)", Expect: 24.0,
		Reason: "PRODUCT over a range",
	},

	// --- Phase A statistical / text / info ---
	{
		ID: "count-counta", Phase: "A", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {"A1": "1", "A2": "x", "A3": "3"}},
		FromSheet: "FY", Expr: "COUNT(A1:A3)+COUNTA(A1:A3)", Expect: 5.0,
		Reason: "COUNT numbers only; COUNTA non-empty",
	},
	{
		ID: "left-upper", Phase: "A", Category: "text",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `LEFT(UPPER("hello"),2)`, Expect: "HE",
		Reason: "LEFT and UPPER",
	},
	{
		ID: "len-trim", Phase: "A", Category: "text",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `LEN(TRIM("  ab  "))`, Expect: 2.0,
		Reason: "LEN and TRIM",
	},
	{
		ID: "isnumber-isblank", Phase: "A", Category: "information",
		Sheet: map[string]map[string]string{"FY": {"A1": "1"}},
		FromSheet: "FY", Expr: "AND(ISNUMBER(A1),ISBLANK(B1))", Expect: true,
		Reason: "ISNUMBER / ISBLANK",
	},
	{
		ID: "concat-fn", Phase: "A", Category: "text",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `CONCAT("a","b","c")`, Expect: "abc",
		Reason: "CONCAT joins arguments",
	},

	// --- Phase B date & time ---
	{
		ID: "date-serial", Phase: "B", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "DATE(2020,1,15)", Expect: 43845.0,
		Reason: "DATE returns an Excel serial (2020-01-15)",
	},
	{
		ID: "date-ymd-extract", Phase: "B", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "YEAR(DATE(2020,3,5))+MONTH(DATE(2020,3,5))+DAY(DATE(2020,3,5))", Expect: 2028.0,
		Reason: "YEAR/MONTH/DAY round-trip through DATE",
	},
	{
		ID: "today-pinned", Phase: "B", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "TODAY()", Expect: 43845.0,
		Now: "2020-01-15T12:00:00Z",
		Reason: "TODAY uses the pinned corpus clock when provided",
	},
	{
		ID: "edate-eomonth", Phase: "B", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "EDATE(DATE(2020,1,31),1)=DATE(2020,2,29)", Expect: true,
		Reason: "EDATE clamps to end of shorter month",
	},
	{
		ID: "days-between", Phase: "B", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "DAYS(DATE(2020,1,20),DATE(2020,1,10))", Expect: 10.0,
		Reason: "DAYS is end-minus-start in serial days",
	},
	{
		ID: "time-fraction", Phase: "B", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "TIME(6,0,0)", Expect: 0.25,
		Reason: "TIME returns a fraction of a day",
	},

	// --- Phase B lookup ---
	{
		ID: "index-match", Phase: "B", Category: "lookup",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "a", "A2": "b", "A3": "c",
			"B1": "10", "B2": "20", "B3": "30",
		}},
		FromSheet: "FY", Expr: `INDEX(B1:B3,MATCH("b",A1:A3,0))`, Expect: 20.0,
		Reason: "INDEX/MATCH exact lookup",
	},
	{
		ID: "vlookup-exact", Phase: "B", Category: "lookup",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "x", "B1": "1",
			"A2": "y", "B2": "2",
			"A3": "z", "B3": "3",
		}},
		FromSheet: "FY", Expr: `VLOOKUP("y",A1:B3,2,FALSE)`, Expect: 2.0,
		Reason: "VLOOKUP exact match (range_lookup FALSE)",
	},
	{
		ID: "xlookup", Phase: "B", Category: "lookup",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "x", "A2": "y", "A3": "z",
			"B1": "1", "B2": "2", "B3": "3",
		}},
		FromSheet: "FY", Expr: `XLOOKUP("z",A1:A3,B1:B3)`, Expect: 3.0,
		Reason: "XLOOKUP exact over parallel columns",
	},
	{
		ID: "choose-column-row", Phase: "B", Category: "lookup",
		Sheet: map[string]map[string]string{"FY": {"B5": "9"}},
		FromSheet: "FY", Expr: `CHOOSE(2,"a","b","c")&COLUMN(B5)&ROW(B5)`, Expect: "b25",
		Reason: "CHOOSE plus COLUMN/ROW on a bare ref",
	},
	{
		ID: "address", Phase: "B", Category: "lookup",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "ADDRESS(2,3,4)", Expect: "C2",
		Reason: "ADDRESS with relative style 4",
	},

	// --- Phase B conditional aggregates ---
	{
		ID: "sumif", Phase: "B", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "1", "A2": "2", "A3": "3",
			"B1": "10", "B2": "20", "B3": "30",
		}},
		FromSheet: "FY", Expr: `SUMIF(A1:A3,">1",B1:B3)`, Expect: 50.0,
		Reason: "SUMIF with numeric criteria and sum_range",
	},
	{
		ID: "countif", Phase: "B", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "apple", "A2": "banana", "A3": "apricot",
		}},
		FromSheet: "FY", Expr: `COUNTIF(A1:A3,"a*")`, Expect: 2.0,
		Reason: "COUNTIF wildcard prefix",
	},
	{
		ID: "averageif", Phase: "B", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "1", "A2": "2", "A3": "3", "A4": "4",
		}},
		FromSheet: "FY", Expr: `AVERAGEIF(A1:A4,">=2")`, Expect: 3.0,
		Reason: "AVERAGEIF over the criteria range itself",
	},
	{
		ID: "sumifs", Phase: "B", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "east", "A2": "west", "A3": "east",
			"B1": "1", "B2": "2", "B3": "3",
			"C1": "10", "C2": "20", "C3": "30",
		}},
		FromSheet: "FY", Expr: `SUMIFS(C1:C3,A1:A3,"east",B1:B3,">1")`, Expect: 30.0,
		Reason: "SUMIFS with two criteria pairs",
	},
	{
		ID: "countifs", Phase: "B", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {
			"A1": "1", "A2": "2", "A3": "3", "A4": "4",
			"B1": "x", "B2": "y", "B3": "x", "B4": "x",
		}},
		FromSheet: "FY", Expr: `COUNTIFS(A1:A4,">=2",B1:B4,"x")`, Expect: 2.0,
		Reason: "COUNTIFS conjunction",
	},

	// --- Phase C first tranche ---
	{
		ID: "trig-degrees", Phase: "C", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "ROUND(DEGREES(ACOS(-1)),0)", Expect: 180.0,
		Reason: "ACOS/DEGREES identity for π",
	},
	{
		ID: "sin-radians", Phase: "C", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "ROUND(SIN(RADIANS(30)),10)", Expect: 0.5,
		Reason: "SIN of 30 degrees via RADIANS",
	},
	{
		ID: "combin-fact", Phase: "C", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "COMBIN(5,2)+FACT(4)", Expect: 34.0,
		Reason: "COMBIN and FACT",
	},
	{
		ID: "even-odd-mround", Phase: "C", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "EVEN(2.5)+ODD(2.5)+MROUND(10,3)", Expect: 16.0,
		Reason: "EVEN/ODD/MROUND",
	},
	{
		ID: "sumsq-large-small", Phase: "C", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {"A1": "1", "A2": "4", "A3": "9"}},
		FromSheet: "FY", Expr: "SUMSQ(A1:A3)+LARGE(A1:A3,1)+SMALL(A1:A3,1)", Expect: 108.0,
		Reason: "SUMSQ plus LARGE/SMALL",
	},
	{
		ID: "stdev-s", Phase: "C", Category: "statistical",
		Sheet: map[string]map[string]string{"FY": {"A1": "2", "A2": "4", "A3": "4", "A4": "4", "A5": "5", "A6": "5", "A7": "7", "A8": "9"}},
		FromSheet: "FY", Expr: "ROUND(STDEV.S(A1:A8),6)", Expect: 2.13809,
		Reason: "STDEV.S sample standard deviation",
	},
	{
		ID: "exact-clean", Phase: "C", Category: "text",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `AND(EXACT("Ab","Ab"),NOT(EXACT("Ab","ab")),CLEAN("a"&CHAR(1)&"b")="ab")`, Expect: true,
		Reason: "EXACT is case-sensitive; CLEAN strips controls",
	},
	{
		ID: "rand-pinned", Phase: "C", Category: "math",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "RAND()", Expect: 0.25,
		Rand: floatPtr(0.25),
		Reason: "RAND uses the pinned corpus clock when provided",
	},
	{
		ID: "pmt-basic", Phase: "C", Category: "financial",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "ROUND(PMT(0.05/12,60,10000),2)", Expect: -188.71,
		Reason: "PMT loan payment (Excel sign convention)",
	},
	{
		ID: "networkdays", Phase: "C", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: "NETWORKDAYS(DATE(2020,1,1),DATE(2020,1,10))", Expect: 8.0,
		Reason: "NETWORKDAYS excludes weekends",
	},
	{
		ID: "timevalue", Phase: "C", Category: "date",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `TIMEVALUE("6:00")`, Expect: 0.25,
		Reason: "TIMEVALUE parses a clock string to a day fraction",
	},
	{
		ID: "hyperlink-display", Phase: "C", Category: "lookup",
		Sheet: map[string]map[string]string{"FY": {}}, FromSheet: "FY",
		Expr: `HYPERLINK("https://example.com","click")`, Expect: "click",
		Reason: "HYPERLINK returns friendly name; no navigation in-grid",
	},
}

func floatPtr(v float64) *float64 { return &v }

// Corpus is the JSON projection top-level shape.
type Corpus struct {
	Schema    string    `json:"schema"`
	Decisions Decisions `json:"decisions"`
	Cases     []Case    `json:"cases"`
}

// CasesJSON renders the corpus bytes cases.json must contain.
func CasesJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Corpus{
		Schema:    SchemaID,
		Decisions: Locked,
		Cases:     Cases,
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
