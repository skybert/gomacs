package editor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/lsp"
	"github.com/skybert/gomacs/internal/terminal"
)

func TestBufferWordCompletions_basic(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello world helo help")
	items := bufferWordCompletions(buf, "hel")
	labels := make(map[string]bool)
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels["hello"] {
		t.Error("expected 'hello' in completions")
	}
	if !labels["helo"] {
		t.Error("expected 'helo' in completions")
	}
	if !labels["help"] {
		t.Error("expected 'help' in completions")
	}
	// 'world' does not start with 'hel'
	if labels["world"] {
		t.Error("unexpected 'world' in completions")
	}
}

func TestBufferWordCompletions_caseInsensitive(t *testing.T) {
	buf := buffer.NewWithContent("test", "Beautiful beautiful BEAUTIFUL beau")
	items := bufferWordCompletions(buf, "bea")
	labels := make(map[string]bool)
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels["Beautiful"] {
		t.Error("expected 'Beautiful'")
	}
	if !labels["beautiful"] {
		t.Error("expected 'beautiful'")
	}
	if !labels["BEAUTIFUL"] {
		t.Error("expected 'BEAUTIFUL'")
	}
	// "beau" is a word in the buffer that starts with "bea", but is the same
	// length as the minimum check may or may not exclude it depending on the
	// point position; here it should appear because it's longer than the prefix
	if !labels["beau"] {
		t.Error("expected 'beau'")
	}
}

func TestBufferWordCompletions_noDuplicates(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello hello hello")
	items := bufferWordCompletions(buf, "hel")
	if len(items) != 1 {
		t.Errorf("expected 1 unique item, got %d", len(items))
	}
}

func TestBufferWordCompletions_prefixNotReturned(t *testing.T) {
	// Words equal in length to the prefix are excluded (no meaningful expansion).
	buf := buffer.NewWithContent("test", "foo foobar")
	items := bufferWordCompletions(buf, "foo")
	for _, it := range items {
		if it.Label == "foo" {
			t.Error("prefix word 'foo' should not appear as completion")
		}
	}
	found := false
	for _, it := range items {
		if it.Label == "foobar" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'foobar' in completions")
	}
}

func TestBufferWordCompletions_emptyPrefix(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello world")
	items := bufferWordCompletions(buf, "")
	if items != nil {
		t.Error("expected nil items for empty prefix")
	}
}

func TestLspCompWordPrefix_empty(t *testing.T) {
	buf := buffer.NewWithContent("test", "")
	prefix, start := lspCompWordPrefix(buf)
	if prefix != "" {
		t.Errorf("expected empty prefix, got %q", prefix)
	}
	if start != 0 {
		t.Errorf("expected start=0, got %d", start)
	}
}

func TestLspCompWordPrefix_atWord(t *testing.T) {
	buf := buffer.NewWithContent("test", "os.Stdout")
	// Point is after 'Stdout' (position 9)
	buf.SetPoint(9)
	prefix, start := lspCompWordPrefix(buf)
	if prefix != "Stdout" {
		t.Errorf("expected 'Stdout', got %q", prefix)
	}
	if start != 3 {
		t.Errorf("expected start=3, got %d", start)
	}
}

// ---------------------------------------------------------------------------
// isProseContext
// ---------------------------------------------------------------------------

// newTestEditorWithMode creates a test editor whose active buffer has the
// given mode and content, with point placed at the end of the content.
func newTestEditorWithMode(content, mode string) *Editor {
	e := newTestEditor(content)
	e.ActiveBuffer().SetMode(mode)
	e.ActiveBuffer().SetPoint(len([]rune(content)))
	return e
}

func TestIsProseContext_markdownMode(t *testing.T) {
	e := newTestEditorWithMode("Hello world", "markdown")
	if !e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected markdown mode to be a prose context")
	}
}

func TestIsProseContext_textMode(t *testing.T) {
	e := newTestEditorWithMode("Hello world", "text")
	if !e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected text mode to be a prose context")
	}
}

func TestIsProseContext_fundamentalMode(t *testing.T) {
	e := newTestEditorWithMode("Hello world", "fundamental")
	if !e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected fundamental mode to be a prose context")
	}
}

