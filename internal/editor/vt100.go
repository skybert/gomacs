package editor

// vt100.go — VT100 terminal emulator screen buffer and parser.
//
// vtScreen maintains a 2-D grid of cells and processes a byte stream of VT100
// / ANSI escape sequences, updating the grid as though it were a real terminal.
// Used by the built-in shell buffer (M-x shell).
//
// Two representation choices keep a flood of output cheap.  The grid is a slice
// of row slices, so scrolling rotates row headers and clears one row instead of
// moving the whole screen.  And a cell stores an interned style id rather than
// an embedded syntax.Face, which keeps it at 8 bytes; faceAt resolves ids back
// to faces for rendering.

import (
	"unicode/utf8"

	"github.com/skybert/gomacs/internal/syntax"
)

// vtStyleID indexes a screen's style palette.  vtStyleDefault is always the
// zero face, so a zero-valued cell needs no further initialisation.
type vtStyleID uint16

const vtStyleDefault vtStyleID = 0

// vtStylePalette interns syntax.Face values so that a grid cell needs only a
// 2-byte id.  A shell session only ever uses a handful of distinct faces and
// the palette only grows, so ids stay valid for the lifetime of the screen and
// resolution is a plain slice index.
type vtStylePalette struct {
	faces []syntax.Face
	ids   map[syntax.Face]vtStyleID
}

func newVTStylePalette() *vtStylePalette {
	return &vtStylePalette{
		faces: []syntax.Face{{}},
		ids:   map[syntax.Face]vtStyleID{{}: vtStyleDefault},
	}
}

// intern returns the id for f, adding it to the palette on first sight.  This
// runs once per SGR sequence, never per character.  Pathological input with
// more distinct faces than a vtStyleID can address falls back to the default
// style rather than growing the palette further.
func (p *vtStylePalette) intern(f syntax.Face) vtStyleID {
	if id, ok := p.ids[f]; ok {
		return id
	}
	if len(p.faces) > int(^vtStyleID(0)) {
		return vtStyleDefault
	}
	id := vtStyleID(len(p.faces))
	p.faces = append(p.faces, f)
	p.ids[f] = id
	return id
}

// face resolves a style id back to its syntax.Face.
func (p *vtStylePalette) face(id vtStyleID) syntax.Face {
	if int(id) < len(p.faces) {
		return p.faces[id]
	}
	return syntax.Face{}
}

// vtCell is a single character cell on the terminal screen: a rune plus the id
// of its style in the screen's palette.
type vtCell struct {
	ch    rune
	style vtStyleID
}

// vtBlank is the cell an erase leaves behind: a space in the default style.
var vtBlank = vtCell{ch: ' '}

// vtScreen is a VT100 terminal emulator.
type vtScreen struct {
	rows, cols int
	grid       [][]vtCell // main screen, one slice per row
	altGrid    [][]vtCell // alternate screen

	useAlt bool // true when alternate screen is active

	curRow, curCol           int
	savedRow, savedCol       int
	altSavedRow, altSavedCol int
	scrollTop, scrollBot     int // scroll region, 0-based inclusive

	curFace  syntax.Face
	curStyle vtStyleID // palette id of curFace
	palette  *vtStylePalette

	// scratch holds the row headers being recycled by a scroll.
	scratch [][]vtCell

	// params is scratch for CSI parameter parsing, reused so that dispatching a
	// CSI sequence allocates nothing.
	params [vtMaxParams]int

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
		palette:   newVTStylePalette(),
	}
	s.grid = vtBlankGrid(rows, cols)
	s.altGrid = vtBlankGrid(rows, cols)
	return s
}

// vtBlankGrid allocates a rows×cols grid of blank cells.  All rows are carved
// out of one backing array so that a fresh screen is contiguous in memory.
func vtBlankGrid(rows, cols int) [][]vtCell {
	backing := make([]vtCell, rows*cols)
	for i := range backing {
		backing[i] = vtBlank
	}
	g := make([][]vtCell, rows)
	for r := range rows {
		g[r] = backing[r*cols : (r+1)*cols : (r+1)*cols]
	}
	return g
}

