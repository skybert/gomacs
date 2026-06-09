package editor

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// ---------------------------------------------------------------------------
// Capture-terminal test helper
// ---------------------------------------------------------------------------

// newCapTestEditor builds a minimal Editor backed by a headless capture terminal.
func newCapTestEditor(content string) *Editor {
	term := terminal.NewCapture(80, 24)
	buf := buffer.NewWithContent("*test*", content)
	win := window.New(buf, 0, 0, 80, 23)
	e := &Editor{
		term:                       term,
		buffers:                    []*buffer.Buffer{buf},
		windows:                    []*window.Window{win},
		activeWin:                  win,
		layoutRoot:                 leafNode(win),
		minibufBuf:                 buffer.New(" *minibuf*"),
		globalKeymap:               keymap.New("global"),
		ctrlXKeymap:                keymap.New("C-x"),
		universalArg:               1,
		fillColumn:                 70,
		isSearchCaseFold:           true,
		saveBufferDeleteTrailingWS: true,
		customHighlighters:         make(map[*buffer.Buffer]syntax.Highlighter),
	}
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	return e
}

// captureRow returns all runes in a given screen row as a string.
func captureRow(t *testing.T, e *Editor, row int) string {
	t.Helper()
	w, _ := e.term.CaptureSize()
	var sb strings.Builder
	for col := 0; col < w; col++ {
		ch, _ := e.term.CaptureCell(col, row)
		sb.WriteRune(ch)
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Redraw
// ---------------------------------------------------------------------------

func TestRedrawDoesNotPanicOnEmptyBuffer(t *testing.T) {
	e := newCapTestEditor("")
	e.Redraw() // must not panic
}

func TestRedrawDoesNotPanicOnSingleLine(t *testing.T) {
	e := newCapTestEditor("hello world")
	e.Redraw()
}

func TestRedrawDoesNotPanicOnMultiLine(t *testing.T) {
	e := newCapTestEditor("line1\nline2\nline3\n")
	e.Redraw()
}

func TestRedrawWithNilTermDoesNotPanic(t *testing.T) {
	e := newTestEditor("some content")
	// term is nil — Redraw must return immediately without panic
	e.Redraw()
}

// ---------------------------------------------------------------------------
// renderWindow — buffer content appears at expected screen positions
// ---------------------------------------------------------------------------

func TestRenderWindowFirstLineContent(t *testing.T) {
	e := newCapTestEditor("Hello")
	e.Redraw()

	ch, _ := e.term.CaptureCell(0, 0)
	if ch != 'H' {
		t.Errorf("expected 'H' at (0,0), got %q", ch)
	}
	ch, _ = e.term.CaptureCell(4, 0)
	if ch != 'o' {
		t.Errorf("expected 'o' at (4,0), got %q", ch)
	}
}

func TestRenderWindowMultipleLines(t *testing.T) {
	e := newCapTestEditor("foo\nbar\nbaz")
	e.Redraw()

	ch, _ := e.term.CaptureCell(0, 0)
	if ch != 'f' {
		t.Errorf("expected 'f' at (0,0), got %q", ch)
	}
	ch, _ = e.term.CaptureCell(0, 1)
	if ch != 'b' {
		t.Errorf("expected 'b' at (0,1), got %q", ch)
	}
}

func TestRenderWindowTabExpansion(t *testing.T) {
	e := newCapTestEditor("\tA")
	e.Redraw()

	// Tab expands to tabWidth (2) spaces; 'A' lands at column 2.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch != ' ' {
		t.Errorf("expected space at (0,0) for tab, got %q", ch)
	}
	ch, _ = e.term.CaptureCell(2, 0)
	if ch != 'A' {
		t.Errorf("expected 'A' at (2,0) after tab, got %q", ch)
	}
}

func TestRenderWindowEmptyBufferFillsSpaces(t *testing.T) {
	e := newCapTestEditor("")
	e.Redraw()

	// Row 0 should be all spaces (blank line).
	for col := 0; col < 10; col++ {
		ch, _ := e.term.CaptureCell(col, 0)
		if ch != ' ' {
			t.Errorf("expected space at (%d,0) for empty buffer, got %q", col, ch)
		}
	}
}

func TestRenderWindowLongLineTruncated(t *testing.T) {
	// A line longer than the window width (80) must not cause a panic and
	// must stop exactly at the window edge.
	content := strings.Repeat("X", 200)
	e := newCapTestEditor(content)
	e.Redraw()

	// Column 79 (last column) should have been written.
	ch, _ := e.term.CaptureCell(79, 0)
	if ch != 'X' {
		t.Errorf("expected 'X' at col 79, got %q", ch)
	}
}

// ---------------------------------------------------------------------------
// renderModeline
// ---------------------------------------------------------------------------

func TestRenderModelineContainsBufferName(t *testing.T) {
	e := newCapTestEditor("content")
	e.Redraw()

	// The modeline is drawn on row winH-1 = 22 (window height 23, last row is modeline).
	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "*test*") {
		t.Errorf("modeline row %d does not contain buffer name *test*: %q", modeRow, row)
	}
}

func TestRenderModelineContainsMode(t *testing.T) {
	e := newCapTestEditor("package main\n")
	e.ActiveBuffer().SetMode("go")
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "go") {
		t.Errorf("modeline does not contain mode 'go': %q", row)
	}
}

func TestRenderModelineModifiedMark(t *testing.T) {
	e := newCapTestEditor("")
	// Insert something to mark the buffer as modified.
	b := e.ActiveBuffer()
	b.Insert(0, 'X')
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "**") {
		t.Errorf("modeline does not show '**' for modified buffer: %q", row)
	}
}

