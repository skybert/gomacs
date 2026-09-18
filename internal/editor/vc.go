package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---------------------------------------------------------------------------
// vcBackend interface and implementations
// ---------------------------------------------------------------------------

// vcBackend is the interface for a version control system backend.
type vcBackend interface {
	Name() string
	Root(dir string) string
	Status(root string) (string, error)
	Diff(root, filePath string) (string, error)
	DiffStaged(root, filePath string) (string, error)
	Log(root, filePath string) (string, error)
	Show(root, rev string) (string, error)
	ShowLog(root, rev string) (string, error)
	Grep(root, pattern string) (string, error)
	Blame(root, filePath string) (string, error)
	Revert(root, filePath string) error
	Unstage(root, filePath string) error
}

var vcBackends = []vcBackend{gitBackend{}}

func vcFind(dir string) (vcBackend, string) {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	for _, be := range vcBackends {
		if root := be.Root(dir); root != "" {
			return be, root
		}
	}
	return nil, ""
}

func vcDir(buf *buffer.Buffer) string {
	if f := buf.Filename(); f != "" {
		return filepath.Dir(f)
	}
	dir, _ := os.Getwd()
	return dir
}

// vcDirFor returns the directory a VC command run from buf should look for a
// backend in. VC output buffers (*vc-status*, *VC Log*, …) have no filename of
// their own, so the repository root recorded for them is preferred over the
// process working directory, which is some unrelated repository as often as not.
func (e *Editor) vcDirFor(buf *buffer.Buffer) string {
	if root := e.vcLogRoots[buf]; root != "" {
		return root
	}
	return vcDir(buf)
}

// ---------------------------------------------------------------------------
// gitBackend
// ---------------------------------------------------------------------------

type gitBackend struct{}

func (gitBackend) Name() string { return "git" }

