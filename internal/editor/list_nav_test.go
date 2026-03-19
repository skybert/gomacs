package editor

import (
	"testing"
)

// ---------------------------------------------------------------------------
// forward-list
// ---------------------------------------------------------------------------

func TestForwardListParens(t *testing.T) {
	e := newTestEditor("(hello world)")
	buf(e).SetPoint(0)
	e.cmdForwardList()
	// Point should be after the closing ')' = position 13.
	if got := buf(e).Point(); got != 13 {
		t.Fatalf("forward-list: want point=13, got %d", got)
	}
}

func TestForwardListInsideParens(t *testing.T) {
	// Point at pos 1 inside "(foo (bar) baz)".
	// Scanning from pos 1: '(' at 5 → depth=1; ')' at 9 → depth=0 → done.
	// forward-list moves to the first complete balanced expression → pos 10.
	e := newTestEditor("(foo (bar) baz)")
	buf(e).SetPoint(1)
	e.cmdForwardList()
	if got := buf(e).Point(); got != 10 {
		t.Fatalf("forward-list inside: want point=10, got %d", got)
	}
}

func TestForwardListCurly(t *testing.T) {
	e := newTestEditor("func foo() {\n\treturn 1\n}")
	buf(e).SetPoint(0)
	e.cmdForwardList()
	// First ')' at position 10 (after "func foo(")
	// text: f u n c   f o o ( )   { \n \t r e t u r n   1 \n }
	// pos:  0 1 2 3 4 5 6 7 8 9 10...
	// '(' at 8, ')' at 9, ')' closes depth 0: no, '(' opens depth=1 then ')' closes to depth=0
	// Wait: depth starts at 0. '(' → depth=1; ')' → depth=0, found at pos 9, point=10
	if got := buf(e).Point(); got != 10 {
		t.Fatalf("forward-list curly: want point=10, got %d", got)
	}
}

func TestForwardListNoList(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdForwardList()
	// No brackets, point should remain at 0.
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("forward-list no-list: want point=0, got %d", got)
	}
}

func TestForwardListSquareBrackets(t *testing.T) {
	e := newTestEditor("arr[0]")
	buf(e).SetPoint(0)
	e.cmdForwardList()
	// '[' at pos 3 → depth=1; ']' at pos 5 → depth=0, point=6
	// But at depth=0, ']' at pos 5 → point=6.
	// Wait: scanning from pos 0:
	//   'a','r','r' → no bracket
	//   '[' at 3 → depth=1
	//   '0' → skip
	//   ']' at 5 → depth=0, found, point=6
	if got := buf(e).Point(); got != 6 {
		t.Fatalf("forward-list square: want point=6, got %d", got)
	}
}

func TestForwardListAtClosingBracket(t *testing.T) {
	// If point is at a closing bracket at depth 0, it should move past it.
	e := newTestEditor("(foo) bar")
	buf(e).SetPoint(4) // point at ')'
	e.cmdForwardList()
	// ')' at pos 4, depth=0 → point=5
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("forward-list at close: want point=5, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// backward-list
// ---------------------------------------------------------------------------

func TestBackwardListParens(t *testing.T) {
	e := newTestEditor("(hello world)")
	buf(e).SetPoint(13) // after the ')'
	e.cmdBackwardList()
	// ')' at 12, depth=1; '(' at 0 → depth=0, point=0
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("backward-list: want point=0, got %d", got)
	}
}

func TestBackwardListNested(t *testing.T) {
	// "(foo (bar))"
	// pos: 0 1 2 3 4 5 6 7 8 9 10
	// chars: ( f o o   ( b a r )  )
	e := newTestEditor("(foo (bar))")
	buf(e).SetPoint(11) // after outer ')'
	e.cmdBackwardList()
	// Scanning backwards from pos 10:
	//   ')' at 10 → depth=1
	//   ')' at 9 → depth=2
	//   '(' at 5 → depth=1
	//   '(' at 0 → depth=0, found, point=0
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("backward-list nested: want point=0, got %d", got)
	}
}

func TestBackwardListNoList(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(11)
	e.cmdBackwardList()
	// No brackets, point should remain at 11.
	if got := buf(e).Point(); got != 11 {
		t.Fatalf("backward-list no-list: want point=11, got %d", got)
	}
}

func TestBackwardListSquareBrackets(t *testing.T) {
	// "arr[0]"
	// pos:  0 1 2 3 4 5
	// char: a r r [ 0 ]
	e := newTestEditor("arr[0]")
	buf(e).SetPoint(6) // after ']'
	e.cmdBackwardList()
	// ']' at 5 → depth=1; '[' at 3 → depth=0, point=3
	if got := buf(e).Point(); got != 3 {
		t.Fatalf("backward-list square: want point=3, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// readWordFrom / readWordEndingAt
// ---------------------------------------------------------------------------

func TestReadWordFrom(t *testing.T) {
	e := newTestEditor("hello world")
	word, length := readWordFrom(e.ActiveBuffer(), 0, 11)
	if word != "hello" || length != 5 {
		t.Fatalf("readWordFrom: got word=%q length=%d, want %q 5", word, length, "hello")
	}
}

func TestReadWordEndingAt(t *testing.T) {
	e := newTestEditor("hello world")
	word, start := readWordEndingAt(e.ActiveBuffer(), 4)
	if word != "hello" || start != 0 {
		t.Fatalf("readWordEndingAt: got word=%q start=%d, want %q 0", word, start, "hello")
	}
}

func TestReadWordEndingAtMid(t *testing.T) {
	e := newTestEditor("hello world")
	// "world" ends at 10, starts at 6
	word, start := readWordEndingAt(e.ActiveBuffer(), 10)
	if word != "world" || start != 6 {
		t.Fatalf("readWordEndingAt mid: got word=%q start=%d, want %q 6", word, start, "world")
	}
}
