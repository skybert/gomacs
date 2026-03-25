package syntax

import (
	"testing"
)

func TestManParse_Bold(t *testing.T) {
	// Bold: c\bc
	raw := "l\bls\bs"
	plain, spans := ManParse(raw)
	if plain != "ls" {
		t.Fatalf("plain = %q, want %q", plain, "ls")
	}
	if len(spans) == 0 {
		t.Fatal("expected bold spans, got none")
	}
	for _, s := range spans {
		if s.Face != FaceKeyword {
			t.Errorf("bold span face = %+v, want FaceKeyword", s.Face)
		}
	}
}

func TestManParse_Underline(t *testing.T) {
	// Underline: _\bc
	raw := "_\bf_\bo_\bo"
	plain, spans := ManParse(raw)
	if plain != "foo" {
		t.Fatalf("plain = %q, want %q", plain, "foo")
	}
	if len(spans) == 0 {
		t.Fatal("expected underline spans, got none")
	}
	for _, s := range spans {
		if s.Face != FaceType {
			t.Errorf("underline span face = %+v, want FaceType", s.Face)
		}
	}
}

func TestManParse_SectionHeader(t *testing.T) {
	raw := "NAME\n       ls - list directory contents\n"
	plain, spans := ManParse(raw)

	// The plain text should be unchanged (no overstrike).
	if plain != raw {
		t.Fatalf("plain = %q, want %q", plain, raw)
	}

	// There should be a span covering "NAME" with FaceHeader1.
	found := false
	for _, s := range spans {
		if plain[s.Start:s.End] == "NAME" && s.Face == FaceHeader1 {
			found = true
		}
	}
	if !found {
		t.Errorf("no FaceHeader1 span for section header %q; spans = %v", "NAME", spans)
	}
}

func TestManParse_OptionFlag(t *testing.T) {
	raw := "       -l     use a long listing format\n"
	plain, spans := ManParse(raw)

	found := false
	for _, s := range spans {
		if s.Start < len([]rune(plain)) && s.End <= len([]rune(plain)) {
			chunk := string([]rune(plain)[s.Start:s.End])
			if chunk == "-l" && s.Face == FaceFunction {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no FaceFunction span for option flag -l; spans = %v", spans)
	}
}

func TestManParse_NoOverstrike(t *testing.T) {
	raw := "plain text without any formatting"
	plain, _ := ManParse(raw)
	if plain != raw {
		t.Fatalf("plain = %q, want %q", plain, raw)
	}
}
