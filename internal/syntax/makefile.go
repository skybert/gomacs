package syntax

import "strings"

// MakefileHighlighter highlights GNU Makefile syntax.
//
// Highlighted elements:
//   - Target names (word before ':' on a non-indented line)
//   - Recipe lines (lines starting with a tab)
//   - Variable definitions (VAR = / VAR := / VAR ?= / VAR +=)
//   - Variable references $(VAR) and ${VAR}
//   - Automatic variables ($@, $<, $^, $*, $?)
//   - Comments (#...)
//   - Directives: ifeq, ifneq, ifdef, ifndef, else, endif, include, define, endef
type MakefileHighlighter struct{}

var (
	FaceMakefileTarget    = Face{Fg: "cyan", Bold: true}
	FaceMakefileRecipe    = Face{Fg: "green"}
	FaceMakefileVariable  = Face{Fg: "goldenrod"}
	FaceMakefileVarRef    = Face{Fg: "orange"}
	FaceMakefileComment   = Face{Fg: "slategray", Italic: true}
	FaceMakefileDirective = Face{Fg: "violet", Bold: true}
	FaceMakefileAutoVar   = Face{Fg: "red"}
)

var makefileDirectives = map[string]bool{
	"ifeq": true, "ifneq": true, "ifdef": true, "ifndef": true,
	"else": true, "endif": true, "include": true, "-include": true,
	"sinclude": true, "define": true, "endef": true, "export": true,
	"unexport": true, "override": true, "private": true, "vpath": true,
}

func (MakefileHighlighter) Highlight(text string, start, end int) []Span {
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

		// Recipe line: starts with a tab.
		if runes[lineStart] == '\t' {
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceMakefileRecipe})
			// Also highlight variable references within recipes.
			spans = append(spans, makeVarRefs(runes, lineStart, lineEnd)...)
			continue
		}

		// Comment line.
		if runes[lineStart] == '#' {
			spans = append(spans, Span{Start: lineStart, End: lineEnd, Face: FaceMakefileComment})
			continue
		}

		// Directive line: starts with a known directive keyword.
		firstWord, firstWordEnd := makeFirstWord(runes, lineStart, lineEnd)
		if makefileDirectives[firstWord] {
			spans = append(spans, Span{Start: lineStart, End: firstWordEnd, Face: FaceMakefileDirective})
			// Highlight any variable refs in the rest.
			spans = append(spans, makeVarRefs(runes, firstWordEnd, lineEnd)...)
			continue
		}

		// Look for a target: "target: ..." or "target := ..." (variable def).
		// Find the first '=' or ':' that isn't part of '::'.
		colonPos := -1
		eqPos := -1
		for j := lineStart; j < lineEnd; j++ {
			ch := runes[j]
			if ch == '#' {
				break
			}
			if ch == '=' && eqPos < 0 {
				eqPos = j
				break
			}
			if ch == ':' && colonPos < 0 {
				// Check for ':=' or '::=' (immediate assignment) — treat as var def.
				if j+1 < lineEnd && runes[j+1] == '=' {
					eqPos = j
					break
				}
				colonPos = j
				break
			}
		}

		switch {
		case eqPos > lineStart:
			// Variable definition: highlight the LHS name.
			nameEnd := eqPos
			// Strip trailing space and modifier chars (?+:).
			for nameEnd > lineStart && (runes[nameEnd-1] == ' ' || runes[nameEnd-1] == '\t' ||
				runes[nameEnd-1] == '?' || runes[nameEnd-1] == '+' || runes[nameEnd-1] == ':') {
				nameEnd--
			}
			if nameEnd > lineStart {
				spans = append(spans, Span{Start: lineStart, End: nameEnd, Face: FaceMakefileVariable})
			}
			// Highlight variable refs in the value part.
			spans = append(spans, makeVarRefs(runes, eqPos, lineEnd)...)

		case colonPos > lineStart:
			// Target definition: highlight the target name.
			nameEnd := colonPos
			// Trim trailing spaces before ':'.
			for nameEnd > lineStart && (runes[nameEnd-1] == ' ' || runes[nameEnd-1] == '\t') {
				nameEnd--
			}
			if nameEnd > lineStart {
				spans = append(spans, Span{Start: lineStart, End: nameEnd, Face: FaceMakefileTarget})
			}
			// Highlight any variable refs in prerequisites.
			spans = append(spans, makeVarRefs(runes, colonPos+1, lineEnd)...)

		default:
			// No '=' or ':' — highlight any variable refs.
			spans = append(spans, makeVarRefs(runes, lineStart, lineEnd)...)
		}
	}
	return spans
}

// makeVarRefs finds $(…) / ${…} and $X auto-var references within [start, end).
func makeVarRefs(runes []rune, start, end int) []Span {
	var spans []Span
	for i := start; i < end; i++ {
		if runes[i] != '$' {
			continue
		}
		if i+1 >= end {
			break
		}
		next := runes[i+1]
		switch next {
		case '(', '{':
			// Find matching close.
			close := ')'
			if next == '{' {
				close = '}'
			}
			depth := 1
			j := i + 2
			for j < end && depth > 0 {
				if runes[j] == rune(next) {
					depth++
				} else if runes[j] == close {
					depth--
				}
				j++
			}
			spans = append(spans, Span{Start: i, End: j, Face: FaceMakefileVarRef})
			i = j - 1
		case '@', '<', '^', '*', '?', '%', '+', '/':
			// Automatic variable: $X
			spans = append(spans, Span{Start: i, End: i + 2, Face: FaceMakefileAutoVar})
			i++
		}
	}
	return spans
}

// makeFirstWord returns the first whitespace-delimited word in the line and
// the position just after it.
func makeFirstWord(runes []rune, start, end int) (string, int) {
	i := start
	for i < end && (runes[i] == ' ' || runes[i] == '\t') {
		i++
	}
	j := i
	for j < end && runes[j] != ' ' && runes[j] != '\t' && runes[j] != '\n' {
		j++
	}
	return strings.ToLower(string(runes[i:j])), j
}
