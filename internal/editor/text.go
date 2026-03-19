package editor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
)

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

// cmdTransposeWords transposes the words around point (M-t).
func (e *Editor) cmdTransposeWords() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()

	pos := pt
	for pos < length && !isWordRune(buf.RuneAt(pos)) {
		pos++
	}
	w1Start := pos
	for pos < length && isWordRune(buf.RuneAt(pos)) {
		pos++
	}
	w1End := pos
	if w1Start == w1End {
		e.Message("No words to transpose")
		return
	}
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
	onBlank := bol == eol

	if !onBlank {
		pos := eol
		if pos < buf.Len() {
			pos++
		}
		for pos < buf.Len() {
			lineEOL := buf.EndOfLine(pos)
			if pos != lineEOL {
				break
			}
			buf.Delete(pos-1, 1)
		}
		return
	}

	start := bol
	for start > 0 {
		prevBOL := buf.BeginningOfLine(start - 1)
		prevEOL := buf.EndOfLine(prevBOL)
		if prevBOL < prevEOL {
			break
		}
		start = prevBOL
	}
	end := eol
	for end < buf.Len() {
		nextBOL := end + 1
		if nextBOL >= buf.Len() {
			break
		}
		nextEOL := buf.EndOfLine(nextBOL)
		if nextBOL < nextEOL {
			break
		}
		end = nextEOL
	}
	if end > start {
		buf.Delete(start, end-start)
		buf.InsertString(start, "\n")
		buf.SetPoint(start)
	}
}

// deleteTrailingWhitespace removes trailing whitespace from every line in
// [regionStart, regionEnd).  It works backward through the buffer so that
// deletions do not shift the positions of lines yet to be processed.
func (e *Editor) deleteTrailingWhitespace(buf *buffer.Buffer, regionStart, regionEnd int) {
	type span struct{ end, count int }
	var spans []span
	pos := regionStart
	for pos < regionEnd {
		eol := buf.EndOfLine(pos)
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
		pos = eol + 1
	}
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
		nlPos := bol - 1
		leadEnd := bol
		for leadEnd < buf.Len() && (buf.RuneAt(leadEnd) == ' ' || buf.RuneAt(leadEnd) == '\t') {
			leadEnd++
		}
		count := leadEnd - nlPos
		buf.Delete(nlPos, count)
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
		start, end = 0, buf.Len()
	}
	text := buf.Substring(start, end)
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

	paraStart := pt
	for paraStart > 0 {
		if runes[paraStart-1] == '\n' {
			lineStart := buf.BeginningOfLine(paraStart - 1)
			lineEnd := buf.EndOfLine(lineStart)
			if lineStart == lineEnd {
				break
			}
		}
		paraStart--
	}
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
	words := strings.Fields(paraText)
	if len(words) == 0 {
		return
	}

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

	line, _ := buf.LineCol(start)
	endLine, _ := buf.LineCol(end)
	for l := line; l <= endLine; l++ {
		linePos := buf.LineStart(l)
		if linePos >= buf.Len() {
			break
		}
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
			continue
		}
		if n > 0 {
			buf.InsertString(linePos, indent)
		} else {
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
