package editor

import (
	"sync"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/window"
)

// dapState holds all state for an active debug session.
// The Editor field e.dap is nil when no session is running.
type dapState struct {
	// backend is the protocol-level adapter (DAP today, but abstractable).
	// Set after the handshake completes; nil during early setup.
	backend debugBackend
	// client is the underlying DAP client, kept for internal helpers that need
	// direct protocol access (fetch variables, scopes, stack frames, etc.).
	client *dap.Client

	mode string // buffer mode that started the session (e.g. "go")

	// Current stopped position; empty stoppedFile means the program is running.
	stoppedFile   string
	stoppedLine   int // 1-based
	stoppedThread int

	// Locals panel: flat tree of variables in the current scope.
	localsMu sync.RWMutex
	locals   []dapVariable

	// Call-stack panel: frames for the stopped thread, plus the per-thread
	// breakdown rendered in the panel (stopped thread first).
	framesMu sync.RWMutex
	frames   []dap.StackFrame
	threads  []dapThread

	// Buffers backing the three debug panels.
	localsBuf *buffer.Buffer
	stackBuf  *buffer.Buffer
	replBuf   *buffer.Buffer

	// prevActiveWin is restored as the active window when the session ends.
	prevActiveWin *window.Window

	// prevReadOnly remembers the read-only flag every source buffer had before
	// the session forced it read-only, so teardown can restore each of them.
	prevReadOnly map[*buffer.Buffer]bool

	// localsAutoExpandDepth is the maximum depth to auto-expand variable trees.
	// Default 1. Configurable via (setq debug-locals-auto-expand-depth 2).
	localsAutoExpandDepth int

	// localsLineMap maps rendered line index (0-based) to the *dapVariable it
	// represents in the variable tree.  Rebuilt on every call to dapRenderLocals.
	// Only accessed from the main goroutine so no lock is needed.
	localsLineMap []*dapVariable

	// stackLineMap maps rendered line index (0-based) of the stack panel to the
	// frame on that line; nil for thread header lines.  Rebuilt on every call to
	// dapRenderStack and only accessed from the main goroutine.
	stackLineMap []*dap.StackFrame

	// replHistory stores previously evaluated REPL expressions.
	replHistory    []string
	replHistoryIdx int
}

// dapThread is one thread of the debuggee together with the stack frames fetched
// for it.  The call-stack panel shows every thread, grouped, with the thread
// that reported the stopped event first and flagged.
type dapThread struct {
	id      int
	name    string
	stopped bool
	frames  []dap.StackFrame
}

// dapHasBreakpoint reports whether there is a breakpoint on the given 1-based
// line of the file.  Returns false when no debug session is active or the file
// has no breakpoints.  Must be called from the main goroutine (no lock needed).
//
// The breakpoint map is keyed by canonical path, so file is canonicalised here:
// a caller passing a relative name or an unresolved /var path would otherwise
// never match.  renderWindow does not use this helper — it resolves the path
// once per frame and indexes the map directly, since canonPath hits the disk.
func (e *Editor) dapHasBreakpoint(file string, line int) bool {
	lines, ok := e.dapBreakpoints[file]
	if !ok {
		if file == "" {
			return false
		}
		if lines, ok = e.dapBreakpoints[canonPath(file)]; !ok {
			return false
		}
	}
	_, has := lines[line]
	return has
}

type dapVariable struct {
	depth    int
	name     string
	value    string
	typeStr  string
	varRef   int // variablesReference; 0 = leaf
	expanded bool
	children []dapVariable
}
