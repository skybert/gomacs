package syntax

// DiffHighlighter highlights unified diff output.
// Added lines (+) are green, removed lines (-) are red,
// hunk headers (@@ ... @@) are cyan, file headers (--- / +++) are bold.
type DiffHighlighter struct{}

var (
	FaceDiffAdded   = Face{Fg: "green"}
	FaceDiffRemoved = Face{Fg: "red"}
	FaceDiffHunk    = Face{Fg: "cyan", Bold: true}
	FaceDiffFile    = Face{Bold: true}
)

// Highlight colours one diff line at a time.  Lines are independent, so the
// scan stops as soon as a line begins at or past end; the line it is already on
// is still measured to its real end so its span is not truncated.
func (h DiffHighlighter) Highlight(text string, start, end int) []Span {
	return h.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.
func (h DiffHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	return h.scan(runes, start, end, nil)
}

// HighlightResume implements Resumable.  Diff lines are independent, so a line
// start is a safe restart point and ScanState.Pos alone describes where the scan
// is.
func (h DiffHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	return h.scan(runes, st.Pos, end, cp)
}

func (h DiffHighlighter) scan(runes []rune, start, end int, cp *Checkpoints) []Span {
	n := len(runes)
	end = min(end, n)
	var spans []Span
	i := start
	for i < end {
		cp.mark(ScanState{Pos: i}, len(spans))
		// Find the end of the current line.
		lineStart := i
		for i < n && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < n {
			i++ // skip '\n'
		}
		if lineStart >= lineEnd {
			continue
		}
		first := runes[lineStart]
		switch {
		case first == '+' && lineStart+1 < lineEnd && runes[lineStart+1] == '+' && lineStart+2 < lineEnd && runes[lineStart+2] == '+':
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceDiffFile})
		case first == '-' && lineStart+1 < lineEnd && runes[lineStart+1] == '-' && lineStart+2 < lineEnd && runes[lineStart+2] == '-':
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceDiffFile})
		case first == '@':
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceDiffHunk})
		case first == '+':
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceDiffAdded})
		case first == '-':
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceDiffRemoved})
		}
	}
	return spans
}
