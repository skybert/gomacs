package editor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
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

	args, err := dapJdtlsLaunchArgs(conn, "/src/Main.java", "/src")
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
}

func TestDapJdtlsLaunchArgs_NoMainClass(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: []any{},
	})
	defer cleanup()
	_, err := dapJdtlsLaunchArgs(conn, "/src/Main.java", "/src")
	if err == nil {
		t.Fatal("expected an error when no main class is found")
	}
	if !strings.Contains(err.Error(), "no main class") {
		t.Errorf("error = %v, want it to say no main class was found", err)
	}
}

func TestDapJdtlsLaunchArgs_MalformedReply(t *testing.T) {
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: "not an array",
	})
	defer cleanup()
	if _, err := dapJdtlsLaunchArgs(conn, "/src/Main.java", "/src"); err == nil {
		t.Fatal("expected an error for a malformed resolveMainClass reply")
	}
}
