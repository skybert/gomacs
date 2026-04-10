package editor

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/lsp"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---- completion state -------------------------------------------------------

// lspCompletionMinChars is the default minimum word-characters before
// completion triggers.  Overridden by (setq completion-menu-trigger-chars N).
const lspDefaultCompletionMinChars = 3

// lspCompWordPrefix returns the word (identifier characters) immediately
// before point in buf.  Returns ("", 0) when there is no prefix.
func lspCompWordPrefix(buf interface {
	Point() int
	RuneAt(int) rune
	Len() int
}) (prefix string, start int) {
	pt := buf.Point()
	end := pt
	i := pt - 1
	for i >= 0 {
		r := buf.RuneAt(i)
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		i--
	}
	start = i + 1
	if start >= end {
		return "", pt
	}
	var sb strings.Builder
	for j := start; j < end; j++ {
		sb.WriteRune(buf.RuneAt(j))
	}
	return sb.String(), start
}

// lspMaybeTriggerCompletion is called after each self-insert.  When the
// prefix before point reaches the trigger threshold it either fires an async
// textDocument/completion request (when an LSP server is ready) or falls back
// to collecting candidate words from the current buffer.
// A dot trigger (e.g. "os.") bypasses the length check for LSP connections.
func (e *Editor) lspMaybeTriggerCompletion() {
	if e.lspCompInflight || e.lspCompActive {
		return
	}
	buf := e.ActiveBuffer()
	if buf.Filename() == "" {
		return
	}
	minChars := e.lspCompletionMinChars
	if minChars <= 0 {
		minChars = lspDefaultCompletionMinChars
	}
	prefix, wordStart := lspCompWordPrefix(buf)

	// Check whether the character immediately before the word prefix is a
	// trigger character (e.g. '.').  If so we trigger even with a short prefix.
	var triggerChar rune
	if wordStart > 0 {
		prev := buf.RuneAt(wordStart - 1)
		if prev == '.' {
			triggerChar = prev
		}
	}

	conn := e.lspConns[buf.Mode()]

	if len([]rune(prefix)) < minChars && (triggerChar == 0 || conn == nil) {
		return
	}

	if conn == nil || !conn.isReady {
		// No LSP: fall back to buffer-word completion.
		if len([]rune(prefix)) >= minChars {
			e.triggerBufferWordCompletion(buf, prefix, wordStart)
		}
		return
	}

	// Send didChange synchronously before the goroutine fires the completion
	// request.  lspMaybeDidChange is normally called from Redraw(), which runs
	// after selfInsert, so without this the LSP server would receive the
	// completion request before it knows about the character the user just typed.
	e.lspMaybeDidChange(buf)

	e.lspCompInflight = true
	e.lspCompWordStart = wordStart
	pos := e.bufPointToLSP(buf)
	uri := lsp.FileURI(buf.Filename())
	ctx := context.Background()
	e.lspAsync(func() func() {
		params := map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     pos,
		}
		if triggerChar != 0 {
			params["context"] = map[string]any{
				"triggerKind":      2, // TriggerCharacter
				"triggerCharacter": string(triggerChar),
			}
		}
		result, err := conn.client.CallCtx(ctx, "textDocument/completion", params)
		return func() {
			e.lspCompInflight = false
			if err != nil || result == nil || string(result) == "null" {
				// Request failed or server returned nothing; try again in case
				// the user has typed enough for a new request.
				e.lspMaybeTriggerCompletion()
				return
			}
			items := parseCompletionItems(result)
			if len(items) == 0 {
				return
			}
			// Re-check that prefix still matches (user may have kept typing).
			prefix2, start2 := lspCompWordPrefix(e.ActiveBuffer())
			if start2 != e.lspCompWordStart {
				// Word boundary shifted; fire a fresh request for the current position.
				e.lspMaybeTriggerCompletion()
				return
			}
			// Filter to items that share the current prefix.
			var filtered []lsp.CompletionItem
			lower := strings.ToLower(prefix2)
			for _, item := range items {
				if strings.HasPrefix(strings.ToLower(item.Label), lower) {
					filtered = append(filtered, item)
				}
			}
			if len(filtered) == 0 {
				return
			}
			e.lspCompItems = filtered
			e.lspCompSelectedIdx = 0
			e.lspCompOffset = 0
			e.lspCompActive = true
		}
	})
}

