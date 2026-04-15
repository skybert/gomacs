package syntax

import (
	"testing"
)

func TestConfHighlighter_Comment(t *testing.T) {
	h := ConfHighlighter{}
	text := "# this is a comment\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d: %v", len(spans), spans)
	}
	if spans[0].Face != FaceComment {
		t.Errorf("expected FaceComment, got %v", spans[0].Face)
	}
}

func TestConfHighlighter_SemicolonComment(t *testing.T) {
	h := ConfHighlighter{}
	text := "; semicolon comment\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 || spans[0].Face != FaceComment {
		t.Errorf("expected FaceComment for semicolon comment, got %v", spans)
	}
}

func TestConfHighlighter_SectionHeader(t *testing.T) {
	h := ConfHighlighter{}
	text := "[database]\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d: %v", len(spans), spans)
	}
	if spans[0].Face != FaceType {
		t.Errorf("expected FaceType for section header, got %v", spans[0].Face)
	}
}

func TestConfHighlighter_TOMLArrayTable(t *testing.T) {
	h := ConfHighlighter{}
	text := "[[servers]]\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 || spans[0].Face != FaceType {
		t.Errorf("expected FaceType for TOML array-of-tables, got %v", spans)
	}
}

func TestConfHighlighter_KeyValue(t *testing.T) {
	h := ConfHighlighter{}
	text := "host = localhost\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 {
		t.Fatalf("expected 1 span for key, got %d: %v", len(spans), spans)
	}
	if spans[0].Face != FaceFunction {
		t.Errorf("expected FaceFunction for key, got %v", spans[0].Face)
	}
}

func TestConfHighlighter_KeyStringValue(t *testing.T) {
	h := ConfHighlighter{}
	text := `name = "Alice"` + "\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var keySpan, strSpan *Span
	for i := range spans {
		switch spans[i].Face {
		case FaceFunction:
			keySpan = &spans[i]
		case FaceString:
			strSpan = &spans[i]
		}
	}
	if keySpan == nil {
		t.Error("expected FaceFunction span for key")
	}
	if strSpan == nil {
		t.Error("expected FaceString span for string value")
	}
}

func TestConfHighlighter_BooleanValue(t *testing.T) {
	h := ConfHighlighter{}
	text := "enabled = true\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kwSpan *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kwSpan = &spans[i]
		}
	}
	if kwSpan == nil {
		t.Error("expected FaceKeyword span for boolean value 'true'")
	}
}

func TestConfHighlighter_NumberValue(t *testing.T) {
	h := ConfHighlighter{}
	text := "port = 5432\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var numSpan *Span
	for i := range spans {
		if spans[i].Face == FaceNumber {
			numSpan = &spans[i]
		}
	}
	if numSpan == nil {
		t.Error("expected FaceNumber span for numeric value")
	}
}

func TestConfHighlighter_InlineComment(t *testing.T) {
	h := ConfHighlighter{}
	text := "host = localhost # the db host\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var commentSpan *Span
	for i := range spans {
		if spans[i].Face == FaceComment {
			commentSpan = &spans[i]
		}
	}
	if commentSpan == nil {
		t.Error("expected FaceComment for inline comment")
	}
}

func TestConfHighlighter_ColonSeparator(t *testing.T) {
	h := ConfHighlighter{}
	text := "timeout: 30\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var keySpan, numSpan *Span
	for i := range spans {
		switch spans[i].Face {
		case FaceFunction:
			keySpan = &spans[i]
		case FaceNumber:
			numSpan = &spans[i]
		}
	}
	if keySpan == nil {
		t.Error("expected FaceFunction for key with colon separator")
	}
	if numSpan == nil {
		t.Error("expected FaceNumber for numeric value with colon separator")
	}
}
