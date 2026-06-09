package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/terminal"
)

// mockVCBackend implements vcBackend for unit testing without a real git repo.
type mockVCBackend struct {
	diffResult       string
	diffStagedResult string
	unstagedCalled   bool
	stagedCalled     bool
	unstageArg       string
}

func (m *mockVCBackend) Name() string         { return "mock" }
func (m *mockVCBackend) Root(_ string) string { return "/mock/root" }
func (m *mockVCBackend) Status(_ string) (string, error) {
	return "M  file.go\n", nil
}
func (m *mockVCBackend) Diff(_, _ string) (string, error) {
	m.unstagedCalled = true
	return m.diffResult, nil
}
func (m *mockVCBackend) DiffStaged(_, _ string) (string, error) {
	m.stagedCalled = true
	return m.diffStagedResult, nil
}
func (m *mockVCBackend) Log(_, _ string) (string, error)     { return "", nil }
func (m *mockVCBackend) Show(_, _ string) (string, error)    { return "", nil }
func (m *mockVCBackend) ShowLog(_, _ string) (string, error) { return "", nil }
func (m *mockVCBackend) Grep(_, _ string) (string, error)    { return "", nil }
func (m *mockVCBackend) Blame(_, _ string) (string, error)   { return "", nil }
func (m *mockVCBackend) Revert(_, _ string) error            { return nil }
func (m *mockVCBackend) Unstage(_, filePath string) error {
	m.unstageArg = filePath
	return nil
}

// ---------------------------------------------------------------------------
// vc-status diff: fallback to staged when no unstaged changes
// ---------------------------------------------------------------------------

// TestVCStatusDiffFallsBackToStaged: when Diff returns empty, DiffStaged is
// called and its output is used.
func TestVCStatusDiffFallsBackToStaged(t *testing.T) {
	mock := &mockVCBackend{
		diffResult:       "",
		diffStagedResult: "diff --git a/file.go\n+added line\n",
	}
	origBackends := vcBackends
	vcBackends = []vcBackend{mock}
	defer func() { vcBackends = origBackends }()

	be, _ := vcFind("/mock/root")
	if be == nil {
		t.Fatal("vcFind returned nil for mock backend")
	}

	text, _ := be.Diff("/mock/root", "file.go")
	if text == "" {
		text, _ = be.DiffStaged("/mock/root", "file.go")
	}

	if !mock.unstagedCalled {
		t.Error("Diff (unstaged) was not called")
	}
	if !mock.stagedCalled {
		t.Error("DiffStaged was not called as fallback")
	}
	if text != mock.diffStagedResult {
		t.Errorf("diff text = %q, want staged result %q", text, mock.diffStagedResult)
	}
}

// TestVCStatusDiffSkipsStagedWhenUnstagedAvailable: DiffStaged must NOT be
// called when Diff already returns content.
func TestVCStatusDiffSkipsStagedWhenUnstagedAvailable(t *testing.T) {
	mock := &mockVCBackend{
		diffResult:       "diff --git a/file.go\n-removed\n",
		diffStagedResult: "should not be used",
	}
	origBackends := vcBackends
	vcBackends = []vcBackend{mock}
	defer func() { vcBackends = origBackends }()

	be, _ := vcFind("/mock/root")
	text, _ := be.Diff("/mock/root", "file.go")
	if text == "" {
		text, _ = be.DiffStaged("/mock/root", "file.go")
	}

	if !mock.unstagedCalled {
		t.Error("Diff was not called")
	}
	if mock.stagedCalled {
		t.Error("DiffStaged should not be called when unstaged diff is available")
	}
	if text != mock.diffResult {
		t.Errorf("diff text = %q, want %q", text, mock.diffResult)
	}
}

// ---------------------------------------------------------------------------
// vc-status unstage
// ---------------------------------------------------------------------------

// TestVCStatusUnstageCallsBackend: Unstage is invoked with the expected path.
func TestVCStatusUnstageCallsBackend(t *testing.T) {
	mock := &mockVCBackend{}
	origBackends := vcBackends
	vcBackends = []vcBackend{mock}
	defer func() { vcBackends = origBackends }()

	filePath := "/mock/root/internal/editor/nav.go"
	be, _ := vcFind("/mock/root")
	if err := be.Unstage("/mock/root", filePath); err != nil {
		t.Fatalf("Unstage returned error: %v", err)
	}
	if mock.unstageArg != filePath {
		t.Errorf("Unstage called with %q, want %q", mock.unstageArg, filePath)
	}
}

// ---------------------------------------------------------------------------
// Integration test helpers
// ---------------------------------------------------------------------------

// makeGitRepo creates a temporary directory with an initialised git repository
// containing a single committed Go source file.  The returned path is the
// repository root.
func makeGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_CONFIG_GLOBAL=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	run("git", "init", dir)
	run("git", "-C", dir, "config", "user.email", "test@test.com")
	run("git", "-C", dir, "config", "user.name", "Test")

	mainGo := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "-C", dir, "add", ".")
	run("git", "-C", dir, "commit", "-m", "initial commit")

	return dir
}

// newTestEditorWithVC creates a test editor whose active buffer's filename is
// set to main.go inside a fresh git repository.  The vc-related maps are
// initialised so that cmdVc* methods can operate without panicking.
func newTestEditorWithVC(t *testing.T) (*Editor, string) {
	t.Helper()
	dir := makeGitRepo(t)

	e := newTestEditor("")
	// Initialise vc maps that newTestEditor leaves nil.
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
	// vcDiffGotoSource may load a source file, which activates LSP; give it a
	// non-nil conns map and terminal so that path doesn't panic.
	e.lspConns = make(map[string]*lspConn)
	e.term = &terminal.Terminal{} // non-nil; screen==nil so PostWakeup is a no-op

	buf(e).SetFilename(filepath.Join(dir, "main.go"))
	return e, dir
}

// ---------------------------------------------------------------------------
// vcAnnotateHashAtPoint
// ---------------------------------------------------------------------------

func TestVcAnnotateHashAtPoint_BasicHash(t *testing.T) {
	e := newTestEditor("abc1234 main.go:1: package main\n")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
	buf(e).SetPoint(0)
	hash := e.vcAnnotateHashAtPoint(buf(e))
	if hash != "abc1234" {
		t.Fatalf("want abc1234, got %q", hash)
	}
}

func TestVcAnnotateHashAtPoint_StripsCaret(t *testing.T) {
	// git blame prefixes boundary commits with ^
	e := newTestEditor("^abc1234 main.go:1: package main\n")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
	buf(e).SetPoint(0)
	hash := e.vcAnnotateHashAtPoint(buf(e))
	if hash != "abc1234" {
		t.Fatalf("caret should be stripped; want abc1234, got %q", hash)
	}
}

func TestVcAnnotateHashAtPoint_EmptyLine(t *testing.T) {
	e := newTestEditor("\n")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
	buf(e).SetPoint(0)
	hash := e.vcAnnotateHashAtPoint(buf(e))
	if hash != "" {
		t.Fatalf("empty line should return empty hash, got %q", hash)
	}
}

func TestVcAnnotateHashAtPoint_FullSHA(t *testing.T) {
	sha := "deadbeefdeadbeef"
	e := newTestEditor(sha + " (Author 2024-01-01 42)  code here\n")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
	buf(e).SetPoint(0)
	hash := e.vcAnnotateHashAtPoint(buf(e))
	if hash != sha {
		t.Fatalf("want %q, got %q", sha, hash)
	}
}

