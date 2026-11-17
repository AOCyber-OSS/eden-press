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
	"fmt"
	"io"
	"os"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/convert"
)

// Session is the one-browser-many-tabs pool primitive every convert/
// exporter (convert/pdf, convert/png) drives its rendering through. New
// allocates exactly ONE Chrome process; NewTab hands out additional tabs on
// that SAME browser -- never a fresh process per call.
type Session struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc

	// rootCtx is an internal, already-Run anchor tab context. It exists
	// solely so NewTab's chromedp.NewContext(rootCtx) calls inherit a
	// non-nil Browser at creation time -- see the package-level comment on
	// New for why this indirection is required.
	rootCtx    context.Context
	rootCancel context.CancelFunc

	userDataDir string

	// allowRequest is convert.Options.AllowRequest: nil refuses every
	// request a page makes (see guard).
	allowRequest func(url string) bool
}

// DefaultStartTimeout bounds browser allocation when Options.StartTimeout is
// zero. Generous on purpose: a cold Chrome in a CPU-throttled container can
// take well over ten seconds to complete its DevTools handshake AND bring its
// first tab up, and a bound that trips on a slow-but-healthy start would trade
// a rare hang for a common false failure. This is a backstop against never
// returning, not a latency SLO.
const DefaultStartTimeout = 90 * time.Second

