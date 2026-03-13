package editor

import (
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

// indentElispLine re-indents the line that contains buf.Point() according to
// elispIndentLevel.  Point is left at the first non-whitespace character (or
// at the indentation column if the line is blank).
func indentElispLine(buf *buffer.Buffer) {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	text := buf.String()
	runes := []rune(text)
	n := len(runes)

	// Count existing leading spaces/tabs on this line.
	leadEnd := bol
	for leadEnd < n && (runes[leadEnd] == ' ' || runes[leadEnd] == '\t') {
		leadEnd++
	}
	existingWS := leadEnd - bol

	// Calculate desired indentation.
	desired := elispIndentLevel(text, bol)

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
