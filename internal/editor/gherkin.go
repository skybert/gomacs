package editor

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/skybert/gomacs/internal/buffer"
)

var (
	gherkinStepPrefixRe = regexp.MustCompile(`(?i)^(given|when|then|and|but|\*)\s+`)
	gherkinParamRe      = regexp.MustCompile(`"[^"]*"|<[^>]*>|\b\d+\b`)
	gherkinNonAlphaRe   = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

// gherkinStepAtPoint returns the step text (without the keyword) from the line
// at point, or "" if the line is not a Gherkin step line.
func gherkinStepAtPoint(buf *buffer.Buffer) string {
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := strings.TrimSpace(buf.Substring(bol, eol))
	loc := gherkinStepPrefixRe.FindStringIndex(line)
	if loc == nil {
		return ""
	}
	return strings.TrimSpace(line[loc[1]:])
}

// stepToCamelCase converts Gherkin step text to a Go CamelCase identifier.
// Quoted strings, angle-bracket parameters, and isolated numbers are stripped;
// the remaining words are title-cased and concatenated.
func stepToCamelCase(step string) string {
	step = gherkinParamRe.ReplaceAllString(step, " ")
	step = strings.ToLower(step) // normalise before title-casing
	parts := gherkinNonAlphaRe.Split(step, -1)
	var sb strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		sb.WriteRune(unicode.ToUpper(r[0]))
		sb.WriteString(string(r[1:]))
	}
	return sb.String()
}

// gherkinGrep searches for pattern in files matching glob under root.
// When root is inside a git repository it delegates to "git grep" (fast,
// searches only tracked files); otherwise it falls back to plain "grep -rEn".
// pattern is an extended regular expression; ignoreCase adds the -i flag.
func gherkinGrep(root, pattern, glob string, ignoreCase bool) string {
	if isGitRepo(root) {
		return gherkinGitGrep(root, pattern, glob, ignoreCase)
	}
	return gherkinPlainGrep(root, pattern, glob, ignoreCase)
}

// isGitRepo reports whether root is inside a git repository by running
// "git -C root rev-parse --git-dir".
func isGitRepo(root string) bool {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// gherkinGitGrep uses "git grep" — searches only tracked files and is
// significantly faster than a recursive filesystem walk.
// Output format: "path/to/file:linenum:content" (relative to root, no ./ prefix).
func gherkinGitGrep(root, pattern, glob string, ignoreCase bool) string {
	args := []string{"-C", root, "grep", "-En"}
	if ignoreCase {
		args = append(args, "-i")
	}
	args = append(args, pattern, "--", glob)
	cmd := exec.Command("git", args...)
	out, _ := cmd.Output()
	return string(out)
}

// gherkinPlainGrep falls back to "grep -rEn" for non-git trees.
// Output format: "./path/to/file:linenum:content" (relative to root).
func gherkinPlainGrep(root, pattern, glob string, ignoreCase bool) string {
	args := []string{"-rEn", "--include=" + glob}
	if ignoreCase {
		args = append(args, "-i")
	}
	args = append(args, pattern, ".")
	cmd := exec.Command("grep", args...)
	cmd.Dir = root
	out, _ := cmd.Output()
	return string(out)
}

// parseGrepLines parses "rel/path:linenum:content" grep output into
// compilationErrors with absolute paths rooted at root.
func parseGrepLines(output, root string) []compilationError {
	var errs []compilationError
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil || lineNum < 1 {
			continue
		}
		relPath := strings.TrimPrefix(parts[0], "./")
		errs = append(errs, compilationError{
			File: filepath.Join(root, relPath),
			Line: lineNum,
		})
	}
	return errs
}

// cmdGherkinFindDefinition implements M-. for Gherkin buffers.
//
// It converts the step at point to a CamelCase name (Go/gocuke) and also
// constructs a Java annotation pattern, then greps the project for both.
// One match → jump directly (M-, navigates back).
// Multiple matches → open a *gherkin-definitions* vc-grep buffer; the results
// are also fed into the next-error list so C-x ` / M-g n cycles through them.
func (e *Editor) cmdGherkinFindDefinition() {
	buf := e.ActiveBuffer()
	step := gherkinStepAtPoint(buf)
	if step == "" {
		e.Message("Not on a Gherkin step line")
		return
	}

	// Resolve project root from VC, falling back to the file's directory.
	root := ""
	if _, vcRoot := vcFind(vcDir(buf)); vcRoot != "" {
		root = vcRoot
	}
	if root == "" && buf.Filename() != "" {
		root = filepath.Dir(buf.Filename())
	}
	if root == "" {
		e.Message("Cannot determine project root")
		return
	}

	camel := stepToCamelCase(step)
	fromFile := buf.Filename()
	fromPoint := buf.Point()
	e.Message("Searching for %s…", camel)

	e.lspAsync(func() func() {
		// Go/gocuke: function name contains the CamelCase step identifier.
		goOut := gherkinGrep(root, `func.*`+regexp.QuoteMeta(camel), "*.go", true)

		// Java: @Given/@When/@Then/@And/@But annotation whose string contains
		// the step text (case-insensitive).
		javaPattern := `@(Given|When|Then|And|But).*"` + regexp.QuoteMeta(step)
		javaOut := gherkinGrep(root, javaPattern, "*.java", true)

		// Combine and deduplicate output.
		combined := strings.TrimSpace(goOut)
		if t := strings.TrimSpace(javaOut); t != "" {
			if combined != "" {
				combined += "\n" + t
			} else {
				combined = t
			}
		}

		matches := parseGrepLines(combined, root)

		return func() {
			switch len(matches) {
			case 0:
				e.Message("No definition found for: %s", step)

			case 1:
				// Push current position so M-, navigates back.
				e.lspDefStack = append(e.lspDefStack, lspDefPos{
					filename: fromFile,
					point:    fromPoint,
				})
				destBuf, err := e.loadFile(matches[0].File)
				if err != nil {
					e.Message("Cannot open %s: %v", matches[0].File, err)
					return
				}
				e.activeWin.SetBuf(destBuf)
				pos := destBuf.LineStart(matches[0].Line)
				destBuf.SetPoint(pos)
				e.activeWin.SetPoint(pos)
				e.syncWindowPoint(e.activeWin)
				e.activeWin.EnsurePointVisible()

			default:
				// Show all results in a grep buffer.
				e.vcShowOutput("*gherkin-definitions*", combined+"\n", "vc-grep")
				if rb := e.FindBuffer("*gherkin-definitions*"); rb != nil {
					e.vcLogRoots[rb] = root
				}
				// Populate compilationErrors so C-x ` / M-g n cycle through them.
				e.compilationErrors = matches
				e.compilationErrorIdx = -1
				e.Message("%d definitions found — C-x ` or M-g n to cycle", len(matches))
			}
		}
	})
}
