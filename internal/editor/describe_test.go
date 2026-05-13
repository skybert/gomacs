package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/elisp"
)

func TestCmdDescribeKey(t *testing.T) {
	e := newTestEditor("")
	e.cmdDescribeKey()
	if !e.describeKeyPending {
		t.Error("cmdDescribeKey: expected describeKeyPending=true")
	}
	if e.message == "" {
		t.Error("cmdDescribeKey: expected a prompt message")
	}
}

func TestCmdDescribeFunction(t *testing.T) {
	e := newTestEditor("")
	e.cmdDescribeFunction()
	if !e.minibufActive {
		t.Error("cmdDescribeFunction: expected minibuffer to be active")
	}
}

func TestCmdDescribeVariable(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.cmdDescribeVariable()
	if !e.minibufActive {
		t.Error("cmdDescribeVariable: expected minibuffer to be active")
	}
}

func TestCmdLoadTheme(t *testing.T) {
	e := newTestEditor("")
	e.cmdLoadTheme()
	if !e.minibufActive {
		t.Error("cmdLoadTheme: expected minibuffer to be active")
	}
}
