package editor

import (
	"github.com/skybert/gomacs/internal/buffer"
)

// shOpenKeywords maps opening sh/bash control keywords to their nesting depth change.
var shOpenKeywords = map[string]bool{
	"if": true, "for": true, "while": true, "until": true, "case": true,
}

// shCloseKeywords maps closing sh/bash control keywords.
var shCloseKeywords = map[string]bool{
	"fi": true, "done": true, "esac": true,
}

// cmdForwardList moves point past the next balanced bracketed expression.
// In bash mode, also navigates over if/fi, for|while|until/done, case/esac.
// Bound to C-M-n.
func (e *Editor) cmdForwardList() {
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	n := buf.Len()
	mode := buf.Mode()
	depth := 0
	i := pt
	for i < n {
		r := buf.RuneAt(i)
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				buf.SetPoint(i + 1)
				return
			}
			depth--
			if depth == 0 {
				buf.SetPoint(i + 1)
				return
			}
		default:
			if mode == "bash" && isWordRune(r) {
				if i == 0 || !isWordRune(buf.RuneAt(i-1)) {
					word, wlen := readWordFrom(buf, i, n)
					if shOpenKeywords[word] {
						depth++
						i += wlen
						continue
					}
					if shCloseKeywords[word] {
						if depth == 0 {
							buf.SetPoint(i + wlen)
							return
						}
						depth--
						if depth == 0 {
							buf.SetPoint(i + wlen)
							return
						}
						i += wlen
						continue
					}
				}
			}
		}
		i++
	}
	e.Message("No list found")
}

// cmdBackwardList moves point backward past the previous balanced bracketed expression.
// In bash mode, also navigates over if/fi, for|while|until/done, case/esac.
// Bound to C-M-p.
func (e *Editor) cmdBackwardList() {
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	mode := buf.Mode()
	depth := 0
	i := pt - 1
	for i >= 0 {
		r := buf.RuneAt(i)
		switch r {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth == 0 {
				buf.SetPoint(i)
				return
			}
			depth--
			if depth == 0 {
				buf.SetPoint(i)
				return
			}
		default:
			if mode == "bash" && isWordRune(r) {
				if i+1 >= pt || !isWordRune(buf.RuneAt(i+1)) {
					word, wordStart := readWordEndingAt(buf, i)
					if shCloseKeywords[word] {
						depth++
						i = wordStart - 1
						continue
					}
					if shOpenKeywords[word] {
						if depth == 0 {
							buf.SetPoint(wordStart)
							return
						}
						depth--
						if depth == 0 {
							buf.SetPoint(wordStart)
							return
						}
						i = wordStart - 1
						continue
					}
				}
			}
		}
		i--
	}
	e.Message("No list found")
}

// readWordFrom reads the word starting at pos in buf (scanning forward).
// Returns the word text and its rune length.
func readWordFrom(buf *buffer.Buffer, pos, end int) (string, int) {
	j := pos
	for j < end && isWordRune(buf.RuneAt(j)) {
		j++
	}
	return buf.Substring(pos, j), j - pos
}

// readWordEndingAt reads the word ending at endPos (inclusive) in buf.
// Returns the word text and the rune index of its first character.
func readWordEndingAt(buf *buffer.Buffer, endPos int) (string, int) {
	j := endPos
	for j > 0 && isWordRune(buf.RuneAt(j-1)) {
		j--
	}
	return buf.Substring(j, endPos+1), j
}
