// Command shotgen generates PNG screenshots of gomacs in various major modes.
// Screenshots are written to doc/<platform>/<mode>.png.
//
// Run from the repository root:
//
//	go run ./cmd/shotgen
//	go run ./cmd/shotgen -font-size 20
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skybert/gomacs/internal/editor"
	"github.com/skybert/gomacs/internal/screenshot"
	"github.com/skybert/gomacs/internal/terminal"
)

// Screen dimensions for screenshots.
// screenCols and screenRows are computed at runtime from the font metrics
// and the target image dimensions below, so that all screenshots are
// approximately the same pixel size regardless of -font-size.
// A larger font size therefore means fewer visible columns/rows (zoomed in).
const (
	targetW = 1920 // target output width in device pixels
	targetH = 1372 // target output height in device pixels
)

// fontSize is the point size used when rendering screenshots.
// Override with -font-size to match your terminal's font size.
var fontSize = screenshot.DefaultFontSize

func main() {
	fs := flag.NewFlagSet("shotgen", flag.ContinueOnError)
	fs.Float64Var(&fontSize, "font-size", screenshot.DefaultFontSize, "font point size for rendering (match your terminal's font size)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	platform := runtime.GOOS
	outDir := filepath.Join("doc", platform)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	h, err := screenshot.LoadFont(fontSize)
	if err != nil {
		log.Fatalf("load font: %v", err)
	}

	screenCols := (targetW - 2*screenshot.PadX) / h.CharW
	screenRows := (targetH - 2*screenshot.PadY) / h.LineH
	log.Printf("screen: %d cols × %d rows", screenCols, screenRows)

	shots := []struct {
		mode string
		fn   func(*editor.Editor)
	}{
		{"go-mode", setupGoMode},
		{"elisp-mode", setupElispMode},
		{"bash-mode", setupBashMode},
		{"markdown-mode", setupMarkdownMode},
		{"dired", setupDired},
		{"vc-status", setupVcStatus},
		{"shell", setupShell},
	}

	for _, s := range shots {
		t := terminal.NewCapture(screenCols, screenRows)
		e, nerr := editor.NewForScreenshot(t, screenCols, screenRows)

		if nerr != nil {
			log.Fatalf("%s: create editor: %v", s.mode, nerr)
		}
		s.fn(e)
		e.Redraw()

		img := screenshot.RenderToImage(t, h)
		outPath := filepath.Join(outDir, s.mode+".png")
		if werr := screenshot.WritePNG(img, outPath); werr != nil {
			log.Fatalf("%s: write PNG: %v", s.mode, werr)
		}
		fmt.Printf("wrote %s\n", outPath)
	}
}

// ---------------------------------------------------------------------------
// Scene setup helpers
// ---------------------------------------------------------------------------

func setupGoMode(e *editor.Editor) {
	if err := e.OpenFile("internal/editor/editor.go"); err != nil {
		_ = e.OpenFile("main.go")
	}
}

func setupElispMode(e *editor.Editor) {
	_ = e.OpenFile("specs/sweet-colors.el")
}

func setupBashMode(e *editor.Editor) {
	_ = e.OpenFile("Makefile")
}

func setupMarkdownMode(e *editor.Editor) {
	if err := e.OpenFile("README.md"); err != nil {
		_ = e.OpenFile("doc/gomacs-user-guide.md")
	}
}

func setupDired(e *editor.Editor) {
	cwd, _ := os.Getwd()
	e.OpenDiredPath(cwd)
}

func setupVcStatus(e *editor.Editor) {
	e.RunCommand("vc-status")
}

func setupShell(e *editor.Editor) {
	e.SetupShellPreview(buildShellContent())
}

// buildShellContent returns a realistic shell session transcript.
func buildShellContent() string {
	var sb strings.Builder
	sb.WriteString("torstein@laptop:~/src/skybert/gomacs$ git log --oneline -5\n")
	if out, err := runCmd("git", "log", "--oneline", "-5"); err == nil && out != "" {
		sb.WriteString(out)
		if !strings.HasSuffix(out, "\n") {
			sb.WriteByte('\n')
		}
	} else {
		sb.WriteString("f5564f6 Fix image descriptions\n")
		sb.WriteString("cf31977 Add links to see images in full resolution\n")
		sb.WriteString("e3f835a Update description with reference to Android doc\n")
		sb.WriteString("5cd333e Ensure the pictures don't render too big\n")
		sb.WriteString("9016ab9 Refer to the manual instead of listing features\n")
	}
	sb.WriteString("torstein@laptop:~/src/skybert/gomacs$ go test ./internal/editor/ -run TestKeyToShellBytes\n")
	sb.WriteString("ok  \tgithub.com/skybert/gomacs/internal/editor\t0.312s\n")
	sb.WriteString("torstein@laptop:~/src/skybert/gomacs$ ")
	return sb.String()
}

// runCmd executes a command and returns its trimmed stdout output.
func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output() //nolint:gosec
	return strings.TrimRight(string(out), "\n"), err
}
