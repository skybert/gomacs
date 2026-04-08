package editor

// vt100.go — VT100 terminal emulator screen buffer and parser.
//
// vtScreen maintains a 2-D grid of cells and processes a byte stream of VT100
// / ANSI escape sequences, updating the grid as though it were a real terminal.
// Used by the built-in shell buffer (M-x shell).

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/skybert/gomacs/internal/syntax"
)

// vtCell is a single character cell on the terminal screen.
type vtCell struct {
	ch   rune
	face syntax.Face
}

// vtScreen is a VT100 terminal emulator.
type vtScreen struct {
	rows, cols int
	cells      []vtCell // main screen [row*cols+col]
	altCells   []vtCell // alternate screen

	useAlt bool // true when alternate screen is active

	curRow, curCol           int
	savedRow, savedCol       int
	altSavedRow, altSavedCol int
	scrollTop, scrollBot     int // scroll region, 0-based inclusive

	curFace syntax.Face

	// parser state
	pstate int    // vtStateNormal / vtStateESC / vtStateCSI / vtStateOSC
	csiBuf []byte // accumulated CSI parameter/intermediate bytes
	oscBuf []byte // accumulated OSC data bytes
}

const (
	vtStateNormal = 0
	vtStateESC    = 1
	vtStateCSI    = 2
	vtStateOSC    = 3
)

// newVTScreen allocates and returns a blank terminal screen.
func newVTScreen(rows, cols int) *vtScreen {
	s := &vtScreen{
		rows:      rows,
		cols:      cols,
		scrollBot: rows - 1,
	}
	s.cells = vtBlankCells(rows * cols)
	s.altCells = vtBlankCells(rows * cols)
	return s
}

func vtBlankCells(n int) []vtCell {
	cells := make([]vtCell, n)
	for i := range cells {
		cells[i] = vtCell{ch: ' '}
	}
	return cells
}

// resize changes the screen dimensions, preserving existing content.
func (s *vtScreen) resize(rows, cols int) {
	newCells := vtBlankCells(rows * cols)
	newAlt := vtBlankCells(rows * cols)
	minR := min(s.rows, rows)
	minC := min(s.cols, cols)
	for r := 0; r < minR; r++ {
		for c := 0; c < minC; c++ {
			newCells[r*cols+c] = s.cells[r*s.cols+c]
			newAlt[r*cols+c] = s.altCells[r*s.cols+c]
		}
	}
	s.rows, s.cols = rows, cols
	s.cells, s.altCells = newCells, newAlt
	s.curRow = min(s.curRow, rows-1)
	s.curCol = min(s.curCol, cols-1)
	s.scrollTop = 0
	s.scrollBot = rows - 1
}

func (s *vtScreen) active() []vtCell {
	if s.useAlt {
		return s.altCells
	}
	return s.cells
}

func (s *vtScreen) cellAt(row, col int) vtCell {
	if row < 0 || row >= s.rows || col < 0 || col >= s.cols {
		return vtCell{ch: ' '}
	}
	return s.active()[row*s.cols+col]
}

func (s *vtScreen) setCell(row, col int, c vtCell) {
	if row < 0 || row >= s.rows || col < 0 || col >= s.cols {
		return
	}
	s.active()[row*s.cols+col] = c
}

// write processes terminal output, updating the screen state.
func (s *vtScreen) write(data []byte) {
	for len(data) > 0 {
		b := data[0]
		switch s.pstate {
		case vtStateNormal:
			// Handle multi-byte UTF-8 sequences transparently.
			if b >= 0x80 {
				r, sz := utf8.DecodeRune(data)
				if r != utf8.RuneError || sz > 1 {
					s.insertChar(r)
					data = data[sz:]
					continue
				}
			}
			data = data[1:]
			s.processNormal(b)
		case vtStateESC:
			data = data[1:]
			s.processESC(b)
		case vtStateCSI:
			data = data[1:]
			s.processCSI(b)
		case vtStateOSC:
			data = data[1:]
			s.processOSC(b)
		}
	}
}

func (s *vtScreen) processNormal(b byte) {
	switch b {
	case 0x07: // BEL — ignore
	case 0x08: // BS
		if s.curCol > 0 {
			s.curCol--
		}
	case 0x09: // HT (tab)
		next := ((s.curCol / 8) + 1) * 8
		s.curCol = min(next, s.cols-1)
	case 0x0a, 0x0b, 0x0c: // LF / VT / FF
		s.lineFeed()
	case 0x0d: // CR
		s.curCol = 0
	case 0x0e, 0x0f: // SO / SI — charset switch; ignore
	case 0x1b: // ESC
		s.pstate = vtStateESC
	case 0x9b: // 8-bit CSI
		s.csiBuf = s.csiBuf[:0]
		s.pstate = vtStateCSI
	default:
		if b >= 0x20 {
			s.insertChar(rune(b))
		}
	}
}

