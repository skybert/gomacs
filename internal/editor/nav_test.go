package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/elisp"
)

func TestCountWords_WholeBuffer(t *testing.T) {
	e := newTestEditor("hello world foo")
	e.cmdCountWords()
	if e.message == "" {
		t.Fatal("cmdCountWords produced no message")
	}
	// Should report 3 words
	if !containsStr(e.message, "3 words") {
		t.Errorf("message = %q, want it to contain \"3 words\"", e.message)
	}
}

func TestCountWords_EmptyBuffer(t *testing.T) {
	e := newTestEditor("")
	e.cmdCountWords()
	if !containsStr(e.message, "0 words") {
		t.Errorf("message = %q, want \"0 words\"", e.message)
	}
}

func TestCountBufferLines(t *testing.T) {
	e := newTestEditor("line1\nline2\nline3")
	e.cmdCountBufferLines()
	if e.message == "" {
		t.Fatal("cmdCountBufferLines produced no message")
	}
}

func TestWhatLine(t *testing.T) {
	e := newTestEditor("aaa\nbbb\nccc")
	buf(e).SetPoint(4) // start of second line
	e.cmdWhatLine()
	if !containsStr(e.message, "2") {
		t.Errorf("message = %q, want line 2", e.message)
	}
}

func TestMarkWholeBuffer(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdMarkWholeBuffer()
	b := buf(e)
	if b.Point() != 0 {
		t.Errorf("point = %d, want 0", b.Point())
	}
	if b.Mark() != b.Len() {
		t.Errorf("mark = %d, want %d (len)", b.Mark(), b.Len())
	}
	if !b.MarkActive() {
		t.Error("mark should be active after mark-whole-buffer")
	}
}

func TestMarkWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdMarkWord()
	b := buf(e)
	if !b.MarkActive() {
		t.Error("mark should be active after mark-word")
	}
}

func TestCmdGomacsVersion(t *testing.T) {
	e := newTestEditor("")
	e.cmdGomacsVersion()
	if e.message == "" {
		t.Fatal("cmdGomacsVersion produced no message")
	}
	if !containsStr(e.message, "gomacs") {
		t.Errorf("message = %q, want it to contain \"gomacs\"", e.message)
	}
}

func TestCmdWhatKey(t *testing.T) {
	e := newTestEditor("")
	e.cmdWhatKey()
	if !e.whatKeyPending {
		t.Error("cmdWhatKey: expected whatKeyPending=true")
	}
	if e.message == "" {
		t.Error("cmdWhatKey: expected a prompt message")
	}
}

func TestCmdHelp(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.cmdHelp()
	found := false
	for _, b := range e.buffers {
		if b.Name() == "*Help*" {
			found = true
			break
		}
	}
	if !found {
		t.Error("cmdHelp: expected *Help* buffer to be created")
	}
}

// containsStr is a helper used by nav tests.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && searchStr(s, substr))
}

func searchStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
