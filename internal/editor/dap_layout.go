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
	src.Buf().SetReadOnly(true)
	src.SetGutterWidth(2)

	// Create (or reuse) panel buffers.
	localsBuf := e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	stackBuf := e.ensureDebugBuf("*Debug Stack*", "debug-stack")
	replBuf := e.ensureDebugBuf("*Debug REPL*", "debug-repl")

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

// debugTeardownLayout removes the 3 debug panel windows, restores the source
// window to full-screen, and clears read-only / gutter settings.
func (e *Editor) debugTeardownLayout() {
	if e.dap == nil {
		return
	}

	// Remove all windows except the source (windows[0]).
	if len(e.windows) >= 4 {
		e.windows = e.windows[:1]
	}

	src := e.windows[0]
	src.Buf().SetReadOnly(false)
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
