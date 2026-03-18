package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---------------------------------------------------------------------------
// Navigation extras
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mark extras
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Editing extras
// ---------------------------------------------------------------------------

// cmdTransposeWords transposes the words around point (M-t).
func (e *Editor) cmdTransposeWords() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()

	// Find the end of the second word (forward from pt).
	pos := pt
	// Skip non-word to find start of first word.
	for pos < length && !isWordRune(buf.RuneAt(pos)) {
		pos++
	}
	// Skip first word.
	w1Start := pos
	for pos < length && isWordRune(buf.RuneAt(pos)) {
		pos++
	}
	w1End := pos
	if w1Start == w1End {
		e.Message("No words to transpose")
		return
	}
	// Skip non-word to second word.
	for pos < length && !isWordRune(buf.RuneAt(pos)) {
		pos++
	}
	w2Start := pos
	for pos < length && isWordRune(buf.RuneAt(pos)) {
		pos++
	}
	w2End := pos
	if w2Start == w2End {
		e.Message("No second word to transpose")
		return
	}

	w1 := buf.Substring(w1Start, w1End)
	w2 := buf.Substring(w2Start, w2End)
	// Replace from right to left to keep positions valid.
	buf.Delete(w2Start, w2End-w2Start)
	buf.InsertString(w2Start, w1)
	buf.Delete(w1Start, w1End-w1Start)
	buf.InsertString(w1Start, w2)
	buf.SetPoint(w2Start + len([]rune(w1)))
}

// cmdDeleteBlankLines deletes blank lines around point (C-x C-o).
// If on a blank line, deletes all surrounding blank lines.
// If on a non-blank line, deletes all blank lines that follow it.
func (e *Editor) cmdDeleteBlankLines() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	onBlank := bol == eol // entire line is empty

	if !onBlank {
		// Delete blank lines immediately following the current line.
		pos := eol
		if pos < buf.Len() {
			pos++ // skip this line's newline
		}
		for pos < buf.Len() {
			lineEOL := buf.EndOfLine(pos)
			if pos != lineEOL { // non-blank line
				break
			}
			// delete the newline that ended the previous line + this empty line
			buf.Delete(pos-1, 1)
		}
		return
	}

	// Delete all consecutive blank lines around this position.
	// Find start of blank region going backward.
	start := bol
	for start > 0 {
		prevBOL := buf.BeginningOfLine(start - 1)
		prevEOL := buf.EndOfLine(prevBOL)
		if prevBOL < prevEOL {
			break // previous line has content
		}
		start = prevBOL
	}
	// Find end going forward.
	end := eol
	for end < buf.Len() {
		nextBOL := end + 1
		if nextBOL >= buf.Len() {
			break
		}
		nextEOL := buf.EndOfLine(nextBOL)
		if nextBOL < nextEOL {
			break // next line has content
		}
		end = nextEOL
	}
	if end > start {
		buf.Delete(start, end-start)
		// Insert one blank line.
		buf.InsertString(start, "\n")
		buf.SetPoint(start)
	}
}

// deleteTrailingWhitespace removes trailing whitespace from every line in
// [regionStart, regionEnd).  It works backward through the buffer so that
// deletions do not shift the positions of lines yet to be processed.
func (e *Editor) deleteTrailingWhitespace(buf *buffer.Buffer, regionStart, regionEnd int) {
	// Collect (eol, trailingCount) pairs working backward.
	type span struct{ end, count int }
	var spans []span
	pos := regionStart
	for pos < regionEnd {
		eol := buf.EndOfLine(pos)
		// Count trailing horizontal whitespace (space / tab) before eol.
		count := 0
		for eol-count-1 >= pos {
			r := buf.RuneAt(eol - count - 1)
			if r != ' ' && r != '\t' {
				break
			}
			count++
		}
		if count > 0 {
			spans = append(spans, span{eol, count})
		}
		pos = eol + 1 // advance past newline
	}
	// Delete from the end so positions remain valid.
	for i := len(spans) - 1; i >= 0; i-- {
		buf.Delete(spans[i].end-spans[i].count, spans[i].count)
	}
}

// cmdDeleteTrailingWhitespace removes trailing whitespace from the buffer or
// the active region (M-x delete-trailing-whitespace).
func (e *Editor) cmdDeleteTrailingWhitespace() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := 0, buf.Len()
	if buf.MarkActive() && buf.Mark() >= 0 {
		start = buf.Mark()
		end = buf.Point()
		if start > end {
			start, end = end, start
		}
	}
	e.deleteTrailingWhitespace(buf, start, end)
}

