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
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
)

// The trailer is the one place document-level data lives in the Markdown: a
// single comment at the end of the document,
//
//	<!-- eden:model {"v":1, ...} -->
//
// holding what has no spelling in the body: the typed Properties and their
// values, the review Comments (with their anchors and suggestions), and the
// block and list-item ids. A document with none of that has no trailer, so simple
// Markdown stays clean.
//
// The payload is one json.Marshal on one line. encoding/json sorts map keys and
// escapes `<`, `>` and `&`, so the output is byte-deterministic and can never
// hold `-->`.
//
// # Stable ids
//
// The model addresses blocks by id (edit_document), and comments anchor to them,
// so a trip through Markdown, and an edit of the Markdown by a person, must not
// renumber them. The trailer's "blocks" lists every block of the document in
// order (depth first, a block before its children) with its id, kind and a
// fingerprint of its content, and for a list block the same for each item.
// FromMarkdown reads the Markdown it is given into a new document, then matches
// that document's blocks against an older sequence (FromMarkdownOptions.Prev if
// given, otherwise the trailer) and carries the ids of the matches over:
//
//  1. blocks equal in kind and fingerprint are matched in order: the common head
//     and tail first, then a longest common subsequence of the middle (a
//     duplicate paragraph pairs by position, not by guess);
//  2. blocks still unmatched that are equal in kind and fingerprint are matched
//     wherever they are, so a block moved by an edit keeps its id;
//  3. between two matched neighbours, the leftover blocks of the same kind are
//     paired in order, so a block whose text was edited keeps its id;
//  4. every list block matched this way does the same with its items.
//
// Whatever is left is new and gets the next free id (EnsureBlockIDs' b_N and
// EnsureListItemIDs' li_N scheme), numbered above every id the older sequence or
// a comment ever named: a comment may still name a block that has been deleted,
// as delete_block leaves it, and a new block must not inherit that comment.
// Comments are never dropped or re-anchored; they are read from the trailer as
// they stand.
//
// The fingerprint is FNV-1a (32 bit, 8 hex digits) of the JSON of the block with
// its id, span and children left out and its text in canonical runs (and of an
// item the same way). It only has to be stable, not secret; a collision at worst
// pairs two blocks that differ.

// trailerVersion is the only payload version this package reads.
const trailerVersion = 1

// modelTrailer is the JSON of the trailer. FromMarkdown reads it strictly: an
// unknown key or version is ConstructDirectiveJSON.
type modelTrailer struct {
	Version        int               `json:"v"`
	Blocks         []trailerBlock    `json:"blocks,omitempty"`
	Properties     []PropertyDef     `json:"properties,omitempty"`
	PropertyValues map[string]string `json:"propertyValues,omitempty"`
	Comments       []Comment         `json:"comments,omitempty"`
}

// trailerBlock is one block of the id sequence: its id, kind and fingerprint, and
// for a list block an [id, fingerprint] pair per item.
type trailerBlock struct {
	ID    string      `json:"id"`
	Kind  BlockKind   `json:"k"`
	FP    string      `json:"f"`
	Items [][2]string `json:"li,omitempty"`
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
	blocks := flatBlocks(doc)
	haveIDs := false
	for _, b := range blocks {
		if b.ID != "" || anyItemID(b) {
			haveIDs = true
			break
		}
	}
	if haveIDs {
		for _, nd := range nodesOf(blocks) {
			tb := trailerBlock{ID: nd.id, Kind: nd.kind, FP: nd.fp}
			for _, it := range nd.items {
				tb.Items = append(tb.Items, [2]string{it.id, it.fp})
			}
			tr.Blocks = append(tr.Blocks, tb)
		}
	}
	if len(tr.Blocks) == 0 && len(tr.Properties) == 0 && len(tr.PropertyValues) == 0 && len(tr.Comments) == 0 {
		return ""
	}
	return "<!-- eden:model " + jsonLine(tr) + " -->"
}

