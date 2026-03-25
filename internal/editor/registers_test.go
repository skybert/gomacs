package editor

import (
	"testing"
)

// newEditorWithRegisters returns a test editor with an initialised register map.
func newEditorWithRegisters(content string) *Editor {
	e := newTestEditor(content)
	e.registers = make(map[rune]register)
	return e
}

func TestPointToRegister(t *testing.T) {
	e := newEditorWithRegisters("hello world")
	buf(e).SetPoint(5)
	e.cmdPointToRegister()
	// Simulate the char-read callback directly.
	e.readCharCallback('a')
	reg, ok := e.registers['a']
	if !ok {
		t.Fatal("register 'a' not set")
	}
	if reg.kind != "point" {
		t.Errorf("kind = %q, want \"point\"", reg.kind)
	}
	if reg.pos != 5 {
		t.Errorf("pos = %d, want 5", reg.pos)
	}
}

func TestJumpToRegister_Empty(t *testing.T) {
	e := newEditorWithRegisters("hello")
	e.cmdJumpToRegister()
	e.readCharCallback('z')
	if !containsStr(e.message, "empty") {
		t.Errorf("message = %q, want 'empty'", e.message)
	}
}

func TestJumpToRegister_Point(t *testing.T) {
	e := newEditorWithRegisters("hello world")
	buf(e).SetPoint(6)
	e.cmdPointToRegister()
	e.readCharCallback('b')
	buf(e).SetPoint(0)
	e.cmdJumpToRegister()
	e.readCharCallback('b')
	if buf(e).Point() != 6 {
		t.Errorf("point = %d, want 6", buf(e).Point())
	}
}

func TestCopyToRegister(t *testing.T) {
	e := newEditorWithRegisters("hello world")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdCopyToRegister()
	e.readCharCallback('c')
	reg, ok := e.registers['c']
	if !ok {
		t.Fatal("register 'c' not set")
	}
	if reg.kind != "text" {
		t.Errorf("kind = %q, want \"text\"", reg.kind)
	}
	if reg.text != "hello" {
		t.Errorf("text = %q, want \"hello\"", reg.text)
	}
}

func TestInsertRegister(t *testing.T) {
	e := newEditorWithRegisters("world")
	e.registers['d'] = register{kind: "text", text: "hello "}
	buf(e).SetPoint(0)
	e.cmdInsertRegister()
	e.readCharCallback('d')
	if got := buf(e).String(); got != "hello world" {
		t.Errorf("got %q, want \"hello world\"", got)
	}
}