// triggerBufferWordCompletion fills the completion popup with words from the
// current buffer that begin with prefix.  This is used as a fallback when no
// LSP server is available for the buffer's mode.
// In prose modes (text, markdown, fundamental) and when the cursor is inside a
// comment span, the popup is delayed by lspCompProseDelay to avoid interrupting
// fast typing.
func (e *Editor) triggerBufferWordCompletion(buf *buffer.Buffer, prefix string, wordStart int) {
	items := bufferWordCompletions(buf, prefix)
	if len(items) == 0 {
		return
	}

	// Cancel any previously scheduled delayed trigger.
	e.lspCompDelayCancel()

	if e.isProseContext(buf) {
		// Delay to avoid popping up mid-sentence for fast typists.
		ctx, cancel := context.WithCancel(context.Background())
		e.lspCompDelayCancel = cancel
		wordStartCopy := wordStart
		go func() {
			select {
			case <-time.After(lspCompProseDelay):
			case <-ctx.Done():
				return
			}
			e.lspAsync(func() func() {
				return func() {
					// Re-check that the user hasn't typed past the trigger point.
					if e.lspCompActive || e.lspCompInflight {
						return
					}
					prefix2, start2 := lspCompWordPrefix(e.ActiveBuffer())
					if start2 != wordStartCopy {
						return
					}
					items2 := bufferWordCompletions(e.ActiveBuffer(), prefix2)
					if len(items2) == 0 {
						return
					}
					e.lspCompItems = items2
					e.lspCompSelectedIdx = 0
					e.lspCompOffset = 0
					e.lspCompWordStart = wordStartCopy
					e.lspCompActive = true
				}
			})
		}()
		return
	}

	e.lspCompItems = items
	e.lspCompSelectedIdx = 0
	e.lspCompOffset = 0
	e.lspCompWordStart = wordStart
	e.lspCompActive = true
}

// lspCompProseDelay is how long to wait before showing buffer-word completions
// in prose modes or comment spans, so fast typists aren't interrupted.
const lspCompProseDelay = 400 * time.Millisecond

// isProseContext reports whether buf is in a prose mode or the cursor is
// currently inside a comment span.
func (e *Editor) isProseContext(buf *buffer.Buffer) bool {
	switch buf.Mode() {
	case "markdown", "text", "fundamental", "":
		return true
	}
	// For programming modes, check whether the cursor sits in a comment span.
	cache := e.getSpanCache(buf)
	pt := buf.Point()
	if pt > 0 {
		pt-- // check the character just before the cursor
	}
	f := faceAtPos(cache.spans, pt)
	return f == syntax.FaceComment
}

// bufferWordCompletions scans buf and returns CompletionItems for every unique
// word that starts with prefix (case-insensitive) and is longer than prefix.
func bufferWordCompletions(buf interface {
	Point() int
	RuneAt(int) rune
	Len() int
}, prefix string) []lsp.CompletionItem {
	if prefix == "" {
		return nil
	}
	n := buf.Len()
	lower := strings.ToLower(prefix)
	seen := make(map[string]bool)
	var items []lsp.CompletionItem
	i := 0
	for i < n {
		r := buf.RuneAt(i)
		if isWordRune(r) {
			start := i
			for i < n && isWordRune(buf.RuneAt(i)) {
				i++
			}
			if i-start <= len([]rune(prefix)) {
				continue
			}
			var sb strings.Builder
			for j := start; j < i; j++ {
				sb.WriteRune(buf.RuneAt(j))
			}
			word := sb.String()
			if strings.HasPrefix(strings.ToLower(word), lower) && !seen[word] {
				seen[word] = true
				items = append(items, lsp.CompletionItem{Label: word})
			}
		} else {
			i++
		}
	}
	return items
}

