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

package profile

import (
	"testing"

	"go.aocyber.ai/eden-press/chase/theme"
)

// sizedProfile is a fakeProfile with a two-entry size table and a Default that
// is deliberately not in ByName, so a lookup that wrongly returned a table
// entry could not be mistaken for the default.
func sizedProfile() fakeProfile {
	return fakeProfile{id: "sized", sizes: SizeTable{
		ByName: map[string]theme.Size{
			"big":   {Name: "big", WidthPx: 1000, HeightPx: 2000},
			"small": {Name: "small", WidthPx: 10, HeightPx: 20},
		},
		Default: theme.Size{Name: "dflt", WidthPx: 500, HeightPx: 600},
	}}
}

// TestResolveSize pins the ONE size resolver the CSS packer and both exporters
// share: the `size` directive looked up in the profile's own table, the table's
// Default for no directive or an unknown name.
func TestResolveSize(t *testing.T) {
	p := sizedProfile()
	for _, tc := range []struct {
		name       string
		directives map[string]string
		want       theme.Size
	}{
		{"nil directives", nil, p.sizes.Default},
		{"empty directives", map[string]string{}, p.sizes.Default},
		{"no size key", map[string]string{"theme": "gaia"}, p.sizes.Default},
		{"empty size value", map[string]string{"size": ""}, p.sizes.Default},
		{"known name", map[string]string{"size": "big"}, p.sizes.ByName["big"]},
		{"another known name", map[string]string{"size": "small"}, p.sizes.ByName["small"]},
		{"unknown name falls back", map[string]string{"size": "nonsense"}, p.sizes.Default},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveSize(p, tc.directives); got != tc.want {
				t.Errorf("ResolveSize = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestUsesInlineSVG: a profile lays its units out inside an inline-SVG layer
// exactly when its two container chains differ.
func TestUsesInlineSVG(t *testing.T) {
	svg := fakeProfile{id: "svg", container: func(inline bool) string {
		if inline {
			return "div.x > svg > foreignObject"
		}
		return "div.x"
	}}
	flat := fakeProfile{id: "flat", container: func(bool) string { return "div.x" }}

	if !UsesInlineSVG(svg) {
		t.Error("UsesInlineSVG(profile with distinct SVG chain) = false, want true")
	}
	if UsesInlineSVG(flat) {
		t.Error("UsesInlineSVG(profile with one chain) = true, want false")
	}
}
