//go:build !darwin

package screenshot

import (
	"image"
	"image/draw"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/skybert/gomacs/internal/terminal"
)

// fontDPI is the target render DPI for non-Darwin platforms.
const fontDPI = 96.0

// systemFontPaths lists candidate locations for static (non-variable) Source
// Code Pro by platform. Variable font files (containing '[') render at the
// wrong weight via the opentype package so they are listed separately as a
// last resort only here.
var systemFontPaths = map[string][]string{
	"linux": {
		"/usr/share/fonts/adobe-source-code-pro/SourceCodePro-Regular.otf",
		"/usr/share/fonts/opentype/adobe-source-code-pro/SourceCodePro-Regular.otf",
		"/usr/share/fonts/truetype/adobe-source-code-pro/SourceCodePro-Regular.ttf",
		filepath.Join(os.Getenv("HOME"), ".fonts/SourceCodePro-Regular.ttf"),
		filepath.Join(os.Getenv("HOME"), ".local/share/fonts/SourceCodePro-Regular.ttf"),
	},
	"windows": {
		`C:\Windows\Fonts\SourceCodePro-Regular.ttf`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Microsoft\Windows\Fonts\SourceCodePro-Regular.ttf`),
	},
}

// otherFont holds the platform-specific font face.
type otherFont struct {
	face font.Face
}

// LoadFont loads a suitable monospace font at the given point size.
func LoadFont(fontSize float64) (FontHandle, error) {
	paths := systemFontPaths[runtime.GOOS]
	for _, p := range paths {
		data, err := os.ReadFile(p) //nolint:gosec
		if err != nil {
			continue
		}
		ttf, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
			Size:    fontSize,
			DPI:     fontDPI,
			Hinting: font.HintingFull,
		})
		if err != nil {
			continue
		}
		charW, lineH := measureFace(face)
		log.Printf("using font: %s", p)
		return FontHandle{CharW: charW, LineH: lineH, impl: otherFont{face: face}}, nil
	}
	log.Printf("system font not found, falling back to Go Mono")
	ttf, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return FontHandle{}, err
	}
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     fontDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return FontHandle{}, err
	}
	charW, lineH := measureFace(face)
	return FontHandle{CharW: charW, LineH: lineH, impl: otherFont{face: face}}, nil
}

func measureFace(f font.Face) (charW, lineH int) {
	adv, _ := f.GlyphAdvance('M')
	charW = adv.Round()
	m := f.Metrics()
	lineH = (m.Ascent + m.Descent).Ceil()
	return charW, lineH
}

// RenderToImage renders the terminal capture buffer to an RGBA image.
func RenderToImage(t *terminal.Terminal, h FontHandle) *image.RGBA {
	of := h.impl.(otherFont)
	charW := h.CharW
	lineH := h.LineH
	cols, rows := t.CaptureSize()
	imgW := cols*charW + 2*PadX
	imgH := rows*lineH + 2*PadY

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	defBg := ResolveColor(DefaultBg, 0x22, 0x22, 0x35)
	defFg := ResolveColor(DefaultFg, 0xb8, 0xc0, 0xd4)

	draw.Draw(img, img.Bounds(), &image.Uniform{C: defBg}, image.Point{}, draw.Src)

	ascent := of.face.Metrics().Ascent.Ceil()

	for row := range rows {
		for col := range cols {
			ch, f := t.CaptureCell(col, row)
			if ch == 0 {
				ch = ' '
			}
			bg, fg := CellColors(f, defBg, defFg)

			cellX := PadX + col*charW
			cellY := PadY + row*lineH

			draw.Draw(img,
				image.Rect(cellX, cellY, cellX+charW, cellY+lineH),
				&image.Uniform{C: bg}, image.Point{}, draw.Src)

			if ch == ' ' {
				continue
			}

			d := &font.Drawer{
				Dst:  img,
				Src:  &image.Uniform{C: fg},
				Face: of.face,
				Dot:  fixed.P(cellX, cellY+ascent),
			}
			d.DrawString(string(ch))
		}
	}
	return img
}
