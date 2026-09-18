package syntax

import "unicode"

// MarkdownHighlighter highlights Markdown text using a line-by-line state machine.
type MarkdownHighlighter struct{}

// Highlight returns face spans for text, only emitting spans that overlap [start, end).
// start and end are rune offsets into text.
//
// Scanning always begins at offset 0 so that the fenced-code-block state is
// tracked correctly, but it stops as soon as a line begins at or after end —
// everything past that point would only be discarded.
func (m MarkdownHighlighter) Highlight(text string, start, end int) []Span {
	return m.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.
func (m MarkdownHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	return m.scan(runes, ScanState{}, start, end, nil)
}

// HighlightResume implements Resumable.  Markdown's only cross-line state is
// whether the scan sits inside a fenced code block, which ScanState.Fence
// carries; a line start with that flag is a safe restart point.
func (m MarkdownHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	return m.scan(runes, st, st.Pos, end, cp)
}

func (m MarkdownHighlighter) scan(runes []rune, st ScanState, start, end int, cp *Checkpoints) []Span {
	var spans []Span

	n := len(runes)
	runeOffset := st.Pos // rune offset of the current line's first rune
	inFence := st.Fence  // true while inside a fenced code block
	for runeOffset < n {
		lineStart := runeOffset
		if lineStart >= end {
			break
		}
		cp.mark(ScanState{Pos: lineStart, Fence: inFence}, len(spans))

		lineEnd := lineStart
		for lineEnd < n && runes[lineEnd] != '\n' {
			lineEnd++
		}
		if lineEnd < n {
			lineEnd++ // the line includes its terminating newline
		}
		line := runes[lineStart:lineEnd]
		runeOffset = lineEnd

		// --- Fenced code block detection (``` lines) ---
		if hasFencePrefix(line) {
			inFence = !inFence
			// Highlight the fence line itself as code.
			addSpan(&spans, lineStart, lineEnd, FaceCode, start, end)
			continue
		}

		if inFence {
			addSpan(&spans, lineStart, lineEnd, FaceCode, start, end)
			continue
		}

		// --- Block-level patterns (whole-line) ---

		// Blockquote: lines starting with >
		if hasPrefix(line, ">") {
			addSpan(&spans, lineStart, lineEnd, FaceBlockquote, start, end)
			continue
		}

		// Headers: ###, ##, #
		if hasPrefix(line, "### ") || equalString(line, "###") {
			addSpan(&spans, lineStart, lineEnd, FaceHeader3, start, end)
			continue
		}
		if hasPrefix(line, "## ") || equalString(line, "##") {
			addSpan(&spans, lineStart, lineEnd, FaceHeader2, start, end)
			continue
		}
		if hasPrefix(line, "# ") || equalString(line, "#") {
			addSpan(&spans, lineStart, lineEnd, FaceHeader1, start, end)
			continue
		}

		// --- Inline patterns (within the line) ---
		spans = append(spans, inlineSpans(line, lineStart, start, end)...)
	}

	return spans
}

// hasPrefix reports whether the runes in line begin with s.
func hasPrefix(line []rune, s string) bool {
	for _, r := range s {
		if len(line) == 0 || line[0] != r {
			return false
		}
		line = line[1:]
	}
	return true
}

// equalString reports whether line is exactly s.  The line slice keeps its
// terminating newline, so this only matches an unterminated final line — which
// is the behaviour the string-based comparisons it replaced had.
func equalString(line []rune, s string) bool {
	if len(line) != len(s) {
		return false
	}
	return hasPrefix(line, s)
}

// hasFencePrefix reports whether line opens or closes a fenced code block, i.e.
// whether its first non-whitespace runes are three backticks.
func hasFencePrefix(line []rune) bool {
	i := 0
	for i < len(line) && unicode.IsSpace(line[i]) {
		i++
	}
	return len(line)-i >= 3 && line[i] == '`' && line[i+1] == '`' && line[i+2] == '`'
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
func inlineSpans(runes []rune, lineRuneBase, winStart, winEnd int) []Span {
	var spans []Span
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
