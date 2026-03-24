package syntax

import (
	"testing"
)

func TestANSIParse_plain(t *testing.T) {
	plain, spans := ANSIParse("hello world")
	if plain != "hello world" {
		t.Errorf("plain = %q, want %q", plain, "hello world")
	}
	if len(spans) != 0 {
		t.Errorf("spans = %v, want none", spans)
	}
}

func TestANSIParse_bold(t *testing.T) {
	plain, spans := ANSIParse("\x1b[1mhello\x1b[0m")
	if plain != "hello" {
		t.Errorf("plain = %q, want %q", plain, "hello")
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if !spans[0].Face.Bold {
		t.Error("expected bold face")
	}
	if spans[0].Start != 0 || spans[0].End != 5 {
		t.Errorf("span = [%d,%d], want [0,5]", spans[0].Start, spans[0].End)
	}
}

func TestANSIParse_colors(t *testing.T) {
	// red fg
	plain, spans := ANSIParse("\x1b[31merror\x1b[0m: something")
	if plain != "error: something" {
		t.Errorf("plain = %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if spans[0].Face.Fg != "red" {
		t.Errorf("fg = %q, want red", spans[0].Face.Fg)
	}
	if spans[0].Start != 0 || spans[0].End != 5 {
		t.Errorf("span = [%d,%d], want [0,5]", spans[0].Start, spans[0].End)
	}
}

func TestANSIParse_256color(t *testing.T) {
	plain, spans := ANSIParse("\x1b[38;5;9mbright red\x1b[0m")
	if plain != "bright red" {
		t.Errorf("plain = %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	// color index 9 → bright-red
	if spans[0].Face.Fg != "bright-red" {
		t.Errorf("fg = %q, want bright-red", spans[0].Face.Fg)
	}
}

func TestANSIParse_rgb(t *testing.T) {
	plain, spans := ANSIParse("\x1b[38;2;255;0;0mred\x1b[0m")
	if plain != "red" {
		t.Errorf("plain = %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if spans[0].Face.Fg != "#ff0000" {
		t.Errorf("fg = %q, want #ff0000", spans[0].Face.Fg)
	}
}

func TestANSIParse_multipleSpans(t *testing.T) {
	// "foo" in red then "bar" in green then plain " baz"
	plain, spans := ANSIParse("\x1b[31mfoo\x1b[32mbar\x1b[0m baz")
	if plain != "foobar baz" {
		t.Errorf("plain = %q", plain)
	}
	if len(spans) != 2 {
		t.Fatalf("len(spans) = %d, want 2; spans=%v", len(spans), spans)
	}
	if spans[0].Face.Fg != "red" || spans[0].Start != 0 || spans[0].End != 3 {
		t.Errorf("span[0] = %+v", spans[0])
	}
	if spans[1].Face.Fg != "green" || spans[1].Start != 3 || spans[1].End != 6 {
		t.Errorf("span[1] = %+v", spans[1])
	}
}

func TestANSIHighlighter(t *testing.T) {
	_, spans := ANSIParse("\x1b[31merror\x1b[0m")
	hl := ANSIHighlighter{Spans: spans}
	got := hl.Highlight("error", 0, 5)
	if len(got) != len(spans) {
		t.Errorf("len(got) = %d, want %d", len(got), len(spans))
	}
}
