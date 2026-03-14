package editor

import (
	"testing"
)

// ---------------------------------------------------------------------------
// netBraceCountJSON
// ---------------------------------------------------------------------------

func TestNetBraceCountJSONEmpty(t *testing.T) {
	if got := netBraceCountJSON(""); got != 0 {
		t.Errorf("empty line: want 0, got %d", got)
	}
}

func TestNetBraceCountJSONOpenCurly(t *testing.T) {
	if got := netBraceCountJSON("{"); got != 1 {
		t.Errorf("{: want 1, got %d", got)
	}
}

func TestNetBraceCountJSONOpenSquare(t *testing.T) {
	if got := netBraceCountJSON("["); got != 1 {
		t.Errorf("[: want 1, got %d", got)
	}
}

func TestNetBraceCountJSONClose(t *testing.T) {
	if got := netBraceCountJSON("}"); got != -1 {
		t.Errorf("}: want -1, got %d", got)
	}
	if got := netBraceCountJSON("]"); got != -1 {
		t.Errorf("]: want -1, got %d", got)
	}
}

func TestNetBraceCountJSONIgnoresStrings(t *testing.T) {
	// Braces inside strings must not be counted.
	if got := netBraceCountJSON(`"{ not a brace }"`); got != 0 {
		t.Errorf("brace in string: want 0, got %d", got)
	}
}

func TestNetBraceCountJSONEscapedQuote(t *testing.T) {
	// An escaped quote must not end the string prematurely.
	// The { after the escaped quote is still inside the string.
	if got := netBraceCountJSON(`"he said \"hello\" {"`); got != 0 {
		t.Errorf("escaped quote in string: want 0, got %d", got)
	}
}

func TestNetBraceCountJSONMixed(t *testing.T) {
	// "key": { opens one level
	if got := netBraceCountJSON(`"key": {`); got != 1 {
		t.Errorf(`"key": {: want 1, got %d`, got)
	}
}

// ---------------------------------------------------------------------------
// calcIndentJSON
// ---------------------------------------------------------------------------

func TestCalcIndentJSONTopLevel(t *testing.T) {
	lines := []string{`{`}
	// Line 0 is { itself; no preceding lines → depth 0 before line 0.
	if got := calcIndentJSON(lines, 0, "  "); got != "" {
		t.Errorf("top-level opening brace: want \"\", got %q", got)
	}
}

func TestCalcIndentJSONFirstKey(t *testing.T) {
	lines := []string{`{`, `"key": "value"`}
	// After {, depth is 1 → one level of indent.
	if got := calcIndentJSON(lines, 1, "  "); got != "  " {
		t.Errorf("first key: want \"  \", got %q", got)
	}
}

func TestCalcIndentJSONClosingBrace(t *testing.T) {
	lines := []string{`{`, `"key": "value"`, `}`}
	// } dedents: depth accumulated from previous lines is 1, then -1 → 0.
	if got := calcIndentJSON(lines, 2, "  "); got != "" {
		t.Errorf("closing brace: want \"\", got %q", got)
	}
}

func TestCalcIndentJSONNestedObject(t *testing.T) {
	lines := []string{
		`{`,
		`  "outer": {`,
		``,
	}
	// After two lines that each open one brace, depth = 2.
	if got := calcIndentJSON(lines, 2, "  "); got != "    " {
		t.Errorf("nested object: want \"    \", got %q", got)
	}
}

func TestCalcIndentJSONArray(t *testing.T) {
	lines := []string{`[`, ``}
	// After [, depth = 1.
	if got := calcIndentJSON(lines, 1, "  "); got != "  " {
		t.Errorf("array entry: want \"  \", got %q", got)
	}
}

func TestCalcIndentJSONClosingSquare(t *testing.T) {
	lines := []string{`[`, `  1`, `]`}
	// ] at start of line dedents from depth 1 → 0.
	if got := calcIndentJSON(lines, 2, "  "); got != "" {
		t.Errorf("closing bracket: want \"\", got %q", got)
	}
}
