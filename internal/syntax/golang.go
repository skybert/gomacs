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
	spans, _ := g.scan(text, 0, start, end, nil)
	return spans
}

// HighlightRunes implements RuneHighlighter.
func (g GoHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	spans, _ := g.scan(string(runes), 0, start, end, nil)
	return spans
}

// goScanSlack is how far past the requested end the source handed to go/scanner
// reaches.  Anything but a multi-line token (a raw string, a /* … */ comment)
// finishes well inside it, so the rescan below is rare.
const goScanSlack = 256

// HighlightResume implements Resumable.
//
// go/scanner keeps no state that a token boundary does not settle: a raw string
// or a /* … */ comment is a single token, so restarting the scanner on the source
// that begins at a token boundary yields exactly the tokens the full scan would
// have produced from there.  ScanState.Pos therefore carries the whole state.
//
// Only the source from Pos onwards has to be encoded to bytes for go/scanner,
// and only as far as the scan will read — which is what keeps a keystroke at the
// top of a large file from paying to encode the whole file.  Cutting the source
// short would truncate a token that straddles the cut and so mis-colour it, so a
// scan whose last token reaches the cut is redone against everything that is
// left.  The rescan produces the same tokens up to that point, so the
// checkpoints already reported stay valid and are not reported again.
func (g GoHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	from := st.Pos
	cut := min(max(end+goScanSlack, from), len(runes))
	spans, reach := g.scan(string(runes[from:cut]), from, from, end, cp)
	if cut < len(runes) && reach >= cut {
		spans, _ = g.scan(string(runes[from:]), from, from, end, cp)
	}
	return spans
}

// scan tokenizes src, the buffer text starting at rune offset base, and returns
// spans in whole-buffer rune coordinates that overlap [start, end).  The second
// result is the offset just past the last token that began before end: when it
// reaches the end of src, a token was cut short and the caller must rescan with
// more source.
func (g GoHighlighter) scan(src string, base, start, end int, cp *Checkpoints) ([]Span, int) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var s scanner.Scanner
	// Collect errors silently — partial / in-progress source is common.
	s.Init(file, []byte(src), nil /* no error handler */, scanner.ScanComments)

	// curByte and curRune form a monotonically-advancing cursor that converts
	// byte offsets to rune offsets without rescanning from position 0 each time.
	// Since go/scanner emits tokens in strictly increasing byte-offset order
	// this is O(len(src)) overall instead of O(len(src) × num_tokens).
	curByte := 0
	curRune := base

	advanceTo := func(target int) int {
		for curByte < target {
			_, size := utf8.DecodeRuneInString(src[curByte:])
			curByte += size
			curRune++
		}
		return curRune
	}

	var spans []Span
	reach := base

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
		byteStart = min(byteStart, len(src))
		byteEnd = min(byteEnd, len(src))

		// Advance cursor to byteStart, then byteEnd — never backward.
		runeStart := advanceTo(byteStart)

		// go/scanner emits tokens in increasing position order, so once a token
		// starts at or past end every remaining token does too: stop scanning
		// rather than tokenizing the rest of the file just to discard it.
		if runeStart >= end {
			break
		}

		// The start of a token is a safe place to resume from.
		cp.mark(ScanState{Pos: runeStart}, len(spans))

		runeEnd := advanceTo(byteEnd)
		reach = runeEnd

		// Only emit spans that overlap [start, end).
		if runeEnd <= start {
			continue
		}

		face, ok := faceForToken(tok, lit)
		if !ok {
			continue
		}

		spans = append(spans, Span{Start: runeStart, End: runeEnd, Face: face})
	}

	return spans, reach
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
