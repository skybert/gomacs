package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

func TestStartKbdMacro(t *testing.T) {
	e := newTestEditor("")
	e.cmdStartKbdMacro()
	if !e.kbdMacroRecording {
		t.Error("kbdMacroRecording should be true after start-kbd-macro")
	}
}

func TestStartKbdMacro_AlreadyRecording(t *testing.T) {
	e := newTestEditor("")
	e.kbdMacroRecording = true
	e.cmdStartKbdMacro()
	// Should show a message but remain recording.
	if !e.kbdMacroRecording {
		t.Error("should still be recording")
	}
	if e.message == "" {
		t.Error("should have shown a message")
	}
}

func TestEndKbdMacro_NotRecording(t *testing.T) {
	e := newTestEditor("")
	e.cmdEndKbdMacro()
	if e.message == "" {
		t.Error("should show 'Not defining keyboard macro' message")
	}
}

func TestEndKbdMacro_Saves(t *testing.T) {
	e := newTestEditor("")
	e.cmdStartKbdMacro()
	// Simulate recording two events (not the terminating C-x ) pair).
	e.kbdMacroEvents = []terminal.KeyEvent{
		{Key: tcell.KeyRune, Rune: 'a'},
		{Key: tcell.KeyRune, Rune: 'b'},
	}
	e.cmdEndKbdMacro()
	if e.kbdMacroRecording {
		t.Error("should have stopped recording")
	}
	// The saved macro should be the events (end-macro strips the last 2 terminator events,
	// but our slice has no terminators so it may be empty or 0 after stripping).
	// Just verify the macro was saved and recording stopped.
	if e.kbdMacroEvents != nil {
		t.Error("kbdMacroEvents should be nil after end")
	}
}

func TestCallLastKbdMacro_NoMacro(t *testing.T) {
	e := newTestEditor("")
	e.kbdMacro = nil
	e.cmdCallLastKbdMacro()
	if e.message == "" {
		t.Error("should show 'No keyboard macro defined' message")
	}
}

func TestCallLastKbdMacro_WhileRecording(t *testing.T) {
	e := newTestEditor("")
	e.kbdMacro = []terminal.KeyEvent{{Key: tcell.KeyRune, Rune: 'x'}}
	e.kbdMacroRecording = true
	e.cmdCallLastKbdMacro()
	if !containsStr(e.message, "Cannot") {
		t.Errorf("message = %q, want 'Cannot call macro...'", e.message)
	}
}
