package editor

import (
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// lookupFor builds a look function for clipboardCmdFor that reports the named
// tools as installed and every other tool as missing.
func lookupFor(installed ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, ok := range installed {
			if ok == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

// TestClipboardCmdFor covers every platform/environment combination, including
// the fall-through case that used to silently disable the clipboard: a Wayland
// session without wl-copy but with an X11 tool reachable through XWayland.
func TestClipboardCmdFor(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		wayland string
		x11     string
		tools   []string
		want    []string
	}{
		{
			name:  "darwin uses pbcopy",
			goos:  "darwin",
			tools: nil,
			want:  []string{"pbcopy"},
		},
		{
			name:    "darwin ignores display env",
			goos:    "darwin",
			wayland: "wayland-0",
			x11:     ":0",
			tools:   []string{"wl-copy", "xclip"},
			want:    []string{"pbcopy"},
		},
		{
			name:    "wayland with wl-copy",
			goos:    "linux",
			wayland: "wayland-0",
			tools:   []string{"wl-copy"},
			want:    []string{"wl-copy"},
		},
		{
			name:    "wayland prefers wl-copy over xclip",
			goos:    "linux",
			wayland: "wayland-0",
			x11:     ":0",
			tools:   []string{"wl-copy", "xclip"},
			want:    []string{"wl-copy"},
		},
		{
			name:    "wayland without wl-copy falls back to xclip",
			goos:    "linux",
			wayland: "wayland-0",
			x11:     ":0",
			tools:   []string{"xclip"},
			want:    []string{"xclip", "-selection", "clipboard"},
		},
		{
			name:    "wayland without wl-copy or xclip falls back to xsel",
			goos:    "linux",
			wayland: "wayland-0",
			x11:     ":0",
			tools:   []string{"xsel"},
			want:    []string{"xsel", "--clipboard", "--input"},
		},
		{
			name:  "x11 with xclip",
			goos:  "linux",
			x11:   ":0",
			tools: []string{"xclip"},
			want:  []string{"xclip", "-selection", "clipboard"},
		},
		{
			name:  "x11 with only xsel",
			goos:  "linux",
			x11:   ":0",
			tools: []string{"xsel"},
			want:  []string{"xsel", "--clipboard", "--input"},
		},
		{
			name:  "x11 prefers xclip over xsel",
			goos:  "linux",
			x11:   ":0",
			tools: []string{"xclip", "xsel"},
			want:  []string{"xclip", "-selection", "clipboard"},
		},
		{
			name:  "x11 with no tool installed",
			goos:  "linux",
			x11:   ":0",
			tools: nil,
			want:  nil,
		},
		{
			name:    "wayland only, no tool installed",
			goos:    "linux",
			wayland: "wayland-0",
			tools:   nil,
			want:    nil,
		},
		{
			name:  "no display at all",
			goos:  "linux",
			tools: []string{"wl-copy", "xclip", "xsel"},
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clipboardCmdFor(tc.goos, tc.wayland, tc.x11, lookupFor(tc.tools...))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("clipboardCmdFor(%q, %q, %q, %v) = %v, want %v",
					tc.goos, tc.wayland, tc.x11, tc.tools, got, tc.want)
			}
		})
	}
}

// TestClipboardCmdForUsesLookPathResult verifies that a tool is only chosen
// when the lookup succeeds — an erroring lookup must not select it.
func TestClipboardCmdForUsesLookPathResult(t *testing.T) {
	calls := []string{}
	look := func(name string) (string, error) {
		calls = append(calls, name)
		return "", errors.New("not found")
	}
	if got := clipboardCmdFor("linux", "wayland-0", ":0", look); got != nil {
		t.Fatalf("no tool installed: got %v, want nil", got)
	}
	want := []string{"wl-copy", "xclip", "xsel"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("lookup order = %v, want %v", calls, want)
	}
}

// TestClipboardCmdWiring checks that clipboardCmd turns the argv chosen by
// clipboardCmdFor into a matching exec.Cmd on this machine.  The display
// environment is neutralised so both sides see the same inputs.
func TestClipboardCmdWiring(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	want := clipboardCmdFor(runtime.GOOS, "", "", exec.LookPath)
	got := clipboardCmd()

	switch {
	case want == nil:
		if got != nil {
			t.Fatalf("expected no clipboard command, got %v", got.Args)
		}
	case got == nil:
		t.Fatalf("expected clipboard command %v, got nil", want)
	default:
		if !reflect.DeepEqual(got.Args, want) {
			t.Errorf("clipboardCmd args = %v, want %v", got.Args, want)
		}
		if !strings.HasSuffix(got.Path, want[0]) {
			t.Errorf("clipboardCmd path = %q, want suffix %q", got.Path, want[0])
		}
	}
}

// TestClipboardWriteNoPanic runs the clipboard command in a background
// goroutine; it must not panic even when no clipboard tool is available.
func TestClipboardWriteNoPanic(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	clipboardWrite("hello clipboard")
}
