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

	// Call-stack panel: frames for the stopped thread.
	framesMu sync.RWMutex
	frames   []dap.StackFrame

	// Buffers backing the three debug panels.
	localsBuf *buffer.Buffer
	stackBuf  *buffer.Buffer
	replBuf   *buffer.Buffer

	// prevActiveWin is restored as the active window when the session ends.
	prevActiveWin *window.Window

	// localsAutoExpandDepth is the maximum depth to auto-expand variable trees.
	// Default 1. Configurable via (setq dap-locals-auto-expand-depth 2).
	localsAutoExpandDepth int

	// localsLineMap maps rendered line index (0-based) to the *dapVariable it
	// represents in the variable tree.  Rebuilt on every call to dapRenderLocals.
	// Only accessed from the main goroutine so no lock is needed.
	localsLineMap []*dapVariable

	// replHistory stores previously evaluated REPL expressions.
	replHistory    []string
	replHistoryIdx int
}

// dapHasBreakpoint reports whether there is a breakpoint on the given 1-based
// line of the file.  Returns false when no debug session is active or the file
// has no breakpoints.  Must be called from the main goroutine (no lock needed).
func (e *Editor) dapHasBreakpoint(file string, line int) bool {
	lines, ok := e.dapBreakpoints[file]
	if !ok {
		return false
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