func anyItemID(b *Block) bool {
	for _, it := range b.Items {
		if it.ID != "" {
			return true
		}
	}
	return false
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

// ---- fingerprints and the id sequence ----

func fingerprint(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := fnv.New32a()
	h.Write(b)
	return fmt.Sprintf("%08x", h.Sum32())
}

// canonItem is a list item reduced to what identifies its content: level and
// runs in canonical form.
func canonItem(it ListItem) ListItem {
	return ListItem{Level: it.Level, Runs: canonRuns(runsOfItem(it))}
}

func itemFP(it ListItem) string { return fingerprint(canonItem(it)) }

// blockFP fingerprints a block's own content: everything but its id, span and
// children (a child is a block of the sequence in its own right).
func blockFP(b *Block) string {
	c := *b
	c.ID, c.Span, c.Children = "", nil, nil
	c.Runs, c.Text = canonRuns(runsOfBlock(*b)), ""
	if len(b.Items) > 0 {
		c.Items = make([]ListItem, len(b.Items))
		for i, it := range b.Items {
			c.Items[i] = canonItem(it)
		}
	}
	return fingerprint(c)
}

// flatBlocks lists every block of doc depth first, a block before its children.
func flatBlocks(doc *Document) []*Block {
	var out []*Block
	var walk func([]Block)
	walk = func(bs []Block) {
		for i := range bs {
			out = append(out, &bs[i])
			walk(bs[i].Children)
		}
	}
	for i := range doc.Sections {
		walk(doc.Sections[i].Blocks)
	}
	return out
}

// idItem and idNode are one list item and one block of an id sequence: from a
// Document, from a parsed one, or from a trailer.
type idItem struct{ id, fp string }

type idNode struct {
	id    string
	kind  BlockKind
	fp    string
	items []idItem
}

func nodesOf(blocks []*Block) []idNode {
	out := make([]idNode, len(blocks))
	for i, b := range blocks {
		n := idNode{id: b.ID, kind: b.Kind, fp: blockFP(b)}
		for _, it := range b.Items {
			n.items = append(n.items, idItem{id: it.ID, fp: itemFP(it)})
		}
		out[i] = n
	}
	return out
}

func nodesOfTrailer(tr *modelTrailer) []idNode {
	out := make([]idNode, len(tr.Blocks))
	for i, b := range tr.Blocks {
		n := idNode{id: b.ID, kind: b.Kind, fp: b.FP}
		for _, it := range b.Items {
			n.items = append(n.items, idItem{id: it[0], fp: it[1]})
		}
		out[i] = n
	}
	return out
}

// ---- reconciliation ----

// lcsCellLimit caps the table of the longest-common-subsequence step (a middle of
// 2000 by 2000 blocks); beyond it the middle is left to the move and pairing
// steps, which are linear.
const lcsCellLimit = 4_000_000

// reconcileIDs carries ids from an older sequence (Prev's blocks, else the
// trailer's) to the blocks just read, then gives every block and item still
// without one a fresh id. With neither Prev nor an id sequence in the trailer it
// does nothing: a document read from plain Markdown has no ids, as before.
func (r *mdReader) reconcileIDs(doc *Document, prev *Document) {
	var old []idNode
	var oldComments []Comment
	switch {
	case prev != nil:
		old = nodesOf(flatBlocks(prev))
		oldComments = prev.Comments
	case r.trailer != nil && len(r.trailer.Blocks) > 0:
		old = nodesOfTrailer(r.trailer)
	default:
		return
	}

	floorB, floorL := 0, 0
	for _, n := range old {
		floorB = max(floorB, idNumber(n.id, "b_"))
		for _, it := range n.items {
			floorL = max(floorL, idNumber(it.id, "li_"))
		}
	}
	for _, c := range append(append([]Comment(nil), oldComments...), doc.Comments...) {
		floorB = max(floorB, idNumber(c.BlockID, "b_"))
	}

	cur := flatBlocks(doc)
	curNodes := nodesOf(cur)
	pair := alignIDs(keysOf(old), kindsOf(old), keysOf(curNodes), kindsOf(curNodes))
	for j, i := range pair {
		if i < 0 {
			continue
		}
		cur[j].ID = old[i].id
		if cur[j].Kind != BlockList || len(cur[j].Items) == 0 {
			continue
		}
		oldItems, newItems := old[i].items, curNodes[j].items
		ip := alignIDs(itemKeys(oldItems), make([]string, len(oldItems)), itemKeys(newItems), make([]string, len(newItems)))
		for q, p := range ip {
			if p >= 0 {
				cur[j].Items[q].ID = oldItems[p].id
			}
		}
	}
	assignFreshIDs(doc, floorB, floorL)
}

func keysOf(ns []idNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = string(n.kind) + "\x00" + n.fp
	}
	return out
}

func kindsOf(ns []idNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = string(n.kind)
	}
	return out
}

func itemKeys(items []idItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.fp
	}
	return out
}