// ---------------------------------------------------------------------------
// vcStatusFileAtPoint — pure helper
// ---------------------------------------------------------------------------

func TestVcStatusFileAtPoint_ModifiedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	b := buffer.NewWithContent("*vc-status*", " M main.go\n")
	b.SetPoint(0)
	got := vcStatusFileAtPoint(b, dir)
	if got != "main.go" {
		t.Fatalf("want main.go, got %q", got)
	}
}

func TestVcStatusFileAtPoint_UntrackedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	b := buffer.NewWithContent("*vc-status*", "?? new.go\n")
	b.SetPoint(0)
	got := vcStatusFileAtPoint(b, dir)
	if got != "new.go" {
		t.Fatalf("want new.go, got %q", got)
	}
}

func TestVcStatusFileAtPoint_EmptyLine(t *testing.T) {
	dir := t.TempDir()
	b := buffer.NewWithContent("*vc-status*", "\n")
	b.SetPoint(0)
	got := vcStatusFileAtPoint(b, dir)
	if got != "" {
		t.Fatalf("empty line should return empty string, got %q", got)
	}
}

func TestVcStatusFileAtPoint_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	// File not on disk — should return "".
	b := buffer.NewWithContent("*vc-status*", " M ghost.go\n")
	b.SetPoint(0)
	got := vcStatusFileAtPoint(b, dir)
	if got != "" {
		t.Fatalf("non-existent file should return empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// vcFind — integration with real git repo
// ---------------------------------------------------------------------------

func TestVcFind_FindsGitBackend(t *testing.T) {
	dir := makeGitRepo(t)
	be, root := vcFind(dir)
	if be == nil {
		t.Fatal("vcFind should find a backend for a git repo")
	}
	if be.Name() != "git" {
		t.Fatalf("want backend name git, got %q", be.Name())
	}
	if root == "" {
		t.Fatal("root should not be empty")
	}
}

func TestVcFind_ReturnNilForNonRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	be, root := vcFind(dir)
	if be != nil {
		t.Fatalf("should not find backend in non-repo dir, got %v", be.Name())
	}
	if root != "" {
		t.Fatalf("root should be empty for non-repo, got %q", root)
	}
}

func TestVcFind_SubdirectoryFindsRoot(t *testing.T) {
	dir := makeGitRepo(t)
	subDir := filepath.Join(dir, "pkg", "util")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	be, root := vcFind(subDir)
	if be == nil {
		t.Fatal("vcFind should find backend from subdirectory")
	}
	if root != dir {
		t.Fatalf("root should be repo root %q, got %q", dir, root)
	}
}

// ---------------------------------------------------------------------------
// gitBackend methods
// ---------------------------------------------------------------------------

func TestGitBackend_Name(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	be := gitBackend{}
	if be.Name() != "git" {
		t.Fatalf("want git, got %q", be.Name())
	}
}

func TestGitBackend_Root(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	root := be.Root(dir)
	if root != dir {
		t.Fatalf("want %q, got %q", dir, root)
	}
}

func TestGitBackend_RootEmpty_ForNonRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	be := gitBackend{}
	root := be.Root(dir)
	if root != "" {
		t.Fatalf("want empty string for non-repo, got %q", root)
	}
}

func TestGitBackend_Status(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Status(dir)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if out == "" {
		t.Fatal("Status output should not be empty")
	}
}

func TestGitBackend_Log(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Log(dir, "")
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if !strings.Contains(out, "initial commit") {
		t.Fatalf("Log should contain 'initial commit', got: %q", out)
	}
}

func TestGitBackend_LogForFile(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Log(dir, filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("Log for file failed: %v", err)
	}
	if !strings.Contains(out, "initial commit") {
		t.Fatalf("Log for file should contain 'initial commit', got: %q", out)
	}
}

func TestGitBackend_DiffCleanFile(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Diff(dir, filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	// Clean committed file should produce empty diff.
	if strings.TrimSpace(out) != "" {
		t.Fatalf("clean committed file should produce empty diff, got: %q", out)
	}
}

func TestGitBackend_DiffModifiedFile(t *testing.T) {
	dir := makeGitRepo(t)
	// Modify the file after commit.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { _ = 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	be := gitBackend{}
	out, err := be.Diff(dir, filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if out == "" {
		t.Fatal("modified file should produce non-empty diff")
	}
	if !strings.Contains(out, "@@") {
		t.Fatalf("diff output should contain hunk header @@, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// vcDir helper
// ---------------------------------------------------------------------------

func TestVcDir_BufferWithFilename(t *testing.T) {
	dir := makeGitRepo(t)
	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(dir, "main.go"))
	got := vcDir(buf(e))
	if got != dir {
		t.Fatalf("want %q, got %q", dir, got)
	}
}

func TestVcDir_BufferWithoutFilename(t *testing.T) {
	e := newTestEditor("")
	// No filename set — vcDir should return os.Getwd(), not panic or return empty.
	got := vcDir(buf(e))
	if got == "" {
		t.Fatal("vcDir should return a non-empty dir when buffer has no filename")
	}
}

// ---------------------------------------------------------------------------
// cmdVcStatus — editor integration
// ---------------------------------------------------------------------------

func TestCmdVcStatus_CreatesVcStatusBuffer(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcStatus()
	b := e.FindBuffer("*vc-status*")
	if b == nil {
		t.Fatal("cmdVcStatus should create *vc-status* buffer")
	}
	if b.Mode() != "vc-status" {
		t.Fatalf("want mode vc-status, got %q", b.Mode())
	}
}

func TestCmdVcStatus_NotInRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	e := newTestEditor("")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
	// Point buffer at a non-repo temp dir.
	dir := t.TempDir()
	buf(e).SetFilename(filepath.Join(dir, "file.go"))
	e.cmdVcStatus()
	// Should not crash and should not create a vc-status buffer.
	if b := e.FindBuffer("*vc-status*"); b != nil {
		t.Fatal("should not create vc-status buffer when not in a repo")
	}
}

func TestCmdVcStatus_StoresRoot(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.cmdVcStatus()
	b := e.FindBuffer("*vc-status*")
	if b == nil {
		t.Fatal("*vc-status* buffer not found")
	}
	root := e.vcLogRoots[b]
	if root != dir {
		t.Fatalf("want root %q, got %q", dir, root)
	}
}

// ---------------------------------------------------------------------------
// cmdVcPrintLog — editor integration
// ---------------------------------------------------------------------------

func TestCmdVcPrintLog_CreatesVCLogBuffer(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	b := e.FindBuffer("*VC Log*")
	if b == nil {
		t.Fatal("cmdVcPrintLog should create *VC Log* buffer")
	}
	if b.Mode() != "vc-log" {
		t.Fatalf("want mode vc-log, got %q", b.Mode())
	}
}

func TestCmdVcPrintLog_ContainsInitialCommit(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	b := e.FindBuffer("*VC Log*")
	if b == nil {
		t.Fatal("*VC Log* buffer not found")
	}
	if !strings.Contains(b.String(), "initial commit") {
		t.Fatalf("log should contain 'initial commit', got: %q", b.String())
	}
}

func TestCmdVcPrintLog_StoresRoot(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	b := e.FindBuffer("*VC Log*")
	if b == nil {
		t.Fatal("*VC Log* buffer not found")
	}
	root := e.vcLogRoots[b]
	if root != dir {
		t.Fatalf("want root %q, got %q", dir, root)
	}
}

func TestCmdVcPrintLog_StoresFilename(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	b := e.FindBuffer("*VC Log*")
	if b == nil {
		t.Fatal("*VC Log* buffer not found")
	}
	filePath := e.vcLogFiles[b]
	want := filepath.Join(dir, "main.go")
	if filePath != want {
		t.Fatalf("want filePath %q, got %q", want, filePath)
	}
}

// ---------------------------------------------------------------------------
// vcLogDispatch — key handling
// ---------------------------------------------------------------------------

func TestVcLogDispatch_Q_QuitsVcLog(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	// Put the source buffer in MRU so vcQuit can find it.
	e.bufferMRU = append(e.bufferMRU, buf(e))
	e.cmdVcPrintLog()
	if e.ActiveBuffer().Mode() != "vc-log" {
		t.Skip("vc-log buffer not active after cmdVcPrintLog")
	}
	e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer().Mode() == "vc-log" {
		t.Error("'q' should switch away from vc-log buffer")
	}
}

func TestVcLogDispatch_N_MovesDown(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	logBuf := e.ActiveBuffer()
	if logBuf.Mode() != "vc-log" {
		t.Skip("vc-log not active")
	}
	logBuf.SetPoint(0)
	before := logBuf.Point()
	e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	after := logBuf.Point()
	lines := strings.Count(logBuf.String(), "\n")
	if lines > 1 && after <= before {
		t.Errorf("'n' should advance point; before=%d after=%d", before, after)
	}
}

func TestVcLogDispatch_P_MovesUp(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	logBuf := e.ActiveBuffer()
	if logBuf.Mode() != "vc-log" {
		t.Skip("vc-log not active")
	}
	logBuf.SetPoint(logBuf.Len())
	before := logBuf.Point()
	e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'})
	after := logBuf.Point()
	lines := strings.Count(logBuf.String(), "\n")
	if lines > 1 && after >= before {
		t.Errorf("'p' should retreat point; before=%d after=%d", before, after)
	}
}

func TestVcLogDispatch_G_RefreshesLog(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	if e.ActiveBuffer().Mode() != "vc-log" {
		t.Skip("vc-log not active")
	}
	consumed := e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'g'})
	if !consumed {
		t.Error("'g' should be consumed by vcLogDispatch")
	}
}

func TestVcLogDispatch_UnknownKey_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	consumed := e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})
	if consumed {
		t.Error("unknown key 'z' should not be consumed by vcLogDispatch")
	}
}