func TestIsProseContext_emptyMode(t *testing.T) {
	e := newTestEditorWithMode("Hello world", "")
	if !e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected empty mode to be a prose context")
	}
}

func TestIsProseContext_goModeCode(t *testing.T) {
	// Point sits in code (not a comment), so it should not be prose.
	src := "package main\n\nfunc main() {\n}"
	e := newTestEditorWithMode(src, "go")
	// Place point inside "func" keyword — clearly not a comment.
	e.ActiveBuffer().SetPoint(15)
	if e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected Go code (non-comment) to NOT be a prose context")
	}
}

func TestIsProseContext_goModeComment(t *testing.T) {
	// Point sits inside a line comment "// hello" starting at offset 0.
	// The Go highlighter marks "// hello" as FaceComment.
	src := "// hello\nfunc main() {}"
	e := newTestEditorWithMode(src, "go")
	// Place point at offset 4 — inside "// hello".
	e.ActiveBuffer().SetPoint(4)
	if !e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected cursor inside Go comment to be a prose context")
	}
}

func TestIsProseContext_goModeAtCommentStart(t *testing.T) {
	// Point at offset 1 (inside "//") — still in the comment span.
	src := "// hello"
	e := newTestEditorWithMode(src, "go")
	e.ActiveBuffer().SetPoint(1)
	if !e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected cursor at start of Go comment to be a prose context")
	}
}

func TestIsProseContext_goModePointZero(t *testing.T) {
	// When point==0 the function clamps pt to 0; character at 0 is "f" (code).
	src := "func main() {}"
	e := newTestEditorWithMode(src, "go")
	e.ActiveBuffer().SetPoint(0)
	if e.isProseContext(e.ActiveBuffer()) {
		t.Error("expected point=0 in Go code to NOT be a prose context")
	}
}

// ---------------------------------------------------------------------------
// parseCompletionItems
// ---------------------------------------------------------------------------

func TestParseCompletionItems_nil(t *testing.T) {
	items := parseCompletionItems(nil)
	if items != nil {
		t.Errorf("expected nil for nil input, got %v", items)
	}
}

func TestParseCompletionItems_nullString(t *testing.T) {
	items := parseCompletionItems(json.RawMessage("null"))
	if items != nil {
		t.Errorf("expected nil for 'null' input, got %v", items)
	}
}

func TestParseCompletionItems_bareArray(t *testing.T) {
	raw := json.RawMessage(`[{"label":"Println"},{"label":"Printf"}]`)
	items := parseCompletionItems(raw)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Label != "Println" {
		t.Errorf("expected items[0].Label='Println', got %q", items[0].Label)
	}
	if items[1].Label != "Printf" {
		t.Errorf("expected items[1].Label='Printf', got %q", items[1].Label)
	}
}

func TestParseCompletionItems_completionList(t *testing.T) {
	raw := json.RawMessage(`{"isIncomplete":false,"items":[{"label":"Println","insertText":"Println("},{"label":"Printf","detail":"func"}]}`)
	items := parseCompletionItems(raw)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Label != "Println" {
		t.Errorf("expected items[0].Label='Println', got %q", items[0].Label)
	}
	if items[0].InsertText != "Println(" {
		t.Errorf("expected items[0].InsertText='Println(', got %q", items[0].InsertText)
	}
	if items[1].Detail != "func" {
		t.Errorf("expected items[1].Detail='func', got %q", items[1].Detail)
	}
}