// vtEraseRow blanks every cell in a row.
func vtEraseRow(row []vtCell) {
	for i := range row {
		row[i] = vtBlank
	}
}

// resize changes the screen dimensions, preserving existing content.
func (s *vtScreen) resize(rows, cols int) {
	newGrid := vtBlankGrid(rows, cols)
	newAlt := vtBlankGrid(rows, cols)
	minR := min(s.rows, rows)
	minC := min(s.cols, cols)
	for r := range minR {
		copy(newGrid[r][:minC], s.grid[r][:minC])
		copy(newAlt[r][:minC], s.altGrid[r][:minC])
	}
	s.rows, s.cols = rows, cols
	s.grid, s.altGrid = newGrid, newAlt
	s.curRow = min(s.curRow, rows-1)
	s.curCol = min(s.curCol, cols-1)
	s.scrollTop = 0
	s.scrollBot = rows - 1
}

func (s *vtScreen) active() [][]vtCell {
	if s.useAlt {
		return s.altGrid
	}
	return s.grid
}

func (s *vtScreen) cellAt(row, col int) vtCell {
	if row < 0 || row >= s.rows || col < 0 || col >= s.cols {
		return vtBlank
	}
	return s.active()[row][col]
}

// faceAt returns the resolved face of the cell at (row, col), or the zero face
// when the coordinates are outside the screen.
func (s *vtScreen) faceAt(row, col int) syntax.Face {
	return s.palette.face(s.cellAt(row, col).style)
}

