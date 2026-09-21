package editor

import (
	"strings"
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

func TestImenuSymbolsJava(t *testing.T) {
	src := `public class Greeter {
    public void greet(String name) {
        System.out.println(name);
    }
    private int count(List<String> items) {
        return items.size();
    }
}
`
	b := buffer.NewWithContent("Greeter.java", src)
	b.SetMode("java")
	entries := imenuSymbols(b)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d: %v", len(entries), entries)
	}
	names := map[string]bool{}
	for _, en := range entries {
		names[en.label] = true
	}
	if !names["greet (line 2)"] {
		t.Errorf("missing greet entry; got %v", entries)
	}
	if !names["count (line 5)"] {
		t.Errorf("missing count entry; got %v", entries)
	}
}

func TestImenuSymbolsJava_Static(t *testing.T) {
	src := `class Util {
    public static String format(String s) {
        return s;
    }
}
`
	b := buffer.NewWithContent("Util.java", src)
	b.SetMode("java")
	entries := imenuSymbols(b)
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	if entries[0].label != "format (line 2)" {
		t.Errorf("unexpected entry: %s", entries[0].label)
	}
}

func TestLineStartOffset_BeyondEnd(t *testing.T) {
	b := buffer.NewWithContent("x", "abc")
	// Asking for a line beyond the buffer should return buf length.
	got := lineStartOffset(b, 99)
	if got != 3 {
		t.Errorf("beyond end: want 3, got %d", got)
	}
}

func TestLineStartOffset_SingleLine(t *testing.T) {
	b := buffer.NewWithContent("x", "hello")
	if got := lineStartOffset(b, 1); got != 0 {
		t.Errorf("single line, line 1: want 0, got %d", got)
	}
}

func TestLineStartOffset_MultilineUnicode(t *testing.T) {
	// "hé\nworld\n" — 'é' is one rune, so line 2 starts at rune offset 3.
	b := buffer.NewWithContent("x", "hé\nworld\n")
	if got := lineStartOffset(b, 2); got != 3 {
		t.Errorf("unicode: want 3, got %d", got)
	}
}

// ============================================================================
// cmdImenu — entries present
// ============================================================================

// TestCmdImenu_WithGoEntries verifies that calling cmdImenu on a Go-mode
// buffer with at least one function declaration opens the minibuffer and
// offers completions.
func TestCmdImenu_WithGoEntries(t *testing.T) {
	src := "package main\n\nfunc HelloWorld() {}\nfunc Goodbye() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Go entries: expected minibufActive=true")
	}
	if e.minibufPrompt == "" {
		t.Error("cmdImenu: prompt should not be empty")
	}
	if !strings.Contains(e.minibufPrompt, "imenu") {
		t.Errorf("cmdImenu: prompt %q should contain 'imenu'", e.minibufPrompt)
	}
}

// TestCmdImenu_WithMarkdownEntries verifies behaviour in Markdown mode.
func TestCmdImenu_WithMarkdownEntries(t *testing.T) {
	src := "# Introduction\n\nSome text.\n\n## Usage\n\nMore text.\n"
	e := newTestEditor(src)
	buf(e).SetMode("markdown")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Markdown entries: expected minibufActive=true")
	}
}

// TestCmdImenu_WithElispEntries verifies behaviour in Elisp mode.
func TestCmdImenu_WithElispEntries(t *testing.T) {
	src := "(defun my-func ()\n  nil)\n(defvar my-var 42)\n"
	e := newTestEditor(src)
	buf(e).SetMode("elisp")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Elisp entries: expected minibufActive=true")
	}
}

