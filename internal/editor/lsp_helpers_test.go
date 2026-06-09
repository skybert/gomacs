package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/skybert/gomacs/internal/lsp"
)

// ---------------------------------------------------------------------------
// parseSingleLocation
// ---------------------------------------------------------------------------

func TestParseSingleLocationNull(t *testing.T) {
	loc, err := parseSingleLocation(json.RawMessage("null"))
	if err != nil || loc.URI != "" {
		t.Errorf("parseSingleLocation(null) = (%v, %v), want zero,nil", loc, err)
	}
	loc, err = parseSingleLocation(nil)
	if err != nil || loc.URI != "" {
		t.Errorf("parseSingleLocation(nil) = (%v, %v), want zero,nil", loc, err)
	}
}

func TestParseSingleLocationSingle(t *testing.T) {
	raw := json.RawMessage(`{"uri":"file:///tmp/foo.go","range":{"start":{"line":3,"character":2},"end":{"line":3,"character":7}}}`)
	loc, err := parseSingleLocation(raw)
	if err != nil {
		t.Fatalf("parseSingleLocation: %v", err)
	}
	if loc.URI != "file:///tmp/foo.go" {
		t.Errorf("URI = %q", loc.URI)
	}
	if loc.Range.Start.Line != 3 {
		t.Errorf("Start.Line = %d, want 3", loc.Range.Start.Line)
	}
}

func TestParseSingleLocationArray(t *testing.T) {
	raw := json.RawMessage(`[{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":1}}},{"uri":"file:///b.go","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":1}}}]`)
	loc, err := parseSingleLocation(raw)
	if err != nil {
		t.Fatalf("parseSingleLocation: %v", err)
	}
	if loc.URI != "file:///a.go" {
		t.Errorf("expected first location, got URI=%q", loc.URI)
	}
}

func TestParseSingleLocationEmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	loc, err := parseSingleLocation(raw)
	if err != nil || loc.URI != "" {
		t.Errorf("parseSingleLocation([]) = (%v, %v), want zero,nil", loc, err)
	}
}

// ---------------------------------------------------------------------------
// parseLocations
// ---------------------------------------------------------------------------

func TestParseLocationsNull(t *testing.T) {
	if locs := parseLocations(json.RawMessage("null")); locs != nil {
		t.Errorf("parseLocations(null) = %v, want nil", locs)
	}
	if locs := parseLocations(nil); locs != nil {
		t.Errorf("parseLocations(nil) = %v, want nil", locs)
	}
}

func TestParseLocationsArray(t *testing.T) {
	raw := json.RawMessage(`[{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":1}}},{"uri":"file:///b.go","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":1}}}]`)
	locs := parseLocations(raw)
	if len(locs) != 2 {
		t.Fatalf("len = %d, want 2", len(locs))
	}
	if locs[0].URI != "file:///a.go" || locs[1].URI != "file:///b.go" {
		t.Errorf("URIs = %q, %q", locs[0].URI, locs[1].URI)
	}
}

func TestParseLocationsBadJSON(t *testing.T) {
	if locs := parseLocations(json.RawMessage(`not json`)); locs != nil {
		t.Errorf("expected nil for invalid JSON, got %v", locs)
	}
}

// ---------------------------------------------------------------------------
// extractHoverText
// ---------------------------------------------------------------------------

func TestExtractHoverTextNull(t *testing.T) {
	if got := extractHoverText(nil); got != "" {
		t.Errorf("extractHoverText(nil) = %q", got)
	}
	if got := extractHoverText(json.RawMessage("null")); got != "" {
		t.Errorf("extractHoverText(null) = %q", got)
	}
}

func TestExtractHoverTextMarkupContent(t *testing.T) {
	raw := json.RawMessage(`{"contents":{"kind":"markdown","value":"  hello world  "}}`)
	if got := extractHoverText(raw); got != "hello world" {
		t.Errorf("extractHoverText = %q, want %q", got, "hello world")
	}
}

func TestExtractHoverTextPlainString(t *testing.T) {
	raw := json.RawMessage(`{"contents":"  plain text  "}`)
	if got := extractHoverText(raw); got != "plain text" {
		t.Errorf("extractHoverText = %q, want %q", got, "plain text")
	}
}

func TestExtractHoverTextNoContents(t *testing.T) {
	raw := json.RawMessage(`{}`)
	if got := extractHoverText(raw); got != "" {
		t.Errorf("extractHoverText empty = %q", got)
	}
}