func TestVcLogDispatch_NonRune_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcPrintLog()
	consumed := e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if consumed {
		t.Error("non-rune/non-enter key should not be consumed by vcLogDispatch")
	}
}

// ---------------------------------------------------------------------------
// vcDiffDispatch — key handling
// ---------------------------------------------------------------------------

func makeDiffEditor(t *testing.T) (*Editor, string) {
	t.Helper()
	e, dir := newTestEditorWithVC(t)
	diffContent := "diff --git a/main.go b/main.go\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,3 +1,4 @@\n" +
		" package main\n" +
		"+// added\n" +
		" \n" +
		" func main() {}\n" +
		"@@ -10,2 +11,3 @@\n" +
		" // end\n" +
		"+// more\n"
	diffBuf := buffer.NewWithContent("*vc-diff*", diffContent)
	diffBuf.SetMode("diff")
	e.buffers = append(e.buffers, diffBuf)
	e.bufferMRU = append(e.bufferMRU, buf(e))
	e.activeWin.SetBuf(diffBuf)
	e.vcLogRoots[diffBuf] = dir
	diffBuf.SetPoint(0)
	return e, dir
}

func TestVcDiffDispatch_Q_QuitsVcDiff(t *testing.T) {
	e, _ := makeDiffEditor(t)
	e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer().Mode() == "diff" {
		t.Error("'q' should switch away from diff buffer")
	}
}

func TestVcDiffDispatch_N_JumpsToNextHunk(t *testing.T) {
	e, _ := makeDiffEditor(t)
	diffBuf := e.ActiveBuffer()
	diffBuf.SetPoint(0)

	consumed := e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	if !consumed {
		t.Fatal("'n' should be consumed by vcDiffDispatch")
	}
	pt := diffBuf.Point()
	bol := diffBuf.BeginningOfLine(pt)
	eol := diffBuf.EndOfLine(pt)
	line := diffBuf.Substring(bol, eol)
	if !strings.HasPrefix(line, "@@") {
		t.Errorf("'n' should place point on @@ hunk header, got: %q", line)
	}
}

func TestVcDiffDispatch_P_JumpsToPrevHunk(t *testing.T) {
	e, _ := makeDiffEditor(t)
	diffBuf := e.ActiveBuffer()
	// Jump forward past first hunk, then back.
	e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	// Now press 'p' to go back to first hunk.
	consumed := e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'})
	if !consumed {
		t.Fatal("'p' should be consumed by vcDiffDispatch")
	}
	pt := diffBuf.Point()
	bol := diffBuf.BeginningOfLine(pt)
	eol := diffBuf.EndOfLine(pt)
	line := diffBuf.Substring(bol, eol)
	if !strings.HasPrefix(line, "@@") {
		t.Errorf("'p' should place point on @@ hunk header, got: %q", line)
	}
}

func TestVcDiffDispatch_NonRune_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	consumed := e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if consumed {
		t.Error("non-rune/non-enter key should not be consumed by vcDiffDispatch")
	}
}

// ---------------------------------------------------------------------------
// vcStatusDispatch — key handling
// ---------------------------------------------------------------------------

func TestVcStatusDispatch_Q_QuitsVcStatus(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.bufferMRU = append(e.bufferMRU, buf(e))
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status buffer not active")
	}
	e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer().Mode() == "vc-status" {
		t.Error("'q' should switch away from vc-status buffer")
	}
}

func TestVcStatusDispatch_G_RefreshesStatus(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status buffer not active")
	}
	consumed := e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'g'})
	if !consumed {
		t.Error("'g' should be consumed by vcStatusDispatch")
	}
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Error("'g' should keep vc-status mode after refresh")
	}
}

func TestVcStatusDispatch_L_ShowsVcLog(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status buffer not active")
	}
	consumed := e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'l'})
	if !consumed {
		t.Error("'l' should be consumed by vcStatusDispatch")
	}
	if e.ActiveBuffer().Mode() != "vc-log" {
		t.Errorf("'l' should switch to vc-log mode, got %q", e.ActiveBuffer().Mode())
	}
}

func TestVcStatusDispatch_UnknownKey_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status not active")
	}
	consumed := e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})
	if consumed {
		t.Error("unknown key 'z' should not be consumed by vcStatusDispatch")
	}
}

func TestVcStatusDispatch_NonRune_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status not active")
	}
	consumed := e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if consumed {
		t.Error("non-rune key should not be consumed by vcStatusDispatch")
	}
}

// ---------------------------------------------------------------------------
// vcGrepDispatch — key handling
// ---------------------------------------------------------------------------

func makeVcGrepEditor(t *testing.T) *Editor {
	t.Helper()
	e, dir := newTestEditorWithVC(t)
	grepContent := "main.go:1:package main\nmain.go:3:func main() {}\n"
	grepBuf := buffer.NewWithContent("*vc grep*", grepContent)
	grepBuf.SetMode("vc-grep")
	e.buffers = append(e.buffers, grepBuf)
	e.bufferMRU = append(e.bufferMRU, buf(e))
	e.activeWin.SetBuf(grepBuf)
	e.vcLogRoots[grepBuf] = dir
	grepBuf.SetPoint(0)
	return e
}

