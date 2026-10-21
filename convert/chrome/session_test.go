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
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"go.aocyber.ai/eden-press/convert"
)

// TestSessionMultiTab is Test-list case 6: one Session/browser serves
// multiple tabs. Gated on Chrome presence -- t.Skip cleanly (never fail) if
// Discover cannot find a Chrome/Chromium executable anywhere, which is
// exactly the EXP-04 no-system-Chrome case 05-05 hardens in CI.
func TestSessionMultiTab(t *testing.T) {
	if _, _, err := Discover(DiscoverOptions{}); errors.Is(err, ErrChromeNotFound) {
		t.Skip("no Chrome discovered")
	}

	sess, err := New(convert.Options{})
	if err != nil {
		if errors.Is(err, ErrChromeNotFound) {
			t.Skip("no Chrome discovered")
		}
		t.Fatalf("New: %v", err)
	}
	defer sess.Close()

	tab1, cancel1 := sess.NewTab()
	defer cancel1()
	if err := chromedp.Run(tab1, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("tab1 Navigate: %v", err)
	}

	tab2, cancel2 := sess.NewTab()
	defer cancel2()
	if err := chromedp.Run(tab2, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("tab2 Navigate: %v", err)
	}

	c1 := chromedp.FromContext(tab1)
	c2 := chromedp.FromContext(tab2)
	if c1.Browser == nil || c2.Browser == nil {
		t.Fatal("expected both tabs to have a non-nil Browser")
	}
	if c1.Browser != c2.Browser {
		t.Error("expected both tabs to share the SAME Browser (one browser, many tabs)")
	}
	if c1.Target == c2.Target {
		t.Error("expected each tab to have its OWN Target (distinct tabs)")
	}
}

// TestNewTabIsVisible is the regression test for the png export hang. A tab
// that chromedp opens in the browser's shared window is HIDDEN in a full
// (new-headless) Chrome: Chrome hides every tab of a window except the active
// one, and a hidden page gets no compositor frames. Page.captureScreenshot
// waits for a frame, so png.ToImages waited too -- on a loaded CI runner for
// minutes, and sometimes until go test's 10-minute alarm. Every tab NewTab
// hands out must be visible, and must stay visible while another tab of the
// same Session is open.
//
// chrome-headless-shell has no shared window (each of its tabs is visible), so
// this test passes there either way. It bites on a full Chrome, which is what
// Discover's auto-detection finds on a developer machine or a CI runner.
func TestNewTabIsVisible(t *testing.T) {
	if _, _, err := Discover(DiscoverOptions{}); errors.Is(err, ErrChromeNotFound) {
		t.Skip("no Chrome discovered")
	}

	sess, err := New(convert.Options{})
	if err != nil {
		if errors.Is(err, ErrChromeNotFound) {
			t.Skip("no Chrome discovered")
		}
		t.Fatalf("New: %v", err)
	}
	defer sess.Close()

	// open attaches the tab with an UNBOUNDED first Run: chromedp runs the
	// tab's event loop on the context of that first Run, so a deadline there
	// would end the tab with it.
	open := func(name string) context.Context {
		t.Helper()
		tab, cancel := sess.NewTab()
		t.Cleanup(cancel)
		if err := chromedp.Run(tab); err != nil {
			t.Fatalf("%s: attaching: %v", name, err)
		}
		return tab
	}
	// visibility is bounded: reading it needs no frame, but a regression must
	// fail here, not hang.
	visibility := func(name string, tab context.Context) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(tab, 30*time.Second)
		defer cancel()
		var state string
		if err := chromedp.Run(ctx, chromedp.Evaluate("document.visibilityState", &state)); err != nil {
			t.Fatalf("%s: reading document.visibilityState: %v", name, err)
		}
		return state
	}

	tab1 := open("tab1")
	if got := visibility("tab1", tab1); got != "visible" {
		t.Errorf("tab1: document.visibilityState = %q, want \"visible\" (a hidden tab gets no frames, so a screenshot of it can wait forever)", got)
	}

	tab2 := open("tab2")
	if got := visibility("tab2", tab2); got != "visible" {
		t.Errorf("tab2: document.visibilityState = %q, want \"visible\"", got)
	}
	if got := visibility("tab1 after tab2 opened", tab1); got != "visible" {
		t.Errorf("tab1 after tab2 opened: document.visibilityState = %q, want \"visible\" (opening a tab must not hide another)", got)
	}
}

