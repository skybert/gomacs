// Package terminal manages the full-screen tcell display used by the editor.
//
// Terminal wraps a tcell.Screen and exposes higher-level drawing primitives
// that accept syntax.Face values for colors and text attributes.
package terminal

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
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
	return <-t.screen.EventQ()
}

// PostWakeup injects a synthetic EventInterrupt to unblock PollEvent.
// Used by background goroutines to notify the main event loop of pending work.
func (t *Terminal) PostWakeup() {
	t.screen.EventQ() <- tcell.NewEventInterrupt(nil)
}

// ShowCursor moves the hardware cursor to (col, row).
func (t *Terminal) ShowCursor(col, row int) {
	t.screen.ShowCursor(col, row)
}

// PostEventChannel returns the screen's event channel directly.
// Events may be read from the channel in the standard Go fashion, which
// allows integration with select statements.
func (t *Terminal) PostEventChannel() chan tcell.Event {
	return t.screen.EventQ()
}

// ---------------------------------------------------------------------------
// Color / style conversion
// ---------------------------------------------------------------------------

// namedColors maps lowercase CSS/X11 color names to their tcell equivalents.
var namedColors = map[string]tcell.Color{
	"default": color.Default,
	"black":   color.Black,
	"maroon":  color.Maroon,
	"green":   color.Green,
	"olive":   color.Olive,
	"navy":    color.Navy,
	"purple":  color.Purple,
	"teal":    color.Teal,
	"silver":  color.Silver,
	"gray":    color.Gray,
	"grey":    color.Gray,
	"red":     color.Red,
	"lime":    color.Lime,
	"yellow":  color.Yellow,
	"blue":    color.Blue,
	"fuchsia": color.Fuchsia,
	"magenta": color.Fuchsia,
	"aqua":    color.Aqua,
	"cyan":    color.Aqua,
	"white":   color.White,

	// Bright / high-intensity variants (ANSI 8–15).
	"darkgray":      color.DarkGray,
	"darkgrey":      color.DarkGray,
	"bright-black":  color.DarkGray,
	"brightblack":   color.DarkGray,
	"brightred":     color.OrangeRed,
	"bright-red":    color.OrangeRed,
	"brightgreen":   color.GreenYellow,
	"bright-green":  color.GreenYellow,
	"brightyellow":  color.LightGoldenrodYellow,
	"bright-yellow": color.LightGoldenrodYellow,
	"brightblue":    color.CornflowerBlue,
	"bright-blue":   color.CornflowerBlue,
	"brightcyan":    color.LightCyan,
	"bright-cyan":   color.LightCyan,
	"brightwhite":   color.GhostWhite,
	"bright-white":  color.GhostWhite,

	// Commonly used X11/Emacs face colors.
	"orange":    color.Orange,
	"pink":      color.Pink,
	"violet":    color.Violet,
	"brown":     color.SaddleBrown,
	"goldenrod": color.Goldenrod,
	"salmon":    color.Salmon,
	"turquoise": color.Turquoise,
	"tan":       color.Tan,
	"khaki":     color.Khaki,
	"coral":     color.Coral,
	"tomato":    color.Tomato,
	"sienna":    color.Sienna,
	"chocolate": color.Chocolate,
	"indigo":    color.Indigo,
	"slateblue": color.SlateBlue,
	"slategray": color.SlateGray,
	"slategrey": color.SlateGray,
	"steelblue": color.SteelBlue,
	"royalblue": color.RoyalBlue,
	"cadetblue": color.CadetBlue,
	"mintcream": color.MintCream,
	"snow":      color.Snow,
	"ivory":     color.Ivory,
	"linen":     color.Linen,
	"lavender":  color.Lavender,
	"beige":     color.Beige,
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
		return color.Default
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

	return color.Default
}

// faceToStyle converts a syntax.Face into a tcell.Style.
func faceToStyle(face syntax.Face) tcell.Style {
	style := tcell.StyleDefault.
		Foreground(parseColor(face.Fg)).
		Background(parseColor(face.Bg)).
		Bold(face.Bold).
		Italic(face.Italic).
		Reverse(face.Reverse)

	if face.Underline {
		if face.UnderlineColor != "" {
			style = style.Underline(true, parseColor(face.UnderlineColor))
		} else {
			style = style.Underline(true)
		}
	}
	return style
}