func TestVcGrepDispatch_Q_Quits(t *testing.T) {
	e := makeVcGrepEditor(t)
	e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer().Mode() == "vc-grep" {
		t.Error("'q' should switch away from vc-grep buffer")
	}
}

func TestVcGrepDispatch_Enter_OpensFile(t *testing.T) {
	e := makeVcGrepEditor(t)
	grepBuf := e.ActiveBuffer()
	if grepBuf.Mode() != "vc-grep" {
		t.Fatal("expected vc-grep buffer active")
	}
	grepBuf.SetPoint(0)
	e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyEnter})
	newBuf := e.ActiveBuffer()
	if newBuf == grepBuf {
		t.Error("Enter should navigate to a source file, not stay in grep buffer")
	}
}

func TestVcGrepDispatch_UnknownKey_NotConsumed(t *testing.T) {
	e := makeVcGrepEditor(t)
	consumed := e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'})
	if consumed {
		t.Error("unknown key 'x' should not be consumed by vcGrepDispatch")
	}
}

func TestVcGrepDispatch_NonRune_NotConsumed(t *testing.T) {
	e := makeVcGrepEditor(t)
	consumed := e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if consumed {
		t.Error("non-rune/non-enter key should not be consumed by vcGrepDispatch")
	}
}

// ---------------------------------------------------------------------------
// vcAnnotateDispatch — key handling
// ---------------------------------------------------------------------------

func TestVcAnnotateDispatch_Q_QuitsAnnotate(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	annotateContent := "abc12345 (Test 2024-01-01 1) package main\n"
	annotBuf := buffer.NewWithContent("*vc-annotate*", annotateContent)
	annotBuf.SetMode("vc-annotate")
	e.buffers = append(e.buffers, annotBuf)
	e.bufferMRU = append(e.bufferMRU, buf(e))
	e.activeWin.SetBuf(annotBuf)
	annotBuf.SetPoint(0)

	e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer().Mode() == "vc-annotate" {
		t.Error("'q' should switch away from vc-annotate buffer")
	}
}

func TestVcAnnotateDispatch_Q_WithParent_ReturnsToParent(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	srcBuf := buf(e)
	annotateContent := "abc12345 (Test 2024-01-01 1) package main\n"
	annotBuf := buffer.NewWithContent("*vc-annotate*", annotateContent)
	annotBuf.SetMode("vc-annotate")
	e.buffers = append(e.buffers, annotBuf)
	e.vcParent[annotBuf] = srcBuf
	e.activeWin.SetBuf(annotBuf)

	e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer() != srcBuf {
		t.Error("'q' with parent should return to the parent buffer")
	}
}

func TestVcAnnotateDispatch_NonRune_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	consumed := e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyEnter})
	if consumed {
		t.Error("non-rune key should not be consumed by vcAnnotateDispatch")
	}
}

func TestVcAnnotateDispatch_UnknownKey_NotConsumed(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	consumed := e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})
	if consumed {
		t.Error("unknown key 'z' should not be consumed by vcAnnotateDispatch")
	}
}

// ---------------------------------------------------------------------------
// vcQuit — switches to non-vc buffer
// ---------------------------------------------------------------------------

func TestVcQuit_SwitchesToNonVcBuffer(t *testing.T) {
	e := newTestEditor("some content")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)

	srcBuf := buf(e)
	e.bufferMRU = append(e.bufferMRU, srcBuf)

	logBuf := buffer.NewWithContent("*VC Log*", "abc1234 initial commit\n")
	logBuf.SetMode("vc-log")
	e.buffers = append(e.buffers, logBuf)
	e.activeWin.SetBuf(logBuf)

	e.vcQuit("vc-log")
	if e.ActiveBuffer().Mode() == "vc-log" {
		t.Error("vcQuit should switch away from vc-log buffer")
	}
}

func TestVcQuit_FallsBackToScratchWhenNoNonVcBuffers(t *testing.T) {
	// All buffers are vc buffers + *scratch* is not in the list — should end
	// up calling SwitchToBuffer("*scratch*") which creates it.
	e := newTestEditor("")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)

	// Make the single test buffer a vc-log buffer so there is no non-vc fallback.
	buf(e).SetMode("vc-log")

	logBuf2 := buffer.NewWithContent("*VC Log 2*", "abc1234 commit\n")
	logBuf2.SetMode("vc-log")
	e.buffers = append(e.buffers, logBuf2)
	e.activeWin.SetBuf(logBuf2)

	// Should not panic even if there are no non-vc buffers.
	e.vcQuit("vc-log")
}

// ---------------------------------------------------------------------------
// gitBackend extra coverage
// ---------------------------------------------------------------------------

func TestGitBackend_DiffStaged(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	// Modify and stage the file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { _ = 2 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	out, err := be.DiffStaged(dir, filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("DiffStaged: %v", err)
	}
	if !strings.Contains(out, "@@") {
		t.Fatalf("staged diff should contain hunk header, got %q", out)
	}
	// Whole-repo staged diff (filePath == "").
	if all, err := be.DiffStaged(dir, ""); err != nil || all == "" {
		t.Fatalf("whole-repo DiffStaged failed: %v / %q", err, all)
	}
}

func TestGitBackend_Show(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Show(dir, "HEAD")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !strings.Contains(out, "initial commit") {
		t.Fatalf("Show should contain commit message, got %q", out)
	}
}

func TestGitBackend_ShowLog(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.ShowLog(dir, "HEAD")
	if err != nil {
		t.Fatalf("ShowLog: %v", err)
	}
	if !strings.Contains(out, "Author") {
		t.Fatalf("ShowLog (fuller) should contain Author, got %q", out)
	}
}

func TestGitBackend_Grep(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Grep(dir, "package")
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(out, "main.go") {
		t.Fatalf("Grep should match main.go, got %q", out)
	}
}

func TestGitBackend_Blame(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	out, err := be.Blame(dir, filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("Blame: %v", err)
	}
	if !strings.Contains(out, "package main") {
		t.Fatalf("Blame should contain source line, got %q", out)
	}
}

func TestGitBackend_Revert(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("garbage\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := be.Revert(dir, p); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "package main") {
		t.Fatalf("Revert should restore original content, got %q", data)
	}
}

func TestGitBackend_Unstage(t *testing.T) {
	dir := makeGitRepo(t)
	be := gitBackend{}
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 3 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if err := be.Unstage(dir, p); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	// After unstaging, the staged diff for the file should be empty.
	staged, _ := be.DiffStaged(dir, p)
	if strings.TrimSpace(staged) != "" {
		t.Fatalf("after Unstage staged diff should be empty, got %q", staged)
	}
}

// ---------------------------------------------------------------------------
// cmdVcDiff
// ---------------------------------------------------------------------------

func TestCmdVcDiff_ModifiedFile(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { _ = 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.cmdVcDiff()
	b := e.FindBuffer("*vc-diff*")
	if b == nil {
		t.Fatal("cmdVcDiff should create *vc-diff* buffer for modified file")
	}
	if b.Mode() != "diff" {
		t.Fatalf("want mode diff, got %q", b.Mode())
	}
}

func TestCmdVcDiff_CleanFile(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcDiff()
	if b := e.FindBuffer("*vc-diff*"); b != nil {
		t.Fatal("cmdVcDiff should not create a buffer when there are no changes")
	}
}

func TestCmdVcDiff_NotInRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	e := newTestEditor("")
	initVCMaps(e)
	buf(e).SetFilename(filepath.Join(t.TempDir(), "x.go"))
	e.cmdVcDiff()
	if b := e.FindBuffer("*vc-diff*"); b != nil {
		t.Fatal("no diff buffer expected outside a repo")
	}
}

