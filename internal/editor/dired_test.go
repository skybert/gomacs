package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
)

// newDiredTestEditor builds a test Editor with diredStates initialised so
// dired commands can attach state to buffers.
func newDiredTestEditor() *Editor {
	e := newTestEditor("")
	e.diredStates = make(map[*buffer.Buffer]*diredState)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)
	return e
}

// makeDiredFixture writes a small directory tree under a tempdir and returns
// (dir, file path) so tests can manipulate it.
func makeDiredFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOpenDired(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if buf(e).Mode() != "dired" {
		t.Errorf("active buffer mode = %q, want dired", buf(e).Mode())
	}
	content := buf(e).String()
	if !strings.Contains(content, "a.txt") || !strings.Contains(content, "b.txt") {
		t.Errorf("dired content missing entries: %q", content)
	}
	if !strings.Contains(content, "sub/") {
		t.Errorf("dired content missing sub/: %q", content)
	}
}

func TestOpenDiredReusesBuffer(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	first := buf(e)
	e.openDired(dir)
	if buf(e) != first {
		t.Error("openDired should reuse existing buffer")
	}
}

func TestDiredRefreshUnreadableDir(t *testing.T) {
	e := newDiredTestEditor()
	// Open a dired buffer pointing at a non-existent directory; refresh should
	// write an error line without panicking.
	e.openDired("/nonexistent-xyzzy/zzz/dir")
	if !strings.Contains(buf(e).String(), "Error reading") {
		t.Errorf("missing error text: %q", buf(e).String())
	}
}

func TestDiredCurrentFile(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	b := buf(e)
	ds := e.diredStates[b]
	if ds == nil {
		t.Fatal("no dired state")
	}
	// Point should already be on first file row.
	name, ok := e.diredCurrentFile(b, ds)
	if !ok || name == "" {
		t.Fatalf("diredCurrentFile = (%q, %v)", name, ok)
	}
	// Name should be one of our fixture files / the sub directory.
	if name != "a.txt" && name != "b.txt" && name != "sub" {
		t.Errorf("unexpected current file %q", name)
	}
}

func TestDiredCurrentFileShortLine(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	b := buf(e)
	ds := e.diredStates[b]
	// Move to the very first line (header).
	b.SetPoint(0)
	if name, ok := e.diredCurrentFile(b, ds); ok {
		t.Errorf("header line should not yield filename, got %q", name)
	}
}

func TestDiredDispatchNotInDiredMode(t *testing.T) {
	e := newDiredTestEditor()
	if e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Error("dispatch returned true in non-dired buffer")
	}
}

func TestDiredDispatchModifierPassthrough(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n', Mod: tcell.ModCtrl}) {
		t.Error("modified key should not be consumed")
	}
}

func TestDiredDispatchNonRuneKeysOtherThanEnter(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyTab}) {
		t.Error("Tab should not be consumed")
	}
}

func TestDiredDispatchNavigation(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Error("'n' not consumed")
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Error("'p' not consumed")
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: ' '}) {
		t.Error("' ' not consumed")
	}
}

func TestDiredDispatchOpenFile(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)

	// Position cursor on a.txt: navigate down until we find a.txt.
	b := buf(e)
	ds := e.diredStates[b]
	// Iterate lines until we find a.txt.
	found := false
	for range 20 {
		name, ok := e.diredCurrentFile(b, ds)
		if ok && name == "a.txt" {
			found = true
			break
		}
		e.cmdNextLine()
	}
	if !found {
		t.Skip("a.txt not found in dired buffer")
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f'}) {
		t.Error("'f' should open file")
	}
	if got := buf(e).Filename(); !strings.HasSuffix(got, "a.txt") {
		t.Errorf("after 'f', active file = %q", got)
	}
}

func TestDiredDispatchOpenSubDirEnter(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)

	b := buf(e)
	ds := e.diredStates[b]
	// First entry is "sub" (directories sort first).
	for range 20 {
		name, ok := e.diredCurrentFile(b, ds)
		if ok && name == "sub" {
			break
		}
		e.cmdNextLine()
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Error("Enter not consumed")
	}
	if !strings.HasSuffix(e.diredStates[buf(e)].dir, "sub") {
		t.Errorf("after Enter, dir = %q", e.diredStates[buf(e)].dir)
	}
}

func TestDiredDispatchMarkDelete(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)

	b := buf(e)
	ds := e.diredStates[b]
	// Find a.txt
	for range 20 {
		name, ok := e.diredCurrentFile(b, ds)
		if ok && name == "a.txt" {
			break
		}
		e.cmdNextLine()
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'd'}) {
		t.Error("'d' not consumed")
	}
	if ds.marks["a.txt"] != 'D' {
		t.Errorf("mark = %q, want 'D'", ds.marks["a.txt"])
	}
}

