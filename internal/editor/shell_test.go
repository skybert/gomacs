package editor

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

var testManPages = []string{"cat", "chmod", "cp", "find", "git", "ls", "man", "mkdir", "rm", "sort"}

func TestFilterManPages_EmptyQuery(t *testing.T) {
	got := filterManPages(testManPages, "")
	if len(got) != len(testManPages) {
		t.Fatalf("empty query: want %d results, got %d", len(testManPages), len(got))
	}
}

func TestFilterManPages_PrefixMatchFirst(t *testing.T) {
	// "man" is an exact/prefix match; "chmod" contains m-a-n as subsequence.
	got := filterManPages(testManPages, "man")
	if len(got) == 0 {
		t.Fatal("expected at least one match for 'man'")
	}
	if got[0] != "man" {
		t.Errorf("first result = %q, want \"man\"", got[0])
	}
}

func TestFilterManPages_NoMatch(t *testing.T) {
	got := filterManPages(testManPages, "zzzzz")
	if len(got) != 0 {
		t.Errorf("expected no matches for 'zzzzz', got %v", got)
	}
}

func TestFilterManPages_SubstringBeforeSubsequence(t *testing.T) {
	// "or" is prefix (score 0); "forward" and "sorter" both contain "or" as
	// substring (score 1); within the same score tier they sort alphabetically.
	pages := []string{"forward", "or", "sorter"}
	got := filterManPages(pages, "or")
	if got[0] != "or" {
		t.Errorf("first = %q, want \"or\"", got[0])
	}
	if got[1] != "forward" {
		t.Errorf("second = %q, want \"forward\"", got[1])
	}
	if got[2] != "sorter" {
		t.Errorf("third = %q, want \"sorter\"", got[2])
	}
}

func TestFilterManPages_AlphaWithinSameTier(t *testing.T) {
	// "ash", "bash", "dash" all contain "ash" (prefix or substring).
	// "zsh" does NOT contain 'a' so it does not match.
	pages := []string{"zsh", "ash", "bash", "dash"}
	got := filterManPages(pages, "ash")
	want := []string{"ash", "bash", "dash"} // zsh doesn't match
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestManpathDirs_FallbackNotEmpty(t *testing.T) {
	// manpathDirs must return at least one directory (even if manpath binary
	// is absent we fall back to standard paths).
	dirs := manpathDirs()
	if len(dirs) == 0 {
		t.Fatal("manpathDirs returned no directories")
	}
}

func TestShellRun_Echo(t *testing.T) {
	out, err := shellRun(context.Background(), "echo hello", "")
	if err != nil {
		t.Fatalf("shellRun: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("want hello, got %q", out)
	}
}

func TestShellRun_WithStdin(t *testing.T) {
	out, err := shellRun(context.Background(), "tr a-z A-Z", "abc")
	if err != nil {
		t.Fatalf("shellRun: %v", err)
	}
	if strings.TrimSpace(out) != "ABC" {
		t.Fatalf("want ABC, got %q", out)
	}
}

func TestShellRun_Error(t *testing.T) {
	_, err := shellRun(context.Background(), "exit 3", "")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestManPageNames_ReturnsList(t *testing.T) {
	// Just ensure it does not crash and returns a (possibly empty) slice.
	names := manPageNames()
	_ = names
	// manCompletions should filter the same data.
	all := manCompletions("")
	if len(all) != len(names) {
		t.Fatalf("manCompletions(\"\") should equal manPageNames(): %d vs %d", len(all), len(names))
	}
}

func TestCmdShellCommand_CreatesOutputBuffer(t *testing.T) {
	e := newTestEditor("")
	e.cmdShellCommand()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdShellCommand should activate minibuffer")
	}
	e.minibufDoneFunc("echo shelltest")
	b := e.FindBuffer("*Shell Command Output*")
	if b == nil {
		t.Fatal("expected *Shell Command Output* buffer")
	}
	if !strings.Contains(b.String(), "shelltest") {
		t.Fatalf("output buffer should contain command output, got %q", b.String())
	}
}

func TestCmdShellCommand_EmptyNoop(t *testing.T) {
	e := newTestEditor("")
	e.cmdShellCommand()
	e.minibufDoneFunc("")
	if b := e.FindBuffer("*Shell Command Output*"); b != nil {
		t.Fatal("empty command should not create output buffer")
	}
}

func TestCmdShellCommandOnRegion_TransformsRegion(t *testing.T) {
	e := newTestEditor("abc def")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdShellCommandOnRegion()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdShellCommandOnRegion should activate minibuffer")
	}
	e.minibufDoneFunc("tr a-z A-Z")
	if got := b.String(); strings.TrimSpace(got) != "ABC DEF" {
		t.Fatalf("region should be upper-cased, got %q", got)
	}
}

func TestCmdShellCommandOnRegion_NoRegion(t *testing.T) {
	e := newTestEditor("abc")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(0) // mark == point → empty region
	e.cmdShellCommandOnRegion()
	e.minibufDoneFunc("tr a-z A-Z")
	if b.String() != "abc" {
		t.Fatalf("empty region should leave buffer unchanged, got %q", b.String())
	}
}

func TestCmdMan_CreatesManBuffer(t *testing.T) {
	if _, err := exec.LookPath("man"); err != nil {
		t.Skip("man not available")
	}
	e := newCapTestEditor("")
	e.cmdMan()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdMan should activate minibuffer")
	}
	e.minibufDoneFunc("ls")
	b := e.FindBuffer("*Man ls*")
	if b == nil {
		t.Skip("man ls produced no entry in this environment")
	}
	if b.Mode() != "man" {
		t.Fatalf("want mode man, got %q", b.Mode())
	}
}

func TestCmdMan_EmptyTopicNoop(t *testing.T) {
	e := newCapTestEditor("")
	e.cmdMan()
	e.minibufDoneFunc("  ")
	if b := e.FindBuffer("*Man   *"); b != nil {
		t.Fatal("blank topic should not create a man buffer")
	}
}