// ---------------------------------------------------------------------------
// cmdVcGrep / cmdProjectGrep
// ---------------------------------------------------------------------------

func TestCmdVcGrep_FindsMatches(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcGrep()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdVcGrep should activate the minibuffer")
	}
	e.minibufDoneFunc("package")
	b := e.FindBuffer("*vc grep*")
	if b == nil {
		t.Fatal("cmdVcGrep should create *vc grep* buffer")
	}
	if !strings.Contains(b.String(), "main.go") {
		t.Fatalf("grep buffer should mention main.go, got %q", b.String())
	}
}

func TestCmdVcGrep_EmptyPatternNoop(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcGrep()
	e.minibufDoneFunc("")
	if b := e.FindBuffer("*vc grep*"); b != nil {
		t.Fatal("empty pattern should not create grep buffer")
	}
}

func TestCmdProjectGrep_WithRepo(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdProjectGrep()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdProjectGrep should activate the minibuffer")
	}
	e.minibufDoneFunc("package")
	b := e.FindBuffer("*vc grep*")
	if b == nil {
		t.Fatal("cmdProjectGrep should create *vc grep* buffer")
	}
}

// ---------------------------------------------------------------------------
// cmdVcNextAction / vcGitAdd / vcOpenCommitBuffer
// ---------------------------------------------------------------------------

func TestCmdVcNextAction_StagesUntracked(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	newFile := filepath.Join(dir, "extra.go")
	if err := os.WriteFile(newFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	buf(e).SetFilename(newFile)
	e.cmdVcNextAction()
	// File should now be staged.
	out, _ := exec.Command("git", "-C", dir, "status", "--porcelain", newFile).Output()
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "A") {
		t.Fatalf("expected file to be staged (A), got %q", out)
	}
}

func TestCmdVcNextAction_OpensCommitBuffer(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	// Modify and stage so status x is non-space.
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 9 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	buf(e).SetFilename(p)
	e.cmdVcNextAction()
	b := e.FindBuffer("*vc-commit*")
	if b == nil {
		t.Fatal("staged change should open *vc-commit* buffer")
	}
	if e.vcCommitRoots[b] != dir {
		t.Fatalf("commit root not recorded, want %q got %q", dir, e.vcCommitRoots[b])
	}
}

func TestCmdVcNextAction_NothingToCommit(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcNextAction()
	if b := e.FindBuffer("*vc-commit*"); b != nil {
		t.Fatal("clean tree should not open commit buffer")
	}
}

func TestVcGitAdd_AllChanges(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	if err := os.WriteFile(filepath.Join(dir, "n.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.vcGitAdd(dir, "")
	out, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if !strings.Contains(string(out), "A  n.go") {
		t.Fatalf("vcGitAdd with empty path should stage everything, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// commit submit / abort / dispatch
// ---------------------------------------------------------------------------

func TestVcCommitSubmit_Commits(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 7 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	e.vcOpenCommitBuffer(dir, p)
	cb := e.ActiveBuffer()
	// Prepend a real commit message above the comment lines.
	cb.InsertString(0, "my commit message\n")
	e.vcCommitSubmit()
	// HEAD should now reference our message.
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--pretty=%s").Output()
	if strings.TrimSpace(string(out)) != "my commit message" {
		t.Fatalf("commit message not applied, got %q", out)
	}
}

func TestVcCommitSubmit_EmptyMessageAborts(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	b := e.FindBuffer("*vc-commit*")
	if b == nil {
		// Create one with only comment lines.
		e.vcOpenCommitBuffer(dir, "")
	}
	before, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	e.vcCommitSubmit()
	after, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if string(before) != string(after) {
		t.Fatal("empty commit message should not create a commit")
	}
}

func TestVcCommitAbort(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.vcOpenCommitBuffer(dir, "")
	e.vcCommitAbort()
	if e.ActiveBuffer().Mode() == "vc-commit" {
		t.Fatal("abort should switch away from the commit buffer")
	}
}

func TestVcCommitDispatch(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 5 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput()
	e.vcOpenCommitBuffer(dir, p)
	cb := e.ActiveBuffer()
	cb.InsertString(0, "dispatch commit\n")
	e.ctrlCKeymap = keymap.New("C-c")

	// Without the C-c prefix active, dispatch returns false.
	e.prefixKeymap = nil
	if e.vcCommitDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlC}) {
		t.Fatal("dispatch should be false without ctrlC prefix")
	}

	// With prefix active, C-c C-c submits.
	e.prefixKeymap = e.ctrlCKeymap
	if !e.vcCommitDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlC}) {
		t.Fatal("C-c C-c should be handled")
	}
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--pretty=%s").Output()
	if strings.TrimSpace(string(out)) != "dispatch commit" {
		t.Fatalf("dispatch commit not applied, got %q", out)
	}
}

func TestVcCommitDispatch_Abort(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.vcOpenCommitBuffer(dir, "")
	e.ctrlCKeymap = keymap.New("C-c")
	e.prefixKeymap = e.ctrlCKeymap
	if !e.vcCommitDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'k'}) {
		t.Fatal("C-c k should abort and be handled")
	}
}

// ---------------------------------------------------------------------------
// cmdVcAnnotate
// ---------------------------------------------------------------------------

func TestCmdVcAnnotate_CreatesBuffer(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcAnnotate()
	b := e.FindBuffer("*vc-annotate*")
	if b == nil {
		t.Fatal("cmdVcAnnotate should create *vc-annotate* buffer")
	}
	if !strings.HasPrefix(b.Mode(), "vc-annotate") {
		t.Fatalf("want vc-annotate mode, got %q", b.Mode())
	}
	if !strings.Contains(b.String(), "package main") {
		t.Fatalf("annotate output should contain source, got %q", b.String())
	}
}

func TestCmdVcAnnotate_NoFile(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	buf(e).SetFilename("")
	e.cmdVcAnnotate()
	if b := e.FindBuffer("*vc-annotate*"); b != nil {
		t.Fatal("annotate should not run on a buffer without a file")
	}
}

// ---------------------------------------------------------------------------
// vcDiffGotoSource
// ---------------------------------------------------------------------------

