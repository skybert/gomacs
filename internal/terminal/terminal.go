// Package terminal manages the full-screen tcell display used by the editor.
//
// Terminal wraps a tcell.Screen and exposes higher-level drawing primitives
// that accept syntax.Face values for colors and text attributes.
package terminal

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/skybert/gomacs/internal/syntax"
)

// ---------------------------------------------------------------------------
// Terminal
// ---------------------------------------------------------------------------

// Terminal wraps a tcell.Screen and provides the drawing API used by the
// editor's rendering layer.
type Terminal struct {
	screen tcell.Screen
}

// New initialises a new tcell screen.
// Mouse support is intentionally disabled (Emacs operates in keyboard mode).
func New() (*Terminal, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, fmt.Errorf("terminal.New: create screen: %w", err)
	}
	if err := s.Init(); err != nil {
		return nil, fmt.Errorf("terminal.New: init screen: %w", err)
	}
	// Disable mouse capture so that terminal text selection still works.
	s.DisableMouse()
	s.SetStyle(tcell.StyleDefault)
	return &Terminal{screen: s}, nil
}

// Close tears down the tcell screen and restores the terminal to its previous
// state.  It is safe to call Close more than once.
func (t *Terminal) Close() {
	if t.screen != nil {
		t.screen.Fini()
	}
}

// Size returns the current terminal dimensions in columns and rows.
func (t *Terminal) Size() (width, height int) {
	return t.screen.Size()
}

// Clear erases all cells on the back buffer.
func (t *Terminal) Clear() {
	t.screen.Clear()
}

// Show flushes the back buffer to the terminal.
func (t *Terminal) Show() {
	t.screen.Show()
}

// SetCell draws a single rune at (col, row) using the colors and attributes
// described by face.
func (t *Terminal) SetCell(col, row int, ch rune, face syntax.Face) {
	style := faceToStyle(face)
	t.screen.SetContent(col, row, ch, nil, style)
}

// DrawString draws the string s starting at (col, row).
// Each rune in s advances col by one cell.  Combining characters and
// wide runes are handled by tcell via SetContent.
func (t *Terminal) DrawString(col, row int, s string, face syntax.Face) {
	style := faceToStyle(face)
	x := col
	for _, r := range s {
		t.screen.SetContent(x, row, r, nil, style)
		x++
	}
}

// PollEvent blocks until a tcell event arrives and returns it.
// Callers should type-switch on (*tcell.EventKey), (*tcell.EventResize), etc.
func (t *Terminal) PollEvent() tcell.Event {
	return t.screen.PollEvent()
}

// ShowCursor moves the hardware cursor to (col, row).
func (t *Terminal) ShowCursor(col, row int) {
	t.screen.ShowCursor(col, row)
}

// PostEventChannel returns a channel that delivers tcell events.
// A background goroutine polls the screen and forwards every event;
// it exits when the channel is closed or the screen is finalised.
//
// Callers that prefer a channel-based select loop should use this instead of
// PollEvent.
func (t *Terminal) PostEventChannel() chan tcell.Event {
	ch := make(chan tcell.Event, 16)
	go func() {
		defer close(ch)
		for {
			ev := t.screen.PollEvent()
			if ev == nil {
				// screen has been Fini'd
				return
			}
			ch <- ev
		}
	}()
	return ch
}

// ---------------------------------------------------------------------------
// Color / style conversion
// ---------------------------------------------------------------------------

