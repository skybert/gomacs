package editor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
)

// imenuEntry is one symbol/heading entry for the imenu popup.
type imenuEntry struct {
	label string // displayed in popup, e.g. "FuncName (line 42)"
	line  int    // 1-based line number
}

// imenuPatterns maps a mode to the regex used to extract symbols.
// Capture group 1 (or 2 for modes with a prefix group) is the symbol name.
var imenuPatterns = map[string]*regexp.Regexp{
	"go":       regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)`),
	"python":   regexp.MustCompile(`^(?:def|class)\s+(\w+)`),
	"java":     regexp.MustCompile(`^\s+(?:public|private|protected|static).*\s(\w+)\s*\(`),
	"bash":     regexp.MustCompile(`^(\w+)\s*\(\)`),
	"elisp":    regexp.MustCompile(`^\s*\(def(?:un|var|const|custom|macro)\s+(\S+)`),
	"markdown": regexp.MustCompile(`^(#{1,6})\s+(.+)`),
}

// imenuSymbols scans buf line by line and returns a list of named entries.
func imenuSymbols(buf *buffer.Buffer) []imenuEntry {
	mode := buf.Mode()
	lines := strings.Split(buf.String(), "\n")
	var entries []imenuEntry
	re := imenuPatterns[mode]
	if re == nil {
		return entries
	}
	for i, line := range lines {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var name string
		if mode == "markdown" {
			// m[1]=hashes, m[2]=heading text
			name = m[2]
		} else {
			name = m[1]
		}
		entries = append(entries, imenuEntry{
			label: fmt.Sprintf("%s (line %d)", name, i+1),
			line:  i + 1,
		})
	}
	return entries
}

// lineStartOffset returns the rune offset of the first character of the
// given 1-based line number in buf.
func lineStartOffset(buf *buffer.Buffer, lineN int) int {
	text := buf.String()
	runes := []rune(text)
	line := 1
	for i, r := range runes {
		if line == lineN {
			return i
		}
		if r == '\n' {
			line++
		}
	}
	return len(runes)
}

// cmdImenu opens a fuzzy-narrowing minibuffer popup to jump to a symbol.
func (e *Editor) cmdImenu() {
	e.clearArg()
	buf := e.ActiveBuffer()
	entries := imenuSymbols(buf)
	if len(entries) == 0 {
		e.Message("No imenu entries for mode %q", buf.Mode())
		return
	}
	labels := make([]string, len(entries))
	for i, en := range entries {
		labels[i] = en.label
	}
	e.ReadMinibuffer("imenu: ", func(choice string) {
		for _, en := range entries {
			if en.label == choice {
				pt := lineStartOffset(buf, en.line)
				buf.SetPoint(pt)
				e.activeWin.SetPoint(pt)
				e.activeWin.EnsurePointVisible()
				return
			}
		}
	})
	e.SetMinibufCompletions(func(q string) []string {
		var out []string
		for _, l := range labels {
			if fuzzyMatch(l, q) {
				out = append(out, l)
			}
		}
		return out
	})
}
