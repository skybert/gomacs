package editor

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/skybert/gomacs/internal/buffer"
)

// cmdGotoLine reads a line number and moves point there (M-g g).
func (e *Editor) cmdGotoLine() {
	e.clearArg()
	e.ReadMinibuffer("Goto line: ", func(s string) {
		s = strings.TrimSpace(s)
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			e.Message("Invalid line number: %s", s)
			return
		}
		buf := e.ActiveBuffer()
		buf.SetPoint(buf.LineStart(n))
		e.Message("Line %d", n)
	})
}

// cmdWhatCursorPosition shows character and position info (C-x =).
func (e *Editor) cmdWhatCursorPosition() {
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	line, col := buf.LineCol(pt)
	total := buf.Len()
	pct := 0
	if total > 0 {
		pct = pt * 100 / total
	}
	if pt < total {
		ch := buf.RuneAt(pt)
		e.Message("Char: %c (0x%04X)  point=%d of %d (%d%%)  line=%d  col=%d",
			ch, ch, pt+1, total, pct, line, col)
	} else {
		e.Message("point=%d of %d (end)  line=%d  col=%d", pt+1, total, line, col)
	}
}

// cmdWhatLine shows the current line number.
func (e *Editor) cmdWhatLine() {
	e.clearArg()
	buf := e.ActiveBuffer()
	line, _ := buf.LineCol(buf.Point())
	e.Message("Line %d of %d", line, buf.LineCount())
}

// cmdCountWords counts words in the buffer (or region if active).
func (e *Editor) cmdCountWords() {
	e.clearArg()
	buf := e.ActiveBuffer()
	var text string
	if buf.MarkActive() {
		pt := buf.Point()
		mark := buf.Mark()
		if mark < pt {
			text = buf.Substring(mark, pt)
		} else {
			text = buf.Substring(pt, mark)
		}
	} else {
		text = buf.String()
	}
	words := 0
	lines := strings.Count(text, "\n") + 1
	chars := len([]rune(text))
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			words++
			inWord = true
		}
	}
	e.Message("%d words, %d lines, %d characters", words, lines, chars)
}

// cmdMarkWholeBuffer marks the entire buffer (C-x h).
func (e *Editor) cmdMarkWholeBuffer() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetMark(buf.Len())
	buf.SetMarkActive(true)
	buf.SetPoint(0)
	e.Message("Mark set")
}

// cmdMarkWord sets mark at the end of the next word (M-@).
func (e *Editor) cmdMarkWord() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	end := pt
	length := buf.Len()
	for range n {
		if e.subwordMode {
			end = subwordForwardOne(buf, end)
		} else {
			for end < length && !isWordRune(buf.RuneAt(end)) {
				end++
			}
			for end < length && isWordRune(buf.RuneAt(end)) {
				end++
			}
		}
	}
	if buf.Mark() >= 0 {
		buf.PushMarkRing(buf.Mark())
	}
	buf.SetMark(end)
	buf.SetMarkActive(true)
	e.Message("Mark set")
}

func (e *Editor) cmdNextError() {
	e.clearArg()
	if len(e.compilationErrors) == 0 {
		e.Message("No errors")
		return
	}
	e.compilationErrorIdx = (e.compilationErrorIdx + 1) % len(e.compilationErrors)
	e.gotoCompilationError(e.compilationErrorIdx)
}

func (e *Editor) cmdPreviousError() {
	e.clearArg()
	if len(e.compilationErrors) == 0 {
		e.Message("No errors")
		return
	}
	e.compilationErrorIdx = (e.compilationErrorIdx - 1 + len(e.compilationErrors)) % len(e.compilationErrors)
	e.gotoCompilationError(e.compilationErrorIdx)
}

// gotoCompilationError opens the source file and jumps to the error at idx.
func (e *Editor) gotoCompilationError(idx int) {
	ce := e.compilationErrors[idx]
	b, err := e.loadFile(ce.File)
	if err != nil {
		e.Message("next-error: cannot open %s: %v", ce.File, err)
		return
	}
	pos := b.LineStart(ce.Line)
	b.SetPoint(pos)
	// Show file in the active window (or upper window if split).
	for _, w := range e.windows {
		if w.Buf().Mode() != "compilation" {
			w.SetBuf(b)
			e.activeWin = w
			break
		}
	}
	if e.activeWin.Buf() != b {
		e.activeWin.SetBuf(b)
	}
	e.Message("Error %d/%d: %s:%d", idx+1, len(e.compilationErrors), ce.File, ce.Line)
}