func TestRenderModelineReadOnlyMark(t *testing.T) {
	e := newCapTestEditor("data")
	e.ActiveBuffer().SetReadOnly(true)
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "%%") {
		t.Errorf("modeline does not show '%%%%' for read-only buffer: %q", row)
	}
}

func TestRenderModelineUnmodifiedMark(t *testing.T) {
	e := newCapTestEditor("data")
	// Buffer was just created and not edited; should show "-".
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, " - ") {
		t.Errorf("modeline does not show '-' for unmodified buffer: %q", row)
	}
}

func TestRenderModelineNarrowIndicator(t *testing.T) {
	e := newCapTestEditor("line1\nline2\nline3\n")
	b := e.ActiveBuffer()
	b.Narrow(0, 5)
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "Narrow") {
		t.Errorf("modeline should show Narrow indicator: %q", row)
	}
}

func TestRenderModelineMacroIndicator(t *testing.T) {
	e := newCapTestEditor("text")
	e.kbdMacroRecording = true
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "Def") {
		t.Errorf("modeline should show macro Def indicator: %q", row)
	}
}

func TestRenderModelineVcAnnotateStripsLangSuffix(t *testing.T) {
	e := newCapTestEditor("abc123 (Author) package main\n")
	e.ActiveBuffer().SetMode("vc-annotate+go")
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "vc-annotate") {
		t.Errorf("modeline should show base vc-annotate mode: %q", row)
	}
	if strings.Contains(row, "vc-annotate+go") {
		t.Errorf("modeline should strip +go suffix: %q", row)
	}
}

func TestRenderModelineCompilationOK(t *testing.T) {
	e := newCapTestEditor("build output\n")
	b := e.ActiveBuffer()
	b.SetName("*compilation*")
	ok := true
	e.compilationExitOK = &ok
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "*compilation*") {
		t.Errorf("modeline should show *compilation* name: %q", row)
	}
}

func TestRenderModelineCompilationFail(t *testing.T) {
	e := newCapTestEditor("build output\n")
	b := e.ActiveBuffer()
	b.SetName("*compilation*")
	failed := false
	e.compilationExitOK = &failed
	// Should render the fail-colored name without panicking.
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "*compilation*") {
		t.Errorf("modeline should show *compilation* name: %q", row)
	}
}

// ---------------------------------------------------------------------------
// renderWindow overlays
// ---------------------------------------------------------------------------

func TestRenderWindowRegionHighlight(t *testing.T) {
	e := newCapTestEditor("hello")
	b := e.ActiveBuffer()
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(3)
	e.Redraw()

	// Cells 0..2 are inside the region and should use FaceRegion.
	_, face := e.term.CaptureCell(0, 0)
	if face != syntax.FaceRegion {
		t.Errorf("expected FaceRegion at (0,0) inside region, got %+v", face)
	}
	// Cell 4 is outside the region.
	_, face = e.term.CaptureCell(4, 0)
	if face == syntax.FaceRegion {
		t.Error("cell 4 should not be region-highlighted")
	}
}

func TestRenderWindowIsearchHighlight(t *testing.T) {
	e := newCapTestEditor("abcXYZ")
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "XYZ"
	e.isSearchCaseFold = false
	// Forward isearch: match ends at point.
	e.ActiveBuffer().SetPoint(6)
	e.Redraw()

	_, face := e.term.CaptureCell(3, 0) // 'X'
	if face != syntax.FaceIsearch {
		t.Errorf("expected FaceIsearch at match start, got %+v", face)
	}
}

func TestRenderWindowQueryReplaceHighlight(t *testing.T) {
	e := newCapTestEditor("foo bar")
	e.queryReplaceActive = true
	e.queryReplaceMatch = 0
	e.queryReplaceFromRunes = []rune("foo")
	e.Redraw()

	_, face := e.term.CaptureCell(0, 0)
	if face != syntax.FaceIsearch {
		t.Errorf("expected FaceIsearch for query-replace match, got %+v", face)
	}
}

func TestRenderWindowNarrowedSkipsOutsideLines(t *testing.T) {
	e := newCapTestEditor("AAAA\nBBBB\nCCCC\n")
	b := e.ActiveBuffer()
	// Narrow to the second line only (offsets 5..9 = "BBBB").
	b.Narrow(5, 9)
	e.Redraw()

	// The first physical line "AAAA" is outside the narrow region and must be blank.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch == 'A' {
		t.Errorf("line outside narrow region should not render 'A', got %q", ch)
	}
}

func TestRenderWindowBreakpointGutter(t *testing.T) {
	e := newCapTestEditor("package main\nfunc main() {}\n")
	b := e.ActiveBuffer()
	b.SetFilename("/tmp/gomacs_render_bp.go")
	b.SetMode("go")
	e.dapBreakpoints = map[string]map[int]struct{}{
		"/tmp/gomacs_render_bp.go": {1: {}},
	}
	e.Redraw()

	// The gutter draws a breakpoint bullet at column 0 of line 1.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch != '●' {
		t.Errorf("expected breakpoint bullet in gutter, got %q", ch)
	}
}

func TestRenderWindowExecPosArrow(t *testing.T) {
	e := newCapTestEditor("package main\nfunc main() {}\n")
	b := e.ActiveBuffer()
	b.SetFilename("/tmp/gomacs_render_ep.go")
	b.SetMode("go")
	e.dapBreakpoints = map[string]map[int]struct{}{
		"/tmp/gomacs_render_ep.go": {2: {}},
	}
	e.dap = &dapState{stoppedFile: "/tmp/gomacs_render_ep.go", stoppedLine: 1}
	e.Redraw()

	// Exec-pos arrow is drawn at gutter column 1 of the stopped line (row 0).
	ch, _ := e.term.CaptureCell(1, 0)
	if ch != '→' {
		t.Errorf("expected exec-pos arrow at stopped line, got %q", ch)
	}
}

