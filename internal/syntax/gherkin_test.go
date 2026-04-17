package syntax

import "testing"

func TestGherkinHighlighter_Comment(t *testing.T) {
	h := GherkinHighlighter{}
	text := "# this is a comment\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 || spans[0].Face != FaceComment {
		t.Fatalf("expected 1 FaceComment span, got %v", spans)
	}
}

func TestGherkinHighlighter_FeatureKeyword(t *testing.T) {
	h := GherkinHighlighter{}
	text := "Feature: login\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kw *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kw = &spans[i]
			break
		}
	}
	if kw == nil {
		t.Error("expected FaceKeyword for 'Feature:'")
	}
}

func TestGherkinHighlighter_ScenarioKeyword(t *testing.T) {
	h := GherkinHighlighter{}
	text := "  Scenario: user logs in\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kw *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kw = &spans[i]
			break
		}
	}
	if kw == nil {
		t.Error("expected FaceKeyword for 'Scenario:'")
	}
}

func TestGherkinHighlighter_ScenarioOutline(t *testing.T) {
	h := GherkinHighlighter{}
	text := "  Scenario Outline: user logs in as <role>\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kw *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kw = &spans[i]
			break
		}
	}
	if kw == nil {
		t.Fatal("expected FaceKeyword for 'Scenario Outline:'")
	}
	// Keyword should cover "Scenario Outline:" not just "Scenario:".
	kwText := string([]rune(text)[kw.Start:kw.End])
	if kwText != "Scenario Outline:" {
		t.Errorf("keyword span = %q, want \"Scenario Outline:\"", kwText)
	}
}

func TestGherkinHighlighter_GivenStep(t *testing.T) {
	h := GherkinHighlighter{}
	text := "    Given user logs in\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kw *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kw = &spans[i]
			break
		}
	}
	if kw == nil {
		t.Error("expected FaceKeyword for 'Given '")
	}
}

func TestGherkinHighlighter_Tag(t *testing.T) {
	h := GherkinHighlighter{}
	text := "@smoke @login\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	count := 0
	for _, sp := range spans {
		if sp.Face == FaceType {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 FaceType spans for tags, got %d: %v", count, spans)
	}
}

func TestGherkinHighlighter_Parameter(t *testing.T) {
	h := GherkinHighlighter{}
	text := "    Given the user is <role>\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var param *Span
	for i := range spans {
		if spans[i].Face == FaceType {
			param = &spans[i]
		}
	}
	if param == nil {
		t.Error("expected FaceType for <role> parameter")
	}
}

func TestGherkinHighlighter_InlineString(t *testing.T) {
	h := GherkinHighlighter{}
	text := `    When I enter "admin" as the username` + "\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var strSpan *Span
	for i := range spans {
		if spans[i].Face == FaceString {
			strSpan = &spans[i]
		}
	}
	if strSpan == nil {
		t.Error("expected FaceString for inline string")
	}
}

func TestGherkinHighlighter_TableRow(t *testing.T) {
	h := GherkinHighlighter{}
	text := "    | username | password |\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	count := 0
	for _, sp := range spans {
		if sp.Face == FaceFunction {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 FaceFunction spans for | separators, got %d", count)
	}
}

func TestGherkinHighlighter_Docstring(t *testing.T) {
	h := GherkinHighlighter{}
	text := "    \"\"\"\n    some content\n    \"\"\"\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	count := 0
	for _, sp := range spans {
		if sp.Face == FaceString {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 FaceString spans for docstring, got %d", count)
	}
}

func TestGherkinHighlighter_BackgroundKeyword(t *testing.T) {
	h := GherkinHighlighter{}
	text := "Background:\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kw *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kw = &spans[i]
			break
		}
	}
	if kw == nil {
		t.Error("expected FaceKeyword for 'Background:'")
	}
}