func (s *vtScreen) processESC(b byte) {
	s.pstate = vtStateNormal
	switch b {
	case '[':
		s.csiBuf = s.csiBuf[:0]
		s.pstate = vtStateCSI
	case ']':
		s.oscBuf = s.oscBuf[:0]
		s.pstate = vtStateOSC
	case 'M': // Reverse Index — scroll down if at top of scroll region
		if s.curRow == s.scrollTop {
			s.scrollDown(1)
		} else if s.curRow > 0 {
			s.curRow--
		}
	case '7', 's': // Save cursor
		s.savedRow, s.savedCol = s.curRow, s.curCol
	case '8', 'u': // Restore cursor
		s.curRow, s.curCol = s.savedRow, s.savedCol
	case 'c': // Full reset
		*s = *newVTScreen(s.rows, s.cols)
	case 'D': // Index (like LF)
		s.lineFeed()
	case 'E': // Next Line
		s.curCol = 0
		s.lineFeed()
	case '(', ')': // Charset designation — consume next byte via normal flow
	}
}

func (s *vtScreen) processOSC(b byte) {
	if b == 0x07 || b == 0x9c { // BEL or ST terminates OSC
		s.pstate = vtStateNormal
		s.oscBuf = s.oscBuf[:0]
		return
	}
	if b == 0x1b { // ESC — could be start of ESC\ (ST); close OSC
		s.pstate = vtStateNormal
		s.oscBuf = s.oscBuf[:0]
		return
	}
	s.oscBuf = append(s.oscBuf, b)
}

func (s *vtScreen) processCSI(b byte) {
	// Parameter bytes: 0x30–0x3F; intermediate bytes: 0x20–0x2F.
	if (b >= 0x30 && b <= 0x3f) || (b >= 0x20 && b <= 0x2f) {
		s.csiBuf = append(s.csiBuf, b)
		return
	}
	// Final byte (0x40–0x7E): dispatch and return to normal.
	s.pstate = vtStateNormal
	s.dispatchCSI(b)
}

