package editor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/lsp"
)

// dapFakeServer accepts one connection, then replies success to every request
// with the given per-command body (or an empty object).  It runs until the
// connection closes.
func dapFakeServer(t *testing.T, bodies map[string]any) (*dap.Client, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		ln.Close() //nolint:errcheck
		t.Fatal(err)
	}
	srv, err := ln.Accept()
	ln.Close() //nolint:errcheck
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		reader := srv
		buf := make([]byte, 65536)
		var acc []byte
		for {
			n, rerr := reader.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
				for {
					hdrEnd := indexHeaderEnd(acc)
					if hdrEnd < 0 {
						break
					}
					clen := parseContentLength(string(acc[:hdrEnd]))
					total := hdrEnd + 4 + clen
					if len(acc) < total {
						break
					}
					body := acc[hdrEnd+4 : total]
					acc = acc[total:]
					var msg dap.Message
					_ = json.Unmarshal(body, &msg)
					respBody, ok := bodies[msg.Command]
					if !ok {
						respBody = map[string]any{}
					}
					resp := map[string]any{
						"seq": 1, "type": "response",
						"request_seq": msg.Seq, "command": msg.Command,
						"success": true, "body": respBody,
					}
					rb, _ := json.Marshal(resp)
					_, _ = fmt.Fprintf(srv, "Content-Length: %d\r\n\r\n%s", len(rb), rb)
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	c := dap.NewConnClient(clientConn)
	return c, func() { c.Close(); srv.Close() } //nolint:errcheck
}

func indexHeaderEnd(b []byte) int {
	return strings.Index(string(b), "\r\n\r\n")
}

func parseContentLength(header string) int {
	var n int
	if i := strings.Index(header, "Content-Length: "); i >= 0 {
		_, _ = fmt.Sscanf(header[i:], "Content-Length: %d", &n)
	}
	return n
}

func TestDapBackend_StepCommands(t *testing.T) {
	c, cleanup := dapFakeServer(t, nil)
	defer cleanup()
	b := &dapBackend{client: c}

	if err := b.Continue(1); err != nil {
		t.Errorf("Continue: %v", err)
	}
	if err := b.StepNext(1); err != nil {
		t.Errorf("StepNext: %v", err)
	}
	if err := b.StepIn(1); err != nil {
		t.Errorf("StepIn: %v", err)
	}
	if err := b.StepOut(1); err != nil {
		t.Errorf("StepOut: %v", err)
	}
	if err := b.SetBreakpoints("/tmp/x.go", []int{3, 7}); err != nil {
		t.Errorf("SetBreakpoints: %v", err)
	}
}

func TestDapBackend_Evaluate(t *testing.T) {
	c, cleanup := dapFakeServer(t, map[string]any{
		"stackTrace": map[string]any{
			"stackFrames": []map[string]any{{"id": 42, "name": "main", "line": 5}},
		},
		"evaluate": map[string]any{"result": "41"},
	})
	defer cleanup()
	b := &dapBackend{client: c}

	// frameID 0 + stoppedThread != 0 triggers the stackTrace lookup first.
	got, err := b.Evaluate("x", 0, 1, "hover")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got != "41" {
		t.Errorf("Evaluate result = %q, want \"41\"", got)
	}
}

func TestDapBackend_Close(t *testing.T) {
	c, _ := dapFakeServer(t, nil)
	b := &dapBackend{client: c}
	done := make(chan struct{})
	go func() { b.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
}

func TestDapFetchLocals_And_Vars(t *testing.T) {
	c, cleanup := dapFakeServer(t, map[string]any{
		"scopes": map[string]any{"scopes": []map[string]any{
			{"name": "Locals", "variablesReference": 10},
		}},
		"variables": map[string]any{"variables": []map[string]any{
			{"name": "x", "value": "1", "variablesReference": 0},
			{"name": "obj", "value": "{...}", "variablesReference": 11},
		}},
	})
	defer cleanup()

	vars := dapFetchLocals(c, 1, 1)
	if len(vars) != 2 {
		t.Fatalf("expected 2 locals, got %d", len(vars))
	}
	if vars[0].name != "x" {
		t.Errorf("first local = %q, want x", vars[0].name)
	}
	// "obj" has a non-zero ref and depth(0) < maxDepth(1) → children fetched.
	if !vars[1].expanded || len(vars[1].children) == 0 {
		t.Errorf("expandable var should have children fetched: %+v", vars[1])
	}
}

func TestDapFetchVars_ZeroRef(t *testing.T) {
	c, cleanup := dapFakeServer(t, nil)
	defer cleanup()
	if got := dapFetchVars(c, 0, 0, 1); got != nil {
		t.Errorf("ref 0 should return nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// jdtls (java-mode) adapter
// ---------------------------------------------------------------------------

// fakeJdtlsServer wires a ready lspConn to an in-process language server that
// answers workspace/executeCommand requests from replies, keyed by the command
// name in the request's arguments.  A command with no entry fails, which is what
// a jdtls started without the java-debug bundle does.
func fakeJdtlsServer(t *testing.T, replies map[string]any) (*lspConn, func()) {
	t.Helper()
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		br := bufio.NewReader(r1)
		for {
			clen := 0
			for {
				line, e := br.ReadString('\n')
				if e != nil {
					return
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
					_, _ = fmt.Sscanf(v, "%d", &clen)
				}
			}
			if clen == 0 {
				continue
			}
			body := make([]byte, clen)
			if _, e := io.ReadFull(br, body); e != nil {
				return
			}
			var req struct {
				ID     *int   `json:"id"`
				Method string `json:"method"`
				Params struct {
					Command string `json:"command"`
				} `json:"params"`
			}
			_ = json.Unmarshal(body, &req)
			if req.ID == nil {
				continue
			}
			resp := map[string]any{"jsonrpc": "2.0", "id": *req.ID}
			if result, ok := replies[req.Params.Command]; ok {
				resp["result"] = result
			} else {
				resp["error"] = map[string]any{
					"code":    -32601,
					"message": "No delegateCommandHandler for " + req.Params.Command,
				}
			}
			rb, _ := json.Marshal(resp)
			_, _ = fmt.Fprintf(w2, "Content-Length: %d\r\n\r\n%s", len(rb), rb)
		}
	}()

	conn := &lspConn{client: lsp.NewConnClient(w1, r2), isReady: true}
	return conn, func() { conn.client.Close(); _ = r1.Close(); _ = w2.Close() }
}

// TestJdtlsConnError checks that each way of not having a usable java language
// server produces its own actionable message, rather than all of them collapsing
// into jdtlsExecuteCommand's generic "no ready jdtls language server".
func TestJdtlsConnError(t *testing.T) {
	info := *langModeByName("java")

	noCmd := info
	noCmd.lspCmd = nil
	err := jdtlsConnError(nil, &noCmd)
	if err == nil || !strings.Contains(err.Error(), "java-lsp-command") {
		t.Errorf("unconfigured server: err = %v, want it to name java-lsp-command", err)
	}

	err = jdtlsConnError(nil, &info)
	switch {
	case err == nil:
		t.Error("no connection: want an error")
	case !strings.Contains(err.Error(), "not running"):
		t.Errorf("no connection: err = %v, want it to say the server is not running", err)
	case !strings.Contains(err.Error(), "jdtls"):
		t.Errorf("no connection: err = %v, want it to name the configured command", err)
	}

	if err = jdtlsConnError(&lspConn{}, &info); err == nil ||
		!strings.Contains(err.Error(), "not running") {
		t.Errorf("connection without a client: err = %v, want it to say the server is not running", err)
	}

	notReady, cleanup := fakeJdtlsServer(t, nil)
	defer cleanup()
	notReady.isReady = false
	if err = jdtlsConnError(notReady, &info); err == nil ||
		!strings.Contains(err.Error(), "initialising") {
		t.Errorf("not-ready connection: err = %v, want it to say the server is initialising", err)
	}

	notReady.isReady = true
	if err = jdtlsConnError(notReady, &info); err != nil {
		t.Errorf("ready connection: err = %v, want nil", err)
	}
}

func TestJdtlsExecuteCommand_NoConnection(t *testing.T) {
	if _, err := jdtlsExecuteCommand(nil, jdtlsStartDebugSession); err == nil {
		t.Fatal("expected an error without a connection")
	}
	notReady := &lspConn{}
	_, err := jdtlsExecuteCommand(notReady, jdtlsStartDebugSession)
	if err == nil {
		t.Fatal("expected an error for a connection that is not ready yet")
	}
	if !strings.Contains(err.Error(), "java-debug") {
		t.Errorf("error = %v, want it to name the missing plugin", err)
	}
}

func TestJdtlsExecuteCommand_MissingDelegateHandler(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, nil) // no commands registered
	defer cleanup()
	_, err := jdtlsExecuteCommand(conn, jdtlsStartDebugSession)
	if err == nil {
		t.Fatal("expected an error when jdtls has no java-debug bundle")
	}
	if !strings.Contains(err.Error(), jdtlsStartDebugSession) {
		t.Errorf("error = %v, want it to name the command", err)
	}
	if !strings.Contains(err.Error(), "initializationOptions.bundles") {
		t.Errorf("error = %v, want it to explain how to fix the setup", err)
	}
}

func TestJdtlsParsePort(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{`5005`, 5005, false},
		{`"5005"`, 5005, false},
		{`0`, 0, true},
		{`"not a port"`, 0, true},
		{`null`, 0, true},
		{`{"port":5005}`, 0, true},
	}
	for _, tt := range tests {
		got, err := jdtlsParsePort([]byte(tt.raw))
		if tt.wantErr {
			if err == nil {
				t.Errorf("jdtlsParsePort(%s) = %d, want an error", tt.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("jdtlsParsePort(%s): %v", tt.raw, err)
		}
		if got != tt.want {
			t.Errorf("jdtlsParsePort(%s) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestDapStartJdtls_ConnectsToReportedPort(t *testing.T) {
	// Stand in for the java-debug adapter jdtls would have created.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr == nil {
			accepted <- c
		}
	}()

	conn, cleanup := fakeJdtlsServer(t, map[string]any{jdtlsStartDebugSession: port})
	defer cleanup()

	client, err := dapStartJdtls(conn)
	if err != nil {
		t.Fatalf("dapStartJdtls: %v", err)
	}
	defer client.Close()

	select {
	case c := <-accepted:
		c.Close() //nolint:errcheck
	case <-time.After(5 * time.Second):
		t.Fatal("dapStartJdtls did not connect to the reported port")
	}
}

func TestDapStartJdtls_BadPort(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{jdtlsStartDebugSession: "nonsense"})
	defer cleanup()
	if _, err := dapStartJdtls(conn); err == nil {
		t.Fatal("expected an error for a non-numeric port")
	}
}

func TestJdtlsPickMainClass(t *testing.T) {
	classes := []jdtlsMainClass{
		{MainClass: "com.example.Other", ProjectName: "p", FilePath: "/src/Other.java"},
		{MainClass: "com.example.Main", ProjectName: "p", FilePath: "/src/Main.java"},
	}
	if got := jdtlsPickMainClass(classes, "/src/Main.java"); got.MainClass != "com.example.Main" {
		t.Errorf("picked %q, want the class declared in the buffer's file", got.MainClass)
	}
	// Unknown file → first entry.
	if got := jdtlsPickMainClass(classes, "/src/Nope.java"); got.MainClass != "com.example.Other" {
		t.Errorf("picked %q, want the first reported class as fallback", got.MainClass)
	}
	// Entries without a file path never match by path.
	noPath := []jdtlsMainClass{{MainClass: "A"}, {MainClass: "B"}}
	if got := jdtlsPickMainClass(noPath, "/src/A.java"); got.MainClass != "A" {
		t.Errorf("picked %q, want the first entry", got.MainClass)
	}
}

func TestDapJdtlsClasspath(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveClasspath: []any{[]string{"/mods/a.jar"}, []string{"/target/classes", "/libs/b.jar"}},
	})
	defer cleanup()

	modulePaths, classPaths, err := dapJdtlsClasspath(conn, jdtlsMainClass{MainClass: "com.example.Main", ProjectName: "p"})
	if err != nil {
		t.Fatalf("dapJdtlsClasspath: %v", err)
	}
	if len(modulePaths) != 1 || modulePaths[0] != "/mods/a.jar" {
		t.Errorf("modulePaths = %v, want the first array of the reply", modulePaths)
	}
	if len(classPaths) != 2 || classPaths[0] != "/target/classes" {
		t.Errorf("classPaths = %v, want the second array of the reply", classPaths)
	}
}

func TestDapJdtlsClasspath_Empty(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveClasspath: []any{[]string{}, []string{}},
	})
	defer cleanup()
	if _, _, err := dapJdtlsClasspath(conn, jdtlsMainClass{MainClass: "M"}); err == nil {
		t.Fatal("expected an error for an empty classpath")
	}
}