func (gitBackend) Root(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (gitBackend) Status(root string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "status").CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Diff(root, filePath string) (string, error) {
	var cmd *exec.Cmd
	if filePath != "" {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--", filePath) //nolint:gosec
	} else {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "diff") //nolint:gosec
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (gitBackend) DiffStaged(root, filePath string) (string, error) {
	var cmd *exec.Cmd
	if filePath != "" {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--staged", "--", filePath) //nolint:gosec
	} else {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--staged") //nolint:gosec
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (gitBackend) Log(root, filePath string) (string, error) {
	args := []string{"-C", root, "log", "--oneline", "-50"}
	if filePath != "" {
		args = append(args, "--", filePath)
	}
	out, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Show(root, rev string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "show", rev).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) ShowLog(root, rev string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "show", "--no-patch", "--format=fuller", rev).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Grep(root, pattern string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "grep", "-n", pattern).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Blame(root, filePath string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "blame", "--date=short", "--abbrev=8", filePath).CombinedOutput() //nolint:gosec
	return string(out), err
}

func (gitBackend) Revert(root, filePath string) error {
	cmd := exec.CommandContext(context.Background(), "git", "-C", root, "restore", "--", filePath) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git restore: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (gitBackend) Unstage(root, filePath string) error {
	var cmd *exec.Cmd
	if filePath != "" {
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "restore", "--staged", "--", filePath) //nolint:gosec
	} else {
		// "git restore --staged" refuses to run without a pathspec, so name the
		// whole repository explicitly — ":/" is the top level regardless of cwd.
		cmd = exec.CommandContext(context.Background(), "git", "-C", root, "restore", "--staged", "--", ":/") //nolint:gosec
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git restore --staged: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared VC helpers
// ---------------------------------------------------------------------------

func (e *Editor) vcShowOutput(name, text, mode string) {
	b := e.FindBuffer(name)
	if b == nil {
		b = buffer.NewWithContent(name, text)
		e.buffers = append(e.buffers, b)
	} else {
		b.SetReadOnly(false)
		b.Delete(0, b.Len())
		b.InsertString(0, text)
	}
	b.SetMode(mode)
	b.SetReadOnly(true)
	b.SetPoint(0)
	e.showBuf(b)
}

// vcQuit switches away from the current VC output buffer to the most recently
// used buffer that isn't a VC output buffer of any kind (using bufferMRU),
// falling back to *scratch*.
func (e *Editor) vcQuit(skipMode string) {
	isVCMode := func(mode string) bool {
		switch mode {
		case "vc-log", "vc-status", "vc-grep", "diff", "vc-commit", "vc-show", "vc-fixup-select", "compilation", "lsp-refs":
			return true
		}
		return strings.HasPrefix(mode, "vc-annotate")
	}
	for _, b := range e.bufferMRU {
		if !isVCMode(b.Mode()) {
			e.activeWin.SetBuf(b)
			return
		}
	}
	cur := e.ActiveBuffer()
	for _, b := range e.buffers {
		if b != cur && !isVCMode(b.Mode()) {
			e.activeWin.SetBuf(b)
			return
		}
	}
	e.SwitchToBuffer("*scratch*")
}

// ---------------------------------------------------------------------------
// VC commands
// ---------------------------------------------------------------------------

const (
	// grepBufferName is the buffer vc-grep and project-grep present their hits
	// in; Emacs calls it *grep*.
	grepBufferName = "*grep*"
	// grepNoMatches is shown in grepBufferName when a search found nothing.
	grepNoMatches = "No matches found."
)

// cmdVcPrintLog shows the VCS log (C-x v l).
func (e *Editor) cmdVcPrintLog() {
	e.clearArg()
	buf := e.ActiveBuffer()
	be, root := vcFind(e.vcDirFor(buf))
	if be == nil {
		e.Message("vc-print-log: not in a version control repository")
		return
	}
	text, err := be.Log(root, buf.Filename())
	if err != nil && text == "" {
		text = err.Error()
	}
	e.vcShowOutput("*VC Log*", text, "vc-log")
	logBuf := e.ActiveBuffer()
	e.vcLogRoots[logBuf] = root
	e.vcLogFiles[logBuf] = buf.Filename()
}

// cmdVcDiff shows uncommitted changes for the current file (C-x v =).
func (e *Editor) cmdVcDiff() {
	e.clearArg()
	buf := e.ActiveBuffer()
	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-diff: not in a version control repository")
		return
	}
	text, err := be.Diff(root, buf.Filename())
	if err != nil && text == "" {
		text = err.Error()
	}
	if text == "" {
		e.Message("vc-diff: no uncommitted changes")
		return
	}
	e.vcShowOutput("*vc-diff*", text, "diff")
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// cmdVcStatus runs the VCS status command (C-x v s).
func (e *Editor) cmdVcStatus() {
	e.clearArg()
	be, root := vcFind(e.vcDirFor(e.ActiveBuffer()))
	if be == nil {
		e.Message("vc-status: not in a version control repository")
		return
	}
	text, err := be.Status(root)
	if err != nil && text == "" {
		text = err.Error()
	}
	e.vcShowOutput("*vc-status*", text, "vc-status")
	e.vcLogRoots[e.ActiveBuffer()] = root
}

// cmdVcGrep prompts for a pattern and shows grep results (C-x v G).
func (e *Editor) cmdVcGrep() {
	e.clearArg()
	be, root := vcFind(e.vcDirFor(e.ActiveBuffer()))
	if be == nil {
		e.Message("vc-grep: not in a version control repository")
		return
	}
	e.ReadMinibuffer(be.Name()+" grep: ", func(pattern string) {
		if pattern == "" {
			return
		}
		text, err := be.Grep(root, pattern)
		if err != nil && text == "" {
			text = grepNoMatches
		}
		if text == "" {
			text = grepNoMatches
		}
		e.vcShowGrepResults(text, root)
	})
}

// cmdProjectGrep prompts for a pattern and searches the project. Uses the VC
// backend's grep if one is available, otherwise falls back to grep -R -i -n
// (C-x p g).
func (e *Editor) cmdProjectGrep() {
	e.clearArg()
	// Search from the current buffer's own directory, not the process working
	// directory; bufferDir also covers dired buffers, which have no filename.
	dir := strings.TrimSuffix(e.bufferDir(e.ActiveBuffer()), "/")
	be, root := vcFind(dir)
	prompt := "Project grep: "
	if be != nil {
		prompt = be.Name() + " grep: "
	} else {
		// Without a backend the buffer's directory is the project root, so the
		// hits resolve against the directory that was actually searched.
		root = dir
	}
	e.ReadMinibuffer(prompt, func(pattern string) {
		if pattern == "" {
			return
		}
		var text string
		if be != nil {
			var err error
			text, err = be.Grep(root, pattern)
			if err != nil && text == "" {
				text = grepNoMatches
			}
		} else {
			// No VC backend — use plain grep. Run it inside root with "." as the
			// search path so the hits come back root-relative, exactly like the
			// backends' own grep output.
			cmd := exec.CommandContext(context.Background(), "grep", "-R", "-i", "-n", pattern, ".")
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			text = string(out)
			if err != nil && text == "" {
				text = grepNoMatches
			}
		}
		if text == "" {
			text = grepNoMatches
		}
		e.vcShowGrepResults(text, root)
	})
}

// vcShowGrepResults presents grep output in the *grep* buffer and feeds the
// hits into the next-error list, so C-x ` / M-g n / M-g p walk through them
// just like they do for *compilation* errors.
func (e *Editor) vcShowGrepResults(text, root string) {
	e.vcShowOutput(grepBufferName, text, "vc-grep")
	if root != "" {
		e.vcLogRoots[e.ActiveBuffer()] = root
	}
	hits := parseGrepHits(text, root)
	e.compilationErrors = hits
	e.compilationErrorIdx = -1
	if len(hits) > 0 {
		e.Message("%d hit(s) — C-x ` or M-g n to cycle", len(hits))
	}
}

// parseGrepHits parses "file:line:content" grep output into compilationError
// entries. Relative paths are resolved against root; absolute paths (as
// produced by grep -R over an absolute directory) are kept as they are. Lines
// without a positive line number — headers, "No matches found.", binary-file
// notices — are skipped. Content containing further colons is left alone.
func parseGrepHits(output, root string) []compilationError {
	var hits []compilationError
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil || lineNum < 1 {
			continue
		}
		file := strings.TrimPrefix(parts[0], "./")
		if file == "" {
			continue
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, file)
		}
		hits = append(hits, compilationError{File: file, Line: lineNum})
	}
	return hits
}

// cmdVcRevert reverts the current file to its last committed version (C-x v u).
// Prompts for confirmation before discarding changes.
func (e *Editor) cmdVcRevert() {
	e.clearArg()
	buf := e.ActiveBuffer()
	filePath := buf.Filename()
	if filePath == "" {
		e.Message("vc-revert: buffer has no associated file")
		return
	}
	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-revert: not in a version control repository")
		return
	}

	// Get the diff first; if there's nothing to revert, say so and stop.
	text, err := be.Diff(root, filePath)
	if err != nil && text == "" {
		e.Message("vc-revert: %v", err)
		return
	}
	if text == "" {
		e.Message("vc-revert: no uncommitted changes in %s", filepath.Base(filePath))
		return
	}

	// Show the diff in a bottom split so the user can see what will be lost.
	diffBuf := e.FindBuffer("*vc-diff*")
	if diffBuf == nil {
		diffBuf = buffer.NewWithContent("*vc-diff*", text)
		e.buffers = append(e.buffers, diffBuf)
	} else {
		diffBuf.SetReadOnly(false)
		diffBuf.Delete(0, diffBuf.Len())
		diffBuf.InsertString(0, text)
	}
	diffBuf.SetMode("diff")
	diffBuf.SetReadOnly(true)
	diffBuf.SetPoint(0)
	e.showCompilationWindow(diffBuf)

	// Prompt with the source buffer still active. One keystroke answers it;
	// anything other than y cancels.
	e.Message("Revert %s (discard changes shown above)? (y/n)", filepath.Base(filePath))
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		// Close the diff split we opened.
		e.removeWindowShowingBuf(diffBuf)

		if r != 'y' && r != 'Y' {
			e.Message("Revert cancelled")
			return
		}
		if err := be.Revert(root, filePath); err != nil {
			e.Message("vc-revert: %v", err)
			return
		}
		// Reload the buffer from disk.
		data, err := os.ReadFile(filePath) //nolint:gosec
		if err != nil {
			e.Message("vc-revert: reverted on disk but could not re-read: %v", err)
			return
		}
		buf.SetReadOnly(false)
		pt := buf.Point()
		buf.Delete(0, buf.Len())
		buf.InsertString(0, string(data))
		buf.SetModified(false)
		buf.SetPoint(min(pt, buf.Len()))
		e.Message("Reverted %s", filepath.Base(filePath))
	}
}

// ---------------------------------------------------------------------------
// VC next-action (C-x v v)
// ---------------------------------------------------------------------------

// cmdVcNextAction advances the file through the version control state machine.
func (e *Editor) cmdVcNextAction() {
	e.clearArg()
	buf := e.ActiveBuffer()
	filePath := buf.Filename()

	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-next-action: not in a version control repository")
		return
	}

	var args []string
	if filePath != "" {
		args = []string{"-C", root, "status", "--porcelain", filePath}
	} else {
		args = []string{"-C", root, "status", "--porcelain"}
	}
	out, err := exec.CommandContext(context.Background(), "git", args...).Output() //nolint:gosec
	if err != nil {
		e.Message("vc-next-action: git status failed: %v", err)
		return
	}
	status := strings.TrimSpace(string(out))

	if status == "" {
		e.Message("vc-next-action: nothing to commit for %s", filepath.Base(filePath))
		return
	}

	xy := ""
	if len(status) >= 2 {
		xy = status[:2]
	}
	x := ""
	if len(xy) >= 1 {
		x = string(xy[0])
	}

	switch {
	case xy == "??":
		e.vcGitAdd(root, filePath)
	case x == " " || x == "!":
		e.vcGitAdd(root, filePath)
	default:
		e.vcOpenCommitBuffer(root, filePath)
	}
}

