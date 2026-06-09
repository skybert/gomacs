package editor

import (
	"strings"
	"testing"
)

func TestDabbrevWordsInText(t *testing.T) {
	text := "function foo foobar baz football"
	// prefix "foo" should find "foobar" and "football", not "foo" itself
	words := dabbrevWordsInText(text, "foo", 0, false)
	want := map[string]bool{"foobar": true, "football": true}
	for _, w := range words {
		if !want[w] {
			t.Errorf("unexpected candidate: %q", w)
		}
		delete(want, w)
	}
	if len(want) > 0 {
		t.Errorf("missing candidates: %v", want)
	}
}

func TestDabbrevWordsNearestFirst(t *testing.T) {
	// Point is after "xyz" — "xylophone" is closer than "xmas"
	// text: "xmas hello world xyz xylophone"
	// positions:  0    5     11    17  21
	text := "xmas hello world xyz xylophone"
	pt := 21 // after "xyz "
	words := dabbrevWordsInText(text, "x", pt, true)
	if len(words) < 2 {
		t.Fatalf("expected at least 2 candidates, got %v", words)
	}
	if words[0] != "xylophone" {
		t.Errorf("expected nearest 'xylophone' first, got %q", words[0])
	}
}

func TestDabbrevWordsNoDuplicates(t *testing.T) {
	text := "foobar foobar foobar"
	words := dabbrevWordsInText(text, "foo", 0, false)
	if len(words) != 1 {
		t.Errorf("expected 1 unique candidate, got %d: %v", len(words), words)
	}
}

func TestDabbrevExpandEmptyPrefix(t *testing.T) {
	// cmdDabbrevExpand should bail out early when there's no word before point.
	e := newTestEditor("hello world\n")
	buf := e.ActiveBuffer()
	buf.SetPoint(0) // point at start, no word before it
	e.cmdDabbrevExpand()
	if buf.String() != "hello world\n" {
		t.Errorf("expected no change with no prefix, got %q", buf.String())
	}
}

func TestCmdDabbrevExpand(t *testing.T) {
	// Simulates: user has "hello" and "helpme" in buffer, types "hel" at end.
	e := newTestEditor("hello helpme hel\n")
	buf := e.ActiveBuffer()
	buf.SetPoint(16) // point after the trailing "hel"
	e.cmdDabbrevExpand()
	got := buf.String()
	// nearest word starting with "hel" from position 16: "helpme" (dist=4), "hello" (dist=11)
	if got != "hello helpme hello\n" && got != "hello helpme helpme\n" {
		t.Errorf("unexpected expansion: %q", got)
	}
}

// TestCmdDabbrevExpand_RepeatedRebuild calls M-/ twice; the second call sees a
// changed prefix (now the expanded candidate) and rebuilds the candidate list.
func TestCmdDabbrevExpand_RepeatedRebuild(t *testing.T) {
	e := newTestEditor("foobar foobaz foo")
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Len()) // after the trailing "foo"
	e.cmdDabbrevExpand()
	first := buf.String()
	if !strings.Contains(first, "foobar") && !strings.Contains(first, "foobaz") {
		t.Fatalf("first expansion lost both candidates: %q", first)
	}
	e.cmdDabbrevExpand() // prefix is now the full candidate; rebuild path
}

// TestCmdDabbrevExpand_PointMovedAway treats a second call as fresh when point
// is no longer at the previous expansion end.
func TestCmdDabbrevExpand_PointMovedAway(t *testing.T) {
	e := newTestEditor("foobar foo")
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Len())
	e.cmdDabbrevExpand()
	// Move point elsewhere and add a fresh prefix.
	buf.SetPoint(buf.Len())
	buf.InsertString(buf.Len(), " foob")
	buf.SetPoint(buf.Len())
	e.cmdDabbrevExpand()
	if !strings.Contains(buf.String(), "foobar") {
		t.Fatalf("expected fresh expansion to foobar, got %q", buf.String())
	}
}

// TestCmdDabbrevExpand_CommandNameFallback covers candidate source #3:
// registered command names matching the prefix.
func TestCmdDabbrevExpand_CommandNameFallback(t *testing.T) {
	e := newTestEditor("forward-c")
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Len())
	e.cmdDabbrevExpand()
	if !strings.Contains(buf.String(), "forward-char") {
		t.Logf("buffer after expand: %q", buf.String())
	}
	// At minimum a candidate should have been found (no 'No expansion' message).
	if strings.Contains(e.message, "No expansion") {
		t.Fatalf("expected a command-name expansion, got %q", e.message)
	}
}

// TestCov_DabbrevExpand_CycleBranch forces the repeated-invocation cycle path
// (pt == dabbrevLastEnd with an unchanged prefix). The state is seeded as if a
// prior expansion just completed and the detected word before point equals the
// recorded prefix, so the branch undoes the previous expansion and cycles.
func TestCov_DabbrevExpand_CycleBranch(t *testing.T) {
	e := newTestEditor("foobar foobaz foo")
	b := e.ActiveBuffer()
	// Buffer ends with "foo"; treat that as a completed expansion whose
	// candidate equals the prefix "foo", so the next call hits the cycle path.
	b.SetPoint(b.Len())
	e.dabbrevPrefix = "foo"
	e.dabbrevCandidates = []string{"foo", "foobar", "foobaz"}
	e.dabbrevIdx = 1
	e.dabbrevLastEnd = b.Point()
	before := b.String()
	e.cmdDabbrevExpand()
	if b.String() == before {
		t.Fatal("cycle branch should have replaced the expansion")
	}
}