// cmdJoinLine merges the current line with the previous one (M-^).
func (e *Editor) cmdJoinLine() {
	if e.bufReadOnly() {
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	for range n {
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		if bol == 0 {
			return
		}
		// Position of the newline that ends the previous line.
		nlPos := bol - 1
		// Count leading whitespace on current line.
		leadEnd := bol
		for leadEnd < buf.Len() && (buf.RuneAt(leadEnd) == ' ' || buf.RuneAt(leadEnd) == '\t') {
			leadEnd++
		}
		// Delete from nl through leading whitespace, then insert single space.
		count := leadEnd - nlPos
		buf.Delete(nlPos, count)
		// Insert a space unless joining at paren boundary.
		if nlPos > 0 && nlPos < buf.Len() {
			prev := buf.RuneAt(nlPos - 1)
			next := buf.RuneAt(nlPos)
			if prev != '(' && next != ')' {
				buf.InsertString(nlPos, " ")
			}
		}
		buf.SetPoint(nlPos)
	}
}

// cmdBackToIndentation moves point to the first non-whitespace char on the line (M-m).
func (e *Editor) cmdBackToIndentation() {
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	pos := bol
	for pos < eol && (buf.RuneAt(pos) == ' ' || buf.RuneAt(pos) == '\t') {
		pos++
	}
	buf.SetPoint(pos)
}

// cmdUpcaseRegion converts text in the region to upper case (C-x C-u).
func (e *Editor) cmdUpcaseRegion() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := regionBounds(buf)
	if start == end {
		return
	}
	text := strings.ToUpper(buf.Substring(start, end))
	buf.Delete(start, end-start)
	buf.InsertString(start, text)
	buf.SetPoint(end)
	buf.SetMarkActive(false)
}

// cmdDowncaseRegion converts text in the region to lower case (C-x C-l).
func (e *Editor) cmdDowncaseRegion() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := regionBounds(buf)
	if start == end {
		return
	}
	text := strings.ToLower(buf.Substring(start, end))
	buf.Delete(start, end-start)
	buf.InsertString(start, text)
	buf.SetPoint(end)
	buf.SetMarkActive(false)
}

// regionBounds returns the [start, end) of the active region (or point,point if none).
func regionBounds(buf *buffer.Buffer) (start, end int) {
	pt := buf.Point()
	mark := buf.Mark()
	if !buf.MarkActive() || mark < 0 {
		return pt, pt
	}
	if mark < pt {
		return mark, pt
	}
	return pt, mark
}

// cmdSortLines sorts the lines in the active region lexicographically.
// If no region is active, the entire buffer is sorted.
func (e *Editor) cmdSortLines() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := regionBounds(buf)
	if start == end {
		// No region: sort whole buffer.
		start, end = 0, buf.Len()
	}
	text := buf.Substring(start, end)
	// Preserve a trailing newline if present; sort the rest.
	trailing := ""
	if len(text) > 0 && text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
		trailing = "\n"
	}
	lines := strings.Split(text, "\n")
	sort.Strings(lines)
	sorted := strings.Join(lines, "\n") + trailing
	buf.Delete(start, end-start)
	buf.InsertString(start, sorted)
	buf.SetPoint(start)
	buf.SetMarkActive(false)
	e.Message("Sorted %d lines", len(lines))
}

// cmdDeleteDuplicateLines removes duplicate adjacent lines from the active region.
// Lines are compared exactly (case-sensitive).  If no region is active the
// entire buffer is processed.
func (e *Editor) cmdDeleteDuplicateLines() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := regionBounds(buf)
	if start == end {
		start, end = 0, buf.Len()
	}
	text := buf.Substring(start, end)
	trailing := ""
	if len(text) > 0 && text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
		trailing = "\n"
	}
	lines := strings.Split(text, "\n")
	seen := make(map[string]bool, len(lines))
	unique := lines[:0]
	for _, l := range lines {
		if !seen[l] {
			seen[l] = true
			unique = append(unique, l)
		}
	}
	removed := len(lines) - len(unique)
	deduped := strings.Join(unique, "\n") + trailing
	buf.Delete(start, end-start)
	buf.InsertString(start, deduped)
	buf.SetPoint(start)
	buf.SetMarkActive(false)
	e.Message("Deleted %d duplicate line(s)", removed)
}

// cmdFillParagraph re-flows the current paragraph to fit within fillColumn (M-q).
func (e *Editor) cmdFillParagraph() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	text := buf.String()
	runes := []rune(text)
	total := len(runes)

	// Find paragraph boundaries: blank lines (or buffer start/end).
	paraStart := pt
	for paraStart > 0 {
		if runes[paraStart-1] == '\n' {
			// Check if the previous line is blank.
			lineStart := buf.BeginningOfLine(paraStart - 1)
			lineEnd := buf.EndOfLine(lineStart)
			if lineStart == lineEnd {
				break
			}
		}
		paraStart--
	}
	// Clamp to beginning of line.
	paraStart = buf.BeginningOfLine(paraStart)

	paraEnd := pt
	for paraEnd < total {
		if runes[paraEnd] == '\n' {
			nextLineStart := paraEnd + 1
			if nextLineStart >= total {
				paraEnd = total
				break
			}
			nextLineEnd := buf.EndOfLine(nextLineStart)
			if nextLineStart == nextLineEnd {
				break
			}
		}
		paraEnd++
	}

	if paraStart >= paraEnd {
		return
	}

	paraText := string(runes[paraStart:paraEnd])

	// Collect all words from the paragraph.
	words := strings.Fields(paraText)
	if len(words) == 0 {
		return
	}

	// Re-flow words into lines of at most fillColumn characters.
	col := e.fillColumn
	var sb strings.Builder
	lineLen := 0
	for i, w := range words {
		wlen := len([]rune(w))
		switch {
		case i == 0:
			sb.WriteString(w)
			lineLen = wlen
		case lineLen+1+wlen <= col:
			sb.WriteByte(' ')
			sb.WriteString(w)
			lineLen += 1 + wlen
		default:
			sb.WriteByte('\n')
			sb.WriteString(w)
			lineLen = wlen
		}
	}
	filled := sb.String()

	buf.Delete(paraStart, paraEnd-paraStart)
	buf.InsertString(paraStart, filled)
	buf.SetPoint(paraStart + len([]rune(filled)))
}

