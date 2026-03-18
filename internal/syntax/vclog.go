package syntax

import "strings"

// VcGrepHighlighter highlights git grep output.
// Each line has the format "filename:linenum:content".
// The filename is coloured cyan, the line number yellow, and the source
// content is coloured using BashHighlighter (perl-mode equivalent).
type VcGrepHighlighter struct{}

var (
	FaceVcGrepFile = Face{Fg: "cyan"}
	FaceVcGrepLine = Face{Fg: "yellow"}
)

func (VcGrepHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	src := BashHighlighter{}
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
		line := string(runes[lineStart:lineEnd])
		firstColon := strings.Index(line, ":")
		if firstColon <= 0 {
			continue
		}
		// Highlight filename.
		spans = append(spans, Span{Start: lineStart, End: lineStart + firstColon, Face: FaceVcGrepFile})
		// Find second colon for line number.
		rest := line[firstColon+1:]
		secondColon := strings.Index(rest, ":")
		if secondColon < 0 {
			continue
		}
		lineNumStart := lineStart + firstColon + 1
		lineNumEnd := lineNumStart + secondColon
		spans = append(spans, Span{Start: lineNumStart, End: lineNumEnd, Face: FaceVcGrepLine})
		// Highlight source content with BashHighlighter.
		contentStart := lineNumEnd + 1
		if contentStart < lineEnd {
			spans = append(spans, src.Highlight(text, contentStart, lineEnd)...)
		}
	}
	return spans
}

// VcShowHighlighter highlights git show output.
// The commit header (lines before the first "diff --git" line) has its
// metadata labels (commit, Author, Date, etc.) coloured as keywords while
// the SHA on the "commit" line is coloured yellow.  The diff portion is
// delegated to DiffHighlighter.
type VcShowHighlighter struct{}

func (VcShowHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)

	// Find the rune index where the diff portion starts.
	diffStart := end
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
		if strings.HasPrefix(string(runes[lineStart:lineEnd]), "diff --git") {
			diffStart = lineStart
			break
		}
	}

	var spans []Span

	// Highlight commit header lines.
	i = start
	for i < diffStart {
		lineStart := i
		for i < diffStart && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < diffStart {
			i++
		}
		if lineStart >= lineEnd {
			continue
		}
		line := string(runes[lineStart:lineEnd])
		if strings.HasPrefix(line, "commit ") {
			// Highlight "commit" label and the SHA that follows.
			labelEnd := lineStart + len("commit")
			spans = append(spans, Span{Start: lineStart, End: labelEnd, Face: FaceKeyword})
			shaStart := lineStart + len("commit ")
			shaEnd := shaStart
			for shaEnd < lineEnd && isHexRune(runes[shaEnd]) {
				shaEnd++
			}
			if shaEnd > shaStart {
				spans = append(spans, Span{Start: shaStart, End: shaEnd, Face: FaceVcLogSHA})
			}
		} else if colonIdx := strings.Index(line, ":"); colonIdx > 0 {
			// "Author:", "AuthorDate:", "Date:", "Merge:", "Parent:", etc.
			spans = append(spans, Span{Start: lineStart, End: lineStart + colonIdx + 1, Face: FaceKeyword})
		}
	}

	// Delegate diff portion to DiffHighlighter.
	if diffStart < end {
		spans = append(spans, DiffHighlighter{}.Highlight(text, diffStart, end)...)
	}

	return spans
}

// VcLogHighlighter highlights git log --oneline output.
// Commit SHAs at the start of each line are coloured yellow.
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
// The hash is coloured yellow, the metadata in parens as a comment, and the
// source code on the right is highlighted using Source (if non-nil).
type VcAnnotateHighlighter struct {
	// Source is an optional highlighter applied to the source-code portion of
	// each blame line (the text that follows the closing parenthesis).
	Source Highlighter
}

// vcAnnotateSrcPortion records where source code lives in one blame line.
type vcAnnotateSrcPortion struct {
	origStart int // index in the full blame runes where source code begins
	length    int // rune count of the source code (excluding the trailing newline)
}

func (h VcAnnotateHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	var portions []vcAnnotateSrcPortion

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
		metaEnd := j
		if j < lineEnd && runes[j] == '(' {
			parenStart := j
			for j < lineEnd && runes[j] != ')' {
				j++
			}
			if j < lineEnd {
				j++ // include closing ')'
			}
			metaEnd = j
			spans = append(spans, Span{Start: parenStart, End: j, Face: FaceComment})
		}
		// Source code begins after the closing ')' and an optional space.
		if h.Source != nil {
			srcStart := metaEnd
			if srcStart < lineEnd && runes[srcStart] == ' ' {
				srcStart++
			}
			if srcStart < lineEnd {
				portions = append(portions, vcAnnotateSrcPortion{
					origStart: srcStart,
					length:    lineEnd - srcStart,
				})
			}
		}
	}

	// Apply source highlighting to the collected source portions.
	if h.Source != nil && len(portions) > 0 {
		spans = append(spans, h.highlightSource(runes, portions)...)
	}

	return spans
}

// highlightSource builds a virtual source text from the extracted source
// portions, runs the inner Source highlighter on it, then remaps the resulting
// spans back to their original positions in the blame output.  Spans that
// cross virtual-line boundaries (e.g. multi-line strings) are dropped.
func (h VcAnnotateHighlighter) highlightSource(runes []rune, portions []vcAnnotateSrcPortion) []Span {
	type offset struct {
		origStart int
		virtStart int
	}
	offsets := make([]offset, len(portions))
	var virtualRunes []rune
	for idx, p := range portions {
		offsets[idx] = offset{origStart: p.origStart, virtStart: len(virtualRunes)}
		virtualRunes = append(virtualRunes, runes[p.origStart:p.origStart+p.length]...)
		virtualRunes = append(virtualRunes, '\n')
	}

	virtualText := string(virtualRunes)
	srcSpans := h.Source.Highlight(virtualText, 0, len(virtualRunes))

	var out []Span
	for _, ss := range srcSpans {
		// Find which portion contains ss.Start.
		idx := -1
		for i, off := range offsets {
			portionEnd := off.virtStart + portions[i].length
			if ss.Start >= off.virtStart && ss.Start < portionEnd {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue // starts on a virtual '\n'
		}
		portionEnd := offsets[idx].virtStart + portions[idx].length
		if ss.End > portionEnd {
			continue // crosses a line boundary — skip
		}
		origStart := offsets[idx].origStart + (ss.Start - offsets[idx].virtStart)
		origEnd := offsets[idx].origStart + (ss.End - offsets[idx].virtStart)
		out = append(out, Span{Start: origStart, End: origEnd, Face: ss.Face})
	}
	return out
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

// LangToHighlighter returns the Highlighter for a language mode name.
// Returns nil for unknown languages.
func LangToHighlighter(lang string) Highlighter {
	switch strings.ToLower(lang) {
	case "go":
		return GoHighlighter{}
	case "markdown":
		return MarkdownHighlighter{}
	case "elisp":
		return ElispHighlighter{}
	case "python":
		return PythonHighlighter{}
	case "java":
		return JavaHighlighter{}
	case "bash":
		return BashHighlighter{}
	case "json":
		return JSONHighlighter{}
	case "makefile":
		return MakefileHighlighter{}
	default:
		return nil
	}
}
