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

<img
  src="gomacs-android-git-diff.png"
  alt="gomacs on android"
  style="width: 800px"
/>

## Go with LSP

Given that you've installed `gopls` in Termux with:
```text
$ pkg install gopls
```

Gomacs provides a deceint Go editing enviornment. It'll work out of
the box with the `gopls` LSP server, just open a `.go` file and edit
away:

<img
  src="gomacs-android-go-lsp.png"
  alt="gomacs on android"
  style="height: 400px"
/>

Admittedly, the auto complete box is a tad too wide for the default
portrait mode on my phone:

<img
  src="gomacs-android-go-lsp-landscape.png"
  alt="gomacs on android"
  style="height: 400px"
/>

Turning the phone over to landscape mode, though, makes it look pretty decent:

<img
  src="gomacs-android-man-ls.png"
  alt="gomacs on android"
  style="height: 400px"
/>

## Markdown support

<img
  src="gomacs-android-markdown.png"
  alt="gomacs on android"
  style="height: 400px"
/>

## Resource usage

In this `top`, you'll see `gomacs` and `emacs` (with `go-mode` and
`eglot`) editing one `.go` file, connecting to an external `gopls`
process for LSP completion:

<img
  src="gomacs-android-top.png"
  alt="gomacs on android"
  style="height: 400px"
/>

Of course, `emacs` has a hundred thousand times more features than
`gomacs`, but it's fun comparing them nevertheless.