// browserOutput resolves the caller's optional convert.Options.BrowserLog into
// the writer handed to chromedp.CombinedOutput. It returns w unchanged when the
// caller set one, and io.Discard when they did not.
//
// Never pass a caller's nil writer straight through to chromedp -- routing it
// through here is what keeps Chrome's output pipe alive. chromedp's
// allocate.go readOutput scans Chrome's output for the "DevTools listening on"
// line and then branches on the forward writer: with a nil writer it calls
// Close on the pipe, on the reasoning that the process's output is no longer
// needed. Everything Chrome emits after startup is destroyed at the source.
//
// io.Discard takes the other branch instead. allocate.go:253 then runs io.Copy
// on the allocator's WaitGroup for the browser's whole lifetime, so the pipe
// stays open and drained. Behaviour for callers who never set BrowserLog is
// unchanged -- the bytes go nowhere either way -- but a caller who DOES set it
// gets a live diagnostic rather than a pipe chromedp already closed.
//
// This is not hypothetical: it is why two failed production deploys produced no
// browser output at all while Chrome's GPU process crash-looped continuously.
// The crash messages were emitted after the websocket URL was parsed, which is
// precisely when the pipe had already been closed.
func browserOutput(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

// NoSandboxEnv names the environment variable that, set to "1", launches
// Chrome without its sandbox, as convert.Options.NoSandbox does.
const NoSandboxEnv = "EDEN_PRESS_CHROME_NO_SANDBOX"

// testAllocOptions are extra launch options the package's own tests add. It is
// nil outside tests. The request-interception tests need it: Chrome's Local
// Network Access checks already refuse loopback fetches from an about:blank
// page on current Chrome, which would make "the probe server saw no request"
// true with or without the interception under test. Those tests switch that
// check off here so a zero means the interception refused the request.
var testAllocOptions []chromedp.ExecAllocatorOption

// New builds ONE chromedp ExecAllocator/browser with the CI-hardening and
// determinism launch flags baked in as DEFAULTS (not opt-ins), resolving the
// executable via Discover.
//
// Implementation note (why a root anchor tab, not just the allocator ctx):
// chromedp.NewContext(parent) copies parent's *Browser field at CALL time --
// it does not lazily re-read it later. The allocator context returned by
// chromedp.NewExecAllocator is never itself Run (doing so directly is a
// chromedp error), so its attached Context.Browser field is permanently
// nil. If NewTab called chromedp.NewContext(s.allocCtx) directly on every
// invocation, each call would independently see a nil Browser and allocate
// its OWN Chrome process -- exactly the per-render-process anti-pattern this
// Session exists to avoid. Instead, New creates one internal tab context
// (rootCtx) and immediately Runs it (zero actions) so the browser is
// allocated eagerly and rootCtx's Context.Browser is populated once; every
// subsequent NewTab derives from that SAME already-populated rootCtx, so it
// correctly inherits the live Browser and only ever creates a new Target
// (tab) -- exactly chromedp's own documented multi-tab pattern
// (ExampleNewContext_manyTabs in the chromedp package itself).
func New(opts convert.Options) (*Session, error) {
	execPath, _, err := Discover(DiscoverOptions{BrowserPath: opts.BrowserPath})
	if err != nil {
		return nil, err
	}

	userDataDir, err := os.MkdirTemp("", "eden-press-chrome-*")
	if err != nil {
		return nil, fmt.Errorf("convert/chrome: creating unique user-data-dir: %w", err)
	}

	startTimeout := opts.StartTimeout
	if startTimeout <= 0 {
		startTimeout = DefaultStartTimeout
	}

	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		// Unique per-run profile dir -- avoids cross-run contention if
		// multiple Sessions exist concurrently (Pitfall 11).
		chromedp.UserDataDir(userDataDir),
		// Container /dev/shm is often too small; disabling shared-memory
		// usage avoids BUS_ADRERR crashes (Pitfall 11). Also already present
		// in chromedp.DefaultExecAllocatorOptions; set again explicitly so
		// the determinism recipe is self-documenting and order-independent.
		chromedp.Flag("disable-dev-shm-usage", true),
		// Determinism: fixed device-scale-factor so screenshots/PDFs render
		// pixel-identically across hosts with different display densities.
		chromedp.Flag("force-device-scale-factor", "1"),
		// Determinism: fixed locale, independent of the host's system locale.
		chromedp.Flag("lang", "en-US"),
		// Determinism: fixed timezone for the Chrome process's own clock,
		// independent of the host machine's TZ.
		chromedp.Env("TZ=UTC"),
		// Survival in a GPU-less container -- NOT determinism, and NOT
		// supplied by chromedp.DefaultExecAllocatorOptions (verified: the
		// flag is absent from that list, so nothing sets it unless we do).
		//
		// Without it Chrome's GPU/viz process crash-loops wherever no GPU
		// exists, which is every container this ships in. The page renderer
		// then never obtains an execution context and never answers
		// Runtime.enable -- the third command chromedp sends while attaching
		// to the first target -- and Browser.Execute's only escape is
		// ctx.Done(), so the Run below blocks. Reproduced live with a CDP
		// wire trace: same image, same flags, hung past 45s; adding this one
		// flag returned allocation in 808ms and rendered a full 5754-byte PNG.
		//
		// STANDING PROHIBITION: do NOT add --disable-software-rasterizer
		// alongside this. It is a common copy-paste pairing and it disables
		// SwiftShader, which is what actually rasterizes PDF/PNG output once
		// the GPU is off -- trading a fixed hang for silently broken output.
		// --disable-gpu ALONE was verified sufficient and verified to still
		// rasterize. TestNewDoesNotDisableSoftwareRasterizer guards this.
		//
		// Set as a bare Flag rather than via chromedp.DisableGPU on purpose:
		// that helper also sets --enable-unsafe-swiftshader, which was not
		// part of what was verified in the pod.
		chromedp.Flag("disable-gpu", true),
		// Keep Chrome's own output reachable. chromedp reads it only until it
		// finds the "DevTools listening on" line, then -- if no writer was
		// configured -- CLOSES the pipe, destroying everything Chrome says
		// afterwards. The GPU crash-loop above starts talking after that
		// point, which is exactly why two failed production deploys produced
		// zero browser diagnostics while Chrome complained continuously.
		//
		// browserOutput substitutes io.Discard when the caller set no writer,
		// so the pipe is kept open and drained either way; with a writer set,
		// allocate.go:253 runs io.Copy on the allocator's WaitGroup for the
		// browser's lifetime. Never pass opts.BrowserLog straight through --
		// a nil reaching chromedp restores the pipe-closing branch.
		chromedp.CombinedOutput(browserOutput(opts.BrowserLog)),
		// The wait for the "DevTools listening on" URL gets the SAME bound as
		// the whole allocation below. chromedp's ExecAllocator otherwise keeps
		// its own 20s websocket-URL timeout, which pre-empted StartTimeout: a
		// browser merely slow to start (a contended CI runner launching several
		// at once) failed at 20s with "websocket url timeout reached".
		// TestNewStartTimeoutGovernsTheDevToolsHandshake guards this.
		chromedp.WSURLReadTimeout(startTimeout),
	)
	// The sandbox stays ON unless the caller says the container provides the
	// isolation instead (eden-press#42). The renderer parses documents that
	// may be untrusted, and the sandbox is what contains a renderer exploit.
	// It is turned off only where it cannot work: CI/container execution
	// commonly runs as root, or with no user-namespace sandbox available, and
	// Chrome then refuses to launch with it. That is a deployment fact, so it
	// is set per deployment -- convert.Options.NoSandbox, or NoSandboxEnv=1 in
	// the container's environment -- and the container must then supply the
	// isolation itself (non-root user, no privileges, no route to anything the
	// export should not reach).
	if opts.NoSandbox || os.Getenv(NoSandboxEnv) == "1" {
		allocOpts = append(allocOpts, chromedp.NoSandbox)
	}
	allocOpts = append(allocOpts, testAllocOptions...)
	if execPath != "" {
		// Only pin an explicit executable when Discover resolved one, e.g.
		// tier 1 (BrowserPath) or tier 2 (CHROME_PATH). An empty execPath
		// (tier 3, "auto") deliberately omits ExecPath so chromedp's own
		// ExecAllocator performs its own auto-detection.
		allocOpts = append(allocOpts, chromedp.ExecPath(execPath))
	}

	// context.Background(), NOT a deadline: this context owns the BROWSER'S
	// LIFETIME, so a timeout here would kill a healthy long-lived Session
	// mid-render. Startup is bounded separately, below.
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)

	rootCtx, rootCancel := chromedp.NewContext(allocCtx)

	// Bound ALLOCATION -- the process launch plus the DevTools handshake.
	//
	// This Run previously had no bound of any kind, so a browser that never
	// became usable blocked the caller forever, with no error and nothing
	// logged. That is unobservable by construction, and it is exactly how an
	// export sidecar died in production: the pod ran, the port listened, and
	// its readiness endpoint simply never answered.
	//
	// Two distinct failures land here, and the bound must cover both. A
	// browser can fail to complete its DevTools handshake at all, or it can
	// complete the handshake and stall afterwards -- the incident was the
	// SECOND kind. The websocket URL was parsed and the connection
	// established; the GPU crash-loop then starved the renderer so it never
	// answered Runtime.enable, and this Run blocked on a browser that was, by
	// every earlier signal, up. --disable-gpu above addresses that cause; this
	// bound remains the backstop for whatever the next cause turns out to be.
	//
	// Done in a goroutine rather than context.WithTimeout(rootCtx, ...) on
	// purpose. rootCtx carries the chromedp *Context that Run populates and
	// that every later NewTab inherits; wrapping it in a deadline would make
	// the SUCCESS path's browser reachable through a context that expires,
	// which is the bug this fixes wearing a different hat.
	//
	// The goroutine can outlive this function on the timeout path -- that is
	// deliberate. It is parked on a Run that may never return, so waiting for
	// it would reintroduce the hang. Cancelling the contexts below is what
	// stops the browser; the goroutine then observes that and exits.
	// startTimeout is resolved above, before the allocator options.
	allocated := make(chan error, 1) // buffered: never block the sender
	go func() { allocated <- chromedp.Run(rootCtx) }()

	select {
	case err := <-allocated:
		if err != nil {
			rootCancel()
			allocCancel()
			_ = os.RemoveAll(userDataDir)
			return nil, fmt.Errorf("convert/chrome: allocating browser: %w", err)
		}
	case <-time.After(startTimeout):
		rootCancel()
		allocCancel()
		_ = os.RemoveAll(userDataDir)
		return nil, fmt.Errorf(
			"convert/chrome: browser did not become usable within %s (launched %q; the "+
				"process may have failed to complete the DevTools handshake, or completed "+
				"it and then stalled before its first tab became usable -- set "+
				"convert.Options.BrowserLog to capture Chrome's own output and tell them "+
				"apart)",
			startTimeout, execPath)
	}

	return &Session{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		rootCtx:     rootCtx,
		rootCancel:  rootCancel,
		userDataDir: userDataDir,

		allowRequest: opts.AllowRequest,
	}, nil
}