// fakeHangingBrowser writes an executable stand-in for Chrome that starts
// successfully and then never says anything: it prints no "DevTools listening
// on ..." line and does not exit until killed. It returns its path.
//
// The fake is load-bearing, not a convenience. A REAL Chrome cannot be asked
// to hang on demand -- the failure being pinned here is exactly the one that
// only appears when a browser process starts but never completes its DevTools
// handshake, which is not a state a healthy browser can be driven into. The
// fake reproduces that state precisely, and because it IS the browser as far
// as the allocator is concerned, the test needs no Chrome installed and must
// never skip.
//
// "exec sleep" rather than a bare "sleep" matters: exec REPLACES the shell, so
// the process chromedp launches is itself the sleeping process. Without it,
// cancellation would kill the shell while its sleep child survived as an
// orphan for the full duration.
func fakeHangingBrowser(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-chrome.sh")
	const script = "#!/bin/sh\n" +
		"# Deliberately silent: never prints \"DevTools listening on ...\".\n" +
		"exec sleep 120\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake browser: %v", err)
	}
	// Explicit chmod: WriteFile's perm argument is subject to the process
	// umask, and the allocator will refuse a non-executable path.
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fake browser: %v", err)
	}
	return path
}

// TestNewStartTimeoutBoundsHangingBrowser is the regression test for the
// production failure: browser allocation had no bound of any kind, so a
// browser that started but never completed its DevTools handshake blocked New
// forever -- no error, no log line, nothing to alert on.
//
// The single most important assertion here is that New RETURNS AT ALL.
// Unbounded, this case never finishes.
func TestNewStartTimeoutBoundsHangingBrowser(t *testing.T) {
	fake := fakeHangingBrowser(t)

	const startTimeout = 300 * time.Millisecond
	// A generous multiple of startTimeout. The claim under test is "New
	// returns", not "New returns quickly", and the honest failure mode of a
	// regression is a hang -- so the select below converts that hang into a
	// named test failure instead of a ten-minute package timeout and a
	// goroutine dump.
	const returnDeadline = 5 * time.Second

	type result struct {
		sess *Session
		err  error
	}
	// Buffered: if New has regressed and returns late, its goroutine must
	// still be able to send without blocking forever.
	done := make(chan result, 1)

	start := time.Now()
	go func() {
		sess, err := New(convert.Options{BrowserPath: fake, StartTimeout: startTimeout})
		done <- result{sess: sess, err: err}
	}()

	var got result
	select {
	case got = <-done:
	case <-time.After(returnDeadline):
		t.Fatalf("New did not return within %s against a browser that never completes its "+
			"DevTools handshake -- browser allocation is unbounded again", returnDeadline)
	}
	elapsed := time.Since(start)
	t.Logf("New returned in %s (StartTimeout %s, deadline %s)", elapsed, startTimeout, returnDeadline)

	if got.err == nil {
		t.Fatal("New returned a nil error for a browser that never became usable")
	}
	if got.sess != nil {
		t.Error("New returned a non-nil Session alongside an error; the caller would leak a " +
			"browser it has no reason to Close")
	}
	if elapsed < startTimeout {
		t.Errorf("New returned in %s, sooner than its own %s bound -- the error came from "+
			"something other than the startup timeout, so this test is no longer "+
			"exercising the hang", elapsed, startTimeout)
	}

	msg := got.err.Error()
	if !strings.Contains(msg, "did not become usable") {
		t.Errorf("error %q does not identify the failure as a startup timeout (want it to "+
			"contain %q)", msg, "did not become usable")
	}
	// Discover resolves the executable through a three-tier fallback chain
	// (BrowserPath, then CHROME_PATH, then PATH auto-detection), so "a browser
	// failed to start" is not actionable on its own: whoever reads this error
	// cannot tell WHICH executable was launched, and therefore cannot tell
	// which tier to fix.
	if !strings.Contains(msg, fake) {
		t.Errorf("error %q does not name the executable it launched (want it to contain %q)",
			msg, fake)
	}
}

