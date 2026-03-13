# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make              # fmt → lint → test → vulncheck → build (default)
make build        # gofmt + go build -o build/gomacs
make test         # go test ./...
make lint         # golangci-lint run ./...
make fmt          # gofmt -w -s .

# Run a single test
go test ./internal/editor/ -run TestSplitWindowRight
go test ./internal/syntax/ -run TestElispHighlighter

# Run the editor (skip init file)
./build/gomacs -Q somefile.go
```

## Architecture

The project is a TTY-only Emacs clone in Go. The binary is `gomacs`; the module is `github.com/skybert/gomacs`.

### Package responsibilities

| Package | Role |
|---|---|
| `internal/buffer` | Gap-buffer text storage with undo ring, mark, read-only flag. Positions are **rune indices**, not bytes. |
| `internal/keymap` | Layered `ModKey → Binding` maps. A `Binding` is either a command name (string) or a sub-`Keymap` (prefix key). |
| `internal/terminal` | Thin wrapper around `tcell.Screen`. `SetCell`/`DrawString` take `syntax.Face` values. Hex `#rrggbb` colors are supported. |
| `internal/window` | Rectangular view into a buffer: screen region, per-window scroll line, goal column for vertical motion, `ViewLines()` for rendering. |
| `internal/syntax` | Stateless `Highlighter` interface returning `[]Span`. Hand-written scanners for Go, Markdown, Elisp, Python, Java, Bash. Package-level `Face` vars are mutated by `LoadTheme`. |
| `internal/elisp` | Lisp-2 evaluator: `Lexer → Parser (cons cells) → Evaluator`. Go functions are registered via `RegisterGoFn(name, func([]Value, *Env))`. |
| `internal/editor` | Top-level wiring. `Editor` owns everything; `editor.go` handles the event loop, rendering, minibuffer, and isearch. `commands.go` holds all `cmd*` functions registered in `init()`. `indent.go` holds Elisp auto-indentation logic. |

### Data flow

```
tcell event → terminal.ParseKey → keymap.Lookup → execCommand(name) → cmd*()
                                                                         ↓
                                                              buffer mutations
                                                                         ↓
                                                              Redraw() → renderWindow() + renderModeline()
                                                                       → renderMinibuffer() / renderCandidatePopup()
```

### Key conventions

**Buffer positions** — all offsets are rune indices. `buf.LineCol(pt)` returns 1-based line and 0-based column.

**Window rendering** — `w.Left()` must be added to every screen column when calling `term.SetCell` or `term.DrawString`. The separator column between side-by-side windows is drawn in `Redraw()` at `rightWin.Left()-1`; `cmdSplitWindowRight` reserves this column by making each pane 1 column narrower.

**Minibuffer completion popup** — `Editor.minibufCompletions` is a `func(string) []string` set via `SetMinibufCompletions` after `ReadMinibuffer`. Candidates are lazily refreshed (fuzzy subsequence match) before each render; navigation uses `minibufSelectedIdx` / `minibufCandidateOffset`.

**Command registration** — every command is registered once in `editor/commands.go`'s `init()` with `registerCommand(name, fn, doc)`. The command name used in `init()` must exactly match any keymap binding in `setupKeymaps()`.

**Elisp integration** — Go features are exposed to init.el via `e.lisp.RegisterGoFn("name", func([]Value, *Env))`. `(load-theme 'sweet)` and `(global-set-key (kbd "…") 'command-name)` are the primary extension points.

**Colour themes** — `syntax.Face` variables are package-level vars mutated by `syntax.LoadTheme(name)`. The Sweet theme is applied at startup. Theme functions live in `internal/syntax/theme.go`.

### Lint notes

The project uses a strict `.golangci.yml`. Common suppressions you may need:
- `//nolint:exhaustive` on switches over external tcell enums that have a `default:` case.
- `//nolint:gosec` is already excluded for `G304` (intentional user-provided file opens).
- Repeated string literals → extract a `const` (goconst triggers at 3 occurrences).
- Nested `if`/`else if` chains → rewrite as `switch` (gocritic `ifElseChain`).
- `for i := 0; i < n; i++` over integers → `for range n` (intrange).
- `if x > max { x = max }` → `x = min(x, max)` (modernize).
