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

	sb.WriteString("Commands\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n\n")

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		keys := e.keysForCommand(name)
		keyStr := ""
		if len(keys) > 0 {
			keyStr = "  (" + strings.Join(keys, ", ") + ")"
		}
		doc := commandDocs[name]
		if doc == "" {
			doc = "Not documented."
		}
		fmt.Fprintf(&sb, "%-40s%s\n  %s\n\n", name, keyStr, doc)
	}

	sb.WriteString("\nConfiguration Variables\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n\n")

	type configVar struct{ name, doc string }
	configVars := []configVar{
		{"fill-column", "Column target for fill-paragraph (M-q). Default: 70."},
		{"isearch-case-insensitive", "When t, isearch ignores case. Default: t."},
		{"save-buffer-delete-trailing-whitespace", "When t, save-buffer strips trailing whitespace. Default: t."},
		{"visual-lines", "When t, long lines wrap visually. Default: t."},
		{"spell-command", "Path to spell-checker executable. Default: \"aspell\"."},
		{"spell-language", "Language code for spell checker. Default: \"en\"."},
		{"go-indent", "Indent string for Go mode. Default: \"\\t\"."},
		{"python-indent", "Indent string or width for Python mode. Default: 4."},
		{"java-indent", "Indent string or width for Java mode. Default: 4."},
		{"sh-indent", "Indent string or width for Bash mode. Default: 2."},
		{"json-indent", "Indent string or width for JSON mode. Default: 2."},
		{"markdown-indent", "Indent string or width for Markdown mode. Default: 2."},
		{"yaml-indent", "Indent string or width for YAML mode. Default: 2."},
		{"theme", "Color theme name. Default: \"sweet\". Set with (setq theme 'sweet)."},
		{"lsp-completion-min-chars", "Minimum chars before LSP completion triggers. Default: 1."},
	}
	for _, cv := range configVars {
		val := "(not set)"
		if v, ok := e.lisp.GetGlobalVar(cv.name); ok {
			val = v.String()
		}
		fmt.Fprintf(&sb, "%-44s  %s\n  Current value: %s\n\n", cv.name, cv.doc, val)
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