// cmdSetFillColumn sets the fill column (C-x f).
func (e *Editor) cmdSetFillColumn() {
	if e.universalArgSet {
		col := e.universalArg
		e.clearArg()
		e.fillColumn = col
		e.Message("Fill column set to %d", col)
		return
	}
	e.clearArg()
	_, col := e.ActiveBuffer().LineCol(e.ActiveBuffer().Point())
	e.fillColumn = col
	e.Message("Fill column set to %d (current column)", col)
}

// cmdIndentRegion indents every line in the region by one tab stop (C-M-\).
func (e *Editor) cmdIndentRegion() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := regionBounds(buf)
	if start == end {
		return
	}

	// Walk lines from start to end and indent each.
	line, _ := buf.LineCol(start)
	endLine, _ := buf.LineCol(end)
	for l := line; l <= endLine; l++ {
		linePos := buf.LineStart(l)
		if linePos >= buf.Len() {
			break
		}
		// Only indent non-empty lines.
		eol := buf.EndOfLine(linePos)
		if linePos < eol {
			buf.InsertString(linePos, "\t")
		}
	}
	buf.SetMarkActive(false)
}

// cmdIndentRigidly indents/dedents the region rigidly (C-x TAB).
func (e *Editor) cmdIndentRigidly() {
	if e.bufReadOnly() {
		return
	}
	n := 1
	if e.universalArgSet {
		n = e.universalArg
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	start, end := regionBounds(buf)
	if start == end {
		return
	}

	indent := strings.Repeat(" ", abs(n))
	line, _ := buf.LineCol(start)
	endLine, _ := buf.LineCol(end)

	for l := line; l <= endLine; l++ {
		linePos := buf.LineStart(l)
		if linePos >= buf.Len() {
			break
		}
		eol := buf.EndOfLine(linePos)
		if linePos == eol {
			continue // skip blank lines
		}
		if n > 0 {
			buf.InsertString(linePos, indent)
		} else {
			// Remove up to |n| spaces/tabs from start of line.
			removed := 0
			for removed < abs(n) && linePos+removed < eol {
				r := buf.RuneAt(linePos + removed)
				if r != ' ' && r != '\t' {
					break
				}
				removed++
			}
			if removed > 0 {
				buf.Delete(linePos, removed)
			}
		}
	}
	buf.SetMarkActive(false)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ---------------------------------------------------------------------------
// Replace commands
// ---------------------------------------------------------------------------

// cmdReplaceString replaces all occurrences of FROM with TO from point onward.
func (e *Editor) cmdReplaceString() {
	e.ReadMinibuffer("Replace string: ", func(from string) {
		if from == "" {
			return
		}
		e.ReadMinibuffer(fmt.Sprintf("Replace string %s with: ", from), func(to string) {
			buf := e.ActiveBuffer()
			if buf.ReadOnly() {
				e.Message("Buffer is read-only")
				return
			}
			start := buf.Point()
			text := buf.String()
			runes := []rune(text)
			needle := []rune(from)
			replacement := []rune(to)
			count := 0
			pos := start
			for pos <= len(runes)-len(needle) {
				if runesMatch(runes[pos:], needle) {
					buf.Delete(pos, len(needle))
					buf.InsertString(pos, to)
					runes = []rune(buf.String())
					pos += len(replacement)
					count++
				} else {
					pos++
				}
			}
			e.Message("Replaced %d occurrence(s)", count)
		})
	})
}

// ---------------------------------------------------------------------------
// Narrowing commands
// ---------------------------------------------------------------------------

// cmdNarrowToRegion restricts the buffer to the active region (C-x n n).
func (e *Editor) cmdNarrowToRegion() {
	e.clearArg()
	buf := e.ActiveBuffer()
	if !buf.MarkActive() {
		e.Message("Mark not active")
		return
	}
	start, end := regionBounds(buf)
	buf.Narrow(start, end)
	buf.SetMarkActive(false)
	e.Message("Narrowed to region")
}

// cmdWiden cancels narrowing (C-x n w).
func (e *Editor) cmdWiden() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.Widen()
	e.Message("Widened")
}

// ---------------------------------------------------------------------------
// Register commands
// ---------------------------------------------------------------------------

// register holds a single register value (position or text).
type register struct {
	kind string // "point" or "text"
	pos  int
	text string
	buf  string // buffer name for point registers
}

// cmdPointToRegister stores the current point in a register (C-x r SPC).
func (e *Editor) cmdPointToRegister() {
	e.clearArg()
	e.Message("Point to register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		buf := e.ActiveBuffer()
		e.registers[r] = register{
			kind: "point",
			pos:  buf.Point(),
			buf:  buf.Name(),
		}
		e.Message("Saved point to register %c", r)
	}
}

// cmdJumpToRegister jumps to a position stored in a register (C-x r j).
func (e *Editor) cmdJumpToRegister() {
	e.clearArg()
	e.Message("Jump to register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		reg, ok := e.registers[r]
		if !ok {
			e.Message("Register %c is empty", r)
			return
		}
		switch reg.kind {
		case "point":
			if reg.buf != e.ActiveBuffer().Name() {
				e.SwitchToBuffer(reg.buf)
			}
			e.ActiveBuffer().SetPoint(reg.pos)
		case "text":
			e.Message("Register %c contains text, use insert-register", r)
		}
	}
}

// cmdCopyToRegister saves the region text to a register (C-x r s).
func (e *Editor) cmdCopyToRegister() {
	e.clearArg()
	e.Message("Copy to register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		buf := e.ActiveBuffer()
		start, end := regionBounds(buf)
		if start == end {
			e.Message("No region")
			return
		}
		text := buf.Substring(start, end)
		e.registers[r] = register{kind: "text", text: text}
		e.Message("Copied region to register %c", r)
	}
}

// cmdInsertRegister inserts the text stored in a register (C-x r i).
func (e *Editor) cmdInsertRegister() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	e.Message("Insert register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		reg, ok := e.registers[r]
		if !ok {
			e.Message("Register %c is empty", r)
			return
		}
		if reg.kind != "text" {
			e.Message("Register %c does not contain text", r)
			return
		}
		buf := e.ActiveBuffer()
		pt := buf.Point()
		buf.InsertString(pt, reg.text)
		buf.SetPoint(pt + len([]rune(reg.text)))
	}
}

