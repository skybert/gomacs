package keymap_test

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/keymap"
)

// ---------------------------------------------------------------------------
// Basic bind + lookup
// ---------------------------------------------------------------------------

func TestBindAndLookupCommand(t *testing.T) {
	km := keymap.New("test")
	key := keymap.CtrlKey('f')
	km.Bind(key, "forward-char")

	b, ok := km.Lookup(key)
	if !ok {
		t.Fatal("expected key to be found")
	}
	if b.Command != "forward-char" {
		t.Fatalf("expected command %q, got %q", "forward-char", b.Command)
	}
	if b.Prefix != nil {
		t.Fatal("expected Prefix to be nil for a command binding")
	}
}

func TestLookupNotFound(t *testing.T) {
	km := keymap.New("empty")
	_, ok := km.Lookup(keymap.CtrlKey('x'))
	if ok {
		t.Fatal("expected key not to be found in an empty keymap")
	}
}

// ---------------------------------------------------------------------------
// Prefix binding
// ---------------------------------------------------------------------------

func TestPrefixKeyLookup(t *testing.T) {
	ctrlXMap := keymap.New("ctrl-x-map")
	ctrlXMap.Bind(keymap.CtrlKey('s'), "save-buffer")

	global := keymap.New("global")
	ctrlX := keymap.CtrlKey('x')
	global.BindPrefix(ctrlX, ctrlXMap)

	b, ok := global.Lookup(ctrlX)
	if !ok {
		t.Fatal("expected Ctrl+x to be found as a prefix")
	}
	if b.Prefix == nil {
		t.Fatal("expected Prefix to be non-nil")
	}
	if b.Command != "" {
		t.Fatalf("expected Command to be empty for a prefix binding, got %q", b.Command)
	}

	// Follow the prefix map.
	b2, ok2 := b.Prefix.Lookup(keymap.CtrlKey('s'))
	if !ok2 {
		t.Fatal("expected Ctrl+s to be found inside ctrl-x-map")
	}
	if b2.Command != "save-buffer" {
		t.Fatalf("expected %q, got %q", "save-buffer", b2.Command)
	}
}

// ---------------------------------------------------------------------------
// Parent fallback
// ---------------------------------------------------------------------------

func TestParentFallback(t *testing.T) {
	parent := keymap.New("parent")
	parent.Bind(keymap.MetaKey('x'), "execute-extended-command")

	child := keymap.NewWithParent("child", parent)
	// Bind something only in child.
	child.Bind(keymap.CtrlKey('n'), "next-line")

	// Key in child → found in child.
	b, ok := child.Lookup(keymap.CtrlKey('n'))
	if !ok || b.Command != "next-line" {
		t.Fatalf("expected next-line from child, got ok=%v cmd=%q", ok, b.Command)
	}

	// Key only in parent → falls through.
	b, ok = child.Lookup(keymap.MetaKey('x'))
	if !ok {
		t.Fatal("expected Meta+x to be found via parent fallback")
	}
	if b.Command != "execute-extended-command" {
		t.Fatalf("expected %q via fallback, got %q", "execute-extended-command", b.Command)
	}
}

func TestChildShadowsParent(t *testing.T) {
	parent := keymap.New("parent")
	parent.Bind(keymap.CtrlKey('a'), "beginning-of-line")

	child := keymap.NewWithParent("child", parent)
	child.Bind(keymap.CtrlKey('a'), "move-beginning-of-line")

	b, ok := child.Lookup(keymap.CtrlKey('a'))
	if !ok {
		t.Fatal("expected key to be found")
	}
	// Child binding should win.
	if b.Command != "move-beginning-of-line" {
		t.Fatalf("expected child binding %q, got %q", "move-beginning-of-line", b.Command)
	}
}

// ---------------------------------------------------------------------------
// Helper constructors
// ---------------------------------------------------------------------------

func TestMakeKey(t *testing.T) {
	k := keymap.MakeKey(tcell.KeyF1, 0, 0)
	if k.Key != tcell.KeyF1 {
		t.Fatalf("expected KeyF1, got %v", k.Key)
	}
}

func TestPlainKey(t *testing.T) {
	k := keymap.PlainKey('a')
	if k.Key != tcell.KeyRune || k.Rune != 'a' || k.Mod != 0 {
		t.Fatalf("unexpected PlainKey: %+v", k)
	}
}

func TestMetaKey(t *testing.T) {
	k := keymap.MetaKey('f')
	if k.Mod != tcell.ModAlt {
		t.Fatalf("expected ModAlt, got %v", k.Mod)
	}
	if k.Rune != 'f' {
		t.Fatalf("expected rune 'f', got %q", k.Rune)
	}
}