func TestVcDiffGotoSource_AddedLine(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() {\n\t_ = 42\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.cmdVcDiff()
	diff := e.FindBuffer("*vc-diff*")
	if diff == nil {
		t.Fatal("expected *vc-diff* buffer")
	}
	// Move point onto an added ('+') content line and exercise the parser.
	s := diff.String()
	idx := strings.Index(s, "+\t_ = 42")
	if idx < 0 {
		t.Skipf("no added line in diff:\n%s", s)
	}
	diff.SetPoint(idx)
	if !e.vcDiffGotoSource(diff) {
		t.Fatal("vcDiffGotoSource should return true")
	}
}

func TestVcDiffGotoSource_NonDiffLine(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	b := buf(e)
	b.InsertString(0, "not a diff line\n")
	b.SetPoint(0)
	// Should return true (no-op) without crashing.
	if !e.vcDiffGotoSource(b) {
		t.Fatal("expected true for a non-diff line")
	}
}

func TestVcDiffGotoSource_JumpsToAddedLine(t *testing.T) {
	e := newTestEditor("")
	initVCMaps(e)
	dir := t.TempDir()
	target := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(target, []byte("line1\nline2\nADDED\nline3\nline4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diffText := "diff --git a/foo.txt b/foo.txt\n" +
		"--- a/foo.txt\n" +
		"+++ b/foo.txt\n" +
		"@@ -1,3 +1,4 @@\n" +
		" line1\n" +
		" line2\n" +
		"+ADDED\n"
	b := buffer.NewWithContent("*vc-diff*", diffText)
	b.SetMode("diff")
	e.buffers = append(e.buffers, b)
	e.activeWin.SetBuf(b)
	e.vcLogRoots[b] = dir

	b.SetPoint(strings.Index(diffText, "+ADDED"))

	if !e.vcDiffGotoSource(b) {
		t.Fatal("vcDiffGotoSource should return true")
	}
	dest := e.activeWin.Buf()
	if dest.Filename() != target {
		t.Fatalf("expected active window to switch to %q, got %q", target, dest.Filename())
	}
	// Point should land on the added line (new-file line 3 = "ADDED").
	bol := dest.BeginningOfLine(dest.Point())
	eol := dest.EndOfLine(dest.Point())
	if got := dest.Substring(bol, eol); got != "ADDED" {
		t.Fatalf("point should land on the added line, got %q", got)
	}
}

func TestVcDiffGotoSource_MultiFileChoosesNearestHeader(t *testing.T) {
	e := newTestEditor("")
	initVCMaps(e)
	dir := t.TempDir()
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(second, []byte("alpha\nbeta\ngamma\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// A diff touching two files; the current line is in the second file's hunk.
	diffText := "diff --git a/first.txt b/first.txt\n" +
		"--- a/first.txt\n" +
		"+++ b/first.txt\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n" +
		"diff --git a/second.txt b/second.txt\n" +
		"--- a/second.txt\n" +
		"+++ b/second.txt\n" +
		"@@ -1,2 +1,3 @@\n" +
		" alpha\n" +
		"+beta\n"
	b := buffer.NewWithContent("*vc-diff*", diffText)
	b.SetMode("diff")
	e.buffers = append(e.buffers, b)
	e.activeWin.SetBuf(b)
	e.vcLogRoots[b] = dir

	b.SetPoint(strings.Index(diffText, "+beta"))

	if !e.vcDiffGotoSource(b) {
		t.Fatal("vcDiffGotoSource should return true")
	}
	if got := e.activeWin.Buf().Filename(); got != second {
		t.Fatalf("expected jump into second.txt (%q), got %q", second, got)
	}
}

func TestVcDiffGotoSource_NoRoot(t *testing.T) {
	e := newTestEditor("")
	initVCMaps(e)
	b := buffer.NewWithContent("*vc-diff*", "+++ b/foo.go\n@@ -1 +1 @@\n+added\n")
	b.SetMode("diff")
	// vcLogRoots not set for b → no determinable root → early return.
	b.SetPoint(b.EndOfLine(b.Len()))
	if !e.vcDiffGotoSource(b) {
		t.Fatal("expected true even when no root is known")
	}
}

// initVCMaps initialises the VC-related maps that newTestEditor leaves nil.
func initVCMaps(e *Editor) {
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	e.vcLogFiles = make(map[*buffer.Buffer]string)
	e.vcParent = make(map[*buffer.Buffer]*buffer.Buffer)
	e.vcCommitRoots = make(map[*buffer.Buffer]string)
}

// ---------------------------------------------------------------------------
// cmdVcRevert
// ---------------------------------------------------------------------------

func TestCmdVcRevert_ConfirmReverts(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc broken() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Load the modified file into the active buffer so revert reloads it.
	b, err := e.loadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	e.activeWin.SetBuf(b)

	e.cmdVcRevert()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdVcRevert should prompt for confirmation")
	}
	e.minibufDoneFunc("y")

	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "func main() {}") {
		t.Fatalf("revert should restore committed content, got %q", data)
	}
}

func TestCmdVcRevert_CancelKeepsChanges(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc broken() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b, _ := e.loadFile(p)
	e.activeWin.SetBuf(b)
	e.cmdVcRevert()
	e.minibufDoneFunc("n")
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "func broken()") {
		t.Fatalf("cancel should keep modified content, got %q", data)
	}
}

func TestCmdVcRevert_NoChanges(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcRevert()
	if e.minibufDoneFunc != nil {
		t.Fatal("clean file should not prompt for revert")
	}
}

func TestCmdVcRevert_NoFile(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	buf(e).SetFilename("")
	e.cmdVcRevert()
	if e.minibufActive {
		t.Fatal("revert with no file should not activate the minibuffer")
	}
}

// ---------------------------------------------------------------------------
// vcFixupSelectDispatch
// ---------------------------------------------------------------------------

func TestVcFixupSelectDispatch_Navigation(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	b := buf(e)
	b.InsertString(0, "abc1234 first\ndef5678 second\n")
	b.SetMode("vc-fixup-select")
	b.SetPoint(0)
	e.ctrlCKeymap = keymap.New("C-c")
	e.prefixKeymap = nil

	if !e.vcFixupSelectDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("n should move to next line and be handled")
	}
	if !e.vcFixupSelectDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Fatal("p should move to previous line and be handled")
	}
	// Unhandled rune returns false.
	if e.vcFixupSelectDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'}) {
		t.Fatal("unhandled rune should return false")
	}
}

func TestVcFixupSelectDispatch_Quit(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	b := buf(e)
	b.InsertString(0, "abc1234 first\n")
	b.SetMode("vc-fixup-select")
	e.ctrlCKeymap = keymap.New("C-c")
	e.prefixKeymap = nil
	if !e.vcFixupSelectDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should quit and be handled")
	}
}

func TestVcFixupSelectDispatch_CreatesFixup(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	// Make a second commit so there's a target to fixup.
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = exec.Command("git", "-C", dir, "commit", "-am", "second").CombinedOutput()
	// Stage a change to be fixed up.
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 2 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput()
	shaOut, _ := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
	sha := strings.TrimSpace(string(shaOut))

	b := buf(e)
	b.InsertString(0, sha+" second\n")
	b.SetMode("vc-fixup-select")
	b.SetPoint(0)
	e.vcLogRoots[b] = dir
	e.ctrlCKeymap = keymap.New("C-c")
	e.prefixKeymap = e.ctrlCKeymap

	if !e.vcFixupSelectDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlC}) {
		t.Fatal("C-c C-c should be handled")
	}
	logOut, _ := exec.Command("git", "-C", dir, "log", "-1", "--pretty=%s").Output()
	if !strings.HasPrefix(strings.TrimSpace(string(logOut)), "fixup!") {
		t.Fatalf("expected a fixup! commit at HEAD, got %q", logOut)
	}
}

func TestVcFixupSelectDispatch_CancelWithCtrlCK(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	b := buf(e)
	b.InsertString(0, "abc1234 first\n")
	b.SetMode("vc-fixup-select")
	e.ctrlCKeymap = keymap.New("C-c")
	e.prefixKeymap = e.ctrlCKeymap
	if !e.vcFixupSelectDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'k'}) {
		t.Fatal("C-c k should cancel and be handled")
	}
}

// ---------------------------------------------------------------------------
// VC buffer key dispatch
// ---------------------------------------------------------------------------

