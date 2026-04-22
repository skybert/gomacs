package editor

import (
	"testing"
)

func TestFuzzyMatch_Prefix(t *testing.T) {
	if !fuzzyMatch("forward-char", "forward") {
		t.Error("fuzzyMatch(forward-char, forward) should be true")
	}
}

func TestFuzzyMatch_Subsequence(t *testing.T) {
	if !fuzzyMatch("execute-extended-command", "exc") {
		t.Error("fuzzyMatch subsequence should be true")
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	if fuzzyMatch("forward-char", "zzz") {
		t.Error("fuzzyMatch should be false for non-subsequence")
	}
}

func TestFuzzyMatch_Empty(t *testing.T) {
	if !fuzzyMatch("anything", "") {
		t.Error("empty query should match everything")
	}
}

func TestFuzzyScore_Prefix(t *testing.T) {
	if got := fuzzyScore("man", "man"); got != 0 {
		t.Errorf("exact prefix: score = %d, want 0", got)
	}
}

func TestFuzzyScore_Substring(t *testing.T) {
	if got := fuzzyScore("command", "man"); got != 1 {
		t.Errorf("substring: score = %d, want 1", got)
	}
}

func TestFuzzyScore_PrefixBeatsSubstring(t *testing.T) {
	prefix := fuzzyScore("man", "man")
	sub := fuzzyScore("command", "man")
	if prefix >= sub {
		t.Errorf("prefix score (%d) should be < substring score (%d)", prefix, sub)
	}
}

func TestPushCommandLRU_Deduplicates(t *testing.T) {
	e := newTestEditor("")
	e.pushCommandLRU("save-buffer")
	e.pushCommandLRU("find-file")
	e.pushCommandLRU("save-buffer") // push again
	if e.commandLRU[0] != "save-buffer" {
		t.Errorf("first = %q, want \"save-buffer\"", e.commandLRU[0])
	}
	// save-buffer should appear only once.
	count := 0
	for _, n := range e.commandLRU {
		if n == "save-buffer" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("save-buffer appears %d times, want 1", count)
	}
}

func TestPushCommandLRU_Cap(t *testing.T) {
	e := newTestEditor("")
	for i := range commandLRUMax + 10 {
		e.pushCommandLRU(string(rune('a' + i%26)))
	}
	if len(e.commandLRU) > commandLRUMax {
		t.Errorf("LRU length = %d, want <= %d", len(e.commandLRU), commandLRUMax)
	}
}

func TestCommonPrefix_Empty(t *testing.T) {
	if got := commonPrefix([]string{}); got != "" {
		t.Errorf("empty input: got %q, want \"\"", got)
	}
}

func TestCommonPrefix_Single(t *testing.T) {
	if got := commonPrefix([]string{"hello"}); got != "hello" {
		t.Errorf("single: got %q, want \"hello\"", got)
	}
}

func TestCommonPrefix_Common(t *testing.T) {
	if got := commonPrefix([]string{"forward-char", "forward-word", "forward-list"}); got != "forward-" {
		t.Errorf("got %q, want \"forward-\"", got)
	}
}

func TestCommonPrefix_NoCommon(t *testing.T) {
	if got := commonPrefix([]string{"abc", "xyz"}); got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}

func TestModeFromShebang(t *testing.T) {
	cases := []struct {
		shebang string
		want    string
	}{
		{"#!/bin/bash\necho hi\n", "bash"},
		{"#!/usr/bin/env bash\necho hi\n", "bash"},
		{"#! /bin/bash\necho hi\n", "bash"},
		{"#! /usr/bin/env bash\necho hi\n", "bash"},
		{"#!/bin/sh\n", "bash"},
		{"#!/usr/bin/env sh\n", "bash"},
		{"#!/usr/bin/perl\n", "perl"},
		{"#!/usr/bin/env perl\n", "perl"},
		{"#!/usr/bin/python\n", "python"},
		{"#!/usr/bin/python3\n", "python"},
		{"#!/usr/bin/python2\n", "python"},
		{"#!/usr/bin/python3.10\n", "python"},
		{"#!/usr/bin/env python3.10\n", "python"},
		{"#!/usr/bin/env python3.12\n", "python"},
		{"# not a shebang\n", ""},
		{"", ""},
		{"no shebang at all", ""},
	}
	for _, tc := range cases {
		got := modeFromShebang(tc.shebang)
		if got != tc.want {
			t.Errorf("modeFromShebang(%q) = %q, want %q", tc.shebang, got, tc.want)
		}
	}
}

func TestStepToCamelCase(t *testing.T) {
	cases := []struct {
		step string
		want string
	}{
		{"user logs in", "UserLogsIn"},
		{"the user is logged in", "TheUserIsLoggedIn"},
		{"user enters \"admin\" as the username", "UserEntersAsTheUsername"},
		{"the user has 42 apples", "TheUserHasApples"},
		{"user is logged in as <role>", "UserIsLoggedInAs"},
		{"I am on the login page", "IAmOnTheLoginPage"},
		// Mixed / upper case input must normalise to the same result.
		{"User Logs In", "UserLogsIn"},
		{"USER LOGS IN", "UserLogsIn"},
		{"The USER is LOGGED IN", "TheUserIsLoggedIn"},
	}
	for _, tc := range cases {
		got := stepToCamelCase(tc.step)
		if got != tc.want {
			t.Errorf("stepToCamelCase(%q) = %q, want %q", tc.step, got, tc.want)
		}
	}
}

func TestGherkinStepAtPoint(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"Given user logs in\n", "user logs in"},
		{"  When I click submit\n", "I click submit"},
		{"  Then the response is 200\n", "the response is 200"},
		{"  And the cookie is set\n", "the cookie is set"},
		{"Feature: login\n", ""},
		{"  Scenario: test\n", ""},
		{"  | col1 | col2 |\n", ""},
		{"  # a comment\n", ""},
	}
	for _, tc := range cases {
		buf := newTestEditor(tc.content).ActiveBuffer()
		buf.SetPoint(0)
		got := gherkinStepAtPoint(buf)
		if got != tc.want {
			t.Errorf("gherkinStepAtPoint(%q) = %q, want %q", tc.content, got, tc.want)
		}
	}
}

func TestParseGrepLines(t *testing.T) {
	root := "/project"
	output := "./steps/login.go:42:func (s *Suite) UserLogsIn() error {\n" +
		"./steps/login.go:43:	// implementation\n"
	matches := parseGrepLines(output, root)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].File != "/project/steps/login.go" {
		t.Errorf("File = %q, want \"/project/steps/login.go\"", matches[0].File)
	}
	if matches[0].Line != 42 {
		t.Errorf("Line = %d, want 42", matches[0].Line)
	}
}