// fakeSlowDevToolsBrowser writes a fake browser that prints its "DevTools
// listening on" line only after delaySec seconds -- past chromedp's own 20s
// websocket-URL default -- naming a closed port (127.0.0.1:1), then idles.
func fakeSlowDevToolsBrowser(t *testing.T, delaySec int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-slow-chrome.sh")
	script := "#!/bin/sh\n" +
		"sleep " + strconv.Itoa(delaySec) + "\n" +
		"echo 'DevTools listening on ws://127.0.0.1:1/devtools/browser/fake' >&2\n" +
		"exec sleep 120\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake browser: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fake browser: %v", err)
	}
	return path
}

// TestNewStartTimeoutGovernsTheDevToolsHandshake: Options.StartTimeout (default
// DefaultStartTimeout, 90s) must bound the wait for the browser's DevTools URL
// too. chromedp's ExecAllocator has its own 20s websocket-URL timeout, and when
// New did not pass StartTimeout through, a browser that was merely slow to
// start (a contended CI runner launching several at once) failed at 20s with
// "websocket url timeout reached" -- long before the documented bound.
//
// The fake prints its DevTools URL at 21s, naming a closed port, so New must
// still fail, but on the connection, not on chromedp's 20s timer.
func TestNewStartTimeoutGovernsTheDevToolsHandshake(t *testing.T) {
	if testing.Short() {
		t.Skip("waits past chromedp's 20s websocket-URL default")
	}
	fake := fakeSlowDevToolsBrowser(t, 21)

	type result struct {
		sess *Session
		err  error
	}
	done := make(chan result, 1)
	go func() {
		sess, err := New(convert.Options{BrowserPath: fake, StartTimeout: 40 * time.Second})
		done <- result{sess: sess, err: err}
	}()

	var got result
	select {
	case got = <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("New did not return within 60s against a browser that prints its DevTools URL at 21s")
	}
	if got.sess != nil {
		got.sess.Close()
		t.Fatal("New returned a Session for a DevTools URL that names a closed port")
	}
	if got.err == nil {
		t.Fatal("New returned a nil error for a DevTools URL that names a closed port")
	}
	if strings.Contains(got.err.Error(), "websocket url timeout") {
		t.Fatalf("New gave up at chromedp's built-in 20s websocket-URL timeout instead of its own "+
			"StartTimeout (40s): %v", got.err)
	}
}

// browserLogSentinel is the line the recording fake prints to stderr. It is
// deliberately unmistakable so its arrival in a caller's writer cannot be
// confused with anything chromedp or the shell might emit on its own.
const browserLogSentinel = "eden-press-fake-chrome-sentinel-9f3a1c"

// fakeBrowserLifetime is the StartTimeout the recording tests give New, and so
// is how long the fake browser survives before New cancels its context and
// kills it.
//
// It is deliberately generous, and 300ms was measurably NOT enough: under
// `go test ./... -race` the whole repo's packages compete for cores, and the
// fake lost the race between being exec'd and being killed -- the argv file it
// had not yet written then never appeared, no matter how long the assertion
// polled for it. Polling cannot rescue a process that is already dead, so the
// margin has to live here rather than in the poll deadline.
//
// These tests assert on what the fake RECORDED, never on how long anything
// took, so a large value costs a few seconds and buys determinism. The bound
// being exercised by TestNewStartTimeoutBoundsHangingBrowser is a different
// concern and keeps its own tight timeout.
const fakeBrowserLifetime = 3 * time.Second