// newTabTimeout bounds NewTab's one round trip to the browser (opening the
// tab's window). Like DefaultStartTimeout it is a backstop against never
// returning, not a latency target.
const newTabTimeout = 30 * time.Second

// NewTab opens a new TAB on the SAME browser -- never a new process -- in a
// window of its own, and returns a child chromedp context for it. The context
// is not itself Run; the caller drives it via chromedp.Run(ctx, actions...),
// whose first call attaches to the tab.
//
// Why a window per tab: chromedp (v0.16) opens the tab of a child context in
// the browser's shared window. A full, new-headless Chrome hides every tab of a
// window except the active one -- it reports each such tab, even a Session's
// only one, as document.visibilityState "hidden" -- and a hidden page gets no
// compositor frames. Page.captureScreenshot waits for a frame, so png.ToImages
// waited too: on a loaded CI runner, whose full Chromium the build job
// auto-detects, for minutes and sometimes until go test's 10-minute alarm. A
// tab in its own window is visible and stays visible whatever else opens.
// chrome-headless-shell (the pinned EXP-04 browser) has no shared window and
// ignores newWindow, so nothing changes there; chromedp itself opens each tab in
// a new window by default from v0.21. TestNewTabIsVisible guards this.
//
// If the window cannot be opened, NewTab falls back to chromedp's own tab
// creation, whose first Run then reports the browser's error as before. A tab
// that is never Run keeps its blank window until Close ends the browser.
func (s *Session) NewTab() (context.Context, context.CancelFunc) {
	ctx, cancel := s.openTab()
	return s.guard(ctx, cancel)
}