// cmdCopyRectangleToRegister is a stub (C-x r r).
func (e *Editor) cmdCopyRectangleToRegister() {
	e.clearArg()
	e.Message("Rectangle registers not yet implemented")
}

// ---------------------------------------------------------------------------
// Shell commands
// ---------------------------------------------------------------------------

// cmdShellCommand runs a shell command and shows output (M-!).
func (e *Editor) cmdShellCommand() {
	e.clearArg()
	e.ReadMinibuffer("Shell command: ", func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return
		}
		ctx := context.Background()
		out, err := shellRun(ctx, cmd, "")
		result := out
		if err != nil && result == "" {
			result = err.Error()
		}
		outBuf := e.FindBuffer("*Shell Command Output*")
		if outBuf == nil {
			outBuf = buffer.NewWithContent("*Shell Command Output*", result)
			e.buffers = append(e.buffers, outBuf)
		} else {
			outBuf.Delete(0, outBuf.Len())
			outBuf.InsertString(0, result)
		}
		outBuf.SetPoint(0)
		e.activeWin.SetBuf(outBuf)
	})
}

// cmdShellCommandOnRegion pipes the region through a shell command (M-|).
func (e *Editor) cmdShellCommandOnRegion() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	e.ReadMinibuffer("Shell command on region: ", func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return
		}
		buf := e.ActiveBuffer()
		start, end := regionBounds(buf)
		if start == end {
			e.Message("No region")
			return
		}
		input := buf.Substring(start, end)
		ctx := context.Background()
		result, err := shellRun(ctx, cmd, input)
		if err != nil && result == "" {
			e.Message("Shell error: %v", err)
			return
		}
		buf.Delete(start, end-start)
		buf.InsertString(start, result)
		buf.SetPoint(start + len([]rune(result)))
		buf.SetMarkActive(false)
		e.Message("Shell command done")
	})
}

// ---------------------------------------------------------------------------
// Next/previous error stubs
// ---------------------------------------------------------------------------

func (e *Editor) cmdNextError() {
	e.clearArg()
	e.Message("next-error: not yet implemented")
}

func (e *Editor) cmdPreviousError() {
	e.clearArg()
	e.Message("previous-error: not yet implemented")
}

// cmdIspellWord is a stub for M-$.
func (e *Editor) cmdIspellWord() {
	e.clearArg()
	e.Message("ispell-word: not yet implemented")
}

// shellRun runs cmd via sh -c with optional stdin text, returns combined output.
func shellRun(ctx context.Context, cmd, stdin string) (string, error) {
	sh := exec.CommandContext(ctx, "sh", "-c", cmd) //nolint:gosec
	if stdin != "" {
		sh.Stdin = strings.NewReader(stdin)
	}
	out, err := sh.CombinedOutput()
	return string(out), err
}

// ---------------------------------------------------------------------------
// Version control (C-x v)
// ---------------------------------------------------------------------------

// vcBackend is the interface for a version control system backend.
// Adding support for a new VCS (e.g. Mercurial) means implementing this
// interface and appending an instance to vcBackends.
type vcBackend interface {
	// Name returns the VCS identifier (e.g. "git").
	Name() string
	// Root walks upward from dir looking for a repo root; returns "" if not found.
	Root(dir string) string
	// Status returns the full status output.
	Status(root string) (string, error)
	// Diff returns uncommitted changes, optionally scoped to filePath.
	Diff(root, filePath string) (string, error)
	// Log returns a short commit log, optionally scoped to filePath.
	Log(root, filePath string) (string, error)
	// Show returns the full content of one commit identified by rev.
	Show(root, rev string) (string, error)
	// ShowLog returns the commit metadata and message for rev without the diff.
	ShowLog(root, rev string) (string, error)
	// Grep runs a line-numbered grep for pattern and returns the output.
	Grep(root, pattern string) (string, error)
	// Blame returns annotated file content (git blame --date=short).
	Blame(root, filePath string) (string, error)
}

// vcBackends lists every supported backend; the first one whose Root()
// matches wins.  Extend this slice to add new VCS support.
var vcBackends = []vcBackend{gitBackend{}}

// vcFind returns the first backend that recognises dir as part of a repo,
// plus the repo root path.  If dir is empty it falls back to os.Getwd().
func vcFind(dir string) (vcBackend, string) {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	for _, be := range vcBackends {
		if root := be.Root(dir); root != "" {
			return be, root
		}
	}
	return nil, ""
}

