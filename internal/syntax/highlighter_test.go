package syntax

import "testing"

func TestNilHighlighter(t *testing.T) {
	var h NilHighlighter
	if spans := h.Highlight("anything at all", 0, 5); spans != nil {
		t.Errorf("NilHighlighter.Highlight returned %v, want nil", spans)
	}
	if spans := h.Highlight("", 0, 0); spans != nil {
		t.Errorf("NilHighlighter.Highlight on empty returned %v, want nil", spans)
	}
}
