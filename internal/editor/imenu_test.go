package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

func TestImenuSymbolsGo(t *testing.T) {
	src := `package main

func Foo() {}
func (r *Receiver) Bar() {}
var x = 1
`
	buf := buffer.NewWithContent("test.go", src)
	buf.SetMode("go")
	entries := imenuSymbols(buf)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d: %v", len(entries), entries)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.label] = true
	}
	if !names["Foo (line 3)"] {
		t.Errorf("missing Foo entry; got %v", entries)
	}
	if !names["Bar (line 4)"] {
		t.Errorf("missing Bar entry; got %v", entries)
	}
}

func TestImenuSymbolsPython(t *testing.T) {
	src := `def greet(name):
    pass

class Animal:
    pass
`
	buf := buffer.NewWithContent("test.py", src)
	buf.SetMode("python")
	entries := imenuSymbols(buf)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if entries[0].label != "greet (line 1)" {
		t.Errorf("unexpected entry[0]: %s", entries[0].label)
	}
	if entries[1].label != "Animal (line 4)" {
		t.Errorf("unexpected entry[1]: %s", entries[1].label)
	}
}

func TestImenuSymbolsMarkdown(t *testing.T) {
	src := `# Introduction
Some text.
## Usage
More text.
### Advanced
`
	buf := buffer.NewWithContent("test.md", src)
	buf.SetMode("markdown")
	entries := imenuSymbols(buf)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(entries), entries)
	}
	if entries[0].label != "Introduction (line 1)" {
		t.Errorf("unexpected entry[0]: %s", entries[0].label)
	}
	if entries[1].label != "Usage (line 3)" {
		t.Errorf("unexpected entry[1]: %s", entries[1].label)
	}
	if entries[2].label != "Advanced (line 5)" {
		t.Errorf("unexpected entry[2]: %s", entries[2].label)
	}
}

func TestImenuSymbolsBash(t *testing.T) {
	src := `#!/bin/bash
deploy() {
    echo deploy
}
rollback() {
    echo rollback
}
`
	buf := buffer.NewWithContent("test.sh", src)
	buf.SetMode("bash")
	entries := imenuSymbols(buf)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if entries[0].label != "deploy (line 2)" {
		t.Errorf("unexpected entry[0]: %s", entries[0].label)
	}
	if entries[1].label != "rollback (line 5)" {
		t.Errorf("unexpected entry[1]: %s", entries[1].label)
	}
}

func TestImenuSymbolsElisp(t *testing.T) {
	src := `(defun my-func ()
  nil)
(defvar my-var 42)
`
	buf := buffer.NewWithContent("test.el", src)
	buf.SetMode("elisp")
	entries := imenuSymbols(buf)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if entries[0].label != "my-func (line 1)" {
		t.Errorf("unexpected entry[0]: %s", entries[0].label)
	}
	if entries[1].label != "my-var (line 3)" {
		t.Errorf("unexpected entry[1]: %s", entries[1].label)
	}
}

func TestImenuNoEntries(t *testing.T) {
	buf := buffer.NewWithContent("test.txt", "just text\n")
	buf.SetMode("fundamental")
	entries := imenuSymbols(buf)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for fundamental mode, got %d", len(entries))
	}
}

func TestLineStartOffset(t *testing.T) {
	buf := buffer.NewWithContent("test.go", "abc\ndef\nghi\n")
	if got := lineStartOffset(buf, 1); got != 0 {
		t.Errorf("line 1: want 0, got %d", got)
	}
	if got := lineStartOffset(buf, 2); got != 4 {
		t.Errorf("line 2: want 4, got %d", got)
	}
	if got := lineStartOffset(buf, 3); got != 8 {
		t.Errorf("line 3: want 8, got %d", got)
	}
}