// vcDir returns the directory to use as starting point for VCS detection
// given the active buffer.  Prefers the buffer's file directory; falls back
// to the process working directory so that commands work from *scratch* too.
func vcDir(buf *buffer.Buffer) string {
	if f := buf.Filename(); f != "" {
		return filepath.Dir(f)
	}
	dir, _ := os.Getwd()
	return dir
}

// ---------------------------------------------------------------------------
// gitBackend — Git implementation of vcBackend
// ---------------------------------------------------------------------------

type gitBackend struct{}

func (gitBackend) Name() string { return "git" }

func (gitBackend) Root(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (gitBackend) Status(root string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "status").CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Diff(root, filePath string) (string, error) {
	var cmd *exec.Cmd
	if filePath != "" {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--", filePath) //nolint:gosec
	} else {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "diff") //nolint:gosec
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (gitBackend) Log(root, filePath string) (string, error) {
	args := []string{"-C", root, "log", "--oneline", "-50"}
	if filePath != "" {
		args = append(args, "--", filePath)
	}
	out, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Show(root, rev string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "show", rev).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) ShowLog(root, rev string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "show", "--no-patch", "--format=fuller", rev).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Grep(root, pattern string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "grep", "-n", pattern).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Blame(root, filePath string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "blame", "--date=short", "--abbrev=8", filePath).CombinedOutput() //nolint:gosec
	return string(out), err
}

// ---------------------------------------------------------------------------
// Shared VC helpers
// ---------------------------------------------------------------------------

// vcShowOutput opens or reuses a VC output buffer with the given name,
// sets its content to text, applies the given mode, and makes it current.
func (e *Editor) vcShowOutput(name, text, mode string) {
	b := e.FindBuffer(name)
	if b == nil {
		b = buffer.NewWithContent(name, text)
		e.buffers = append(e.buffers, b)
	} else {
		b.SetReadOnly(false)
		b.Delete(0, b.Len())
		b.InsertString(0, text)
	}
	b.SetMode(mode)
	b.SetReadOnly(true)
	b.SetPoint(0)
	e.showBuf(b)
}

// vcQuit switches away from the current VC output buffer to the most recently
// used buffer that isn't a VC output buffer of any kind (using bufferMRU),
// falling back to *scratch*.
func (e *Editor) vcQuit(skipMode string) {
	vcModes := map[string]bool{
		"vc-log": true, "vc-status": true, "vc-grep": true, "diff": true, "vc-commit": true, "vc-annotate": true, "vc-show": true,
	}
	for _, b := range e.bufferMRU {
		if !vcModes[b.Mode()] {
			e.activeWin.SetBuf(b)
			return
		}
	}
	// Fallback: first buffer in e.buffers that isn't the current one.
	cur := e.ActiveBuffer()
	for _, b := range e.buffers {
		if b != cur && !vcModes[b.Mode()] {
			e.activeWin.SetBuf(b)
			return
		}
	}
	e.SwitchToBuffer("*scratch*")
}

// ---------------------------------------------------------------------------
// VC commands
// ---------------------------------------------------------------------------

// cmdVcPrintLog shows the VCS log (C-x v l).
// When the active buffer visits a file, the log is scoped to that file.
func (e *Editor) cmdVcPrintLog() {
	e.clearArg()
	buf := e.ActiveBuffer()
	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-print-log: not in a version control repository")
		return
	}
	text, err := be.Log(root, buf.Filename())
	if err != nil && text == "" {
		text = err.Error()
	}
	e.vcShowOutput("*VC Log*", text, "vc-log")
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// cmdVcDiff shows uncommitted changes for the current file (C-x v =).
func (e *Editor) cmdVcDiff() {
	e.clearArg()
	buf := e.ActiveBuffer()
	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-diff: not in a version control repository")
		return
	}
	text, err := be.Diff(root, buf.Filename())
	if err != nil && text == "" {
		text = err.Error()
	}
	if text == "" {
		e.Message("vc-diff: no uncommitted changes")
		return
	}
	e.vcShowOutput("*vc diff*", text, "diff")
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// cmdVcStatus runs the VCS status command (C-x v s).
func (e *Editor) cmdVcStatus() {
	e.clearArg()
	be, root := vcFind(vcDir(e.ActiveBuffer()))
	if be == nil {
		e.Message("vc-status: not in a version control repository")
		return
	}
	text, err := be.Status(root)
	if err != nil && text == "" {
		text = err.Error()
	}
	e.vcShowOutput("*vc status*", text, "vc-status")
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// cmdVcGrep prompts for a pattern and shows grep results (C-x v g).
func (e *Editor) cmdVcGrep() {
	e.clearArg()
	be, root := vcFind(vcDir(e.ActiveBuffer()))
	if be == nil {
		e.Message("vc-grep: not in a version control repository")
		return
	}
	e.ReadMinibuffer(be.Name()+" grep: ", func(pattern string) {
		if pattern == "" {
			return
		}
		text, err := be.Grep(root, pattern)
		if err != nil && text == "" {
			text = "No matches found."
		}
		if text == "" {
			text = "No matches found."
		}
		e.vcShowOutput("*vc grep*", text, "vc-grep")
		e.vcLogRoots[e.ActiveBuffer()] = root
	})
}

// ---------------------------------------------------------------------------
// VC next-action (C-x v v)
// ---------------------------------------------------------------------------

// cmdVcNextAction is the primary VC command.  It advances the file through
// the version control state machine:
//   - Untracked → git add (stage the file)
//   - Modified but not staged → git add (stage the file)
//   - Staged → open a *vc-commit* buffer for the commit message
//   - Nothing to do → inform the user
func (e *Editor) cmdVcNextAction() {
	e.clearArg()
	buf := e.ActiveBuffer()
	filePath := buf.Filename()

	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-next-action: not in a version control repository")
		return
	}

	// git status --porcelain <file> produces one line:
	//   XY filename
	// where X = index status, Y = working tree status.
	// For untracked files the line is "?? filename".
	var args []string
	if filePath != "" {
		args = []string{"-C", root, "status", "--porcelain", filePath}
	} else {
		args = []string{"-C", root, "status", "--porcelain"}
	}
	out, err := exec.CommandContext(context.Background(), "git", args...).Output() //nolint:gosec
	if err != nil {
		e.Message("vc-next-action: git status failed: %v", err)
		return
	}
	status := strings.TrimSpace(string(out))

	if status == "" {
		e.Message("vc-next-action: nothing to commit for %s", filepath.Base(filePath))
		return
	}

	// Interpret the XY status code.
	xy := ""
	if len(status) >= 2 {
		xy = status[:2]
	}
	x := ""
	if len(xy) >= 1 {
		x = string(xy[0])
	}

	switch {
	case xy == "??":
		// Untracked file: stage it.
		e.vcGitAdd(root, filePath)
	case x == " " || x == "!":
		// Modified in working tree but not staged: stage it.
		e.vcGitAdd(root, filePath)
	default:
		// Something is staged: open commit buffer.
		e.vcOpenCommitBuffer(root, filePath)
	}
}

// vcGitAdd runs git add on filePath and reports the result.
func (e *Editor) vcGitAdd(root, filePath string) {
	var args []string
	if filePath != "" {
		args = []string{"-C", root, "add", filePath}
	} else {
		args = []string{"-C", root, "add", "."}
	}
	if out, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput(); err != nil { //nolint:gosec
		e.Message("git add failed: %s", strings.TrimSpace(string(out)))
		return
	}
	if filePath != "" {
		e.Message("Staged %s", filepath.Base(filePath))
	} else {
		e.Message("Staged all changes")
	}
}

// vcOpenCommitBuffer opens (or reuses) a *vc-commit* buffer pre-populated
// with a comment block listing staged files and their changes.  The user types
// the commit message above the comments, then presses C-c C-c to submit or
// C-c C-k to abort.
func (e *Editor) vcOpenCommitBuffer(root, filePath string) {
	// Stage the file if a specific file was given (ensures it's included).
	if filePath != "" {
		_ = exec.CommandContext(context.Background(), "git", "-C", root, "add", filePath).Run() //nolint:gosec
	}

	// List all staged files via `git diff --cached --name-status`.
	nameOut, _ := exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--cached", "--name-status").Output() //nolint:gosec

	// Also get the summary stat for the overall change count.
	statOut, _ := exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--cached", "--stat").Output() //nolint:gosec

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("# Changes to be committed:\n")
	for _, line := range strings.Split(strings.TrimRight(string(nameOut), "\n"), "\n") {
		if line == "" {
			continue
		}
		// name-status format: "M\tpath/to/file" or "A\tnewfile"
		parts := strings.SplitN(line, "\t", 2)
		statusCode := ""
		path := line
		if len(parts) == 2 {
			switch parts[0] {
			case "M":
				statusCode = "modified:   "
			case "A":
				statusCode = "new file:   "
			case "D":
				statusCode = "deleted:    "
			case "R":
				statusCode = "renamed:    "
			default:
				statusCode = parts[0] + ":         "
			}
			path = parts[1]
		}
		sb.WriteString("#\t" + statusCode + path + "\n")
	}
	// Append the overall stat summary.
	for _, line := range strings.Split(strings.TrimRight(string(statOut), "\n"), "\n") {
		if line != "" && strings.Contains(line, "changed") {
			sb.WriteString("# " + strings.TrimSpace(line) + "\n")
		}
	}
	sb.WriteString("#\n")
	sb.WriteString("# C-c C-c  commit    C-c C-k  abort\n")

	b := e.FindBuffer("*vc-commit*")
	if b == nil {
		b = buffer.NewWithContent("*vc-commit*", sb.String())
		e.buffers = append(e.buffers, b)
	} else {
		b.SetReadOnly(false)
		b.Delete(0, b.Len())
		b.InsertString(0, sb.String())
	}
	b.SetMode("vc-commit")
	b.SetReadOnly(false)
	b.SetPoint(0) // place cursor at the very top so the user types there
	e.vcCommitRoots[b] = root
	e.activeWin.SetBuf(b)
}

// vcCommitDispatch intercepts C-c C-c (submit) and C-c C-k (abort) when the
// active buffer is a *VC Commit* buffer.  It must be checked both when the
// prefix is active (second key of C-c C-x) and when it is not.
func (e *Editor) vcCommitDispatch(ke terminal.KeyEvent) bool {
	if e.prefixKeymap != e.ctrlCKeymap {
		return false
	}
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyCtrlC {
		return false
	}
	if ke.Key == tcell.KeyCtrlC {
		e.vcCommitSubmit()
		e.prefixKeymap = nil
		return true
	}
	if ke.Key == tcell.KeyRune && ke.Rune == 'k' {
		e.vcCommitAbort()
		e.prefixKeymap = nil
		return true
	}
	return false
}

// vcCommitSubmit reads the commit message from the *VC Commit* buffer (lines
// not starting with '#') and runs git commit.
func (e *Editor) vcCommitSubmit() {
	buf := e.ActiveBuffer()
	root := e.vcCommitRoots[buf]
	if root == "" {
		e.Message("vc-commit: no repository root found")
		return
	}
	// Collect non-comment lines as the commit message.
	full := buf.String()
	var msgLines []string
	for _, line := range strings.Split(full, "\n") {
		if !strings.HasPrefix(line, "#") {
			msgLines = append(msgLines, line)
		}
	}
	msg := strings.TrimSpace(strings.Join(msgLines, "\n"))
	if msg == "" {
		e.Message("Aborting commit: empty commit message")
		return
	}
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "commit", "-m", msg).CombinedOutput() //nolint:gosec
	if err != nil {
		e.Message("git commit failed: %s", strings.TrimSpace(string(out)))
		return
	}
	e.Message("Committed: %s", strings.TrimSpace(string(out)))
	e.vcQuit("vc-commit")
}

