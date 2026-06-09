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

func TestMakefileAutoVar(t *testing.T) {
	src := "all:\n\tcp $< $@\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))
	var autoCount int
	for _, sp := range spans {
		if sp.Face == FaceMakefileAutoVar {
			autoCount++
		}
	}
	if autoCount < 2 {
		t.Errorf("expected >=2 auto-var spans ($< and $@), got %d", autoCount)
	}
}

func TestMakefileBraceVarRef(t *testing.T) {
	src := "OUT = ${BIN}/prog\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))
	var found bool
	for _, sp := range spans {
		if sp.Face == FaceMakefileVarRef && string(runes[sp.Start:sp.End]) == "${BIN}" {
			found = true
		}
	}
	if !found {
		t.Error("expected FaceMakefileVarRef for ${BIN}")
	}
}

func TestMakefileNestedVarRef(t *testing.T) {
	src := "X = $(dir $(FOO))\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))
	var found bool
	for _, sp := range spans {
		if sp.Face == FaceMakefileVarRef {
			found = true
		}
	}
	if !found {
		t.Error("expected FaceMakefileVarRef for nested $(dir $(FOO))")
	}
}

func TestMakefileBareLineNoColon(t *testing.T) {
	// A non-target, non-assignment line with a var ref still highlights the ref.
	src := "$(info building)\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))
	var found bool
	for _, sp := range spans {
		if sp.Face == FaceMakefileVarRef {
			found = true
		}
	}
	if !found {
		t.Error("expected FaceMakefileVarRef in bare line with $(info ...)")
	}
}

func TestMakefileImmediateAssignVarDef(t *testing.T) {
	src := "FOO := bar\n"
	hl := MakefileHighlighter{}
	runes := []rune(src)
	spans := hl.Highlight(src, 0, len(runes))
	var found bool
	for _, sp := range spans {
		if sp.Face == FaceMakefileVariable && string(runes[sp.Start:sp.End]) == "FOO" {
			found = true
		}
	}
	if !found {
		t.Error("expected FaceMakefileVariable for FOO in ':=' assignment")
	}
}

func TestMakefileEmpty(t *testing.T) {
	hl := MakefileHighlighter{}
	if spans := hl.Highlight("", 0, 0); len(spans) != 0 {
		t.Errorf("expected no spans for empty makefile, got %v", spans)
	}
}
