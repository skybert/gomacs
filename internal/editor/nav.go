package editor

import (
	"runtime"
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
		for end < length && !isWordRune(buf.RuneAt(end)) {
			end++
		}
		for end < length && isWordRune(buf.RuneAt(end)) {
			end++
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
	e.Message("next-error: not yet implemented")
}

func (e *Editor) cmdPreviousError() {
	e.clearArg()
	e.Message("previous-error: not yet implemented")
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

// cmdGomacsVersion displays the gomacs version, Go runtime version, and uptime.
func (e *Editor) cmdGomacsVersion() {
	v := e.version
	if v == "" {
		v = "dev"
	}
	uptime := time.Since(e.startTime).Round(time.Second)
	e.Message("gomacs %s  Go: %s  Uptime: %v", v, runtime.Version(), uptime)
}
