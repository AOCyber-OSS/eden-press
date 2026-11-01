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

package main

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/cmd/eden-press/reload"
)

const testServeDeck = "# Hello Serve\n\nServed content.\n"

// newTestServeCmd builds a bare cobra.Command carrying the persistent +
// serve-mode flag surface (--port/--host), mirroring what newRootCmd() +
// newServeCmd() register -- enough to drive buildServeOptions/
// resolveServeAddr through cfg without constructing the whole subcommand
// tree (same pattern as flags_test.go's newTestConvertCmd).
func newTestServeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	registerPersistentFlags(cmd)
	registerServeFlags(cmd)
	return cmd
}

// prepServeCmd resets the package cfg, builds a serve-mode test command,
// applies args through the real cobra/koanf flag pipeline (ParseFlags ->
// applyConfig), and returns the ready-to-use command -- the setup every
// test below shares.
func prepServeCmd(t *testing.T, args []string) *cobra.Command {
	t.Helper()
	resetCfg()
	cmd := newTestServeCmd()
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := applyConfig(cmd); err != nil {
		t.Fatalf("applyConfig: %v", err)
	}
	return cmd
}

// TestServeConvertsMarkdownOnRequest is test-list case 1: GET /deck.md (a
// temp markdown file in root) returns text/html containing the rendered
// deck content AND the injected reload snippet -- proving convert-on-
// request reuses 04-03's press.Render/assembleHTML pipeline and 04-06's
// reload client rather than reinventing either.
func TestServeConvertsMarkdownOnRequest(t *testing.T) {
	cmd := prepServeCmd(t, nil)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(testServeDeck), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	hub := reload.NewHub()
	srv := httptest.NewServer(serveMux(dir, hub, cmd))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/deck.md")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html prefix", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "Hello Serve") {
		t.Errorf("body missing rendered deck content: %q", got)
	}
	if !strings.Contains(got, "EventSource") {
		t.Errorf("body missing injected reload snippet: %q", got)
	}
	if !strings.Contains(got, "/__reload") {
		t.Errorf("body missing reload endpoint URL: %q", got)
	}
}

