package syntax

import (
	"regexp"
	"strings"
)

// compErrRe matches lines of the form: file:line: or file:line:col:
// This covers Go compiler, golangci-lint, staticcheck, and similar tools.
var compErrRe = regexp.MustCompile(`^([^:\s][^:]*):(\d+)(?::(\d+))?:`)

// CompilationHighlighter highlights error/warning location coordinates in
// compilation output. Lines matching file:line:col: get the filename colored
// with FaceType (cyan) and the :line:col: portion with FaceNumber (magenta).
type CompilationHighlighter struct{}

func (CompilationHighlighter) Highlight(text string, start, end int) []Span {
	var spans []Span
	pos := 0
	for line := range strings.SplitSeq(text, "\n") {
		m := compErrRe.FindStringSubmatchIndex(line)
		if m != nil {
			// m[2]:m[3] = file, m[4]:m[5] = line number, m[6]:m[7] = col (optional)
			fileStart := pos + m[2]
			fileEnd := pos + m[3]
			coordEnd := pos + m[1] // end of entire match (includes trailing colon)

			spans = append(spans, Span{Start: fileStart, End: fileEnd, Face: FaceType})
			if fileEnd < coordEnd {
				spans = append(spans, Span{Start: fileEnd, End: coordEnd, Face: FaceNumber})
			}
		}
		pos += len([]rune(line)) + 1 // +1 for newline
	}
	return spans
}