// cmdCountBufferLines shows total lines in the buffer plus how many are before
// and after point (C-x l).
func (e *Editor) cmdCountBufferLines() {
	e.clearArg()
	buf := e.ActiveBuffer()
	line, _ := buf.LineCol(buf.Point())
	total := buf.LineCount()
	before := line - 1
	after := total - line
	e.Message("Buffer has %d lines; point on line %d (%d before, %d after)", total, line, before, after)
}

// cmdMessages switches to the *messages* buffer, creating it if needed.
func (e *Editor) cmdMessages() {
	b := e.FindBuffer("*messages*")
	if b == nil {
		b = buffer.NewWithContent("*messages*", "")
		e.buffers = append(e.buffers, b)
		b.SetReadOnly(true)
	}
	e.activeWin.SetBuf(b)
}

// cmdHelp shows a *Help* buffer listing all registered commands (with key
// bindings and documentation) and the known configuration variables (C-h h).
func (e *Editor) cmdHelp() {
	e.clearArg()
	var sb strings.Builder
	sb.WriteString("gomacs help\n")
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	type group struct {
		title    string
		commands []string
	}
	groups := []group{
		{"Navigation", []string{
			"backward-char", "forward-char", "previous-line", "next-line",
			"beginning-of-line", "end-of-line",
			"beginning-of-buffer", "end-of-buffer",
			"forward-word", "backward-word",
			"beginning-of-sentence", "end-of-sentence",
			"scroll-up", "scroll-down", "recenter",
			"goto-line", "what-line", "what-cursor-position", "count-buffer-lines",
			"back-to-indentation",
			"forward-list", "backward-list",
		}},
		{"Editing", []string{
			"newline", "self-insert-command", "open-line",
			"delete-char", "backward-delete-char",
			"kill-line", "kill-word", "backward-kill-word", "kill-sentence", "kill-region",
			"copy-region-as-kill", "yank", "yank-pop",
			"transpose-chars", "transpose-words", "join-line",
			"undo", "redo",
			"delete-blank-lines", "delete-duplicate-lines", "delete-trailing-whitespace", "sort-lines",
			"upcase-word", "downcase-word", "capitalize-word", "upcase-region", "downcase-region",
			"fill-paragraph", "set-fill-column",
		}},
		{"Search & Replace", []string{
			"isearch-forward", "isearch-backward",
			"query-replace", "replace-string",
		}},
		{"Marks & Registers", []string{
			"set-mark-command", "mark-word", "mark-whole-buffer", "exchange-point-and-mark",
			"point-to-register", "jump-to-register", "copy-to-register", "insert-register",
			"copy-rectangle-to-register",
		}},
		{"Narrowing", []string{
			"narrow-to-region", "widen",
		}},
		{"Indentation & Comments", []string{
			"indent-or-complete", "indent-region", "indent-rigidly",
			"comment-dwim",
		}},
		{"Files & Buffers", []string{
			"find-file", "save-buffer", "save-buffers-kill-terminal", "save-some-buffers",
			"kill-buffer", "switch-to-buffer", "list-buffers", "toggle-read-only",
			"dired", "messages",
		}},
		{"Windows", []string{
			"split-window-below", "split-window-right", "delete-other-windows", "other-window",
		}},
		{"Shell & Build", []string{
			"shell-command", "shell-command-on-region", "man",
			"project-build", "project-find-file", "project-grep", "next-error", "previous-error",
		}},
		{"Version Control", []string{
			"vc-print-log", "vc-diff", "vc-status", "vc-grep",
			"vc-annotate", "vc-next-action", "vc-revert",
		}},
		{"Spell Checking", []string{
			"spell", "ispell-word",
		}},
		{"LSP", []string{
			"lsp-find-definition", "lsp-pop-definition", "lsp-find-references", "lsp-hover",
		}},
		{"Keyboard Macros", []string{
			"start-kbd-macro", "end-kbd-macro", "call-last-kbd-macro",
		}},
		{"Major Modes", []string{
			"go-mode", "python-mode", "java-mode", "bash-mode", "markdown-mode",
			"elisp-mode", "text-mode", "fundamental-mode", "json-mode", "yaml-mode",
			"makefile-mode", "load-theme",
		}},
		{"Help & Info", []string{
			"help", "describe-key", "describe-function", "describe-variable",
			"gomacs-version", "count-words", "imenu",
		}},
		{"Completion", []string{
			"dabbrev-expand",
		}},
		{"Misc", []string{
			"eval-last-sexp", "execute-extended-command", "keyboard-quit", "universal-argument",
		}},
	}

	printCmd := func(name string) {
		keys := e.keysForCommand(name)
		keyStr := ""
		if len(keys) > 0 {
			keyStr = "  (" + strings.Join(keys, ", ") + ")"
		}
		doc := commandDocs[name]
		if doc == "" {
			doc = "Not documented."
		}
		fmt.Fprintf(&sb, "  %-38s%s\n    %s\n\n", name, keyStr, doc)
	}

	// Collect all commands already assigned to a group.
	assigned := make(map[string]bool)
	for _, g := range groups {
		for _, name := range g.commands {
			assigned[name] = true
		}
	}

	// Collect any registered commands not yet assigned.
	var other []string
	for name := range commands {
		if !assigned[name] {
			other = append(other, name)
		}
	}
	sort.Strings(other)

	sb.WriteString("Commands\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n\n")

	for _, g := range groups {
		sb.WriteString(g.title + "\n")
		for _, name := range g.commands {
			if _, ok := commands[name]; ok {
				printCmd(name)
			}
		}
	}
	if len(other) > 0 {
		sb.WriteString("Other\n")
		for _, name := range other {
			printCmd(name)
		}
	}

	sb.WriteString("\nConfiguration Variables\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n\n")

	type configVar struct{ name, doc string }
	configVars := []configVar{
		{"auto-revert", "When t, reload unmodified buffers if their file changes on disk. Default: t."},
		{"delete-trailing-whitespace", "When t, save-buffer strips trailing whitespace. Default: t."},
		{"fill-column", "Column target for fill-paragraph (M-q). Default: 70."},
		{"go-indent", "Indent string for Go mode. Default: \"\\t\"."},
		{"isearch-case-insensitive", "When t, isearch ignores case. Default: t."},
		{"java-indent", "Indent string or width for Java mode. Default: 4."},
		{"json-indent", "Indent string or width for JSON mode. Default: 2."},
		{"completion-menu-trigger-chars", "Minimum chars typed before completion menu appears. Default: 3."},
		{"lsp-completion-min-chars", "Alias for completion-menu-trigger-chars (deprecated name)."},
		{"markdown-indent", "Indent string or width for Markdown mode. Default: 2."},
		{"python-indent", "Indent string or width for Python mode. Default: 4."},
		{"screenshot-dir", "Directory for M-x screenshot output. Default: working directory at startup."},
		{"sh-indent", "Indent string or width for Bash mode. Default: 2."},
		{"spell-command", "Path to spell-checker executable. Default: \"aspell\"."},
		{"spell-language", "Language code for spell checker. Default: \"en\"."},
		{"subword-mode", "When t, word motion treats CamelCase sub-words. Default: t."},
		{"theme", "Color theme name. Default: \"sweet\". Set with (setq theme 'sweet)."},
		{"visual-lines", "When t, long lines wrap visually. Default: t."},
		{"yaml-indent", "Indent string or width for YAML mode. Default: 2."},
	}
	for _, cv := range configVars {
		val := "(not set)"
		if v, ok := e.lisp.GetGlobalVar(cv.name); ok {
			val = v.String()
		}
		fmt.Fprintf(&sb, "  %-42s  %s\n    Current value: %s\n\n", cv.name, cv.doc, val)
	}

	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		helpBuf = buffer.NewWithContent("*Help*", sb.String())
		e.buffers = append(e.buffers, helpBuf)
	} else {
		helpBuf.SetReadOnly(false)
		helpBuf.Delete(0, helpBuf.Len())
		helpBuf.InsertString(0, sb.String())
	}
	helpBuf.SetMode("help")
	helpBuf.SetReadOnly(true)
	helpBuf.SetPoint(0)
	e.activeWin.SetBuf(helpBuf)
}

// cmdGomacsVersion displays the gomacs version, Go runtime version, and uptime.
func (e *Editor) cmdGomacsVersion() {
	v := e.version
	if v == "" {
		v = "dev"
	}
	uptime := time.Since(e.startTime).Round(time.Second)
	e.Message("gomacs %s  Go: %s  Uptime: %v", v, runtime.Version(), uptime)
}
