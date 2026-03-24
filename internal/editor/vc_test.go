package editor

import (
	"testing"
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