// namedColors maps lowercase CSS/X11 color names to their tcell equivalents.
var namedColors = map[string]tcell.Color{
	"default": tcell.ColorDefault,
	"black":   tcell.ColorBlack,
	"maroon":  tcell.ColorMaroon,
	"green":   tcell.ColorGreen,
	"olive":   tcell.ColorOlive,
	"navy":    tcell.ColorNavy,
	"purple":  tcell.ColorPurple,
	"teal":    tcell.ColorTeal,
	"silver":  tcell.ColorSilver,
	"gray":    tcell.ColorGray,
	"grey":    tcell.ColorGray,
	"red":     tcell.ColorRed,
	"lime":    tcell.ColorLime,
	"yellow":  tcell.ColorYellow,
	"blue":    tcell.ColorBlue,
	"fuchsia": tcell.ColorFuchsia,
	"magenta": tcell.ColorFuchsia,
	"aqua":    tcell.ColorAqua,
	"cyan":    tcell.ColorAqua,
	"white":   tcell.ColorWhite,

	// Bright / high-intensity variants (ANSI 8–15).
	"darkgray":      tcell.ColorDarkGray,
	"darkgrey":      tcell.ColorDarkGray,
	"bright-black":  tcell.ColorDarkGray,
	"brightblack":   tcell.ColorDarkGray,
	"brightred":     tcell.ColorOrangeRed,
	"bright-red":    tcell.ColorOrangeRed,
	"brightgreen":   tcell.ColorGreenYellow,
	"bright-green":  tcell.ColorGreenYellow,
	"brightyellow":  tcell.ColorLightGoldenrodYellow,
	"bright-yellow": tcell.ColorLightGoldenrodYellow,
	"brightblue":    tcell.ColorCornflowerBlue,
	"bright-blue":   tcell.ColorCornflowerBlue,
	"brightcyan":    tcell.ColorLightCyan,
	"bright-cyan":   tcell.ColorLightCyan,
	"brightwhite":   tcell.ColorGhostWhite,
	"bright-white":  tcell.ColorGhostWhite,

	// Commonly used X11/Emacs face colors.
	"orange":    tcell.ColorOrange,
	"pink":      tcell.ColorPink,
	"violet":    tcell.ColorViolet,
	"brown":     tcell.ColorSaddleBrown,
	"goldenrod": tcell.ColorGoldenrod,
	"salmon":    tcell.ColorSalmon,
	"turquoise": tcell.ColorTurquoise,
	"tan":       tcell.ColorTan,
	"khaki":     tcell.ColorKhaki,
	"coral":     tcell.ColorCoral,
	"tomato":    tcell.ColorTomato,
	"sienna":    tcell.ColorSienna,
	"chocolate": tcell.ColorChocolate,
	"indigo":    tcell.ColorIndigo,
	"slateblue": tcell.ColorSlateBlue,
	"slategray": tcell.ColorSlateGray,
	"slategrey": tcell.ColorSlateGray,
	"steelblue": tcell.ColorSteelBlue,
	"royalblue": tcell.ColorRoyalBlue,
	"cadetblue": tcell.ColorCadetBlue,
	"mintcream": tcell.ColorMintCream,
	"snow":      tcell.ColorSnow,
	"ivory":     tcell.ColorIvory,
	"linen":     tcell.ColorLinen,
	"lavender":  tcell.ColorLavender,
	"beige":     tcell.ColorBeige,
}

// parseColor resolves a color string to a tcell.Color.
//
// Accepted formats (in priority order):
//  1. Empty string or "default"  → tcell.ColorDefault
//  2. Named color (case-insensitive) from namedColors table
//  3. "#rrggbb" hex → tcell TrueColor
//  4. Decimal integer 0–255 → ANSI palette color
//
// Anything unrecognised falls back to tcell.ColorDefault.
func parseColor(s string) tcell.Color {
	if s == "" {
		return tcell.ColorDefault
	}
	lower := strings.ToLower(strings.TrimSpace(s))

	if c, ok := namedColors[lower]; ok {
		return c
	}

	// Hex color: "#rrggbb"
	if strings.HasPrefix(lower, "#") && len(lower) == 7 {
		r64, rerr := strconv.ParseInt(lower[1:3], 16, 32)
		g64, gerr := strconv.ParseInt(lower[3:5], 16, 32)
		b64, berr := strconv.ParseInt(lower[5:7], 16, 32)
		if rerr == nil && gerr == nil && berr == nil {
			return tcell.NewRGBColor(int32(r64), int32(g64), int32(b64))
		}
	}

	// ANSI palette index: "0"–"255"
	if n, err := strconv.Atoi(lower); err == nil && n >= 0 && n <= 255 {
		return tcell.PaletteColor(n)
	}

	return tcell.ColorDefault
}

// faceToStyle converts a syntax.Face into a tcell.Style.
//
// Bold, Italic, and Reverse use the bool-accepting Style methods introduced in
// tcell v2.  Underline is applied via the AttrMask so that we avoid any
// ambiguity with the variadic Underline(...interface{}) signature.
func faceToStyle(face syntax.Face) tcell.Style {
	style := tcell.StyleDefault.
		Foreground(parseColor(face.Fg)).
		Background(parseColor(face.Bg)).
		Bold(face.Bold).
		Italic(face.Italic).
		Reverse(face.Reverse)

	if face.Underline {
		// Apply underline via the attribute mask to avoid calling the
		// variadic Underline(params ...interface{}) method with a bool.
		_, _, attrs := style.Decompose()
		style = style.Attributes(attrs | tcell.AttrUnderline)
	}
	return style
}