// TestCmdImenu_CompletionCallbackFilters verifies that the completion function
// registered by cmdImenu returns only labels that fuzzy-match the query.
func TestCmdImenu_CompletionCallbackFilters(t *testing.T) {
	src := "package main\n\nfunc HelloWorld() {}\nfunc Goodbye() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	e.cmdImenu()

	if e.minibufCompletions == nil {
		t.Fatal("cmdImenu: minibufCompletions should be set")
	}

	// "Hello" should match "HelloWorld (line 3)" but not "Goodbye (line 4)".
	results := e.minibufCompletions("Hello")
	found := false
	for _, r := range results {
		if strings.Contains(r, "HelloWorld") {
			found = true
		}
		if strings.Contains(r, "Goodbye") {
			t.Errorf("cmdImenu completions: 'Hello' query should not return Goodbye entry, got %v", results)
		}
	}
	if !found {
		t.Errorf("cmdImenu completions: 'Hello' query should return HelloWorld entry, got %v", results)
	}
}

// TestCmdImenu_CallbackNavigatesToLine verifies that finishing the minibuffer
// with a valid label moves point to the start of the corresponding line.
func TestCmdImenu_CallbackNavigatesToLine(t *testing.T) {
	// "package main\n" is 13 chars; "func HelloWorld" starts at rune 14.
	src := "package main\n\nfunc HelloWorld() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	// Move point away from the target.
	buf(e).SetPoint(0)

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu: minibuf should be active")
	}

	// Simulate selecting "HelloWorld (line 3)" — the callback should move point.
	entries := imenuSymbols(buf(e))
	if len(entries) == 0 {
		t.Fatal("no imenu entries found in Go buffer")
	}
	var target imenuEntry
	for _, en := range entries {
		if strings.Contains(en.label, "HelloWorld") {
			target = en
			break
		}
	}
	if target.label == "" {
		t.Fatal("HelloWorld imenu entry not found")
	}

	// Invoke the callback directly (simulates user pressing Enter).
	e.minibufDoneFunc(target.label)

	wantPt := lineStartOffset(buf(e), target.line)
	if got := buf(e).Point(); got != wantPt {
		t.Errorf("cmdImenu navigation: want point=%d, got %d", wantPt, got)
	}
}

// TestCmdImenu_CallbackUnknownLabelIgnored verifies that an unknown label
// passed to the callback does not move point or panic.
func TestCmdImenu_CallbackUnknownLabelIgnored(t *testing.T) {
	src := "package main\n\nfunc Foo() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	buf(e).SetPoint(5)
	e.cmdImenu()

	// Passing a label that doesn't match any entry should be a no-op.
	ptBefore := buf(e).Point()
	e.minibufDoneFunc("this label does not exist")
	if got := buf(e).Point(); got != ptBefore {
		t.Errorf("cmdImenu unknown label: point should not change; before=%d after=%d", ptBefore, got)
	}
}

// TestCmdImenu_PythonWithEntries verifies Imenu in Python mode.
func TestCmdImenu_PythonWithEntries(t *testing.T) {
	src := "def greet(name):\n    pass\n\nclass Animal:\n    pass\n"
	e := newTestEditor(src)
	buf(e).SetMode("python")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Python entries: expected minibufActive=true")
	}
}

// TestCmdImenu_BashWithEntries verifies Imenu in Bash mode.
func TestCmdImenu_BashWithEntries(t *testing.T) {
	src := "#!/bin/bash\ndeploy() {\n    echo deploy\n}\nrollback() {\n    echo rollback\n}\n"
	e := newTestEditor(src)
	buf(e).SetMode("bash")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Bash entries: expected minibufActive=true")
	}
}

// TestCmdImenu_NoEntries verifies that cmdImenu on a mode with no imenu
// entries (fundamental) reports a message instead of activating the
// minibuffer.
func TestCmdImenu_NoEntries(t *testing.T) {
	e := newTestEditor("hello world")
	// fundamental mode has no imenu entries.
	e.cmdImenu()
	if e.minibufActive {
		t.Error("cmdImenu with no entries: minibuf should not be active")
	}
	if e.message == "" {
		t.Error("cmdImenu with no entries: expected a message")
	}
}

// ============================================================================
// clipboardCmd
// ============================================================================

// ============================================================================
// cmdProjectFindFile
// ============================================================================

// ============================================================================
// promptSaveNext
// ============================================================================