func (e *Editor) vcGitAdd(root, filePath string) {
	var args []string
	if filePath != "" {
		args = []string{"-C", root, "add", filePath}
	} else {
		args = []string{"-C", root, "add", "."}
	}
	if out, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput(); err != nil { //nolint:gosec
		e.Message("git add failed: %s", strings.TrimSpace(string(out)))
		return
	}
	if filePath != "" {
		e.Message("Staged %s", filepath.Base(filePath))
	} else {
		e.Message("Staged all changes")
	}
}

func (e *Editor) vcOpenCommitBuffer(root, filePath string) {
	if filePath != "" {
		_ = exec.CommandContext(context.Background(), "git", "-C", root, "add", filePath).Run() //nolint:gosec
	}

	nameOut, _ := exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--cached", "--name-status").Output() //nolint:gosec
	statOut, _ := exec.CommandContext(context.Background(), "git", "-C", root, "diff", "--cached", "--stat").Output()        //nolint:gosec

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("# Changes to be committed:\n")
	for _, line := range strings.Split(strings.TrimRight(string(nameOut), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		statusCode := ""
		path := line
		if len(parts) == 2 {
			switch parts[0] {
			case "M":
				statusCode = "modified:   "
			case "A":
				statusCode = "new file:   "
			case "D":
				statusCode = "deleted:    "
			case "R":
				statusCode = "renamed:    "
			default:
				statusCode = parts[0] + ":         "
			}
			path = parts[1]
		}
		sb.WriteString("#\t" + statusCode + path + "\n")
	}
	for _, line := range strings.Split(strings.TrimRight(string(statOut), "\n"), "\n") {
		if line != "" && strings.Contains(line, "changed") {
			sb.WriteString("# " + strings.TrimSpace(line) + "\n")
		}
	}
	sb.WriteString("#\n")
	sb.WriteString("# C-c C-c  commit    C-c C-k  abort\n")

	b := e.FindBuffer("*vc-commit*")
	if b == nil {
		b = buffer.NewWithContent("*vc-commit*", sb.String())
		e.buffers = append(e.buffers, b)
	} else {
		b.SetReadOnly(false)
		b.Delete(0, b.Len())
		b.InsertString(0, sb.String())
	}
	b.SetMode("vc-commit")
	b.SetReadOnly(false)
	b.SetPoint(0)
	e.vcCommitRoots[b] = root
	e.activeWin.SetBuf(b)
}

// vcCommitDispatch intercepts C-c C-c (submit) and C-c C-k (abort) in *vc-commit* buffers.
func (e *Editor) vcCommitDispatch(ke terminal.KeyEvent) bool {
	if e.prefixKeymap != e.ctrlCKeymap {
		return false
	}
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyCtrlC {
		return false
	}
	if ke.Key == tcell.KeyCtrlC {
		e.vcCommitSubmit()
		e.prefixKeymap = nil
		return true
	}
	if ke.Key == tcell.KeyRune && ke.Rune == 'k' {
		e.vcCommitAbort()
		e.prefixKeymap = nil
		return true
	}
	return false
}

func (e *Editor) vcCommitSubmit() {
	buf := e.ActiveBuffer()
	root := e.vcCommitRoots[buf]
	if root == "" {
		e.Message("vc-commit: no repository root found")
		return
	}
	full := buf.String()
	var msgLines []string
	for _, line := range strings.Split(full, "\n") {
		if !strings.HasPrefix(line, "#") {
			msgLines = append(msgLines, line)
		}
	}
	msg := strings.TrimSpace(strings.Join(msgLines, "\n"))
	if msg == "" {
		e.Message("Aborting commit: empty commit message")
		return
	}
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "commit", "-m", msg).CombinedOutput() //nolint:gosec
	if err != nil {
		e.Message("git commit failed: %s", strings.TrimSpace(string(out)))
		return
	}
	e.Message("Committed: %s", strings.TrimSpace(string(out)))
	e.vcQuit("vc-commit")
}

