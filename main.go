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
// gomacs loads ~/.emacs or ~/.emacs.d/init.el on startup unless -Q is given.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/skybert/gomacs/internal/editor"
)

func main() {
	quick := flag.Bool("Q", false, "start with minimum customisation (skip init file)")
	flag.Parse()

	opts := editor.Options{Quick: *quick}
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
