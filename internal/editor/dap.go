package editor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// dapAsync runs work in a goroutine.  When work returns a callback fn, fn is
// sent to e.dapCbs and the event loop is woken so it runs on the main goroutine.
func (e *Editor) dapAsync(work func() func()) {
	go func() {
		fn := work()
		if fn != nil {
			e.dapCbs <- fn
			e.term.PostWakeup()
		}
	}()
}

// ---- Toggle breakpoint -------------------------------------------------------

func (e *Editor) cmdDebugToggleBreakpoint() {
	e.clearArg()
	buf := e.ActiveBuffer()
	fname := buf.Filename()
	if fname == "" {
		e.Message("debug-toggle-breakpoint: buffer has no file")
		return
	}
	abs, err := filepath.Abs(fname)
	if err != nil {
		abs = fname
	}
	line, _ := buf.LineCol(buf.Point())

	if e.dapBreakpoints[abs] == nil {
		e.dapBreakpoints[abs] = make(map[int]struct{})
	}
	if _, exists := e.dapBreakpoints[abs][line]; exists {
		delete(e.dapBreakpoints[abs], line)
		e.Message("Breakpoint removed at line %d", line)
	} else {
		e.dapBreakpoints[abs][line] = struct{}{}
		e.Message("Breakpoint set at line %d", line)
	}

	// If a session is running, sync breakpoints for this file immediately.
	if e.dap != nil {
		e.dapSyncBreakpoints(abs)
	}
}

// dapSyncBreakpoints sends a setBreakpoints request for the given file.
func (e *Editor) dapSyncBreakpoints(absFile string) {
	if e.dap == nil {
		return
	}
	lines := e.dapBreakpoints[absFile]
	bps := make([]dap.SourceBreakpoint, 0, len(lines))
	for l := range lines {
		bps = append(bps, dap.SourceBreakpoint{Line: l})
	}
	client := e.dap.client
	e.dapAsync(func() func() {
		_, err := client.Request("setBreakpoints", dap.SetBreakpointsArgs{
			Source:      dap.Source{Path: absFile, Name: filepath.Base(absFile)},
			Breakpoints: bps,
		})
		if err != nil {
			return func() { e.Message("debug setBreakpoints: %v", err) }
		}
		return nil
	})
}

// ---- Start session ----------------------------------------------------------

func (e *Editor) cmdDebugStart() {
	e.clearArg()
	if e.dap != nil {
		e.Message("Debug session already active; use debug-exit first")
		return
	}
	buf := e.ActiveBuffer()
	info := langModeByName(buf.Mode())
	if info == nil || len(info.dapCmd) == 0 {
		e.Message("No debug adapter configured for mode %q", buf.Mode())
		return
	}

	launchArgs, err := e.dapLaunchArgs(buf)
	if err != nil {
		e.Message("debug-start: %v", err)
		return
	}

	cmd := info.dapCmd[0]
	args := info.dapCmd[1:]

	e.Message("Debugger: starting %s…", cmd)

	// Initialise dapState early so event handlers can reference it.
	e.dap = &dapState{
		mode:                  buf.Mode(),
		localsAutoExpandDepth: 1,
	}

	e.dapAsync(func() func() {
		c, startErr := dap.Start(cmd, args...)
		if startErr != nil {
			return func() {
				e.dap = nil
				e.Message("debug-start: cannot start %s: %v", cmd, startErr)
			}
		}

		// Wire the event handler before any requests go out.
		c.SetEventHandler(func(event string, body json.RawMessage) {
			e.dapCbs <- func() { e.dapHandleEvent(event, body) }
			e.term.PostWakeup()
		})

		// DAP handshake: initialize → launch → setBreakpoints → configurationDone.
		_, initErr := c.Request("initialize", dap.InitializeArgs{
			AdapterID:       "gomacs",
			LinesStartAt1:   true,
			ColumnsStartAt1: true,
		})
		if initErr != nil {
			c.Close()
			return func() {
				e.dap = nil
				e.Message("debug initialize: %v", initErr)
			}
		}

		_, launchErr := c.Request("launch", launchArgs)
		if launchErr != nil {
			c.Close()
			return func() {
				e.dap = nil
				e.Message("debug launch: %v", launchErr)
			}
		}

		// Send all pending breakpoints.
		for file, lines := range e.dapBreakpoints {
			if len(lines) == 0 {
				continue
			}
			bps := make([]dap.SourceBreakpoint, 0, len(lines))
			for l := range lines {
				bps = append(bps, dap.SourceBreakpoint{Line: l})
			}
			_, _ = c.Request("setBreakpoints", dap.SetBreakpointsArgs{
				Source:      dap.Source{Path: file, Name: filepath.Base(file)},
				Breakpoints: bps,
			})
		}

		_, _ = c.Request("configurationDone", nil)

		return func() {
			if e.dap == nil {
				// Session was cancelled (e.g. fast termination) before layout.
				c.Close()
				return
			}
			e.dap.client = c
			e.dap.backend = &dapBackend{client: c}
			e.debugSetupLayout()
			e.Message("Debug session started")
			// If a stopped event arrived during the handshake (before client was
			// set), fetch stack/locals now that the client is available.
			if e.dap.stoppedThread != 0 {
				e.dapFetchStoppedInfo(dap.StoppedEvent{
					ThreadID: e.dap.stoppedThread,
					Reason:   "breakpoint",
				})
			}
		}
	})
}

