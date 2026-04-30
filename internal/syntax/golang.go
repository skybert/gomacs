package syntax

import (
	"go/scanner"
	"go/token"
	"unicode/utf8"
)

// GoHighlighter highlights Go source code using go/scanner.
type GoHighlighter struct{}

// builtinTypes is the set of predeclared Go types.
var builtinTypes = map[string]bool{
	"bool":       true,
	"byte":       true,
	"complex64":  true,
	"complex128": true,
	"error":      true,
	"float32":    true,
	"float64":    true,
	"int":        true,
	"int8":       true,
	"int16":      true,
	"int32":      true,
	"int64":      true,
	"rune":       true,
	"string":     true,
	"uint":       true,
	"uint8":      true,
	"uint16":     true,
	"uint32":     true,
	"uint64":     true,
	"uintptr":    true,
}

// builtinFuncs is the set of predeclared Go functions.
var builtinFuncs = map[string]bool{
	"append":  true,
	"cap":     true,
	"close":   true,
	"copy":    true,
	"delete":  true,
	"len":     true,
	"make":    true,
	"new":     true,
	"panic":   true,
	"print":   true,
	"println": true,
	"real":    true,
	"imag":    true,
	"recover": true,
}

// Highlight tokenizes text using go/scanner and returns face spans for
// tokens whose rune positions overlap [start, end).
func (g GoHighlighter) Highlight(text string, start, end int) []Span {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(text))

	var s scanner.Scanner
	// Collect errors silently — partial / in-progress source is common.
	s.Init(file, []byte(text), nil /* no error handler */, scanner.ScanComments)

	// curByte and curRune form a monotonically-advancing cursor that converts
	// byte offsets to rune offsets without rescanning from position 0 each time.
	// Since go/scanner emits tokens in strictly increasing byte-offset order
	// this is O(len(text)) overall instead of O(len(text) × num_tokens).
	curByte := 0
	curRune := 0

	advanceTo := func(target int) int {
		for curByte < target {
			_, size := utf8.DecodeRuneInString(text[curByte:])
			curByte += size
			curRune++
		}
		return curRune
	}

	var spans []Span

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		// pos is a token.Pos; convert to a zero-based byte offset.
		byteStart := fset.Position(pos).Offset

		// Determine the literal text length in bytes.
		var tokLen int
		if lit != "" {
			tokLen = len(lit)
		} else {
			tokLen = len(tok.String())
		}
		byteEnd := byteStart + tokLen

		// Guard against scanner returning offsets beyond the source.
		if byteStart > len(text) {
			byteStart = len(text)
		}
		if byteEnd > len(text) {
			byteEnd = len(text)
		}

		// Advance cursor to byteStart, then byteEnd — never backward.
		runeStart := advanceTo(byteStart)
		runeEnd := advanceTo(byteEnd)

		// Only emit spans that overlap [start, end).
		if runeEnd <= start || runeStart >= end {
			continue
		}

		face, ok := faceForToken(tok, lit)
		if !ok {
			continue
		}

		spans = append(spans, Span{Start: runeStart, End: runeEnd, Face: face})
	}

	return spans
}

// faceForToken maps a token type (and optional literal) to a Face.
// Returns (face, true) when the token should be highlighted, (zero, false) otherwise.
func faceForToken(tok token.Token, lit string) (Face, bool) {
	switch {
	case isKeyword(tok):
		return FaceKeyword, true

	case tok == token.STRING || tok == token.CHAR:
		return FaceString, true

	case tok == token.COMMENT:
		return FaceComment, true

	case tok == token.INT || tok == token.FLOAT || tok == token.IMAG:
		return FaceNumber, true

	case tok == token.IDENT:
		name := lit
		if builtinTypes[name] {
			return FaceType, true
		}
		if builtinFuncs[name] {
			return FaceFunction, true
		}
	}

	return Face{}, false
}

// isKeyword reports whether tok is a Go keyword.
func isKeyword(tok token.Token) bool {
	//nolint:exhaustive // external enum; default case handles unknowns
	switch tok {
	case token.BREAK, token.CASE, token.CHAN, token.CONST, token.CONTINUE,
		token.DEFAULT, token.DEFER, token.ELSE, token.FALLTHROUGH, token.FOR,
		token.FUNC, token.GO, token.GOTO, token.IF, token.IMPORT,
		token.INTERFACE, token.MAP, token.PACKAGE, token.RANGE, token.RETURN,
		token.SELECT, token.STRUCT, token.SWITCH, token.TYPE, token.VAR:
		return true
	}
	return false
}
