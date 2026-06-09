package lsp

import "testing"

func TestFileURI(t *testing.T) {
	tests := []struct {
		path string
		want URI
	}{
		{"/tmp/foo.go", "file:///tmp/foo.go"},
		{"/a/b/c", "file:///a/b/c"},
		{"", "file://"},
	}
	for _, tt := range tests {
		if got := FileURI(tt.path); got != tt.want {
			t.Errorf("FileURI(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestPathFromURI(t *testing.T) {
	tests := []struct {
		uri  URI
		want string
	}{
		{"file:///tmp/foo.go", "/tmp/foo.go"},
		{"file://", ""},
		{"not-a-uri", "not-a-uri"},
		{"", ""},
		{"file:/x", "file:/x"}, // missing slashes; not file://
	}
	for _, tt := range tests {
		if got := PathFromURI(tt.uri); got != tt.want {
			t.Errorf("PathFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestSeverityLabel(t *testing.T) {
	tests := []struct {
		sev  int
		want string
	}{
		{SeverityError, "E"},
		{SeverityWarning, "W"},
		{SeverityInfo, "I"},
		{SeverityHint, "4"},
		{99, "99"},
		{0, "0"},
	}
	for _, tt := range tests {
		if got := SeverityLabel(tt.sev); got != tt.want {
			t.Errorf("SeverityLabel(%d) = %q, want %q", tt.sev, got, tt.want)
		}
	}
}

func TestUTF16OffsetASCII(t *testing.T) {
	if got := UTF16Offset("hello", 3); got != 3 {
		t.Errorf("UTF16Offset(hello, 3) = %d, want 3", got)
	}
	if got := UTF16Offset("", 0); got != 0 {
		t.Errorf("UTF16Offset(empty) = %d, want 0", got)
	}
	if got := UTF16Offset("abc", 0); got != 0 {
		t.Errorf("UTF16Offset(abc, 0) = %d, want 0", got)
	}
}

func TestUTF16OffsetMultibyte(t *testing.T) {
	// "ñ" is U+00F1 (1 UTF-16 code unit)
	s := "añb"
	if got := UTF16Offset(s, 2); got != 2 {
		t.Errorf("UTF16Offset(añb, 2) = %d, want 2", got)
	}

	// "𝄞" is U+1D11E (2 UTF-16 code units, surrogate pair)
	s = "a𝄞b"
	if got := UTF16Offset(s, 2); got != 3 {
		t.Errorf("UTF16Offset(a𝄞b, 2) = %d, want 3 (surrogate pair counts as 2)", got)
	}
	if got := UTF16Offset(s, 3); got != 4 {
		t.Errorf("UTF16Offset(a𝄞b, 3) = %d, want 4", got)
	}
}

func TestUTF16OffsetClampedToLineLen(t *testing.T) {
	// Asking past end should not crash; loop simply terminates.
	if got := UTF16Offset("ab", 100); got != 2 {
		t.Errorf("UTF16Offset past end = %d, want 2", got)
	}
}
