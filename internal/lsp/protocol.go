// Package lsp provides a minimal Language Server Protocol client.
package lsp

import (
	"fmt"
	"path/filepath"
)

// URI is a document URI ("file:///absolute/path").
type URI = string

// FileURI converts an absolute filesystem path to a file:// URI.
func FileURI(path string) URI {
	return "file://" + filepath.ToSlash(path)
}

// PathFromURI strips the "file://" prefix from a file URI.
func PathFromURI(uri URI) string {
	if len(uri) >= 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}

// Position is a 0-based (line, UTF-16-character) pair.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a [Start, End) span within a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a URI + Range.
type Location struct {
	URI   URI   `json:"uri"`
	Range Range `json:"range"`
}

// Diagnostic severity constants.
const (
	SeverityError   = 1
	SeverityWarning = 2
	SeverityInfo    = 3
	SeverityHint    = 4
)

// Diagnostic is a compiler/linter message attached to a source range.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
}

// CompletionItem is one entry in a completion list.
type CompletionItem struct {
	Label      string `json:"label"`
	Detail     string `json:"detail,omitempty"`
	InsertText string `json:"insertText,omitempty"`
	Kind       int    `json:"kind,omitempty"`
}

// SeverityLabel returns a short one-letter label for a diagnostic severity.
func SeverityLabel(s int) string {
	switch s {
	case SeverityError:
		return "E"
	case SeverityWarning:
		return "W"
	case SeverityInfo:
		return "I"
	default:
		return fmt.Sprintf("%d", s)
	}
}

// UTF16Offset converts a 0-based rune-column within lineText to the UTF-16
// code-unit offset that LSP expects in Position.Character.
func UTF16Offset(lineText string, runeCol int) int {
	col := 0
	i := 0
	for _, r := range lineText {
		if i >= runeCol {
			break
		}
		if r >= 0x10000 {
			col += 2
		} else {
			col++
		}
		i++
	}
	return col
}
