package editor

import (
	"os"
	"regexp"
	"sort"
	"strings"
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

// TestHelpCommandGroupsCoverAllCommands is the durable guarantee behind the
// gomacs-spec.md requirement that "all functions and variables are listed
// and logically grouped in M-x help": every command registered via
// registerCommand (commands.go's init()) must appear in exactly one
// helpCommandGroups entry, so the "Other" bucket in cmdHelp's output stays
// empty. It fails the moment someone adds a new command without grouping it,
// or accidentally lists a command under two groups.
func TestHelpCommandGroupsCoverAllCommands(t *testing.T) {
	seen := make(map[string]int)
	for _, g := range helpCommandGroups {
		for _, name := range g.commands {
			seen[name]++
		}
	}

	var duplicated []string
	for name, count := range seen {
		if count > 1 {
			duplicated = append(duplicated, name)
		}
	}
	if len(duplicated) > 0 {
		sort.Strings(duplicated)
		t.Errorf("commands listed in more than one helpCommandGroups entry: %v", duplicated)
	}

	var ungrouped []string
	for name := range commands {
		if seen[name] == 0 {
			ungrouped = append(ungrouped, name)
		}
	}
	if len(ungrouped) > 0 {
		sort.Strings(ungrouped)
		t.Errorf("commands registered via registerCommand but not present in any "+
			"helpCommandGroups entry (would fall into the \"Other\" bucket in M-x "+
			"help): %v", ungrouped)
	}
}

// TestHelpDebuggerGroupCoversDebugCommands documents that the Debugger group
// added for the debug-* commands stays complete: every registered command
// with a "debug-" prefix must be in the Debugger group, and nothing else.
func TestHelpDebuggerGroupCoversDebugCommands(t *testing.T) {
	var debuggerGroup []string
	for _, g := range helpCommandGroups {
		if g.title == "Debugger" {
			debuggerGroup = g.commands
		}
	}
	if debuggerGroup == nil {
		t.Fatal("no \"Debugger\" group found in helpCommandGroups")
	}
	inGroup := make(map[string]bool, len(debuggerGroup))
	for _, name := range debuggerGroup {
		inGroup[name] = true
	}
	for name := range commands {
		wantInGroup := strings.HasPrefix(name, "debug-")
		if wantInGroup != inGroup[name] {
			t.Errorf("command %q: in Debugger group = %v, want %v", name, inGroup[name], wantInGroup)
		}
	}
}

// TestHelpConfigVarsCoverApplyElispConfig guards CLAUDE.md's rule that every
// configuration variable read via e.lisp.GetGlobalVar(...) in
// applyElispConfig() (internal/editor/editor.go) must also be documented in
// cmdHelp's configVars listing. Rather than hand-maintaining a duplicate list
// here (which could itself rot), it parses applyElispConfig's own source and
// checks every GetGlobalVar("...") literal it finds against
// helpConfigVarGroups, the source of truth for M-x help's variable listing.
func TestHelpConfigVarsCoverApplyElispConfig(t *testing.T) {
	data, err := os.ReadFile("editor.go")
	if err != nil {
		t.Fatalf("reading editor.go: %v", err)
	}
	src := string(data)

	const fnSig = "func (e *Editor) applyElispConfig()"
	start := strings.Index(src, fnSig)
	if start == -1 {
		t.Fatal("applyElispConfig() not found in editor.go; has it been renamed or moved?")
	}
	rest := src[start:]
	end := strings.Index(rest, "\n}\n")
	if end == -1 {
		t.Fatal("could not find the end of applyElispConfig()'s body")
	}
	body := rest[:end]

	varRe := regexp.MustCompile(`GetGlobalVar\("([a-zA-Z0-9-]+)"\)`)
	matches := varRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		t.Fatal("found no GetGlobalVar(...) calls in applyElispConfig(); parsing is likely broken")
	}

	known := make(map[string]bool)
	for _, g := range helpConfigVarGroups {
		for _, cv := range g.vars {
			known[cv.name] = true
		}
	}

	seen := make(map[string]bool)
	for _, m := range matches {
		name := m[1]
		seen[name] = true
	}
	var missing []string
	for name := range seen {
		if !known[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("applyElispConfig() reads these variables via GetGlobalVar but they "+
			"are missing from helpConfigVarGroups (M-x help never documents them): %v", missing)
	}
}

// TestHelpConfigVarGroupsNoDuplicates ensures no configuration variable is
// accidentally listed twice across helpConfigVarGroups (e.g. once under the
// wrong group and once under the right one after a fix).
func TestHelpConfigVarGroupsNoDuplicates(t *testing.T) {
	seen := make(map[string]int)
	for _, g := range helpConfigVarGroups {
		for _, cv := range g.vars {
			seen[cv.name]++
		}
	}
	var duplicated []string
	for name, count := range seen {
		if count > 1 {
			duplicated = append(duplicated, name)
		}
	}
	if len(duplicated) > 0 {
		sort.Strings(duplicated)
		t.Errorf("config variables listed in more than one helpConfigVarGroups entry: %v", duplicated)
	}
}

// TestHelpConfigVarsIncludeSaveBufferDeleteTrailingWhitespace pins down the
// specific gap this change fixes: save-buffer-delete-trailing-whitespace
// (read at editor.go's applyElispConfig, distinct from the older
// delete-trailing-whitespace alias) must be documented in M-x help.
func TestHelpConfigVarsIncludeSaveBufferDeleteTrailingWhitespace(t *testing.T) {
	for _, g := range helpConfigVarGroups {
		for _, cv := range g.vars {
			if cv.name == "save-buffer-delete-trailing-whitespace" {
				return
			}
		}
	}
	t.Error("helpConfigVarGroups is missing \"save-buffer-delete-trailing-whitespace\"")
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

func TestGotoLineMoves(t *testing.T) {
	e := newTestEditor("line1\nline2\nline3\nline4\n")
	b := buf(e)
	b.SetPoint(0)

	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("3")

	want := b.LineStart(3)
	if got := b.Point(); got != want {
		t.Fatalf("goto-line 3: want point=%d, got %d", want, got)
	}
}

func TestGotoLineInvalidSetsMessage(t *testing.T) {
	e := newTestEditor("one\ntwo\n")
	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("notanumber")

	if !strings.Contains(e.message, "Invalid") {
		t.Fatalf("goto-line invalid: expected 'Invalid' in message, got %q", e.message)
	}
}

func TestGotoLineFirstLine(t *testing.T) {
	e := newTestEditor("alpha\nbeta\ngamma\n")
	b := buf(e)
	b.SetPoint(10)

	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("1")

	if got := b.Point(); got != 0 {
		t.Fatalf("goto-line 1: want point=0, got %d", got)
	}
}

func TestGotoLineSetsMessage(t *testing.T) {
	e := newTestEditor("a\nb\nc\n")
	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("2")

	if e.message == "" {
		t.Fatal("goto-line: expected message to be set")
	}
}

func TestWhatCursorPositionMidBuffer(t *testing.T) {
	e := newTestEditor("hello\nworld")
	b := buf(e)
	b.SetPoint(3)
	e.cmdWhatCursorPosition()
	if e.message == "" {
		t.Fatal("what-cursor-position: expected message to be set")
	}
	if !strings.Contains(e.message, "point=") {
		t.Errorf("what-cursor-position: expected 'point=' in message, got %q", e.message)
	}
}

func TestWhatCursorPositionAtEnd(t *testing.T) {
	e := newTestEditor("hi")
	b := buf(e)
	b.SetPoint(b.Len())
	e.cmdWhatCursorPosition()
	if !strings.Contains(e.message, "end") {
		t.Errorf("what-cursor-position at end: expected 'end' in message, got %q", e.message)
	}
}

func TestWhatCursorPositionReportsLineCol(t *testing.T) {
	e := newTestEditor("abc\ndef")
	b := buf(e)
	b.SetPoint(5) // inside "def" on line 2
	e.cmdWhatCursorPosition()
	if !strings.Contains(e.message, "line=") {
		t.Errorf("what-cursor-position: expected 'line=' in message, got %q", e.message)
	}
}

func TestCountWordsFiveWords(t *testing.T) {
	e := newTestEditor("one two three four five")
	e.cmdCountWords()
	if !strings.Contains(e.message, "5") {
		t.Errorf("count-words: expected '5' in message, got %q", e.message)
	}
}

func TestCountWordsRegionTwoWords(t *testing.T) {
	e := newTestEditor("alpha beta gamma delta")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(10) // "alpha beta" — 2 words
	e.cmdCountWords()
	if !strings.Contains(e.message, "2") {
		t.Errorf("count-words region: expected '2' in message, got %q", e.message)
	}
}

func TestCmdMessagesCreatesBuffer(t *testing.T) {
	e := newTestEditor("some content")
	e.cmdMessages()
	active := e.ActiveBuffer()
	if active.Name() != "*messages*" {
		t.Fatalf("messages: want active buffer '*messages*', got %q", active.Name())
	}
}

func TestCmdMessagesNoDuplicateBuffers(t *testing.T) {
	e := newTestEditor("content")
	e.cmdMessages()
	e.cmdMessages() // second call should reuse existing buffer
	count := 0
	for _, b := range e.buffers {
		if b.Name() == "*messages*" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("messages: expected exactly 1 *messages* buffer, got %d", count)
	}
}

func TestCmdMessagesBufferIsReadOnly(t *testing.T) {
	e := newTestEditor("content")
	e.cmdMessages()
	msgBuf := e.FindBuffer("*messages*")
	if msgBuf == nil {
		t.Fatal("messages: *messages* buffer not found")
	}
	if !msgBuf.ReadOnly() {
		t.Fatal("messages: *messages* buffer should be read-only")
	}
}