// dapLaunchArgs returns the launch arguments for the current buffer, detecting
// whether it should run as a test, a main program, or a headless server.
func (e *Editor) dapLaunchArgs(buf *buffer.Buffer) (dap.LaunchArgs, error) {
	fname := buf.Filename()
	if fname == "" {
		return nil, fmt.Errorf("buffer has no associated file")
	}
	abs, err := filepath.Abs(fname)
	if err != nil {
		abs = fname
	}

	info := langModeByName(buf.Mode())
	root := ""
	if info != nil {
		root = findProjectRoot(abs, info.rootMarkers)
	} else {
		root = filepath.Dir(abs)
	}

	switch {
	case strings.HasSuffix(abs, "_test.go"):
		testName := dapTestFuncAtPoint(buf)
		return dap.LaunchArgs{
			"mode":    "test",
			"program": filepath.Dir(abs), // package dir, not module root
			"args":    []string{"-test.run", testName},
		}, nil

	case bufContainsMainFunc(buf):
		return dap.LaunchArgs{
			"mode":    "debug",
			"program": filepath.Dir(abs),
		}, nil

	default:
		// Server / library: start headless.
		return dap.LaunchArgs{
			"mode":    "debug",
			"program": root,
		}, nil
	}
}

// dapTestFuncAtPoint scans backward from buf.Point() for the nearest
// "func TestXxx(…" declaration and returns the test name.  Falls back to "."
// (match all) if none is found.
func dapTestFuncAtPoint(buf *buffer.Buffer) string {
	content := buf.Substring(0, buf.Len())
	pt := buf.Point()
	// Work backwards through the text to find a Test func declaration.
	text := string([]rune(content)[:pt])
	re := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "."
	}
	return matches[len(matches)-1][1]
}

// bufContainsMainFunc reports whether the buffer content contains a
// "func main()" declaration.
func bufContainsMainFunc(buf *buffer.Buffer) bool {
	content := buf.Substring(0, buf.Len())
	return strings.Contains(content, "func main()")
}

// ---- Stepping / execution control ------------------------------------------

func (e *Editor) cmdDebugContinue() {
	e.clearArg()
	if e.dap == nil || e.dap.backend == nil {
		return
	}
	threadID := e.dap.stoppedThread
	backend := e.dap.backend
	e.dapAsync(func() func() {
		if err := backend.Continue(threadID); err != nil {
			return func() { e.Message("debug continue: %v", err) }
		}
		return nil
	})
}

func (e *Editor) cmdDebugStepNext() {
	e.clearArg()
	if e.dap == nil || e.dap.backend == nil {
		return
	}
	threadID := e.dap.stoppedThread
	backend := e.dap.backend
	e.dapAsync(func() func() {
		if err := backend.StepNext(threadID); err != nil {
			return func() { e.Message("debug step-next: %v", err) }
		}
		return nil
	})
}

