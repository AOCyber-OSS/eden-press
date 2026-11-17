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

package xlsx

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"io"
	"testing"
)

// zipPart is one entry of a hand-built package. Either body (small, literal)
// or fill (a byte repeated n times, streamed so the test never holds the
// inflated part) supplies its content.
type zipPart struct {
	name string
	body string
	fill byte
	n    int64
	// declare, when non-zero, overrides the uncompressed size written to the
	// entry's header: a lying header that under-states what the part inflates
	// to.
	declare uint64
}

// buildPackage writes parts into a zip. Content of fill parts is deflated as
// it is generated, so a part that inflates to hundreds of megabytes costs a
// few hundred kilobytes here.
func buildPackage(t *testing.T, parts []zipPart) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		if p.declare != 0 {
			writeLyingPart(t, zw, p)
			continue
		}
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatalf("create %s: %v", p.name, err)
		}
		if p.n > 0 {
			if err := streamFill(w, p.fill, p.n); err != nil {
				t.Fatalf("fill %s: %v", p.name, err)
			}
			continue
		}
		if _, err := io.WriteString(w, p.body); err != nil {
			t.Fatalf("write %s: %v", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// writeLyingPart deflates the real content, then stores it raw under a header
// that declares p.declare uncompressed bytes.
func writeLyingPart(t *testing.T, zw *zip.Writer, p zipPart) {
	t.Helper()
	var raw bytes.Buffer
	fw, err := newDeflater(&raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := streamFill(fw, p.fill, p.n); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name:               p.name,
		Method:             zip.Deflate,
		CompressedSize64:   uint64(raw.Len()),
		UncompressedSize64: p.declare,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
}

func streamFill(w io.Writer, c byte, n int64) error {
	chunk := bytes.Repeat([]byte{c}, 1<<20)
	for n > 0 {
		k := int64(len(chunk))
		if n < k {
			k = n
		}
		if _, err := w.Write(chunk[:k]); err != nil {
			return err
		}
		n -= k
	}
	return nil
}

// minimalParts is the smallest package Open accepts: one sheet whose
// worksheet part is sheetPart.
func minimalParts(sheetPart zipPart) []zipPart {
	sheetPart.name = "xl/worksheets/sheet1.xml"
	return []zipPart{
		{name: "xl/workbook.xml", body: `<workbook><sheets><sheet name="S" sheetId="1" r:id="rId1" xmlns:r="r"/></sheets></workbook>`},
		{name: "xl/_rels/workbook.xml.rels", body: `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`},
		sheetPart,
	}
}

// A part that inflates past MaxPartBytes is refused with ErrLimitExceeded.
// The part is whitespace, which is valid XML filler, so the only possible
// reason for the refusal is the size.
func TestOpenRejectsOversizedPart(t *testing.T) {
	pkg := buildPackage(t, minimalParts(zipPart{fill: ' ', n: MaxPartBytes + 1}))
	t.Logf("crafted package: %d bytes compressed, %d bytes inflated part", len(pkg), MaxPartBytes+1)

	_, err := Open(pkg)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Open(oversized part) err = %v, want ErrLimitExceeded", err)
	}
}

func newDeflater(w io.Writer) (io.WriteCloser, error) {
	return flate.NewWriter(w, flate.DefaultCompression)
}

// A header that under-states the part's size does not get it past the cap:
// the bound is on the bytes inflated, not on what the header claims.
func TestOpenRejectsPartThatInflatesPastItsHeader(t *testing.T) {
	pkg := buildPackage(t, minimalParts(zipPart{fill: ' ', n: MaxPartBytes + 1, declare: 1024}))

	_, err := Open(pkg)
	if err == nil {
		t.Fatal("Open accepted a part whose header declares 1 KiB but inflates past MaxPartBytes")
	}
	t.Logf("refused with: %v", err)
}

// Parts that each fit under the part limit but together inflate past the
// total limit are refused: a package of many just-under-the-cap parts is the
// same bomb split up. Two sheets point at two 1 MiB parts; the total cap is
// 1.5 MiB.
func TestOpenRejectsPackageOverTotalLimit(t *testing.T) {
	parts := minimalParts(zipPart{fill: ' ', n: 1 << 20})
	parts[0].body = `<workbook><sheets><sheet name="A" sheetId="1" r:id="rId1" xmlns:r="r"/><sheet name="B" sheetId="2" r:id="rId2" xmlns:r="r"/></sheets></workbook>`
	parts[1].body = `<Relationships>` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>` +
		`</Relationships>`
	parts = append(parts, zipPart{name: "xl/worksheets/sheet2.xml", fill: ' ', n: 1 << 20})
	pkg := buildPackage(t, parts)

	lim := testLimits()
	lim.partBytes = 1 << 20
	lim.totalBytes = 3 << 19
	_, err := openWithLimits(pkg, lim)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("openWithLimits(2 x 1 MiB, total cap 1.5 MiB) err = %v, want ErrLimitExceeded", err)
	}
}

// testLimits is defaultLimits, for a test to lower one bound of.
func testLimits() limits { return defaultLimits }

// A package with more zip entries than the part-count limit is refused before
// any part is read: the central directory alone can be a memory cost.
func TestOpenRejectsTooManyParts(t *testing.T) {
	parts := minimalParts(zipPart{body: `<worksheet/>`})
	for i := 0; i < 8; i++ {
		parts = append(parts, zipPart{name: "xl/media/pad" + string(rune('a'+i)), body: "x"})
	}
	pkg := buildPackage(t, parts)

	lim := testLimits()
	lim.parts = 10
	_, err := openWithLimits(pkg, lim)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("openWithLimits(11 parts, cap 10) err = %v, want ErrLimitExceeded", err)
	}

	lim.parts = len(parts)
	if _, err := openWithLimits(pkg, lim); err != nil {
		t.Fatalf("openWithLimits(%d parts, cap %d) = %v, want the package accepted at the cap", len(parts), lim.parts, err)
	}
}

// An optional part over the limit is an error, not a part treated as absent:
// silently dropping xl/sharedStrings.xml would import every shared-string cell
// as a wrong value instead of refusing the package.
func TestOpenRejectsOversizedOptionalPart(t *testing.T) {
	for _, name := range []string{"xl/sharedStrings.xml", "docProps/core.xml"} {
		t.Run(name, func(t *testing.T) {
			parts := append(minimalParts(zipPart{body: `<worksheet/>`}),
				zipPart{name: name, fill: ' ', n: 2 << 10})
			pkg := buildPackage(t, parts)

			lim := testLimits()
			lim.partBytes = 1 << 10
			_, err := openWithLimits(pkg, lim)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("openWithLimits(oversized %s) err = %v, want ErrLimitExceeded", name, err)
			}
		})
	}
}
