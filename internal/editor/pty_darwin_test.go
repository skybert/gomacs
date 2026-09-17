//go:build darwin

package editor

import (
	"testing"
	"time"
)

// TestOpenPTY_Darwin exercises the real posix_openpt/grantpt/unlockpt/ptsname
// path on macOS. It opens a master/slave pair, does a small round-trip
// write/read to prove the pair is actually connected, and closes both ends
// so the test does not leak file descriptors.
func TestOpenPTY_Darwin(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		// Some sandboxes cannot allocate a PTY at all; skip rather than
		// fail so the test suite stays green in restricted environments.
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
	// and read the canonicalised line back from the slave. openPTY does not
	// set O_NONBLOCK, so the returned *os.File does not support
	// SetReadDeadline/SetWriteDeadline ("file type does not support
	// deadline"); instead run the read in a goroutine and bound the wait
	// with a select/timer so a broken pair fails fast rather than hanging
	// the test run forever.
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

// TestSetPTYSize_Darwin verifies setPTYSize succeeds on a freshly opened PTY
// and that it rejects an invalid (closed) file, giving deterministic
// coverage of the error path as well as the happy path.
func TestSetPTYSize_Darwin(t *testing.T) {
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

// TestOpenPTY_Darwin_SlaveNameLooksLikeADevPtsPath is a light sanity check
// that the slave device name follows the expected /dev/ttys* or /dev/pts/*
// convention rather than being garbage from the ptsname() C call.
func TestOpenPTY_Darwin_SlaveNameLooksLikeADevPtsPath(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		t.Skipf("openPTY: %v (environment may not support PTY allocation)", err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()

	name := slave.Name()
	if len(name) < len("/dev/") || name[:len("/dev/")] != "/dev/" {
		t.Errorf("slave name %q does not look like a /dev path", name)
	}
}
