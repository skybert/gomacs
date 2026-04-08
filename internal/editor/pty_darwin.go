//go:build darwin

package editor

/*
#include <stdlib.h>
#include <fcntl.h>
#include <string.h>
#include <unistd.h>

static int pty_open_master(int *out_fd) {
    int fd = posix_openpt(O_RDWR | O_NOCTTY | O_CLOEXEC);
    if (fd < 0) return -1;
    if (grantpt(fd) < 0) { close(fd); return -1; }
    if (unlockpt(fd) < 0) { close(fd); return -1; }
    *out_fd = fd;
    return 0;
}

static int pty_slave_name(int master_fd, char *buf, int buflen) {
    char *name = ptsname(master_fd);
    if (!name) return -1;
    strncpy(buf, name, (size_t)buflen - 1);
    buf[buflen - 1] = '\0';
    return 0;
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY opens a master/slave PTY pair and returns them as *os.File.
func openPTY() (*os.File, *os.File, error) {
	var masterFdC C.int
	if C.pty_open_master(&masterFdC) < 0 {
		return nil, nil, fmt.Errorf("posix_openpt/grantpt/unlockpt failed")
	}
	masterFd := int(masterFdC)

	var nameBuf [128]C.char
	if C.pty_slave_name(masterFdC, &nameBuf[0], C.int(len(nameBuf))) < 0 {
		_ = unix.Close(masterFd)
		return nil, nil, fmt.Errorf("ptsname failed")
	}
	slaveName := C.GoString((*C.char)(unsafe.Pointer(&nameBuf[0])))

	slaveFd, err := unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = unix.Close(masterFd)
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
