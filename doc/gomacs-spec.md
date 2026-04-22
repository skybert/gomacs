# User prompts

All yes/no questions should be answerable with `y` and `n`.

# File system

If a file is updated outside of `gomacs`, the contents of that file in an open buffer should be automatically updated. This can be turned off with:

```lisp
(setq auto-revert nil)
```

# Selection

The selection colour should be the green colour of the current theme.

# Version control

## vc-status

In `vc-status`, when selecting c to commit on the "Changes not stage
for commit" header, it allows you to type a commit message, but this
doesn't make any sense, since there are now staged files. instead,
when hitting `c` on the "Changes not stage for commit", `gomacs` should
ask the user if he wants to stage all these files (since it's the
header of the files) and only if the answer is `y` (yes), should it
stage them and open the commit message buffer.

Moreover, `l` should invoke `vc-print-log`, `d` should should the diff
on modified files, `s` should stage modified, unstaged files. `c`
should open a VC commit buffer with comments showing the staged files
that will be included in the commit.

Pressing `f` should make a fixup commit. The user should be prompted
with the list of current commits (the same screen as `vc-print-log`)
on which to attach the fixup. `C-c C-c` on the line of that commit
should complete the fixup flow.

## vc-grep

`vc-grep` should not wrap its lines. visual line mode should not be
active in `vc-grep` buffers.

# Spell checker

`ispell-buffer` should start a spell checker session for the entire
buffer. `ispell-word` should only spell check the current word.

# Clipboard

When copying text in `gomacs`, it SHOULD go on the OS clipboard as
well. It MUST support Linux/Xorg, Linux/Wayland and macOS.

# Search

`C-s` should trigger `isearch` (incremental) search. It should jump to
the first match as the user types. Consecutive invocations of `C-s`
should jump to the next occurrence in the buffer.

When having focus on a search hit, hitting `C-w` will include one more
word in the search. So, if the `C-s` for the word "Camel" takes the
point to the first word in "CamelCase", hitting `C-w` will bring the
next compound word "Case" into the search, as if the user had typed
"CamelCase" to begin with.  Continuously hitting `C-w` will include more
and more words into the search.

Hitting `C-r` will search for the current query backwards, or upwards,
in the buffer.

Once the search hits top or bottom, it should write so in the
minibuffer.
# Project commands

## project-find-file

`project-find-file` should give a fuzzy search menu, like `find-file`,
for files in the current project. Use the VC backend to determine
where the project file boundaries are. As with all other completion
lists in `gomacs`, the list should be LRU sorted.

The default binding for `project-find-file` should be `C-x p f`.


## project-grep

`project-grep` should have the default binding of `C-x p g`. It should
call `vc-grep` if there's a VC backend, if not use a standard grep command should be used:
```
grep -R -i -n
```

The search hits should be presented in a buffer called `*grep*`
they+should hook onto the `next-error` function that the
`*compilation*`+buffer has, letting the user quickly going through
each of the hits.+The `next-error` function should be bound to `C-x \``
(Ctrl + backtick). tests

# Minibuffer

The code API doc in the minibuffer should have syntax highlighting.

# Auto completion

When files in `gomacs`, it should offer an auto completion box
(intellisense) after the user has typed three characters (including
".") under the following circumstances:

When being in a mode with LSP support, like `go-mode`, the LSP
server should provide the completion candidates. By default, `gomacs`
should display the menu after `3` characters, i.e. when the user has
typed `os.` to list the functions available in the `os` Go package.

When editing a file which does not have an LSP backend, offer
completion candidates based on words in the current buffer. e.g. when
the user edits an `.md` file, in which it somewhere says "beautiful",
when the user types "bea", `gomacs` should offer the completion menu
with the candidate "beautiful".  When showing the completion menu in
non-programming modes, or in comments in programming buffers, the
completion menu shouldn't show immediately as it'll break the flow of
someone touch typing (fast) into the buffer.

The number of characters the user has to type before triggering the
completion menu can be specified with:
```lisp
(setq completion-menu-trigger-chars 3)
```

The completion box should have a thin grey border, using the grey
colour defined in the current them, to differentiate it from the rest
of the buffer while not overwhelming the user interface.

The user can navigate up and down in the completion with either the
arrow up and down keys, or with `M-n` (next) and `M-p` (previous).
Pressing `Tab` or `Enter` selects the current candidate. `C-g` closes
the completion menu without inserting any candidate, but leaving the
point at where the user was typing before the menu was triggered.

# conf-mode

`gomacs` should have `conf-mode` for editing configuration files. It
shouldn't have any indentation logic, but should provide syntax
highlighting. `conf-mode` should by default be enabled for these
files:

- `.conf`
- Files ending with `rc`
- `.toml`

