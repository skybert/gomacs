package syntax

import (
	"strconv"
	"strings"
)

// ANSIParse strips ANSI SGR escape sequences from raw, returning the plain text
// and a slice of Spans with Face values derived from the color codes.
// Supported codes: 0 (reset), 1 (bold), 3 (italic), 22 (bold off), 23 (italic off),
// 30-37/90-97 (fg), 40-47/100-107 (bg), 39/49 (default fg/bg), 38;5;N (256-color fg),
// 48;5;N (256-color bg), 38;2;R;G;B (RGB fg), 48;2;R;G;B (RGB bg).
func ANSIParse(raw string) (string, []Span) {
	var sb strings.Builder
	var spans []Span

	runes := []rune(raw)
	n := len(runes)

	// current style state
	var fg, bg string
	bold, italic := false, false

	spanStart := 0
	plainPos := 0

	flushSpan := func() {
		if plainPos > spanStart && (fg != "" || bg != "" || bold || italic) {
			spans = append(spans, Span{
				Start: spanStart,
				End:   plainPos,
				Face:  Face{Fg: fg, Bg: bg, Bold: bold, Italic: italic},
			})
		}
	}

	i := 0
	for i < n {
		if runes[i] != '\x1b' || i+1 >= n || runes[i+1] != '[' {
			sb.WriteRune(runes[i])
			plainPos++
			i++
			continue
		}
		// Parse CSI escape: \x1b [ <params> m
		j := i + 2
		for j < n && runes[j] != 'm' {
			j++
		}
		if j >= n {
			// Malformed escape, emit raw
			sb.WriteRune(runes[i])
			plainPos++
			i++
			continue
		}
		// Flush current span before style change
		flushSpan()
		spanStart = plainPos

		// Parse semicolon-separated param list
		paramStr := string(runes[i+2 : j])
		params := parseSGRParams(paramStr)
		k := 0
		for k < len(params) {
			p := params[k]
			switch {
			case p == 0:
				fg, bg = "", ""
				bold, italic = false, false
			case p == 1:
				bold = true
			case p == 22:
				bold = false
			case p == 3:
				italic = true
			case p == 23:
				italic = false
			case p >= 30 && p <= 37:
				fg = ansiColorName(p - 30)
			case p == 39:
				fg = ""
			case p >= 40 && p <= 47:
				bg = ansiColorName(p - 40)
			case p == 49:
				bg = ""
			case p >= 90 && p <= 97:
				fg = ansiBrightColorName(p - 90)
			case p >= 100 && p <= 107:
				bg = ansiBrightColorName(p - 100)
			case p == 38 && k+2 < len(params) && params[k+1] == 5:
				fg = ansi256Color(params[k+2])
				k += 2
			case p == 48 && k+2 < len(params) && params[k+1] == 5:
				bg = ansi256Color(params[k+2])
				k += 2
			case p == 38 && k+4 < len(params) && params[k+1] == 2:
				fg = ansiRGBColor(params[k+2], params[k+3], params[k+4])
				k += 4
			case p == 48 && k+4 < len(params) && params[k+1] == 2:
				bg = ansiRGBColor(params[k+2], params[k+3], params[k+4])
				k += 4
			}
			k++
		}
		i = j + 1 // skip past 'm'
	}
	flushSpan()
	return sb.String(), spans
}

func parseSGRParams(s string) []int {
	if s == "" {
		return []int{0}
	}
	parts := strings.Split(s, ";")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}

// ansiColorName maps ANSI color index 0-7 to color name.
func ansiColorName(n int) string {
	names := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	if n >= 0 && n < len(names) {
		return names[n]
	}
	return "default"
}

// ansiBrightColorName maps ANSI bright color index 0-7 to color name.
func ansiBrightColorName(n int) string {
	names := []string{"bright-black", "bright-red", "bright-green", "bright-yellow",
		"bright-blue", "bright-magenta", "bright-cyan", "bright-white"}
	if n >= 0 && n < len(names) {
		return names[n]
	}
	return "default"
}

// ansi256Color maps a 256-color palette index to a hex color string.
func ansi256Color(n int) string {
	if n < 0 || n > 255 {
		return "default"
	}
	// First 16: standard colors
	if n < 8 {
		return ansiColorName(n)
	}
	if n < 16 {
		return ansiBrightColorName(n - 8)
	}
	// 16-231: 6x6x6 color cube
	if n < 232 {
		n -= 16
		r := (n / 36) * 51
		g := ((n / 6) % 6) * 51
		b := (n % 6) * 51
		return ansiRGBColor(r, g, b)
	}
	// 232-255: grayscale
	gray := 8 + (n-232)*10
	return ansiRGBColor(gray, gray, gray)
}

func ansiRGBColor(r, g, b int) string {
	return "#" + hexByte(r) + hexByte(g) + hexByte(b)
}

func hexByte(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 255 {
		n = 255
	}
	s := strconv.FormatInt(int64(n), 16)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

// ANSIHighlighter implements Highlighter using pre-computed ANSI-derived spans.
// It ignores the text argument and returns the spans stored at construction time.
type ANSIHighlighter struct {
	Spans []Span
}

func (h ANSIHighlighter) Highlight(_ string, _, _ int) []Span {
	return h.Spans
}
