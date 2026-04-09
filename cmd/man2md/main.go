// cmd/man2md converts doc/gomacs.1.in to doc/gomacs-user-guide.md and appends any
// screenshots found in doc/*.png.  Run via "make doc" from the project root.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	version := gitVersion()
	authors := readAuthors("AUTHORS")
	date := time.Now().Format("2006-01-02")

	md, err := convert("doc/gomacs.1.in", version, authors, date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "man2md: %v\n", err)
		os.Exit(1)
	}

	shots, _ := filepath.Glob("doc/*.png")
	sort.Strings(shots)

	platShots, _ := filepath.Glob("doc/*/*.png")
	sort.Strings(platShots)
	platMap := map[string][]string{}
	var platOrder []string
	for _, s := range platShots {
		dir := filepath.Base(filepath.Dir(s))
		if _, seen := platMap[dir]; !seen {
			platOrder = append(platOrder, dir)
		}
		platMap[dir] = append(platMap[dir], s)
	}

	if len(shots) > 0 || len(platShots) > 0 {
		md += "\n## Screenshots\n\n"
		for _, s := range shots {
			name := strings.TrimSuffix(filepath.Base(s), ".png")
			md += fmt.Sprintf("<img src=\"%s\" alt=\"%s\"/>\n\n", filepath.Base(s), name)
		}
		for _, plat := range platOrder {
			md += fmt.Sprintf("\n### %s\n\n", strings.ToTitle(plat[:1])+plat[1:])
			for _, s := range platMap[plat] {
				name := strings.TrimSuffix(filepath.Base(s), ".png")
				rel := plat + "/" + filepath.Base(s)
				md += fmt.Sprintf("<img src=\"%s\" alt=\"%s\"/>\n\n", rel, name)
			}
		}
	}

	if err := os.WriteFile("doc/gomacs-user-guide.md", []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "man2md: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("wrote doc/gomacs-user-guide.md")
}

func gitVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(out))
}

func readAuthors(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			names = append(names, line)
		}
	}
	return strings.Join(names, ", ")
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------

func convert(filename, version, authors, date string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		l = strings.ReplaceAll(l, "@VERSION@", version)
		l = strings.ReplaceAll(l, "@AUTHORS@", authors)
		l = strings.ReplaceAll(l, "@DATE@", date)
		lines = append(lines, l)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	p := &proc{lines: lines}
	p.run()
	return p.sb.String(), nil
}

// proc is the line-by-line man→Markdown state machine.
type proc struct {
	lines []string
	i     int
	sb    strings.Builder

	inTable   bool
	tableRows [][2]string

	inNF bool // inside .nf/.fi code block

	// Tagged paragraph (.TP) state.
	// tpPhase: 0=off, 1=awaiting term, 2=in definition (term already emitted).
	tpPhase int
}

func (p *proc) emit(s string) { p.sb.WriteString(s) }
func (p *proc) nl()           { p.sb.WriteByte('\n') }

func (p *proc) run() {
	for p.i < len(p.lines) {
		p.processLine(p.lines[p.i])
		p.i++
	}
	p.flushTP()
	if p.inNF {
		p.emit("```\n")
	}
	if p.inTable {
		p.flushTable()
	}
}

func (p *proc) processLine(line string) {
	// Collect table rows between .TS and .TE.
	if p.inTable {
		if line == ".TE" {
			p.inTable = false
			p.flushTable()
			return
		}
		if isTableFormat(line) {
			return
		}
		cols := strings.SplitN(line, "\t", 2)
		var row [2]string
		row[0] = inline(cols[0])
		if len(cols) > 1 {
			row[1] = inline(cols[1])
		}
		p.tableRows = append(p.tableRows, row)
		return
	}

	// Pass code-block lines through verbatim (no Markdown markup inside fences).
	if p.inNF {
		if line == ".fi" {
			p.emit("```\n")
			p.inNF = false
			return
		}
		p.emit(plainText(line) + "\n")
		return
	}

	// Macro lines start with '.'.
	if strings.HasPrefix(line, ".") {
		p.processMacro(line)
		return
	}

	// Plain text line.
	text := inline(line)
	if text == "" {
		p.nl()
		return
	}
	p.emitContent(text)
}

func (p *proc) processMacro(line string) {
	args := splitArgs(line)
	if len(args) == 0 {
		return
	}
	macro := args[0]
	rest := inline(strings.Join(args[1:], " "))

	switch macro {
	case ".TH":
		p.emit("# gomacs(1)\n")

	case ".SH":
		p.flushTP()
		p.emit("\n## " + rest + "\n\n")

	case ".SS":
		p.flushTP()
		p.emit("\n### " + rest + "\n\n")

	case ".PP":
		p.flushTP()
		p.nl()

	case ".TP":
		p.flushTP()
		p.tpPhase = 1

	case ".B":
		p.emitContent("**" + rest + "**")

	case ".I":
		p.emitContent("_" + rest + "_")

	case ".BR":
		p.emitContent(altFonts(args[1:], true))

	case ".RB":
		p.emitContent(altFonts(args[1:], false))

	case ".TS":
		p.inTable = true
		p.tableRows = nil

	case ".TE":
		p.inTable = false
		p.flushTable()

	case ".nf":
		p.emit("\n```\n")
		p.inNF = true

	case ".fi":
		p.emit("```\n")
		p.inNF = false

		// Everything else (comments, spacing macros, etc.) is ignored.
	}
}

