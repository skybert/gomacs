package editor

import (
	"testing"
)

func TestFuzzyMatch_Prefix(t *testing.T) {
	if !fuzzyMatch("forward-char", "forward") {
		t.Error("fuzzyMatch(forward-char, forward) should be true")
	}
}

func TestFuzzyMatch_Subsequence(t *testing.T) {
	if !fuzzyMatch("execute-extended-command", "exc") {
		t.Error("fuzzyMatch subsequence should be true")
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	if fuzzyMatch("forward-char", "zzz") {
		t.Error("fuzzyMatch should be false for non-subsequence")
	}
}

func TestFuzzyMatch_Empty(t *testing.T) {
	if !fuzzyMatch("anything", "") {
		t.Error("empty query should match everything")
	}
}

func TestFuzzyScore_Prefix(t *testing.T) {
	if got := fuzzyScore("man", "man"); got != 0 {
		t.Errorf("exact prefix: score = %d, want 0", got)
	}
}

func TestFuzzyScore_Substring(t *testing.T) {
	if got := fuzzyScore("command", "man"); got != 1 {
		t.Errorf("substring: score = %d, want 1", got)
	}
}

func TestFuzzyScore_PrefixBeatsSubstring(t *testing.T) {
	prefix := fuzzyScore("man", "man")
	sub := fuzzyScore("command", "man")
	if prefix >= sub {
		t.Errorf("prefix score (%d) should be < substring score (%d)", prefix, sub)
	}
}

func TestPushCommandLRU_Deduplicates(t *testing.T) {
	e := newTestEditor("")
	e.pushCommandLRU("save-buffer")
	e.pushCommandLRU("find-file")
	e.pushCommandLRU("save-buffer") // push again
	if e.commandLRU[0] != "save-buffer" {
		t.Errorf("first = %q, want \"save-buffer\"", e.commandLRU[0])
	}
	// save-buffer should appear only once.
	count := 0
	for _, n := range e.commandLRU {
		if n == "save-buffer" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("save-buffer appears %d times, want 1", count)
	}
}

func TestPushCommandLRU_Cap(t *testing.T) {
	e := newTestEditor("")
	for i := range commandLRUMax + 10 {
		e.pushCommandLRU(string(rune('a' + i%26)))
	}
	if len(e.commandLRU) > commandLRUMax {
		t.Errorf("LRU length = %d, want <= %d", len(e.commandLRU), commandLRUMax)
	}
}

func TestCommonPrefix_Empty(t *testing.T) {
	if got := commonPrefix([]string{}); got != "" {
		t.Errorf("empty input: got %q, want \"\"", got)
	}
}

func TestCommonPrefix_Single(t *testing.T) {
	if got := commonPrefix([]string{"hello"}); got != "hello" {
		t.Errorf("single: got %q, want \"hello\"", got)
	}
}

func TestCommonPrefix_Common(t *testing.T) {
	if got := commonPrefix([]string{"forward-char", "forward-word", "forward-list"}); got != "forward-" {
		t.Errorf("got %q, want \"forward-\"", got)
	}
}

func TestCommonPrefix_NoCommon(t *testing.T) {
	if got := commonPrefix([]string{"abc", "xyz"}); got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}
