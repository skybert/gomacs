package editor

import (
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
)

// langModeInfo describes a major language mode and its LSP server.
type langModeInfo struct {
	// modeName is the internal mode string stored on the buffer (e.g. "go").
	modeName string
	// lspCmd is the command and arguments to start the LSP server.
	// Empty means no LSP support for this mode.
	lspCmd []string
	// rootMarkers are filenames that indicate the project root when walking
	// upward from the file's directory (e.g. "go.mod", "pyproject.toml").
	rootMarkers []string
}

// langModes lists every supported language mode.
var langModes = []langModeInfo{
	{modeName: "go", lspCmd: []string{"gopls"}, rootMarkers: []string{"go.mod", "go.work"}},
	{modeName: "python", rootMarkers: []string{"pyproject.toml", "setup.py", "setup.cfg"}},
	{modeName: "java", rootMarkers: []string{"pom.xml", "build.gradle"}},
	{modeName: "bash", rootMarkers: []string{}},
	{modeName: "markdown", rootMarkers: []string{}},
	{modeName: "elisp", rootMarkers: []string{}},
	{modeName: "json", rootMarkers: []string{}},
	{modeName: "makefile", rootMarkers: []string{}},
	{modeName: "fundamental", rootMarkers: []string{}},
}

// langModeByName returns the langModeInfo for the given mode name, or nil.
func langModeByName(name string) *langModeInfo {
	for i := range langModes {
		if langModes[i].modeName == name {
			return &langModes[i]
		}
	}
	return nil
}

// setLangMode sets buf's mode and activates LSP if the mode supports it and
// buf has an associated file.
func (e *Editor) setLangMode(buf *buffer.Buffer, mode string) {
	buf.SetMode(mode)
	if buf.Filename() != "" {
		e.lspActivate(buf)
	}
}

// cmdGoMode activates Go mode on the current buffer.
func (e *Editor) cmdGoMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "go")
	e.Message("go-mode")
}

// cmdPythonMode activates Python mode on the current buffer.
func (e *Editor) cmdPythonMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "python")
	e.Message("python-mode")
}

// cmdJavaMode activates Java mode on the current buffer.
func (e *Editor) cmdJavaMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "java")
	e.Message("java-mode")
}

// cmdBashMode activates Bash mode on the current buffer.
func (e *Editor) cmdBashMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "bash")
	e.Message("bash-mode")
}

// cmdMarkdownMode activates Markdown mode on the current buffer.
func (e *Editor) cmdMarkdownMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "markdown")
	e.Message("markdown-mode")
}

// cmdElispMode activates Emacs Lisp mode on the current buffer.
func (e *Editor) cmdElispMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "elisp")
	e.Message("elisp-mode")
}

// cmdFundamentalMode activates Fundamental mode (no syntax or indentation).
func (e *Editor) cmdFundamentalMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "fundamental")
	e.Message("fundamental-mode")
}

// cmdJsonMode activates JSON mode on the current buffer.
func (e *Editor) cmdJsonMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "json")
	e.Message("json-mode")
}

// cmdMakefileMode activates Makefile mode on the current buffer.
func (e *Editor) cmdMakefileMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "makefile")
	e.Message("makefile-mode")
}

// modeIndentStr returns the per-level indent string for the given major mode.
// The value is read from the Elisp global variable "<mode>-indent" (bash uses
// "sh-indent").  An Int value is expanded to that many spaces; a StringVal is
// used verbatim.  If the variable is unset, sensible defaults are returned:
// "\t" for Go, two spaces for everything else.
func (e *Editor) modeIndentStr(mode string) string {
	varName := mode + "-indent"
	if mode == "bash" {
		varName = "sh-indent"
	}
	if v, ok := e.lisp.GetGlobalVar(varName); ok {
		switch val := v.(type) {
		case elisp.Int:
			if val.V > 0 {
				return strings.Repeat(" ", int(val.V))
			}
		case elisp.StringVal:
			if val.V != "" {
				return val.V
			}
		}
	}
	if mode == "go" {
		return "\t"
	}
	return "  "
}
