package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/elisp"
)

// newEditorWithLisp returns a test editor with an initialised Elisp evaluator.
func newEditorWithLisp(content string) *Editor {
	e := newTestEditor(content)
	e.lisp = elisp.NewEvaluator()
	return e
}

func TestLangModeByName_Known(t *testing.T) {
	for _, name := range []string{"go", "python", "java", "bash", "markdown", "elisp", "json", "yaml", "makefile", "text", "fundamental"} {
		if m := langModeByName(name); m == nil {
			t.Errorf("langModeByName(%q) = nil, want a result", name)
		}
	}
}

func TestLangModeByName_Unknown(t *testing.T) {
	if m := langModeByName("cobol"); m != nil {
		t.Errorf("langModeByName(\"cobol\") = %v, want nil", m)
	}
}

func TestModeIndentStr_GoDefault(t *testing.T) {
	e := newEditorWithLisp("")
	if got := e.modeIndentStr("go"); got != "\t" {
		t.Errorf("go default = %q, want \"\\t\"", got)
	}
}

func TestModeIndentStr_OtherDefault(t *testing.T) {
	e := newEditorWithLisp("")
	if got := e.modeIndentStr("python"); got != "  " {
		t.Errorf("python default = %q, want \"  \"", got)
	}
}

func TestModeIndentStr_ElispInt(t *testing.T) {
	e := newEditorWithLisp("")
	// Set python-indent to 4 via the evaluator.
	_, err := e.lisp.EvalString("(setq python-indent 4)")
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("python"); got != "    " {
		t.Errorf("python-indent=4: got %q, want \"    \"", got)
	}
}

func TestModeIndentStr_ElispString(t *testing.T) {
	e := newEditorWithLisp("")
	_, err := e.lisp.EvalString(`(setq go-indent "    ")`)
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("go"); got != "    " {
		t.Errorf("go-indent string: got %q, want \"    \"", got)
	}
}

func TestModeIndentStr_BashUsesSh(t *testing.T) {
	e := newEditorWithLisp("")
	_, err := e.lisp.EvalString("(setq sh-indent 4)")
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("bash"); got != "    " {
		t.Errorf("bash (sh-indent=4): got %q, want \"    \"", got)
	}
}

func TestCmdGoMode(t *testing.T) {
	e := newTestEditor("package main")
	e.cmdGoMode()
	if buf(e).Mode() != "go" {
		t.Errorf("mode = %q, want \"go\"", buf(e).Mode())
	}
}

func TestCmdPythonMode(t *testing.T) {
	e := newTestEditor("x = 1")
	e.cmdPythonMode()
	if buf(e).Mode() != "python" {
		t.Errorf("mode = %q, want \"python\"", buf(e).Mode())
	}
}
