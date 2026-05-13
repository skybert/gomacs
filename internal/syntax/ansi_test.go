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

// ---------------------------------------------------------------------------
// ansiColorName
// ---------------------------------------------------------------------------

func TestAnsiColorName(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "black"},
		{1, "red"},
		{2, "green"},
		{3, "yellow"},
		{4, "blue"},
		{5, "magenta"},
		{6, "cyan"},
		{7, "white"},
		// out-of-range
		{-1, "default"},
		{8, "default"},
		{100, "default"},
	}
	for _, tc := range cases {
		if got := ansiColorName(tc.n); got != tc.want {
			t.Errorf("ansiColorName(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ansiBrightColorName
// ---------------------------------------------------------------------------

func TestAnsiBrightColorName(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "bright-black"},
		{1, "bright-red"},
		{2, "bright-green"},
		{3, "bright-yellow"},
		{4, "bright-blue"},
		{5, "bright-magenta"},
		{6, "bright-cyan"},
		{7, "bright-white"},
		// out-of-range
		{-1, "default"},
		{8, "default"},
		{100, "default"},
	}
	for _, tc := range cases {
		if got := ansiBrightColorName(tc.n); got != tc.want {
			t.Errorf("ansiBrightColorName(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// hexByte
// ---------------------------------------------------------------------------

func TestHexByte(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "00"},
		{15, "0f"},
		{16, "10"},
		{255, "ff"},
		// clamp low
		{-1, "00"},
		{-100, "00"},
		// clamp high
		{256, "ff"},
		{1000, "ff"},
	}
	for _, tc := range cases {
		if got := hexByte(tc.n); got != tc.want {
			t.Errorf("hexByte(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ansi256Color
// ---------------------------------------------------------------------------

func TestAnsi256Color(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		// out-of-range
		{-1, "default"},
		{256, "default"},
		// n 0-7: standard color names
		{0, "black"},
		{7, "white"},
		// n 8-15: bright color names
		{8, "bright-black"},
		{15, "bright-white"},
		// n 16: first color cube entry (0,0,0) = black
		{16, "#000000"},
		// n 17: (0,0,51) — first blue step
		{17, "#000033"},
		// n 21: (0,0,255) — max blue in first row
		{21, "#0000ff"},
		// n 231: last color cube entry (255,255,255)
		{231, "#ffffff"},
		// n 232: first grayscale (gray=8)
		{232, "#080808"},
		// n 255: last grayscale (gray=8+23*10=238)
		{255, "#eeeeee"},
		// mid grayscale: n=244 → gray=8+12*10=128
		{244, "#808080"},
	}
	for _, tc := range cases {
		if got := ansi256Color(tc.n); got != tc.want {
			t.Errorf("ansi256Color(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Additional ANSIParse tests
// ---------------------------------------------------------------------------

func TestANSIParse_empty(t *testing.T) {
	plain, spans := ANSIParse("")
	if plain != "" {
		t.Errorf("plain = %q, want empty string", plain)
	}
	if len(spans) != 0 {
		t.Errorf("spans = %v, want none", spans)
	}
}

func TestANSIParse_backgroundColor(t *testing.T) {
	// red background (41)
	plain, spans := ANSIParse("\x1b[41mfoo\x1b[49m")
	if plain != "foo" {
		t.Errorf("plain = %q, want %q", plain, "foo")
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if spans[0].Face.Bg != "red" {
		t.Errorf("bg = %q, want red", spans[0].Face.Bg)
	}
	// After \x1b[49m the background is reset; the span covers only "foo"
	if spans[0].Start != 0 || spans[0].End != 3 {
		t.Errorf("span = [%d,%d], want [0,3]", spans[0].Start, spans[0].End)
	}
}

func TestANSIParse_resetFg(t *testing.T) {
	// set red fg, then reset with 39, then plain text
	plain, spans := ANSIParse("\x1b[31mhi\x1b[39m there")
	if plain != "hi there" {
		t.Errorf("plain = %q", plain)
	}
	// Only "hi" should carry a span; " there" is unstyled
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if spans[0].Face.Fg != "red" {
		t.Errorf("fg = %q, want red", spans[0].Face.Fg)
	}
	if spans[0].Start != 0 || spans[0].End != 2 {
		t.Errorf("span = [%d,%d], want [0,2]", spans[0].Start, spans[0].End)
	}
}

func TestANSIParse_brightForeground(t *testing.T) {
	// \x1b[91m through \x1b[97m are bright foreground colors (90-97 range)
	cases := []struct {
		code int
		want string
	}{
		{91, "bright-red"},
		{92, "bright-green"},
		{93, "bright-yellow"},
		{94, "bright-blue"},
		{95, "bright-magenta"},
		{96, "bright-cyan"},
		{97, "bright-white"},
	}
	for _, tc := range cases {
		raw := "\x1b[" + string(rune('0'+tc.code/10)) + string(rune('0'+tc.code%10)) + "mX\x1b[0m"
		_, spans := ANSIParse(raw)
		if len(spans) != 1 {
			t.Fatalf("code %d: len(spans) = %d, want 1", tc.code, len(spans))
		}
		if spans[0].Face.Fg != tc.want {
			t.Errorf("code %d: fg = %q, want %q", tc.code, spans[0].Face.Fg, tc.want)
		}
	}
}

func TestANSIParse_256colorBackground(t *testing.T) {
	// \x1b[48;5;16m sets background to first color-cube entry (black = #000000)
	plain, spans := ANSIParse("\x1b[48;5;16mfoo\x1b[0m")
	if plain != "foo" {
		t.Errorf("plain = %q, want foo", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if spans[0].Face.Bg != "#000000" {
		t.Errorf("bg = %q, want #000000", spans[0].Face.Bg)
	}
}

func TestANSIParse_rgbBackground(t *testing.T) {
	// \x1b[48;2;0;255;0m sets background to pure green
	plain, spans := ANSIParse("\x1b[48;2;0;255;0mgreen\x1b[0m")
	if plain != "green" {
		t.Errorf("plain = %q, want green", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if spans[0].Face.Bg != "#00ff00" {
		t.Errorf("bg = %q, want #00ff00", spans[0].Face.Bg)
	}
}

func TestANSIParse_italic(t *testing.T) {
	plain, spans := ANSIParse("\x1b[3mslant\x1b[0m")
	if plain != "slant" {
		t.Errorf("plain = %q, want slant", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1", len(spans))
	}
	if !spans[0].Face.Italic {
		t.Error("expected italic face")
	}
	if spans[0].Face.Bold {
		t.Error("expected non-bold face")
	}
}
