//go:build darwin

package screenshot

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreText -framework Foundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreText/CoreText.h>
#include <stdlib.h>

// getDisplayScale returns the highest pixel-to-point ratio among all active
// displays.  Returns 2.0 on a system that has any Retina (HiDPI) display and
// 1.0 on a system with only standard-density displays.
static double getDisplayScale(void) {
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	if (count == 0) return 1.0;
	CGDirectDisplayID *displays = (CGDirectDisplayID *)malloc(count * sizeof(CGDirectDisplayID));
	if (!displays) return 1.0;
	CGGetActiveDisplayList(count, displays, &count);
	double maxScale = 1.0;
	for (uint32_t i = 0; i < count; i++) {
		CGDisplayModeRef mode = CGDisplayCopyDisplayMode(displays[i]);
		if (!mode) continue;
		size_t pxW = CGDisplayModeGetPixelWidth(mode);
		size_t ptW = CGDisplayModeGetWidth(mode);
		CGDisplayModeRelease(mode);
		if (ptW > 0) {
			double scale = (double)pxW / (double)ptW;
			if (scale > maxScale) maxScale = scale;
		}
	}
	free(displays);
	return maxScale;
}

// Creates a Y-up bitmap context using kCGBitmapByteOrder32Big so that pixel
// bytes are always stored R,G,B,A regardless of host endianness.
static CGContextRef makeContext(int w, int h) {
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(
		NULL, w, h, 8, w * 4, cs,
		kCGBitmapByteOrder32Big | kCGImageAlphaPremultipliedLast);
	CGColorSpaceRelease(cs);
	CGContextSetShouldAntialias(ctx, true);
	CGContextSetShouldSmoothFonts(ctx, true);
	CGContextSetAllowsFontSmoothing(ctx, true);
	return ctx;
}

static CTFontRef makeFont(const char *name, double size) {
	CFStringRef n = CFStringCreateWithCString(NULL, name, kCFStringEncodingUTF8);
	CTFontRef f = CTFontCreateWithName(n, size, NULL);
	CFRelease(n);
	return f;
}

static double fontAdvance(CTFontRef f) {
	UniChar c = 'M';
	CGGlyph g = 0;
	CTFontGetGlyphsForCharacters(f, &c, &g, 1);
	return CTFontGetAdvancesForGlyphs(f, kCTFontOrientationHorizontal, &g, NULL, 1);
}

static double fontAscent(CTFontRef f)  { return CTFontGetAscent(f); }
static double fontDescent(CTFontRef f) { return CTFontGetDescent(f); }
static double fontLeading(CTFontRef f) { return CTFontGetLeading(f); }

// Fill a rect. (x,y) = bottom-left corner in Y-up coordinates.
static void fillRect(CGContextRef ctx,
		double x, double y, double w, double h,
		double r, double g, double b) {
	CGContextSetRGBFillColor(ctx, r, g, b, 1.0);
	CGContextFillRect(ctx, CGRectMake(x, y, w, h));
}

// Draw a glyph.  pos = (cellX, baseline) in Y-up coordinates.
static void drawGlyph(CGContextRef ctx, CTFontRef f,
		uint32_t cp, double x, double y,
		double r, double g, double b) {
	UniChar uc[2];
	CGGlyph gl[2] = {0, 0};
	int n;
	if (cp >= 0x10000) {
		cp -= 0x10000;
		uc[0] = (UniChar)(0xD800 + (cp >> 10));
		uc[1] = (UniChar)(0xDC00 + (cp & 0x3FF));
		n = 2;
	} else {
		uc[0] = (UniChar)cp;
		n = 1;
	}
	CTFontGetGlyphsForCharacters(f, uc, gl, n);
	if (gl[0] == 0) return;
	CGPoint pos = CGPointMake(x, y);
	CGContextSetRGBFillColor(ctx, r, g, b, 1.0);
	CTFontDrawGlyphs(f, gl, &pos, 1, ctx);
}