func TestDapJdtlsLaunchArgs(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: []any{
			map[string]any{"mainClass": "com.example.Main", "projectName": "demo", "filePath": "/src/Main.java"},
		},
		jdtlsResolveClasspath: []any{[]string{}, []string{"/target/classes"}},
	})
	defer cleanup()

	args, note, err := dapJdtlsLaunchArgs(conn, "/src/Main.java", "/src")
	if err != nil {
		t.Fatalf("dapJdtlsLaunchArgs: %v", err)
	}
	if args["mainClass"] != "com.example.Main" {
		t.Errorf("mainClass = %v", args["mainClass"])
	}
	if args["projectName"] != "demo" {
		t.Errorf("projectName = %v", args["projectName"])
	}
	if args["request"] != "launch" || args["type"] != "java" {
		t.Errorf("args = %v, want a java launch request", args)
	}
	paths, ok := args["classPaths"].([]string)
	if !ok || len(paths) != 1 || paths[0] != "/target/classes" {
		t.Errorf("classPaths = %v", args["classPaths"])
	}
	if args["cwd"] != "/src" {
		t.Errorf("cwd = %v, want the project root", args["cwd"])
	}
	if _, hasArgs := args["args"]; hasArgs {
		t.Errorf("args = %v, want no program arguments for a plain main class", args["args"])
	}
	if note != "" {
		t.Errorf("note = %q, want none when a main class was resolved", note)
	}
}