// vcCommitAbort kills the *VC Commit* buffer and returns to the previous buffer.
func (e *Editor) vcCommitAbort() {
	e.Message("Commit aborted")
	e.vcQuit("vc-commit")
}

// ---------------------------------------------------------------------------
// VC key dispatch functions
// ---------------------------------------------------------------------------

// vcLogDispatch handles keys in a *VC Log* buffer.
// q quits; l shows the commit log message; d and Enter show the full diff.
func (e *Editor) vcLogDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()
	root := e.vcLogRoots[buf]

	switch {
	case ke.Key == tcell.KeyRune && ke.Rune == 'q':
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
			return true
		}
		e.vcQuit("vc-log")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'l':
		// Show commit log message (no diff).
		if root == "" {
			return true
		}
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		eol := buf.EndOfLine(pt)
		line := buf.Substring(bol, eol)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.ShowLog(root, fields[0])
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Log Message*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'd', ke.Key == tcell.KeyEnter:
		// Show full commit diff.
		if root == "" {
			return true
		}
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		eol := buf.EndOfLine(pt)
		line := buf.Substring(bol, eol)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.Show(root, fields[0])
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Show*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true
	}
	return false
}

// vcDiffDispatch handles keys in any "diff" mode buffer (*VC Diff*, *VC Show*).
// q  – close; n/p – next/previous hunk; Enter – goto source line.
func (e *Editor) vcDiffDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	switch {
	case ke.Key == tcell.KeyRune && ke.Rune == 'q':
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
			return true
		}
		e.vcQuit("diff")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'n':
		pt := buf.Point()
		eol := buf.EndOfLine(pt)
		search := eol + 1
		n := buf.Len()
		for search < n {
			bol := search
			eol2 := buf.EndOfLine(bol)
			line := buf.Substring(bol, eol2)
			if strings.HasPrefix(line, "@@") {
				buf.SetPoint(bol)
				return true
			}
			search = eol2 + 1
		}
		e.Message("No next hunk")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'p':
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		search := bol - 1
		for search > 0 {
			bol2 := buf.BeginningOfLine(search)
			eol2 := buf.EndOfLine(bol2)
			line := buf.Substring(bol2, eol2)
			if strings.HasPrefix(line, "@@") {
				buf.SetPoint(bol2)
				return true
			}
			search = bol2 - 1
		}
		e.Message("No previous hunk")
		return true

	case ke.Key == tcell.KeyEnter:
		return e.vcDiffGotoSource(buf)
	}
	return false
}