// syncWriter is a mutex-guarded sink for Chrome's combined output.
//
// A bare bytes.Buffer would be a data race, not a style preference: chromedp
// writes browser output from readOutput's own goroutine while the test
// goroutine reads it back, so `go test -race` flags the unsynchronised buffer
// immediately.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// fakeRecordingBrowser writes an executable stand-in for Chrome that records
// the argv it was launched with, announces itself on stderr, and then hangs
// exactly like fakeHangingBrowser does.
//
// The fake is what makes these tests runnable with no Chrome installed, which
// is the situation on any dev box where Chrome lives outside PATH: Discover's
// tier 1 (Options.BrowserPath) short-circuits the entire fallback chain, so the
// allocator launches this script believing it is a browser. That matters
// because a regression test that skips is not a regression test, and the flags
// being pinned here are exactly the ones a CI box without Chrome would
// otherwise never check.
//
// Three details are load-bearing:
//   - argv is recorded one argument per line, so an assertion can match a bare
//     token like "--disable-gpu" exactly rather than substring-matching a
//     flattened command line (which would also match --disable-gpu-sandbox).
//   - the sentinel goes to stderr and ends in a newline. allocate.go sets
//     cmd.Stderr = cmd.Stdout before taking StdoutPipe, so stderr reaches
//     readOutput on the same pipe, and readOutput only forwards COMPLETE lines
//     (it buffers on ReadBytes('\n')).
//   - "exec sleep" REPLACES the shell, so cancelling the context kills the
//     process that is actually sleeping instead of orphaning a child.
func fakeRecordingBrowser(t *testing.T) (execPath, argvPath string) {
	t.Helper()

	dir := t.TempDir()
	execPath = filepath.Join(dir, "fake-chrome-recording.sh")
	argvPath = filepath.Join(dir, "argv.txt")

	script := "#!/bin/sh\n" +
		"# Record every argument one-per-line for the argv assertions.\n" +
		": > '" + argvPath + "'\n" +
		"for arg in \"$@\"; do\n" +
		"\tprintf '%s\\n' \"$arg\" >> '" + argvPath + "'\n" +
		"done\n" +
		"# Complete line on stderr; readOutput forwards it while hunting for the\n" +
		"# DevTools URL that this fake deliberately never prints.\n" +
		"echo '" + browserLogSentinel + "' >&2\n" +
		"exec sleep 120\n"

	if err := os.WriteFile(execPath, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake recording browser: %v", err)
	}
	// Explicit chmod: WriteFile's perm argument is subject to the process
	// umask, and the allocator will refuse a non-executable path.
	if err := os.Chmod(execPath, 0o755); err != nil {
		t.Fatalf("chmod fake recording browser: %v", err)
	}
	return execPath, argvPath
}

// runNewAgainstFake calls New in a goroutine and waits for it with a generous
// deadline, mirroring TestNewStartTimeoutBoundsHangingBrowser.
//
// New is EXPECTED to return an error here -- the fake never completes the
// DevTools handshake, so the StartTimeout backstop always trips. That error is
// not the assertion; the tests that call this examine what the fake RECORDED.
// The select exists so that a regression which reintroduces an unbounded
// allocation surfaces as a named failure instead of a package-wide timeout.
func runNewAgainstFake(t *testing.T, opts convert.Options) {
	t.Helper()

	// Must comfortably exceed fakeBrowserLifetime, or this "did New return at
	// all" guard would fire on a healthy run instead of on the regression it
	// exists to catch.
	const returnDeadline = fakeBrowserLifetime + 5*time.Second

	type result struct {
		sess *Session
		err  error
	}
	done := make(chan result, 1)
	go func() {
		sess, err := New(opts)
		done <- result{sess: sess, err: err}
	}()

	select {
	case got := <-done:
		if got.sess != nil {
			got.sess.Close()
		}
	case <-time.After(returnDeadline):
		t.Fatalf("New did not return within %s against a browser that never completes its "+
			"DevTools handshake -- browser allocation is unbounded again", returnDeadline)
	}
}

