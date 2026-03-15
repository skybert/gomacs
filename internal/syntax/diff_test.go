package syntax

import "testing"

func TestDiffHighlighter(t *testing.T) {
	src := "--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,3 @@\n context\n-removed\n+added\n"
	hl := DiffHighlighter{}
	spans := hl.Highlight(src, 0, len([]rune(src)))

	type want struct {
		text string
		face Face
	}
	runes := []rune(src)
	findFace := func(text string) Face {
		for _, sp := range spans {
			if sp.Start < len(runes) && sp.End <= len(runes) {
				got := string(runes[sp.Start:sp.End])
				if got == text {
					return sp.Face
				}
			}
		}
		return FaceDefault
	}

	cases := []want{
		{"--- a/foo.go", FaceDiffFile},
		{"+++ b/foo.go", FaceDiffFile},
		{"@@ -1,3 +1,3 @@", FaceDiffHunk},
		{"-removed", FaceDiffRemoved},
		{"+added", FaceDiffAdded},
	}
	for _, tc := range cases {
		got := findFace(tc.text)
		if got != tc.face {
			t.Errorf("text %q: face = %v, want %v", tc.text, got, tc.face)
		}
	}
}
