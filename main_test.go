package main

import (
	"errors"
	"flag"
	"os"
	"testing"
	"time"
)

// fakeFileInfo is a minimal os.FileInfo stub used to exercise stdinIsPiped
// without touching a real file descriptor.
type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "stdin" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func TestParseArgs_NoArguments(t *testing.T) {
	got, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	want := parsedArgs{Quick: false, Version: false, Files: nil}
	if got.Quick != want.Quick || got.Version != want.Version || len(got.Files) != 0 {
		t.Errorf("parseArgs(nil) = %+v, want %+v", got, want)
	}
}

func TestParseArgs_SingleFile(t *testing.T) {
	got, err := parseArgs([]string{"foo.go"})
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if got.Quick || got.Version {
		t.Errorf("parseArgs(%q): unexpected flag set: %+v", "foo.go", got)
	}
	if len(got.Files) != 1 || got.Files[0] != "foo.go" {
		t.Errorf("Files = %v, want [foo.go]", got.Files)
	}
}

func TestParseArgs_MultipleFiles(t *testing.T) {
	args := []string{"a.go", "b.go", "c.go"}
	got, err := parseArgs(args)
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if len(got.Files) != len(args) {
		t.Fatalf("Files = %v, want %v", got.Files, args)
	}
	for i, f := range args {
		if got.Files[i] != f {
			t.Errorf("Files[%d] = %q, want %q", i, got.Files[i], f)
		}
	}
}

func TestParseArgs_QuickFlagBeforeFiles(t *testing.T) {
	got, err := parseArgs([]string{"-Q", "foo.go", "bar.go"})
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if !got.Quick {
		t.Error("Quick = false, want true")
	}
	if len(got.Files) != 2 || got.Files[0] != "foo.go" || got.Files[1] != "bar.go" {
		t.Errorf("Files = %v, want [foo.go bar.go]", got.Files)
	}
}

// TestParseArgs_QuickFlagAfterFiles pins the standard library's flag-parsing
// behaviour: parsing stops at the first non-flag argument, so a "-Q" that
// appears after a positional argument is itself treated as a positional
// argument rather than as the -Q flag.
func TestParseArgs_QuickFlagAfterFiles(t *testing.T) {
	got, err := parseArgs([]string{"foo.go", "-Q"})
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if got.Quick {
		t.Error("Quick = true, want false (flag parsing should have already stopped)")
	}
	if len(got.Files) != 2 || got.Files[0] != "foo.go" || got.Files[1] != "-Q" {
		t.Errorf("Files = %v, want [foo.go -Q]", got.Files)
	}
}

func TestParseArgs_VersionFlag(t *testing.T) {
	got, err := parseArgs([]string{"-version"})
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if !got.Version {
		t.Error("Version = false, want true")
	}
	if len(got.Files) != 0 {
		t.Errorf("Files = %v, want none", got.Files)
	}
}

func TestParseArgs_QuickAndVersionCombined(t *testing.T) {
	got, err := parseArgs([]string{"-Q", "-version"})
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if !got.Quick || !got.Version {
		t.Errorf("parseArgs(-Q -version) = %+v, want both true", got)
	}
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	_, err := parseArgs([]string{"-bogus"})
	if err == nil {
		t.Fatal("parseArgs with an unknown flag: expected error, got nil")
	}
	if errors.Is(err, flag.ErrHelp) {
		t.Errorf("parseArgs with an unknown flag: got flag.ErrHelp, want a plain parse error")
	}
}

func TestParseArgs_Help(t *testing.T) {
	_, err := parseArgs([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Errorf("parseArgs([-h]) error = %v, want flag.ErrHelp", err)
	}
}

// TestParseArgs_DoubleDashTerminator pins the standard library's "--"
// handling: everything after "--" is positional, even strings that would
// otherwise look like flags.
func TestParseArgs_DoubleDashTerminator(t *testing.T) {
	got, err := parseArgs([]string{"--", "-Q", "file.go"})
	if err != nil {
		t.Fatalf("parseArgs: unexpected error: %v", err)
	}
	if got.Quick {
		t.Error("Quick = true, want false (-Q after -- is positional)")
	}
	if len(got.Files) != 2 || got.Files[0] != "-Q" || got.Files[1] != "file.go" {
		t.Errorf("Files = %v, want [-Q file.go]", got.Files)
	}
}

func TestParseArgs_IsIndependentAcrossCalls(t *testing.T) {
	// Each call must get its own FlagSet; state from one call must not
	// leak into the next (this was a risk when moving off the
	// package-level flag.CommandLine).
	if _, err := parseArgs([]string{"-Q"}); err != nil {
		t.Fatalf("first parseArgs: unexpected error: %v", err)
	}
	got, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("second parseArgs: unexpected error: %v", err)
	}
	if got.Quick {
		t.Error("Quick = true on second call, want false: flag state leaked across calls")
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"dev", "gomacs dev"},
		{"1.2.3", "gomacs 1.2.3"},
		{"", "gomacs "},
	}
	for _, tt := range tests {
		if got := versionString(tt.version); got != tt.want {
			t.Errorf("versionString(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestStdinIsPiped(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"char device (tty attached)", os.ModeCharDevice, false},
		{"regular file (redirected)", 0, true},
		{"named pipe", os.ModeNamedPipe, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stdinIsPiped(fakeFileInfo{mode: tt.mode}); got != tt.want {
				t.Errorf("stdinIsPiped(mode=%v) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}
