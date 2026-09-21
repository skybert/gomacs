package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	// in initializationOptions.bundles when the server was started.  gomacs
	// starts jdtls itself (see the java entry in langModes), so the hint also
	// names the variable that points it at the right launcher.
	jdtlsSetupHint = "java-mode debugging needs a running jdtls started with the " +
		"java-debug plugin (com.microsoft.java.debug.plugin-*.jar) in " +
		"initializationOptions.bundles; set the launcher with " +
		`(setq java-lsp-command "…")`
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

// jdtlsConnError explains why conn cannot be asked to start a debug session, or
// returns nil when it can.  cmdDebugStart checks this before the handshake so
// that the three things the user has to get right — a language-server command,
// that server actually running, and it having finished initialising — each
// produce their own message.  Without it every one of them surfaces as
// jdtlsExecuteCommand's single "no ready jdtls language server", which does not
// say what to do about it.
func jdtlsConnError(conn *lspConn, info *langModeInfo) error {
	varName := lspCommandVarName(info.modeName)
	switch {
	case len(info.lspCmd) == 0:
		return fmt.Errorf("no language server configured for %s-mode; set one with "+
			`(setq %s "jdtls"). %s`, info.modeName, varName, jdtlsSetupHint)
	case conn == nil || conn.client == nil:
		return fmt.Errorf("the %s-mode language server (%s) is not running; check it is "+
			`on PATH, or change it with (setq %s "…"). %s`,
			info.modeName, strings.Join(info.lspCmd, " "), varName, jdtlsSetupHint)
	case !conn.isReady:
		return fmt.Errorf("the %s-mode language server (%s) is still initialising; "+
			"try again in a moment", info.modeName, info.lspCmd[0])
	}
	return nil
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
// the main class (preferring the one declared in file) and its classpath.  A
// project with no main class at all is not an error — see
// dapJdtlsFallbackLaunchArgs — so that a test class or a file belonging to a
// server that is started some other way can still be debugged.  note is a
// message for the user, empty unless that fallback was taken.
func dapJdtlsLaunchArgs(conn *lspConn, file, root string) (launch dap.LaunchArgs, note string, err error) {
	raw, err := jdtlsExecuteCommand(conn, jdtlsResolveMainClass, root)
	if err != nil {
		return nil, "", err
	}
	var classes []jdtlsMainClass
	if err := json.Unmarshal(raw, &classes); err != nil {
		return nil, "", fmt.Errorf("%s: %w", jdtlsResolveMainClass, err)
	}
	if len(classes) == 0 {
		return dapJdtlsFallbackLaunchArgs(conn, file, root)
	}
	main := jdtlsPickMainClass(classes, file)

	modulePaths, classPaths, err := dapJdtlsClasspath(conn, main)
	if err != nil {
		return nil, "", err
	}

	return jdtlsLaunchArgs(main.MainClass, main.ProjectName, modulePaths, classPaths, root, nil), "", nil
}

// jdtlsLaunchArgs builds a java-debug "launch" request body.  args are the
// debuggee's own command-line arguments and are omitted when empty.
func jdtlsLaunchArgs(mainClass, projectName string, modulePaths, classPaths []string, root string, args []string) dap.LaunchArgs {
	launch := dap.LaunchArgs{
		"type":        "java",
		"request":     "launch",
		"mainClass":   mainClass,
		"projectName": projectName,
		"modulePaths": modulePaths,
		"classPaths":  classPaths,
		"cwd":         root,
		"console":     "internalConsole",
	}
	if len(args) > 0 {
		launch["args"] = args
	}
	return launch
}

// dapJdtlsFallbackLaunchArgs builds launch arguments for a project in which
// vscode.java.resolveMainClass found nothing to run — a library, a test module,
// or a service whose entry point lives elsewhere (Quarkus, Spring Boot started by
// its plugin, …).  Refusing to start there would leave java-mode without the
// spec's test and "micro server" contexts, which go-mode already has.
//
// The launch target is the class in the current buffer.  java-debug offers no
// "debug just this file" request and no JUnit support — that lives in the
// separate vscode-java-test bundle, which gomacs does not ship — but two things
// make this work anyway:
//
//   - vscode.java.resolveClasspath does not require its argument to declare a
//     main method; it only locates the single project containing the type, so the
//     project's real runtime classpath comes back regardless.
//   - a test class can therefore be run through whichever JUnit runner is already
//     on that classpath (see javaTestRunner), which is what makes breakpoints in
//     the test itself hit.
//
// Anything else is launched as a plain class, and the note says what to do when
// that class has no main method.
func dapJdtlsFallbackLaunchArgs(conn *lspConn, file, root string) (dap.LaunchArgs, string, error) {
	noMain := fmt.Sprintf("%s found no main class under %s", jdtlsResolveMainClass, root)

	unit, err := javaReadCompilationUnit(file)
	if err != nil {
		return nil, "", fmt.Errorf("%s, and %w", noMain, err)
	}

	// projectName is left empty: the language server reported no main class, so
	// there is no project name to copy from it, and resolveClasspath finds the
	// project from the type on its own.
	modulePaths, classPaths, err := dapJdtlsClasspath(conn, jdtlsMainClass{MainClass: unit.class})
	if err != nil {
		return nil, "", fmt.Errorf("%s, and no classpath could be resolved for %s: %w; open a "+
			"class that declares a main method, or check that jdtls has imported the project "+
			"this file belongs to", noMain, unit.class, err)
	}

	mainClass, args, why := javaFallbackTarget(unit, classPaths)
	return jdtlsLaunchArgs(mainClass, "", modulePaths, classPaths, root, args),
		noMain + "; " + why, nil
}

// javaFallbackTarget picks what dapJdtlsFallbackLaunchArgs should launch for the
// buffer's own class, and returns a note explaining the choice.
func javaFallbackTarget(unit javaCompilationUnit, classPaths []string) (mainClass string, args []string, why string) {
	if !unit.isTest {
		return unit.class, nil, fmt.Sprintf("debugging %s itself — if it declares no main method, "+
			"add one or start the debugger from the class that does", unit.class)
	}
	runner, runnerArgs, ok := javaTestRunner(unit.class, classPaths)
	if !ok {
		return unit.class, nil, fmt.Sprintf("%s looks like a test but no JUnit runner is on its "+
			"classpath, so it is launched as a plain class; add junit-platform-console-standalone "+
			"(JUnit 5) or junit (JUnit 4) to the test classpath to run it as a test", unit.class)
	}
	return runner, runnerArgs, fmt.Sprintf("debugging test class %s with %s", unit.class, runner)
}

// junitRunners maps a jar on the classpath to the JUnit entry point it provides
// and the argument that selects one test class, most capable runner first.
// Launching one of these instead of the test class is exactly what an IDE does;
// the runner only needs to be on the classpath java-debug already resolved.
var junitRunners = []struct {
	jar       string // substring of the jar's file name
	mainClass string
	selectFmt string // how the runner is told which class to run
}{
	// JUnit 5 — and JUnit 4 through the vintage engine.
	{"junit-platform-console", "org.junit.platform.console.ConsoleLauncher", "--select-class=%s"},
	// JUnit 4: JUnitCore takes bare class names.
	{"junit-4", "org.junit.runner.JUnitCore", "%s"},
	{"junit.jar", "org.junit.runner.JUnitCore", "%s"},
}

// javaTestRunner returns the JUnit runner to launch for class, given the
// classpath jdtls resolved.  ok is false when the classpath holds no runner that
// can be started from the command line.
func javaTestRunner(class string, classPaths []string) (mainClass string, args []string, ok bool) {
	for _, r := range junitRunners {
		for _, cp := range classPaths {
			if strings.Contains(filepath.Base(cp), r.jar) {
				return r.mainClass, []string{fmt.Sprintf(r.selectFmt, class)}, true
			}
		}
	}
	return "", nil, false
}

// javaCompilationUnit is what the java fallback can learn about a source file
// from the file alone.
type javaCompilationUnit struct {
	class  string // fully qualified class name
	isTest bool   // the file declares tests rather than a program
}

var (
	// javaPackageRe matches a package declaration; the group is the package name.
	javaPackageRe = regexp.MustCompile(`(?m)^[ \t]*package[ \t]+([\w.]+)[ \t]*;`)
	// javaTestAnnotationRe matches the JUnit annotations that mark a test method.
	javaTestAnnotationRe = regexp.MustCompile(
		`(?m)^[ \t]*@(?:org\.junit\.(?:jupiter\.api\.)?)?` +
			`(?:Test|ParameterizedTest|RepeatedTest|TestFactory|TestTemplate)\b`)
)

// javaReadCompilationUnit derives the fully qualified class name of a .java file
// and whether it is a test.  It reads the file from disk rather than the buffer
// because it runs on a worker goroutine, which must not touch editor state; an
// unsaved package rename is the only thing that can make the two disagree.
func javaReadCompilationUnit(file string) (javaCompilationUnit, error) {
	if !strings.EqualFold(filepath.Ext(file), ".java") {
		return javaCompilationUnit{}, fmt.Errorf("%s is not a java source file, so it declares no "+
			"class to launch", filepath.Base(file))
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return javaCompilationUnit{}, fmt.Errorf("reading %s: %w", file, err)
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	class := name
	if pkg := javaPackageRe.FindSubmatch(src); pkg != nil {
		class = string(pkg[1]) + "." + name
	}
	return javaCompilationUnit{class: class, isTest: javaLooksLikeTest(file, name, src)}, nil
}

// javaLooksLikeTest reports whether a java source file holds tests, going by the
// JUnit annotations it uses, the JUnit 3 base class, the class-name conventions
// build tools key their test detection off, and the maven/gradle test source root.
func javaLooksLikeTest(file, class string, src []byte) bool {
	if javaTestAnnotationRe.Match(src) || bytes.Contains(src, []byte("extends TestCase")) {
		return true
	}
	if strings.HasPrefix(class, "Test") || strings.HasSuffix(class, "Test") ||
		strings.HasSuffix(class, "Tests") || strings.HasSuffix(class, "TestCase") {
		return true
	}
	return strings.Contains(filepath.ToSlash(file), "/src/test/")
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
