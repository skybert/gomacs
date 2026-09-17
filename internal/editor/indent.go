package editor

import (
	"sort"
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
)

// elispIndentLevel returns the number of spaces that the line starting at
// lineStart should be indented to, given the full buffer text.
//
// The algorithm finds the innermost unclosed parenthesis before lineStart and
// indents by 2 relative to that paren's column.  Top-level forms are at
// column 0.  String and comment contents are skipped so stray parens inside
// them do not disturb the count.
//
// This is the string entry point; the editor itself goes through
// elispIndentLevelAt, which reads the buffer directly.
func elispIndentLevel(text string, lineStart int) int {
	runes := []rune(text)
	if lineStart > len(runes) {
		lineStart = len(runes)
	}

	// Stack of column positions for unclosed '('.
	var stack []int
	inString := false

	for i := 0; i < lineStart; i++ {
		r := runes[i]

		if inString {
			if r == '\\' && i+1 < lineStart {
				i++ // skip escaped character
			} else if r == '"' {
				inString = false
			}
			continue
		}

		switch r {
		case '"':
			inString = true
		case ';':
			// Line comment: skip to end of line.
			for i+1 < lineStart && runes[i+1] != '\n' {
				i++
			}
		case '(':
			stack = append(stack, columnOf(runes, i))
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if len(stack) == 0 {
		return 0 // top-level form
	}
	return stack[len(stack)-1] + 2
}

// columnOf returns the 0-based column of position pos within its line.
func columnOf(runes []rune, pos int) int {
	col := 0
	for pos > 0 && runes[pos-1] != '\n' {
		pos--
		col++
	}
	return col
}

// elispIndentLevelAt is the buffer-backed elispIndentLevel: identical results,
// but it neither copies the buffer nor rescans it from position 0.  The paren
// stack is taken from the nearest cached checkpoint at or before lineStart (see
// indentCache) and the scan continues from there, extending the checkpoint list
// as it goes.
func elispIndentLevelAt(buf *buffer.Buffer, lineStart int) int {
	lineStart = min(lineStart, buf.Len())
	c := indentCacheFor(buf)

	// Nearest checkpoint at or before lineStart; index 0 (position 0) always
	// qualifies.
	i := sort.SearchInts(c.elispPos, lineStart+1) - 1
	extend := i == len(c.elispPos)-1

	// Stack of column positions for unclosed '(', seeded from the checkpoint.
	stack := append(c.stack[:0], c.elispStack[i]...)
	inString := c.elispInStr[i]
	pos := c.elispPos[i]
	col := 0   // column of the rune at pos
	lines := 0 // lines consumed since the checkpoint

	for pos < lineStart {
		r := buf.RuneAt(pos)
		width := 1 // runes consumed by this step

		switch {
		case inString:
			switch {
			case r == '\\' && pos+1 < lineStart:
				width = 2 // the escaped rune is literal
			case r == '"':
				inString = false
			}
		case r == '"':
			inString = true
		case r == ';':
			// Line comment: skip to end of line.
			for pos+width < lineStart && buf.RuneAt(pos+width) != '\n' {
				width++
			}
		case r == '(':
			stack = append(stack, col)
		case r == ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}

		// Advance, keeping col in step: only a newline resets it, and the one
		// rune a step can skip over is the one escaped inside a string.
		newline := r == '\n' || (width == 2 && buf.RuneAt(pos+1) == '\n')
		pos += width
		if !newline {
			col += width
			continue
		}
		col = 0
		lines++
		if extend && lines%indentCheckpointStride == 0 && pos <= lineStart {
			c.elispPos = append(c.elispPos, pos)
			c.elispStack = append(c.elispStack, append([]int(nil), stack...))
			c.elispInStr = append(c.elispInStr, inString)
		}
	}
	c.stack = stack // keep the grown backing array for the next call

	if len(stack) == 0 {
		return 0 // top-level form
	}
	return stack[len(stack)-1] + 2
}

// indentElispLine re-indents the line that contains buf.Point() according to
// elispIndentLevelAt.  Point is left at the first non-whitespace character (or
// at the indentation column if the line is blank).
func indentElispLine(buf *buffer.Buffer) {
	bol := buf.BeginningOfLine(buf.Point())
	n := buf.Len()

	// Count existing leading spaces/tabs on this line.
	leadEnd := bol
	for leadEnd < n {
		if r := buf.RuneAt(leadEnd); r != ' ' && r != '\t' {
			break
		}
		leadEnd++
	}
	existingWS := leadEnd - bol

	// Calculate desired indentation.
	desired := elispIndentLevelAt(buf, bol)

	// Replace leading whitespace only when it differs.
	if existingWS != desired {
		buf.Delete(bol, existingWS)
		if desired > 0 {
			buf.InsertString(bol, strings.Repeat(" ", desired))
		}
	}

	// Place point at the first non-whitespace character (or end of indent).
	buf.SetPoint(min(bol+desired, buf.Len()))
}
