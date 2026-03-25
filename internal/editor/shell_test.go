package editor

import (
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