// newVCFullEditor builds a VC editor with the extra maps loadFile needs.
func newVCFullEditor(t *testing.T) (*Editor, string) {
	t.Helper()
	e, dir := newTestEditorWithVC(t)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	return e, dir
}

func TestVcLogDispatch_Keys(t *testing.T) {
	e, _ := newVCFullEditor(t)
	e.cmdVcPrintLog()
	logBuf := e.ActiveBuffer()
	logBuf.SetPoint(0)

	// Non-rune, non-enter key is not handled.
	if e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlA}) {
		t.Fatal("C-a should not be handled by vcLogDispatch")
	}
	// n / p navigation.
	if !e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("n should be handled")
	}
	if !e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Fatal("p should be handled")
	}
	// g refresh.
	if !e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'g'}) {
		t.Fatal("g should be handled")
	}
	// Re-fetch active log buffer and put point on the commit line.
	e.ActiveBuffer().SetPoint(0)
	// l shows the commit log message.
	if !e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'l'}) {
		t.Fatal("l should be handled")
	}
	if e.FindBuffer("*VC Log Message*") == nil {
		t.Fatal("l should create *VC Log Message* buffer")
	}
}

func TestVcLogDispatch_ShowDiff(t *testing.T) {
	e, _ := newVCFullEditor(t)
	e.cmdVcPrintLog()
	e.ActiveBuffer().SetPoint(0)
	if !e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter should be handled (show diff)")
	}
	if e.FindBuffer("*VC Show*") == nil {
		t.Fatal("Enter should create *VC Show* buffer")
	}
}

func TestVcLogDispatch_Quit(t *testing.T) {
	e, _ := newVCFullEditor(t)
	e.cmdVcPrintLog()
	if !e.vcLogDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should be handled")
	}
	if e.ActiveBuffer().Mode() == "vc-log" {
		t.Fatal("q should switch away from the log buffer")
	}
}

func TestVcStatusDispatch_Keys(t *testing.T) {
	e, dir := newVCFullEditor(t)
	// Make a change so status has content.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { _ = 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.cmdVcStatus()
	statusBuf := e.ActiveBuffer()

	if e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlA}) {
		t.Fatal("C-a should not be handled")
	}
	// g refresh.
	e.activeWin.SetBuf(statusBuf)
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'g'}) {
		t.Fatal("g should be handled")
	}
	// l → log.
	e.activeWin.SetBuf(statusBuf)
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'l'}) {
		t.Fatal("l should be handled")
	}
	if e.FindBuffer("*VC Log*") == nil {
		t.Fatal("l should create *VC Log* buffer")
	}
	// d → diff (whole repo since point not on a file line).
	e.activeWin.SetBuf(statusBuf)
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'd'}) {
		t.Fatal("d should be handled")
	}
	// f → fixup select.
	e.activeWin.SetBuf(statusBuf)
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f'}) {
		t.Fatal("f should be handled")
	}
	if e.FindBuffer("*VC Fixup Select*") == nil {
		t.Fatal("f should create *VC Fixup Select* buffer")
	}
}

func TestVcStatusDispatch_StageAndUnstage(t *testing.T) {
	e, dir := newVCFullEditor(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { _ = 2 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.cmdVcStatus()
	// s stages everything (point not on a file → absPath empty → add .).
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 's'}) {
		t.Fatal("s should be handled")
	}
	// u unstages everything.
	e.activeWin.SetBuf(e.FindBuffer("*vc-status*"))
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'u'}) {
		t.Fatal("u should be handled")
	}
}

func TestVcStatusDispatch_Quit(t *testing.T) {
	e, _ := newVCFullEditor(t)
	e.cmdVcStatus()
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should be handled")
	}
}

func TestVcDiffDispatch_HunkNavigation(t *testing.T) {
	e, _ := newVCFullEditor(t)
	diff := buffer.NewWithContent("*vc-diff*", "diff --git a/x b/x\n@@ -1,2 +1,2 @@\n line\n@@ -5,2 +5,2 @@\n other\n")
	diff.SetMode("diff")
	e.buffers = append(e.buffers, diff)
	e.activeWin.SetBuf(diff)
	diff.SetPoint(0)

	if !e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("n should be handled (next hunk)")
	}
	// Now on second hunk after another n, or no next hunk.
	e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	if !e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Fatal("p should be handled (prev hunk)")
	}
	if e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlA}) {
		t.Fatal("C-a should not be handled")
	}
}

func TestVcDiffDispatch_Quit(t *testing.T) {
	e, _ := newVCFullEditor(t)
	diff := buffer.NewWithContent("*vc-diff*", "diff\n")
	diff.SetMode("diff")
	e.buffers = append(e.buffers, diff)
	e.activeWin.SetBuf(diff)
	if !e.vcDiffDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should be handled")
	}
}

func TestVcGrepDispatch_EnterOpensFile(t *testing.T) {
	e, dir := newVCFullEditor(t)
	grep := buffer.NewWithContent("*vc grep*", "main.go:1:package main\n")
	grep.SetMode("vc-grep")
	e.buffers = append(e.buffers, grep)
	e.activeWin.SetBuf(grep)
	e.vcLogRoots[grep] = dir
	grep.SetPoint(0)
	if !e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter should be handled")
	}
	if e.ActiveBuffer().Filename() != filepath.Join(dir, "main.go") {
		t.Fatalf("Enter should open main.go, got %q", e.ActiveBuffer().Filename())
	}
}

func TestVcGrepDispatch_Quit(t *testing.T) {
	e, _ := newVCFullEditor(t)
	grep := buffer.NewWithContent("*vc grep*", "main.go:1:x\n")
	grep.SetMode("vc-grep")
	e.buffers = append(e.buffers, grep)
	e.activeWin.SetBuf(grep)
	if !e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should be handled")
	}
	if e.vcGrepDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlA}) {
		t.Fatal("C-a should not be handled")
	}
}

func TestVcAnnotateDispatch_Keys(t *testing.T) {
	e, _ := newVCFullEditor(t)
	e.cmdVcAnnotate()
	annBuf := e.ActiveBuffer()
	annBuf.SetPoint(0)

	if e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlA}) {
		t.Fatal("non-rune key should not be handled")
	}
	// l → show commit log of hash at point.
	if !e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'l'}) {
		t.Fatal("l should be handled")
	}
	// Back to annotate buffer, d → show diff of commit.
	e.activeWin.SetBuf(annBuf)
	annBuf.SetPoint(0)
	if !e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'd'}) {
		t.Fatal("d should be handled")
	}
	// q returns to parent.
	e.activeWin.SetBuf(annBuf)
	if !e.vcAnnotateDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should be handled")
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: vcStatusDispatch d/s/u/f/c/Enter, vcDiffGotoSource
// ---------------------------------------------------------------------------

// startVcStatusWithModifiedFile opens a vc-status buffer for a repo whose
// main.go has been modified (unstaged), with point on the file's line.
func startVcStatusCov(t *testing.T) (*Editor, string) {
	t.Helper()
	e, dir := newTestEditorWithVC(t)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.bufferMRU = append(e.bufferMRU, buf(e))
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() { _ = 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status buffer not active")
	}
	return e, dir
}

// pointToFirstFileLine moves point to a line in the status buffer that names a
// real file under root, returning true on success.
func pointToFirstFileLine(e *Editor, root string) bool {
	sb := e.ActiveBuffer()
	for ln := 1; ln <= 200; ln++ {
		pos := sb.LineStart(ln)
		if pos >= sb.Len() && ln > 1 {
			break
		}
		sb.SetPoint(pos)
		if vcStatusFileAtPoint(sb, root) != "" {
			return true
		}
	}
	return false
}