func (e *Editor) vcCommitAbort() {
	e.Message("Commit aborted")
	e.vcQuit("vc-commit")
}

// ---------------------------------------------------------------------------
// VC key dispatch functions
// ---------------------------------------------------------------------------

// vcFixupSelectDispatch handles keys in a *VC Fixup Select* buffer.
// The user navigates to the target commit and presses C-c C-c to create a
// fixup commit (git commit --fixup=<sha>).
func (e *Editor) vcFixupSelectDispatch(ke terminal.KeyEvent) bool {
	// C-c C-c: run git commit --fixup=<sha> for commit at point.
	// C-c C-k: cancel.
	if e.prefixKeymap == e.ctrlCKeymap {
		if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyCtrlC {
			return false
		}
		if ke.Key == tcell.KeyRune && ke.Rune == 'k' {
			e.prefixKeymap = nil
			e.Message("Fixup cancelled")
			buf := e.ActiveBuffer()
			if parent, ok := e.vcParent[buf]; ok {
				e.activeWin.SetBuf(parent)
			} else {
				e.vcQuit("vc-fixup-select")
			}
			return true
		}
		if ke.Key == tcell.KeyCtrlC {
			e.prefixKeymap = nil
			buf := e.ActiveBuffer()
			root := e.vcLogRoots[buf]
			if root == "" {
				e.Message("vc-fixup: no repository root")
				return true
			}
			pt := buf.Point()
			bol := buf.BeginningOfLine(pt)
			eol := buf.EndOfLine(pt)
			line := buf.Substring(bol, eol)
			fields := strings.Fields(line)
			if len(fields) == 0 {
				e.Message("vc-fixup: no commit on this line")
				return true
			}
			sha := fields[0]
			out, err := exec.CommandContext(context.Background(), "git", "-C", root, "commit", "--fixup="+sha).CombinedOutput() //nolint:gosec
			if err != nil {
				e.Message("git commit --fixup failed: %s", strings.TrimSpace(string(out)))
				return true
			}
			e.Message("Fixup commit created: %s", strings.TrimSpace(string(out)))
			// Return to parent vc-status buffer.
			if parent, ok := e.vcParent[buf]; ok {
				e.activeWin.SetBuf(parent)
			} else {
				e.vcQuit("vc-fixup-select")
			}
			return true
		}
		return false
	}

	if ke.Key != tcell.KeyRune {
		return false
	}
	switch ke.Rune {
	case 'q':
		buf := e.ActiveBuffer()
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
		} else {
			e.vcQuit("vc-fixup-select")
		}
		return true
	case 'n':
		e.cmdNextLine()
		return true
	case 'p':
		e.cmdPreviousLine()
		return true
	}
	return false
}

