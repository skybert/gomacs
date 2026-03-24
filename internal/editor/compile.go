package editor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// compilationError records a file + line extracted from build output.
type compilationError struct {
	File string
	Line int
	Col  int
}

// errRe matches compiler/linter error lines of the form file:line: or file:line:col:
// Covers Go compiler, golangci-lint, staticcheck, and similar tools.
var errRe = regexp.MustCompile(`^([^:\s][^:]*):(\d+)(?::(\d+))?:`)

// defaultBuildCommand returns the suggested build command for dir.
// Returns "mvn clean install" if a pom.xml is present, otherwise "make".
func defaultBuildCommand(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "pom.xml")); err == nil {
		return "mvn clean install"
	}
	return "make"
}

// cmdBuild prompts for a build command (defaulting to "make" or
// "mvn clean install" for Maven projects) and runs it in the VC root
// (falling back to cwd). Output goes into a *compilation* buffer shown in a
// bottom split window (M-x build).
func (e *Editor) cmdBuild() {
	e.clearArg()

	be, root := vcFind(vcDir(e.ActiveBuffer()))
	var dir string
	if be != nil && root != "" {
		dir = root
	} else {
		var err error
		dir, err = filepath.Abs(".")
		if err != nil {
			e.Message("build: cannot determine working directory: %v", err)
			return
		}
	}

	def := defaultBuildCommand(dir)
	e.ReadMinibuffer("Build command: ", func(cmdStr string) {
		cmdStr = strings.TrimSpace(cmdStr)
		if cmdStr == "" {
			cmdStr = def
		}
		e.runBuild(dir, cmdStr)
	})
	// Pre-fill with the default command so the user can edit or accept it.
	e.minibufBuf.InsertString(0, def)
	e.minibufBuf.SetPoint(e.minibufBuf.Len())
}

// runBuild executes cmdStr in dir and streams the output into the
// *compilation* buffer.
func (e *Editor) runBuild(dir, cmdStr string) {
	// Get or create the *compilation* buffer.
	compBuf := e.FindBuffer("*compilation*")
	if compBuf == nil {
		compBuf = buffer.NewWithContent("*compilation*", "")
		e.buffers = append(e.buffers, compBuf)
	}
	compBuf.SetReadOnly(false)
	compBuf.Delete(0, compBuf.Len())
	compBuf.InsertString(0, "Running "+cmdStr+"…\n")
	compBuf.SetMode("compilation")
	compBuf.SetReadOnly(true)

	// Show the compilation buffer in a bottom split.
	e.showCompilationWindow(compBuf)

	e.Message("Running %s…", cmdStr)

	parts := strings.Fields(cmdStr)
	e.lspAsync(func() func() {
		var cmd *exec.Cmd
		if len(parts) == 1 {
			cmd = exec.CommandContext(context.Background(), parts[0]) //nolint:gosec
		} else {
			cmd = exec.CommandContext(context.Background(), parts[0], parts[1:]...) //nolint:gosec
		}
		cmd.Dir = dir
		out, runErr := cmd.CombinedOutput()
		exitOK := runErr == nil
		raw := string(out)
		plain, ansiSpans := syntax.ANSIParse(raw)

		// Parse error positions from the plain text.
		var errs []compilationError
		for line := range strings.SplitSeq(plain, "\n") {
			m := errRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			file := m[1]
			if !filepath.IsAbs(file) {
				file = filepath.Join(dir, file)
			}
			lineNum, _ := strconv.Atoi(m[2])
			col := 0
			if m[3] != "" {
				col, _ = strconv.Atoi(m[3])
			}
			errs = append(errs, compilationError{File: file, Line: lineNum, Col: col})
		}

		// Compute compilation coordinate highlights from the plain text.
		compSpans := syntax.CompilationHighlighter{}.Highlight(plain, 0, len([]rune(plain)))
		// Merge: ANSI spans first (higher priority via faceAtPos first-match).
		allSpans := append(ansiSpans, compSpans...) //nolint:gocritic

		return func() {
			compBuf.SetReadOnly(false)
			compBuf.Delete(0, compBuf.Len())
			compBuf.InsertString(0, plain)
			compBuf.SetReadOnly(true)
			// Record exit status for modeline colouring.
			e.compilationExitOK = &exitOK
			// Store combined highlighter so getSpanCache uses it.
			e.customHighlighters[compBuf] = syntax.ANSIHighlighter{Spans: allSpans}
			// Invalidate span cache so next render uses the new highlighter.
			delete(e.spanCaches, compBuf)
			// Auto-scroll to end.
			compBuf.SetPoint(compBuf.Len())
			for _, w := range e.windows {
				if w.Buf() == compBuf {
					w.SetBuf(compBuf)
				}
			}
			e.compilationErrors = errs
			e.compilationErrorIdx = -1
			if len(errs) == 0 {
				e.Message("Build finished with no errors")
			} else {
				e.Message("Build finished: %d error(s). M-g n / M-g p to navigate.", len(errs))
			}
		}
	})
}

// showCompilationWindow ensures the *compilation* buffer is visible in a
// bottom split window without changing the active (editing) window.
func (e *Editor) showCompilationWindow(compBuf *buffer.Buffer) {
	// Check if any window already shows the compilation buffer.
	for _, w := range e.windows {
		if w.Buf() == compBuf {
			return
		}
	}

	// If only one window, split it below.
	if len(e.windows) == 1 {
		w := e.windows[0]
		totalH := w.Height()
		if totalH < 6 {
			// Too small to split; just show in current window.
			e.activeWin.SetBuf(compBuf)
			return
		}
		topH := (totalH * 2) / 3
		botH := totalH - topH
		w.SetRegion(w.Top(), w.Left(), w.Width(), topH)
		newWin := window.New(compBuf, w.Top()+topH, w.Left(), w.Width(), botH)
		e.windows = append(e.windows, newWin)
		return
	}

	// Multiple windows: show compilation in the last window that isn't active.
	for i := len(e.windows) - 1; i >= 0; i-- {
		if e.windows[i] != e.activeWin {
			e.windows[i].SetBuf(compBuf)
			return
		}
	}
}

// compilationDispatch handles key events when the active buffer is *compilation*.
func (e *Editor) compilationDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune {
		return false
	}
	switch ke.Rune {
	case 'q':
		e.vcQuit("compilation")
		return true
	case 'g':
		e.cmdBuild()
		return true
	case 'n':
		e.cmdNextError()
		return true
	case 'p':
		e.cmdPreviousError()
		return true
	}
	return false
}
