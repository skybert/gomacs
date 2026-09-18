package editor

import (
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/window"
)

// debugSetupLayout creates the 3-pane debug layout and replaces the current
// single-window arrangement with:
//
//	+--------------------+----------+
//	|  source (read-only)| *Locals* |  ~70%/30% width, ~65% height
//	|  gutter on left    +----------+
//	|                    | *Stack*  |
//	+--------------------+----------+
//	|  *Debug REPL*  (full width)   |  ~35% height
//	+-------------------------------+
//
// The current active window becomes windows[0] (source); three new windows
// are added at indices [1], [2], [3].
func (e *Editor) debugSetupLayout() {
	tw, th := e.term.Size()
	totalH := th - 1 // subtract minibuffer row

	// Compute geometry.
	rightW := max(tw/3, 10)
	sourceW := tw - rightW - 1 // 1 separator column between source and panels
	topH := (totalH * 65) / 100
	replH := totalH - topH
	localsH := topH / 2
	stackH := topH - localsH

	// Collapse any existing splits to a single source window.
	src := e.activeWin
	e.windows = []*window.Window{src}

	// Resize the source window to occupy the left/top area.
	src.SetRegion(0, 0, sourceW, topH)
	e.debugMarkSourceReadOnly(src.Buf())
	src.SetGutterWidth(2)

	// Create (or reuse) panel buffers.  The REPL carries the debugged language
	// in its mode so that it gets that language's syntax highlighting.
	localsBuf := e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	stackBuf := e.ensureDebugBuf("*Debug Stack*", "debug-stack")
	replBuf := e.ensureDebugBuf("*Debug REPL*", dapReplModeFor(e.dap.mode))

	// Create panel windows.
	localsWin := window.New(localsBuf, 0, sourceW+1, rightW, localsH)
	stackWin := window.New(stackBuf, localsH, sourceW+1, rightW, stackH)
	replWin := window.New(replBuf, topH, 0, tw, replH)

	// Add the three panel windows (source stays at index 0).
	e.windows = append(e.windows, localsWin, stackWin, replWin)

	// Stash panel buffer pointers on the dapState.
	e.dap.localsBuf = localsBuf
	e.dap.stackBuf = stackBuf
	e.dap.replBuf = replBuf
	e.dap.prevActiveWin = src

	// Seed the REPL prompt line.
	dapReplReset(replBuf)

	e.invalidateLayout()
}

// ensureDebugBuf returns the named buffer (creating it if absent) and sets its mode.
func (e *Editor) ensureDebugBuf(name, mode string) *buffer.Buffer {
	b := e.FindBuffer(name)
	if b == nil {
		b = buffer.New(name)
		e.buffers = append(e.buffers, b)
	}
	b.SetMode(mode)
	return b
}

// dapReplModeFor returns the buffer mode for the REPL of a debug session on the
// given language, e.g. "debug-repl+java".  highlighterFor understands the
// suffix and highlights the REPL with that language's highlighter; an empty or
// unknown language falls back to plain "debug-repl".
func dapReplModeFor(lang string) string {
	if lang == "" {
		return debugReplMode
	}
	return debugReplMode + "+" + lang
}

// debugMarkSourceReadOnly forces buf read-only for the duration of the debug
// session so that single-letter navigation shortcuts work, remembering the
// buffer's previous flag so debugTeardownLayout can restore it.  Called for the
// buffer the session started in and for every source file stepping opens later.
func (e *Editor) debugMarkSourceReadOnly(buf *buffer.Buffer) {
	if e.dap == nil || buf == nil {
		return
	}
	if e.dap.prevReadOnly == nil {
		e.dap.prevReadOnly = make(map[*buffer.Buffer]bool)
	}
	if _, seen := e.dap.prevReadOnly[buf]; !seen {
		e.dap.prevReadOnly[buf] = buf.ReadOnly()
	}
	buf.SetReadOnly(true)
}

// debugAdoptSourceBuffer brings a buffer that was opened after the debug session
// started under the session's read-only rule.  Stepping into a new file goes
// through debugMarkSourceReadOnly directly (see dapFetchStoppedInfo), but a file
// the *user* opens mid-session — find-file while stopped at a breakpoint — would
// otherwise stay writable, and the single-letter shortcuts (n i o c e q) would
// be typed into the file instead of driving the debugger.
//
// Only file-backed buffers are adopted: the debug panels, *Help*, *messages* and
// the VC commit buffer have no filename and are not source code.  The buffer's
// original flag is recorded by debugMarkSourceReadOnly, so debugTeardownLayout
// restores it on debug-exit and a buffer the user could write before the session
// is writable again after it.  A no-op when no session is active.
func (e *Editor) debugAdoptSourceBuffer(buf *buffer.Buffer) {
	if e.dap == nil || buf == nil || buf.Filename() == "" {
		return
	}
	e.debugMarkSourceReadOnly(buf)
}

// debugTeardownLayout removes the 3 debug panel windows, restores the source
// window to full-screen, and clears read-only / gutter settings.  Every source
// buffer the session forced read-only gets its original flag back.
func (e *Editor) debugTeardownLayout() {
	if e.dap == nil {
		return
	}

	// Remove all windows except the source (windows[0]).
	if len(e.windows) >= 4 {
		e.windows = e.windows[:1]
	}

	src := e.windows[0]
	for b, readOnly := range e.dap.prevReadOnly {
		b.SetReadOnly(readOnly)
	}
	if _, tracked := e.dap.prevReadOnly[src.Buf()]; !tracked {
		// Session state was set up without going through debugSetupLayout.
		src.Buf().SetReadOnly(false)
	}
	e.dap.prevReadOnly = nil
	src.SetGutterWidth(0)

	// Restore active window.
	e.activeWin = src
	// Reset split tree to a single window after DAP teardown.
	e.layoutRoot = leafNode(src)

	e.invalidateLayout()
}

// dapRelayoutWindows reflows the 4 debug windows to fit totalW×totalH.
// Called by relayoutWindows when e.dap != nil and len(e.windows) == 4.
func (e *Editor) dapRelayoutWindows(totalW, totalH int) {
	if len(e.windows) != 4 {
		return
	}

	rightW := max(totalW/3, 10)
	sourceW := totalW - rightW - 1
	topH := (totalH * 65) / 100
	replH := totalH - topH
	localsH := topH / 2
	stackH := topH - localsH

	e.windows[0].SetRegion(0, 0, sourceW, topH)
	e.windows[1].SetRegion(0, sourceW+1, rightW, localsH)
	e.windows[2].SetRegion(localsH, sourceW+1, rightW, stackH)
	e.windows[3].SetRegion(topH, 0, totalW, replH)
}
