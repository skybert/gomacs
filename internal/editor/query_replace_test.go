package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func keRune2(r rune) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}
}

// startQR is a helper: resets point to 0 then starts query-replace.
func startQR(e *Editor, from, to string) {
	e.ActiveBuffer().SetPoint(0)
	e.startQueryReplace(from, to)
}

func TestStartQueryReplace_NoMatch(t *testing.T) {
	e := newTestEditor("hello world")
	e.startQueryReplace("xyz", "ABC")
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false when no match found")
	}
}

func TestStartQueryReplace_WithMatch(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	if !e.queryReplaceActive {
		t.Error("queryReplaceActive should be true when a match is found")
	}
	if e.queryReplaceMatch != 0 {
		t.Errorf("queryReplaceMatch should be 0, got %d", e.queryReplaceMatch)
	}
}

func TestStartQueryReplace_EmptyFrom(t *testing.T) {
	e := newTestEditor("hello world")
	e.startQueryReplace("", "replacement")
	if e.queryReplaceActive {
		t.Error("startQueryReplace with empty from should not activate")
	}
}

func TestStartQueryReplace_SetsFromRunes(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	if string(e.queryReplaceFromRunes) != "hello" {
		t.Errorf("queryReplaceFromRunes = %q, want %q", string(e.queryReplaceFromRunes), "hello")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — y (replace and continue)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Y_ReplacesAndContinues(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	if !e.queryReplaceActive {
		t.Fatal("query replace did not activate")
	}
	// First 'y': replace first occurrence.
	e.queryReplaceHandleKey(keRune2('y'))
	got := e.ActiveBuffer().String()
	if got[:3] != "bar" {
		t.Errorf("after first y: buffer starts with %q, want %q", got[:3], "bar")
	}
	// Should have found next occurrence.
	if !e.queryReplaceActive {
		t.Error("queryReplaceActive should still be true after first replacement")
	}
}

func TestQueryReplaceHandleKey_Y_AllReplacements(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('y'))
	e.queryReplaceHandleKey(keRune2('y'))
	got := e.ActiveBuffer().String()
	want := "bar bar"
	if got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after all replacements")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — n (skip)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_N_Skips(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('n'))
	// Buffer unchanged, but moved to second occurrence.
	if e.ActiveBuffer().String() != "foo foo" {
		t.Errorf("after n: buffer changed unexpectedly: %q", e.ActiveBuffer().String())
	}
	if e.queryReplaceMatch != 4 {
		t.Errorf("queryReplaceMatch should be 4 (second 'foo'), got %d", e.queryReplaceMatch)
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — ! (replace all)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Bang_ReplacesAll(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('!'))
	got := e.ActiveBuffer().String()
	want := "bar bar bar"
	if got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after replace-all")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — . (replace one and quit)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Dot_ReplacesAndQuits(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('.'))
	got := e.ActiveBuffer().String()
	if got != "bar foo" {
		t.Errorf("buffer = %q, want %q", got, "bar foo")
	}
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after dot")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — q / C-g (quit)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Q_Quits(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('q'))
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after q")
	}
	// Buffer should be unchanged.
	if e.ActiveBuffer().String() != "foo foo" {
		t.Errorf("buffer changed after quit: %q", e.ActiveBuffer().String())
	}
}

func TestQueryReplaceHandleKey_CtrlG_Quits(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	e.queryReplaceHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after C-g")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceDoReplaceRaw
// ---------------------------------------------------------------------------

func TestQueryReplaceDoReplaceRaw_ReplacesInPlace(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	e.queryReplaceDoReplaceRaw()
	got := e.ActiveBuffer().String()
	want := "goodbye world"
	if got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}
