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

func TestManFlagSpans_BracketAndComma(t *testing.T) {
	// Flags preceded by '[' and ',' must be recognized.
	line := "[--verbose,--quiet]"
	spans := manFlagSpans(line, 0)
	if len(spans) < 2 {
		t.Fatalf("expected >=2 flag spans, got %d: %v", len(spans), spans)
	}
	for _, sp := range spans {
		if sp.Face != FaceFunction {
			t.Errorf("flag span face = %+v, want FaceFunction", sp.Face)
		}
	}
}

func TestManFlagSpans_LoneDashRejected(t *testing.T) {
	// A lone '-' (e.g. stdin marker) with no following option char is skipped.
	line := "read from - now"
	spans := manFlagSpans(line, 0)
	if len(spans) != 0 {
		t.Errorf("expected no flag spans for lone '-', got %v", spans)
	}
}

func TestManFlagSpans_Offset(t *testing.T) {
	// lineOffset must be added to span positions.
	const off = 100
	spans := manFlagSpans("--help", off)
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Start != off {
		t.Errorf("span start = %d, want %d", spans[0].Start, off)
	}
}

func TestManParse_Empty(t *testing.T) {
	plain, spans := ManParse("")
	if plain != "" {
		t.Errorf("plain = %q, want empty", plain)
	}
	if len(spans) != 0 {
		t.Errorf("expected no spans for empty input, got %v", spans)
	}
}