// vcLogDispatch handles keys in a *VC Log* buffer.
func (e *Editor) vcLogDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()
	root := e.vcLogRoots[buf]

	switch {
	case ke.Key == tcell.KeyRune && ke.Rune == 'q':
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
			return true
		}
		e.vcQuit("vc-log")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'n':
		e.cmdNextLine()
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'p':
		e.cmdPreviousLine()
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'g':
		if root == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		filePath := e.vcLogFiles[buf]
		text, err := be.Log(root, filePath)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Log*", text, "vc-log")
		logBuf := e.ActiveBuffer()
		e.vcLogRoots[logBuf] = root
		e.vcLogFiles[logBuf] = filePath
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'l':
		if root == "" {
			return true
		}
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		eol := buf.EndOfLine(pt)
		line := buf.Substring(bol, eol)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.ShowLog(root, fields[0])
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Log Message*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'd', ke.Key == tcell.KeyEnter:
		if root == "" {
			return true
		}
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		eol := buf.EndOfLine(pt)
		line := buf.Substring(bol, eol)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.Show(root, fields[0])
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Show*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true
	}
	return false
}

// vcDiffDispatch handles keys in any "diff" or "vc-show" mode buffer.
func (e *Editor) vcDiffDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	switch {
	case ke.Key == tcell.KeyRune && ke.Rune == 'q':
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
			return true
		}
		e.vcQuit("diff")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'n':
		pt := buf.Point()
		eol := buf.EndOfLine(pt)
		search := eol + 1
		n := buf.Len()
		for search < n {
			bol := search
			eol2 := buf.EndOfLine(bol)
			line := buf.Substring(bol, eol2)
			if strings.HasPrefix(line, "@@") {
				buf.SetPoint(bol)
				return true
			}
			search = eol2 + 1
		}
		e.Message("No next hunk")
		return true

	case ke.Key == tcell.KeyRune && ke.Rune == 'p':
		pt := buf.Point()
		bol := buf.BeginningOfLine(pt)
		search := bol - 1
		for search > 0 {
			bol2 := buf.BeginningOfLine(search)
			eol2 := buf.EndOfLine(bol2)
			line := buf.Substring(bol2, eol2)
			if strings.HasPrefix(line, "@@") {
				buf.SetPoint(bol2)
				return true
			}
			search = bol2 - 1
		}
		e.Message("No previous hunk")
		return true

	case ke.Key == tcell.KeyEnter:
		return e.vcDiffGotoSource(buf)
	}
	return false
}