func TestParseCompletionItems_emptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	items := parseCompletionItems(raw)
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseCompletionItems_invalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json at all`)
	items := parseCompletionItems(raw)
	if items != nil {
		t.Errorf("expected nil for invalid JSON, got %v", items)
	}
}

// ---------------------------------------------------------------------------
// lspCompNext / lspCompPrev
// ---------------------------------------------------------------------------

func newTestEditorWithCompItems(items []lsp.CompletionItem) *Editor {
	e := newTestEditor("")
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	e.lspCompItems = items
	e.lspCompActive = true
	e.lspCompSelectedIdx = 0
	e.lspCompOffset = 0
	return e
}

func TestLspCompNext_advances(t *testing.T) {
	items := []lsp.CompletionItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	e := newTestEditorWithCompItems(items)
	e.lspCompNext()
	if e.lspCompSelectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", e.lspCompSelectedIdx)
	}
}

func TestLspCompNext_clampsAtEnd(t *testing.T) {
	items := []lsp.CompletionItem{{Label: "a"}, {Label: "b"}}
	e := newTestEditorWithCompItems(items)
	e.lspCompSelectedIdx = 1
	e.lspCompNext()
	if e.lspCompSelectedIdx != 1 {
		t.Errorf("expected selectedIdx clamped at 1, got %d", e.lspCompSelectedIdx)
	}
}

func TestLspCompNext_scrollsOffset(t *testing.T) {
	// With 7 items and maxVisible=6, advancing past idx 5 should scroll offset.
	items := make([]lsp.CompletionItem, 7)
	for i := range items {
		items[i] = lsp.CompletionItem{Label: string(rune('a' + i))}
	}
	e := newTestEditorWithCompItems(items)
	e.lspCompSelectedIdx = lspCompMaxVisible - 1 // idx 5, offset 0
	e.lspCompNext()                              // idx 6 -> offset must become 1
	if e.lspCompSelectedIdx != lspCompMaxVisible {
		t.Errorf("expected selectedIdx=%d, got %d", lspCompMaxVisible, e.lspCompSelectedIdx)
	}
	if e.lspCompOffset != 1 {
		t.Errorf("expected offset=1, got %d", e.lspCompOffset)
	}
}

func TestLspCompPrev_goesBack(t *testing.T) {
	items := []lsp.CompletionItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	e := newTestEditorWithCompItems(items)
	e.lspCompSelectedIdx = 2
	e.lspCompPrev()
	if e.lspCompSelectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", e.lspCompSelectedIdx)
	}
}

func TestLspCompPrev_clampsAtZero(t *testing.T) {
	items := []lsp.CompletionItem{{Label: "a"}, {Label: "b"}}
	e := newTestEditorWithCompItems(items)
	e.lspCompSelectedIdx = 0
	e.lspCompPrev()
	if e.lspCompSelectedIdx != 0 {
		t.Errorf("expected selectedIdx clamped at 0, got %d", e.lspCompSelectedIdx)
	}
}

func TestLspCompPrev_scrollsOffset(t *testing.T) {
	items := make([]lsp.CompletionItem, 7)
	for i := range items {
		items[i] = lsp.CompletionItem{Label: string(rune('a' + i))}
	}
	e := newTestEditorWithCompItems(items)
	e.lspCompSelectedIdx = 1
	e.lspCompOffset = 1 // offset > selectedIdx after prev: should decrement offset
	e.lspCompPrev()
	if e.lspCompSelectedIdx != 0 {
		t.Errorf("expected selectedIdx=0, got %d", e.lspCompSelectedIdx)
	}
	if e.lspCompOffset != 0 {
		t.Errorf("expected offset=0, got %d", e.lspCompOffset)
	}
}

func TestLspCompNext_emptyItems(t *testing.T) {
	e := newTestEditorWithCompItems(nil)
	e.lspCompNext() // must not panic
	if e.lspCompSelectedIdx != 0 {
		t.Errorf("expected selectedIdx=0, got %d", e.lspCompSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// lspCompDismiss
// ---------------------------------------------------------------------------

func TestLspCompDismiss_clearsState(t *testing.T) {
	items := []lsp.CompletionItem{{Label: "Println"}, {Label: "Printf"}}
	e := newTestEditorWithCompItems(items)
	e.lspCompSelectedIdx = 1
	e.lspCompOffset = 1

	e.lspCompDismiss()

	if e.lspCompActive {
		t.Error("expected lspCompActive=false after dismiss")
	}
	if len(e.lspCompItems) != 0 {
		t.Errorf("expected lspCompItems cleared, got %d items", len(e.lspCompItems))
	}
	if e.lspCompSelectedIdx != 0 {
		t.Errorf("expected selectedIdx=0, got %d", e.lspCompSelectedIdx)
	}
	if e.lspCompOffset != 0 {
		t.Errorf("expected offset=0, got %d", e.lspCompOffset)
	}
}

// ---------------------------------------------------------------------------
// lspCompletionInsert
// ---------------------------------------------------------------------------

func TestLspCompletionInsert_basic(t *testing.T) {
	// Buffer contains "fmt.Pr" with point at end; wordStart=4, prefix="Pr".
	e := newTestEditor("fmt.Pr")
	e.ActiveBuffer().SetPoint(6)
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	e.lspCompActive = true
	e.lspCompWordStart = 4 // "Pr" starts at rune 4
	e.lspCompItems = []lsp.CompletionItem{
		{Label: "Println"},
		{Label: "Printf"},
	}
	e.lspCompSelectedIdx = 0

	e.lspCompletionInsert()

	got := e.ActiveBuffer().String()
	want := "fmt.Println"
	if got != want {
		t.Errorf("after insert: want %q, got %q", want, got)
	}
	wantPoint := 11
	if pt := e.ActiveBuffer().Point(); pt != wantPoint {
		t.Errorf("after insert: want point=%d, got %d", wantPoint, pt)
	}
}

func TestLspCompletionInsert_usesInsertText(t *testing.T) {
	// When InsertText is set it should be preferred over Label.
	e := newTestEditor("os.Ex")
	e.ActiveBuffer().SetPoint(5)
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	e.lspCompActive = true
	e.lspCompWordStart = 3 // "Ex" starts at rune 3
	e.lspCompItems = []lsp.CompletionItem{
		{Label: "Exit", InsertText: "Exit(0)"},
	}
	e.lspCompSelectedIdx = 0

	e.lspCompletionInsert()

	got := e.ActiveBuffer().String()
	want := "os.Exit(0)"
	if got != want {
		t.Errorf("after insert with InsertText: want %q, got %q", want, got)
	}
}

func TestLspCompletionInsert_noopWhenInactive(t *testing.T) {
	e := newTestEditor("hello")
	e.ActiveBuffer().SetPoint(5)
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	e.lspCompActive = false
	e.lspCompItems = []lsp.CompletionItem{{Label: "helloWorld"}}

	e.lspCompletionInsert()

	if got := e.ActiveBuffer().String(); got != "hello" {
		t.Errorf("expected buffer unchanged, got %q", got)
	}
}

func TestLspCompletionInsert_noopWhenNoItems(t *testing.T) {
	e := newTestEditor("hel")
	e.ActiveBuffer().SetPoint(3)
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	e.lspCompActive = true
	e.lspCompItems = nil

	e.lspCompletionInsert()

	if got := e.ActiveBuffer().String(); got != "hel" {
		t.Errorf("expected buffer unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// triggerBufferWordCompletion
// ---------------------------------------------------------------------------

func TestTriggerBufferWordCompletion_CodeActivates(t *testing.T) {
	// In a code context (not prose, not in a comment) the popup activates
	// synchronously.
	e := newTestEditorWithMode("alpha alphabet al", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetPoint(b.Len())
	e.triggerBufferWordCompletion(b, "al", b.Len()-2)
	if !e.lspCompActive {
		t.Fatal("expected completion popup to activate in code context")
	}
	if len(e.lspCompItems) == 0 {
		t.Fatal("expected completion items")
	}
}

func TestTriggerBufferWordCompletion_NoMatchNoPopup(t *testing.T) {
	e := newTestEditorWithMode("alpha beta", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	e.triggerBufferWordCompletion(b, "zzz", 0)
	if e.lspCompActive {
		t.Fatal("no matching words should not activate the popup")
	}
}

func TestTriggerBufferWordCompletion_ProseDelays(t *testing.T) {
	// Prose context defers activation, so it is not active immediately.
	e := newTestEditorWithMode("alpha alphabet al", "text")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetPoint(b.Len())
	e.triggerBufferWordCompletion(b, "al", b.Len()-2)
	if e.lspCompActive {
		t.Fatal("prose context should defer activation, not activate immediately")
	}
}

// ---------------------------------------------------------------------------
// lspCompletionHandleKey
// ---------------------------------------------------------------------------

func newCompletionEditor() *Editor {
	e := newTestEditor("hel")
	e.lspCompItems = []lsp.CompletionItem{{Label: "hello"}, {Label: "help"}, {Label: "helm"}}
	e.lspCompActive = true
	e.lspCompSelectedIdx = 0
	e.lspCompWordStart = 0
	e.lspCompDelayCancel = func() {}
	return e
}

func TestLspCompletionHandleKey_InactiveReturnsFalse(t *testing.T) {
	e := newTestEditor("x")
	if e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyTab}) {
		t.Fatal("handler should return false when popup is inactive")
	}
}

func TestLspCompletionHandleKey_DownUp(t *testing.T) {
	e := newCompletionEditor()
	if !e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyDown}) {
		t.Fatal("Down should be consumed")
	}
	if e.lspCompSelectedIdx != 1 {
		t.Fatalf("Down should advance selection, got %d", e.lspCompSelectedIdx)
	}
	if !e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyUp}) {
		t.Fatal("Up should be consumed")
	}
	if e.lspCompSelectedIdx != 0 {
		t.Fatalf("Up should move selection back, got %d", e.lspCompSelectedIdx)
	}
}

func TestLspCompletionHandleKey_AltNP(t *testing.T) {
	e := newCompletionEditor()
	if !e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n', Mod: tcell.ModAlt}) {
		t.Fatal("M-n should be consumed")
	}
	if e.lspCompSelectedIdx != 1 {
		t.Fatalf("M-n should advance, got %d", e.lspCompSelectedIdx)
	}
	if !e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p', Mod: tcell.ModAlt}) {
		t.Fatal("M-p should be consumed")
	}
	if e.lspCompSelectedIdx != 0 {
		t.Fatalf("M-p should retreat, got %d", e.lspCompSelectedIdx)
	}
}

func TestLspCompletionHandleKey_TabInserts(t *testing.T) {
	e := newCompletionEditor()
	e.ActiveBuffer().SetPoint(3)
	if !e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyTab}) {
		t.Fatal("Tab should be consumed")
	}
	if e.lspCompActive {
		t.Fatal("Tab should dismiss the popup after inserting")
	}
	if got := e.ActiveBuffer().String(); got != "hello" {
		t.Fatalf("Tab should insert the selected completion, got %q", got)
	}
}

func TestLspCompletionHandleKey_EscapeDismisses(t *testing.T) {
	e := newCompletionEditor()
	if !e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyEscape}) {
		t.Fatal("Escape should be consumed")
	}
	if e.lspCompActive {
		t.Fatal("Escape should dismiss the popup")
	}
}

func TestLspCompletionHandleKey_RuneDismissesAndContinues(t *testing.T) {
	e := newCompletionEditor()
	if e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'}) {
		t.Fatal("a plain rune should not be consumed (normal dispatch continues)")
	}
	if e.lspCompActive {
		t.Fatal("a plain rune should dismiss the popup")
	}
}

func TestLspCompletionHandleKey_BackspaceDismissesAndContinues(t *testing.T) {
	e := newCompletionEditor()
	if e.lspCompletionHandleKey(terminal.KeyEvent{Key: tcell.KeyBackspace}) {
		t.Fatal("Backspace should not be consumed")
	}
	if e.lspCompActive {
		t.Fatal("Backspace should dismiss the popup")
	}
}

// ---------------------------------------------------------------------------
// triggerBufferWordCompletion (non-prose synchronous path)
// ---------------------------------------------------------------------------

func TestTriggerBufferWordCompletion_GoMode(t *testing.T) {
	e := newTestEditorWithMode("fooXXbar fooXX", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetPoint(b.Len())
	e.triggerBufferWordCompletion(b, "fooXX", b.Len()-5)
	if !e.lspCompActive {
		t.Fatal("expected popup active after buffer-word completion in go mode")
	}
	if len(e.lspCompItems) == 0 {
		t.Fatal("expected at least one completion item")
	}
}

func TestTriggerBufferWordCompletion_NoCandidates(t *testing.T) {
	e := newTestEditorWithMode("uniqueword", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetPoint(b.Len())
	e.triggerBufferWordCompletion(b, "zzz", b.Len())
	if e.lspCompActive {
		t.Fatal("expected no popup when there are no candidates")
	}
}

// ---------------------------------------------------------------------------
// lspMaybeTriggerCompletion — fallback to buffer words when no LSP server
// ---------------------------------------------------------------------------

func TestLspMaybeTriggerCompletion_Fallback(t *testing.T) {
	e := newTestEditorWithMode("fooXXbar fooXX", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetFilename("/tmp/x.go")
	b.SetPoint(b.Len())
	e.lspMaybeTriggerCompletion()
	if !e.lspCompActive {
		t.Fatal("expected buffer-word fallback popup to activate")
	}
}

func TestLspMaybeTriggerCompletion_NoFilename(t *testing.T) {
	e := newTestEditorWithMode("fooXXbar fooXX", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetPoint(b.Len())
	e.lspMaybeTriggerCompletion() // no filename → early return
	if e.lspCompActive {
		t.Fatal("expected no completion without a filename")
	}
}

func TestLspMaybeTriggerCompletion_ShortPrefix(t *testing.T) {
	e := newTestEditorWithMode("ab", "go")
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetFilename("/tmp/x.go")
	b.SetPoint(b.Len())
	e.lspMaybeTriggerCompletion() // prefix "ab" < minChars(3) → no popup
	if e.lspCompActive {
		t.Fatal("expected no completion for a short prefix")
	}
}

// ---------------------------------------------------------------------------
// renderLSPCompletionPopup — capture terminal
// ---------------------------------------------------------------------------

func TestRenderLSPCompletionPopup_Inactive(t *testing.T) {
	e := newCapTestEditor("hello")
	e.lspCompActive = false
	e.renderLSPCompletionPopup() // no-op, must not panic
}

func TestRenderLSPCompletionPopup_DrawsBorder(t *testing.T) {
	e := newCapTestEditor("os.\nmore text here\nyet another line\n")
	e.ActiveBuffer().SetPoint(3)
	e.lspCompItems = []lsp.CompletionItem{
		{Label: "Open", Detail: "func"},
		{Label: "Create", Detail: "func"},
	}
	e.lspCompActive = true
	e.Redraw()
	w, h := e.term.CaptureSize()
	found := false
	for row := 0; row < h && !found; row++ {
		for col := 0; col < w; col++ {
			if ch, _ := e.term.CaptureCell(col, row); ch == '╭' {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected completion popup border glyph on screen")
	}
}

func TestRenderLSPCompletionPopup_ManyItemsScroll(t *testing.T) {
	e := newCapTestEditor("x\n\n\n\n\n\n\n\n\n\n\n\n")
	e.ActiveBuffer().SetPoint(0)
	items := make([]lsp.CompletionItem, 10)
	for i := range items {
		items[i] = lsp.CompletionItem{Label: string(rune('a'+i)) + "fn"}
	}
	e.lspCompItems = items
	e.lspCompActive = true
	e.lspCompOffset = 2
	e.lspCompSelectedIdx = 3
	e.Redraw() // exercises offset/visible-slice logic without panicking
}

// ---------------------------------------------------------------------------
// triggerBufferWordCompletion
// ---------------------------------------------------------------------------

func TestCov_TriggerBufferWordCompletion_NonProse(t *testing.T) {
	e := newTestEditorWithMode("function functional functor", "go")
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Len())
	e.triggerBufferWordCompletion(buf, "func", buf.Len()-3)
	if !e.lspCompActive {
		t.Fatal("expected completion popup to activate in non-prose mode")
	}
	if len(e.lspCompItems) == 0 {
		t.Fatal("expected candidate items")
	}
}

func TestCov_TriggerBufferWordCompletion_NoMatchesNoop(t *testing.T) {
	e := newTestEditorWithMode("alpha beta", "go")
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	buf := e.ActiveBuffer()
	e.triggerBufferWordCompletion(buf, "zzz", 0)
	if e.lspCompActive {
		t.Fatal("no candidates should leave the popup inactive")
	}
}

func TestCov_TriggerBufferWordCompletion_ProseSchedules(t *testing.T) {
	e := newTestEditorWithMode("hello helicopter helium", "text")
	_, cancel := context.WithCancel(context.Background())
	e.lspCompDelayCancel = cancel
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Len())
	// Prose context schedules a delayed trigger and returns immediately without
	// activating the popup synchronously.
	e.triggerBufferWordCompletion(buf, "hel", buf.Len()-3)
	if e.lspCompActive {
		t.Fatal("prose context should not activate popup synchronously")
	}
}
