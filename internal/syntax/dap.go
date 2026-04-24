package syntax

import (
	"regexp"
	"strings"
	"unicode"
)

// ---- Locals panel -----------------------------------------------------------

// DapLocalsHighlighter highlights the *DAP Locals* panel.
//
// Each line has one of three forms:
//
//	▶ name type = value
//	▼ name type = value
//	  name type = value   (leaf, no expand arrow)
type DapLocalsHighlighter struct{}

var (
	// matches "  ▶ name type = value" or "  ▼ name …"
	reLocalsLine = regexp.MustCompile(`^(\s*)(▶|▼| )\s+(\S+)(?:\s+(\S+))?\s*=\s*(.*)$`)
)

func (DapLocalsHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	pos := 0
	for pos < len(runes) {
		// Find end of line.
		eol := pos
		for eol < len(runes) && runes[eol] != '\n' {
			eol++
		}
		line := string(runes[pos:eol])
		m := reLocalsLine.FindStringSubmatchIndex(line)
		if m != nil && m[0] >= 0 {
			// m indices: [0,1]=whole [2,3]=group1(indent) [4,5]=group2(arrow)
			// [6,7]=group3(name) [8,9]=group4(type,optional) [10,11]=group5(value)
			b2r := func(byteOff int) int {
				return pos + len([]rune(line[:byteOff]))
			}
			arrowStart, arrowEnd := b2r(m[4]), b2r(m[5])
			nameStart, nameEnd := b2r(m[6]), b2r(m[7])

			if runes[arrowStart] == '▶' || runes[arrowStart] == '▼' {
				spans = appendIfOverlap(spans, Span{arrowStart, arrowEnd, FaceFunction}, start, end)
			}
			spans = appendIfOverlap(spans, Span{nameStart, nameEnd, FaceKeyword}, start, end)

			// Type (group 4, optional)
			if m[8] >= 0 {
				typeStart, typeEnd := b2r(m[8]), b2r(m[9])
				spans = appendIfOverlap(spans, Span{typeStart, typeEnd, FaceType}, start, end)
			}

			// Value (group 5)
			if m[10] >= 0 {
				valStart, valEnd := b2r(m[10]), b2r(m[11])
				valStr := string(runes[valStart:valEnd])
				valFace := dapValueFace(valStr)
				spans = appendIfOverlap(spans, Span{valStart, valEnd, valFace}, start, end)
			}
		}
		pos = eol + 1
	}
	return spans
}

// dapValueFace picks a face for a DAP variable value.
func dapValueFace(val string) Face {
	if val == "" {
		return FaceDefault
	}
	// String
	if strings.HasPrefix(val, `"`) || strings.HasPrefix(val, "`") {
		return FaceString
	}
	// Number (decimal or hex)
	if isNumericVal(val) {
		return FaceNumber
	}
	// Nil / bool
	if val == "nil" || val == "true" || val == "false" {
		return FaceKeyword
	}
	return FaceDefault
}

func isNumericVal(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if !unicode.IsDigit(r) && r != '.' && r != 'x' && r != 'X' &&
			(r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// ---- Stack panel ------------------------------------------------------------

// DapStackHighlighter highlights the *DAP Stack* panel.
//
// Each line has the form:
//
//	#N  FuncName (file.go:42)
var reStackLine = regexp.MustCompile(`^(#\d+)\s+(\S+)\s+\(([^:)]+):(\d+)\)`)

type DapStackHighlighter struct{}

func (DapStackHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	var spans []Span
	pos := 0
	for pos < len(runes) {
		eol := pos
		for eol < len(runes) && runes[eol] != '\n' {
			eol++
		}
		line := string(runes[pos:eol])
		m := reStackLine.FindStringSubmatchIndex(line)
		if m != nil && m[0] >= 0 {
			b2r := func(byteOff int) int {
				return pos + len([]rune(line[:byteOff]))
			}
			// Group 1: #N  → FaceNumber
			// Group 2: FuncName → FaceFunction
			// Group 3: file → FaceString
			// Group 4: line → FaceKeyword
			spans = appendIfOverlap(spans, Span{b2r(m[2]), b2r(m[3]), FaceNumber}, start, end)
			spans = appendIfOverlap(spans, Span{b2r(m[4]), b2r(m[5]), FaceFunction}, start, end)
			spans = appendIfOverlap(spans, Span{b2r(m[6]), b2r(m[7]), FaceString}, start, end)
			spans = appendIfOverlap(spans, Span{b2r(m[8]), b2r(m[9]), FaceKeyword}, start, end)
		}
		pos = eol + 1
	}
	return spans
}

// appendIfOverlap appends span s to spans if it overlaps [start, end).
func appendIfOverlap(spans []Span, s Span, start, end int) []Span {
	if s.End > start && s.Start < end {
		spans = append(spans, s)
	}
	return spans
}