func TestCtrlKeyLetters(t *testing.T) {
	tests := []struct {
		r    rune
		want tcell.Key
	}{
		{'a', tcell.KeyCtrlA},
		{'b', tcell.KeyCtrlB},
		{'f', tcell.KeyCtrlF},
		{'n', tcell.KeyCtrlN},
		{'p', tcell.KeyCtrlP},
		{'x', tcell.KeyCtrlX},
		{'z', tcell.KeyCtrlZ},
		// Uppercase should map identically.
		{'A', tcell.KeyCtrlA},
		{'Z', tcell.KeyCtrlZ},
	}
	for _, tt := range tests {
		k := keymap.CtrlKey(tt.r)
		if k.Key != tt.want {
			t.Errorf("CtrlKey(%q): got key %v, want %v", tt.r, k.Key, tt.want)
		}
	}
}

func TestCtrlSpace(t *testing.T) {
	k := keymap.CtrlKey(' ')
	if k.Key != tcell.KeyRune || k.Rune != ' ' || k.Mod != tcell.ModCtrl {
		t.Fatalf("expected {KeyRune, ' ', ModCtrl}, got %+v", k)
	}
}

func TestCtrlSlash(t *testing.T) {
	k := keymap.CtrlKey('/')
	if k.Key != tcell.KeyRune || k.Rune != '/' || k.Mod != tcell.ModCtrl {
		t.Fatalf("expected {KeyRune, '/', ModCtrl}, got %+v", k)
	}
}

func TestName(t *testing.T) {
	km := keymap.New("global-map")
	if km.Name() != "global-map" {
		t.Fatalf("expected %q, got %q", "global-map", km.Name())
	}
}

// ---------------------------------------------------------------------------
// FormatKey
// ---------------------------------------------------------------------------

func TestFormatKeyCtrlLetter(t *testing.T) {
	tests := []struct {
		key  keymap.ModKey
		want string
	}{
		{keymap.CtrlKey('f'), "C-f"},
		{keymap.CtrlKey('x'), "C-x"},
		{keymap.CtrlKey('g'), "C-g"},
		{keymap.CtrlKey(' '), "C-SPC"},
		{keymap.CtrlKey('/'), "C-/"},
	}
	for _, tc := range tests {
		got := keymap.FormatKey(tc.key)
		if got != tc.want {
			t.Errorf("FormatKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestFormatKeyMeta(t *testing.T) {
	tests := []struct {
		key  keymap.ModKey
		want string
	}{
		{keymap.MetaKey('x'), "M-x"},
		{keymap.MetaKey('f'), "M-f"},
		{keymap.MetaKey('<'), "M-<"},
	}
	for _, tc := range tests {
		got := keymap.FormatKey(tc.key)
		if got != tc.want {
			t.Errorf("FormatKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestFormatKeySpecial(t *testing.T) {
	tests := []struct {
		key  keymap.ModKey
		want string
	}{
		{keymap.MakeKey(tcell.KeyEnter, 0, 0), "RET"},
		{keymap.MakeKey(tcell.KeyTab, 0, 0), "TAB"},
		{keymap.MakeKey(tcell.KeyEscape, 0, 0), "ESC"},
		{keymap.MakeKey(tcell.KeyRight, 0, 0), "<right>"},
		{keymap.MakeKey(tcell.KeyLeft, 0, 0), "<left>"},
		{keymap.MakeKey(tcell.KeyUp, 0, 0), "<up>"},
		{keymap.MakeKey(tcell.KeyDown, 0, 0), "<down>"},
	}
	for _, tc := range tests {
		got := keymap.FormatKey(tc.key)
		if got != tc.want {
			t.Errorf("FormatKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestFormatKeyPlainRune(t *testing.T) {
	key := keymap.PlainKey('a')
	got := keymap.FormatKey(key)
	if got != "a" {
		t.Errorf("FormatKey plain 'a' = %q, want \"a\"", got)
	}
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

func TestBindingsReturnsOwnBindings(t *testing.T) {
	km := keymap.New("test")
	km.Bind(keymap.CtrlKey('f'), "forward-char")
	km.Bind(keymap.CtrlKey('b'), "backward-char")

	bindings := km.Bindings()
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
}

func TestBindingsDoesNotIncludeParent(t *testing.T) {
	parent := keymap.New("parent")
	parent.Bind(keymap.CtrlKey('f'), "forward-char")
	child := keymap.NewWithParent("child", parent)
	child.Bind(keymap.CtrlKey('b'), "backward-char")

	bindings := child.Bindings()
	if len(bindings) != 1 {
		t.Fatalf("Bindings() should return only own bindings; got %d", len(bindings))
	}
}
