package editor

import (
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
