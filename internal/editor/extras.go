package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

// vcGitDir returns the git repository root for the file in buf, or "".
// It walks upward from the file's directory looking for a ".git" entry.
func vcGitDir(buf *buffer.Buffer) string {
	filename := buf.Filename()
	if filename == "" {
		return ""
	}
	dir := filepath.Dir(filename)
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
	e.activeWin.SetBuf(b)
}

// cmdVcPrintLog shows the git log for the current buffer's file (C-x v l).
func (e *Editor) cmdVcPrintLog() {
	e.clearArg()
	buf := e.ActiveBuffer()
	root := vcGitDir(buf)
	if root == "" {
		e.Message("vc-print-log: not in a git repository")
		return
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "log", "--oneline", "-50") //nolint:gosec
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && text == "" {
		text = err.Error()
	}
	e.vcShowOutput("*VC Log*", text, "vc-log")
	// Record the repo root so vcLogDispatch can run git show.
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// cmdVcDiff shows uncommitted changes for the current repository (C-x v =).
func (e *Editor) cmdVcDiff() {
	e.clearArg()
	buf := e.ActiveBuffer()
	root := vcGitDir(buf)
	if root == "" {
		e.Message("vc-diff: not in a git repository")
		return
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "diff") //nolint:gosec
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && text == "" {
		text = err.Error()
	}
	if text == "" {
		e.Message("vc-diff: no uncommitted changes")
		return
	}
	e.vcShowOutput("*VC Diff*", text, "diff")
	// Record the repo root so vcDiffDispatch can navigate to source files.
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// vcLogDispatch handles keys in a *VC Log* buffer.
// q quits (switches to a previous buffer); Enter opens the commit under point.
// Returns true if the key was consumed.
func (e *Editor) vcLogDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}
	if ke.Key == tcell.KeyRune && ke.Rune == 'q' {
		// Switch to the most recently used non-vc-log buffer.
		for _, b := range e.buffers {
			if b != e.ActiveBuffer() && b.Mode() != "vc-log" {
				e.activeWin.SetBuf(b)
				return true
			}
		}
		e.SwitchToBuffer("*scratch*")
		return true
	}
	if ke.Key != tcell.KeyEnter {
		return false
	}

	// Enter: extract the abbreviated commit hash (first word on the line)
	// and open a *VC Show* buffer with git show output.
	buf := e.ActiveBuffer()
	root := e.vcLogRoots[buf]
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
	hash := fields[0]

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "show", hash) //nolint:gosec
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && text == "" {
		text = err.Error()
	}
	e.vcShowOutput("*VC Show*", text, "diff")
	// Record the root so vcDiffDispatch can navigate to source lines.
	e.vcLogRoots[e.ActiveBuffer()] = root
	return true
}

// vcDiffDispatch handles keys in any "diff" mode buffer (*VC Diff*, *VC Show*).
// q  – close (switch to a non-diff buffer)
// n  – jump to the next hunk (@@ header)
// p  – jump to the previous hunk (@@ header)
// Enter on a +/- line – open the corresponding source file at that line
func (e *Editor) vcDiffDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	switch {
	case ke.Key == tcell.KeyRune && ke.Rune == 'q':
		for _, b := range e.buffers {
			if b != buf && b.Mode() != "diff" {
				e.activeWin.SetBuf(b)
				return true
			}
		}
		e.SwitchToBuffer("*scratch*")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'n':
		// Jump to the next @@ hunk header.
		pt := buf.Point()
		eol := buf.EndOfLine(pt)
		// Start search after the current line.
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
		// Jump to the previous @@ hunk header.
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		// Start search before the current line.
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
// +/- diff line under point.  It parses the unified diff context to find
// the file path and new-file line number, then opens the file there.
func (e *Editor) vcDiffGotoSource(buf *buffer.Buffer) bool {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	curLine := buf.Substring(bol, eol)

	// Only act on actual diff lines (+/-), not file/hunk headers.
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

	// Collect all lines up to and including the current one.
	allText := buf.Substring(0, eol)
	lines := strings.Split(allText, "\n")
	curIdx := len(lines) - 1

	// Scan backward to find the nearest +++ b/<path> and @@ header.
	filePath := ""
	newFileLineNum := 0

	for i := curIdx - 1; i >= 0; i-- {
		l := lines[i]
		if filePath == "" && strings.HasPrefix(l, "+++ ") {
			// Strip leading "b/" if present (standard git diff format).
			rel := strings.TrimPrefix(l[4:], "b/")
			filePath = filepath.Join(root, rel)
		}
		if strings.HasPrefix(l, "@@ ") {
			// Parse +newStart from "@@ -A,B +C,D @@ ..."
			fields := strings.Fields(l)
			if len(fields) >= 3 {
				newPart := strings.TrimPrefix(fields[2], "+")
				newPart = strings.Split(newPart, ",")[0]
				start, _ := strconv.Atoi(newPart)
				// Count lines between this @@ and curIdx to find the new-file line.
				newLine := start
				for j := i + 1; j < curIdx; j++ {
					if !strings.HasPrefix(lines[j], "-") {
						newLine++
					}
				}
				// For + lines the cursor is on that new-file line;
				// for - lines navigate to the surrounding context (same newLine).
				if strings.HasPrefix(curLine, "+") {
					newLine++ // the + line itself advances the new-file counter
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

	// Open the file and jump to the computed line.
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