func (s *vtScreen) dispatchCSI(cmd byte) {
	buf := s.csiBuf
	private := false
	if len(buf) > 0 && (buf[0] == '?' || buf[0] == '>' || buf[0] == '=') {
		private = true
		buf = buf[1:]
	}
	nums := vtParseParams(buf)

	// nAt returns nums[i] if present and > 0, else def.
	nAt := func(i, def int) int {
		if i < len(nums) && nums[i] > 0 {
			return nums[i]
		}
		return def
	}
	// n0At returns nums[i] if present, else def (allows 0 as a real value).
	n0At := func(i, def int) int {
		if i < len(nums) {
			return nums[i]
		}
		return def
	}

	switch cmd {
	case 'A': // Cursor Up
		s.curRow = max(s.scrollTop, s.curRow-nAt(0, 1))
	case 'B': // Cursor Down
		s.curRow = min(s.scrollBot, s.curRow+nAt(0, 1))
	case 'C': // Cursor Right
		s.curCol = min(s.cols-1, s.curCol+nAt(0, 1))
	case 'D': // Cursor Left
		s.curCol = max(0, s.curCol-nAt(0, 1))
	case 'E': // Cursor Next Line
		s.curRow = min(s.rows-1, s.curRow+nAt(0, 1))
		s.curCol = 0
	case 'F': // Cursor Previous Line
		s.curRow = max(0, s.curRow-nAt(0, 1))
		s.curCol = 0
	case 'G': // Cursor Horizontal Absolute
		s.curCol = vtClamp(nAt(0, 1)-1, 0, s.cols-1)
	case 'H', 'f': // Cursor Position
		s.curRow = vtClamp(nAt(0, 1)-1, 0, s.rows-1)
		s.curCol = vtClamp(nAt(1, 1)-1, 0, s.cols-1)
	case 'J': // Erase in Display
		switch n0At(0, 0) {
		case 0: // from cursor to end
			s.eraseRight(s.curRow, s.curCol)
			for r := s.curRow + 1; r < s.rows; r++ {
				s.eraseLine(r)
			}
		case 1: // from start to cursor
			for r := 0; r < s.curRow; r++ {
				s.eraseLine(r)
			}
			s.eraseLeft(s.curRow, s.curCol)
		case 2, 3: // entire screen
			for r := 0; r < s.rows; r++ {
				s.eraseLine(r)
			}
		}
	case 'K': // Erase in Line
		switch n0At(0, 0) {
		case 0:
			s.eraseRight(s.curRow, s.curCol)
		case 1:
			s.eraseLeft(s.curRow, s.curCol)
		case 2:
			s.eraseLine(s.curRow)
		}
	case 'L': // Insert Lines
		n := nAt(0, 1)
		if s.curRow >= s.scrollTop && s.curRow <= s.scrollBot {
			savedTop := s.scrollTop
			s.scrollTop = s.curRow
			s.scrollDown(n)
			s.scrollTop = savedTop
		}
	case 'M': // Delete Lines
		n := nAt(0, 1)
		if s.curRow >= s.scrollTop && s.curRow <= s.scrollBot {
			savedTop := s.scrollTop
			s.scrollTop = s.curRow
			s.scrollUp(n)
			s.scrollTop = savedTop
		}
	case 'P': // Delete Characters
		n := nAt(0, 1)
		cells := s.active()
		r := s.curRow
		for c := s.curCol; c < s.cols-n; c++ {
			cells[r*s.cols+c] = cells[r*s.cols+c+n]
		}
		for c := s.cols - n; c < s.cols; c++ {
			cells[r*s.cols+c] = vtCell{ch: ' '}
		}
	case 'S': // Scroll Up
		s.scrollUp(nAt(0, 1))
	case 'T': // Scroll Down
		s.scrollDown(nAt(0, 1))
	case 'X': // Erase Characters
		n := nAt(0, 1)
		for c := s.curCol; c < s.curCol+n && c < s.cols; c++ {
			s.setCell(s.curRow, c, vtCell{ch: ' '})
		}
	case '@': // Insert Characters
		n := nAt(0, 1)
		cells := s.active()
		r := s.curRow
		for c := s.cols - 1; c >= s.curCol+n; c-- {
			cells[r*s.cols+c] = cells[r*s.cols+c-n]
		}
		for c := s.curCol; c < s.curCol+n && c < s.cols; c++ {
			cells[r*s.cols+c] = vtCell{ch: ' ', face: s.curFace}
		}
	case 'd': // Vertical Position Absolute
		s.curRow = vtClamp(nAt(0, 1)-1, 0, s.rows-1)
	case 'm': // SGR
		s.processSGR(nums)
	case 'r': // Set Scroll Region
		top := nAt(0, 1) - 1
		bot := nAt(1, s.rows) - 1
		if top >= 0 && bot < s.rows && top < bot {
			s.scrollTop = top
			s.scrollBot = bot
			s.curRow = 0
			s.curCol = 0
		}
	case 's': // Save cursor (ANSI.SYS compatibility)
		s.savedRow, s.savedCol = s.curRow, s.curCol
	case 'u': // Restore cursor
		s.curRow, s.curCol = s.savedRow, s.savedCol
	case 'h':
		if private {
			for _, p := range nums {
				s.setPrivateMode(p, true)
			}
		}
	case 'l':
		if private {
			for _, p := range nums {
				s.setPrivateMode(p, false)
			}
		}
	}
}

func (s *vtScreen) setPrivateMode(mode int, enable bool) {
	switch mode {
	case 1047: // Alternate screen
		s.switchAlt(enable)
	case 1048: // Save/restore cursor
		if enable {
			s.altSavedRow, s.altSavedCol = s.curRow, s.curCol
		} else {
			s.curRow, s.curCol = s.altSavedRow, s.altSavedCol
		}
	case 1049: // Alternate screen + save/restore cursor
		if enable {
			s.altSavedRow, s.altSavedCol = s.curRow, s.curCol
			s.switchAlt(true)
		} else {
			s.switchAlt(false)
			s.curRow, s.curCol = s.altSavedRow, s.altSavedCol
		}
		// Other private modes (cursor visibility, mouse, wrap, etc.) — ignore.
	}
}

func (s *vtScreen) switchAlt(enable bool) {
	if enable && !s.useAlt {
		s.useAlt = true
		for i := range s.altCells {
			s.altCells[i] = vtCell{ch: ' '}
		}
		s.curRow, s.curCol = 0, 0
	} else if !enable && s.useAlt {
		s.useAlt = false
	}
}

