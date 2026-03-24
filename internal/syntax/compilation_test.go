package syntax

import "testing"

func TestCompilationHighlighter_golangciLint(t *testing.T) {
	line := "internal/editor/nav.go:218:3: QF1012: Use fmt.Fprintf instead of WriteString(fmt.Sprintf(...))"
	hl := CompilationHighlighter{}
	spans := hl.Highlight(line, 0, len([]rune(line)))
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans, got %d: %v", len(spans), spans)
	}
	// First span: filename
	if spans[0].Face != FaceType {
		t.Errorf("span[0] face = %+v, want FaceType", spans[0].Face)
	}
	wantFile := "internal/editor/nav.go"
	got := line[spans[0].Start:spans[0].End]
	if got != wantFile {
		t.Errorf("span[0] text = %q, want %q", got, wantFile)
	}
	// Second span: :218:3: coordinates
	if spans[1].Face != FaceNumber {
		t.Errorf("span[1] face = %+v, want FaceNumber", spans[1].Face)
	}
}

func TestCompilationHighlighter_goCompiler(t *testing.T) {
	line := "main.go:10:5: undefined: Foo"
	hl := CompilationHighlighter{}
	spans := hl.Highlight(line, 0, len([]rune(line)))
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans, got %d", len(spans))
	}
	if line[spans[0].Start:spans[0].End] != "main.go" {
		t.Errorf("filename span = %q", line[spans[0].Start:spans[0].End])
	}
}

func TestCompilationHighlighter_noMatch(t *testing.T) {
	line := "Build succeeded"
	hl := CompilationHighlighter{}
	spans := hl.Highlight(line, 0, len([]rune(line)))
	if len(spans) != 0 {
		t.Errorf("expected no spans for plain line, got %v", spans)
	}
}

func TestCompilationHighlighter_multiline(t *testing.T) {
	text := "ok\ninternal/editor/nav.go:218:3: QF1012: msg\nok\n"
	hl := CompilationHighlighter{}
	spans := hl.Highlight(text, 0, len([]rune(text)))
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans for multi-line input, got %d", len(spans))
	}
	// Filename should start at rune offset 3 (after "ok\n")
	if spans[0].Start != 3 {
		t.Errorf("span[0].Start = %d, want 3", spans[0].Start)
	}
}
