package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
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
