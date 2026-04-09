# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make              # fmt → lint → test → vulncheck → build (default)
make build        # gofmt + go build -o build/gomacs
make build/shotgen  # build the screenshot generator tool
make test         # go test ./...
make lint         # golangci-lint run ./...
make fmt          # gofmt -w -s .

# Run a single test
go test ./internal/editor/ -run TestSplitWindowRight
go test ./internal/syntax/ -run TestElispHighlighter

# Coverage report (artifacts go to build/)
go test ./... -coverprofile=build/coverage.out
go tool cover -func=build/coverage.out

# Run the editor (skip init file)
./build/gomacs -Q somefile.go
```

**Important:** Always use `make` targets rather than bare `go build` commands.
Running `go build ./cmd/shotgen` without `-o` drops a stray `./shotgen` binary
in the repo root. Use `make build/shotgen` instead.

## Architecture

The project is a TTY-only Emacs clone in Go. The binary is `gomacs`; the module is `github.com/skybert/gomacs`.

### Package responsibilities

| Package | Role |
|---|---|
| `internal/buffer` | Gap-buffer text storage with undo ring, mark, read-only flag. Positions are **rune indices**, not bytes. |
| `internal/keymap` | Layered `ModKey → Binding` maps. A `Binding` is either a command name (string) or a sub-`Keymap` (prefix key). |
| `internal/terminal` | Thin wrapper around `tcell.Screen`. `SetCell`/`DrawString` take `syntax.Face` values. Hex `#rrggbb` colors are supported. |
| `internal/window` | Rectangular view into a buffer: screen region, per-window scroll line, goal column for vertical motion, `ViewLines()` for rendering. |
| `internal/syntax` | Stateless `Highlighter` interface returning `[]Span`. Hand-written scanners for Go, Markdown, Elisp, Python, Java, Bash, JSON. Package-level `Face` vars are mutated by `LoadTheme`. |
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

**Mode configuration via Elisp** — major-mode settings are read from the Elisp environment by `applyElispConfig()` (called after loading `~/.gomacs` / `init.el`). The following `setq` variables are supported:

| Variable | Type | Default | Effect |
|---|---|---|---|
| `auto-revert` | bool/nil | `t` | Auto-reload unmodified buffers when their file changes on disk |
| `fill-column` | integer | `70` | Column target for `fill-paragraph` (M-q) |
| `isearch-case-insensitive` | bool/nil | `t` | Case-insensitive isearch; set to `nil` for case-sensitive |
| `python-indent` | integer or string | `"  "` | Per-level indent for Python |
| `go-indent` | string | `"\t"` | Per-level indent for Go |
| `java-indent` | integer or string | `"  "` | Per-level indent for Java |
| `sh-indent` | integer or string | `"  "` | Per-level indent for Bash (`bash-indent` is not the name; use `sh-indent`) |
| `json-indent` | integer or string | `"  "` | Per-level indent for JSON |
| `markdown-indent` | integer or string | `"  "` | Per-level indent for Markdown |
| `screenshot-dir` | string | `""` (startup cwd) | Directory for `M-x screenshot` PNG output; created if absent |

Example `~/.gomacs`:
```elisp
(setq fill-column 80)
(setq python-indent 4)
(setq isearch-case-insensitive nil)  ; restore case-sensitive search
```

**isearch case folding** — isearch is case-insensitive by default (`isSearchCaseFold = true`).
Set `(setq isearch-case-insensitive nil)` in `~/.gomacs` to restore case-sensitive search.
`applyElispConfig()` reads this variable after loading the init file.

## Configuration

All configuration variables are documented in the man page in
`doc/gomacs.1.in`. The configuration keys are on the form
`<mode>-<var>`, so for `go-mode`, the indent setting is set using:

```lisp
(setq go-indent 2)
```

### Lint notes

The project uses a strict `.golangci.yml`. Common suppressions you may need:
- `//nolint:exhaustive` on switches over external tcell enums that have a `default:` case.
- `//nolint:gosec` is already excluded for `G304` (intentional user-provided file opens).
- Repeated string literals → extract a `const` (goconst triggers at 3 occurrences).
- Nested `if`/`else if` chains → rewrite as `switch` (gocritic `ifElseChain`).
- `for i := 0; i < n; i++` over integers → `for range n` (intrange).
- `if x > max { x = max }` → `x = min(x, max)` (modernize).

### Testing

Every non-trivial Go function must have a corresponding unit test.

**Rules:**
- All new code must ship with tests in the same PR.
- Tests live in `_test.go` files in the same package (white-box testing preferred).
- Use `newTestEditor(content)` (defined in `internal/editor/commands_test.go`) to build a headless editor — no real terminal needed.
- Build artefacts (coverage profiles, binaries) go under `build/`; never commit them.
- The `terminal` package wraps tcell's screen directly; test only the pure helpers (`parseColor`, `faceToStyle`, `ParseKey`) — do not try to spin up a real screen in tests.
- When adding a highlighter to `internal/syntax`, add a `*_test.go` alongside it covering keywords, strings, comments, and numbers.

### Configuration variables

When adding a new Elisp configuration variable (i.e. a new `GetGlobalVar` call in
`applyElispConfig()` in `internal/editor/editor.go`), you **must** also:

1. Add an entry to the `configVars` slice in `cmdHelp()` in `internal/editor/nav.go`
   so that `M-x help` lists it with a description and its current value.
2. Add a row to the configuration table in `doc/gomacs.1.in` (the man page).
3. Update the `**Mode configuration via Elisp**` table in this file if the variable
   is mode-specific, or add it as a standalone bullet if it is global.