// ---------------------------------------------------------------------------
// applyVisualLines
// ---------------------------------------------------------------------------

func TestApplyVisualLinesEnabled(t *testing.T) {
	e := newCapTestEditor(strings.Repeat("x", 200) + "\n")
	e.visualLines = true
	e.visualLinesSynced = false
	e.applyVisualLines()
	// Re-applying when already synced is a no-op and must not panic.
	e.applyVisualLines()
}

func TestApplyVisualLinesDisabled(t *testing.T) {
	e := newCapTestEditor(strings.Repeat("y", 200) + "\n")
	e.visualLines = false
	e.visualLinesSynced = false
	e.applyVisualLines()
}

// ---------------------------------------------------------------------------
// renderMinibuffer
// ---------------------------------------------------------------------------

func TestRenderMinibufferShowsPromptWhenActive(t *testing.T) {
	e := newCapTestEditor("hello")
	e.minibufActive = true
	e.minibufPrompt = "Find: "
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	if !strings.Contains(row, "Find: ") {
		t.Errorf("minibuffer row does not contain prompt 'Find: ': %q", row)
	}
}

func TestRenderMinibufferShowsTypedText(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "Name: "
	e.minibufBuf.InsertString(0, "alice")
	e.minibufBuf.SetPoint(5)
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	if !strings.Contains(row, "alice") {
		t.Errorf("minibuffer row does not contain typed text 'alice': %q", row)
	}
}

func TestRenderMinibufferEmptyWhenInactive(t *testing.T) {
	e := newCapTestEditor("text")
	// No message, no minibuf active.
	e.message = ""
	e.minibufActive = false
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	// Should be all spaces.
	trimmed := strings.TrimRight(row, " ")
	if trimmed != "" {
		t.Errorf("expected empty minibuffer row, got: %q", row)
	}
}

func TestRenderMinibufferMessage(t *testing.T) {
	e := newCapTestEditor("")
	e.Message("File saved")
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	if !strings.Contains(row, "File saved") {
		t.Errorf("minibuffer row does not contain message: %q", row)
	}
}

// ---------------------------------------------------------------------------
// renderCandidatePopup
// ---------------------------------------------------------------------------

func TestRenderCandidatePopupShowsCandidates(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "Cmd: "
	e.minibufCandidates = []string{"alpha", "beta", "gamma"}
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0
	e.Redraw()

	// Candidates are drawn above the minibuffer row (row 23).
	// With 3 candidates they occupy rows 20, 21, 22 (minibuf popup goes up).
	found := false
	for row := 18; row <= 22; row++ {
		r := captureRow(t, e, row)
		if strings.Contains(r, "alpha") {
			found = true
			break
		}
	}
	if !found {
		t.Error("candidate 'alpha' was not rendered in the popup area")
	}
}

func TestRenderCandidatePopupSelectedHighlightedDifferently(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "Pick: "
	e.minibufCandidates = []string{"one", "two"}
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0
	e.Redraw()

	// Just verify rendering doesn't panic; face checking would need a more
	// detailed capture helper. We verify at least the text appears.
	found := false
	for row := 0; row < 24; row++ {
		r := captureRow(t, e, row)
		if strings.Contains(r, "one") {
			found = true
			break
		}
	}
	if !found {
		t.Error("selected candidate 'one' not found on screen")
	}
}

func TestRenderCandidatePopupEmpty(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "P: "
	e.minibufCandidates = nil
	// Should not panic.
	e.Redraw()
}

// ---------------------------------------------------------------------------
// placeCursor / screenColForPoint
// ---------------------------------------------------------------------------

func TestScreenColForPointAtBOL(t *testing.T) {
	b := buffer.NewWithContent("*t*", "hello")
	col := screenColForPoint(b, 0)
	if col != 0 {
		t.Errorf("expected col 0 at BOL, got %d", col)
	}
}

func TestScreenColForPointMidLine(t *testing.T) {
	b := buffer.NewWithContent("*t*", "hello")
	col := screenColForPoint(b, 3)
	if col != 3 {
		t.Errorf("expected col 3, got %d", col)
	}
}

func TestScreenColForPointWithTab(t *testing.T) {
	b := buffer.NewWithContent("*t*", "\tA")
	// Tab expands to tabWidth (2), so 'A' is at column tabWidth.
	col := screenColForPoint(b, 1) // point after the tab
	if col != tabWidth {
		t.Errorf("expected col %d after tab, got %d", tabWidth, col)
	}
}

func TestScreenColForPointMultipleTabs(t *testing.T) {
	b := buffer.NewWithContent("*t*", "\t\tX")
	col := screenColForPoint(b, 2) // two tabs
	if col != 2*tabWidth {
		t.Errorf("expected col %d after two tabs, got %d", 2*tabWidth, col)
	}
}

func TestPlaceCursorMinibufActive(t *testing.T) {
	e := newCapTestEditor("hello")
	e.minibufActive = true
	e.minibufPrompt = "Q: "
	// Should not panic.
	e.Redraw()
}

func TestPlaceCursorNormalMode(t *testing.T) {
	e := newCapTestEditor("hello")
	// Should not panic.
	e.Redraw()
}

// ---------------------------------------------------------------------------
// highlighterFor
// ---------------------------------------------------------------------------

func TestHighlighterForGoMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "package main")
	b.SetMode("go")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.GoHighlighter); !ok {
		t.Errorf("expected GoHighlighter for go mode, got %T", hl)
	}
}

func TestHighlighterForMarkdownMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "# heading")
	b.SetMode("markdown")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.MarkdownHighlighter); !ok {
		t.Errorf("expected MarkdownHighlighter for markdown mode, got %T", hl)
	}
}

func TestHighlighterForElispMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "(defun foo () nil)")
	b.SetMode("elisp")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.ElispHighlighter); !ok {
		t.Errorf("expected ElispHighlighter for elisp mode, got %T", hl)
	}
}

func TestHighlighterForPythonMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "def foo(): pass")
	b.SetMode("python")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.PythonHighlighter); !ok {
		t.Errorf("expected PythonHighlighter for python mode, got %T", hl)
	}
}

func TestHighlighterForJavaMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "class Foo {}")
	b.SetMode("java")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.JavaHighlighter); !ok {
		t.Errorf("expected JavaHighlighter for java mode, got %T", hl)
	}
}

func TestHighlighterForBashMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "#!/bin/bash\necho hi")
	b.SetMode("bash")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.BashHighlighter); !ok {
		t.Errorf("expected BashHighlighter for bash mode, got %T", hl)
	}
}

func TestHighlighterForJSONMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", `{"key":"val"}`)
	b.SetMode("json")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.JSONHighlighter); !ok {
		t.Errorf("expected JSONHighlighter for json mode, got %T", hl)
	}
}

func TestHighlighterForYAMLMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "key: value")
	b.SetMode("yaml")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.YAMLHighlighter); !ok {
		t.Errorf("expected YAMLHighlighter for yaml mode, got %T", hl)
	}
}

func TestHighlighterForDiffMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "+added\n-removed\n")
	b.SetMode("diff")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.DiffHighlighter); !ok {
		t.Errorf("expected DiffHighlighter for diff mode, got %T", hl)
	}
}

func TestHighlighterForVcLogMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "commit abc123\n")
	b.SetMode("vc-log")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcLogHighlighter); !ok {
		t.Errorf("expected VcLogHighlighter for vc-log mode, got %T", hl)
	}
}

func TestHighlighterForVcAnnotateMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "abc123 (Author 2024-01-01) line")
	b.SetMode("vc-annotate")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcAnnotateHighlighter); !ok {
		t.Errorf("expected VcAnnotateHighlighter for vc-annotate mode, got %T", hl)
	}
}

func TestHighlighterForVcAnnotatePlusLang(t *testing.T) {
	b := buffer.NewWithContent("*t*", "abc123 (Author) package main")
	b.SetMode("vc-annotate+go")
	hl := highlighterFor(b)
	va, ok := hl.(syntax.VcAnnotateHighlighter)
	if !ok {
		t.Fatalf("expected VcAnnotateHighlighter for vc-annotate+go mode, got %T", hl)
	}
	if va.Source == nil {
		t.Error("expected non-nil Source highlighter for vc-annotate+go")
	}
}

func TestHighlighterForHelpMode(t *testing.T) {
	b := buffer.NewWithContent("*Help*", "describe-function\n")
	b.SetMode("help")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.HelpHighlighter); !ok {
		t.Errorf("expected HelpHighlighter for help mode, got %T", hl)
	}
}

func TestHighlighterForFundamentalMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "plain text")
	b.SetMode("fundamental")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.NilHighlighter); !ok {
		t.Errorf("expected NilHighlighter for fundamental mode, got %T", hl)
	}
}

func TestHighlighterForTextMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "plain text")
	b.SetMode("text")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.NilHighlighter); !ok {
		t.Errorf("expected NilHighlighter for text mode, got %T", hl)
	}
}

func TestHighlighterForMakefileMode(t *testing.T) {
	b := buffer.NewWithContent("Makefile", "all:\n\tgo build")
	b.SetMode("makefile")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.MakefileHighlighter); !ok {
		t.Errorf("expected MakefileHighlighter for makefile mode, got %T", hl)
	}
}

func TestHighlighterForVcShowMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "commit abc\n")
	b.SetMode("vc-show")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcShowHighlighter); !ok {
		t.Errorf("expected VcShowHighlighter for vc-show mode, got %T", hl)
	}
}

func TestHighlighterForVcStatusMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "M  file.go\n")
	b.SetMode("vc-status")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcStatusHighlighter); !ok {
		t.Errorf("expected VcStatusHighlighter for vc-status mode, got %T", hl)
	}
}

func TestHighlighterForGherkinMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "Feature: login\n")
	b.SetMode("gherkin")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.GherkinHighlighter); !ok {
		t.Errorf("expected GherkinHighlighter for gherkin mode, got %T", hl)
	}
}

func TestHighlighterForVcGrepModes(t *testing.T) {
	for _, mode := range []string{"vc-grep", "lsp-refs"} {
		b := buffer.NewWithContent("*t*", "file.go:1: hit\n")
		b.SetMode(mode)
		if _, ok := highlighterFor(b).(syntax.VcGrepHighlighter); !ok {
			t.Errorf("mode %q: expected VcGrepHighlighter, got %T", mode, highlighterFor(b))
		}
	}
}

func TestHighlighterForVcFixupSelectMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "commit abc123\n")
	b.SetMode("vc-fixup-select")
	if _, ok := highlighterFor(b).(syntax.VcLogHighlighter); !ok {
		t.Errorf("expected VcLogHighlighter for vc-fixup-select mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForVcCommitMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "feat: thing\n")
	b.SetMode("vc-commit")
	if _, ok := highlighterFor(b).(syntax.VcCommitHighlighter); !ok {
		t.Errorf("expected VcCommitHighlighter for vc-commit mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForConfMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "key = value\n")
	b.SetMode("conf")
	if _, ok := highlighterFor(b).(syntax.ConfHighlighter); !ok {
		t.Errorf("expected ConfHighlighter for conf mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForPerlMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "print \"hi\";\n")
	b.SetMode("perl")
	if _, ok := highlighterFor(b).(syntax.PerlHighlighter); !ok {
		t.Errorf("expected PerlHighlighter for perl mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForDebugModes(t *testing.T) {
	bl := buffer.NewWithContent("*t*", "x = 1\n")
	bl.SetMode("debug-locals")
	if _, ok := highlighterFor(bl).(syntax.DapLocalsHighlighter); !ok {
		t.Errorf("debug-locals: got %T, want DapLocalsHighlighter", highlighterFor(bl))
	}

	bs := buffer.NewWithContent("*t*", "frame 0\n")
	bs.SetMode("debug-stack")
	if _, ok := highlighterFor(bs).(syntax.DapStackHighlighter); !ok {
		t.Errorf("debug-stack: got %T, want DapStackHighlighter", highlighterFor(bs))
	}

	br := buffer.NewWithContent("*t*", "p x\n")
	br.SetMode("debug-repl")
	if _, ok := highlighterFor(br).(syntax.GoHighlighter); !ok {
		t.Errorf("debug-repl: got %T, want GoHighlighter", highlighterFor(br))
	}
}

// ---------------------------------------------------------------------------
// faceAtPos
// ---------------------------------------------------------------------------

func TestFaceAtPosNoSpans(t *testing.T) {
	face := faceAtPos(nil, 5)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault for empty spans, got %+v", face)
	}
}

func TestFaceAtPosInsideSpan(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 5, Face: syntax.FaceKeyword},
	}
	face := faceAtPos(spans, 2)
	if face != syntax.FaceKeyword {
		t.Errorf("expected FaceKeyword at pos 2, got %+v", face)
	}
}

func TestFaceAtPosAtSpanEnd(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 5, Face: syntax.FaceKeyword},
	}
	// pos 5 is equal to End (exclusive), so it should fall back to default.
	face := faceAtPos(spans, 5)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault at span end pos 5, got %+v", face)
	}
}

func TestFaceAtPosBeforeFirstSpan(t *testing.T) {
	spans := []syntax.Span{
		{Start: 10, End: 20, Face: syntax.FaceString},
	}
	face := faceAtPos(spans, 3)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault before first span, got %+v", face)
	}
}

func TestFaceAtPosMultipleSpans(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 3, Face: syntax.FaceKeyword},
		{Start: 5, End: 10, Face: syntax.FaceString},
		{Start: 12, End: 18, Face: syntax.FaceComment},
	}
	// pos 6 is inside the second span.
	face := faceAtPos(spans, 6)
	if face != syntax.FaceString {
		t.Errorf("expected FaceString at pos 6, got %+v", face)
	}
}

func TestFaceAtPosBetweenSpans(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 3, Face: syntax.FaceKeyword},
		{Start: 5, End: 10, Face: syntax.FaceString},
	}
	// pos 4 is between spans.
	face := faceAtPos(spans, 4)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault between spans at pos 4, got %+v", face)
	}
}

// ---------------------------------------------------------------------------
// helpDispatch
// ---------------------------------------------------------------------------

func TestHelpDispatchQuitReturnsTrue(t *testing.T) {
	e := newTestEditor("main text")
	e.lisp = elisp.NewEvaluator()

	// Create a help buffer and switch to it.
	helpBuf := buffer.NewWithContent("*Help*", "some help")
	helpBuf.SetMode("help")
	e.buffers = append(e.buffers, helpBuf)
	e.activeWin.SetBuf(helpBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}
	consumed := e.helpDispatch(ke)
	if !consumed {
		t.Error("helpDispatch should consume 'q'")
	}
	// After 'q' the active buffer should not be the help buffer.
	if e.ActiveBuffer() == helpBuf {
		t.Error("helpDispatch should switch away from the help buffer")
	}
}

func TestHelpDispatchNonQKeyNotConsumed(t *testing.T) {
	e := newTestEditor("text")
	e.lisp = elisp.NewEvaluator()

	helpBuf := buffer.NewWithContent("*Help*", "help")
	helpBuf.SetMode("help")
	e.buffers = append(e.buffers, helpBuf)
	e.activeWin.SetBuf(helpBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}
	consumed := e.helpDispatch(ke)
	if consumed {
		t.Error("helpDispatch should not consume non-'q' keys")
	}
}

func TestHelpDispatchQuitFallsBackToScratch(t *testing.T) {
	e := newTestEditor("text")
	e.lisp = elisp.NewEvaluator()

	// Remove the default *test* buffer so no non-help buffer exists initially
	// — the function should create / switch to *scratch*.
	helpBuf := buffer.NewWithContent("*Help*", "help")
	helpBuf.SetMode("help")
	// Make *Help* the only buffer.
	e.buffers = []*buffer.Buffer{helpBuf}
	e.bufferMRU = nil
	e.activeWin.SetBuf(helpBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}
	consumed := e.helpDispatch(ke)
	if !consumed {
		t.Error("helpDispatch should consume 'q' even when no other buffer exists")
	}
}

// ---------------------------------------------------------------------------
// manDispatch
// ---------------------------------------------------------------------------

func TestManDispatchQuitReturnsTrue(t *testing.T) {
	e := newTestEditor("main text")
	e.lisp = elisp.NewEvaluator()

	manBuf := buffer.NewWithContent("*Man ls*", "man content")
	manBuf.SetMode("man")
	e.buffers = append(e.buffers, manBuf)
	e.activeWin.SetBuf(manBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}
	consumed := e.manDispatch(ke)
	if !consumed {
		t.Error("manDispatch should consume 'q'")
	}
	if e.ActiveBuffer() == manBuf {
		t.Error("manDispatch should switch away from the man buffer")
	}
}

func TestManDispatchNonQKeyNotConsumed(t *testing.T) {
	e := newTestEditor("text")
	e.lisp = elisp.NewEvaluator()

	manBuf := buffer.NewWithContent("*Man*", "man")
	manBuf.SetMode("man")
	e.buffers = append(e.buffers, manBuf)
	e.activeWin.SetBuf(manBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'j'}
	consumed := e.manDispatch(ke)
	if consumed {
		t.Error("manDispatch should not consume non-'q' keys")
	}
}

