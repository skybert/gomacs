package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBuildCommand_Make(t *testing.T) {
	dir := t.TempDir()
	if got := defaultBuildCommand(dir); got != "make" {
		t.Errorf("want \"make\", got %q", got)
	}
}

func TestDefaultBuildCommand_Maven(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := defaultBuildCommand(dir); got != "mvn clean install" {
		t.Errorf("want \"mvn clean install\", got %q", got)
	}
}

func TestErrRe_GoCompilerLine(t *testing.T) {
	line := "internal/editor/editor.go:42: undefined: foo"
	m := errRe.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("errRe did not match Go compiler line %q", line)
	}
	if m[1] != "internal/editor/editor.go" {
		t.Errorf("file = %q, want %q", m[1], "internal/editor/editor.go")
	}
	if m[2] != "42" {
		t.Errorf("line = %q, want %q", m[2], "42")
	}
}

func TestErrRe_WithColumn(t *testing.T) {
	line := "src/main.go:10:5: syntax error"
	m := errRe.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("errRe did not match %q", line)
	}
	if m[2] != "10" {
		t.Errorf("line = %q, want \"10\"", m[2])
	}
	if m[3] != "5" {
		t.Errorf("col = %q, want \"5\"", m[3])
	}
}

func TestErrRe_NoMatch(t *testing.T) {
	if errRe.MatchString("this is a plain info line") {
		t.Error("errRe should not match a plain info line")
	}
}