// alignIDs matches the entries of a newer sequence to an older one and returns,
// for each new entry, the index of its old partner or -1. keys say which entries
// are equal; kinds say which may stand in for each other when they are not.
func alignIDs(oldKeys, oldKinds, newKeys, newKinds []string) []int {
	pair := make([]int, len(newKeys))
	for i := range pair {
		pair[i] = -1
	}
	used := make([]bool, len(oldKeys))
	var ancOld, ancNew []int // matched in order: the gaps between them are what is left
	set := func(j, i int) { pair[j], used[i] = i, true }

	// 1. Equal entries: the common head, the common tail, then a longest common
	// subsequence of what lies between.
	lo := 0
	for lo < len(oldKeys) && lo < len(newKeys) && oldKeys[lo] == newKeys[lo] {
		set(lo, lo)
		ancOld, ancNew = append(ancOld, lo), append(ancNew, lo)
		lo++
	}
	oe, ne := len(oldKeys), len(newKeys)
	var tailOld, tailNew []int
	for oe > lo && ne > lo && oldKeys[oe-1] == newKeys[ne-1] {
		oe--
		ne--
		set(ne, oe)
		tailOld, tailNew = append(tailOld, oe), append(tailNew, ne)
	}
	for _, m := range lcsPairs(oldKeys[lo:oe], newKeys[lo:ne]) {
		set(lo+m[1], lo+m[0])
		ancOld, ancNew = append(ancOld, lo+m[0]), append(ancNew, lo+m[1])
	}
	for k := len(tailOld) - 1; k >= 0; k-- {
		ancOld, ancNew = append(ancOld, tailOld[k]), append(ancNew, tailNew[k])
	}

	// 2. Equal entries that moved.
	spare := map[string][]int{}
	for i, k := range oldKeys {
		if !used[i] {
			spare[k] = append(spare[k], i)
		}
	}
	for j, k := range newKeys {
		if pair[j] < 0 && len(spare[k]) > 0 {
			set(j, spare[k][0])
			spare[k] = spare[k][1:]
		}
	}

	// 3. Between matched neighbours, pair what is left by kind, in order.
	prevO, prevN := -1, -1
	for a := 0; a <= len(ancOld); a++ {
		nextO, nextN := len(oldKeys), len(newKeys)
		if a < len(ancOld) {
			nextO, nextN = ancOld[a], ancNew[a]
		}
		oi := prevO + 1
		for j := prevN + 1; j < nextN; j++ {
			if pair[j] >= 0 {
				continue
			}
			for k := oi; k < nextO; k++ {
				if !used[k] && oldKinds[k] == newKinds[j] {
					set(j, k)
					oi = k + 1
					break
				}
			}
		}
		prevO, prevN = nextO, nextN
	}
	return pair
}

// lcsPairs is a longest common subsequence of a and b as (index in a, index in b)
// pairs, or nil when the table would exceed lcsCellLimit.
func lcsPairs(a, b []string) [][2]int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 || n*m > lcsCellLimit {
		return nil
	}
	w := m + 1
	dp := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i*w+j] = dp[(i+1)*w+j+1] + 1
			} else {
				dp[i*w+j] = max(dp[(i+1)*w+j], dp[i*w+j+1])
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			out = append(out, [2]int{i, j})
			i++
			j++
		case dp[(i+1)*w+j] >= dp[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// idNumber is N for an id of the form prefix+N, else 0.
func idNumber(id, prefix string) int {
	if !strings.HasPrefix(id, prefix) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// assignFreshIDs gives every block and list item without an id the next free one,
// depth first: b_N for blocks and li_N for items, N one above the highest number
// in doc, in floorB or in floorL. With floors no higher than doc's own it assigns
// exactly what EnsureBlockIDs and EnsureListItemIDs would (a test pins that); the
// floors only keep a retired id from being handed out again.
func assignFreshIDs(doc *Document, floorB, floorL int) {
	nextB, nextL := floorB, floorL
	for _, b := range flatBlocks(doc) {
		nextB = max(nextB, idNumber(b.ID, "b_"))
		for _, it := range b.Items {
			nextL = max(nextL, idNumber(it.ID, "li_"))
		}
	}
	for _, b := range flatBlocks(doc) {
		if b.ID == "" {
			nextB++
			b.ID = "b_" + strconv.Itoa(nextB)
		}
		for i := range b.Items {
			if b.Items[i].ID == "" {
				nextL++
				b.Items[i].ID = "li_" + strconv.Itoa(nextL)
			}
		}
	}
}