func (s *vtScreen) setCell(row, col int, c vtCell) {
	if row < 0 || row >= s.rows || col < 0 || col >= s.cols {
		return
	}
	s.active()[row][col] = c
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
	nums := vtParseParams(buf, s.params[:0])

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
			for r := range s.curRow {
				s.eraseLine(r)
			}
			s.eraseLeft(s.curRow, s.curCol)
		case 2, 3: // entire screen
			for r := range s.rows {
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
		row := s.active()[s.curRow]
		// Clamped so that an absurd count deletes to end of line instead of
		// running off the row.
		n := min(nAt(0, 1), s.cols-s.curCol)
		copy(row[s.curCol:s.cols-n], row[s.curCol+n:])
		vtEraseRow(row[s.cols-n:])
	case 'S': // Scroll Up
		s.scrollUp(nAt(0, 1))
	case 'T': // Scroll Down
		s.scrollDown(nAt(0, 1))
	case 'X': // Erase Characters
		n := nAt(0, 1)
		for c := s.curCol; c < s.curCol+n && c < s.cols; c++ {
			s.setCell(s.curRow, c, vtBlank)
		}
	case '@': // Insert Characters
		row := s.active()[s.curRow]
		n := min(nAt(0, 1), s.cols-s.curCol)
		copy(row[s.curCol+n:], row[s.curCol:s.cols-n])
		for c := s.curCol; c < s.curCol+n; c++ {
			row[c] = vtCell{ch: ' ', style: s.curStyle}
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
		for _, row := range s.altGrid {
			vtEraseRow(row)
		}
		s.curRow, s.curCol = 0, 0
	} else if !enable && s.useAlt {
		s.useAlt = false
	}
}

func (s *vtScreen) processSGR(params []int) {
	if len(params) == 0 {
		s.curFace = syntax.Face{}
		s.curStyle = vtStyleDefault
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
				s.curFace.Fg = vtHexColor(params[i+2], params[i+3], params[i+4])
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
				s.curFace.Bg = vtHexColor(params[i+2], params[i+3], params[i+4])
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
	s.curStyle = s.palette.intern(s.curFace)
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
	s.setCell(s.curRow, s.curCol, vtCell{ch: r, style: s.curStyle})
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
	vtEraseRow(s.active()[row])
}

func (s *vtScreen) eraseRight(row, fromCol int) {
	vtEraseRow(s.active()[row][vtClamp(fromCol, 0, s.cols):])
}

func (s *vtScreen) eraseLeft(row, toCol int) {
	vtEraseRow(s.active()[row][:vtClamp(toCol+1, 0, s.cols)])
}

// scrollUp moves the scroll region up by n lines.  Only row headers move, so
// the cost is one header rotation plus n row clears rather than a copy of the
// whole screen.
func (s *vtScreen) scrollUp(n int) {
	if n <= 0 {
		return
	}
	g := s.active()
	top, bot := s.scrollTop, s.scrollBot
	height := bot - top + 1
	if n >= height {
		for r := top; r <= bot; r++ {
			vtEraseRow(g[r])
		}
		return
	}
	// The n rows that scroll off the top are recycled as the new blank rows at
	// the bottom of the region.
	s.scratch = append(s.scratch[:0], g[top:top+n]...)
	copy(g[top:bot+1-n], g[top+n:bot+1])
	copy(g[bot+1-n:bot+1], s.scratch)
	for r := bot + 1 - n; r <= bot; r++ {
		vtEraseRow(g[r])
	}
}

// scrollDown moves the scroll region down by n lines, recycling row headers the
// same way scrollUp does.
func (s *vtScreen) scrollDown(n int) {
	if n <= 0 {
		return
	}
	g := s.active()
	top, bot := s.scrollTop, s.scrollBot
	height := bot - top + 1
	if n >= height {
		for r := top; r <= bot; r++ {
			vtEraseRow(g[r])
		}
		return
	}
	s.scratch = append(s.scratch[:0], g[bot+1-n:bot+1]...)
	copy(g[top+n:bot+1], g[top:bot+1-n])
	copy(g[top:top+n], s.scratch)
	for r := top; r < top+n; r++ {
		vtEraseRow(g[r])
	}
}

// vtMaxParams caps how many CSI parameters are parsed.  Real sequences use at
// most five (SGR truecolour: 38;2;R;G;B); the cap only bites on pathological
// input, where the surplus parameters are dropped.
const vtMaxParams = 16

// vtParamLimit is the largest parameter value accepted.  Anything bigger is
// treated as unparseable — and therefore zero — which is what strconv.Atoi
// would have reported for an overlong digit run.
const vtParamLimit = 1 << 20

// vtParseParams parses CSI parameter bytes into dst, returning the filled
// prefix.  dst is caller-owned scratch (vtScreen.params) so that a CSI dispatch
// does not allocate.  A parameter containing anything other than digits — a
// colon sub-parameter, say — reads as zero.
func vtParseParams(p []byte, dst []int) []int {
	if len(p) == 0 {
		return nil
	}
	nums := dst[:0]
	n, ok := 0, true
	for _, b := range p {
		switch {
		case b == ';':
			if len(nums) == cap(nums) {
				return nums
			}
			if !ok {
				n = 0
			}
			nums = append(nums, n)
			n, ok = 0, true
		case b >= '0' && b <= '9':
			n = n*10 + int(b-'0')
			if n > vtParamLimit {
				ok = false
			}
		default:
			ok = false
		}
	}
	if len(nums) < cap(nums) {
		if !ok {
			n = 0
		}
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
		return vtHexColor(v, v, v)
	}
	n -= 16
	r := (n / 36) * 51
	g := ((n / 6) % 6) * 51
	b := (n % 6) * 51
	return vtHexColor(r, g, b)
}

// vtHexColor formats an RGB triple as "#rrggbb".  SGR colour sequences are
// common enough in shell output that it is worth avoiding fmt here.
func vtHexColor(r, g, b int) string {
	const hexDigits = "0123456789abcdef"
	out := [7]byte{'#'}
	for i, v := range [3]int{r, g, b} {
		v = vtClamp(v, 0, 0xff)
		out[1+i*2] = hexDigits[v>>4]
		out[2+i*2] = hexDigits[v&0xf]
	}
	return string(out[:])
}

func vtClamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