func (e *Editor) cmdDebugStepIn() {
	e.clearArg()
	if e.dap == nil || e.dap.backend == nil {
		return
	}
	threadID := e.dap.stoppedThread
	backend := e.dap.backend
	e.dapAsync(func() func() {
		if err := backend.StepIn(threadID); err != nil {
			return func() { e.Message("debug step-in: %v", err) }
		}
		return nil
	})
}

func (e *Editor) cmdDebugStepOut() {
	e.clearArg()
	if e.dap == nil || e.dap.backend == nil {
		return
	}
	threadID := e.dap.stoppedThread
	backend := e.dap.backend
	e.dapAsync(func() func() {
		if err := backend.StepOut(threadID); err != nil {
			return func() { e.Message("debug step-out: %v", err) }
		}
		return nil
	})
}

// ---- Evaluate ---------------------------------------------------------------

func (e *Editor) cmdDebugEval() {
	e.clearArg()
	if e.dap == nil {
		e.Message("No active debug session")
		return
	}
	buf := e.ActiveBuffer()
	var expr string
	if buf.MarkActive() {
		mark, pt := buf.Mark(), buf.Point()
		if mark > pt {
			mark, pt = pt, mark
		}
		expr = buf.Substring(mark, pt)
	} else {
		expr = dapWordAtPoint(buf)
	}
	if expr == "" {
		e.Message("debug-eval: no expression at point")
		return
	}

	frameID := 0
	if len(e.dap.frames) > 0 {
		frameID = e.dap.frames[0].ID
	}
	stoppedThread := e.dap.stoppedThread
	backend := e.dap.backend
	e.dapAsync(func() func() {
		result, err := backend.Evaluate(expr, frameID, stoppedThread, "hover")
		if err != nil {
			return func() { e.Message("debug eval: %v", err) }
		}
		return func() {
			e.Message("%s = %s", expr, result)
			e.dapReplAppend(fmt.Sprintf("%s = %s", expr, result))
		}
	})
}

// dapWordAtPoint returns the identifier/word at buf's current point, or "".
func dapWordAtPoint(buf *buffer.Buffer) string {
	pt := buf.Point()
	n := buf.Len()
	isIdent := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' }

	start := pt
	for start > 0 && isIdent(buf.RuneAt(start-1)) {
		start--
	}
	end := pt
	for end < n && isIdent(buf.RuneAt(end)) {
		end++
	}
	if start >= end {
		return ""
	}
	return buf.Substring(start, end)
}

// ---- Exit -------------------------------------------------------------------

func (e *Editor) cmdDebugExit() {
	e.clearArg()
	if e.dap == nil {
		return
	}
	if e.dap.backend != nil {
		// Best-effort disconnect; run in background to avoid blocking the loop.
		backend := e.dap.backend
		go backend.Close()
	}
	e.debugTeardownLayout()
	e.dap = nil
	e.Message("Debug session ended")
}

// ---- Event handler ----------------------------------------------------------

// dapHandleEvent processes a DAP event on the main goroutine.
func (e *Editor) dapHandleEvent(event string, body json.RawMessage) {
	if e.dap == nil {
		return
	}
	switch event {
	case "stopped":
		var ev dap.StoppedEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return
		}
		e.dap.stoppedThread = ev.ThreadID

		// Client may not be set yet if this event races the setup callback.
		// dapFetchStoppedInfo guards against nil client internally.
		e.dapFetchStoppedInfo(ev)

	case "continued":
		e.dap.stoppedFile = ""
		e.dap.stoppedLine = 0

	case "terminated", "exited":
		e.cmdDebugExit()

	case "output":
		var ev dap.OutputEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return
		}
		if ev.Output != "" {
			e.dapReplAppend(ev.Output)
		}
	}
}

// dapFetchLocals fetches variables for a stack frame up to the given depth.
func dapFetchLocals(client *dap.Client, frameID, maxDepth int) []dapVariable {
	raw, err := client.Request("scopes", dap.ScopesArgs{FrameID: frameID})
	if err != nil {
		return nil
	}
	var scopeResp dap.ScopesResponse
	if err := json.Unmarshal(raw, &scopeResp); err != nil {
		return nil
	}
	if len(scopeResp.Scopes) == 0 {
		return nil
	}
	return dapFetchVars(client, scopeResp.Scopes[0].VariablesReference, 0, maxDepth)
}

