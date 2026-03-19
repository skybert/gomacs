package syntax

import "testing"

func yamlSpans(text string) []Span {
	h := YAMLHighlighter{}
	return h.Highlight(text, 0, len([]rune(text)))
}

func findYAMLSpan(spans []Span, face Face) (Span, bool) {
	for _, s := range spans {
		if s.Face == face {
			return s, true
		}
	}
	return Span{}, false
}

func hasYAMLFaceAt(spans []Span, pos int, face Face) bool {
	for _, s := range spans {
		if s.Face == face && pos >= s.Start && pos < s.End {
			return true
		}
	}
	return false
}

func TestYAMLComment(t *testing.T) {
	spans := yamlSpans("# this is a comment\n")
	s, ok := findYAMLSpan(spans, FaceComment)
	if !ok {
		t.Fatal("expected a comment span")
	}
	if s.Start != 0 {
		t.Errorf("comment should start at 0, got %d", s.Start)
	}
}

func TestYAMLDocumentMarker(t *testing.T) {
	spans := yamlSpans("---\n")
	_, ok := findYAMLSpan(spans, FaceKeyword)
	if !ok {
		t.Fatal("expected keyword span for '---'")
	}
}

func TestYAMLMappingKey(t *testing.T) {
	spans := yamlSpans("name: Alice\n")
	if !hasYAMLFaceAt(spans, 0, FaceFunction) {
		t.Errorf("expected FaceFunction at pos 0 (key); got %v", spans)
	}
}

func TestYAMLStringValue(t *testing.T) {
	text := "key: \"hello world\"\n"
	spans := yamlSpans(text)
	_, ok := findYAMLSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected a string span for double-quoted value")
	}
}

func TestYAMLSingleQuotedString(t *testing.T) {
	text := "key: 'hello'\n"
	spans := yamlSpans(text)
	_, ok := findYAMLSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected a string span for single-quoted value")
	}
}

func TestYAMLNumber(t *testing.T) {
	spans := yamlSpans("count: 42\n")
	_, ok := findYAMLSpan(spans, FaceNumber)
	if !ok {
		t.Fatal("expected a number span")
	}
}

func TestYAMLBooleanTrue(t *testing.T) {
	spans := yamlSpans("enabled: true\n")
	if !hasYAMLFaceAt(spans, 9, FaceKeyword) {
		t.Errorf("expected FaceKeyword at pos 9 (true); got %v", spans)
	}
}

func TestYAMLBooleanFalse(t *testing.T) {
	spans := yamlSpans("enabled: false\n")
	if !hasYAMLFaceAt(spans, 9, FaceKeyword) {
		t.Errorf("expected FaceKeyword at pos 9 (false); got %v", spans)
	}
}

func TestYAMLNull(t *testing.T) {
	spans := yamlSpans("value: null\n")
	if !hasYAMLFaceAt(spans, 7, FaceKeyword) {
		t.Errorf("expected FaceKeyword at pos 7 (null); got %v", spans)
	}
}

func TestYAMLAnchor(t *testing.T) {
	spans := yamlSpans("base: &myanchor\n")
	_, ok := findYAMLSpan(spans, FaceType)
	if !ok {
		t.Fatal("expected FaceType for anchor &myanchor")
	}
}

func TestYAMLAlias(t *testing.T) {
	spans := yamlSpans("other: *myanchor\n")
	_, ok := findYAMLSpan(spans, FaceType)
	if !ok {
		t.Fatal("expected FaceType for alias *myanchor")
	}
}

func TestYAMLInlineComment(t *testing.T) {
	text := "key: value # comment\n"
	spans := yamlSpans(text)
	// Comment starts at index 11.
	if !hasYAMLFaceAt(spans, 11, FaceComment) {
		t.Errorf("expected FaceComment at pos 11; got %v", spans)
	}
}

func TestYAMLDirective(t *testing.T) {
	spans := yamlSpans("%YAML 1.2\n")
	_, ok := findYAMLSpan(spans, FaceKeyword)
	if !ok {
		t.Fatal("expected keyword span for %YAML directive")
	}
}

func TestYAMLMixed(t *testing.T) {
	src := "name: Alice\nage: 30\nenabled: true\nnotes: null\n"
	spans := yamlSpans(src)
	keyCount, numCount, kwCount := 0, 0, 0
	for _, s := range spans {
		switch s.Face {
		case FaceFunction:
			keyCount++
		case FaceNumber:
			numCount++
		case FaceKeyword:
			kwCount++
		}
	}
	if keyCount < 4 {
		t.Errorf("expected >=4 key spans, got %d", keyCount)
	}
	if numCount < 1 {
		t.Errorf("expected >=1 number span, got %d", numCount)
	}
	if kwCount < 2 {
		t.Errorf("expected >=2 keyword spans (true, null), got %d", kwCount)
	}
}