// lspCompletionHandleKey processes a key press while the completion popup is
// visible.  Returns true if the key was consumed, false if normal dispatch
// should continue.
func (e *Editor) lspCompletionHandleKey(ke terminal.KeyEvent) bool {
	if !e.lspCompActive {
		return false
	}
	switch ke.Key {
	case tcell.KeyTab, tcell.KeyEnter:
		e.lspCompletionInsert()
		e.lspCompDismiss()
		return true
	case tcell.KeyDown:
		e.lspCompNext()
		return true
	case tcell.KeyBacktab, tcell.KeyUp:
		e.lspCompPrev()
		return true
	case tcell.KeyRune:
		if ke.Mod == tcell.ModAlt {
			switch ke.Rune {
			case 'n':
				e.lspCompNext()
				return true
			case 'p':
				e.lspCompPrev()
				return true
			}
		}
		e.lspCompDismiss()
		return false
	case tcell.KeyEscape, tcell.KeyCtrlG:
		e.lspCompDismiss()
		// C-g: also emit a "Quit" message consistent with rest of editor.
		if ke.Key == tcell.KeyCtrlG {
			e.Message("Quit")
		}
		return true
	case tcell.KeyBackspace:
		// Backspace: dismiss, then let normal dispatch handle it.
		e.lspCompDismiss()
		return false
	default:
		// Any non-navigation key dismisses the popup and is handled normally.
		e.lspCompDismiss()
		return false
	}
}

func (e *Editor) lspCompNext() {
	if n := len(e.lspCompItems); n > 0 {
		if e.lspCompSelectedIdx < n-1 {
			e.lspCompSelectedIdx++
			if e.lspCompSelectedIdx >= e.lspCompOffset+lspCompMaxVisible {
				e.lspCompOffset++
			}
		}
	}
}

func (e *Editor) lspCompPrev() {
	if e.lspCompSelectedIdx > 0 {
		e.lspCompSelectedIdx--
		if e.lspCompSelectedIdx < e.lspCompOffset {
			e.lspCompOffset--
		}
	}
}

func (e *Editor) lspCompDismiss() {
	if e.lspCompDelayCancel != nil {
		e.lspCompDelayCancel()
	}
	_, e.lspCompDelayCancel = context.WithCancel(context.Background())
	e.lspCompActive = false
	e.lspCompItems = nil
	e.lspCompSelectedIdx = 0
	e.lspCompOffset = 0
}

// lspCompletionInsert replaces the word prefix before point with the selected
// completion label.
func (e *Editor) lspCompletionInsert() {
	if !e.lspCompActive || len(e.lspCompItems) == 0 {
		return
	}
	item := e.lspCompItems[e.lspCompSelectedIdx]
	text := item.InsertText
	if text == "" {
		text = item.Label
	}
	buf := e.ActiveBuffer()
	pt := buf.Point()
	prefixLen := max(0, pt-e.lspCompWordStart)
	buf.Delete(e.lspCompWordStart, prefixLen)
	buf.InsertString(e.lspCompWordStart, text)
	buf.SetPoint(e.lspCompWordStart + len([]rune(text)))
}

// ---- rendering --------------------------------------------------------------

const lspCompMaxVisible = 6