// Probe: return the R byte at pixel (px, py) in device pixels (Y-up coords).
static uint8_t probeR(CGContextRef ctx, int px, int py) {
	int stride = (int)CGBitmapContextGetBytesPerRow(ctx);
	int h = (int)CGBitmapContextGetHeight(ctx);
	uint8_t *data = (uint8_t *)CGBitmapContextGetData(ctx);
	// In CGBitmapContext, buffer row 0 = Y-up y=0 = bottom of image.
	// Row for Y-up py = h - 1 - py  ... OR = py? We will determine this.
	// We write two distinct probe pixels and check which buffer row they land in.
	return data[py * stride + px * 4 + 0]; // assume no flip first
}

static void* contextData(CGContextRef ctx)   { return CGBitmapContextGetData(ctx); }
static int   contextStride(CGContextRef ctx) { return (int)CGBitmapContextGetBytesPerRow(ctx); }
static int   contextHeight(CGContextRef ctx) { return (int)CGBitmapContextGetHeight(ctx); }
*/
import "C"

import (
	"image"
	"log"
	"unsafe"

	"github.com/skybert/gomacs/internal/terminal"
)

// darwinFont holds the platform-specific CoreText font reference and metrics.
type darwinFont struct {
	f       C.CTFontRef
	ascent  float64
	descent float64
}

// LoadFont loads a suitable monospace font at the given point size.
func LoadFont(fontSize float64) (FontHandle, error) {
	// Scale the font by the display's pixel ratio so that 1 image pixel == 1
	// physical screen pixel.  On a 2x Retina display this doubles the font
	// size, giving lineH ≈ 34 px instead of 17 px, which matches the physical
	// cell height of a terminal running the same font at the same point size.
	scale := float64(C.getDisplayScale())
	if scale < 1 {
		scale = 1
	}
	scaledSize := fontSize * scale
	log.Printf("display scale=%.1f, scaledFontSize=%.2f", scale, scaledSize)

	names := []string{
		"Source Code Pro",
		"SourceCodePro-Regular",
		"Menlo",
		"Monaco",
	}
	for _, name := range names {
		f := C.makeFont(C.CString(name), C.double(scaledSize))
		if f == 0 {
			continue
		}
		adv := float64(C.fontAdvance(f))
		if adv < 1 {
			C.CFRelease(C.CFTypeRef(unsafe.Pointer(f))) //nolint:govet
			continue
		}
		ascent := float64(C.fontAscent(f))
		descent := float64(C.fontDescent(f))
		leading := float64(C.fontLeading(f))
		charW := int(adv + 0.5)
		lineH := int(ascent+descent+leading+0.5) + 1
		log.Printf("using font: %s  ascent=%.2f descent=%.2f leading=%.2f charW=%d lineH=%d",
			name, ascent, descent, leading, charW, lineH)
		return FontHandle{
			CharW: charW,
			LineH: lineH,
			impl:  darwinFont{f: f, ascent: ascent, descent: descent},
		}, nil
	}
	return FontHandle{}, nil
}

// probeCGLayout fills y=1 (near bottom, Y-up) with red and y=H-2 (near top,
// Y-up) with green, then checks which buffer row each color lands in.
// Returns true if row-0-in-buffer == Y-up-y=0 (i.e. flip IS needed).
func probeCGLayout(w, h int) bool {
	ctx := C.makeContext(C.int(w), C.int(h))
	defer C.CFRelease(C.CFTypeRef(unsafe.Pointer(ctx))) //nolint:govet

	// Fill y=1 (bottom area, Y-up) with red (R=255,G=0,B=0)
	C.fillRect(ctx, 0, 1, C.double(w), 1, 1.0, 0.0, 0.0)
	// Fill y=H-2 (top area, Y-up) with green (R=0,G=255,B=0)
	C.fillRect(ctx, 0, C.double(h-2), C.double(w), 1, 0.0, 1.0, 0.0)

	stride := int(C.contextStride(ctx))
	data := C.contextData(ctx)
	pixels := unsafe.Slice((*byte)(data), h*stride)

	// Check which row in buffer has red (R=255, G=0)
	for row := range h {
		r := pixels[row*stride+0]
		g := pixels[row*stride+1]
		if r > 200 && g < 50 {
			// Red is at buffer row `row`. In Y-up coords, y=1 is near bottom.
			// If flipNeeded: row 1 should be at buffer row 1 (no flip) → buffer row near bottom
			// Meaning: if row is NEAR h-1, then buffer row 0 = Y-up y=0 = bottom → need flip.
			flipNeeded := row < h/2
			log.Printf("probe: red (Y-up y=1) found at buffer row %d/%d → flipNeeded=%v", row, h, flipNeeded)
			return flipNeeded
		}
	}
	log.Printf("probe: red not found (unexpected)")
	return true // default: flip
}