// emitContent writes text honouring the current tagged-paragraph state.
// When tpPhase==1 the text becomes the bold term header; when tpPhase==2 it
// is part of the definition; otherwise it is emitted as a normal line.
func (p *proc) emitContent(text string) {
	switch p.tpPhase {
	case 1:
		// Emit the term as a bold heading and switch to definition mode.
		p.emit("\n" + text + "  \n")
		p.tpPhase = 2
	case 2:
		p.emit(text + "\n")
	default:
		p.emit(text + "\n")
	}
}

// flushTP closes an open tagged paragraph.
func (p *proc) flushTP() {
	if p.tpPhase != 0 {
		p.nl()
		p.tpPhase = 0
	}
}

// flushTable emits the accumulated table rows as a GitHub-Flavoured Markdown
// table and resets table state.
func (p *proc) flushTable() {
	if len(p.tableRows) == 0 {
		return
	}
	p.emit("| | |\n|---|---|\n")
	for _, row := range p.tableRows {
		c0 := strings.ReplaceAll(row[0], "|", `\|`)
		c1 := strings.ReplaceAll(row[1], "|", `\|`)
		p.emit(fmt.Sprintf("| %s | %s |\n", c0, c1))
	}
	p.nl()
	p.tableRows = nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// inline converts troff inline escapes in s to their Markdown equivalents.
func inline(s string) string {
	s = strings.TrimRight(s, " \t")
	var b strings.Builder
	boldOpen := false
	italicOpen := false

	closeFonts := func() {
		if boldOpen {
			b.WriteString("**")
			boldOpen = false
		}
		if italicOpen {
			b.WriteString("_")
			italicOpen = false
		}
	}

	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		switch s[i+1] {
		case '-':
			b.WriteByte('-')
			i += 2
		case '&':
			i += 2 // zero-width joiner — discard
		case '"':
			// Inline comment: discard the rest of the string.
			return strings.TrimRight(b.String(), " \t")
		case 'f':
			if i+2 >= len(s) {
				b.WriteByte(s[i])
				i++
				continue
			}
			switch s[i+2] {
			case 'B':
				closeFonts()
				b.WriteString("**")
				boldOpen = true
				i += 3
			case 'I':
				closeFonts()
				b.WriteString("_")
				italicOpen = true
				i += 3
			case 'R', 'P':
				closeFonts()
				i += 3
			default:
				b.WriteByte(s[i])
				i++
			}
		default:
			// Unknown escape: emit the escaped character.
			b.WriteByte(s[i+1])
			i += 2
		}
	}
	closeFonts()
	return b.String()
}

// altFonts builds alternating bold/plain text from a slice of man arguments.
// startBold=true means the first argument is bold (.BR); false means plain (.RB).
func altFonts(args []string, startBold bool) string {
	var b strings.Builder
	for i, a := range args {
		bold := (i%2 == 0) == startBold
		text := inline(a)
		if bold {
			b.WriteString("**")
			b.WriteString(text)
			b.WriteString("**")
		} else {
			b.WriteString(text)
		}
	}
	return b.String()
}

// isTableFormat returns true for tbl(1) format lines like "l l." that define
// column alignment — these are skipped during table row collection.
func isTableFormat(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasSuffix(line, ".") && !strings.Contains(line, `\`) && !strings.Contains(line, "\t")
}

// plainText converts troff escapes to plain text, dropping font markers.
// Used inside code blocks where Markdown emphasis must not be emitted.
func plainText(s string) string {
	s = strings.TrimRight(s, " \t")
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			i++
			continue
		}
		switch s[i+1] {
		case '-':
			b.WriteByte('-')
			i += 2
		case '&':
			i += 2
		case '"':
			return b.String()
		case 'f':
			if i+2 < len(s) {
				i += 3 // skip \fX — discard font change
			} else {
				i++
			}
		default:
			b.WriteByte(s[i+1])
			i += 2
		}
	}
	return b.String()
}

// splitArgs splits a man macro line into the macro name and its arguments,
// respecting double-quoted strings (which may contain spaces).
func splitArgs(line string) []string {
	var result []string
	i := 0
	for i < len(line) {
		// Skip whitespace.
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		if line[i] == '"' {
			// Quoted argument.
			i++
			start := i
			for i < len(line) && line[i] != '"' {
				i++
			}
			result = append(result, line[start:i])
			if i < len(line) {
				i++ // skip closing quote
			}
		} else {
			// Unquoted word.
			start := i
			for i < len(line) && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			result = append(result, line[start:i])
		}
	}
	return result
}