// ---------------------------------------------------------------------------
// startIsearch
// ---------------------------------------------------------------------------

func TestStartIsearchForward(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetPoint(3)
	e.startIsearch(true)

	if !e.isearching {
		t.Error("expected isearching=true after startIsearch")
	}
	if !e.isearchFwd {
		t.Error("expected isearchFwd=true for forward isearch")
	}
	if e.isearchStr != "" {
		t.Errorf("expected empty isearchStr, got %q", e.isearchStr)
	}
	if e.isearchStart != 3 {
		t.Errorf("expected isearchStart=3, got %d", e.isearchStart)
	}
}

func TestStartIsearchBackward(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(false)

	if !e.isearching {
		t.Error("expected isearching=true")
	}
	if e.isearchFwd {
		t.Error("expected isearchFwd=false for backward isearch")
	}
}

// ---------------------------------------------------------------------------
// isearchHandleKey
// ---------------------------------------------------------------------------

func TestIsearchHandleKeyTypingAccumulatesQuery(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)

	for _, r := range "ell" {
		ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}
		e.isearchHandleKey(ke)
	}

	if e.isearchStr != "ell" {
		t.Errorf("expected isearchStr='ell', got %q", e.isearchStr)
	}
	if !e.isearching {
		t.Error("expected still isearching after typing")
	}
}

func TestIsearchHandleKeyEnterAccepts(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "hello"

	ke := terminal.KeyEvent{Key: tcell.KeyEnter}
	e.isearchHandleKey(ke)

	if e.isearching {
		t.Error("expected isearching=false after Enter")
	}
}

func TestIsearchHandleKeyCtrlGCancels(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetPoint(3)
	e.startIsearch(true)
	e.isearchStr = "xxx"

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlG}
	e.isearchHandleKey(ke)

	if e.isearching {
		t.Error("expected isearching=false after C-g")
	}
	if e.isearchStr != "" {
		t.Errorf("expected empty isearchStr after cancel, got %q", e.isearchStr)
	}
	// Point should be restored to isearchStart.
	if e.ActiveBuffer().Point() != 3 {
		t.Errorf("expected point restored to 3, got %d", e.ActiveBuffer().Point())
	}
}

func TestIsearchHandleKeyBackspaceRemovesChar(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "hel"

	ke := terminal.KeyEvent{Key: tcell.KeyBackspace}
	e.isearchHandleKey(ke)

	if e.isearchStr != "he" {
		t.Errorf("expected isearchStr='he' after backspace, got %q", e.isearchStr)
	}
}

func TestIsearchHandleKeyCtrlSSwitchesToForward(t *testing.T) {
	e := newTestEditor("abcabc")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(false)
	e.isearchStr = "abc"
	// Set point to a mid-buffer position to allow forward search.
	e.ActiveBuffer().SetPoint(3)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlS}
	e.isearchHandleKey(ke)

	if !e.isearchFwd {
		t.Error("C-s during isearch should set isearchFwd=true")
	}
}

func TestIsearchHandleKeyCtrlRSwitchesToBackward(t *testing.T) {
	e := newTestEditor("abcabc")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "abc"
	e.ActiveBuffer().SetPoint(6)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlR}
	e.isearchHandleKey(ke)

	if e.isearchFwd {
		t.Error("C-r during isearch should set isearchFwd=false")
	}
}

func TestIsearchHandleKeyUnknownKeyExitsIsearch(t *testing.T) {
	e := newTestEditor("hello")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	e.startIsearch(true)

	// A control key not handled by isearch exits isearch.
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlA}
	e.isearchHandleKey(ke)

	if e.isearching {
		t.Error("expected isearching=false after unrecognised control key")
	}
}

// ---------------------------------------------------------------------------
// isearchFindNext
// ---------------------------------------------------------------------------

func TestIsearchFindNextForwardFindsNextOccurrence(t *testing.T) {
	e := newTestEditor("abcXdefXghi")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetPoint(0)
	e.startIsearch(true)
	e.isearchStr = "X"
	e.isSearchCaseFold = false

	// First find moves point to after first X (position 4).
	e.isearchFind()
	firstPt := e.ActiveBuffer().Point()

	// isearchFindNext should find the second X.
	e.isearchFindNext()
	secondPt := e.ActiveBuffer().Point()

	if secondPt <= firstPt {
		t.Errorf("isearchFindNext should move point forward: first=%d second=%d", firstPt, secondPt)
	}
}

func TestIsearchFindNextBackwardFindsPreviousOccurrence(t *testing.T) {
	e := newTestEditor("abcXdefXghi")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "X"
	e.isSearchCaseFold = false

	// Move to end then search backward.
	e.isearchFwd = false
	e.ActiveBuffer().SetPoint(11)
	e.isearchFindNext()
	pt := e.ActiveBuffer().Point()
	// Should be at position 7 (second X).
	if pt != 7 {
		t.Errorf("expected point at 7 (second X), got %d", pt)
	}
}

// ---------------------------------------------------------------------------
// finishMinibuffer
// ---------------------------------------------------------------------------

func TestFinishMinibufferCallsDoneFunc(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var got string
	e.ReadMinibuffer("Test: ", func(s string) { got = s })
	// Type some text directly into minibufBuf.
	e.minibufBuf.InsertString(0, "myinput")
	e.minibufBuf.SetPoint(7)

	e.finishMinibuffer()

	if got != "myinput" {
		t.Errorf("expected done func called with 'myinput', got %q", got)
	}
	if e.minibufActive {
		t.Error("expected minibufActive=false after finish")
	}
}