// RenderToImage renders the terminal capture buffer to an RGBA image.
func RenderToImage(t *terminal.Terminal, h FontHandle) *image.RGBA {
	df := h.impl.(darwinFont)
	charW := h.CharW
	lineH := h.LineH
	cols, rows := t.CaptureSize()
	imgW := cols*charW + 2*PadX
	imgH := rows*lineH + 2*PadY

	// Probe once to determine the correct row mapping.
	flipNeeded := probeCGLayout(imgW, imgH)

	ctx := C.makeContext(C.int(imgW), C.int(imgH))
	defer C.CFRelease(C.CFTypeRef(unsafe.Pointer(ctx))) //nolint:govet

	defBg := ResolveColor(DefaultBg, 0x22, 0x22, 0x35)
	defFg := ResolveColor(DefaultFg, 0xb8, 0xc0, 0xd4)

	C.fillRect(ctx, 0, 0, C.double(imgW), C.double(imgH),
		cr(defBg.R), cr(defBg.G), cr(defBg.B))

	// Y-up coordinates: row 0 of screen is near the TOP of the image.
	// cellBottom (Y-up) = imgH - padY - (row+1)*lineH
	// baseline   (Y-up) = cellBottom + descent
	for row := range rows {
		cellBottom := float64(imgH - PadY - (row+1)*lineH)
		baseline := cellBottom + df.descent

		for col := range cols {
			ch, f := t.CaptureCell(col, row)
			if ch == 0 {
				ch = ' '
			}
			bg, fg := CellColors(f, defBg, defFg)
			cellX := float64(PadX + col*charW)

			C.fillRect(ctx,
				C.double(cellX), C.double(cellBottom),
				C.double(charW), C.double(lineH),
				cr(bg.R), cr(bg.G), cr(bg.B))

			if ch == ' ' {
				continue
			}

			C.drawGlyph(ctx, df.f,
				C.uint32_t(ch),
				C.double(cellX), C.double(baseline),
				cr(fg.R), cr(fg.G), cr(fg.B))
		}
	}

	stride := int(C.contextStride(ctx))
	data := C.contextData(ctx)
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	// kCGBitmapByteOrder32Big | kCGImageAlphaPremultipliedLast → bytes are R,G,B,A in memory.
	// No channel swap needed. Row direction determined by probe.
	for bufRow := range imgH {
		src := unsafe.Slice((*byte)(unsafe.Add(data, bufRow*stride)), imgW*4)
		var imgRow int
		if flipNeeded {
			imgRow = imgH - 1 - bufRow
		} else {
			imgRow = bufRow
		}
		dst := img.Pix[imgRow*img.Stride : imgRow*img.Stride+imgW*4]
		for x := 0; x < imgW*4; x += 4 {
			a := src[x+3]
			if a == 255 || a == 0 {
				dst[x+0] = src[x+0]
				dst[x+1] = src[x+1]
				dst[x+2] = src[x+2]
			} else {
				dst[x+0] = uint8(int(src[x+0]) * 255 / int(a))
				dst[x+1] = uint8(int(src[x+1]) * 255 / int(a))
				dst[x+2] = uint8(int(src[x+2]) * 255 / int(a))
			}
			dst[x+3] = 255
		}
	}
	return img
}

func cr(v uint8) C.double { return C.double(float64(v) / 255.0) }