// writeJavaFile writes src to <dir>/<rel> and returns the path.
func writeJavaFile(t *testing.T, dir, rel, src string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDapJdtlsLaunchArgs_NoMainClassFallsBackToTestClass covers the spec's test
// and "micro server" contexts for java-mode: a project whose classes have no
// main() used to dead-end in "found no main class under …", so a test class or a
// service started by its build plugin could not be debugged at all.  When a JUnit
// runner is on the classpath jdtls resolved, the test class is run through it,
// which is what lets breakpoints inside the test hit.
func TestDapJdtlsLaunchArgs_NoMainClassFallsBackToTestClass(t *testing.T) {
	dir := t.TempDir()
	path := writeJavaFile(t, dir, "src/test/java/com/example/FooTest.java",
		"package com.example;\n\nimport org.junit.jupiter.api.Test;\n\n"+
			"class FooTest {\n  @Test\n  void works() {}\n}\n")

	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: []any{},
		jdtlsResolveClasspath: []any{[]string{}, []string{
			"/target/test-classes",
			"/home/u/.m2/junit-platform-console-standalone-1.10.2.jar",
		}},
	})
	defer cleanup()

	args, note, err := dapJdtlsLaunchArgs(conn, path, dir)
	if err != nil {
		t.Fatalf("dapJdtlsLaunchArgs should fall back, not fail: %v", err)
	}
	if args["mainClass"] != "org.junit.platform.console.ConsoleLauncher" {
		t.Errorf("mainClass = %v, want the JUnit console launcher", args["mainClass"])
	}
	progArgs, ok := args["args"].([]string)
	if !ok || len(progArgs) != 1 || progArgs[0] != "--select-class=com.example.FooTest" {
		t.Errorf("args = %v, want the test class selected", args["args"])
	}
	if args["request"] != "launch" || args["type"] != "java" {
		t.Errorf("args = %v, want a java launch request", args)
	}
	if args["cwd"] != dir {
		t.Errorf("cwd = %v, want the project root %q", args["cwd"], dir)
	}
	if !strings.Contains(note, "com.example.FooTest") {
		t.Errorf("note = %q, want it to name the class being debugged", note)
	}
}