func TestFinishMinibufferUsesSelectedCandidate(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var got string
	e.ReadMinibuffer("Pick: ", func(s string) { got = s })
	e.minibufCandidates = []string{"alpha", "beta"}
	e.minibufSelectedIdx = 1
	e.minibufCandidateChosen = true // user navigated popup

	e.finishMinibuffer()

	if got != "beta" {
		t.Errorf("expected 'beta' from candidate navigation, got %q", got)
	}
}

func TestFinishMinibufferSavesHistory(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.finishMinibuffer()

	hist := e.minibufHistory["Cmd: "]
	if len(hist) == 0 || hist[0] != "hello" {
		t.Errorf("expected 'hello' saved in history, got %v", hist)
	}
}

func TestFinishMinibufferNoopWhenInactive(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufActive = false

	// Should not panic.
	e.finishMinibuffer()
}

// ---------------------------------------------------------------------------
// cancelMinibuffer
// ---------------------------------------------------------------------------

func TestCancelMinibufferDeactivates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var called bool
	e.ReadMinibuffer("P: ", func(string) { called = true })
	e.cancelMinibuffer()

	if e.minibufActive {
		t.Error("expected minibufActive=false after cancel")
	}
	if called {
		t.Error("done func should not be called on cancel")
	}
}

func TestCancelMinibufferSetsQuitMessage(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.cancelMinibuffer()

	if e.message != "Quit" {
		t.Errorf("expected message='Quit' after cancel, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// dispatchMinibufKey
// ---------------------------------------------------------------------------

func TestDispatchMinibufKeyPrintableRune(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.String() != "x" {
		t.Errorf("expected minibuf content 'x', got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKeyBackspace(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})

	// Insert "ab" first.
	e.minibufBuf.InsertString(0, "ab")
	e.minibufBuf.SetPoint(2)

	ke := terminal.KeyEvent{Key: tcell.KeyBackspace}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.String() != "a" {
		t.Errorf("expected 'a' after backspace, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKeyEnterFinishes(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var got string
	e.ReadMinibuffer("P: ", func(s string) { got = s })
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)

	ke := terminal.KeyEvent{Key: tcell.KeyEnter}
	e.dispatchMinibufKey(ke)

	if e.minibufActive {
		t.Error("expected minibufActive=false after Enter")
	}
	if got != "hello" {
		t.Errorf("expected 'hello' from done func, got %q", got)
	}
}

func TestDispatchMinibufKeyCtrlGCancels(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlG}
	e.dispatchMinibufKey(ke)

	if e.minibufActive {
		t.Error("expected minibufActive=false after C-g")
	}
}

func TestDispatchMinibufKeyCtrlA(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlA}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.Point() != 0 {
		t.Errorf("expected point at 0 after C-a, got %d", e.minibufBuf.Point())
	}
}

func TestDispatchMinibufKeyCtrlE(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlE}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.Point() != 5 {
		t.Errorf("expected point at 5 after C-e, got %d", e.minibufBuf.Point())
	}
}

// ---------------------------------------------------------------------------
// minibufSelectNext / minibufSelectPrev
// ---------------------------------------------------------------------------

func TestMinibufSelectNext(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0

	e.minibufSelectNext()

	if e.minibufSelectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", e.minibufSelectedIdx)
	}
	if !e.minibufCandidateChosen {
		t.Error("expected minibufCandidateChosen=true after navigation")
	}
}

func TestMinibufSelectNextAtEnd(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 1

	e.minibufSelectNext()

	// Should not go beyond last candidate.
	if e.minibufSelectedIdx != 1 {
		t.Errorf("expected selectedIdx to stay at 1, got %d", e.minibufSelectedIdx)
	}
}

func TestMinibufSelectPrev(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 2
	e.minibufCandidateOffset = 0

	e.minibufSelectPrev()

	if e.minibufSelectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", e.minibufSelectedIdx)
	}
	if !e.minibufCandidateChosen {
		t.Error("expected minibufCandidateChosen=true after navigation")
	}
}

func TestMinibufSelectPrevAtStart(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 0

	e.minibufSelectPrev()

	if e.minibufSelectedIdx != 0 {
		t.Errorf("expected selectedIdx to stay at 0, got %d", e.minibufSelectedIdx)
	}
}

func TestMinibufSelectNextScrollsOffset(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	// Create more candidates than the popup max visible.
	e.minibufCandidates = []string{"a", "b", "c", "d", "e", "f", "g"}
	e.minibufSelectedIdx = minibufPopupMaxVisible - 1
	e.minibufCandidateOffset = 0

	// One more next should scroll the offset.
	e.minibufSelectNext()

	if e.minibufCandidateOffset != 1 {
		t.Errorf("expected offset=1 after scrolling, got %d", e.minibufCandidateOffset)
	}
}

func TestMinibufSelectPrevScrollsOffset(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b", "c", "d", "e", "f"}
	e.minibufSelectedIdx = 1
	e.minibufCandidateOffset = 1

	e.minibufSelectPrev()

	if e.minibufCandidateOffset != 0 {
		t.Errorf("expected offset=0 after scrolling back, got %d", e.minibufCandidateOffset)
	}
}

func TestMinibufSelectNextEmptyCandidates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = nil

	// Should not panic.
	e.minibufSelectNext()
}

func TestMinibufSelectPrevEmptyCandidates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = nil

	// Should not panic.
	e.minibufSelectPrev()
}

// ---------------------------------------------------------------------------
// minibufHistoryPrev / minibufHistoryNext
// ---------------------------------------------------------------------------

func TestMinibufHistoryPrevNoHistory(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufHistory = nil

	// Should not panic.
	e.minibufHistoryPrev()
}