// TestServeRejectsDirectoryTraversal is test-list case 2 (research Pitfall
// 7): a request path attempting to escape root -- both a plain "../"
// sequence and a URL-encoded ("%2f") variant -- never serves a sentinel
// file placed OUTSIDE root; the response is 400, 403, or 404, never 200
// with the sentinel's content.
//
// The two cases are rejected by different layers, both correct: the plain
// "../" request is neutralized by http.ServeMux's own escaped-path
// cleaning (a real "/" in the escaped path) before it even reaches our
// handler, landing on a contained, nonexistent path (404). The "%2f"
// variant's ESCAPED path has no literal "/" for ServeMux to clean (Go
// 1.22+'s ServeMux cleans r.URL.EscapedPath(), and "%2f" is not a literal
// slash), so it reaches serveFileHandler directly with a decoded
// r.URL.Path still containing ".." -- safeJoin still correctly contains
// it under root, but http.ServeFile's OWN containsDotDot(r.URL.Path) guard
// then independently rejects the ORIGINAL request path with 400, a third,
// redundant layer on top of safeJoin. Either outcome proves the sentinel
// is never reached.
func TestServeRejectsDirectoryTraversal(t *testing.T) {
	cmd := prepServeCmd(t, nil)

	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	const sentinel = "SECRET-OUTSIDE-ROOT"
	if err := os.WriteFile(filepath.Join(parent, "sentinel.txt"), []byte(sentinel), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	hub := reload.NewHub()
	srv := httptest.NewServer(serveMux(root, hub, cmd))
	t.Cleanup(srv.Close)

	for _, target := range []string{"/../sentinel.txt", "/..%2fsentinel.txt"} {
		t.Run(target, func(t *testing.T) {
			resp, err := http.Get(srv.URL + target)
			if err != nil {
				t.Fatalf("http.Get(%q): %v", target, err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("io.ReadAll: %v", err)
			}

			if strings.Contains(string(body), sentinel) {
				t.Fatalf("traversal guard failed: sentinel content leaked for %q: %q", target, string(body))
			}
			switch resp.StatusCode {
			case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound:
				// all three are safe rejections -- see the layered-defense comment above
			default:
				t.Errorf("status = %d for %q, want 400, 403, or 404", resp.StatusCode, target)
			}
		})
	}
}

// TestSafeJoinContainsResultUnderRoot proves safeJoin's pure containment
// logic directly: every crafted urlPath (plain ".." sequences of any
// depth, an absolute-looking path) resolves to a path that remains at or
// under root -- it can never actually escape it -- matching research
// Pitfall 7's "resolve root once, verify containment before any file I/O"
// requirement. A false ok is also an acceptable (safe) outcome.
func TestSafeJoinContainsResultUnderRoot(t *testing.T) {
	root := t.TempDir()

	cases := []string{
		"/deck.md",
		"/../etc/passwd",
		"/../../../../../../etc/passwd",
		"/../../sentinel.txt",
		"/etc/passwd", // an absolute-looking urlPath must never be treated as a real filesystem absolute path
		"",
	}

	for _, urlPath := range cases {
		t.Run(urlPath, func(t *testing.T) {
			abs, ok := safeJoin(root, urlPath)
			if !ok {
				return // rejected outright is also a safe outcome
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				t.Fatalf("filepath.Rel(%q, %q): %v", root, abs, err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				t.Errorf("safeJoin(%q, %q) = %q, escapes root (rel = %q)", root, urlPath, abs, rel)
			}
		})
	}
}

// TestServeStaticFile is test-list case 3: GET /img.png (a temp file in
// root) returns the file's exact bytes, unconverted -- proving non-
// markdown requests fall through to http.ServeFile rather than the
// convert-on-request path.
func TestServeStaticFile(t *testing.T) {
	cmd := prepServeCmd(t, nil)

	dir := t.TempDir()
	want := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x01, 0x02, 0x03}
	if err := os.WriteFile(filepath.Join(dir, "img.png"), want, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	hub := reload.NewHub()
	srv := httptest.NewServer(serveMux(dir, hub, cmd))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/img.png")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

// TestServeReloadEndpoint is test-list case 4: GET /__reload returns
// text/event-stream and receives a broadcast reload frame -- proving serve
// mounts the SAME reload.Hub (04-06) directly on its own mux, rather than
// building a second live-reload mechanism.
func TestServeReloadEndpoint(t *testing.T) {
	cmd := prepServeCmd(t, nil)

	dir := t.TempDir()
	hub := reload.NewHub()
	srv := httptest.NewServer(serveMux(dir, hub, cmd))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/__reload")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/event-stream")
	}

	hub.Broadcast("reload")

	frameCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		var b strings.Builder
		for {
			line, err := reader.ReadString('\n')
			b.WriteString(line)
			if err != nil {
				frameCh <- b.String()
				return
			}
			if strings.Contains(b.String(), "\n\n") {
				frameCh <- b.String()
				return
			}
		}
	}()

	select {
	case frame := <-frameCh:
		if !strings.Contains(frame, "event: reload") {
			t.Errorf("frame = %q, want it to contain %q", frame, "event: reload")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the broadcast reload frame")
	}
}

// TestServeIgnoresFormatSwitchQuery is test-list case 5 (research Pattern 5
// scope correction): GET /deck.md?pdf returns the SAME HTML as a plain GET
// /deck.md -- v1 serve is HTML-only; no ?pdf/?png query-based
// format-switching (that crosses into convert/'s territory, Objectives
// 5/6), so the query string is never inspected.
func TestServeIgnoresFormatSwitchQuery(t *testing.T) {
	cmd := prepServeCmd(t, nil)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(testServeDeck), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	hub := reload.NewHub()
	srv := httptest.NewServer(serveMux(dir, hub, cmd))
	t.Cleanup(srv.Close)

	plain, err := http.Get(srv.URL + "/deck.md")
	if err != nil {
		t.Fatalf("http.Get(plain): %v", err)
	}
	defer plain.Body.Close()
	plainBody, err := io.ReadAll(plain.Body)
	if err != nil {
		t.Fatalf("io.ReadAll(plain): %v", err)
	}

	withQuery, err := http.Get(srv.URL + "/deck.md?pdf")
	if err != nil {
		t.Fatalf("http.Get(withQuery): %v", err)
	}
	defer withQuery.Body.Close()
	queryBody, err := io.ReadAll(withQuery.Body)
	if err != nil {
		t.Fatalf("io.ReadAll(withQuery): %v", err)
	}

	if withQuery.Header.Get("Content-Type") != plain.Header.Get("Content-Type") {
		t.Errorf("Content-Type differs: plain=%q query=%q", plain.Header.Get("Content-Type"), withQuery.Header.Get("Content-Type"))
	}
	if string(queryBody) != string(plainBody) {
		t.Errorf("body with ?pdf query differs from plain body:\nplain=%q\nquery=%q", plainBody, queryBody)
	}
}

// TestServeJSONFormat: Flutter / eden_docs clients ask for JSON via
// ?format=json (or Accept: application/json) and get the lowercase envelope
// instead of HTML.
func TestServeJSONFormat(t *testing.T) {
	cmd := prepServeCmd(t, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "deck.md")
	if err := os.WriteFile(path, []byte(testServeDeck), 0o644); err != nil {
		t.Fatal(err)
	}
	hub := reload.NewHub()
	srv := httptest.NewServer(serveMux(dir, hub, cmd))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/deck.md?format=json")
	if err != nil {
		t.Fatalf("GET json: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	s := string(body)
	if !strings.Contains(s, `"model"`) || !strings.Contains(s, `"html"`) {
		t.Fatalf("json envelope missing keys: %s", s)
	}
	if strings.Contains(s, "<!doctype html>") {
		t.Fatalf("json response looks like HTML: %s", s)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/deck.md", nil)
	req.Header.Set("Accept", "application/json")
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET Accept json: %v", err)
	}
	defer res2.Body.Close()
	if ct := res2.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Accept negotiation Content-Type = %q", ct)
	}
}

// TestResolveServeAddrDefaultsToNon8080Port is test-list case 6: with no
// --port set, resolveServeAddr targets the product default 8321 (NEVER
// 8080) on loopback; --port overrides it. Asserts the resolved ADDRESS
// string only -- no real bind (error_recovery: reserve the fixed port for
// the real serve run; ephemeral httptest binds cover the other cases).
func TestResolveServeAddrDefaultsToNon8080Port(t *testing.T) {
	prepServeCmd(t, nil)

	if got, want := resolveServeAddr(), "127.0.0.1:8321"; got != want {
		t.Errorf("resolveServeAddr() default = %q, want %q (loopback + product default port)", got, want)
	}

	prepServeCmd(t, []string{"--port", "8091"})

	if got, want := resolveServeAddr(), "127.0.0.1:8091"; got != want {
		t.Errorf("resolveServeAddr() with --port override = %q, want %q", got, want)
	}
}