// renderLSPCompletionPopup draws the completion popup in the active window
// just below the cursor position.  Called from Redraw().
// The popup has a thin green border using FaceCompletionBorder.
func (e *Editor) renderLSPCompletionPopup() {
	if !e.lspCompActive || len(e.lspCompItems) == 0 || e.term == nil {
		return
	}
	w := e.activeWin
	buf := w.Buf()
	pt := w.Point()
	line, col := buf.LineCol(pt)
	cursorRow := w.Top() + (line - w.ScrollLine())
	cursorCol := w.Left() + col

	offset := e.lspCompOffset
	end := min(offset+lspCompMaxVisible, len(e.lspCompItems))
	visible := e.lspCompItems[offset:end]
	nVisible := len(visible)

	// Determine popup content width from the widest visible label.
	popupW := 20
	for _, item := range visible {
		label := item.Label
		if item.Detail != "" {
			label += "  " + item.Detail
		}
		if l := len([]rune(label)); l+2 > popupW {
			popupW = l + 2
		}
	}

	// Total popup height including top + bottom border rows is nVisible+2.

	// The popup always appears below the cursor.
	// barRows: number of rows the modeline bar occupies at the bottom of the window.
	winH := w.Height()
	barRows := 1
	if winH >= 3 {
		barRows = 2
	}
	if winH >= 4 {
		barRows = 3
	}
	maxRow := w.Top() + w.Height() - barRows - 1 // last usable text row

	// Top border one row below the cursor; first item one row below that.
	borderTop := cursorRow + 1
	contentRow := cursorRow + 2

	// If the popup would extend past the last usable row, cap it.
	// Never flip to above — just show fewer items.
	if borderTop > maxRow {
		return // no room at all
	}
	if contentRow+nVisible > maxRow {
		nVisible = maxRow - contentRow + 1
		if nVisible <= 0 {
			return
		}
		visible = visible[:nVisible]
	}

	borderBot := contentRow + nVisible

	totalW, totalH := e.term.Size()

	// Clamp left edge so popup fits horizontally; +2 for left+right borders.
	left := max(0, min(cursorCol, totalW-popupW-2))

	border := syntax.FaceCompletionBorder
	candidate := syntax.FaceCandidate

	// Top border: ╭─ ─ ─ ─ ─╮
	e.term.SetCell(left, borderTop, '╭', border)
	for j := 1; j <= popupW; j++ {
		e.term.SetCell(left+j, borderTop, '─', border)
	}
	e.term.SetCell(left+popupW+1, borderTop, '╮', border)

	// Content rows with left │ and right │ borders.
	for i, item := range visible {
		row := contentRow + i
		if row >= totalH-1 {
			break
		}
		face := candidate
		if offset+i == e.lspCompSelectedIdx {
			face = syntax.FaceSelected
		}
		label := item.Label
		if item.Detail != "" {
			label += "  " + item.Detail
		}
		runes := []rune(label)
		e.term.SetCell(left, row, '│', border)
		for j := 0; j < popupW; j++ {
			ch := ' '
			if j < len(runes) {
				ch = runes[j]
			}
			e.term.SetCell(left+1+j, row, ch, face)
		}
		e.term.SetCell(left+popupW+1, row, '│', border)
	}

	// Bottom border: ╰─ ─ ─ ─ ─╯
	if borderBot < totalH-1 {
		e.term.SetCell(left, borderBot, '╰', border)
		for j := 1; j <= popupW; j++ {
			e.term.SetCell(left+j, borderBot, '─', border)
		}
		e.term.SetCell(left+popupW+1, borderBot, '╯', border)
	}
}

// ---- LSP protocol helpers ---------------------------------------------------

// parseCompletionItems parses a textDocument/completion response which may be
// a CompletionList {"items":[...]} or a bare []CompletionItem.
func parseCompletionItems(raw json.RawMessage) []lsp.CompletionItem {
	if raw == nil || string(raw) == "null" {
		return nil
	}
	// Try CompletionList first.
	var list struct {
		Items []lsp.CompletionItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err == nil && list.Items != nil {
		return list.Items
	}
	// Try bare array.
	var items []lsp.CompletionItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items
	}
	return nil
}
