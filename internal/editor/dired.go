package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
)

// diredState holds per-buffer dired metadata.
type diredState struct {
	dir   string          // absolute directory path
	marks map[string]rune // filename → mark ('D' for delete, '*' for general)
}

// cmdDired opens or refreshes a Dired buffer for a directory (C-x d).
func (e *Editor) cmdDired() {
	e.clearArg()
	defaultDir := e.bufferDir(e.ActiveBuffer())
	e.ReadMinibuffer("Dired (directory): ", func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			path = defaultDir
		}
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = home + path[1:]
			}
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			// Treat as file path if not a directory.
			b, ferr := e.loadFile(path)
			if ferr != nil {
				e.Message("Error: %v", ferr)
				return
			}
			e.activeWin.SetBuf(b)
			return
		}
		e.openDired(path)
	})
	if defaultDir != "" {
		e.minibufBuf.InsertString(0, defaultDir)
		e.minibufBuf.SetPoint(e.minibufBuf.Len())
	}
	e.SetMinibufCompletions(filePathCompletions)
	e.SetMinibufPreferTyped(func(s string) bool {
		_, err := os.Stat(s)
		return err == nil
	})
}

// openDired creates (or reuses) a dired buffer for dir and makes it active.
func (e *Editor) openDired(dir string) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	bufName := fmt.Sprintf("*dired:%s*", absDir)
	b := e.FindBuffer(bufName)
	if b == nil {
		b = buffer.New(bufName)
		b.SetMode("dired")
		b.SetReadOnly(true)
		e.buffers = append(e.buffers, b)
	}
	ds := e.diredStates[b]
	if ds == nil {
		ds = &diredState{dir: absDir, marks: make(map[string]rune)}
		e.diredStates[b] = ds
	}
	ds.dir = absDir
	e.diredRefresh(b, ds)
	e.activeWin.SetBuf(b)
}

// diredRefresh rebuilds the dired buffer content from the filesystem.
func (e *Editor) diredRefresh(b *buffer.Buffer, ds *diredState) {
	entries, err := os.ReadDir(ds.dir)
	if err != nil {
		b.SetReadOnly(false)
		b.Delete(0, b.Len())
		b.InsertString(0, fmt.Sprintf("Error reading directory: %v\n", err))
		b.SetReadOnly(true)
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "  %s:\n\n", ds.dir)

	// Sort: directories first, then files, each group alphabetically.
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		mark := ' '
		if m, ok := ds.marks[entry.Name()]; ok {
			mark = rune(m)
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		// Show '>' in the mark column when a buffer for this file is open.
		if mark == ' ' {
			absPath, aerr := filepath.Abs(filepath.Join(ds.dir, entry.Name()))
			if aerr == nil {
				for _, ob := range e.buffers {
					if ob.Filename() == absPath {
						mark = '>'
						break
					}
				}
			}
		}
		modTime := info.ModTime().Format("Jan _2  2006")
		if info.ModTime().Year() == time.Now().Year() {
			modTime = info.ModTime().Format("Jan _2 15:04")
		}
		size := info.Size()
		mode := info.Mode().String()
		fmt.Fprintf(&sb, "%c %s %8d %s %s\n",
			mark, mode, size, modTime, name)
	}

	b.SetReadOnly(false)
	b.Delete(0, b.Len())
	b.InsertString(0, sb.String())
	b.SetPoint(0)
	// Move point to first file entry (skip header lines).
	pos := 0
	lineCount := 0
	for pos < b.Len() && lineCount < 2 {
		if b.RuneAt(pos) == '\n' {
			lineCount++
		}
		pos++
	}
	b.SetPoint(pos)
	b.SetReadOnly(true)
}

// diredCurrentFile returns the filename on the current line of a dired buffer.
func (e *Editor) diredCurrentFile(b *buffer.Buffer, ds *diredState) (string, bool) {
	pt := b.Point()
	bol := b.BeginningOfLine(pt)
	eol := b.EndOfLine(pt)
	line := b.Substring(bol, eol)
	if len([]rune(line)) < 2 {
		return "", false
	}
	// Format: "M perm     size  date  name"
	// Last field is the filename; split on whitespace.
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return "", false
	}
	name := fields[len(fields)-1]
	// Strip trailing slash for directories.
	name = strings.TrimSuffix(name, "/")
	if name == "" || name == ".." || name == "." {
		return "", false
	}
	return name, true
}