func (e *Editor) vcDiffGotoSource(buf *buffer.Buffer) bool {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	curLine := buf.Substring(bol, eol)

	if len(curLine) == 0 {
		return true
	}
	first := curLine[0]
	if first != '+' && first != '-' {
		return true
	}
	if strings.HasPrefix(curLine, "+++") || strings.HasPrefix(curLine, "---") {
		return true
	}

	root := e.vcLogRoots[buf]
	if root == "" {
		return true
	}

	allText := buf.Substring(0, eol)
	lines := strings.Split(allText, "\n")
	curIdx := len(lines) - 1

	filePath := ""
	newFileLineNum := 0

	// Find the nearest hunk header (@@ …) above the current line; it carries
	// the new-file starting line for the hunk.
	hunkIdx := -1
	for i := curIdx - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "@@ ") {
			hunkIdx = i
			break
		}
	}
	if hunkIdx >= 0 {
		fields := strings.Fields(lines[hunkIdx])
		if len(fields) >= 3 {
			newPart := strings.TrimPrefix(fields[2], "+")
			newPart = strings.Split(newPart, ",")[0]
			start, _ := strconv.Atoi(newPart)
			// The current line's new-file number is the hunk start plus the
			// count of lines present in the new file (i.e. everything except
			// deletions) between the header and the current line.
			newLine := start
			for j := hunkIdx + 1; j < curIdx; j++ {
				if !strings.HasPrefix(lines[j], "-") {
					newLine++
				}
			}
			newFileLineNum = newLine
		}
		// The file a hunk belongs to is named by the nearest "+++ b/…" header
		// above the hunk (the +++ line sits above the @@, so we must keep
		// scanning past the hunk header to find it).
		for i := hunkIdx - 1; i >= 0; i-- {
			if strings.HasPrefix(lines[i], "+++ ") {
				rel := strings.TrimPrefix(lines[i][4:], "b/")
				filePath = filepath.Join(root, rel)
				break
			}
		}
	}

	if filePath == "" || newFileLineNum == 0 {
		e.Message("Cannot determine source location")
		return true
	}

	b, err := e.loadFile(filePath)
	if err != nil {
		e.Message("Cannot open %s: %v", filePath, err)
		return true
	}
	e.activeWin.SetBuf(b)
	pos := b.LineStart(newFileLineNum)
	b.SetPoint(pos)
	return true
}

// vcStatusDispatch handles keys in a *VC Status* buffer.
// vcStatusFileAtPoint returns the relative path of the file on the current
// line of a *vc-status* buffer, or "" if the line does not refer to an
// existing file under root.
func vcStatusFileAtPoint(buf *buffer.Buffer, root string) string {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	rel := strings.TrimSuffix(fields[len(fields)-1], ":")
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		return ""
	}
	return rel
}