func (s *Session) openTab() (context.Context, context.CancelFunc) {
	browser := chromedp.FromContext(s.rootCtx).Browser
	if browser == nil {
		return chromedp.NewContext(s.rootCtx)
	}

	ctx, cancel := context.WithTimeout(s.rootCtx, newTabTimeout)
	defer cancel()
	id, err := target.CreateTarget("about:blank").
		WithNewWindow(true).
		Do(cdp.WithExecutor(ctx, browser))
	if err != nil {
		return chromedp.NewContext(s.rootCtx)
	}
	return chromedp.NewContext(s.rootCtx, chromedp.WithTargetID(id))
}

// guard puts the tab behind request interception before any document is
// loaded into it (eden-press#42). The documents an export renders may be
// untrusted, and every URL they embed -- an <img>, a stylesheet, a CSS url(),
// a theme's @import -- would otherwise be fetched by the export host, which
// can reach internal services and cloud metadata that the document's author
// cannot.
//
// Fetch.enable with a "*" pattern pauses every network request the tab makes.
// A paused request continues only when convert.Options.AllowRequest returns
// true for its URL; with no AllowRequest it is failed as BlockedByClient. The
// export path itself needs no network: LoadHTML navigates to about:blank and
// injects the document with SetDocumentContent, and fonts are data: URIs,
// neither of which is a network request.
//
// guard fails closed. NewTab has no error return, so when interception cannot
// be enabled the tab's context is cancelled with the reason as its cause, and
// the caller's first chromedp.Run fails rather than loading a document into
// an unguarded tab.
func (s *Session) guard(ctx context.Context, cancel context.CancelFunc) (context.Context, context.CancelFunc) {
	allow := s.allowRequest
	chromedp.ListenTarget(ctx, func(ev any) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		// A listener must not block the event loop: answer on a goroutine.
		go func() {
			c := chromedp.FromContext(ctx)
			if c == nil || c.Target == nil {
				return
			}
			exec := cdp.WithExecutor(ctx, c.Target)
			if allow != nil && allow(e.Request.URL) {
				_ = fetch.ContinueRequest(e.RequestID).Do(exec)
				return
			}
			_ = fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(exec)
		}()
	})

	// This Run is the tab's FIRST, which attaches to it -- and chromedp ties
	// the tab's lifetime to the context of that first Run. It must therefore
	// run on ctx itself, never on a derived context with a deadline (whose
	// expiry would close the tab under the caller). The bound is a select,
	// as in New.
	enabled := make(chan error, 1)
	go func() {
		enabled <- chromedp.Run(ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}))
	}()
	var err error
	select {
	case err = <-enabled:
	case <-time.After(newTabTimeout):
		err = fmt.Errorf("no answer within %s", newTabTimeout)
	}
	if err != nil {
		guarded, cancelCause := context.WithCancelCause(ctx)
		cancelCause(fmt.Errorf("convert/chrome: enabling request interception: %w", err))
		return guarded, func() { cancelCause(context.Canceled); cancel() }
	}
	return ctx, cancel
}

// Close tears the browser down: cancels the root tab, then the allocator
// (which stops the Chrome process and waits for it to exit), then removes
// the unique user-data-dir created for this run.
func (s *Session) Close() {
	s.rootCancel()
	s.allocCancel()
	if s.userDataDir != "" {
		_ = os.RemoveAll(s.userDataDir)
	}
}
