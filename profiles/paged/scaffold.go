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

package paged

// ScaffoldCSS is the base stylesheet chase/theme prepends before a packed
// theme's own rules for paged output — the paged analogue of
// profiles/slides' ScaffoldCSS.
//
// Three things it must get right, none of which apply to a slide deck:
//
//  1. A page is PHYSICAL paper. Each section starts a new page, has the page's
//     width and real margins, and is at least one page tall; a section longer
//     than a page flows onto as many pages as it needs, and nothing clips it
//     (D16). press appends the size rules after the theme (the scaffold comes
//     before it and would lose). The screen presentation stacks sections
//     vertically with a gap and a shadow, so an on-screen preview reads as a
//     document rather than one endless column.
//
//  2. Page NUMBERS come from a CSS counter, not from an attribute. Marpit
//     stamps data-marpit-pagination onto each slide at parse time; a paged
//     document instead increments a counter per section, so numbering survives
//     sections being added or reordered without re-running the directive pass.
//
//  3. @page carries the vertical page margin for print/PDF, so a section that
//     breaks across pages has a top and a bottom margin on EVERY page, not
//     only where it starts and ends. The paper size itself is stated by the
//     exporter (convert/pdf), which appends a size-only @page rule.
//
// The `--edenpress-*` custom properties are the documented override surface: a
// theme sets them rather than re-declaring the structural rules.
const ScaffoldCSS = `
:root {
  --edenpress-page-margin: 20mm;
  --edenpress-page-gap: 12px;
  --edenpress-header-text: "";
  --edenpress-rule-color: #d0d0d0;
}

/* Each section starts a page and flows: press appends its width, a
   content-driven height and the page height as an on-screen minimum, after
   the theme. Nothing here hides its overflow, so no content is ever clipped.
   The page-number counter is incremented per section and never explicitly
   reset -- an un-reset counter is implicitly created at the root scope, so
   numbering starts at 1 without needing a rule on the container (which the
   scoping pass would prepend the container chain to a second time). */
section {
  counter-increment: edenpress-page;
  position: relative;
  box-sizing: border-box;
  margin: 0 auto var(--edenpress-page-gap);
  padding: var(--edenpress-page-margin);
  background: #fff;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.18);
}

/* Running header and page number live inside the section's own padding, so
   they cannot collide with body content on screen however long it runs. */
section::before {
  content: var(--edenpress-header-text);
  position: absolute;
  top: calc(var(--edenpress-page-margin) / 2);
  left: var(--edenpress-page-margin);
  right: var(--edenpress-page-margin);
  font-size: 0.75em;
  color: #666;
  border-bottom: 1px solid var(--edenpress-rule-color);
  padding-bottom: 2px;
}

/* chase/theme's pagination pass rewrites this content value via the
   PaginationRule; the counter is the default. */
section::after {
  content: counter(edenpress-page);
  position: absolute;
  bottom: calc(var(--edenpress-page-margin) / 2);
  right: var(--edenpress-page-margin);
  font-size: 0.75em;
  color: #666;
}

/* Long-form typography: headings that do not strand themselves at the foot
   of a page, and paragraphs that do not leave single lines behind. */
:where(h1, h2, h3, h4, h5, h6) {
  break-after: avoid;
  page-break-after: avoid;
}

:where(p, li) {
  orphans: 2;
  widows: 2;
}

:where(table) {
  border-collapse: collapse;
  width: 100%;
}

:where(th, td) {
  border: 1px solid var(--edenpress-rule-color);
  padding: 0.35em 0.5em;
  text-align: left;
}

/* Repeat table headers across page breaks. */
:where(thead) {
  display: table-header-group;
}

:where(blockquote) {
  margin: 0.8em 0;
  padding-left: 1em;
  border-left: 3px solid var(--edenpress-rule-color);
  color: #444;
}

/* @page carries the VERTICAL page margin for print/PDF, so a section that
   breaks across pages keeps a top and a bottom margin on every page,
   continuation pages included (press zeroes the section's own top and bottom
   padding in print). The horizontal margin is the section's own left and right
   padding. The value is a literal because var() does not resolve inside
   @page; it equals --edenpress-page-margin's default above, and a test pins
   the two together. convert/pdf states the paper size in a size-only @page
   rule, so this margin is the one that applies. */
@page {
  margin: 20mm 0;
}

/* Print: drop the screen chrome, start every section on a new page. The
   running header is positioned in the section's top padding, which print does
   not have, so it would sit on the first line of text: it is hidden. */
@media print {
  section {
    margin: 0;
    box-shadow: none;
    break-after: page;
    page-break-after: always;
  }

  section::before {
    display: none;
  }

  section:last-child {
    break-after: auto;
    page-break-after: auto;
  }
}
`