func TestDapJdtlsLaunchArgs_NoMainClassFallsBackToOwnClass(t *testing.T) {
	dir := t.TempDir()
	path := writeJavaFile(t, dir, "src/main/java/com/example/Service.java",
		"package com.example;\n\nclass Service {\n  void run() {}\n}\n")

	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: []any{},
		jdtlsResolveClasspath: []any{[]string{"/mods/a.jar"}, []string{"/target/classes"}},
	})
	defer cleanup()

	args, note, err := dapJdtlsLaunchArgs(conn, path, dir)
	if err != nil {
		t.Fatalf("dapJdtlsLaunchArgs should fall back, not fail: %v", err)
	}
	if args["mainClass"] != "com.example.Service" {
		t.Errorf("mainClass = %v, want the buffer's own class", args["mainClass"])
	}
	if _, hasArgs := args["args"]; hasArgs {
		t.Errorf("args = %v, want none for a plain class launch", args["args"])
	}
	mods, ok := args["modulePaths"].([]string)
	if !ok || len(mods) != 1 || mods[0] != "/mods/a.jar" {
		t.Errorf("modulePaths = %v, want the resolved module path", args["modulePaths"])
	}
	if !strings.Contains(note, "main method") {
		t.Errorf("note = %q, want it to say what to do about a missing main method", note)
	}
}