// diredDispatch handles a key press in a dired buffer.
// Returns true if the key was consumed.
func (e *Editor) diredDispatch(ke terminal.KeyEvent) bool {
	buf := e.ActiveBuffer()
	ds := e.diredStates[buf]
	if ds == nil {
		return false
	}

	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}
	// Pass through any modified key (M-x, C-x, etc.) to normal dispatch.
	if ke.Mod != 0 {
		return false
	}

	// Enter opens the entry under point, same as 'f'.
	if ke.Key == tcell.KeyEnter {
		name, ok := e.diredCurrentFile(buf, ds)
		if !ok {
			return true
		}
		full := filepath.Join(ds.dir, name)
		info, err := os.Stat(full)
		if err == nil && info.IsDir() {
			e.openDired(full)
		} else {
			b, err := e.loadFile(full)
			if err != nil {
				e.Message("Error: %v", err)
			} else {
				e.activeWin.SetBuf(b)
			}
		}
		return true
	}

	switch ke.Rune {
	case 'n', ' ':
		e.execCommand("next-line")
		return true

	case 'p':
		e.execCommand("previous-line")
		return true

	case 'f', 'e':
		name, ok := e.diredCurrentFile(buf, ds)
		if !ok {
			return true
		}
		full := filepath.Join(ds.dir, name)
		info, err := os.Stat(full)
		if err == nil && info.IsDir() {
			e.openDired(full)
		} else {
			b, err := e.loadFile(full)
			if err != nil {
				e.Message("Error: %v", err)
			} else {
				e.activeWin.SetBuf(b)
			}
		}
		return true

	case 'd':
		// Mark for deletion.
		name, ok := e.diredCurrentFile(buf, ds)
		if ok {
			ds.marks[name] = 'D'
			e.diredRefresh(buf, ds)
			// Move to next line.
			e.execCommand("next-line")
		}
		return true

	case 'u':
		// Unmark.
		name, ok := e.diredCurrentFile(buf, ds)
		if ok {
			delete(ds.marks, name)
			e.diredRefresh(buf, ds)
		}
		return true

	case 'x':
		// Execute deletions.
		toDelete := make([]string, 0)
		for name, mark := range ds.marks {
			if mark == 'D' {
				toDelete = append(toDelete, name)
			}
		}
		if len(toDelete) == 0 {
			e.Message("No files marked for deletion")
			return true
		}
		sort.Strings(toDelete)
		e.Message("Delete %s? (y/n)", strings.Join(toDelete, ", "))
		e.readCharPending = true
		e.readCharCallback = func(r rune) {
			if r != 'y' && r != 'Y' {
				e.Message("Deletion cancelled")
				return
			}
			deleted := 0
			for _, name := range toDelete {
				full := filepath.Join(ds.dir, name)
				if err := os.RemoveAll(full); err != nil {
					e.Message("Error deleting %s: %v", name, err)
					return
				}
				delete(ds.marks, name)
				deleted++
			}
			e.diredRefresh(buf, ds)
			e.Message("Deleted %d file(s)", deleted)
		}
		return true

	case 'g':
		// Refresh.
		e.diredRefresh(buf, ds)
		e.Message("Refreshed %s", ds.dir)
		return true

	case '^':
		// Go to parent directory.
		parent := filepath.Dir(ds.dir)
		if parent != ds.dir {
			e.openDired(parent)
		}
		return true

	case 'q':
		// Quit dired: switch to previous buffer.
		for _, b := range e.buffers {
			if b != buf && b.Mode() != "dired" {
				e.activeWin.SetBuf(b)
				return true
			}
		}
		e.SwitchToBuffer("*scratch*")
		return true

	case 'o':
		// Open in other window.
		name, ok := e.diredCurrentFile(buf, ds)
		if !ok {
			return true
		}
		full := filepath.Join(ds.dir, name)
		b, err := e.loadFile(full)
		if err != nil {
			e.Message("Error: %v", err)
			return true
		}
		if len(e.windows) == 1 {
			e.execCommand("split-window-below")
		}
		e.execCommand("other-window")
		e.activeWin.SetBuf(b)
		return true
	}

	return false
}
