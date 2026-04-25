//go:build linux

package editor

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// closeAndIgnoreErr closes the file, and ignores any errors it may
// throw. Useful when we're already in an error situation.
func closeAndIgnoreErr(fd int) {
	_ = unix.Close(fd)
}

// openPTY opens a master/slave PTY pair and returns them as *os.File.
func openPTY() (*os.File, *os.File, error) {
	masterFd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	// Unlock the slave PTY (TIOCSPTLCK with 0).
	var zero int32
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(masterFd),
		unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&zero))); errno != 0 {
		// Not fatal — some kernels don't support this ioctl.
		_ = errno
	}

	// Get the slave PTY device number (TIOCGPTN).
	var n uint32
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(masterFd),
		unix.TIOCGPTN, uintptr(unsafe.Pointer(&n))); errno != 0 {
		closeAndIgnoreErr(masterFd)
		return nil, nil, fmt.Errorf("TIOCGPTN: %w", errno)
	}
	slaveName := fmt.Sprintf("/dev/pts/%d", n)

	slaveFd, err := unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		closeAndIgnoreErr(masterFd)
		return nil, nil, fmt.Errorf("open slave PTY %s: %w", slaveName, err)
	}

	return os.NewFile(uintptr(masterFd), "/dev/ptmx"),
		os.NewFile(uintptr(slaveFd), slaveName),
		nil
}

// setPTYSize sets the terminal window size on the master PTY.
func setPTYSize(master *os.File, rows, cols int) error {
	ws := &unix.Winsize{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	return unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, ws)
}
