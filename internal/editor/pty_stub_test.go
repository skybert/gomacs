//go:build !linux && !darwin

package editor

import (
	"testing"
)

// TestOpenPTY_Stub verifies the fallback used on platforms without a native
// PTY implementation (currently everything except linux and darwin):
// openPTY must report the unsupported-platform error rather than pretend to
// succeed, and it must not return usable files.
func TestOpenPTY_Stub(t *testing.T) {
	master, slave, err := openPTY()
	if err == nil {
		t.Fatal("openPTY: want an error on an unsupported platform, got nil")
	}
	if master != nil {
		t.Errorf("openPTY: want nil master on error, got %v", master)
	}
	if slave != nil {
		t.Errorf("openPTY: want nil slave on error, got %v", slave)
	}

	const want = "shell: PTY not supported on this platform"
	if got := err.Error(); got != want {
		t.Errorf("openPTY error = %q, want %q", got, want)
	}
}

// TestSetPTYSize_Stub verifies setPTYSize is a documented no-op on
// unsupported platforms: it must return nil even when passed a nil file, so
// that any (unreachable, since openPTY always fails first) caller cannot be
// tripped up by it.
func TestSetPTYSize_Stub(t *testing.T) {
	if err := setPTYSize(nil, 24, 80); err != nil {
		t.Errorf("setPTYSize(nil, ...) = %v, want nil", err)
	}
}