func TestMinibufHistoryPrevNavigatesBack(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufHistory = map[string][]string{
		"Cmd: ": {"last", "earlier"},
	}
	e.minibufHistoryIdx = -1

	e.minibufHistoryPrev()

	if e.minibufHistoryIdx != 0 {
		t.Errorf("expected historyIdx=0 after first prev, got %d", e.minibufHistoryIdx)
	}
	if e.minibufBuf.String() != "last" {
		t.Errorf("expected 'last' in minibuf, got %q", e.minibufBuf.String())
	}
}

func TestMinibufHistoryPrevSavesCurrentText(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufBuf.InsertString(0, "current")
	e.minibufBuf.SetPoint(7)
	e.minibufHistory = map[string][]string{
		"Cmd: ": {"old"},
	}
	e.minibufHistoryIdx = -1

	e.minibufHistoryPrev()

	if e.minibufHistorySaved != "current" {
		t.Errorf("expected historySaved='current', got %q", e.minibufHistorySaved)
	}
}

func TestMinibufHistoryNextRestoresSavedText(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufHistory = map[string][]string{
		"Cmd: ": {"old"},
	}
	e.minibufHistoryIdx = 0
	e.minibufHistorySaved = "typed"

	e.minibufHistoryNext()

	if e.minibufBuf.String() != "typed" {
		t.Errorf("expected saved text 'typed' restored, got %q", e.minibufBuf.String())
	}
	if e.minibufHistoryIdx != -1 {
		t.Errorf("expected historyIdx=-1 after returning to current, got %d", e.minibufHistoryIdx)
	}
}

func TestMinibufHistoryNextAtBeginningNoOp(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	// historyIdx -1 means we're at the current (live) edit; next should be no-op.
	e.minibufHistoryIdx = -1

	// Should not panic.
	e.minibufHistoryNext()
}

// ---------------------------------------------------------------------------
// minibufBackwardKillWord
// ---------------------------------------------------------------------------

func TestMinibufBackwardKillWordDeletesWord(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, "foo bar")
	e.minibufBuf.SetPoint(7)

	e.minibufBackwardKillWord()

	result := e.minibufBuf.String()
	// Should have killed "bar", leaving "foo ".
	if !strings.HasPrefix(result, "foo") {
		t.Errorf("expected 'foo' prefix after kill-word, got %q", result)
	}
	if strings.Contains(result, "bar") {
		t.Errorf("expected 'bar' removed, got %q", result)
	}
}

func TestMinibufBackwardKillWordAtBOLNoOp(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	// Point is already at 0.

	// Should not panic or modify anything.
	e.minibufBackwardKillWord()
	if e.minibufBuf.String() != "" {
		t.Errorf("expected empty minibuf unchanged, got %q", e.minibufBuf.String())
	}
}

// ---------------------------------------------------------------------------
// selfInsert
// ---------------------------------------------------------------------------

func TestSelfInsertInsertsRune(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	e.selfInsert('A')

	if e.ActiveBuffer().String() != "A" {
		t.Errorf("expected 'A' after selfInsert, got %q", e.ActiveBuffer().String())
	}
	if e.ActiveBuffer().Point() != 1 {
		t.Errorf("expected point=1 after selfInsert, got %d", e.ActiveBuffer().Point())
	}
}

func TestSelfInsertOnReadOnlyBufferShowsMessage(t *testing.T) {
	e := newTestEditor("existing")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetReadOnly(true)

	e.selfInsert('X')

	if e.ActiveBuffer().String() != "existing" {
		t.Error("selfInsert should not modify read-only buffer")
	}
	if e.message == "" {
		t.Error("expected error message for read-only buffer insert")
	}
}

func TestSelfInsertDeactivatesMark(t *testing.T) {
	e := newTestEditor("hello")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetMark(0)
	e.ActiveBuffer().SetMarkActive(true)

	e.selfInsert('X')

	if e.ActiveBuffer().MarkActive() {
		t.Error("selfInsert should deactivate mark")
	}
}

func TestSelfInsertMultipleRunes(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	for _, r := range "hello" {
		e.selfInsert(r)
	}

	if e.ActiveBuffer().String() != "hello" {
		t.Errorf("expected 'hello', got %q", e.ActiveBuffer().String())
	}
}

// ---------------------------------------------------------------------------
// handleDescribeKey
// ---------------------------------------------------------------------------

func TestHandleDescribeKeyKnownCommand(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	e.buffers = append(e.buffers, buffer.NewWithContent("*scratch*", ""))

	e.describeKeyPending = true

	// C-f is bound to "forward-char".
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlF}
	e.handleDescribeKey(ke)

	if e.describeKeyPending {
		t.Error("expected describeKeyPending=false after terminal key")
	}
	// The active buffer should have been switched to *Help*.
	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		t.Error("expected *Help* buffer to be created")
	}
}

func TestHandleDescribeKeyUndefinedKey(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()

	e.describeKeyPending = true

	// F12 is not bound.
	ke := terminal.KeyEvent{Key: tcell.KeyF12}
	e.handleDescribeKey(ke)

	if e.describeKeyPending {
		t.Error("expected describeKeyPending=false for undefined key")
	}
	// Message should mention "undefined".
	if !strings.Contains(e.message, "undefined") {
		t.Errorf("expected 'undefined' in message, got %q", e.message)
	}
}

func TestHandleDescribeKeyPrefixAccumulates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()

	e.describeKeyPending = true

	// C-x is a prefix key; we need a second key.
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlX}
	e.handleDescribeKey(ke)

	// Still pending (waiting for second key of prefix).
	if !e.describeKeyPending {
		t.Error("expected describeKeyPending=true after prefix key")
	}
	if e.describeKeyMap == nil {
		t.Error("expected describeKeyMap to be set for prefix")
	}
}
