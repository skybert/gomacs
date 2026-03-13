// Package keymap provides Emacs-style keymaps: layered maps from key chords
// to either command names (strings) or subordinate prefix keymaps.
//
// Key chords are represented by ModKey, which bundles a tcell.Key constant,
// a Unicode rune (used when Key == tcell.KeyRune), and a modifier mask.
//
// Lookup follows parent chains, so a minor-mode keymap can fall through to a
// major-mode keymap and ultimately to the global keymap.
package keymap

import (
	"fmt"
	"maps"

	"github.com/gdamore/tcell/v2"
)

// ---------------------------------------------------------------------------
// ModKey
// ---------------------------------------------------------------------------

// ModKey is the identity of a single key chord.
//
//   - For printable characters:       Key=tcell.KeyRune, Rune=<char>
//   - For Ctrl+letter:                Key=tcell.KeyCtrlA … tcell.KeyCtrlZ (Rune=0)
//   - For special keys (arrows, …):   Key=tcell.Key<Name> (Rune=0)
//   - For Meta/Alt combos:            Mod includes tcell.ModAlt; Rune=<char>
type ModKey struct {
	Key  tcell.Key
	Rune rune
	Mod  tcell.ModMask
}

// ---------------------------------------------------------------------------
// Binding
// ---------------------------------------------------------------------------

// Binding is the value stored in a Keymap entry.
// Exactly one of Command or Prefix is non-zero.
type Binding struct {
	// Command is the name of the editor command to invoke (e.g.
	// "forward-char", "kill-line").  Non-empty for terminal bindings.
	Command string

	// Prefix is non-nil when this key starts a multi-key sequence; the
	// next keystroke should be looked up in Prefix.
	Prefix *Keymap
}

// ---------------------------------------------------------------------------
// Keymap
// ---------------------------------------------------------------------------

// Keymap maps ModKeys to Bindings.
type Keymap struct {
	name     string
	bindings map[ModKey]Binding
	parent   *Keymap // fallback keymap; may be nil
}

// New creates an empty keymap with the given name and no parent.
func New(name string) *Keymap {
	return &Keymap{
		name:     name,
		bindings: make(map[ModKey]Binding),
	}
}

// NewWithParent creates an empty keymap whose Lookup falls through to parent
// when the key is not found locally.
func NewWithParent(name string, parent *Keymap) *Keymap {
	return &Keymap{
		name:     name,
		bindings: make(map[ModKey]Binding),
		parent:   parent,
	}
}

// Name returns the keymap's name.
func (km *Keymap) Name() string { return km.name }

// Bind adds (or replaces) a terminal binding: key → command.
func (km *Keymap) Bind(key ModKey, command string) {
	km.bindings[key] = Binding{Command: command}
}

// BindPrefix adds (or replaces) a prefix binding: key → subordinate keymap.
func (km *Keymap) BindPrefix(key ModKey, prefix *Keymap) {
	km.bindings[key] = Binding{Prefix: prefix}
}

// Lookup returns the Binding for key, searching parent keymaps if necessary.
// The second return value is false if the key is unbound in the entire chain.
func (km *Keymap) Lookup(key ModKey) (Binding, bool) {
	for cur := km; cur != nil; cur = cur.parent {
		if b, ok := cur.bindings[key]; ok {
			return b, true
		}
	}
	return Binding{}, false
}

// ---------------------------------------------------------------------------
// Helper constructors
// ---------------------------------------------------------------------------

// MakeKey is the generic constructor for a ModKey.
func MakeKey(key tcell.Key, r rune, mod tcell.ModMask) ModKey {
	return ModKey{Key: key, Rune: r, Mod: mod}
}

// PlainKey creates a ModKey for an unmodified printable rune.
func PlainKey(r rune) ModKey {
	return ModKey{Key: tcell.KeyRune, Rune: r}
}

// MetaKey creates a ModKey for Meta/Alt + rune (e.g. M-x → MetaKey('x')).
func MetaKey(r rune) ModKey {
	return ModKey{Key: tcell.KeyRune, Rune: r, Mod: tcell.ModAlt}
}

