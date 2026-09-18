package syntax

import "strings"

// GherkinHighlighter highlights Gherkin (.feature) files.
type GherkinHighlighter struct{}

// gherkinKeywordPrefixes are lowercased line-start prefixes for FaceKeyword.
// Longer multi-word prefixes must appear before any shared shorter prefix.
var gherkinKeywordPrefixes = []string{
	"scenario outline:", "scenario template:",
	"feature:", "background:", "scenario:", "examples:", "rule:",
	"given ", "when ", "then ", "and ", "but ", "* ",
}

func (h GherkinHighlighter) Highlight(text string, start, end int) []Span {
	return h.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.
func (h GherkinHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	return h.scan(runes, ScanState{}, start, end, nil)
}

// HighlightResume implements Resumable.  Gherkin's only cross-line state is
// whether the scan sits inside a docstring, which ScanState.Fence carries; a
// line start with that flag is a safe restart point.
func (h GherkinHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	return h.scan(runes, st, st.Pos, end, cp)
}

func (h GherkinHighlighter) scan(runes []rune, st ScanState, start, end int, cp *Checkpoints) []Span {
	n := len(runes)
	var spans []Span

	emit := func(s, e int, face Face) {
		if e > start && s < end {
			spans = append(spans, Span{Start: s, End: e, Face: face})
		}
	}

	// lineLimit bounds where a *new* line may start.  Scanning within a line
	// still uses n so a line beginning just before end is highlighted in full.
	// The docstring state is still tracked from the first line onwards.
	lineLimit := min(n, end)

	inDocstring := st.Fence
	i := st.Pos
	for i < lineLimit {
		cp.mark(ScanState{Pos: i, Fence: inDocstring}, len(spans))
		lineStart := i
		for i < n && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < n {
			i++ // skip '\n'
		}

		// Find trimmed start (skip leading whitespace).
		ts := lineStart
		for ts < lineEnd && (runes[ts] == ' ' || runes[ts] == '\t') {
			ts++
		}
		if ts >= lineEnd {
			continue
		}

		// Docstring delimiter (""" or ```).
		if lineEnd-ts >= 3 &&
			((runes[ts] == '"' && runes[ts+1] == '"' && runes[ts+2] == '"') ||
				(runes[ts] == '`' && runes[ts+1] == '`' && runes[ts+2] == '`')) {
			inDocstring = !inDocstring
			emit(ts, lineEnd, FaceString)
			continue
		}
		if inDocstring {
			emit(ts, lineEnd, FaceString)
			continue
		}

		// Comment.
		if runes[ts] == '#' {
			emit(ts, lineEnd, FaceComment)
			continue
		}

		// Tag line (@tag1 @tag2 …).
		if runes[ts] == '@' {
			j := ts
			for j < lineEnd {
				if runes[j] == '@' {
					k := j + 1
					for k < lineEnd && runes[k] != ' ' && runes[k] != '\t' && runes[k] != '@' {
						k++
					}
					emit(j, k, FaceType)
					j = k
				} else {
					j++
				}
			}
			continue
		}

		// Table row: highlight each | pipe.
		if runes[ts] == '|' {
			for j := ts; j < lineEnd; j++ {
				if runes[j] == '|' {
					emit(j, j+1, FaceFunction)
				}
			}
			continue
		}

		// Keyword matching (case-insensitive).
		lineStr := strings.ToLower(string(runes[ts:lineEnd]))
		for _, kw := range gherkinKeywordPrefixes {
			if strings.HasPrefix(lineStr, kw) {
				kwEnd := ts + len([]rune(kw))
				emit(ts, kwEnd, FaceKeyword)
				gherkinHighlightStepContent(runes, kwEnd, lineEnd, emit)
				break
			}
		}
	}
	return spans
}

// gherkinHighlightStepContent highlights <parameters> (FaceType) and
// "inline strings" (FaceString) within the text portion of a step line.
func gherkinHighlightStepContent(runes []rune, start, end int, emit func(int, int, Face)) {
	i := start
	for i < end {
		switch runes[i] {
		case '<':
			j := i + 1
			for j < end && runes[j] != '>' {
				j++
			}
			if j < end {
				j++ // include '>'
			}
			emit(i, j, FaceType)
			i = j
		case '"':
			j := i + 1
			for j < end && runes[j] != '"' {
				j++
			}
			if j < end {
				j++
			}
			emit(i, j, FaceString)
			i = j
		default:
			i++
		}
	}
}