func TestDiredDispatchUnmark(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)

	b := buf(e)
	ds := e.diredStates[b]
	ds.marks["a.txt"] = 'D'
	for range 20 {
		name, ok := e.diredCurrentFile(b, ds)
		if ok && name == "a.txt" {
			break
		}
		e.cmdNextLine()
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'u'}) {
		t.Error("'u' not consumed")
	}
	if _, ok := ds.marks["a.txt"]; ok {
		t.Error("expected mark removed by 'u'")
	}
}

func TestDiredDispatchExecuteWithoutMarks(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'}) {
		t.Error("'x' not consumed")
	}
}

func TestDiredDispatchRefresh(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'g'}) {
		t.Error("'g' not consumed")
	}
}

func TestDiredDispatchParent(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	parent := filepath.Dir(dir)
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '^'}) {
		t.Error("'^' not consumed")
	}
	if e.diredStates[buf(e)].dir != parent {
		t.Errorf("after '^', dir = %q, want %q", e.diredStates[buf(e)].dir, parent)
	}
}

func TestDiredDispatchUnknownKey(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'}) {
		t.Error("'z' should not be consumed")
	}
}

func TestDiredDispatchExecuteDeletesConfirmed(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	ds := e.diredStates[buf(e)]
	ds.marks["a.txt"] = 'D'

	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'}) {
		t.Fatal("'x' not consumed")
	}
	if !e.readCharPending || e.readCharCallback == nil {
		t.Fatal("'x' with marks should arm a y/n confirmation")
	}
	e.readCharCallback('y')
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt should have been deleted, stat err = %v", err)
	}
}

func TestDiredDispatchExecuteDeletesCancelled(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	ds := e.diredStates[buf(e)]
	ds.marks["b.txt"] = 'D'

	e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'})
	if e.readCharCallback == nil {
		t.Fatal("expected confirmation callback")
	}
	e.readCharCallback('n')
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err != nil {
		t.Errorf("b.txt should still exist after cancel, stat err = %v", err)
	}
	if !strings.Contains(e.message, "cancelled") {
		t.Errorf("expected cancellation message, got %q", e.message)
	}
}

func TestDiredDispatchQuitSwitchesBuffer(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	if buf(e).Mode() != "dired" {
		t.Fatal("expected dired buffer active")
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Error("'q' not consumed")
	}
	if buf(e).Mode() == "dired" {
		t.Error("'q' should switch away from the dired buffer")
	}
}

func TestDiredDispatchOpenOtherWindow(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.openDired(dir)
	b := buf(e)
	ds := e.diredStates[b]
	for range 20 {
		name, ok := e.diredCurrentFile(b, ds)
		if ok && name == "a.txt" {
			break
		}
		e.cmdNextLine()
	}
	if !e.diredDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'o'}) {
		t.Error("'o' not consumed")
	}
	if len(e.windows) < 2 {
		t.Errorf("'o' should split into a second window, got %d", len(e.windows))
	}
	if !strings.HasSuffix(e.ActiveBuffer().Filename(), "a.txt") {
		t.Errorf("'o' should open a.txt in the other window, got %q", e.ActiveBuffer().Filename())
	}
}

// ---------------------------------------------------------------------------
// cmdDired
// ---------------------------------------------------------------------------

func TestCmdDired_OpensDirectory(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.cmdDired()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdDired should activate the minibuffer")
	}
	e.minibufDoneFunc(dir)
	if e.ActiveBuffer().Mode() != "dired" {
		t.Fatalf("expected a dired buffer, got mode %q", e.ActiveBuffer().Mode())
	}
}

func TestCmdDired_FilePathOpensFile(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	e.cmdDired()
	file := filepath.Join(dir, "a.txt")
	e.minibufDoneFunc(file)
	if e.ActiveBuffer().Filename() != file {
		t.Fatalf("expected file %q to be opened, got %q", file, e.ActiveBuffer().Filename())
	}
}

func TestCmdDired_EmptyUsesDefaultDir(t *testing.T) {
	dir := makeDiredFixture(t)
	e := newDiredTestEditor()
	// Give the active buffer a filename so bufferDir resolves to dir.
	buf(e).SetFilename(filepath.Join(dir, "a.txt"))
	e.cmdDired()
	e.minibufDoneFunc("")
	if e.ActiveBuffer().Mode() != "dired" {
		t.Fatalf("empty input should open dired on the default dir, got mode %q", e.ActiveBuffer().Mode())
	}
}
