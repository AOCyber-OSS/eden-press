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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"go.aocyber.ai/eden-press/cmd/eden-press/reload"
	"go.aocyber.ai/eden-press/press"
)

// newServeCmd registers the "serve" subcommand.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve [flags] [dir]",
		Short: "Serve a rendered Markdown deck with live-reload",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, args)
		},
	}
	registerServeFlags(cmd)
	return cmd
}

// runServe is CLI-03's capstone: `eden-press serve [dir]` serves static
// files rooted at dir ("." if omitted), converts markdown-extension
// requests to HTML on demand through 04-03's render pipeline, and
// live-reloads any open browser tab via the reused 04-06 SSE Hub -- all
// guarded by safeJoin (research Pitfall 7) before any file I/O. It is pure
// composition of serveMux (this file), reload.NewHub (04-06), and
// buildServeOptions/press.Render/assembleHTML (04-03): no new engine
// surface is added here.
func runServe(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("serve: resolve %q: %w", root, err)
	}

	hub := reload.NewHub()
	mux := serveMux(absRoot, hub, cmd)

	addr := resolveServeAddr()
	fmt.Fprintf(cmd.OutOrStdout(), "eden-press serving %s on http://%s\n", absRoot, addr)

	return http.ListenAndServe(addr, mux)
}

// serveMux builds the routing table runServe listens on: "/__reload"
// mounts the SSE Hub directly as its own http.Handler (reload/server.go's
// own doc comment: serve mounts ServeHTTP on its OWN existing mux rather
// than calling hub.Start, unlike watch -- which has no other HTTP server to
// mount onto), and "/" serves static files + converts markdown-extension
// requests on demand, guarded by safeJoin against directory traversal
// (Pitfall 7) before any file I/O. Factored out (rather than inlined in
// runServe) so tests can drive it against an httptest.NewServer without a
// fixed-port bind.
func serveMux(absRoot string, hub *reload.Hub, cmd *cobra.Command) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/__reload", hub)
	mux.Handle("/", serveFileHandler(absRoot, cmd, hub))
	return mux
}

// serveFileHandler is the "/" route's handler, factored out on its own so
// a test can drive it directly if needed to prove the traversal guard's
// containment logic in isolation from http.ServeMux's own path handling.
// Every request path is run through safeJoin BEFORE any file I/O; only a
// contained result ever reaches os.ReadFile/http.ServeFile.
func serveFileHandler(absRoot string, cmd *cobra.Command, hub *reload.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := safeJoin(absRoot, r.URL.Path)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if isMarkdown(p) {
			serveMarkdown(w, r, p, cmd, hub)
			return
		}

		http.ServeFile(w, r, p) // static files (images, css, etc.) -- served verbatim, no conversion
	}
}

// serveMarkdown converts p (a markdown file already confirmed to be
// contained under absRoot by safeJoin) on EVERY request -- no cache in v1,
// matching Marp CLI's own serve behavior -- through 04-03's identical
// buildServeOptions -> press.Render chain. The client picks the body shape:
//
//   - ?format=json or Accept: application/json → lowercase JSON envelope
//     (Flutter / eden_docs clients)
//   - ?format=html, Accept: text/html, or default → standalone HTML
//     (browsers; docs get AssembleDocsHTML when the model is article/paged)
//
// No ?pdf/?png handling (convert/'s job).
func serveMarkdown(w http.ResponseWriter, r *http.Request, p string, cmd *cobra.Command, hub *reload.Hub) {
	md, err := os.ReadFile(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	opts, err := buildServeOptions(cmd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out, err := press.Render(string(md), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch serveResponseFormat(r) {
	case "json":
		env := jsonEnvelope{
			HTML:     out.HTML,
			CSS:      out.CSS,
			Model:    out.Model,
			Comments: out.Comments,
			Meta:     out.Meta,
		}
		b, err := json.MarshalIndent(env, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(append(b, '\n'))
	default:
		body := assembleServeHTML(out, hub)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, body)
	}
}

// serveResponseFormat picks json vs html from ?format= then Accept.
func serveResponseFormat(r *http.Request) string {
	switch strings.ToLower(r.URL.Query().Get("format")) {
	case "json":
		return "json"
	case "html":
		return "html"
	}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") &&
		!strings.Contains(accept, "text/html") {
		return "json"
	}
	return "html"
}

// assembleServeHTML prefers flowing docs HTML when the rendered model looks
// like an article/paged document; otherwise keeps the Marp slide wrapper
// with live-reload for decks. cmd cannot import chase/model (check-cli-
// imports), so docs are detected via Meta.Directives / HTML class, then
// re-exported through press.ExportHTMLJSON.
func assembleServeHTML(out press.Output, hub *reload.Hub) string {
	profile := ""
	if out.Meta.Directives != nil {
		profile = strings.ToLower(out.Meta.Directives["profile"])
	}
	if profile == "article" || profile == "paged" ||
		strings.Contains(out.HTML, "edenpress-paged") {
		if raw, err := json.Marshal(out.Model); err == nil {
			if html, err := press.ExportHTMLJSON(raw); err == nil {
				return string(html)
			}
		}
	}
	return assembleHTML(out, htmlDocOptions{
		InjectScripts: []string{reload.ClientJS("/__reload")},
	})
}

// buildServeOptions is serve's own named call-site for 04-03's
// buildOptions -- it does NOT re-implement or diverge from buildOptions'
// flag/config-file/env resolution in any way (anti-pattern: don't
// re-sanitize or re-implement the assembler); it exists purely so serve.go
// has its own documented entry point per this TRD's action spec.
func buildServeOptions(cmd *cobra.Command) (press.Options, error) {
	return buildOptions(cmd)
}

// isMarkdown reports whether path's extension is a markdown extension
// (".md" or ".markdown", case-insensitive) -- the convert-on-request
// trigger. Anything else is served as a static file verbatim.
func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}

// safeJoin is the directory-traversal guard (research Pitfall 7): urlPath
// (r.URL.Path -- already percent-decoded by net/http, so "%2f" and "/"
// are indistinguishable by the time this function sees them) is
// normalized to a SINGLE rooted path via filepath.Clean("/"+urlPath)
// before being Join'd onto root. A rooted Clean documents that it always
// resolves a leading ".." down to "/", never past it -- so the joined
// result can never itself point outside root, by construction. The
// subsequent filepath.Rel containment check is kept anyway as an
// explicit, auditable second guard (Marp CLI keeps the analogous check as
// defense-in-depth even atop its own framework's own protections) rather
// than trusting the normalization step alone to remain sufficient forever.
func safeJoin(root, urlPath string) (string, bool) {
	clean := filepath.Clean("/" + urlPath)
	joined := filepath.Join(root, clean)

	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", false
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}

	return abs, true
}

// resolveServeAddr resolves the listen address from cfg: --host ("" ->
// "127.0.0.1", loopback by default) and --port (0 -> the product default
// 8321, NEVER 8080). Factored out so a test can assert the resolved
// default/override WITHOUT an actual bind (test-list case 6). --port/
// --host/EDEN_PRESS_PORT/EDEN_PRESS_HOST overrides all flow through cfg
// automatically via 04-04's existing flags>env>file precedence pipeline --
// no serve-specific config wiring is needed here.
func resolveServeAddr() string {
	host := cfg.String("host")
	if host == "" {
		host = "127.0.0.1"
	}

	port := cfg.Int("port")
	if port == 0 {
		port = 8321
	}

	return net.JoinHostPort(host, strconv.Itoa(port))
}
