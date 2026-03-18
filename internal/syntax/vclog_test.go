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

func TestVcAnnotateHighlighterWithSourceHighlighting(t *testing.T) {
	// A go blame line: the source is "package main" which should get keyword
	// highlighting for "package".
	h := VcAnnotateHighlighter{Source: GoHighlighter{}}
	text := "dc84300f (Torstein Johansen 2026-03-16  1) package main\n"
	runes := []rune(text)
	spans := h.Highlight(text, 0, len(runes))

	// Find a span covering "package" in the source portion.
	found := false
	for _, sp := range spans {
		s := string(runes[sp.Start:sp.End])
		if s == "package" && sp.Face == FaceKeyword {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'package' keyword span from source highlighting, not found")
	}
}

func TestLangToHighlighter(t *testing.T) {
	tests := []struct {
		lang string
		want bool // true if a non-nil highlighter is expected
	}{
		{"go", true},
		{"python", true},
		{"java", true},
		{"bash", true},
		{"json", true},
		{"markdown", true},
		{"elisp", true},
		{"makefile", true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		got := LangToHighlighter(tt.lang)
		if (got != nil) != tt.want {
			t.Errorf("LangToHighlighter(%q): got %v, wantNonNil=%v", tt.lang, got, tt.want)
		}
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

func TestVcGrepHighlighter(t *testing.T) {
	h := VcGrepHighlighter{}
	text := "internal/editor/editor.go:42:type Editor struct {\n"
	runes := []rune(text)
	spans := h.Highlight(text, 0, len(runes))

	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans (filename + linenum), got %d", len(spans))
	}
	filename := string(runes[spans[0].Start:spans[0].End])
	if filename != "internal/editor/editor.go" {
		t.Errorf("filename span: got %q, want %q", filename, "internal/editor/editor.go")
	}
	if spans[0].Face != FaceVcGrepFile {
		t.Errorf("filename face: got %v, want FaceVcGrepFile", spans[0].Face)
	}
	linenum := string(runes[spans[1].Start:spans[1].End])
	if linenum != "42" {
		t.Errorf("linenum span: got %q, want %q", linenum, "42")
	}
	if spans[1].Face != FaceVcGrepLine {
		t.Errorf("linenum face: got %v, want FaceVcGrepLine", spans[1].Face)
	}
}

func TestVcShowHighlighterHeader(t *testing.T) {
	h := VcShowHighlighter{}
	text := "commit abc1234def5678\nAuthor: Jane Doe <jane@example.com>\nDate:   Mon Mar 17 10:00:00 2026\n\n    Fix the bug\n"
	runes := []rune(text)
	spans := h.Highlight(text, 0, len(runes))

	var foundCommitLabel, foundSHA, foundAuthor, foundDate bool
	for _, sp := range spans {
		s := string(runes[sp.Start:sp.End])
		switch {
		case s == "commit" && sp.Face == FaceKeyword:
			foundCommitLabel = true
		case s == "abc1234def5678" && sp.Face == FaceVcLogSHA:
			foundSHA = true
		case s == "Author:" && sp.Face == FaceKeyword:
			foundAuthor = true
		case s == "Date:" && sp.Face == FaceKeyword:
			foundDate = true
		}
	}
	if !foundCommitLabel {
		t.Error("expected 'commit' label span with FaceKeyword")
	}
	if !foundSHA {
		t.Error("expected SHA span with FaceVcLogSHA")
	}
	if !foundAuthor {
		t.Error("expected 'Author:' label span with FaceKeyword")
	}
	if !foundDate {
		t.Error("expected 'Date:' label span with FaceKeyword")
	}
}

func TestVcShowHighlighterWithDiff(t *testing.T) {
	h := VcShowHighlighter{}
	text := "commit abc1234\nAuthor: Jane <j@x.com>\n\n    Fix\n\ndiff --git a/foo.go b/foo.go\n+added line\n-removed line\n"
	runes := []rune(text)
	spans := h.Highlight(text, 0, len(runes))

	var foundAdded, foundRemoved bool
	for _, sp := range spans {
		s := string(runes[sp.Start:sp.End])
		if s == "+added line" && sp.Face == FaceDiffAdded {
			foundAdded = true
		}
		if s == "-removed line" && sp.Face == FaceDiffRemoved {
			foundRemoved = true
		}
	}
	if !foundAdded {
		t.Error("expected added diff line span")
	}
	if !foundRemoved {
		t.Error("expected removed diff line span")
	}
}
