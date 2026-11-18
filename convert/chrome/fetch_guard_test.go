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

package chrome

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/convert"
)

// These are eden-press#42: a server-side export of an untrusted document must
// not make the export host fetch the URLs the document embeds -- internal
// services, cloud metadata. They drive NewTab + LoadHTML, the load path both
// convert/pdf.ToPDF and convert/png.ToImages take, against a local probe
// server and count what it receives.
//
// Chrome-gated: without a discoverable Chrome they SKIP, and a skip is not a
// pass. CI's export job runs them against the pinned headless-shell.

// onePixelPNG answers a request that does arrive with something paintable.
var onePixelPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")

// probe stands in for an internal service; hits counts every request.
type probe struct {
	srv  *httptest.Server
	hits atomic.Int64
}

func newProbe(t *testing.T) *probe {
	t.Helper()
	p := &probe{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(onePixelPNG)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// probeDocument references p three ways an untrusted document can: an image,
// a CSS background and a stylesheet link.
func (p *probe) document() string {
	u := p.srv.URL + "/latest/meta-data/"
	return `<!doctype html><html><head><link rel="stylesheet" href="` + u + `style.css">` +
		`<style>body{background:url(` + u + `bg.png)}</style></head>` +
		`<body><h1>untrusted</h1><img src="` + u + `img.png"></body></html>`
}

// guardedSession starts a Session with Chrome's own Local Network Access check
// off (see testAllocOptions), so the interception is the only thing between
// the page and the probe.
func guardedSession(t *testing.T, opts convert.Options) *Session {
	t.Helper()
	if _, _, err := Discover(DiscoverOptions{}); errors.Is(err, ErrChromeNotFound) {
		t.Skip("SKIPPED, NOT PASSED: no Chrome discovered (set CHROME_PATH)")
	}
	prev := testAllocOptions
	testAllocOptions = []chromedp.ExecAllocatorOption{
		chromedp.Flag("disable-features", "LocalNetworkAccessChecks,PrivateNetworkAccessRespectPreflightResults,BlockInsecurePrivateNetworkRequests"),
	}
	t.Cleanup(func() { testAllocOptions = prev })
	sess, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess
}

// loadAndSettle loads doc through LoadHTML and then gives the page time to
// issue and finish its subresource requests.
func loadAndSettle(t *testing.T, sess *Session, doc string) {
	t.Helper()
	tab, cancel := sess.NewTab()
	defer cancel()
	ctx, stop := context.WithTimeout(tab, 30*time.Second)
	defer stop()
	if err := LoadHTML(ctx, doc); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.Sleep(1500*time.Millisecond)); err != nil {
		t.Fatalf("settle: %v", err)
	}
}

func TestExportLoadMakesNoRequestsForEmbeddedURLs(t *testing.T) {
	p := newProbe(t)
	sess := guardedSession(t, convert.Options{})

	loadAndSettle(t, sess, p.document())

	if n := p.hits.Load(); n != 0 {
		t.Fatalf("loading an untrusted document made %d request(s) to %s; want 0", n, p.srv.URL)
	}
}

// The differential control for the test above: the same browser, the same
// document, with AllowRequest letting the probe's image through. The probe
// now sees that request -- so the zero above is the interception refusing,
// not a page that could not have reached the probe anyway -- and still sees
// nothing AllowRequest refused.
func TestAllowRequestLetsOnlyAllowedURLsThrough(t *testing.T) {
	p := newProbe(t)
	var asked atomic.Int64
	sess := guardedSession(t, convert.Options{AllowRequest: func(u string) bool {
		asked.Add(1)
		return u == p.srv.URL+"/latest/meta-data/img.png"
	}})

	loadAndSettle(t, sess, p.document())

	if n := p.hits.Load(); n != 1 {
		t.Fatalf("probe saw %d request(s) with only img.png allowed; want exactly 1", n)
	}
	if n := asked.Load(); n < 3 {
		t.Errorf("AllowRequest was consulted %d time(s); want one per request (3)", n)
	}
}

// eden-press#42: Chrome's sandbox is on unless the caller says the container
// provides the isolation. --no-sandbox was unconditional, so every host that
// rendered an untrusted document did it with the renderer unsandboxed.
func TestNewKeepsTheSandboxByDefault(t *testing.T) {
	t.Setenv(NoSandboxEnv, "")
	fake, argvPath := fakeRecordingBrowser(t)

	runNewAgainstFake(t, convert.Options{BrowserPath: fake, StartTimeout: fakeBrowserLifetime})

	if argv := recordedArgv(t, argvPath); argvHasToken(argv, "--no-sandbox") {
		t.Errorf("browser launched with --no-sandbox by default (argv: %v)", argv)
	}
}

// A container that provides the isolation itself can still launch Chrome
// unsandboxed, by option or by environment.
func TestNoSandboxIsAnExplicitOptOut(t *testing.T) {
	cases := []struct {
		name string
		opt  bool
		env  string
		want bool
	}{
		{"option", true, "", true},
		{"environment", false, "1", true},
		{"environment not 1", false, "true", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(NoSandboxEnv, tc.env)
			fake, argvPath := fakeRecordingBrowser(t)

			runNewAgainstFake(t, convert.Options{BrowserPath: fake, StartTimeout: fakeBrowserLifetime, NoSandbox: tc.opt})

			if got := argvHasToken(recordedArgv(t, argvPath), "--no-sandbox"); got != tc.want {
				t.Errorf("--no-sandbox present = %v, want %v", got, tc.want)
			}
		})
	}
}

// Fetch interception covers the tab it is enabled on, and not a WebSocket or a
// popup's new target. A document whose script opens either must still reach
// nothing: export tabs run no page script (eden-press#42 review).
func TestExportLoadScriptCannotReachThroughPopupOrWebSocket(t *testing.T) {
	cases := map[string]func(base string) string{
		"window.open": func(base string) string {
			return `<script>window.open("` + base + `/popup")</script>`
		},
		"WebSocket": func(base string) string {
			return `<script>new WebSocket("` + strings.Replace(base, "http://", "ws://", 1) + `/ws")</script>`
		},
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProbe(t)
			sess := guardedSession(t, convert.Options{})

			loadAndSettle(t, sess, `<!doctype html><html><body><h1>untrusted</h1>`+script(p.srv.URL)+`</body></html>`)

			if n := p.hits.Load(); n != 0 {
				t.Fatalf("a page script reached the probe %d time(s) via %s; want 0", n, name)
			}
		})
	}
}
