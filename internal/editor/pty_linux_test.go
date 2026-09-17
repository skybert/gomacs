//go:build linux

package editor

import (
	"testing"
	"time"
)

// TestOpenPTY_Linux exercises the /dev/ptmx + TIOCGPTN + TIOCSPTLCK path.
// It opens a master/slave pair, does a small round-trip write/read through
// the pair, and closes both ends so the test does not leak file
// descriptors. Like the darwin variant, openPTY does not set O_NONBLOCK, so
// read/write deadlines are unavailable on the returned *os.File; the read is
// bounded with a goroutine + timer instead of Set{Read,Write}Deadline.
func TestOpenPTY_Linux(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		// Some sandboxes (e.g. containers without /dev/ptmx or devpts
		// mounted) cannot allocate a PTY; skip rather than fail.
		t.Skipf("openPTY: %v (environment may not support PTY allocation)", err)
	}
	if master == nil || slave == nil {
		t.Fatal("openPTY returned nil file with nil error")
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()

	if master.Name() == "" {
		t.Error("master.Name() is empty")
	}
	if slave.Name() == "" {
		t.Error("slave.Name() is empty")
	}

	// Round-trip: write to the master (as if a user typed at the terminal)
	// and read the canonicalised line back from the slave.
	want := "hello pty\n"
	if _, err := master.Write([]byte(want)); err != nil {
		t.Fatalf("master.Write: %v", err)
	}

	type readResult struct {
		n   int
		buf []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		buf := make([]byte, len(want))
		n, err := slave.Read(buf)
		done <- readResult{n: n, buf: buf, err: err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("slave.Read: %v", res.err)
		}
		if got := string(res.buf[:res.n]); got != want {
			t.Errorf("round-trip mismatch: got %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting to read back from slave")
	}
}

// TestSetPTYSize_Linux verifies setPTYSize succeeds on a freshly opened PTY
// and that it rejects an invalid (closed) file, giving deterministic
// coverage of the error path as well as the happy path.
func TestSetPTYSize_Linux(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		t.Skipf("openPTY: %v (environment may not support PTY allocation)", err)
	}
	defer func() { _ = slave.Close() }()

	if err := setPTYSize(master, 24, 80); err != nil {
		_ = master.Close()
		t.Fatalf("setPTYSize on live master: %v", err)
	}

	// Closing the master first, then calling setPTYSize on it, is a
	// deterministic way to reach the ioctl error path.
	if err := master.Close(); err != nil {
		t.Fatalf("master.Close: %v", err)
	}
	if err := setPTYSize(master, 24, 80); err == nil {
		t.Error("setPTYSize on a closed master: want error, got nil")
	}
}

// TestOpenPTY_Linux_SlaveNameLooksLikeADevPtsPath checks that the slave
// device name follows the /dev/pts/<n> convention derived from TIOCGPTN,
// rather than being garbage.
func TestOpenPTY_Linux_SlaveNameLooksLikeADevPtsPath(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		t.Skipf("openPTY: %v (environment may not support PTY allocation)", err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()

	const prefix = "/dev/pts/"
	name := slave.Name()
	if len(name) < len(prefix) || name[:len(prefix)] != prefix {
		t.Errorf("slave name %q does not look like a %s path", name, prefix)
	}
}

// TestCloseAndIgnoreErr makes sure the helper does not panic on a
// deterministic error (double-close of an already-closed fd) and that it
// silently swallows the error as documented.
func TestCloseAndIgnoreErr(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		t.Skipf("openPTY: %v (environment may not support PTY allocation)", err)
	}
	defer func() { _ = slave.Close() }()

	fd := int(master.Fd())
	if err := master.Close(); err != nil {
		t.Fatalf("master.Close: %v", err)
	}
	// fd is now invalid; closeAndIgnoreErr must not panic.
	closeAndIgnoreErr(fd)
}