func TestExtractHoverTextBadJSON(t *testing.T) {
	if got := extractHoverText(json.RawMessage(`not json`)); got != "" {
		t.Errorf("extractHoverText invalid = %q", got)
	}
}

// ---------------------------------------------------------------------------
// wrapDocText
// ---------------------------------------------------------------------------

func TestWrapDocTextSimple(t *testing.T) {
	got := wrapDocText("hello world", 80)
	if len(got) != 1 || got[0] != "hello world" {
		t.Errorf("wrapDocText = %v, want [\"hello world\"]", got)
	}
}

func TestWrapDocTextWrapsLongLines(t *testing.T) {
	got := wrapDocText("aaaa bbbb cccc dddd", 10)
	// expect to wrap at word boundaries: "aaaa bbbb" (9 cols), "cccc dddd"
	if len(got) != 2 {
		t.Errorf("expected 2 lines, got %d: %v", len(got), got)
	}
}

func TestWrapDocTextSplitsOnNewlines(t *testing.T) {
	got := wrapDocText("first\nsecond\nthird", 80)
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWrapDocTextTrimsTrailingBlank(t *testing.T) {
	got := wrapDocText("hello\n\n\n", 80)
	if len(got) != 1 {
		t.Errorf("expected trailing blanks trimmed, got %v", got)
	}
}

func TestWrapDocTextEmpty(t *testing.T) {
	got := wrapDocText("", 80)
	if len(got) != 0 {
		t.Errorf("wrapDocText empty = %v, want []", got)
	}
}

// ---------------------------------------------------------------------------
// lspReadFileLine
// ---------------------------------------------------------------------------

func TestLspReadFileLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	content := "line0\nline1\r\nline2\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lspReadFileLine(p, 0); got != "line0" {
		t.Errorf("line 0 = %q, want %q", got, "line0")
	}
	if got := lspReadFileLine(p, 1); got != "line1" {
		t.Errorf("line 1 = %q, want %q (CR trimmed)", got, "line1")
	}
	if got := lspReadFileLine(p, 2); got != "line2" {
		t.Errorf("line 2 = %q, want %q", got, "line2")
	}
	if got := lspReadFileLine(p, 99); got != "" {
		t.Errorf("out of range = %q, want empty", got)
	}
}

func TestLspReadFileLineMissingFile(t *testing.T) {
	if got := lspReadFileLine("/nonexistent/file.xyzzy", 0); got != "" {
		t.Errorf("missing file = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// findBufferByFilename
// ---------------------------------------------------------------------------

func TestFindBufferByFilename(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetFilename("/tmp/a.go")
	if got := e.findBufferByFilename("/tmp/a.go"); got != buf(e) {
		t.Errorf("findBufferByFilename returned wrong buffer")
	}
	if got := e.findBufferByFilename("/nope"); got != nil {
		t.Errorf("expected nil for unknown filename, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// bufPointToLSP / lspPosToPoint
// ---------------------------------------------------------------------------

func TestBufPointToLSPSimple(t *testing.T) {
	e := newTestEditor("hello\nworld")
	b := buf(e)
	b.SetPoint(8) // 8 = "hello\nwo|rld"; line 2, col 2
	pos := e.bufPointToLSP(b)
	if pos.Line != 1 {
		t.Errorf("Line = %d, want 1 (0-based)", pos.Line)
	}
	if pos.Character != 2 {
		t.Errorf("Character = %d, want 2", pos.Character)
	}
}

func TestLspPosToPointRoundTrip(t *testing.T) {
	e := newTestEditor("abc\ndef\nghi")
	b := buf(e)
	for _, pt := range []int{0, 1, 4, 5, 7, 11} {
		b.SetPoint(pt)
		pos := e.bufPointToLSP(b)
		got := e.lspPosToPoint(b, pos)
		if got != pt {
			t.Errorf("round-trip pt=%d → pos=%+v → pt=%d", pt, pos, got)
		}
	}
}

func TestLspPosToPointPastEnd(t *testing.T) {
	e := newTestEditor("ab")
	b := buf(e)
	pos := lsp.Position{Line: 0, Character: 100}
	pt := e.lspPosToPoint(b, pos)
	if pt != b.Len() {
		t.Errorf("past-end pt = %d, want %d", pt, b.Len())
	}
}
