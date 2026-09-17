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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/skybert/gomacs/internal/editor"
)

// Version is set at build time via: -ldflags "-X main.Version=<version>"
var Version = "dev"

// parsedArgs holds gomacs' command-line configuration once parsed.
type parsedArgs struct {
	Quick   bool     // -Q: skip loading the init file
	Version bool     // -version: print version and exit
	Files   []string // remaining positional arguments: files to open
}

// parseArgs parses gomacs' command-line flags out of args, which is normally
// os.Args[1:]. It uses its own FlagSet rather than the package-level
// flag.CommandLine so it can be called repeatedly (e.g. from tests) without
// leaking state across calls.
//
// The returned error is exactly what FlagSet.Parse returns: flag.ErrHelp for
// -h/-help, or a parse error (e.g. an unknown flag) otherwise. Usage text for
// parse errors is written to os.Stderr, matching flag.ExitOnError behaviour.
func parseArgs(args []string) (parsedArgs, error) {
	fs := flag.NewFlagSet("gomacs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	quick := fs.Bool("Q", false, "start with minimum customisation (skip init file)")
	version := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return parsedArgs{}, err
	}
	return parsedArgs{Quick: *quick, Version: *version, Files: fs.Args()}, nil
}

// versionString formats the string printed by -version.
func versionString(v string) string {
	return "gomacs " + v
}

// stdinIsPiped reports whether fi describes a non-terminal stdin, i.e. data
// has been piped into gomacs rather than a tty being attached.
func stdinIsPiped(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeCharDevice == 0
}

func main() {
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if args.Version {
		fmt.Println(versionString(Version))
		os.Exit(0)
	}

	// Drain piped stdin before tcell claims /dev/tty for keyboard input.
	var stdinData []byte
	if fi, statErr := os.Stdin.Stat(); statErr == nil && stdinIsPiped(fi) {
		stdinData, _ = io.ReadAll(os.Stdin)
		// Reopen /dev/tty as stdin so tcell can read keyboard events.
		if tty, openErr := os.Open("/dev/tty"); openErr == nil {
			os.Stdin = tty
		}
	}

	opts := editor.Options{Quick: args.Quick, StdinData: stdinData, Version: Version}
	e, err := editor.New(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gomacs: init error: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()

	// Open any files supplied on the command line.
	for _, path := range args.Files {
		if err := e.OpenFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "gomacs: %v\n", err)
		}
	}

	e.Run()
}
