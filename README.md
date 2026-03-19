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
- `visual-line-mode` enabled by default: lines longer than 80 characters
  wrap visually (file content unchanged). Disable with `(setq visual-lines nil)`.
- Spell checking with `aspell` (auto-enabled for `markdown`, `text`,
  `fundamental` modes; only comments checked for code modes).
  Configurable with `(setq spell-command "/usr/bin/aspell")`.
  `M-x spell` for interactive spell checking. Misspelled words are underlined.
- `forward-list` (<kbd>C-M-n</kbd>) navigates to the matching closing
  bracket/paren/brace, or closing sh keyword (`fi`, `done`, `esac`) in bash mode.
- `backward-list` (<kbd>C-M-p</kbd>) navigates to the matching opening
  bracket/paren/brace, or opening sh keyword in bash mode.

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

None currently known.

# Missing features

None currently known.
