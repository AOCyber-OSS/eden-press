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

package convert

import (
	"io"
	"time"
)

// ImageFormat selects the raster image format a screenshot-based exporter
// (convert/png, added by a later TRD) produces. The zero value is PNG.
type ImageFormat int

const (
	// PNG is the lossless raster format and the zero value of ImageFormat.
	PNG ImageFormat = iota
	// JPEG is the lossy raster format, selectable when smaller output size
	// matters more than pixel-perfect fidelity.
	JPEG
)

// String returns the lower-case name of the format ("png" or "jpeg"). An
// unrecognized value (outside the two declared constants) returns "unknown"
// rather than panicking.
func (f ImageFormat) String() string {
	switch f {
	case PNG:
		return "png"
	case JPEG:
		return "jpeg"
	default:
		return "unknown"
	}
}

// Options is the shared cross-exporter option surface both convert/pdf and
// convert/png (added by later TRDs) consume. It is deliberately minimal --
// mirroring the frozen-surface discipline of press.Options -- and grows a
// field only when a named consumer needs it, never speculatively.
type Options struct {
	// BrowserPath, when non-empty, is an explicit Chrome/Chromium executable
	// override -- the highest-precedence tier of convert/chrome.Discover's
	// fallback chain. Leave empty to let Discover fall through to
	// CHROME_PATH, then auto-detection, then the documented pinned-download
	// tier.
	BrowserPath string

	// StartTimeout bounds how long browser ALLOCATION may take -- the launch
	// plus the DevTools handshake -- before New gives up and returns an error.
	// Zero means DefaultStartTimeout.
	//
	// This exists because there was previously no bound at all: allocation ran
	// against context.Background(), so a browser that never became usable left
	// the caller blocked forever with nothing logged. A hang is unobservable by
	// construction -- no error, no exit code -- and this one took down an
	// export sidecar in production, where the readiness endpoint that called it
	// simply stopped answering.
	//
	// It covers two different failures. A browser can fail to complete its
	// DevTools handshake at all, or it can complete the handshake and then
	// stall before its first tab is usable. The production incident was the
	// second kind: the websocket connection was established, and Chrome's
	// crash-looping GPU process then starved the renderer so it never answered
	// Runtime.enable. Set BrowserLog to tell the two apart.
	//
	// It bounds ONLY startup. The browser's own lifetime stays tied to an
	// unbounded context, because a deadline there would kill a healthy
	// long-lived Session mid-render.
	StartTimeout time.Duration

	// BrowserLog, when non-nil, receives Chrome's combined stdout+stderr for
	// the browser's entire lifetime. When nil, that output is discarded -- but
	// the pipe is still created and still drained.
	//
	// That distinction is not cosmetic, and it is the difference between
	// diagnosing a failed launch and guessing at one. chromedp reads Chrome's
	// output only until it finds the "DevTools listening on" line; from there
	// it branches on whether a writer was configured, and with NO writer it
	// CLOSES the output pipe outright. Every line Chrome emits after startup is
	// then destroyed at the source.
	//
	// Two failed production deploys produced zero browser diagnostics for
	// exactly this reason: Chrome's GPU process was crash-looping and saying so
	// continuously, but the crash-loop begins after the websocket URL is parsed
	// -- that is, after chromedp had already closed the pipe. Setting this
	// writer surfaced the root cause on the first run afterwards.
	//
	// Leaving it nil is safe and is the default; convert/chrome substitutes
	// io.Discard rather than passing nil through, so the pipe is never closed
	// out from under a browser that still has something to say.
	BrowserLog io.Writer

	// AllowRequest decides which network requests a page rendered for export
	// may make. Every request a tab makes is intercepted, and it goes ahead
	// only when AllowRequest returns true for its URL. nil -- the default --
	// refuses them all (eden-press#42).
	//
	// The documents an export renders may be untrusted, and Chrome fetches
	// whatever they reference from the export host's network position:
	// internal services, cloud metadata (169.254.169.254), anything the host
	// can reach and the document's author cannot. The export path needs no
	// network of its own (the document is injected, fonts are data: URIs), so
	// a deck that references a remote image or a theme that @imports a web
	// font (gaia) renders without it unless the caller allows it here. Allow
	// only origins you trust, and compare parsed URLs, not prefixes.
	AllowRequest func(url string) bool

	// NoSandbox launches Chrome without its sandbox. Leave it false wherever
	// the sandbox can run: it contains a renderer exploit, and the renderer
	// parses documents that may be untrusted.
	//
	// Set it only in a container that provides the isolation instead -- the
	// case where Chrome cannot launch sandboxed at all, typically root or no
	// user-namespace support. The environment variable
	// EDEN_PRESS_CHROME_NO_SANDBOX=1 has the same effect, so a deployment can
	// declare it without a code change.
	NoSandbox bool
}
