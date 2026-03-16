# What is gomacs?

Simple TTY only Emacs clone written in Go.

<img src="doc/gomacs-screenshot.png" alt="gomacs"/>

# Which features?

- Open and write files.
- All files are read and written as UTF-8
- Search, forwards and backwards.
- Syntax highlighting and indention engine of some languages,
  including Go, Emacs Lisp, Markdown and BASH.
- Self documenting features, like <kbd>C-h f</kbd> `describe-function`
  and <kbd>C-h k</kbd> `describe-key`

# LSP support

<img src="doc/gomacs-lsp-auto-complete.png" alt="gomacs lsp"/>

- Show method signature in minibuffer, eldoc style
- Auto completion
- Find definition at point

# M-x completion

<img src="doc/gomacs-m-x.png" alt="gomacs M-x"/>

- LRU (last recently used) sort
- Fuzzy search

# How to run it?

```perl
$ make install
$ ~/.local/bin/gomacs
```

# Why gomacs?

I use this project for exploring what an LLM can do. This program is
not made by me, or coded by me. It's my idea, I've written the spec
and instructions to the robot, but from there on, the implementation
is all done by the LLM.

I take no pride in this because I didn't make it myself. I do find it
amusing, though, to see how far the LLM technology has come and what
it's possible to make it do, granted I can write precise (and terse!)
specifications. As input, I haven't used any Emacs source code
(although the LLM is probably trained on loads of open source
software, Emacs included). Instead, I've fed it the Emacs tutorial and
Emacs info manual, among other things.

If you don't like this, by all means move along, the [real GNU
Emacs](https://gnu.org/software/emacs) is indefinitely more powerful
and usable and is available on all platforms under the sun. The only
reason for using `gomacs` would be if you need a small and simple,
self documenting editor with Emacs shortcuts, that can be downloaded
and run as a single binary. That's it. For all other use cases you'll
be better off using the real thing.

Cheers,

-Torstein

# Known issues

## vc-log

VC log is without any syntax highlighting. The commit SHAs should be
coloured.

## vc-next-action

The `*VC commit*` buffer should be called `*vc-commit*` and it should
list the file names that will be included in this change list. Also,
it should have syntax highlighting.

Also, it doesn't `git add` the files that are changed, making the
later `git commit` fail.

# Missing features

## Version Control Annotate
`vc-annotate` bound to `C-x v g` should run `git blame <file>` when
the repo is Git. The annotation format in a new buffer `*vc-annotate*`
should be like:

```go
7e12ad49 (Torstein Krause Johansen 2026-03-15 11) // this comment is from the source code
```

Hitting `l` on the commit should open a new buffer showing that commit
commit message and headers. `d` should show the commit diff in a
buffer called `*vc-diff*`.
