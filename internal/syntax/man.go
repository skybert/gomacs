package syntax

import (
	"sort"
	"strings"
	"unicode"
)

// ManParse strips man-page overstrike sequences from raw, returning the plain
// text and syntax spans derived from overstrike formatting (bold/underline)
// and structural patterns (section headers, option flags).
//
// Man pages use two overstrike conventions:
//   - Bold:      c\bc  — the same character repeated with a backspace before it
//   - Underline: _\bc  — underscore + backspace + the visible character
func ManParse(raw string) (string, []Span) {
	runes := []rune(raw)
	n := len(runes)
	plain := make([]rune, 0, n)
	var spans []Span

	isBold := false
	isUnder := false
	spanStart := 0

	flushSpan := func(pos int) {
		if (isBold || isUnder) && pos > spanStart {
			face := FaceKeyword // bold
			if isUnder {
				face = FaceType // underline
			}
			spans = append(spans, Span{Start: spanStart, End: pos, Face: face})
		}
	}

	setState := func(bold, under bool, pos int) {
		if bold == isBold && under == isUnder {
			return
		}
		flushSpan(pos)
		spanStart = pos
		isBold = bold
		isUnder = under
	}

	i := 0
	for i < n {
		if i+2 < n && runes[i+1] == '\b' {
			ch := runes[i+2]
			if runes[i] == ch {
				// Bold: c\bc
				setState(true, false, len(plain))
				plain = append(plain, ch)
				i += 3
				continue
			} else if runes[i] == '_' {
				// Underline: _\bc
				setState(false, true, len(plain))
				plain = append(plain, ch)
				i += 3
				continue
			}
		}
		setState(false, false, len(plain))
		plain = append(plain, runes[i])
		i++
	}
	flushSpan(len(plain))

	plainStr := string(plain)

	// Phase 2: structural highlighting on the plain text.
	structural := manStructuralSpans(plainStr)
	spans = append(spans, structural...)

	// faceAtPos requires spans sorted by Start.
	sort.Slice(spans, func(a, b int) bool { return spans[a].Start < spans[b].Start })

	return plainStr, spans
}

// manStructuralSpans scans plain man text for section headers and option flags.
func manStructuralSpans(text string) []Span {
	var spans []Span
	pos := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		content := strings.TrimSuffix(line, "\n")
		lineRuneLen := len([]rune(content))
		if isSectionHeader(content) {
			spans = append(spans, Span{Start: pos, End: pos + lineRuneLen, Face: FaceHeader1})
		} else {
			spans = append(spans, manFlagSpans(content, pos)...)
		}
		pos += len([]rune(line))
	}
	return spans
}

// isSectionHeader returns true for all-uppercase lines at column 0
// (e.g. "NAME", "SYNOPSIS", "DESCRIPTION").
func isSectionHeader(line string) bool {
	if len(line) == 0 || line[0] == ' ' || line[0] == '\t' {
		return false
	}
	hasUpper := false
	for _, r := range line {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	return hasUpper
}

// manFlagSpans returns spans for option flags (-x, --long-opt) within line.
// lineOffset is the rune offset of the start of the line in the full text.
func manFlagSpans(line string, lineOffset int) []Span {
	var spans []Span
	runes := []rune(line)
	n := len(runes)
	for i := 0; i < n; i++ {
		if runes[i] != '-' {
			continue
		}
		// Must be preceded by whitespace, start of line, or open bracket.
		if i > 0 && !unicode.IsSpace(runes[i-1]) && runes[i-1] != '[' && runes[i-1] != ',' {
			continue
		}
		start := i
		i++ // skip first '-'
		if i < n && runes[i] == '-' {
			i++ // skip second '-' for --long
		}
		// Read option name: letters, digits, hyphens, underscores.
		for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '-' || runes[i] == '_') {
			i++
		}
		if i > start+1 { // at least one char after the leading '-'
			spans = append(spans, Span{Start: lineOffset + start, End: lineOffset + i, Face: FaceFunction})
		}
		i-- // outer loop will increment
	}
	return spans
}
