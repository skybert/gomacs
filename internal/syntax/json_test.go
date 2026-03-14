package syntax

import (
	"testing"
)

func jsonSpans(text string) []Span {
	h := JSONHighlighter{}
	return h.Highlight(text, 0, len([]rune(text)))
}

func findJSONSpan(spans []Span, face Face) (Span, bool) {
	for _, s := range spans {
		if s.Face == face {
			return s, true
		}
	}
	return Span{}, false
}

func TestJSONString(t *testing.T) {
	spans := jsonSpans(`"hello"`)
	s, ok := findJSONSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected a string span")
	}
	if s.Start != 0 || s.End != 7 {
		t.Errorf("string span: want [0,7), got [%d,%d)", s.Start, s.End)
	}
}

func TestJSONStringEscape(t *testing.T) {
	spans := jsonSpans(`"hel\"lo"`)
	s, ok := findJSONSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected a string span")
	}
	if s.End != 9 {
		t.Errorf("escaped string end: want 9, got %d", s.End)
	}
}

func TestJSONNumber(t *testing.T) {
	spans := jsonSpans("42")
	s, ok := findJSONSpan(spans, FaceNumber)
	if !ok {
		t.Fatal("expected a number span")
	}
	if s.Start != 0 || s.End != 2 {
		t.Errorf("number span: want [0,2), got [%d,%d)", s.Start, s.End)
	}
}

func TestJSONNegativeFloat(t *testing.T) {
	spans := jsonSpans("-3.14")
	s, ok := findJSONSpan(spans, FaceNumber)
	if !ok {
		t.Fatal("expected a number span")
	}
	if s.Start != 0 || s.End != 5 {
		t.Errorf("float span: want [0,5), got [%d,%d)", s.Start, s.End)
	}
}

func TestJSONTrue(t *testing.T) {
	spans := jsonSpans("true")
	s, ok := findJSONSpan(spans, FaceKeyword)
	if !ok {
		t.Fatal("expected keyword span for 'true'")
	}
	if s.Start != 0 || s.End != 4 {
		t.Errorf("true span: want [0,4), got [%d,%d)", s.Start, s.End)
	}
}

func TestJSONFalse(t *testing.T) {
	spans := jsonSpans("false")
	s, ok := findJSONSpan(spans, FaceKeyword)
	if !ok {
		t.Fatal("expected keyword span for 'false'")
	}
	if s.Start != 0 || s.End != 5 {
		t.Errorf("false span: want [0,5), got [%d,%d)", s.Start, s.End)
	}
}

func TestJSONNull(t *testing.T) {
	spans := jsonSpans("null")
	s, ok := findJSONSpan(spans, FaceKeyword)
	if !ok {
		t.Fatal("expected keyword span for 'null'")
	}
	if s.Start != 0 || s.End != 4 {
		t.Errorf("null span: want [0,4), got [%d,%d)", s.Start, s.End)
	}
}

func TestJSONStructural(t *testing.T) {
	// Structural characters produce no spans.
	spans := jsonSpans("{}")
	if len(spans) != 0 {
		t.Errorf("structural chars should not be highlighted, got %v", spans)
	}
}

func TestJSONMixed(t *testing.T) {
	src := `{"key": 1, "flag": true, "nothing": null}`
	spans := jsonSpans(src)
	// Expect string spans for keys and string values, number, keyword spans.
	strCount, numCount, kwCount := 0, 0, 0
	for _, s := range spans {
		switch s.Face {
		case FaceString:
			strCount++
		case FaceNumber:
			numCount++
		case FaceKeyword:
			kwCount++
		}
	}
	if strCount < 2 {
		t.Errorf("expected >=2 string spans, got %d", strCount)
	}
	if numCount != 1 {
		t.Errorf("expected 1 number span, got %d", numCount)
	}
	if kwCount != 2 {
		t.Errorf("expected 2 keyword spans (true, null), got %d", kwCount)
	}
}