func TestDapJdtlsLaunchArgs_TestWithoutRunnerOnClasspath(t *testing.T) {
	dir := t.TempDir()
	path := writeJavaFile(t, dir, "src/test/java/com/example/BareTest.java",
		"package com.example;\n\nclass BareTest {\n  void works() {}\n}\n")

	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: []any{},
		jdtlsResolveClasspath: []any{[]string{}, []string{"/target/test-classes"}},
	})
	defer cleanup()

	args, note, err := dapJdtlsLaunchArgs(conn, path, dir)
	if err != nil {
		t.Fatalf("dapJdtlsLaunchArgs should fall back, not fail: %v", err)
	}
	if args["mainClass"] != "com.example.BareTest" {
		t.Errorf("mainClass = %v, want the test class itself when no runner is available", args["mainClass"])
	}
	if !strings.Contains(note, "junit-platform-console-standalone") {
		t.Errorf("note = %q, want it to name the jar the user should add", note)
	}
}

func TestDapJdtlsLaunchArgs_FallbackClasspathFailureIsActionable(t *testing.T) {
	dir := t.TempDir()
	path := writeJavaFile(t, dir, "Lib.java", "class Lib {}\n")

	// resolveMainClass answers "nothing here" and resolveClasspath is unavailable,
	// so there is genuinely nothing to launch — but the message has to say what to
	// do about it instead of dead-ending.
	conn, cleanup := fakeJdtlsServer(t, map[string]any{jdtlsResolveMainClass: []any{}})
	defer cleanup()

	_, _, err := dapJdtlsLaunchArgs(conn, path, dir)
	if err == nil {
		t.Fatal("expected an error when no classpath can be resolved either")
	}
	for _, want := range []string{"no main class", "Lib", "main method", "jdtls"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func TestDapJdtlsLaunchArgs_FallbackNonJavaFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("not java\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, cleanup := fakeJdtlsServer(t, map[string]any{jdtlsResolveMainClass: []any{}})
	defer cleanup()

	_, _, err := dapJdtlsLaunchArgs(conn, path, dir)
	if err == nil {
		t.Fatal("expected an error for a buffer that declares no java class")
	}
	if !strings.Contains(err.Error(), "notes.txt") {
		t.Errorf("error = %v, want it to name the file", err)
	}
}

func TestJavaReadCompilationUnit(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name      string
		rel       string
		src       string
		wantClass string
		wantTest  bool
	}{
		{"packaged class", "a/Service.java", "package com.example.svc;\n\nclass Service {}\n",
			"com.example.svc.Service", false},
		{"default package", "b/Main2.java", "class Main2 {}\n", "Main2", false},
		{"junit 5 annotation", "c/Checks.java",
			"package p;\n\nclass Checks {\n  @Test\n  void t() {}\n}\n", "p.Checks", true},
		{"qualified annotation", "d/Checks2.java",
			"package p;\n\nclass Checks2 {\n  @org.junit.jupiter.api.Test\n  void t() {}\n}\n", "p.Checks2", true},
		{"parameterized test", "e/Checks3.java",
			"package p;\n\nclass Checks3 {\n  @ParameterizedTest\n  void t() {}\n}\n", "p.Checks3", true},
		{"junit 3 base class", "f/Old.java",
			"package p;\n\nclass Old extends TestCase {\n}\n", "p.Old", true},
		{"name convention", "g/ServiceTest.java", "package p;\n\nclass ServiceTest {}\n", "p.ServiceTest", true},
		{"maven test root", "src/test/java/p/Weird.java", "package p;\n\nclass Weird {}\n", "p.Weird", true},
		{"annotation in a comment only", "h/Doc.java",
			"package p;\n\n// see @Test for details\nclass Doc {}\n", "p.Doc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeJavaFile(t, dir, tt.rel, tt.src)
			unit, err := javaReadCompilationUnit(path)
			if err != nil {
				t.Fatalf("javaReadCompilationUnit: %v", err)
			}
			if unit.class != tt.wantClass {
				t.Errorf("class = %q, want %q", unit.class, tt.wantClass)
			}
			if unit.isTest != tt.wantTest {
				t.Errorf("isTest = %v, want %v", unit.isTest, tt.wantTest)
			}
		})
	}
}

