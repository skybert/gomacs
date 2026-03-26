package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkProjectFiles_Basic(t *testing.T) {
	// Create a small temporary project tree.
	root := t.TempDir()
	files := []string{
		"main.go",
		"pkg/foo.go",
		"pkg/bar.go",
		"README.md",
	}
	for _, f := range files {
		full := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// .git directory should be skipped.
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got := walkProjectFiles(root)
	if len(got) != len(files) {
		t.Errorf("walkProjectFiles: want %d files, got %d: %v", len(files), len(got), got)
	}
	// Verify .git/config is not present.
	for _, f := range got {
		if containsStr(f, ".git") {
			t.Errorf("walkProjectFiles: .git file leaked: %s", f)
		}
	}
}

func TestProjectFileCompletions_EmptyQuery(t *testing.T) {
	e := newTestEditor("")
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "a.go"),
		filepath.Join(root, "b.go"),
		filepath.Join(root, "c.go"),
	}
	got := e.projectFileCompletions(root, files, "")
	if len(got) != 3 {
		t.Errorf("empty query: want 3 results, got %d", len(got))
	}
}

func TestProjectFileCompletions_FuzzyFilter(t *testing.T) {
	e := newTestEditor("")
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "main.go"),
		filepath.Join(root, "main_test.go"),
		filepath.Join(root, "other.go"),
	}
	got := e.projectFileCompletions(root, files, "main")
	if len(got) != 2 {
		t.Errorf("query 'main': want 2 results, got %d: %v", len(got), got)
	}
	// "main.go" should rank before "main_test.go" (prefix match both, alphabetical).
	if len(got) >= 2 && got[0] != "main.go" {
		t.Errorf("query 'main': want first=main.go, got %q", got[0])
	}
}

func TestProjectFileCompletions_LRUFirst(t *testing.T) {
	e := newTestEditor("")
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "alpha.go"),
		filepath.Join(root, "beta.go"),
		filepath.Join(root, "gamma.go"),
	}
	// Simulate beta.go being the most recently used buffer.
	betaBuf := e.ActiveBuffer()
	betaBuf.SetFilename(filepath.Join(root, "beta.go"))
	e.bufferMRU = append(e.bufferMRU, betaBuf)

	got := e.projectFileCompletions(root, files, "")
	if len(got) == 0 {
		t.Fatal("expected results")
	}
	if got[0] != "beta.go" {
		t.Errorf("LRU: want beta.go first, got %q", got[0])
	}
}
