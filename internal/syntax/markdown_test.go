package syntax

import (
	"testing"
	"unicode/utf8"
)

func mdHighlight(text string) []Span {
	h := MarkdownHighlighter{}
	return h.Highlight(text, 0, utf8.RuneCountInString(text))
}

// findSpanWithFace returns the first span whose Face equals want, or nil.
func findSpanWithFace(spans []Span, want Face) *Span {
	for i := range spans {
		if spans[i].Face == want {
			return &spans[i]
		}
	}
	return nil
}

// requireSpan fails the test if no span with the given face exists and
// returns the span.  It returns nil (after t.Errorf) when not found so the
// caller can return early; use as: sp := requireSpan(...); if sp == nil { return }
func requireSpan(t *testing.T, spans []Span, face Face, msg string) *Span {
	t.Helper()
	sp := findSpanWithFace(spans, face)
	if sp == nil {
		t.Errorf("%s; got %v", msg, spans)
	}
	return sp
}

func TestMarkdownHeader1(t *testing.T) {
	text := "# Header One\n"
	spans := mdHighlight(text)
	sp := requireSpan(t, spans, FaceHeader1, "expected FaceHeader1 span")
	if sp == nil {
		return
	}
	lineRunes := utf8.RuneCountInString("# Header One\n")
	if sp.Start != 0 || sp.End != lineRunes {
		t.Errorf("FaceHeader1 span = [%d,%d), want [0,%d)", sp.Start, sp.End, lineRunes)
	}
}

func TestMarkdownHeader2(t *testing.T) {
	text := "## Header Two\n"
	spans := mdHighlight(text)
	requireSpan(t, spans, FaceHeader2, "expected FaceHeader2 span")
}

func TestMarkdownBold(t *testing.T) {
	text := "some **bold** text\n"
	spans := mdHighlight(text)
	sp := requireSpan(t, spans, FaceBold, "expected FaceBold span for **bold**")
	if sp == nil {
		return
	}
	want := "**bold**"
	wantStart := utf8.RuneCountInString("some ")
	wantEnd := wantStart + utf8.RuneCountInString(want)
	if sp.Start != wantStart || sp.End != wantEnd {
		t.Errorf("FaceBold span = [%d,%d), want [%d,%d)", sp.Start, sp.End, wantStart, wantEnd)
	}
}

func TestMarkdownInlineCode(t *testing.T) {
	text := "use `code` here\n"
	spans := mdHighlight(text)
	sp := requireSpan(t, spans, FaceCode, "expected FaceCode span for `code`")
	if sp == nil {
		return
	}
	wantStart := utf8.RuneCountInString("use ")
	wantEnd := wantStart + utf8.RuneCountInString("`code`")
	if sp.Start != wantStart || sp.End != wantEnd {
		t.Errorf("FaceCode span = [%d,%d), want [%d,%d)", sp.Start, sp.End, wantStart, wantEnd)
	}
}

func TestMarkdownBlockquote(t *testing.T) {
	text := "> a quote\n"
	spans := mdHighlight(text)
	requireSpan(t, spans, FaceBlockquote, "expected FaceBlockquote span")
}

func TestMarkdownCodeFence(t *testing.T) {
	text := "```\nfoo := 1\n```\n"
	spans := mdHighlight(text)

	// Every span should be FaceCode.
	if len(spans) == 0 {
		t.Fatal("expected code spans for fenced block; got none")
	}
	for _, sp := range spans {
		if sp.Face != FaceCode {
			t.Errorf("expected FaceCode, got %+v", sp.Face)
		}
	}

	// There should be three lines highlighted (opening ```, body, closing ```).
	if len(spans) != 3 {
		t.Errorf("expected 3 FaceCode spans (open fence, body, close fence); got %d: %v", len(spans), spans)
	}
}

func TestMarkdownLink(t *testing.T) {
	text := "see [Go](https://go.dev) for details\n"
	spans := mdHighlight(text)
	sp := requireSpan(t, spans, FaceLink, "expected FaceLink span")
	if sp == nil {
		return
	}
	wantStart := utf8.RuneCountInString("see ")
	wantEnd := wantStart + utf8.RuneCountInString("[Go](https://go.dev)")
	if sp.Start != wantStart || sp.End != wantEnd {
		t.Errorf("FaceLink span = [%d,%d), want [%d,%d)", sp.Start, sp.End, wantStart, wantEnd)
	}
}

func TestMarkdownItalic(t *testing.T) {
	text := "some *italic* text\n"
	spans := mdHighlight(text)
	requireSpan(t, spans, FaceItalic, "expected FaceItalic span")
}

func TestMarkdownWindowFilter(t *testing.T) {
	// Only the second line is in the window; first-line spans should be absent.
	text := "# Title\n> quote\n"
	secondLineStart := utf8.RuneCountInString("# Title\n")
	secondLineEnd := utf8.RuneCountInString(text)

	h := MarkdownHighlighter{}
	spans := h.Highlight(text, secondLineStart, secondLineEnd)

	for _, sp := range spans {
		if sp.End <= secondLineStart {
			t.Errorf("span %v is outside the requested window", sp)
		}
	}
	if sp := findSpanWithFace(spans, FaceHeader1); sp != nil {
		t.Errorf("FaceHeader1 span should not appear when window excludes first line: %v", sp)
	}
	if sp := findSpanWithFace(spans, FaceBlockquote); sp == nil {
		t.Errorf("expected FaceBlockquote span for second line")
	}
}