// CtrlKey creates a ModKey for a Ctrl+letter chord.
//
// r must be in the range 'a'–'z', ' ' (space), or '/'.
// Letters map to tcell.KeyCtrlA … tcell.KeyCtrlZ.
// Ctrl+Space maps to tcell.KeyCtrlSpace.
// Ctrl+/ maps to tcell.KeyCtrlUnderscore (terminal convention).
// Any other rune falls back to KeyRune with ModCtrl set.
func CtrlKey(r rune) ModKey {
	// Map lowercase (and uppercase, treated equivalently) letters.
	lr := r
	if lr >= 'A' && lr <= 'Z' {
		lr = lr - 'A' + 'a'
	}
	switch {
	case lr >= 'a' && lr <= 'z':
		// tcell.KeyCtrlA == 1, KeyCtrlB == 2, …, KeyCtrlZ == 26.
		// Cast through int to avoid G115 integer-overflow warning: lr is
		// validated to 'a'–'z', so the offset is always 0–25 (safe for int16).
		offset := int(lr) - int('a')
		return ModKey{Key: tcell.KeyCtrlA + tcell.Key(offset)} //nolint:gosec
	case lr == ' ':
		return ModKey{Key: tcell.KeyCtrlSpace}
	case lr == '/':
		// Most terminals send 0x1f (US) for Ctrl+/, which tcell calls
		// KeyCtrlUnderscore.
		return ModKey{Key: tcell.KeyCtrlUnderscore}
	default:
		// Fallback: record it as a modified rune.
		return ModKey{Key: tcell.KeyRune, Rune: r, Mod: tcell.ModCtrl}
	}
}

// ---------------------------------------------------------------------------
// Key formatting
// ---------------------------------------------------------------------------

// FormatKey returns a human-readable description of a key chord, e.g.
// "C-x", "M-f", "C-SPC", "RET", "<right>".
func FormatKey(key ModKey) string {
	meta := key.Mod&tcell.ModAlt != 0

	var base string
	//nolint:exhaustive // external enum; default case handles unknowns
	switch key.Key {
	case tcell.KeyRune:
		if meta {
			return "M-" + string(key.Rune)
		}
		return string(key.Rune)
	case tcell.KeyCtrlSpace:
		base = "C-SPC"
	case tcell.KeyCtrlUnderscore:
		base = "C-/"
	case tcell.KeyEnter, tcell.KeyCtrlM:
		base = "RET"
	case tcell.KeyTab, tcell.KeyCtrlI:
		base = "TAB"
	case tcell.KeyEscape:
		base = "ESC"
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		base = "DEL"
	case tcell.KeyDelete:
		base = "<delete>"
	case tcell.KeyInsert:
		base = "<insert>"
	case tcell.KeyUp:
		base = "<up>"
	case tcell.KeyDown:
		base = "<down>"
	case tcell.KeyLeft:
		base = "<left>"
	case tcell.KeyRight:
		base = "<right>"
	case tcell.KeyHome:
		base = "<home>"
	case tcell.KeyEnd:
		base = "<end>"
	case tcell.KeyPgUp:
		base = "<prior>"
	case tcell.KeyPgDn:
		base = "<next>"
	case tcell.KeyF1:
		base = "<f1>"
	case tcell.KeyF2:
		base = "<f2>"
	case tcell.KeyF3:
		base = "<f3>"
	case tcell.KeyF4:
		base = "<f4>"
	case tcell.KeyF5:
		base = "<f5>"
	case tcell.KeyF6:
		base = "<f6>"
	case tcell.KeyF7:
		base = "<f7>"
	case tcell.KeyF8:
		base = "<f8>"
	case tcell.KeyF9:
		base = "<f9>"
	case tcell.KeyF10:
		base = "<f10>"
	case tcell.KeyF11:
		base = "<f11>"
	case tcell.KeyF12:
		base = "<f12>"
	default:
		// Ctrl+letter: KeyCtrlA=1 … KeyCtrlZ=26
		if key.Key >= tcell.KeyCtrlA && key.Key <= tcell.KeyCtrlZ {
			letter := rune('a' + int(key.Key-tcell.KeyCtrlA))
			base = "C-" + string(letter)
		} else {
			base = fmt.Sprintf("key(%d)", int(key.Key))
		}
	}
	if meta {
		return "M-" + base
	}
	return base
}

// ---------------------------------------------------------------------------
// Introspection
// ---------------------------------------------------------------------------

// Bindings returns a shallow copy of this keymap's own bindings (does not
// include parent bindings).  The caller may safely iterate over the map.
func (km *Keymap) Bindings() map[ModKey]Binding {
	out := make(map[ModKey]Binding, len(km.bindings))
	maps.Copy(out, km.bindings)
	return out
}