// vcDiffGotoSource navigates to the source file line corresponding to the
// +/- diff line under point.
func (e *Editor) vcDiffGotoSource(buf *buffer.Buffer) bool {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	curLine := buf.Substring(bol, eol)

	if len(curLine) == 0 {
		return true
	}
	first := curLine[0]
	if first != '+' && first != '-' {
		return true
	}
	if strings.HasPrefix(curLine, "+++") || strings.HasPrefix(curLine, "---") {
		return true
	}

	root := e.vcLogRoots[buf]
	if root == "" {
		return true
	}

	allText := buf.Substring(0, eol)
	lines := strings.Split(allText, "\n")
	curIdx := len(lines) - 1

	filePath := ""
	newFileLineNum := 0

	for i := curIdx - 1; i >= 0; i-- {
		l := lines[i]
		if filePath == "" && strings.HasPrefix(l, "+++ ") {
			rel := strings.TrimPrefix(l[4:], "b/")
			filePath = filepath.Join(root, rel)
		}
		if strings.HasPrefix(l, "@@ ") {
			fields := strings.Fields(l)
			if len(fields) >= 3 {
				newPart := strings.TrimPrefix(fields[2], "+")
				newPart = strings.Split(newPart, ",")[0]
				start, _ := strconv.Atoi(newPart)
				newLine := start
				for j := i + 1; j < curIdx; j++ {
					if !strings.HasPrefix(lines[j], "-") {
						newLine++
					}
				}
				if strings.HasPrefix(curLine, "+") {
					newLine++
				}
				newFileLineNum = newLine
			}
			break
		}
	}

	if filePath == "" || newFileLineNum == 0 {
		e.Message("Cannot determine source location")
		return true
	}

	b, err := e.loadFile(filePath)
	if err != nil {
		e.Message("Cannot open %s: %v", filePath, err)
		return true
	}
	e.activeWin.SetBuf(b)
	pos := b.LineStart(newFileLineNum)
	b.SetPoint(pos)
	return true
}