`conf-mode`, like other programming modes, should enable on the fly
spell checking of comments. Comments are lines starting with a `#`
symbol.

# LSP - language server protocol

## lsp-find-references

`lsp-find-references`. By default bound to `M-?`.

# DAP - debugger support

`gomacs` should support the DAP protocol and provide a good, out of
the box experience for a pre-defined list of debuggers. The following
should work out of the box:
- `go-mode`: [dlv](https://github.com/go-delve/delve)
- `java-mode`:
[jdtls](https://github.com/eclipse-jdtls/eclipse.jdt.ls)

The following commands should be available:
- `debug-toggle-breakpoint`
- `debug-step-next`, `n`
- `debug-step-in`, `i`
- `debug-step-out`, `o`
- `debug-continue`, `c`
- `debug-eval`, `e`, which evaluates the thing at point, or if the
  region is active, the region.
- `debug-exit`, `q`, exits the debugging session.

When the debugger is active, invoked with `debug-start`, the source
buffers should be switched to read only, so that the user can use
single letter shortcuts to navigate.

Invoking `debug-start` in a unit test, `foo_test.go`, it should run the
test method in which the cursor is. If the cursor is on a class, or
outside any function, the entire file is debugged. In the same way,
when starting the debugger in a main class, like `main.go` or a Java
file with `void main(String args[])`, it should understand that.

The third context in which it should be aware, is to start a micro
server with appropriate debug flags so that it can break on the break
points set.

Break points should be enabled with `M-x debug-toggle-breakpoint`. Active
break points should be shown in a gutter on the left. This gutter should
be updated when stepping through the code, using a Unicode right arrow
symbol, or similar.

When running the debugger, additonal buffers should be added to the UI,
where each of these are under each other, thus only occupying one column
on the right.
- Local browser, with expandable structs/objects. Where there data
  structure isn't too deep (configurable with `(setq
  debug-locals-auto-expand-depth)`, the browser should auto expand them.
- Call stack browser, for all threads.

At the bottom of the screen, one large buffer with a REPL should be
visible. The REPL should have syntax highlighting and auto completion,
similar to what's in the regular code buffer.

# Performance

`gomacs` should be very fast. It should provide a great the user
experience of fast editor, letting the user input text as fast as she
can type, and scroll up and down in buffers as fast as the operating
system keyboard settings allow.

# Help system

Ensure that all functions and variables are listed and logically
grouped in `M-x help`.

# Unit tests

All Go code MUST have unit tests. These tests MUST be written in
corresponding `_test.go` files of the file they're testing.

# Man page integration

`M-x man` should offer tab completion of the available man pages in
`$MANPATH`.

# Shebang detection

`gomacs` should detect file type and mode by reading the shebang.

It should enable `bash-mode` for files with either of these shebangs:

- `#!/usr/bin/env bash`
- `#!/bin/bash`
- `#! /usr/bin/env bash`
- `#! /bin/bash`

The same should go for:
- `perl`
- `sh`
- `python`, including binaries with major and minor version in them:
  `python3.10`

# Gherkin mode

`gomacs` should understand `.feature` files and enable `gherkin-mode`
for these.

It should wire into the xref system, with `M-.` and `M-,`, so that
when the user is on a step definition like: "Given user logs in", it
should jump to the corresponding implementation of that step. For
Go/gocuke, this means jumping to a function in the same package called
`UserLogsIn()`.

In addition to support for gocuke/Go, add support for the default Java
Gherkin framework where there are annotations like `@Given("user logs
in")`.

If there are more than one, list them in a regular `grep` buffer, with
links that can be cycled with the `next-error` system, bound to "C-x
`".

# Text modes

In pure text modes, like `text` and `markdown`, lines should be
automatically wrapped at `fill-column`. By default this is `70`
characters.

The user can configure this with:

```lisp
(setq fill-column 80)
```

# Built-in shell

`M-x shell` creates a buffer with a terminal. The shell should be
capable of running other terminal applications, like `top`, but the
following shortcuts should always be available to `gomacs` proper:

- `C-<space>` to set mark
- `C-v` to scroll down
- `C-x b` to switch buffer.
- `C-x k` to kill the shell buffer.
- `M-v` to scroll up
- `M-w` to copy active region.
- `M-x` to type in a command

The shell should start the shell interpreter set in the `$SHELL`
environment variable.

ANSI escape codes should be interpreted. For instance, colours should
be displayed as colours, and not the ANSI escapes themselves.

If there already is a `*shell*` buffer open and the user attempts to
create a new one, then an additional shell is created with the VC repo
name, e.g. `*shell/gomacs*`. If the current buffer is not in a VC
repo, use the basename of the current directory. If the
`*shell/<repo>*` already exists, jump to that buffer.
