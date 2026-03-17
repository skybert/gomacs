package syntax

// VcLogHighlighter highlights git log --oneline output.
// Commit SHAs at the start of each line are coloured as functions (orange).
type VcLogHighlighter struct{}

var FaceVcLogSHA = Face{Fg: "yellow"}

func (VcLogHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	i := start
	for i < end {
		lineStart := i
		for i < end && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < end {
			i++ // skip '\n'
		}
		if lineStart >= lineEnd {
			continue
		}
		// SHA: leading hex chars at start of line.
		j := lineStart
		for j < lineEnd && isHexRune(runes[j]) {
			j++
		}
		if j > lineStart && j-lineStart >= 7 {
			spans = append(spans, Span{Start: lineStart, End: j, Face: FaceVcLogSHA})
		}
	}
	return spans
}

// VcAnnotateHighlighter highlights git blame output.
// Format per line: <hash> (<author> <date> <lineno>) <content>
// The hash is coloured as a function (orange), the metadata in parens as a comment.
type VcAnnotateHighlighter struct{}

func (VcAnnotateHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	i := start
	for i < end {
		lineStart := i
		for i < end && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < end {
			i++ // skip '\n'
		}
		if lineStart >= lineEnd {
			continue
		}
		// SHA: leading hex chars (possibly prefixed with ^ for initial commit).
		j := lineStart
		if j < lineEnd && runes[j] == '^' {
			j++
		}
		shaStart := j
		for j < lineEnd && isHexRune(runes[j]) {
			j++
		}
		shaEnd := j
		if shaEnd > shaStart && shaEnd-shaStart >= 7 {
			spans = append(spans, Span{Start: lineStart, End: shaEnd, Face: FaceVcLogSHA})
		}
		// Metadata in parentheses: (author date lineno)
		for j < lineEnd && runes[j] == ' ' {
			j++
		}
		if j < lineEnd && runes[j] == '(' {
			parenStart := j
			for j < lineEnd && runes[j] != ')' {
				j++
			}
			if j < lineEnd {
				j++ // include closing ')'
			}
			spans = append(spans, Span{Start: parenStart, End: j, Face: FaceComment})
		}
	}
	return spans
}

// VcCommitHighlighter highlights vc-commit message buffers.
// Lines starting with '#' are coloured as comments.
type VcCommitHighlighter struct{}

func (VcCommitHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	i := start
	for i < end {
		lineStart := i
		for i < end && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < end {
			i++
		}
		if lineStart < lineEnd && runes[lineStart] == '#' {
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceComment})
		}
	}
	return spans
}

// isHexRune returns true if r is a hexadecimal digit.
func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