// vcStatusDispatch handles keys in a *VC Status* buffer.
// q quits; Enter opens the file under point.
func (e *Editor) vcStatusDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	if ke.Key == tcell.KeyRune && ke.Rune == 'q' {
		e.vcQuit("vc-status")
		return true
	}

	if ke.Key != tcell.KeyEnter {
		return false
	}

	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}

	// The file path is the last whitespace-separated token on the line.
	// git status lines: "\tmodified:   path/to/file" or "\tpath/to/file"
	fields := strings.Fields(trimmed)
	filePath := strings.TrimSuffix(fields[len(fields)-1], ":")

	root := e.vcLogRoots[buf]
	if root == "" {
		return true
	}
	abs := filepath.Join(root, filePath)
	if _, err := os.Stat(abs); err != nil {
		e.Message("vc-status: cannot find file: %s", filePath)
		return true
	}
	b, err := e.loadFile(abs)
	if err != nil {
		e.Message("Cannot open %s: %v", abs, err)
		return true
	}
	e.activeWin.SetBuf(b)
	return true
}

// vcGrepDispatch handles keys in a *vc grep* buffer.
// q quits; Enter navigates to file:line.
func (e *Editor) vcGrepDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	if ke.Key == tcell.KeyRune && ke.Rune == 'q' {
		e.vcQuit("vc-grep")
		return true
	}

	if ke.Key != tcell.KeyEnter {
		return false
	}

	// Output format: "filename:linenum:content"
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	if line == "" {
		return true
	}

	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 2 {
		return true
	}
	relPath := parts[0]
	lineNum, err := strconv.Atoi(parts[1])
	if err != nil || lineNum < 1 {
		return true
	}

	root := e.vcLogRoots[buf]
	if root == "" {
		return true
	}
	abs := filepath.Join(root, relPath)
	b, loadErr := e.loadFile(abs)
	if loadErr != nil {
		e.Message("Cannot open %s: %v", abs, loadErr)
		return true
	}
	e.activeWin.SetBuf(b)
	pos := b.LineStart(lineNum)
	b.SetPoint(pos)
	return true
}

// ---------------------------------------------------------------------------
// Messages buffer
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Version
// ---------------------------------------------------------------------------

// cmdGomacsVersion displays the gomacs version, Go runtime version, and uptime
// in the minibuffer (and appends to *messages*).
func (e *Editor) cmdGomacsVersion() {
	v := e.version
	if v == "" {
		v = "dev"
	}
	uptime := time.Since(e.startTime).Round(time.Second)
	e.Message("gomacs %s  Go: %s  Uptime: %v", v, runtime.Version(), uptime)
}

// ---------------------------------------------------------------------------
// VC annotate (git blame)
// ---------------------------------------------------------------------------

// langForExt maps a file extension (including the dot, e.g. ".go") to a
// language mode name understood by syntax.LangToHighlighter.
func langForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".md", ".markdown":
		return "markdown"
	case ".el":
		return "elisp"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".sh", ".bash":
		return "bash"
	case ".json":
		return "json"
	case ".mk":
		return "makefile"
	default:
		return ""
	}
}

// cmdVcAnnotate runs git blame on the current file and displays the output in
// a *vc-annotate* buffer (C-x v g).
func (e *Editor) cmdVcAnnotate() {
	e.clearArg()
	buf := e.ActiveBuffer()
	filePath := buf.Filename()
	if filePath == "" {
		e.Message("vc-annotate: buffer is not visiting a file")
		return
	}
	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-annotate: not in a version control repository")
		return
	}
	text, err := be.Blame(root, filePath)
	if err != nil && text == "" {
		text = err.Error()
	}
	// Encode the source language in the mode name so the highlighter can
	// apply syntax colouring to the source portion of each blame line.
	mode := "vc-annotate"
	if lang := langForExt(filepath.Ext(filePath)); lang != "" {
		mode = "vc-annotate+" + lang
	}
	e.vcShowOutput("*vc-annotate*", text, mode)
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// vcAnnotateHashAtPoint extracts the commit hash from the current line of a
// *vc-annotate* buffer.  Git blame lines start with an optional '^' followed
// by the abbreviated hash.
func (e *Editor) vcAnnotateHashAtPoint(buf *buffer.Buffer) string {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[0], "^")
}

// vcAnnotateDispatch handles key events in a *vc-annotate* buffer.
// l – show commit log/details; d – show commit diff; q – quit.
func (e *Editor) vcAnnotateDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune {
		return false
	}

	buf := e.ActiveBuffer()
	root := e.vcLogRoots[buf]

	switch ke.Rune {
	case 'q':
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
			return true
		}
		e.vcQuit("vc-annotate")
		return true

	case 'l':
		hash := e.vcAnnotateHashAtPoint(buf)
		if hash == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.ShowLog(root, hash)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Log Message*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true

	case 'd':
		hash := e.vcAnnotateHashAtPoint(buf)
		if hash == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.Show(root, hash)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*vc-diff*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true
	}
	return false
}
