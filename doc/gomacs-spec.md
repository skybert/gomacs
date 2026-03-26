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


## vc-grep

`vc-grep` should not wrap its lines. visual line mode should not be
active in `vc-grep` buffers.

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

# LSP - language server protocol

## lsp-find-references

`lsp-find-references`. By default bound to `M-?`.

# Performance

Optimise `gomacs` speed. Focus on the user experience, how fast the
user can input text and scroll in `gomacs`.

# Help system

Ensure that all functions and variables are listed and logically
grouped in `M-x help`.


# Unit tests

Ensure all go code is unit tested, and these tests are in the
corresponding `_test.go` file of that file they're testing.

# Man page integration

`M-x man` should offer tab completion of the available man pages in `$MANPATH`.



