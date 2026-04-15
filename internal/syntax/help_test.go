package syntax

import (
	"strings"
	"testing"
)

func helpSpans(text string) []Span {
	return HelpHighlighter{}.Highlight(text, 0, len([]rune(text)))
}

func faceAt(spans []Span, runes []rune, word string) Face {
	idx := strings.Index(string(runes), word)
	if idx < 0 {
		return DefaultFace
	}
	for _, s := range spans {
		if s.Start <= idx && idx < s.End {
			return s.Face
		}
	}
	return DefaultFace
}

func TestHelpTitleHighlighted(t *testing.T) {
	text := "gomacs help\n============\n\nCommands\n"
	spans := helpSpans(text)
	runes := []rune(text)
	f := faceAt(spans, runes, "gomacs help")
	if f != FaceHeader1 {
		t.Errorf("title: want FaceHeader1, got %+v", f)
	}
}

func TestHelpSectionHeading(t *testing.T) {
	text := "gomacs help\n====\n\nCommands\n--------\n\n"
	spans := helpSpans(text)
	runes := []rune(text)
	f := faceAt(spans, runes, "Commands")
	if f != FaceHeader1 {
		t.Errorf("section heading: want FaceHeader1, got %+v", f)
	}
}

func TestHelpGroupHeading(t *testing.T) {
	text := "gomacs help\n====\n\nCommands\n--------\n\nNavigation\n"
	spans := helpSpans(text)
	runes := []rune(text)
	f := faceAt(spans, runes, "Navigation")
	if f != FaceFunction {
		t.Errorf("group heading: want FaceFunction, got %+v", f)
	}
}

func TestHelpCommandName(t *testing.T) {
	text := "gomacs help\n====\n\n  backward-char                (C-b)\n    Move back.\n\n"
	spans := helpSpans(text)
	runes := []rune(text)
	f := faceAt(spans, runes, "backward-char")
	if f != FaceKeyword {
		t.Errorf("command name: want FaceKeyword, got %+v", f)
	}
}

func TestHelpKeyBinding(t *testing.T) {
	text := "gomacs help\n====\n\n  backward-char                (C-b)\n    Move back.\n\n"
	spans := helpSpans(text)
	runes := []rune(text)
	f := faceAt(spans, runes, "(C-b)")
	if f != FaceString {
		t.Errorf("key binding: want FaceString, got %+v", f)
	}
}

func TestHelpSeparatorDimmed(t *testing.T) {
	text := "gomacs help\n============\n\n"
	spans := helpSpans(text)
	runes := []rune(text)
	f := faceAt(spans, runes, "============")
	if f != FaceComment {
		t.Errorf("separator: want FaceComment, got %+v", f)
	}
}
