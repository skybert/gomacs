package syntax

import "testing"

func TestMakefileHighlighter(t *testing.T) {
	src := "# top comment\nCC = gcc\nALL_CFLAGS := -Wall\nall: main.o\n\t$(CC) -o $@ $^\nclean:\n\trm -f *.o\nifdef DEBUG\nCFLAGS += -g\nendif\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))

	findFace := func(text string) Face {
		for _, sp := range spans {
			if sp.Start < len(runes) && sp.End <= len(runes) {
				if string(runes[sp.Start:sp.End]) == text {
					return sp.Face
				}
			}
		}
		return FaceDefault
	}

	type want struct {
		text string
		face Face
	}
	cases := []want{
		{"# top comment", FaceMakefileComment},
		{"CC", FaceMakefileVariable},
		{"ALL_CFLAGS", FaceMakefileVariable},
		{"all", FaceMakefileTarget},
		{"clean", FaceMakefileTarget},
		{"ifdef", FaceMakefileDirective},
		{"endif", FaceMakefileDirective},
	}
	for _, tc := range cases {
		got := findFace(tc.text)
		if got != tc.face {
			t.Errorf("text %q: face = %+v, want %+v", tc.text, got, tc.face)
		}
	}
}

func TestMakefileRecipeLine(t *testing.T) {
	src := "target:\n\t$(CC) -o $@ $^\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))

	// The recipe line (starting with tab) should be highlighted FaceMakefileRecipe.
	recipeStart := -1
	for i, r := range runes {
		if r == '\t' {
			recipeStart = i
			break
		}
	}
	if recipeStart < 0 {
		t.Fatal("no tab found in test source")
	}

	found := false
	for _, sp := range spans {
		if sp.Start == recipeStart && sp.Face == FaceMakefileRecipe {
			found = true
			break
		}
	}
	if !found {
		t.Error("recipe line not highlighted with FaceMakefileRecipe")
	}
}

func TestMakefileVarRef(t *testing.T) {
	src := "OUT = $(BIN)/prog\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))

	findFace := func(text string) Face {
		for _, sp := range spans {
			if sp.Start < len(runes) && sp.End <= len(runes) {
				if string(runes[sp.Start:sp.End]) == text {
					return sp.Face
				}
			}
		}
		return FaceDefault
	}

	if got := findFace("$(BIN)"); got != FaceMakefileVarRef {
		t.Errorf("$(BIN): face = %+v, want FaceMakefileVarRef", got)
	}
}
