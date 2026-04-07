# gomacs on Android

Using [termux](https://termux.dev/en/), install, `git`, `go` and `make`:
```text
$ pkg install git
$ pkg install golang
$ pkg install make
```

Then, clone `gomacs`, build and run it with these commands:

```text
$ git clone https://github.com/skybert/gomacs
$ cd gomacs
$ make run
```

## Screenshots

## vc diff

<a href="gomacs-android-git-diff.png">
  <img
    src="gomacs-android-git-diff.png"
    alt="gomacs on android"
    style="height: 500px"
  />
</a>

## Go with LSP

Given that you've installed `gopls` in Termux with:
```text
$ pkg install gopls
```

Gomacs provides a deceint Go editing enviornment. It'll work out of
the box with the `gopls` LSP server, just open a `.go` file and edit
away:

<a href="gomacs-android-go-lsp.png">
  <img
    src="gomacs-android-go-lsp.png"
    alt="gomacs on android"
    style="height: 500px"
  />
</a>

Admittedly, the auto complete box is a tad too wide for the default
portrait mode on my phone:

<a href="gomacs-android-go-lsp-landscape.png">
  <img
    src="gomacs-android-go-lsp-landscape.png"
    alt="gomacs on android"
    style="height: 500px"
  />
</a>

Turning the phone over to landscape mode, though, makes it look pretty decent:

<a href="gomacs-android-man-ls.png">
<img
   src="gomacs-android-man-ls.png"
    alt="gomacs on android"
    style="height: 500px"
  />
  </a>

## Markdown support

<a href="gomacs-android-markdown.png">
  <img
    src="gomacs-android-markdown.png"
    alt="gomacs on android"
    style="height: 500px"
  />
</a>

## Resource usage

In this `top`, you'll see `gomacs` and `emacs` (with `go-mode` and
`eglot`) editing one `.go` file, connecting to an external `gopls`
process for LSP completion:

<a href="gomacs-android-top.png">
  <img
    src="gomacs-android-top.png"
    alt="gomacs on android"
    style="height: 500px"
  />
</a>

Of course, `emacs` has a hundred thousand times more features than
`gomacs`, but it's fun comparing them nevertheless.
