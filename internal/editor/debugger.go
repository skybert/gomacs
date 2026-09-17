package editor

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/skybert/gomacs/internal/dap"
)

// debugBackend abstracts the debug adapter protocol so that editor-level
// command code is not coupled to DAP specifics.  A concrete implementation
// (dapBackend) talks the DAP wire protocol; future backends could use GDB/MI,
// LLDB, or others.
//
// All methods that send network requests execute synchronously in the caller's
// goroutine (usually an e.dapAsync worker goroutine); the Editor's dapAsync /
// dapCbs machinery is responsible for bouncing results back to the main loop.
type debugBackend interface {
	// Stepping / execution.
	Continue(threadID int) error
	StepNext(threadID int) error
	StepIn(threadID int) error
	StepOut(threadID int) error

	// Evaluate an expression.  If frameID==0 and stoppedThread!=0 the
	// implementation should fetch the top frame first.
	Evaluate(expr string, frameID, stoppedThread int, context string) (string, error)

	// Breakpoints.
	SetBreakpoints(file string, lines []int) error

	// Lifecycle — called from the main goroutine.
	Close()
}

// dapBackend implements debugBackend using the DAP wire protocol.
type dapBackend struct {
	client *dap.Client
}

func (b *dapBackend) Continue(threadID int) error {
	_, err := b.client.Request("continue", dap.ContinueArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) StepNext(threadID int) error {
	_, err := b.client.Request("next", dap.NextArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) StepIn(threadID int) error {
	_, err := b.client.Request("stepIn", dap.StepInArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) StepOut(threadID int) error {
	_, err := b.client.Request("stepOut", dap.StepOutArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) Evaluate(expr string, frameID, stoppedThread int, context string) (string, error) {
	// If we don't have a frame yet, fetch the top one first.
	if frameID == 0 && stoppedThread != 0 {
		raw, err := b.client.Request("stackTrace", dap.StackTraceArgs{
			ThreadID: stoppedThread,
			Levels:   1,
		})
		if err == nil {
			var resp dap.StackTraceResponse
			if jerr := json.Unmarshal(raw, &resp); jerr == nil && len(resp.StackFrames) > 0 {
				frameID = resp.StackFrames[0].ID
			}
		}
	}
	raw, err := b.client.Request("evaluate", dap.EvaluateArgs{
		Expression: expr,
		FrameID:    frameID,
		Context:    context,
	})
	if err != nil {
		return "", err
	}
	var resp dap.EvaluateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	return resp.Result, nil
}

func (b *dapBackend) SetBreakpoints(file string, lines []int) error {
	bps := make([]dap.SourceBreakpoint, len(lines))
	for i, l := range lines {
		bps[i] = dap.SourceBreakpoint{Line: l}
	}
	_, err := b.client.Request("setBreakpoints", dap.SetBreakpointsArgs{
		Source:      dap.Source{Path: file},
		Breakpoints: bps,
	})
	return err
}

func (b *dapBackend) Close() {
	b.client.Close()
}

// ---- jdtls (java-mode) adapter ---------------------------------------------

// eclipse.jdt.ls is a language server, not a debug adapter: java debugging is
// provided by the java-debug plugin (microsoft/java-debug), which jdtls loads as
// an Eclipse bundle.  A client therefore cannot spawn a java debug adapter the
// way it spawns "dlv dap".  Instead it asks the running language server to
// execute "vscode.java.startDebugSession"; the server creates an adapter and
// answers with the TCP port it listens on, and DAP traffic then flows over that
// socket.  The launch arguments (main class, classpath) are resolved through two
// further LSP commands.
const (
	jdtlsStartDebugSession = "vscode.java.startDebugSession"
	jdtlsResolveMainClass  = "vscode.java.resolveMainClass"
	jdtlsResolveClasspath  = "vscode.java.resolveClasspath"

	// jdtlsSetupHint is appended to every java debug failure: none of the
	// commands above exist unless the java-debug plugin jar was handed to jdtls
	// in initializationOptions.bundles when the server was started.
	jdtlsSetupHint = "java-mode debugging needs a running jdtls started with the " +
		"java-debug plugin (com.microsoft.java.debug.plugin-*.jar) in " +
		"initializationOptions.bundles"
)

// dapStartJdtls asks the jdtls server behind conn to start a java-debug adapter
// and returns a DAP client connected to the port it reports.
func dapStartJdtls(conn *lspConn) (*dap.Client, error) {
	raw, err := jdtlsExecuteCommand(conn, jdtlsStartDebugSession)
	if err != nil {
		return nil, err
	}
	port, err := jdtlsParsePort(raw)
	if err != nil {
		return nil, err
	}
	netConn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("connecting to java-debug adapter on port %d: %w", port, err)
	}
	return dap.NewConnClient(netConn), nil
}

// jdtlsExecuteCommand runs an LSP workspace/executeCommand on conn, which must
// be a ready language-server connection.
func jdtlsExecuteCommand(conn *lspConn, command string, args ...any) (json.RawMessage, error) {
	if conn == nil || conn.client == nil || !conn.isReady {
		return nil, fmt.Errorf("no ready jdtls language server: %s", jdtlsSetupHint)
	}
	if args == nil {
		args = []any{}
	}
	raw, err := conn.client.Call("workspace/executeCommand", map[string]any{
		"command":   command,
		"arguments": args,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w (%s)", command, err, jdtlsSetupHint)
	}
	return raw, nil
}

// jdtlsParsePort reads the TCP port out of a vscode.java.startDebugSession
// reply.  jdtls answers with a bare JSON number; a quoted number is accepted
// too so that a stringly-typed delegate handler still works.
func jdtlsParsePort(raw json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err == nil && n > 0 {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if n, convErr := strconv.Atoi(s); convErr == nil && n > 0 {
			return n, nil
		}
	}
	return 0, fmt.Errorf("%s returned %s, want a TCP port number (%s)",
		jdtlsStartDebugSession, string(raw), jdtlsSetupHint)
}

// jdtlsMainClass is one entry of a vscode.java.resolveMainClass reply.
type jdtlsMainClass struct {
	MainClass   string `json:"mainClass"`
	ProjectName string `json:"projectName"`
	FilePath    string `json:"filePath"`
}

// dapJdtlsLaunchArgs resolves the launch arguments for a java debug session:
// the main class (preferring the one declared in file) and its classpath.
func dapJdtlsLaunchArgs(conn *lspConn, file, root string) (dap.LaunchArgs, error) {
	raw, err := jdtlsExecuteCommand(conn, jdtlsResolveMainClass, root)
	if err != nil {
		return nil, err
	}
	var classes []jdtlsMainClass
	if err := json.Unmarshal(raw, &classes); err != nil {
		return nil, fmt.Errorf("%s: %w", jdtlsResolveMainClass, err)
	}
	if len(classes) == 0 {
		return nil, fmt.Errorf("%s found no main class under %s", jdtlsResolveMainClass, root)
	}
	main := jdtlsPickMainClass(classes, file)

	modulePaths, classPaths, err := dapJdtlsClasspath(conn, main)
	if err != nil {
		return nil, err
	}

	return dap.LaunchArgs{
		"type":        "java",
		"request":     "launch",
		"mainClass":   main.MainClass,
		"projectName": main.ProjectName,
		"modulePaths": modulePaths,
		"classPaths":  classPaths,
		"cwd":         root,
		"console":     "internalConsole",
	}, nil
}

// jdtlsPickMainClass returns the main class declared in file, falling back to
// the first one the language server reported.
func jdtlsPickMainClass(classes []jdtlsMainClass, file string) jdtlsMainClass {
	for _, c := range classes {
		if c.FilePath != "" && canonPath(c.FilePath) == canonPath(file) {
			return c
		}
	}
	return classes[0]
}

// dapJdtlsClasspath resolves the module and class paths for a main class.
// vscode.java.resolveClasspath answers with a two-element array of string
// arrays: [modulepaths, classpaths].
func dapJdtlsClasspath(conn *lspConn, main jdtlsMainClass) (modulePaths, classPaths []string, err error) {
	raw, err := jdtlsExecuteCommand(conn, jdtlsResolveClasspath, main.MainClass, main.ProjectName)
	if err != nil {
		return nil, nil, err
	}
	var paths [][]string
	if err := json.Unmarshal(raw, &paths); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", jdtlsResolveClasspath, err)
	}
	if len(paths) > 0 {
		modulePaths = paths[0]
	}
	if len(paths) > 1 {
		classPaths = paths[1]
	}
	if len(modulePaths) == 0 && len(classPaths) == 0 {
		return nil, nil, fmt.Errorf("%s returned an empty classpath for %s",
			jdtlsResolveClasspath, main.MainClass)
	}
	return modulePaths, classPaths, nil
}