func TestJavaReadCompilationUnit_Errors(t *testing.T) {
	if _, err := javaReadCompilationUnit("/tmp/notes.txt"); err == nil {
		t.Error("a non-java file should not yield a class to launch")
	}
	missing := filepath.Join(t.TempDir(), "Gone.java")
	if _, err := javaReadCompilationUnit(missing); err == nil {
		t.Error("an unreadable file should be reported")
	}
}

func TestJavaTestRunner(t *testing.T) {
	tests := []struct {
		name      string
		classPath []string
		wantMain  string
		wantArg   string
	}{
		{"junit 5 console standalone",
			[]string{"/cp/junit-platform-console-standalone-1.10.2.jar"},
			"org.junit.platform.console.ConsoleLauncher", "--select-class=p.FooTest"},
		{"junit 5 console",
			[]string{"/cp/junit-platform-console-1.10.2.jar", "/cp/junit-jupiter-api-5.10.2.jar"},
			"org.junit.platform.console.ConsoleLauncher", "--select-class=p.FooTest"},
		{"junit 4 versioned jar",
			[]string{"/cp/junit-4.13.2.jar", "/cp/hamcrest-core-1.3.jar"},
			"org.junit.runner.JUnitCore", "p.FooTest"},
		{"junit 4 plain jar",
			[]string{"/cp/lib/junit.jar"},
			"org.junit.runner.JUnitCore", "p.FooTest"},
		{"console launcher wins over junit 4",
			[]string{"/cp/junit-4.13.2.jar", "/cp/junit-platform-console-standalone-1.10.2.jar"},
			"org.junit.platform.console.ConsoleLauncher", "--select-class=p.FooTest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			main, args, ok := javaTestRunner("p.FooTest", tt.classPath)
			if !ok {
				t.Fatalf("no runner found on %v", tt.classPath)
			}
			if main != tt.wantMain {
				t.Errorf("mainClass = %q, want %q", main, tt.wantMain)
			}
			if len(args) != 1 || args[0] != tt.wantArg {
				t.Errorf("args = %v, want [%q]", args, tt.wantArg)
			}
		})
	}

	// Only the engine, with nothing that can be launched from a command line.
	if _, _, ok := javaTestRunner("p.FooTest", []string{"/cp/junit-jupiter-api-5.10.2.jar"}); ok {
		t.Error("junit-jupiter-api alone provides no launchable runner")
	}
	if _, _, ok := javaTestRunner("p.FooTest", nil); ok {
		t.Error("an empty classpath provides no runner")
	}
}

func TestDapJdtlsLaunchArgs_MalformedReply(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: "not an array",
	})
	defer cleanup()
	if _, _, err := dapJdtlsLaunchArgs(conn, "/src/Main.java", "/src"); err == nil {
		t.Fatal("expected an error for a malformed resolveMainClass reply")
	}
}
