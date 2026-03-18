// gomacs — a TTY-only Emacs clone written in Go.
//
// Usage:
//
//	gomacs [-Q] [file...]
//
// If file arguments are given each file is opened in its own buffer and the
// first one is shown on startup.  Without arguments the *scratch* buffer is
// displayed.
//
// If data is piped to gomacs it is opened in a *stdin* buffer.
//
// gomacs loads ~/.gomacs or ~/.config/gomacs/init.el on startup unless -Q is given.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/skybert/gomacs/internal/editor"
)

// Version is set at build time via: -ldflags "-X main.Version=<version>"
var Version = "dev"

func main() {
	quick := flag.Bool("Q", false, "start with minimum customisation (skip init file)")
	flag.Parse()

	// Drain piped stdin before tcell claims /dev/tty for keyboard input.
	var stdinData []byte
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		stdinData, _ = io.ReadAll(os.Stdin)
		// Reopen /dev/tty as stdin so tcell can read keyboard events.
		if tty, err := os.Open("/dev/tty"); err == nil {
			os.Stdin = tty
		}
	}

	opts := editor.Options{Quick: *quick, StdinData: stdinData, Version: Version}
	e, err := editor.New(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gomacs: init error: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()

	// Open any files supplied on the command line.
	for _, path := range flag.Args() {
		if err := e.OpenFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "gomacs: %v\n", err)
		}
	}

	e.Run()
}