// vcGitAddModified stages modifications and deletions of files git already
// tracks. Untracked files are deliberately left alone.
func vcGitAddModified(root string) error {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "add", "-u").CombinedOutput() //nolint:gosec
	if err != nil {
		return fmt.Errorf("git add -u: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// vcGitAddUntracked stages every file git reports as untracked, honouring
// .gitignore. "git add -u" does not cover these, so they are listed explicitly
// and handed to "git add" — that way no tracked modification is staged by
// accident either.
func vcGitAddUntracked(root string) error {
	listOut, err := exec.CommandContext(context.Background(), "git", "-C", root, "ls-files", "--others", "--exclude-standard").Output() //nolint:gosec
	if err != nil {
		return fmt.Errorf("git ls-files --others: %w", err)
	}
	var files []string
	for f := range strings.SplitSeq(strings.TrimSpace(string(listOut)), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return errNoUntrackedFiles
	}
	args := append([]string{"-C", root, "add", "--"}, files...)
	out, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput() //nolint:gosec
	if err != nil {
		return fmt.Errorf("git add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// errNoUntrackedFiles is reported when the untracked section is empty by the
// time the user confirms staging it.
var errNoUntrackedFiles = errors.New("no untracked files to stage")

// vcStatusStageSection handles `c` pressed on a *vc-status* section header. It
// asks whether to stage exactly what that section lists — tracked
// modifications for "Changes not staged for commit", untracked files for
// "Untracked files" — and on confirmation stages them and opens the commit
// buffer. Returns false when the current line is not a header it handles, so
// the caller can fall through to opening the commit buffer directly.
func (e *Editor) vcStatusStageSection(buf *buffer.Buffer, root string) bool {
	pt := buf.Point()
	line := strings.ToLower(buf.Substring(buf.BeginningOfLine(pt), buf.EndOfLine(pt)))

	var prompt string
	var stage func(string) error
	switch {
	case strings.Contains(line, "untracked"):
		prompt = "Stage all untracked files and commit? (y/n)"
		stage = vcGitAddUntracked
	case strings.Contains(line, "not staged"):
		prompt = "Stage all modified tracked files and commit? (y/n)"
		stage = vcGitAddModified
	default:
		return false
	}

	e.Message("%s", prompt)
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		if r != 'y' && r != 'Y' {
			e.Message("Commit cancelled")
			return
		}
		if err := stage(root); err != nil {
			e.Message("vc-status: %v", err)
			return
		}
		e.vcOpenCommitBuffer(root, "")
	}
	return true
}

func (e *Editor) vcStatusDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()
	root := e.vcLogRoots[buf]

	if ke.Key == tcell.KeyRune && ke.Rune == 'q' {
		e.vcQuit("vc-status")
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 'g' {
		if root == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.Status(root)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*vc-status*", text, "vc-status")
		e.vcLogRoots[e.ActiveBuffer()] = root
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 'l' {
		if root == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.Log(root, "")
		if err != nil && text == "" {
			text = err.Error()
		}
		statusBuf := buf
		e.vcShowOutput("*VC Log*", text, "vc-log")
		logBuf := e.ActiveBuffer()
		e.vcLogRoots[logBuf] = root
		e.vcLogFiles[logBuf] = ""
		e.vcParent[logBuf] = statusBuf
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 'd' {
		if root == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		relPath := vcStatusFileAtPoint(buf, root)
		absPath := ""
		if relPath != "" {
			absPath = filepath.Join(root, relPath)
		}
		// Try unstaged diff first; fall back to staged diff.
		text, err := be.Diff(root, absPath)
		if err != nil && text == "" {
			text = err.Error()
		}
		if text == "" {
			text, err = be.DiffStaged(root, absPath)
			if err != nil && text == "" {
				text = err.Error()
			}
		}
		if text == "" {
			e.Message("vc-status: no uncommitted changes")
			return true
		}
		statusBuf := buf
		e.vcShowOutput("*vc-diff*", text, "diff")
		diffBuf := e.ActiveBuffer()
		e.vcLogRoots[diffBuf] = root
		e.vcParent[diffBuf] = statusBuf
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 'u' {
		if root == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		relPath := vcStatusFileAtPoint(buf, root)
		absPath := ""
		if relPath != "" {
			absPath = filepath.Join(root, relPath)
		}
		if err := be.Unstage(root, absPath); err != nil {
			e.Message("vc-status: %v", err)
			return true
		}
		text, err := be.Status(root)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*vc-status*", text, "vc-status")
		e.vcLogRoots[e.ActiveBuffer()] = root
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 's' {
		if root == "" {
			return true
		}
		relPath := vcStatusFileAtPoint(buf, root)
		absPath := ""
		if relPath != "" {
			absPath = filepath.Join(root, relPath)
		}
		e.vcGitAdd(root, absPath)
		be, _ := vcFind(root)
		if be != nil {
			text, err := be.Status(root)
			if err != nil && text == "" {
				text = err.Error()
			}
			e.vcShowOutput("*vc-status*", text, "vc-status")
			e.vcLogRoots[e.ActiveBuffer()] = root
		}
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 'f' {
		if root == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		statusBuf := buf
		text, err := be.Log(root, "")
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Fixup Select*", text, "vc-fixup-select")
		fixupBuf := e.ActiveBuffer()
		e.vcLogRoots[fixupBuf] = root
		e.vcParent[fixupBuf] = statusBuf
		e.Message("C-c C-c: fixup into commit at point  q: abort")
		return true
	}

	if ke.Key == tcell.KeyRune && ke.Rune == 'c' {
		if root == "" {
			return true
		}
		// If point is on a header line (no file), offer to stage everything the
		// section under that header lists before opening the commit buffer.
		if vcStatusFileAtPoint(buf, root) == "" && e.vcStatusStageSection(buf, root) {
			return true
		}
		e.vcOpenCommitBuffer(root, "")
		return true
	}

	if ke.Key != tcell.KeyEnter {
		return false
	}

	if root == "" {
		return true
	}
	filePath := vcStatusFileAtPoint(buf, root)
	if filePath == "" {
		return true
	}
	abs := filepath.Join(root, filePath)
	b, err := e.loadFile(abs)
	if err != nil {
		e.Message("Cannot open %s: %v", abs, err)
		return true
	}
	e.activeWin.SetBuf(b)
	return true
}

// vcGrepDispatch handles keys in a *grep* buffer.
func (e *Editor) vcGrepDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	if ke.Key == tcell.KeyRune && ke.Rune == 'q' {
		e.vcQuit("vc-grep")
		return true
	}

	if ke.Key != tcell.KeyEnter {
		return false
	}

	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	if line == "" {
		return true
	}

	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 2 {
		return true
	}
	relPath := parts[0]
	lineNum, err := strconv.Atoi(parts[1])
	if err != nil || lineNum < 1 {
		return true
	}

	root := e.vcLogRoots[buf]
	if root == "" {
		return true
	}
	// grep may report absolute paths; only relative ones need the root prefix.
	abs := relPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, relPath)
	}
	b, loadErr := e.loadFile(abs)
	if loadErr != nil {
		e.Message("Cannot open %s: %v", abs, loadErr)
		return true
	}
	e.activeWin.SetBuf(b)
	pos := b.LineStart(lineNum)
	b.SetPoint(pos)
	return true
}

// ---------------------------------------------------------------------------
// VC annotate (git blame)
// ---------------------------------------------------------------------------

// langForExt maps a file extension to a language mode name.
func langForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".md", ".markdown":
		return "markdown"
	case ".el":
		return "elisp"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".sh", ".bash":
		return "bash"
	case ".pl", ".pm", ".t":
		return "perl"
	case ".feature":
		return "gherkin"
	case ".conf", ".toml":
		return "conf"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".mk":
		return "makefile"
	default:
		return ""
	}
}

// cmdVcAnnotate runs git blame on the current file (C-x v g).
func (e *Editor) cmdVcAnnotate() {
	e.clearArg()
	buf := e.ActiveBuffer()
	filePath := buf.Filename()
	if filePath == "" {
		e.Message("vc-annotate: buffer is not visiting a file")
		return
	}
	be, root := vcFind(vcDir(buf))
	if be == nil {
		e.Message("vc-annotate: not in a version control repository")
		return
	}
	text, err := be.Blame(root, filePath)
	if err != nil && text == "" {
		text = err.Error()
	}
	mode := "vc-annotate"
	if lang := langForExt(filepath.Ext(filePath)); lang != "" {
		mode = "vc-annotate+" + lang
	}
	e.vcShowOutput("*vc-annotate*", text, mode)
	annotateBuf := e.ActiveBuffer()
	e.vcLogRoots[annotateBuf] = root
	e.vcParent[annotateBuf] = buf
}

func (e *Editor) vcAnnotateHashAtPoint(buf *buffer.Buffer) string {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[0], "^")
}

// vcAnnotateDispatch handles key events in a *vc-annotate* buffer.
func (e *Editor) vcAnnotateDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune {
		return false
	}

	buf := e.ActiveBuffer()
	root := e.vcLogRoots[buf]

	switch ke.Rune {
	case 'q':
		if parent, ok := e.vcParent[buf]; ok {
			e.activeWin.SetBuf(parent)
			return true
		}
		e.vcQuit("vc-annotate")
		return true

	case 'l':
		hash := e.vcAnnotateHashAtPoint(buf)
		if hash == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.ShowLog(root, hash)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*VC Log Message*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true

	case 'd':
		hash := e.vcAnnotateHashAtPoint(buf)
		if hash == "" {
			return true
		}
		be, _ := vcFind(root)
		if be == nil {
			return true
		}
		text, err := be.Show(root, hash)
		if err != nil && text == "" {
			text = err.Error()
		}
		e.vcShowOutput("*vc-diff*", text, "vc-show")
		e.vcLogRoots[e.ActiveBuffer()] = root
		e.vcParent[e.ActiveBuffer()] = buf
		return true
	}
	return false
}