func dapFetchVars(client *dap.Client, ref, depth, maxDepth int) []dapVariable {
	if ref == 0 || depth > maxDepth {
		return nil
	}
	raw, err := client.Request("variables", dap.VariablesArgs{VariablesReference: ref})
	if err != nil {
		return nil
	}
	var resp dap.VariablesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil
	}
	vars := make([]dapVariable, 0, len(resp.Variables))
	for _, v := range resp.Variables {
		dv := dapVariable{
			depth:   depth,
			name:    v.Name,
			value:   v.Value,
			typeStr: v.Type,
			varRef:  v.VariablesReference,
		}
		if v.VariablesReference != 0 && depth < maxDepth {
			dv.expanded = true
			dv.children = dapFetchVars(client, v.VariablesReference, depth+1, maxDepth)
		}
		vars = append(vars, dv)
	}
	return vars
}

// scrollWindowToLine scrolls window w so that the given 1-based line is visible.
func (e *Editor) scrollWindowToLine(w *window.Window, line int) {
	w.SetScrollLine(line - w.Height()/2)
}

// dapFetchStoppedInfo fetches the stack trace and locals for a stopped event.
// It is a no-op if the client is not yet available (race with setup callback).
func (e *Editor) dapFetchStoppedInfo(ev dap.StoppedEvent) {
	client := e.dap.client
	if client == nil {
		// Client not ready yet — setup callback will call this after setting client.
		return
	}
	threadID := ev.ThreadID
	e.dapAsync(func() func() {
		raw, err := client.Request("stackTrace", dap.StackTraceArgs{
			ThreadID: threadID,
			Levels:   20,
		})
		if err != nil {
			return nil
		}
		var resp dap.StackTraceResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil
		}
		frames := resp.StackFrames

		var localVars []dapVariable
		if len(frames) > 0 {
			localVars = dapFetchLocals(client, frames[0].ID, 1)
		}

		return func() {
			if e.dap == nil {
				return
			}
			e.dap.framesMu.Lock()
			e.dap.frames = frames
			e.dap.framesMu.Unlock()

			e.dap.localsMu.Lock()
			e.dap.locals = localVars
			e.dap.localsMu.Unlock()

			if len(frames) > 0 {
				top := frames[0]
				e.dap.stoppedFile = top.Source.Path
				e.dap.stoppedLine = top.Line
				if e.dap.prevActiveWin != nil {
					win := e.dap.prevActiveWin
					// If we stepped into a different file, load it.
					if top.Source.Path != "" && win.Buf().Filename() != top.Source.Path {
						if fileBuf, err := e.openFileIntoBuffer(top.Source.Path); err == nil {
							win.SetBuf(fileBuf)
						}
					}
					win.Buf().SetPoint(win.Buf().LineStart(top.Line))
					e.scrollWindowToLine(win, top.Line)
				}
			}

			e.dapRefreshPanels()
			e.Message("Stopped: %s (line %d)", ev.Reason, e.dap.stoppedLine)
		}
	})
}

// ---- Dispatch for source / panel buffers -----------------------------------

// debugSourceDispatch intercepts single-letter debug shortcuts when a debug
// session is active and the active buffer is a source file (not a debug panel).
func (e *Editor) debugSourceDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune || ke.Mod != 0 {
		return false
	}
	switch ke.Rune {
	case 'n':
		if e.dap.stoppedThread == 0 {
			e.Message("debugger: not stopped")
			return true
		}
		e.cmdDebugStepNext()
	case 'i', 's':
		if e.dap.stoppedThread == 0 {
			e.Message("debugger: not stopped")
			return true
		}
		e.cmdDebugStepIn()
	case 'o':
		if e.dap.stoppedThread == 0 {
			e.Message("debugger: not stopped")
			return true
		}
		e.cmdDebugStepOut()
	case 'c':
		e.cmdDebugContinue()
	case 'e':
		e.cmdDebugEval()
	case 'q':
		e.cmdDebugExit()
	default:
		return false
	}
	return true
}
