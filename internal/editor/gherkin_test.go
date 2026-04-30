package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// ---------------------------------------------------------------------------
// gherkinStepAtPoint
// ---------------------------------------------------------------------------

func TestGherkinStepAtPoint_GivenStep(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "  Given the user is logged in\n")
	buf.SetPoint(5) // somewhere on the line
	got := gherkinStepAtPoint(buf)
	want := "the user is logged in"
	if got != want {
		t.Errorf("gherkinStepAtPoint = %q, want %q", got, want)
	}
}

func TestGherkinStepAtPoint_WhenStep(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "When the user clicks submit\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	want := "the user clicks submit"
	if got != want {
		t.Errorf("gherkinStepAtPoint = %q, want %q", got, want)
	}
}

func TestGherkinStepAtPoint_ThenStep(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "Then the response is 200\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	want := "the response is 200"
	if got != want {
		t.Errorf("gherkinStepAtPoint = %q, want %q", got, want)
	}
}

func TestGherkinStepAtPoint_AndStep(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "And the email is sent\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	want := "the email is sent"
	if got != want {
		t.Errorf("gherkinStepAtPoint = %q, want %q", got, want)
	}
}

func TestGherkinStepAtPoint_StarStep(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "* the context is set\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	want := "the context is set"
	if got != want {
		t.Errorf("gherkinStepAtPoint = %q, want %q", got, want)
	}
}

func TestGherkinStepAtPoint_NonStepLine(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "Feature: login\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	if got != "" {
		t.Errorf("non-step line should return empty string, got %q", got)
	}
}

func TestGherkinStepAtPoint_ScenarioLine(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "  Scenario: user logs in\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	if got != "" {
		t.Errorf("scenario line should not be treated as a step, got %q", got)
	}
}

func TestGherkinStepAtPoint_CaseInsensitive(t *testing.T) {
	buf := buffer.NewWithContent("test.feature", "GIVEN a user exists\n")
	buf.SetPoint(0)
	got := gherkinStepAtPoint(buf)
	want := "a user exists"
	if got != want {
		t.Errorf("gherkinStepAtPoint = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// stepToCamelCase
// ---------------------------------------------------------------------------

func TestStepToCamelCase_Simple(t *testing.T) {
	got := stepToCamelCase("the user is logged in")
	want := "TheUserIsLoggedIn"
	if got != want {
		t.Errorf("stepToCamelCase = %q, want %q", got, want)
	}
}

func TestStepToCamelCase_QuotedStringsStripped(t *testing.T) {
	got := stepToCamelCase(`the user enters "admin" as username`)
	// "admin" should be stripped, leaving "the user enters as username"
	want := "TheUserEntersAsUsername"
	if got != want {
		t.Errorf("stepToCamelCase = %q, want %q", got, want)
	}
}

func TestStepToCamelCase_AngleBracketParamsStripped(t *testing.T) {
	got := stepToCamelCase("the <role> user logs in")
	want := "TheUserLogsIn"
	if got != want {
		t.Errorf("stepToCamelCase = %q, want %q", got, want)
	}
}

func TestStepToCamelCase_NumbersStripped(t *testing.T) {
	got := stepToCamelCase("the user waits 5 seconds")
	want := "TheUserWaitsSeconds"
	if got != want {
		t.Errorf("stepToCamelCase = %q, want %q", got, want)
	}
}

func TestStepToCamelCase_EmptyString(t *testing.T) {
	got := stepToCamelCase("")
	if got != "" {
		t.Errorf("stepToCamelCase(\"\") = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// parseGrepLines
// ---------------------------------------------------------------------------

func TestParseGrepLines_BasicOutput(t *testing.T) {
	output := "src/steps.go:42:func (s *Steps) TheUserIsLoggedIn() {\n" +
		"src/auth.go:17:func (a *Auth) HandleLogin() {\n"
	errs := parseGrepLines(output, "/project")
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].Line != 42 {
		t.Errorf("errs[0].Line = %d, want 42", errs[0].Line)
	}
	if errs[1].Line != 17 {
		t.Errorf("errs[1].Line = %d, want 17", errs[1].Line)
	}
}

func TestParseGrepLines_RootPrependedToPath(t *testing.T) {
	output := "steps/login.go:10:someContent\n"
	errs := parseGrepLines(output, "/myproject")
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].File != "/myproject/steps/login.go" {
		t.Errorf("File = %q, want %q", errs[0].File, "/myproject/steps/login.go")
	}
}

func TestParseGrepLines_DotSlashPrefixStripped(t *testing.T) {
	output := "./steps/login.go:5:content\n"
	errs := parseGrepLines(output, "/root")
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].File != "/root/steps/login.go" {
		t.Errorf("File = %q, want %q", errs[0].File, "/root/steps/login.go")
	}
}

func TestParseGrepLines_InvalidLinesSkipped(t *testing.T) {
	output := "not-a-valid-line\n" +
		"src/steps.go:abc:content\n" + // non-numeric line number
		"src/steps.go:0:content\n" + // line 0 is invalid
		"src/ok.go:3:good\n"
	errs := parseGrepLines(output, "/root")
	if len(errs) != 1 {
		t.Fatalf("expected 1 valid error, got %d", len(errs))
	}
	if errs[0].Line != 3 {
		t.Errorf("errs[0].Line = %d, want 3", errs[0].Line)
	}
}

func TestParseGrepLines_EmptyOutput(t *testing.T) {
	errs := parseGrepLines("", "/root")
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for empty output, got %d", len(errs))
	}
}
