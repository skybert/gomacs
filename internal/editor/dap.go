package editor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/elisp"
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
	abs := canonPath(fname)
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
			Source:      dap.Source{Path: canonPath(absFile), Name: filepath.Base(absFile)},
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
	if info == nil || !info.hasDebugAdapter() {
		e.Message("No debug adapter configured for mode %q", buf.Mode())
		return
	}

	// A jdtls-style adapter is not a process gomacs spawns but a socket the
	// language server opens on request, so without a ready connection there is
	// nothing to ask.  Diagnose that here, where the mode's configured command is
	// still at hand to name in the message.
	if info.dapKind == dapAdapterJdtls {
		if err := jdtlsConnError(e.lspConns[buf.Mode()], info); err != nil {
			e.Message("debug-start: %v", err)
			return
		}
	}

	launchArgs, runDir, err := e.dapLaunchArgs(buf)
	if err != nil {
		e.Message("debug-start: %v", err)
		return
	}

	// Snapshot everything the worker goroutine needs while still on the main
	// goroutine; the editor's maps must not be read from a worker.
	req := dapLaunchRequest{
		file:    canonPath(buf.Filename()),
		runDir:  runDir,
		launch:  launchArgs,
		lspConn: e.lspConns[buf.Mode()],
	}

	e.Message("Debugger: starting %s…", info.dapAdapterName())

	// Initialise dapState early so event handlers can reference it.
	e.dap = &dapState{
		mode:                  buf.Mode(),
		localsAutoExpandDepth: e.dapLocalsAutoExpandDepth(),
	}

	e.dapAsync(func() func() {
		c, launch, note, startErr := dapStartAdapter(info, req)
		if startErr != nil {
			return func() {
				e.dap = nil
				e.Message("debug-start: %v", startErr)
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

		_, launchErr := c.Request("launch", launch)
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
				Source:      dap.Source{Path: canonPath(file), Name: filepath.Base(file)},
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
			if note != "" {
				// The adapter is debugging something other than the project's main
				// class; say so, because which breakpoints can hit depends on it.
				e.Message("Debug session started: %s", note)
			} else {
				e.Message("Debug session started")
			}
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

// dapLaunchRequest is the main-goroutine snapshot dapStartAdapter needs.  It is
// read from a worker goroutine, so it holds only immutable values.
type dapLaunchRequest struct {
	file    string         // absolute path of the buffer being debugged
	runDir  string         // adapter working directory (project root)
	launch  dap.LaunchArgs // pre-resolved launch args (process adapters)
	lspConn *lspConn       // language-server connection (jdtls adapters)
}

// dapStartAdapter starts the debug adapter for info and returns it together with
// the launch arguments to send.  It blocks on I/O (spawning a process, or LSP
// round-trips for jdtls) and so must run on a worker goroutine.  note is a
// message for the user, non-empty when resolving the launch arguments had to fall
// back to something other than the project's main class.
func dapStartAdapter(info *langModeInfo, req dapLaunchRequest) (c *dap.Client, launch dap.LaunchArgs, note string, err error) {
	if info.dapKind == dapAdapterJdtls {
		// Resolve the launch arguments first: doing so before the adapter exists
		// means a classpath failure does not leak a debug session.
		launch, note, err = dapJdtlsLaunchArgs(req.lspConn, req.file, req.runDir)
		if err != nil {
			return nil, nil, "", err
		}
		c, err = dapStartJdtls(req.lspConn)
		if err != nil {
			return nil, nil, "", err
		}
		return c, launch, note, nil
	}
	c, err = dap.Start(req.runDir, info.dapCmd[0], info.dapCmd[1:]...)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot start %s: %w", info.dapCmd[0], err)
	}
	return c, req.launch, "", nil
}

// dapLocalsAutoExpandDepth returns the locals auto-expand depth configured with
// (setq debug-locals-auto-expand-depth N), defaulting to 1.  It is read when a
// session starts because applyElispConfig can only refresh the value of a
// session that is already running.
func (e *Editor) dapLocalsAutoExpandDepth() int {
	if e.lisp == nil {
		return 1
	}
	if v, ok := e.lisp.GetGlobalVar("debug-locals-auto-expand-depth"); ok {
		if i, isInt := v.(elisp.Int); isInt && i.V > 0 {
			return int(i.V)
		}
	}
	return 1
}

// canonPath returns a canonical absolute path for p, resolving symlinks.  This
// matters because debug adapters (delve) canonicalize their module and DWARF
// source paths — on macOS the temp/working dirs reached via /var resolve to
// /private/var, and a mismatch breaks both the build ("outside main module")
// and breakpoint binding.  Falls back to filepath.Abs, then the original.
func canonPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// dapLaunchArgs returns the launch arguments for the current buffer, detecting
// whether it should run as a test, a main program, or a headless server.  It
// also returns the directory the adapter process should run in (the module
// root), so that delve's `go build` runs inside the target's module.
func (e *Editor) dapLaunchArgs(buf *buffer.Buffer) (dap.LaunchArgs, string, error) {
	fname := buf.Filename()
	if fname == "" {
		return nil, "", fmt.Errorf("buffer has no associated file")
	}
	abs := canonPath(fname)

	info := langModeByName(buf.Mode())
	root := ""
	if info != nil {
		root = findProjectRoot(abs, info.rootMarkers)
	}
	if root == "" {
		root = filepath.Dir(abs)
	}

	if info != nil && info.dapKind == dapAdapterJdtls {
		// jdtls resolves the main class and classpath itself, over LSP, once the
		// adapter is about to start; see dapJdtlsLaunchArgs.
		return nil, root, nil
	}

	switch {
	case strings.HasSuffix(abs, "_test.go"):
		testName := dapTestFuncAtPoint(buf)
		return dap.LaunchArgs{
			"mode":    "test",
			"program": filepath.Dir(abs), // package dir, not module root
			"args":    []string{"-test.run", testName},
		}, root, nil

	case bufContainsMainFunc(buf):
		return dap.LaunchArgs{
			"mode":    "debug",
			"program": filepath.Dir(abs),
		}, root, nil

	default:
		// Server / library: start headless.
		return dap.LaunchArgs{
			"mode":    "debug",
			"program": root,
		}, root, nil
	}
}

// goTestFuncRe matches a top-level Go test function declaration.  Nested and
// indented declarations cannot match, which is deliberate: only a file-level
// TestXxx is a target delve's -test.run can select.
var goTestFuncRe = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

// dapTestFuncAtPoint returns the name of the test function whose body contains
// buf.Point(), for use as delve's -test.run pattern, or "." (run every test in
// the package) when point is not inside one.
//
// Point really has to be inside the function body: the spec says that with the
// cursor "on a class, or outside any function, the entire file is debugged".  So
// a cursor on a top-level var between two tests, or inside a helper declared
// after a test, must not be attributed to the test that happens to precede it.
func dapTestFuncAtPoint(buf *buffer.Buffer) string {
	content := buf.Substring(0, buf.Len())
	runes := []rune(content)
	pt := buf.Point()
	for _, m := range goTestFuncRe.FindAllStringSubmatchIndex(content, -1) {
		// Regexp offsets are byte indices; buffer positions are rune indices.
		start := utf8.RuneCountInString(content[:m[0]])
		if pt < start {
			// Declarations come in source order, so every later one begins even
			// further past point.
			break
		}
		end, ok := goBodyEnd(runes, start)
		if !ok {
			// Unbalanced braces — a half-finished edit.  Treat the declaration as
			// running to the end of the buffer rather than dropping it, so that
			// debugging the test one is in the middle of writing still works.
			end = len(runes)
		}
		if pt <= end {
			return content[m[2]:m[3]]
		}
	}
	return "."
}

// goBodyEnd returns the rune index of the brace closing the first block opened at
// or after runes[start], ignoring braces inside interpreted strings, rune
// literals, raw (backtick) strings and comments.  ok is false when no block is
// opened, or the one that is opened is never closed.
func goBodyEnd(runes []rune, start int) (end int, ok bool) {
	depth := 0
	for i := start; i < len(runes); i++ {
		switch runes[i] {
		case '/':
			i = goSkipComment(runes, i)
		case '"', '\'', '`':
			i = goSkipLiteral(runes, i)
		case '{':
			depth++
		case '}':
			depth--
			if depth <= 0 {
				return i, depth == 0
			}
		}
	}
	return 0, false
}

// goSkipComment returns the index of the last rune of the comment starting at
// runes[i], which must be a '/'.  When it does not start a comment (ordinary
// division) i is returned unchanged.  An unterminated comment runs to the end.
func goSkipComment(runes []rune, i int) int {
	if i+1 >= len(runes) {
		return i
	}
	switch runes[i+1] {
	case '/':
		for j := i + 2; j < len(runes); j++ {
			if runes[j] == '\n' {
				return j - 1
			}
		}
	case '*':
		for j := i + 2; j+1 < len(runes); j++ {
			if runes[j] == '*' && runes[j+1] == '/' {
				return j + 1
			}
		}
	default:
		return i
	}
	return len(runes) - 1
}

// goSkipLiteral returns the index of the rune closing the literal opened by the
// quote, apostrophe or backtick at runes[i].  Backslash escapes are honoured
// except in raw strings, and an unterminated interpreted literal ends at the
// line break, just as it does for the compiler.
func goSkipLiteral(runes []rune, i int) int {
	quote := runes[i]
	raw := quote == '`'
	for j := i + 1; j < len(runes); j++ {
		switch runes[j] {
		case '\\':
			if !raw {
				j++ // skip the escaped rune
			}
		case '\n':
			if !raw {
				return j - 1
			}
		case quote:
			return j
		}
	}
	return len(runes) - 1
}

// javaMainRe matches a Java entry point declaration.  It deliberately covers the
// forms that occur in the wild: any order of the modifiers (and "final"), the
// array brackets on either the type or the parameter name, varargs, a qualified
// java.lang.String, a missing parameter name, and — since JEP 512 — an instance
// main method with no parameters at all.  The modifier list may not span lines,
// which keeps unrelated "static" tokens elsewhere in the file from matching.
var javaMainRe = regexp.MustCompile(
	`(?m)^[ \t]*(?:(?:public|protected|private|static|final|synchronized|strictfp)[ \t]+)*` +
		`void[ \t]+main[ \t]*\(` +
		`[ \t]*(?:(?:final[ \t]+)?(?:java\.lang\.)?String[ \t]*(?:\[[ \t]*\]|\.\.\.)?[ \t]*` +
		`(?:\w+[ \t]*(?:\[[ \t]*\])?)?[ \t]*)?\)`)

// bufContainsMainFunc reports whether the buffer declares a program entry point:
// a Go "func main()" or a Java main method.  Both patterns are checked
// regardless of the buffer's mode — they cannot match the other language — so
// detection still works when the mode was set by hand.
func bufContainsMainFunc(buf *buffer.Buffer) bool {
	content := buf.Substring(0, buf.Len())
	return strings.Contains(content, "func main()") || javaMainRe.MatchString(content)
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

// dapMaxThreads caps how many threads the call-stack panel fetches frames for,
// and dapStackLevels caps the frames per thread, so that a debuggee with
// hundreds of goroutines does not flood the adapter with requests on every stop.
const (
	dapMaxThreads  = 8
	dapStackLevels = 20
)

// dapFetchFrames requests up to dapStackLevels stack frames for one thread.
func dapFetchFrames(client *dap.Client, threadID int) ([]dap.StackFrame, error) {
	raw, err := client.Request("stackTrace", dap.StackTraceArgs{
		ThreadID: threadID,
		Levels:   dapStackLevels,
	})
	if err != nil {
		return nil, err
	}
	var resp dap.StackTraceResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.StackFrames, nil
}

// dapFetchThreads returns one dapThread per thread the adapter knows about, the
// stopped thread (whose frames the caller already has) first.  At most
// dapMaxThreads threads are queried.  Adapters that do not implement the threads
// request still yield the stopped thread on its own.
func dapFetchThreads(client *dap.Client, stoppedID int, stoppedFrames []dap.StackFrame) []dapThread {
	threads := []dapThread{{id: stoppedID, stopped: true, frames: stoppedFrames}}
	raw, err := client.Request("threads", nil)
	if err != nil {
		return threads
	}
	var resp dap.ThreadsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return threads
	}
	for _, t := range resp.Threads {
		if t.ID == stoppedID {
			threads[0].name = t.Name
			continue
		}
		if len(threads) >= dapMaxThreads {
			break
		}
		frames, ferr := dapFetchFrames(client, t.ID)
		if ferr != nil {
			continue
		}
		threads = append(threads, dapThread{id: t.ID, name: t.Name, frames: frames})
	}
	return threads
}

// dapFetchStoppedInfo fetches the stack traces of all threads and the locals of
// the stopped thread's top frame after a stopped event.  It is a no-op if the
// client is not yet available (race with setup callback).
func (e *Editor) dapFetchStoppedInfo(ev dap.StoppedEvent) {
	client := e.dap.client
	if client == nil {
		// Client not ready yet — setup callback will call this after setting client.
		return
	}
	threadID := ev.ThreadID
	maxDepth := max(e.dap.localsAutoExpandDepth, 1)
	e.dapAsync(func() func() {
		frames, err := dapFetchFrames(client, threadID)
		if err != nil {
			return nil
		}
		threads := dapFetchThreads(client, threadID, frames)

		var localVars []dapVariable
		if len(frames) > 0 {
			localVars = dapFetchLocals(client, frames[0].ID, maxDepth)
		}

		return func() {
			if e.dap == nil {
				return
			}
			e.dap.framesMu.Lock()
			e.dap.frames = frames
			e.dap.threads = threads
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
					// If we stepped into a different file, load it.  Compare
					// canonical paths so the same file under a different spelling
					// (e.g. /var vs /private/var) is not reloaded into a dup buffer.
					if top.Source.Path != "" && canonPath(win.Buf().Filename()) != canonPath(top.Source.Path) {
						if fileBuf, err := e.openFileIntoBuffer(top.Source.Path); err == nil {
							win.SetBuf(fileBuf)
							e.debugMarkSourceReadOnly(fileBuf)
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
	// The mode-specific dispatch in dispatchParsedKey matches "debug-repl"
	// exactly, so a language-suffixed REPL mode ("debug-repl+java") arrives here
	// instead.  Hand those keys to the REPL rather than treating them as
	// single-letter source shortcuts.
	if strings.HasPrefix(e.ActiveBuffer().Mode(), debugReplMode) {
		return e.debugReplDispatch(ke)
	}
	if ke.Key != tcell.KeyRune || ke.Mod != 0 {
		return false
	}
	// A source file the user opened mid-session has not been through
	// debugMarkSourceReadOnly.  Adopt it here, before the key can reach
	// self-insert, so that even the first keystroke in it drives the debugger
	// rather than editing a buffer the spec says is read-only while debugging.
	// (loadFile adopts it at open time; this is the backstop for every other way
	// a buffer can become the active one.)
	e.debugAdoptSourceBuffer(e.ActiveBuffer())
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
