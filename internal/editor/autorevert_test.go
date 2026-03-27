package editor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/window"
)

// newTestEditorWithFile creates a minimal Editor with a buffer backed by a
// real file on disk, suitable for testing auto-revert.
func newTestEditorWithFile(t *testing.T, content string) (*Editor, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("setup stat: %v", err)
	}

	buf := buffer.NewWithContent("test.txt", content)
	buf.SetFilename(path)
	win := window.New(buf, 0, 0, 80, 24)
	e := &Editor{
		buffers:          []*buffer.Buffer{buf},
		windows:          []*window.Window{win},
		activeWin:        win,
		autoRevert:       true,
		autoRevertMtimes: map[*buffer.Buffer]time.Time{buf: info.ModTime()},
		spanCaches:       make(map[*buffer.Buffer]*spanCache),
	}
	e.minibufBuf = buffer.New(" *minibuf*")
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	return e, path
}

func TestAutoRevert_ReloadsWhenFileChanges(t *testing.T) {
	e, path := newTestEditorWithFile(t, "original")

	// Ensure the check fires immediately by zeroing last-check time.
	e.autoRevertLastCheck = time.Time{}

	// First call — file unchanged, nothing should happen.
	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "original" {
		t.Fatalf("unexpected reload before file change: %q", got)
	}

	// Write new content with a definitely newer mtime.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.autoRevertLastCheck = time.Time{} // force re-check

	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "updated" {
		t.Errorf("buffer not reloaded: got %q, want %q", got, "updated")
	}
}

func TestAutoRevert_DoesNotReloadModifiedBuffer(t *testing.T) {
	e, path := newTestEditorWithFile(t, "original")

	// Mark buffer as modified (unsaved user edits).
	e.ActiveBuffer().SetModified(true)
	e.autoRevertLastCheck = time.Time{}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.autoRevertLastCheck = time.Time{}

	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "original" {
		t.Errorf("modified buffer was reverted unexpectedly: got %q", got)
	}
}

func TestAutoRevert_DisabledByConfig(t *testing.T) {
	e, path := newTestEditorWithFile(t, "original")
	e.autoRevert = false
	e.autoRevertLastCheck = time.Time{}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.autoRevertLastCheck = time.Time{}

	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "original" {
		t.Errorf("auto-revert disabled but buffer was reloaded: got %q", got)
	}
}