func (s *vtScreen) processSGR(params []int) {
	if len(params) == 0 {
		s.curFace = syntax.Face{}
		return
	}
	i := 0
	for i < len(params) {
		p := params[i]
		switch {
		case p == 0:
			s.curFace = syntax.Face{}
		case p == 1:
			s.curFace.Bold = true
		case p == 3:
			s.curFace.Italic = true
		case p == 4:
			s.curFace.Underline = true
		case p == 7:
			s.curFace.Reverse = true
		case p == 22:
			s.curFace.Bold = false
		case p == 23:
			s.curFace.Italic = false
		case p == 24:
			s.curFace.Underline = false
		case p == 27:
			s.curFace.Reverse = false
		case p >= 30 && p <= 37:
			s.curFace.Fg = vtColorName(p - 30)
		case p == 38:
			if i+2 < len(params) && params[i+1] == 5 {
				s.curFace.Fg = vtAnsi256(params[i+2])
				i += 2
			} else if i+4 < len(params) && params[i+1] == 2 {
				s.curFace.Fg = fmt.Sprintf("#%02x%02x%02x", params[i+2], params[i+3], params[i+4])
				i += 4
			}
		case p == 39:
			s.curFace.Fg = ""
		case p >= 40 && p <= 47:
			s.curFace.Bg = vtColorName(p - 40)
		case p == 48:
			if i+2 < len(params) && params[i+1] == 5 {
				s.curFace.Bg = vtAnsi256(params[i+2])
				i += 2
			} else if i+4 < len(params) && params[i+1] == 2 {
				s.curFace.Bg = fmt.Sprintf("#%02x%02x%02x", params[i+2], params[i+3], params[i+4])
				i += 4
			}
		case p == 49:
			s.curFace.Bg = ""
		case p >= 90 && p <= 97:
			s.curFace.Fg = vtColorName(p - 90 + 8)
		case p >= 100 && p <= 107:
			s.curFace.Bg = vtColorName(p - 100 + 8)
		}
		i++
	}
}

func (s *vtScreen) insertChar(r rune) {
	if s.curCol >= s.cols {
		// Auto-wrap to next line.
		s.curCol = 0
		s.curRow++
		if s.curRow > s.scrollBot {
			s.curRow = s.scrollBot
			s.scrollUp(1)
		}
	}
	s.setCell(s.curRow, s.curCol, vtCell{ch: r, face: s.curFace})
	s.curCol++
}

func (s *vtScreen) lineFeed() {
	if s.curRow >= s.scrollBot {
		s.scrollUp(1)
	} else {
		s.curRow++
	}
}

func (s *vtScreen) eraseLine(row int) {
	cells := s.active()
	for c := 0; c < s.cols; c++ {
		cells[row*s.cols+c] = vtCell{ch: ' '}
	}
}

func (s *vtScreen) eraseRight(row, fromCol int) {
	cells := s.active()
	for c := fromCol; c < s.cols; c++ {
		cells[row*s.cols+c] = vtCell{ch: ' '}
	}
}

func (s *vtScreen) eraseLeft(row, toCol int) {
	cells := s.active()
	for c := 0; c <= toCol && c < s.cols; c++ {
		cells[row*s.cols+c] = vtCell{ch: ' '}
	}
}

func (s *vtScreen) scrollUp(n int) {
	if n <= 0 {
		return
	}
	cells := s.active()
	top, bot := s.scrollTop, s.scrollBot
	height := bot - top + 1
	if n >= height {
		for r := top; r <= bot; r++ {
			s.eraseLine(r)
		}
		return
	}
	for r := top; r <= bot-n; r++ {
		copy(cells[r*s.cols:r*s.cols+s.cols], cells[(r+n)*s.cols:(r+n)*s.cols+s.cols])
	}
	for r := bot - n + 1; r <= bot; r++ {
		s.eraseLine(r)
	}
}

func (s *vtScreen) scrollDown(n int) {
	if n <= 0 {
		return
	}
	cells := s.active()
	top, bot := s.scrollTop, s.scrollBot
	height := bot - top + 1
	if n >= height {
		for r := top; r <= bot; r++ {
			s.eraseLine(r)
		}
		return
	}
	for r := bot; r >= top+n; r-- {
		copy(cells[r*s.cols:r*s.cols+s.cols], cells[(r-n)*s.cols:(r-n)*s.cols+s.cols])
	}
	for r := top; r < top+n; r++ {
		s.eraseLine(r)
	}
}

// vtParseParams parses CSI parameter bytes into a slice of integers.
func vtParseParams(p []byte) []int {
	if len(p) == 0 {
		return nil
	}
	parts := bytes.Split(p, []byte(";"))
	nums := make([]int, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			nums = append(nums, 0)
			continue
		}
		n, _ := strconv.Atoi(string(part))
		nums = append(nums, n)
	}
	return nums
}

var vtColorNames = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright-black", "bright-red", "bright-green", "bright-yellow",
	"bright-blue", "bright-magenta", "bright-cyan", "bright-white",
}

func vtColorName(n int) string {
	if n >= 0 && n < len(vtColorNames) {
		return vtColorNames[n]
	}
	return ""
}

func vtAnsi256(n int) string {
	if n < 16 {
		return vtColorName(n)
	}
	if n >= 232 {
		v := (n-232)*10 + 8
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	n -= 16
	r := (n / 36) * 51
	g := ((n / 6) % 6) * 51
	b := (n % 6) * 51
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func vtClamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