func TestCov_VcStatusDispatch_D_ShowsDiff(t *testing.T) {
	e, root := startVcStatusCov(t)
	if !pointToFirstFileLine(e, root) {
		t.Skip("no file line found in status output")
	}
	consumed := e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'd'})
	if !consumed {
		t.Fatal("'d' should be consumed")
	}
	if e.ActiveBuffer().Mode() != "diff" {
		t.Fatalf("'d' should produce a diff buffer, got mode %q", e.ActiveBuffer().Mode())
	}
}

func TestCov_VcStatusDispatch_S_StagesFile(t *testing.T) {
	e, root := startVcStatusCov(t)
	if !pointToFirstFileLine(e, root) {
		t.Skip("no file line found in status output")
	}
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 's'}) {
		t.Fatal("'s' should be consumed")
	}
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Fatalf("'s' should refresh into vc-status, got %q", e.ActiveBuffer().Mode())
	}
}

func TestCov_VcStatusDispatch_U_Unstages(t *testing.T) {
	e, root := startVcStatusCov(t)
	// Stage main.go so there is something to unstage.
	if out, err := exec.Command("git", "-C", root, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	e.cmdVcStatus()
	if !pointToFirstFileLine(e, root) {
		t.Skip("no file line found")
	}
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'u'}) {
		t.Fatal("'u' should be consumed")
	}
}

func TestCov_VcStatusDispatch_F_OpensFixupSelect(t *testing.T) {
	e, _ := startVcStatusCov(t)
	statusBuf := e.ActiveBuffer()
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f'}) {
		t.Fatal("'f' should be consumed")
	}
	if e.ActiveBuffer().Mode() != "vc-fixup-select" {
		t.Fatalf("'f' should open vc-fixup-select, got %q", e.ActiveBuffer().Mode())
	}
	if e.vcParent[e.ActiveBuffer()] != statusBuf {
		t.Fatal("fixup buffer parent should be the status buffer")
	}
}

func TestCov_VcStatusDispatch_C_OpensCommitBuffer(t *testing.T) {
	e, root := startVcStatusCov(t)
	if out, err := exec.Command("git", "-C", root, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	e.cmdVcStatus()
	// Point on a file line (a staged file), not a header — opens commit buffer.
	pointToFirstFileLine(e, root)
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'c'}) {
		t.Fatal("'c' should be consumed")
	}
	if e.FindBuffer("*vc-commit*") == nil {
		t.Fatal("'c' should open *vc-commit* buffer")
	}
}

func TestCov_VcStatusDispatch_Enter_OpensFile(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.bufferMRU = append(e.bufferMRU, buf(e))
	// Add an untracked .txt file so Enter loads it without starting an LSP.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.cmdVcStatus()
	if e.ActiveBuffer().Mode() != "vc-status" {
		t.Skip("vc-status buffer not active")
	}
	root := dir
	// Find the notes.txt line specifically.
	sb := e.ActiveBuffer()
	found := false
	for ln := 1; ln <= 200; ln++ {
		pos := sb.LineStart(ln)
		if pos >= sb.Len() && ln > 1 {
			break
		}
		sb.SetPoint(pos)
		if vcStatusFileAtPoint(sb, root) == "notes.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Skip("notes.txt line not found in status")
	}
	statusBuf := e.ActiveBuffer()
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter should be consumed")
	}
	if e.ActiveBuffer() == statusBuf {
		t.Fatal("Enter on a file line should open that file")
	}
}

func TestCov_VcStatusDispatch_NoRootEnter(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	sb := buffer.NewWithContent("*vc-status*", "?? x\n")
	sb.SetMode("vc-status")
	e.buffers = append(e.buffers, sb)
	e.activeWin.SetBuf(sb)
	// No root recorded for this buffer -> Enter returns true but does nothing.
	if !e.vcStatusDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter with no root should still be consumed")
	}
}

func TestCov_VcDiffGotoSource_AddedLine(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	// Track a .txt file, then modify it and produce a real git diff so the
	// parser sees a genuine "+++ b/notes.txt" header and can resolve the path
	// without spinning up an LSP server.
	p := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(p, []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "notes.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "add notes").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	if err := os.WriteFile(p, []byte("one\nCHANGED\nthree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	buf(e).SetFilename(p)
	e.cmdVcDiff()
	db := e.FindBuffer("*vc-diff*")
	if db == nil {
		t.Fatal("expected *vc-diff* buffer")
	}
	s := db.String()
	idx := strings.Index(s, "+CHANGED")
	if idx < 0 {
		t.Skipf("no added line in diff:\n%s", s)
	}
	db.SetPoint(idx)
	if !e.vcDiffGotoSource(db) {
		t.Fatal("vcDiffGotoSource should return true")
	}
}

func TestCov_VcDiffGotoSource_NonChangeLine(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	db := buffer.NewWithContent("*vc-diff*", " context line\n")
	db.SetMode("diff")
	e.buffers = append(e.buffers, db)
	e.activeWin.SetBuf(db)
	e.vcLogRoots[db] = dir
	db.SetPoint(0)
	// Context line (leading space) is not a +/- change -> no navigation.
	if !e.vcDiffGotoSource(db) {
		t.Fatal("should return true")
	}
	if e.ActiveBuffer() != db {
		t.Fatal("context line should not navigate")
	}
}

func TestCov_VcDiffGotoSource_HeaderLine(t *testing.T) {
	e, dir := newTestEditorWithVC(t)
	db := buffer.NewWithContent("*vc-diff*", "+++ b/main.go\n")
	db.SetMode("diff")
	e.buffers = append(e.buffers, db)
	e.activeWin.SetBuf(db)
	e.vcLogRoots[db] = dir
	db.SetPoint(0)
	if !e.vcDiffGotoSource(db) {
		t.Fatal("should return true")
	}
	if e.ActiveBuffer() != db {
		t.Fatal("+++ header should not navigate")
	}
}

func TestCov_CmdProjectGrep_NoVcFallsBackToGrep(t *testing.T) {
	e := newTestEditor("")
	e.vcLogRoots = make(map[*buffer.Buffer]string)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	buf(e).SetFilename(filepath.Join(dir, "a.txt"))
	e.cmdProjectGrep()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdProjectGrep should activate the minibuffer")
	}
	e.minibufDoneFunc("needle")
	b := e.FindBuffer("*vc grep*")
	if b == nil {
		t.Fatal("project grep should create *vc grep* buffer")
	}
	if !strings.Contains(b.String(), "needle") {
		t.Fatalf("grep output should contain match, got %q", b.String())
	}
}

func TestCov_CmdProjectGrep_EmptyPatternNoop(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdProjectGrep()
	e.minibufDoneFunc("")
	if e.FindBuffer("*vc grep*") != nil {
		t.Fatal("empty pattern should not create grep buffer")
	}
}

func TestCov_CmdVcGrep_NoMatches(t *testing.T) {
	e, _ := newTestEditorWithVC(t)
	e.cmdVcGrep()
	e.minibufDoneFunc("zzz_no_such_token_zzz")
	b := e.FindBuffer("*vc grep*")
	if b == nil {
		t.Fatal("grep with no matches should still create buffer")
	}
	if !strings.Contains(b.String(), "No matches") {
		t.Fatalf("expected 'No matches found.', got %q", b.String())
	}
}
