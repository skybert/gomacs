package editor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/syntax"
)

// manPageCache holds the lazily computed list of all available man page names.
var (
	manPageCacheOnce  sync.Once
	manPageCacheNames []string
)

// manPageNames returns a sorted, deduplicated list of all man page names found
// in the directories reported by the `manpath` command (falling back to common
// default paths when manpath is unavailable).
func manPageNames() []string {
	manPageCacheOnce.Do(func() {
		dirs := manpathDirs()
		seen := make(map[string]struct{})
		for _, dir := range dirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, section := range entries {
				if !section.IsDir() || !strings.HasPrefix(section.Name(), "man") {
					continue
				}
				sectionDir := filepath.Join(dir, section.Name())
				pages, err := os.ReadDir(sectionDir)
				if err != nil {
					continue
				}
				for _, page := range pages {
					name := page.Name()
					// Strip compression suffix then section suffix: foo.1.gz → foo
					name = strings.TrimSuffix(name, ".gz")
					name = strings.TrimSuffix(name, ".bz2")
					name = strings.TrimSuffix(name, ".xz")
					if dot := strings.LastIndex(name, "."); dot > 0 {
						name = name[:dot]
					}
					seen[name] = struct{}{}
				}
			}
		}
		names := make([]string, 0, len(seen))
		for n := range seen {
			names = append(names, n)
		}
		sort.Strings(names)
		manPageCacheNames = names
	})
	return manPageCacheNames
}

// manpathDirs returns the list of man page root directories.
func manpathDirs() []string {
	out, err := exec.Command("manpath").Output() //nolint:gosec
	if err == nil {
		raw := strings.TrimSpace(string(out))
		if raw != "" {
			return strings.Split(raw, ":")
		}
	}
	// Fall back to common locations.
	return []string{"/usr/share/man", "/usr/local/share/man", "/opt/homebrew/share/man"}
}

// manCompletions returns man page names matching query, sorted by fuzzy score.
func manCompletions(query string) []string {
	all := manPageNames()
	if query == "" {
		return all
	}
	type scored struct {
		name  string
		score int
	}
	var matches []scored
	for _, name := range all {
		if !fuzzyMatch(name, query) {
			continue
		}
		matches = append(matches, scored{name, fuzzyScore(name, query)})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return matches[i].name < matches[j].name
	})
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.name
	}
	return out
}

// cmdMan prompts for a man page topic and displays it in a *Man <topic>* buffer.
func (e *Editor) cmdMan() {
	e.clearArg()
	e.ReadMinibuffer("Man page: ", func(topic string) {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return
		}
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "man", topic) //nolint:gosec
		cmd.Env = append(cmd.Environ(), "MANPAGER=cat", "MANWIDTH=80")
		out, err := cmd.CombinedOutput()
		raw := string(out)
		if err != nil && raw == "" {
			e.Message("man: no entry for %s", topic)
			return
		}

		plain, spans := syntax.ManParse(raw)

		name := "*Man " + topic + "*"
		manBuf := e.FindBuffer(name)
		if manBuf == nil {
			manBuf = buffer.NewWithContent(name, plain)
			e.buffers = append(e.buffers, manBuf)
		} else {
			manBuf.SetReadOnly(false)
			manBuf.Delete(0, manBuf.Len())
			manBuf.InsertString(0, plain)
		}
		manBuf.SetMode("man")
		manBuf.SetReadOnly(true)
		manBuf.SetPoint(0)
		e.customHighlighters[manBuf] = syntax.ANSIHighlighter{Spans: spans}
		delete(e.spanCaches, manBuf)
		e.showBuf(manBuf)
	})
	e.SetMinibufCompletions(manCompletions)
}

// cmdShellCommand runs a shell command and shows output (M-!).
func (e *Editor) cmdShellCommand() {
	e.clearArg()
	e.ReadMinibuffer("Shell command: ", func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return
		}
		ctx := context.Background()
		out, err := shellRun(ctx, cmd, "")
		result := out
		if err != nil && result == "" {
			result = err.Error()
		}
		outBuf := e.FindBuffer("*Shell Command Output*")
		if outBuf == nil {
			outBuf = buffer.NewWithContent("*Shell Command Output*", result)
			e.buffers = append(e.buffers, outBuf)
		} else {
			outBuf.Delete(0, outBuf.Len())
			outBuf.InsertString(0, result)
		}
		outBuf.SetPoint(0)
		e.activeWin.SetBuf(outBuf)
	})
}

// cmdShellCommandOnRegion pipes the region through a shell command (M-|).
func (e *Editor) cmdShellCommandOnRegion() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	e.ReadMinibuffer("Shell command on region: ", func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return
		}
		buf := e.ActiveBuffer()
		start, end := regionBounds(buf)
		if start == end {
			e.Message("No region")
			return
		}
		input := buf.Substring(start, end)
		ctx := context.Background()
		result, err := shellRun(ctx, cmd, input)
		if err != nil && result == "" {
			e.Message("Shell error: %v", err)
			return
		}
		buf.Delete(start, end-start)
		buf.InsertString(start, result)
		buf.SetPoint(start + len([]rune(result)))
		buf.SetMarkActive(false)
		e.Message("Shell command done")
	})
}

// shellRun runs cmd via sh -c with optional stdin text, returns combined output.
func shellRun(ctx context.Context, cmd, stdin string) (string, error) {
	sh := exec.CommandContext(ctx, "sh", "-c", cmd) //nolint:gosec
	if stdin != "" {
		sh.Stdin = strings.NewReader(stdin)
	}
	out, err := sh.CombinedOutput()
	return string(out), err
}
