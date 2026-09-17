package elisp

import "testing"

// Tests for lexer.go: the Elisp tokeniser.

func TestLexer_SetqExpr(t *testing.T) {
	l := NewLexer("(setq x 42)")
	tokens := []Token{}
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}

	expected := []struct {
		typ TokenType
		val string
	}{
		{TokenLParen, "("},
		{TokenSymbol, "setq"},
		{TokenSymbol, "x"},
		{TokenInt, "42"},
		{TokenRParen, ")"},
		{TokenEOF, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, e := range expected {
		if tokens[i].Type != e.typ {
			t.Errorf("token[%d]: expected type %d, got %d", i, e.typ, tokens[i].Type)
		}
		if e.val != "" && tokens[i].Value != e.val {
			t.Errorf("token[%d]: expected value %q, got %q", i, e.val, tokens[i].Value)
		}
	}
}

func TestLexer_String(t *testing.T) {
	l := NewLexer(`"hello\nworld"`)
	tok := l.Next()
	if tok.Type != TokenString {
		t.Fatalf("expected TokenString, got %d", tok.Type)
	}
	if tok.Value != "hello\nworld" {
		t.Fatalf("expected hello\\nworld, got %q", tok.Value)
	}
}

func TestLexer_StringEscapes(t *testing.T) {
	l := NewLexer(`"a\tb\"c\\d\q"`)
	tok := l.Next()
	if tok.Type != TokenString {
		t.Fatalf("expected TokenString, got %d", tok.Type)
	}
	if tok.Value != "a\tb\"c\\d\\q" {
		t.Fatalf("unexpected escape decoding: %q", tok.Value)
	}
}

func TestLexer_StringUnterminated(t *testing.T) {
	l := NewLexer(`"no end`)
	tok := l.Next()
	if tok.Type != TokenString || tok.Value != "no end" {
		t.Fatalf("unterminated string: type=%d val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_StringTrailingBackslash(t *testing.T) {
	l := NewLexer(`"x\`)
	tok := l.Next()
	if tok.Type != TokenString || tok.Value != "x" {
		t.Fatalf("trailing backslash: type=%d val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_Comment(t *testing.T) {
	l := NewLexer("; this is a comment\n42")
	tok := l.Next()
	if tok.Type != TokenInt || tok.Value != "42" {
		t.Fatalf("expected int 42, got type=%d val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_Float(t *testing.T) {
	l := NewLexer("3.14")
	tok := l.Next()
	if tok.Type != TokenFloat {
		t.Fatalf("expected TokenFloat, got %d", tok.Type)
	}
	if tok.Value != "3.14" {
		t.Fatalf("expected 3.14, got %q", tok.Value)
	}
}

func TestLexer_Quote(t *testing.T) {
	l := NewLexer("'foo")
	tok := l.Next()
	if tok.Type != TokenQuote {
		t.Fatalf("expected TokenQuote")
	}
	tok = l.Next()
	if tok.Type != TokenSymbol || tok.Value != "foo" {
		t.Fatalf("expected symbol foo")
	}
}

func TestLexer_CommaAt(t *testing.T) {
	l := NewLexer(",@x")
	tok := l.Next()
	if tok.Type != TokenCommaAt {
		t.Fatalf("expected TokenCommaAt, got %d", tok.Type)
	}
}

// -------------------------------------------------------------------
// lexer — peek1/peek2 at EOF
// -------------------------------------------------------------------

func TestLexer_Peek1AtEOF(t *testing.T) {
	l := NewLexer("")
	if l.peek1() != 0 {
		t.Fatal("peek1 at EOF should return 0")
	}
}

func TestLexer_Peek2AtEOF(t *testing.T) {
	l := NewLexer("x")
	if l.peek2() != 0 {
		t.Fatal("peek2 at EOF should return 0")
	}
}

// -------------------------------------------------------------------
// lexer — readString escape sequences
// -------------------------------------------------------------------

func TestLexer_ReadStringTabEscape(t *testing.T) {
	l := NewLexer(`"\t"`)
	tok := l.Next()
	if tok.Type != TokenString || tok.Value != "\t" {
		t.Fatalf("expected tab char, got %q", tok.Value)
	}
}

func TestLexer_ReadStringUnknownEscape(t *testing.T) {
	l := NewLexer(`"\q"`)
	tok := l.Next()
	if tok.Type != TokenString || tok.Value != `\q` {
		t.Fatalf("expected literal \\q, got %q", tok.Value)
	}
}

func TestLexer_ReadStringEOFInEscape(t *testing.T) {
	// Unterminated string with backslash at end.
	l := NewLexer(`"\`)
	tok := l.Next()
	// Should return a string token (possibly empty/partial) without panicking.
	if tok.Type != TokenString {
		t.Fatalf("expected TokenString even for unterminated escape, got %v", tok.Type)
	}
}

// -------------------------------------------------------------------
// lexer — readNumber float with exponent
// -------------------------------------------------------------------

func TestLexer_ReadNumberExponent(t *testing.T) {
	l := NewLexer("1e5")
	tok := l.Next()
	if tok.Type != TokenFloat || tok.Value != "1e5" {
		t.Fatalf("expected TokenFloat \"1e5\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_ReadNumberNegativeExponent(t *testing.T) {
	l := NewLexer("2.5e-3")
	tok := l.Next()
	if tok.Type != TokenFloat || tok.Value != "2.5e-3" {
		t.Fatalf("expected TokenFloat \"2.5e-3\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_ReadNumberDotTrailingEOF(t *testing.T) {
	// "1." — dot followed by EOF; peek2 returns 0 which is not a digit,
	// but the condition `next == 0` makes it treated as float.
	l := NewLexer("1.")
	tok := l.Next()
	if tok.Type != TokenFloat {
		t.Fatalf("expected TokenFloat for \"1.\", got %v", tok.Type)
	}
}

// -------------------------------------------------------------------
// Numeric literals reached through readToken's sign / dot dispatch
// -------------------------------------------------------------------

func TestLexer_DotFollowedByDigitIsFloat(t *testing.T) {
	// ".5" — the '.' case in readToken must dispatch to readNumber rather
	// than emitting a TokenDot.
	l := NewLexer(".5")
	tok := l.Next()
	if tok.Type != TokenFloat || tok.Value != ".5" {
		t.Fatalf("expected TokenFloat \".5\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_NegativeIntegerLiteral(t *testing.T) {
	l := NewLexer("-42")
	tok := l.Next()
	if tok.Type != TokenInt || tok.Value != "-42" {
		t.Fatalf("expected TokenInt \"-42\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_PlusSignedIntegerLiteral(t *testing.T) {
	l := NewLexer("+7")
	tok := l.Next()
	if tok.Type != TokenInt || tok.Value != "+7" {
		t.Fatalf("expected TokenInt \"+7\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_MinusAloneIsSymbol(t *testing.T) {
	// A '-' not followed by a digit stays a symbol (the subtraction function).
	l := NewLexer("- 1")
	tok := l.Next()
	if tok.Type != TokenSymbol || tok.Value != "-" {
		t.Fatalf("expected TokenSymbol \"-\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_NegativeFloatLiteral(t *testing.T) {
	l := NewLexer("-2.5")
	tok := l.Next()
	if tok.Type != TokenFloat || tok.Value != "-2.5" {
		t.Fatalf("expected TokenFloat \"-2.5\", got type=%v val=%q", tok.Type, tok.Value)
	}
}
