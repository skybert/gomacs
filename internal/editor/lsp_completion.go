package editor

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/lsp"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---- completion state -------------------------------------------------------

// lspCompletionMinChars is the default minimum word-characters before
// completion triggers.  Overridden by (setq lsp-completion-min-chars N).
const lspDefaultCompletionMinChars = 2

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
// prefix before point is at least lspCompletionMinChars characters long and
// an LSP server is ready, it fires an async textDocument/completion request.
// It also triggers immediately when the last inserted character is a trigger
// character (e.g. '.') so that "os." shows os package members.
func (e *Editor) lspMaybeTriggerCompletion() {
	if e.lspCompInflight || e.lspCompActive {
		return
	}
	buf := e.ActiveBuffer()
	if buf.Filename() == "" {
		return
	}
	conn := e.lspConns[buf.Mode()]
	if conn == nil || !conn.isReady {
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

	if len([]rune(prefix)) < minChars && triggerChar == 0 {
		return
	}
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
				return
			}
			items := parseCompletionItems(result)
			if len(items) == 0 {
				return
			}
			// Re-check that prefix still matches (user may have kept typing).
			prefix2, start2 := lspCompWordPrefix(e.ActiveBuffer())
			if start2 != e.lspCompWordStart {
				// Word boundary shifted; discard stale results.
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

// lspCompletionHandleKey processes a key press while the completion popup is
// visible.  Returns true if the key was consumed, false if normal dispatch
// should continue.
func (e *Editor) lspCompletionHandleKey(ke terminal.KeyEvent) bool {
	if !e.lspCompActive {
		return false
	}
	switch ke.Key {
	case tcell.KeyTab, tcell.KeyDown:
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
	case tcell.KeyEnter:
		e.lspCompletionInsert()
		e.lspCompDismiss()
		return true
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
	prefixLen := pt - e.lspCompWordStart
	if prefixLen < 0 {
		prefixLen = 0
	}
	buf.Delete(e.lspCompWordStart, prefixLen)
	buf.InsertString(e.lspCompWordStart, text)
	buf.SetPoint(e.lspCompWordStart + len([]rune(text)))
}

// ---- rendering --------------------------------------------------------------

const lspCompMaxVisible = 6

// renderLSPCompletionPopup draws the completion popup in the active window
// just below the cursor position.  Called from Redraw().
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

	// Popup appears one row below the cursor.
	popupRow := cursorRow + 1
	// If that would overlap the modeline or minibuffer, show above instead.
	maxRow := w.Top() + w.Height() - 2 // last text row (before modeline)
	if popupRow > maxRow {
		popupRow = cursorRow - lspCompMaxVisible - 1
		if popupRow < w.Top() {
			popupRow = w.Top()
		}
	}

	_, totalH := e.term.Size()
	if popupRow < 0 || popupRow >= totalH-1 {
		return
	}

	offset := e.lspCompOffset
	end := min(offset+lspCompMaxVisible, len(e.lspCompItems))
	visible := e.lspCompItems[offset:end]

	// Determine popup width from the widest visible label.
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
	totalW, _ := e.term.Size()
	cursorCol = max(0, min(cursorCol, totalW-popupW-1))

	for i, item := range visible {
		row := popupRow + i
		if row >= totalH-1 {
			break
		}
		face := syntax.FaceCandidate
		if offset+i == e.lspCompSelectedIdx {
			face = syntax.FaceSelected
		}
		label := item.Label
		if item.Detail != "" {
			label += "  " + item.Detail
		}
		runes := []rune(label)
		// Pad or truncate to popupW.
		for j := 0; j < popupW; j++ {
			ch := ' '
			if j < len(runes) {
				ch = runes[j]
			}
			e.term.SetCell(cursorCol+j, row, ch, face)
		}
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
