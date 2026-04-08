//go:build !linux && !darwin

package editor

import (
	"errors"
	"os"
)

// openPTY is not supported on this platform.
func openPTY() (*os.File, *os.File, error) {
	return nil, nil, errors.New("shell: PTY not supported on this platform")
}

// setPTYSize is a no-op on unsupported platforms.
func setPTYSize(*os.File, int, int) error { return nil }
