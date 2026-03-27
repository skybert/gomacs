package syntax

import (
	"strings"
	"testing"
)

func TestApplySweetTheme_Colors(t *testing.T) {
	applySweetTheme()

	tests := []struct {
		face string
		ptr  *Face
		fg   string
		bg   string
	}{
		{"default", &FaceDefault, "#b8c0d4", "#222235"},
		{"keyword", &FaceKeyword, "#e17df3", ""},
		{"string", &FaceString, "#06c993", ""},
		{"comment", &FaceComment, "#808693", ""},
		{"type", &FaceType, "#06c993", ""},
		{"function", &FaceFunction, "#f6ce55", ""},
		{"number", &FaceNumber, "#f6717e", ""},
		{"operator", &FaceOperator, "#f561ab", ""},
		{"header1", &FaceHeader1, "#06c993", ""},
		{"header2", &FaceHeader2, "#e17df3", ""},
		{"header3", &FaceHeader3, "#a3a3ff", ""},
		{"modeline", &FaceModeline, "#a2a9ba", "#292235"},
		{"minibuffer", &FaceMinibuffer, "#b8c0d4", "#222235"},
		{"region", &FaceRegion, "#13131e", "#06c993"},
		{"isearch", &FaceIsearch, "#13131e", "#f6ce55"},
		{"selected", &FaceSelected, "#13131e", "#06c993"},
	}

	for _, tt := range tests {
		if tt.fg != "" && tt.ptr.Fg != tt.fg {
			t.Errorf("sweet %s: Fg = %q, want %q", tt.face, tt.ptr.Fg, tt.fg)
		}
		if tt.bg != "" && tt.ptr.Bg != tt.bg {
			t.Errorf("sweet %s: Bg = %q, want %q", tt.face, tt.ptr.Bg, tt.bg)
		}
	}
}

func TestApplySweetTheme_Attrs(t *testing.T) {
	applySweetTheme()
	if !FaceKeyword.Bold {
		t.Error("sweet keyword: expected Bold=true")
	}
	if !FaceComment.Italic {
		t.Error("sweet comment: expected Italic=true")
	}
	if !FaceHeader1.Bold {
		t.Error("sweet header1: expected Bold=true")
	}
}

func TestApplyDefaultTheme(t *testing.T) {
	applyDefaultTheme()
	if FaceDefault.Fg != "default" {
		t.Errorf("default theme FaceDefault.Fg = %q, want %q", FaceDefault.Fg, "default")
	}
	if FaceKeyword.Fg != "blue" {
		t.Errorf("default theme FaceKeyword.Fg = %q, want %q", FaceKeyword.Fg, "blue")
	}
}

func TestGetFacePtr(t *testing.T) {
	p, ok := GetFacePtr("keyword")
	if !ok {
		t.Fatal("GetFacePtr(keyword): not found")
	}
	if p != &FaceKeyword {
		t.Error("GetFacePtr(keyword): returned wrong pointer")
	}
	_, ok = GetFacePtr("nonexistent")
	if ok {
		t.Error("GetFacePtr(nonexistent): expected not found")
	}
}

func TestSetFaceByName(t *testing.T) {
	applySweetTheme()
	want := Face{Fg: "#ff0000", Bold: true}
	if !SetFaceByName("keyword", want) {
		t.Fatal("SetFaceByName returned false")
	}
	if FaceKeyword != want {
		t.Errorf("FaceKeyword = %+v, want %+v", FaceKeyword, want)
	}
	if !SetFaceByName("nonexistent", want) == false {
		t.Error("SetFaceByName(nonexistent) should return false")
	}
	applySweetTheme() // restore
}

func TestRegisterTheme(t *testing.T) {
	called := false
	RegisterTheme("test-theme", func() {
		called = true
		FaceKeyword = Face{Fg: "#aabbcc"}
	})
	if !LoadTheme("test-theme") {
		t.Fatal("LoadTheme(test-theme) returned false after RegisterTheme")
	}
	if !called {
		t.Error("registered theme function was not called")
	}
	if FaceKeyword.Fg != "#aabbcc" {
		t.Errorf("FaceKeyword.Fg = %q, want %q", FaceKeyword.Fg, "#aabbcc")
	}
	// clean up: remove the test theme and restore sweet
	delete(themes, "test-theme")
	applySweetTheme()
}

func TestFaceNames(t *testing.T) {
	names := FaceNames()
	required := []string{"keyword", "string", "comment", "modeline", "region", "isearch"}
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("FaceNames: missing %q", r)
		}
	}
	// All names should be lower-case and non-empty.
	for _, n := range names {
		if n == "" || strings.ToLower(n) != n {
			t.Errorf("FaceNames: suspicious name %q", n)
		}
	}
}