// recordedArgv polls argvPath until the fake has written it, then splits it
// into one entry per argument.
//
// The poll is cheap insurance rather than a fix for a real ordering problem:
// the fake records argv within milliseconds of launch and New does not return
// until its StartTimeout expires, so the file is essentially always present
// already. On a loaded machine, though, "essentially always" is how flaky tests
// are written.
func recordedArgv(t *testing.T, argvPath string) []string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		b, err := os.ReadFile(argvPath)
		if err == nil && len(b) > 0 {
			return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake browser never recorded its argv at %s (read error: %v) -- it was "+
				"probably never executed, so check that BrowserPath reached Discover "+
				"tier 1 and that the script is executable", argvPath, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// argvHasToken reports whether argv contains an EXACT token. Exactness matters:
// a substring search for "--disable-gpu" would also be satisfied by
// "--disable-gpu-sandbox", which is a different flag with different effects.
func argvHasToken(argv []string, token string) bool {
	for _, arg := range argv {
		if arg == token {
			return true
		}
	}
	return false
}

// TestNewLaunchesBrowserWithDisableGPU pins the flag that fixed a production
// hang, and it is a real change: --disable-gpu is absent from
// chromedp.DefaultExecAllocatorOptions, so nothing supplies it unless New does.
//
// Mechanism, reproduced live in a production scratch pod with a CDP wire trace
// rather than inferred: Chrome's GPU/viz process crash-loops inside a GPU-less
// container. The page renderer therefore never obtains an execution context and
// never answers Runtime.enable -- the third command chromedp sends while
// attaching to the first target, inside attachTarget. Browser.Execute's only
// escape is ctx.Done(), so chromedp.Run blocks. New runs chromedp.Run(rootCtx)
// with zero actions purely to populate the Browser, and that call waits for the
// browser's initial tab, so the hang lands squarely in allocation.
//
// Measured in that pod, same image, everything else held constant:
//
//	production flags today ... hung past 45s
//	+ --disable-gpu .......... allocation returned in 808ms, and a full raster
//	                           render succeeded (5754-byte PNG of styled HTML)
func TestNewLaunchesBrowserWithDisableGPU(t *testing.T) {
	fake, argvPath := fakeRecordingBrowser(t)

	runNewAgainstFake(t, convert.Options{
		BrowserPath:  fake,
		StartTimeout: fakeBrowserLifetime,
	})

	argv := recordedArgv(t, argvPath)
	if !argvHasToken(argv, "--disable-gpu") {
		t.Errorf("browser launched without --disable-gpu (argv: %v)\n\n"+
			"Without it the GPU/viz process crash-loops in a GPU-less container, the "+
			"renderer never answers Runtime.enable, and browser allocation hangs until "+
			"StartTimeout -- which is how an export sidecar died in production.", argv)
	}
}

// TestNewDoesNotDisableSoftwareRasterizer is a standing prohibition, not a
// description of current behaviour, and it is EXPECTED to pass from the moment
// it is written. Its job is to fail the day somebody adds the flag.
//
// --disable-software-rasterizer disables SwiftShader, which is the thing that
// actually rasterizes PDF and PNG output once the GPU is off. Pairing it with
// --disable-gpu is a common copy-paste habit, and here it would trade a fixed
// hang for silently broken output. --disable-gpu ALONE was verified sufficient
// in the production pod and verified to still rasterize correctly, so the
// second flag buys nothing and costs the renderer.
//
// This is also why New passes chromedp.Flag("disable-gpu", true) directly
// rather than the chromedp.DisableGPU helper: that helper additionally sets
// --enable-unsafe-swiftshader, which was not part of what was verified.
func TestNewDoesNotDisableSoftwareRasterizer(t *testing.T) {
	fake, argvPath := fakeRecordingBrowser(t)

	runNewAgainstFake(t, convert.Options{
		BrowserPath:  fake,
		StartTimeout: fakeBrowserLifetime,
	})

	argv := recordedArgv(t, argvPath)
	if argvHasToken(argv, "--disable-software-rasterizer") {
		t.Errorf("browser launched WITH --disable-software-rasterizer (argv: %v)\n\n"+
			"That flag disables SwiftShader, which is what rasterizes PDF/PNG output "+
			"once the GPU is disabled. --disable-gpu alone was verified sufficient AND "+
			"verified to still rasterize; adding this one trades a fixed hang for "+
			"broken output.", argv)
	}
}

// TestNewForwardsBrowserOutputToBrowserLog pins the diagnostic that was missing
// during two failed production deploys.
//
// Chrome was emitting GPU crash messages continuously throughout both, and the
// operator saw none of them, because chromedp closes the browser's output pipe
// outright when no writer is configured -- and it closes it right after parsing
// the websocket URL, which is BEFORE the crash-loop starts talking. Setting
// this writer produced the root cause in a single run.
//
// Note what this test proves about ordering: the fake never prints "DevTools
// listening on", so readOutput is still inside its hunting loop when the
// sentinel arrives, and that loop forwards every complete line it sees. The
// sentinel therefore reaches the caller's writer even though allocation
// ultimately fails -- which is exactly the failing-launch case the field is for.
func TestNewForwardsBrowserOutputToBrowserLog(t *testing.T) {
	fake, _ := fakeRecordingBrowser(t)

	var log syncWriter
	runNewAgainstFake(t, convert.Options{
		BrowserPath:  fake,
		StartTimeout: fakeBrowserLifetime,
		BrowserLog:   &log,
	})

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(log.String(), browserLogSentinel) {
		if time.Now().After(deadline) {
			t.Fatalf("Chrome's output never reached Options.BrowserLog: wanted %q, got %q\n\n"+
				"Without this the operator gets nothing at all from a failed launch, "+
				"because chromedp closes the output pipe when no writer is set.",
				browserLogSentinel, log.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestBrowserOutputNilYieldsDrainingWriter pins the nil path of browserOutput,
// which is the difference between having browser diagnostics and having none.
//
// The distinction is NOT cosmetic, and it is not the "a full pipe buffer blocks
// a chatty Chrome" story it resembles. chromedp v0.16.0's allocate.go
// readOutput reads Chrome's output line-by-line hunting for "DevTools listening
// on", and once it finds that URL it branches on the forward writer: when that
// writer is nil, readOutput calls Close on the pipe outright.
//
// So with no writer set, everything Chrome says after startup is destroyed at
// the source -- Chrome's later writes hit a closed pipe, they do not queue.
// That is precisely why two failed production deploys produced ZERO browser
// diagnostics while Chrome was crash-looping its GPU process and saying so the
// entire time: the crash-loop happens AFTER the websocket URL is parsed, i.e.
// after the pipe was already closed.
//
// Handing chromedp io.Discard instead of nil takes the other branch, where
// allocate.go:253 runs io.Copy on the allocator's WaitGroup for the browser's
// whole lifetime. The pipe stays open and drained, with no behaviour change for
// callers who never set BrowserLog.
func TestBrowserOutputNilYieldsDrainingWriter(t *testing.T) {
	got := browserOutput(nil)
	if got == nil {
		t.Fatal("browserOutput(nil) returned a nil io.Writer: chromedp would take its " +
			"pipe-closing branch and destroy every post-startup line Chrome emits")
	}
	if got != io.Discard {
		t.Errorf("browserOutput(nil) = %#v, want io.Discard -- the nil path must still hand "+
			"chromedp a real writer so the output pipe is kept open and drained", got)
	}
}

// TestBrowserOutputPassesCallerWriterThrough pins the other half of the
// contract: a caller who opts in by setting Options.BrowserLog must reach
// chromedp unmodified. If this helper ever wrapped, buffered, or substituted
// the caller's writer, the diagnostic that produced the production root cause
// in a single run would silently stop arriving.
//
// A bare bytes.Buffer is safe HERE specifically because this is a pure unit
// call with no browser and no goroutine behind it. Tests that hand a writer to
// a live allocator must use the mutex-guarded syncWriter instead, because
// chromedp writes from readOutput's goroutine while the test reads.
func TestBrowserOutputPassesCallerWriterThrough(t *testing.T) {
	var buf bytes.Buffer
	if got := browserOutput(&buf); got != io.Writer(&buf) {
		t.Errorf("browserOutput(w) = %#v, want the caller's own writer %#v", got, &buf)
	}
}

// TestDefaultStartTimeoutIsPositive pins the backstop itself. New treats a
// StartTimeout of zero or less as "unset" and substitutes DefaultStartTimeout,
// so setting that constant to zero would restore unbounded allocation while
// still compiling and still passing every other test in this package. This
// test is the only thing standing between that edit and production.
func TestDefaultStartTimeoutIsPositive(t *testing.T) {
	if DefaultStartTimeout <= 0 {
		t.Fatalf("DefaultStartTimeout = %s, want > 0: New reads a non-positive value as "+
			"\"unset\", which restores the unbounded browser allocation this package "+
			"exists to prevent", DefaultStartTimeout)
	}
}
