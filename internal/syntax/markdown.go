package syntax

import (
	"strings"
	"unicode/utf8"
)

// MarkdownHighlighter highlights Markdown text using a line-by-line state machine.
type MarkdownHighlighter struct{}

// Highlight returns face spans for text, only emitting spans that overlap [start, end).
// start and end are rune offsets into text.
func (m MarkdownHighlighter) Highlight(text string, start, end int) []Span {
	var spans []Span

	runeOffset := 0  // rune offset of the current line's first rune
	inFence := false // true while inside a fenced code block

	lines := splitLines(text)

	for _, line := range lines {
		lineRuneLen := utf8.RuneCountInString(line)
		lineStart := runeOffset
		lineEnd := runeOffset + lineRuneLen

		// --- Fenced code block detection (``` lines) ---
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			// Highlight the fence line itself as code.
			addSpan(&spans, lineStart, lineEnd, FaceCode, start, end)
			runeOffset = lineEnd
			continue
		}

		if inFence {
			addSpan(&spans, lineStart, lineEnd, FaceCode, start, end)
			runeOffset = lineEnd
			continue
		}

		// --- Block-level patterns (whole-line) ---

		// Blockquote: lines starting with >
		if strings.HasPrefix(line, ">") {
			addSpan(&spans, lineStart, lineEnd, FaceBlockquote, start, end)
			runeOffset = lineEnd
			continue
		}

		// Headers: ###, ##, #
		if strings.HasPrefix(line, "### ") || line == "###" {
			addSpan(&spans, lineStart, lineEnd, FaceHeader3, start, end)
			runeOffset = lineEnd
			continue
		}
		if strings.HasPrefix(line, "## ") || line == "##" {
			addSpan(&spans, lineStart, lineEnd, FaceHeader2, start, end)
			runeOffset = lineEnd
			continue
		}
		if strings.HasPrefix(line, "# ") || line == "#" {
			addSpan(&spans, lineStart, lineEnd, FaceHeader1, start, end)
			runeOffset = lineEnd
			continue
		}

		// --- Inline patterns (within the line) ---
		spans = append(spans, inlineSpans(line, lineStart, start, end)...)

		runeOffset = lineEnd
	}

	return spans
}

// splitLines splits text into lines, each including its trailing newline if present.
// This preserves rune offsets: concatenating the slices reconstructs text exactly.
func splitLines(text string) []string {
	var lines []string
	for len(text) > 0 {
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:idx+1])
		text = text[idx+1:]
	}
	return lines
}

// addSpan appends a span if it overlaps the window [winStart, winEnd).
func addSpan(spans *[]Span, spanStart, spanEnd int, face Face, winStart, winEnd int) {
	if spanEnd <= winStart || spanStart >= winEnd {
		return
	}
	*spans = append(*spans, Span{Start: spanStart, End: spanEnd, Face: face})
}

// inlineSpans scans a single line for inline Markdown patterns and returns spans.
// lineRuneBase is the rune offset of line[0] within the full text.
// winStart/winEnd are the overall highlight window (rune offsets in full text).
func inlineSpans(line string, lineRuneBase, winStart, winEnd int) []Span {
	var spans []Span
	runes := []rune(line)
	n := len(runes)
	i := 0

	for i < n {
		// ---- Links: [text](url) ----
		if runes[i] == '[' {
			if end, ok := matchLink(runes, i); ok {
				abs := lineRuneBase + i
				absEnd := lineRuneBase + end
				addSpan(&spans, abs, absEnd, FaceLink, winStart, winEnd)
				i = end
				continue
			}
		}

		// ---- Inline code: `code` ----
		if runes[i] == '`' {
			if end, ok := matchDelimited(runes, i, '`', '`'); ok {
				abs := lineRuneBase + i
				absEnd := lineRuneBase + end
				addSpan(&spans, abs, absEnd, FaceCode, winStart, winEnd)
				i = end
				continue
			}
		}

		// ---- Bold: **text** or __text__ ----
		if i+1 < n && ((runes[i] == '*' && runes[i+1] == '*') || (runes[i] == '_' && runes[i+1] == '_')) {
			marker := runes[i]
			if end, ok := matchDoubleDelimited(runes, i, marker); ok {
				abs := lineRuneBase + i
				absEnd := lineRuneBase + end
				addSpan(&spans, abs, absEnd, FaceBold, winStart, winEnd)
				i = end
				continue
			}
		}

		// ---- Italic: *text* or _text_ ----
		if runes[i] == '*' || runes[i] == '_' {
			marker := runes[i]
			if end, ok := matchSingleDelimited(runes, i, marker); ok {
				abs := lineRuneBase + i
				absEnd := lineRuneBase + end
				addSpan(&spans, abs, absEnd, FaceItalic, winStart, winEnd)
				i = end
				continue
			}
		}

		i++
	}

	return spans
}

// matchLink tries to match [text](url) starting at runes[i].
// Returns (end index exclusive, true) on success.
func matchLink(runes []rune, i int) (int, bool) {
	n := len(runes)
	if runes[i] != '[' {
		return 0, false
	}
	j := i + 1
	for j < n && runes[j] != ']' {
		j++
	}
	if j >= n || runes[j] != ']' {
		return 0, false
	}
	j++ // consume ']'
	if j >= n || runes[j] != '(' {
		return 0, false
	}
	j++ // consume '('
	for j < n && runes[j] != ')' {
		j++
	}
	if j >= n || runes[j] != ')' {
		return 0, false
	}
	return j + 1, true
}

// matchDelimited matches opener...closer around content starting at runes[i].
// opener and closer are single runes (e.g., backtick).
func matchDelimited(runes []rune, i int, opener, closer rune) (int, bool) {
	n := len(runes)
	if runes[i] != opener {
		return 0, false
	}
	j := i + 1
	for j < n && runes[j] != closer {
		j++
	}
	if j >= n || runes[j] != closer {
		return 0, false
	}
	if j == i+1 {
		// Empty delimited span — skip.
		return 0, false
	}
	return j + 1, true
}

// matchDoubleDelimited matches **text** or __text__ starting at runes[i].
func matchDoubleDelimited(runes []rune, i int, marker rune) (int, bool) {
	n := len(runes)
	if i+1 >= n || runes[i] != marker || runes[i+1] != marker {
		return 0, false
	}
	j := i + 2
	for j+1 < n {
		if runes[j] == marker && runes[j+1] == marker {
			return j + 2, true
		}
		j++
	}
	return 0, false
}

// matchSingleDelimited matches *text* or _text_ starting at runes[i],
// but only when not followed immediately by the same marker (which would be bold).
func matchSingleDelimited(runes []rune, i int, marker rune) (int, bool) {
	n := len(runes)
	if runes[i] != marker {
		return 0, false
	}
	// Avoid matching the first char of a double marker as italic.
	if i+1 < n && runes[i+1] == marker {
		return 0, false
	}
	j := i + 1
	for j < n && runes[j] != marker {
		j++
	}
	if j >= n || runes[j] != marker {
		return 0, false
	}
	if j == i+1 {
		return 0, false
	}
	return j + 1, true
}
