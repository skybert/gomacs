package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/syntax"
)

// ---------------------------------------------------------------------------
// findSpellSpans
// ---------------------------------------------------------------------------

func TestFindSpellSpansEmpty(t *testing.T) {
	spans := findSpellSpans("", []rune("hello world"), 0, 11)
	if spans != nil {
		t.Fatalf("expected nil spans with empty command, got %v", spans)
	}
}

func TestFindSpellSpansNoMisspellings(t *testing.T) {
	// "aspell list" with valid words returns no output.
	// We simulate by passing a word set that won't match anything.
	// There's no easy way to unit-test without mocking, so test the
	// helper logic with a known no-match scenario.
	spans := findSpellSpans("", []rune(""), 0, 0)
	if spans != nil {
		t.Fatalf("expected nil spans for empty input, got %v", spans)
	}
}

// ---------------------------------------------------------------------------
// virtToOrigPos
// ---------------------------------------------------------------------------

func TestVirtToOrigPos(t *testing.T) {
	// Simulate two comment regions:
	//   orig[5:10] → virt[0:5]
	//   orig[20:25] → virt[6:11]  (virt[5] is the separator '\n')
	mapping := []commentMapping{
		{origStart: 5, virtStart: 0, length: 5},
		{origStart: 20, virtStart: 6, length: 5},
	}
	tests := []struct {
		virtPos  int
		wantOrig int
	}{
		{0, 5},
		{4, 9},
		{5, -1}, // separator '\n'
		{6, 20},
		{10, 24},
		{11, -1}, // out of range
	}
	for _, tc := range tests {
		got := virtToOrigPos(mapping, tc.virtPos)
		if got != tc.wantOrig {
			t.Errorf("virtToOrigPos(%d) = %d, want %d", tc.virtPos, got, tc.wantOrig)
		}
	}
}

// ---------------------------------------------------------------------------
// isSpellErrorAt
// ---------------------------------------------------------------------------

func TestIsSpellErrorAt(t *testing.T) {
	spans := []syntax.Span{
		{Start: 3, End: 7, Face: FaceSpellError},
		{Start: 10, End: 14, Face: FaceSpellError},
	}
	// Inside first span.
	for _, pos := range []int{3, 4, 5, 6} {
		if !isSpellErrorAt(spans, pos) {
			t.Errorf("isSpellErrorAt(%d): expected true", pos)
		}
	}
	// Outside spans.
	for _, pos := range []int{0, 2, 7, 8, 9, 14, 15} {
		if isSpellErrorAt(spans, pos) {
			t.Errorf("isSpellErrorAt(%d): expected false", pos)
		}
	}
	// Inside second span.
	for _, pos := range []int{10, 11, 12, 13} {
		if !isSpellErrorAt(spans, pos) {
			t.Errorf("isSpellErrorAt(%d): expected true", pos)
		}
	}
}

// ---------------------------------------------------------------------------
// spellCheckAll / spellCheckComments
// ---------------------------------------------------------------------------

func TestSpellCheckAllModes(t *testing.T) {
	allModes := []string{"markdown", "text", "fundamental", ""}
	for _, m := range allModes {
		if !spellCheckAll(m) {
			t.Errorf("spellCheckAll(%q): expected true", m)
		}
	}
	notAllModes := []string{"go", "python", "java", "bash", "elisp", "json"}
	for _, m := range notAllModes {
		if spellCheckAll(m) {
			t.Errorf("spellCheckAll(%q): expected false", m)
		}
	}
}

func TestSpellCheckCommentsModes(t *testing.T) {
	commentModes := []string{"go", "python", "java", "bash", "elisp"}
	for _, m := range commentModes {
		if !spellCheckComments(m) {
			t.Errorf("spellCheckComments(%q): expected true", m)
		}
	}
	// markdown/text/fundamental use full-text checking, not comment-only.
	nonCommentModes := []string{"markdown", "text", "fundamental", "json"}
	for _, m := range nonCommentModes {
		if spellCheckComments(m) {
			t.Errorf("spellCheckComments(%q): expected false", m)
		}
	}
}
