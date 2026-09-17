package editor

import (
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
)

// dapAdapterKind selects how the debug adapter for a language mode is reached.
type dapAdapterKind int

const (
	// dapAdapterProcess spawns dapCmd as a child process which then speaks DAP
	// over the reverse TCP connection dap.Start sets up (e.g. "dlv dap").
	dapAdapterProcess dapAdapterKind = iota
	// dapAdapterJdtls asks a running jdtls language server to start the
	// java-debug adapter and connects to the TCP port jdtls reports back.
	// eclipse.jdt.ls is not itself a debug adapter — see dapStartJdtls.
	dapAdapterJdtls
)

// langModeInfo describes a major language mode and its LSP server.
type langModeInfo struct {
	// modeName is the internal mode string stored on the buffer (e.g. "go").
	modeName string
	// lspCmd is the command and arguments to start the LSP server.
	// Empty means no LSP support for this mode.
	lspCmd []string
	// dapCmd is the command and arguments to start the DAP debug adapter.
	// Empty means no DAP support for this mode, unless dapKind names an adapter
	// that is not reached by spawning a process.
	dapCmd []string
	// dapKind is how the debug adapter is obtained.  The zero value spawns
	// dapCmd as a child process.
	dapKind dapAdapterKind
	// rootMarkers are filenames that indicate the project root when walking
	// upward from the file's directory (e.g. "go.mod", "pyproject.toml").
	rootMarkers []string
}

// hasDebugAdapter reports whether debug-start has any way of obtaining a debug
// adapter for this mode.
func (i *langModeInfo) hasDebugAdapter() bool {
	return len(i.dapCmd) > 0 || i.dapKind != dapAdapterProcess
}

// dapAdapterName returns a human-readable name for the mode's debug adapter,
// used in the "Debugger: starting …" message.
func (i *langModeInfo) dapAdapterName() string {
	if len(i.dapCmd) > 0 {
		return i.dapCmd[0]
	}
	if i.dapKind == dapAdapterJdtls {
		return "jdtls"
	}
	return "debug adapter"
}

// langModes lists every supported language mode.
var langModes = []langModeInfo{
	{modeName: "go", lspCmd: []string{"gopls"}, dapCmd: []string{"dlv", "dap"}, rootMarkers: []string{"go.mod", "go.work"}},
	{modeName: "python", rootMarkers: []string{"pyproject.toml", "setup.py", "setup.cfg"}},
	{modeName: "java", dapKind: dapAdapterJdtls, rootMarkers: []string{"pom.xml", "build.gradle", "build.gradle.kts"}},
	{modeName: "bash", rootMarkers: []string{}},
	{modeName: "perl", rootMarkers: []string{}},
	{modeName: "gherkin", rootMarkers: []string{}},
	{modeName: "markdown", rootMarkers: []string{}},
	{modeName: "elisp", rootMarkers: []string{}},
	{modeName: "json", rootMarkers: []string{}},
	{modeName: "yaml", rootMarkers: []string{}},
	{modeName: "makefile", rootMarkers: []string{}},
	{modeName: "conf", rootMarkers: []string{}},
	{modeName: "text", rootMarkers: []string{}},
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
	e.markVisualLinesDirty()
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

// cmdTextMode activates Text mode (plain-text spell-checking, no syntax highlighting).
func (e *Editor) cmdTextMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "text")
	e.Message("text-mode")
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

// cmdYamlMode activates YAML mode on the current buffer.
func (e *Editor) cmdYamlMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "yaml")
	e.Message("yaml-mode")
}

// cmdMakefileMode activates Makefile mode on the current buffer.
func (e *Editor) cmdMakefileMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "makefile")
	e.Message("makefile-mode")
}

// cmdConfMode activates Conf mode on the current buffer.
func (e *Editor) cmdConfMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "conf")
	e.Message("conf-mode")
}

// cmdPerlMode activates Perl mode on the current buffer.
func (e *Editor) cmdPerlMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "perl")
	e.Message("perl-mode")
}

// cmdGherkinMode activates Gherkin mode on the current buffer.
func (e *Editor) cmdGherkinMode() {
	e.clearArg()
	e.setLangMode(e.ActiveBuffer(), "gherkin")
	e.Message("gherkin-mode")
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
