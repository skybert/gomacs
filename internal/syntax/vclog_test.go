package syntax

import (
	"testing"
)

func TestVcLogHighlighter(t *testing.T) {
	h := VcLogHighlighter{}
	text := "dc84300 Add theming\n12a8b7b Update known issues\n"
	spans := h.Highlight(text, 0, len([]rune(text)))

	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	runes := []rune(text)
	sha1 := string(runes[spans[0].Start:spans[0].End])
	if sha1 != "dc84300" {
		t.Errorf("first SHA: got %q, want %q", sha1, "dc84300")
	}
	sha2 := string(runes[spans[1].Start:spans[1].End])
	if sha2 != "12a8b7b" {
		t.Errorf("second SHA: got %q, want %q", sha2, "12a8b7b")
	}
}

func TestVcLogHighlighterShortHash(t *testing.T) {
	h := VcLogHighlighter{}
	// A word shorter than 7 hex chars should not be highlighted.
	text := "abc123 short\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 0 {
		t.Errorf("expected no spans for short hash, got %d", len(spans))
	}
}

func TestVcAnnotateHighlighter(t *testing.T) {
	h := VcAnnotateHighlighter{}
	text := "dc84300f (Torstein Johansen 2026-03-16  1) package main\n"
	runes := []rune(text)
	spans := h.Highlight(text, 0, len(runes))

	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans (SHA + metadata), got %d", len(spans))
	}
	sha := string(runes[spans[0].Start:spans[0].End])
	if sha != "dc84300f" {
		t.Errorf("SHA span: got %q, want %q", sha, "dc84300f")
	}
	meta := string(runes[spans[1].Start:spans[1].End])
	if meta != "(Torstein Johansen 2026-03-16  1)" {
		t.Errorf("metadata span: got %q, want %q", meta, "(Torstein Johansen 2026-03-16  1)")
	}
	if spans[1].Face != FaceComment {
		t.Errorf("metadata face: got %v, want FaceComment", spans[1].Face)
	}
}

func TestVcCommitHighlighter(t *testing.T) {
	h := VcCommitHighlighter{}
	text := "Fix the bug\n# Changes to be committed:\n# \tmodified: foo.go\n"
	runes := []rune(text)
	spans := h.Highlight(text, 0, len(runes))

	if len(spans) != 2 {
		t.Fatalf("expected 2 comment spans, got %d", len(spans))
	}
	for _, sp := range spans {
		if sp.Face != FaceComment {
			t.Errorf("expected FaceComment, got %v", sp.Face)
		}
		line := string(runes[sp.Start:sp.End])
		if len(line) == 0 || line[0] != '#' {
			t.Errorf("expected comment line starting with '#', got %q", line)
		}
	}
}
